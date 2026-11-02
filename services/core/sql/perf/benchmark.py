#!/usr/bin/env python3
"""Сравнительный замер производительности до/после миграции 0010.

    PERF_DB_URL=postgres://postgres:postgres@localhost:5433/barbershop_perf \
        python3 sql/perf/benchmark.py --out ../../docs/db/perf

Требования: БД с набором generate_perf_data.sql, схема на версии 0009;
psql и go в PATH. Сценарий:
  1) для каждого запроса 7 прогонов EXPLAIN (ANALYZE, BUFFERS), медиана времени,
     сохранение плана последнего прогона;
  2) стоимость записи: вставка 5 000 записей с составом (транзакция откатывается);
  3) применение миграции 0010 (cmd/migrate), ANALYZE, повтор шагов 1–2;
  4) сводная таблица в Markdown.
"""
import argparse
import json
import os
import re
import statistics
import subprocess
import sys

QUERIES_DIR = os.path.join(os.path.dirname(__file__), "..", "queries")
RUNS = 7


def psql(url, sql):
    r = subprocess.run(["psql", url, "-X", "-q", "-At", "-v", "ON_ERROR_STOP=1"],
                       input=sql, capture_output=True, text=True)
    if r.returncode != 0:
        sys.exit(r.stderr)
    return r.stdout


def query_file(name):
    with open(os.path.join(QUERIES_DIR, name), encoding="utf-8") as f:
        return f.read().strip().rstrip(";")


def bench(url, title, sql, params):
    """Медиана Execution Time по RUNS прогонам; возвращает (мс, план, buffers)."""
    prep = f"PREPARE q AS {sql};\n" if params is not None else ""
    run = f"EXPLAIN (ANALYZE, BUFFERS) EXECUTE q({params});" if params is not None \
        else f"EXPLAIN (ANALYZE, BUFFERS) {sql};"
    times, plan = [], ""
    for _ in range(RUNS):
        plan = psql(url, "SET plan_cache_mode = force_custom_plan;\n" + prep + run)
        times.append(float(re.search(r"Execution Time: ([\d.]+)", plan).group(1)))
    return statistics.median(times), plan, 0


def pick_params(url):
    barber, day = psql(url, """SELECT barber_id, work_date FROM crm.work_shifts
        WHERE work_date = current_date - 7 ORDER BY barber_id LIMIT 1""").strip().split("|")
    fut_barber, fut_day = psql(url, """SELECT barber_id, work_date FROM crm.work_shifts
        WHERE work_date = current_date + 3 ORDER BY barber_id LIMIT 1""").strip().split("|")
    client = psql(url, """SELECT client_id FROM crm.appointments GROUP BY client_id
        ORDER BY count(*) DESC, client_id LIMIT 1""").strip()
    return barber, day, fut_barber, fut_day, client


def cases(url):
    barber, day, fb, fd, client = pick_params(url)
    return [
        ("P1", "Q01 расписание мастера на день", query_file("q01_barber_day_schedule.sql"), f"{barber}, '{day}'"),
        ("P2", "Q02 история визитов клиента", query_file("q02_client_history.sql"), f"{client}"),
        ("P3", "Свободные слоты мастера (crm.free_slots)",
         f"SELECT * FROM crm.free_slots({fb}, '{fd}', 45, false)", None),
        ("P4", "Поиск клиентов по подстроке + статистика (список API)",
         """SELECT c.client_id, c.full_name, st.visits_count, st.last_visit_at
            FROM crm.clients c JOIN crm.v_client_stats st ON st.client_id = c.client_id
            WHERE {search} ORDER BY c.full_name LIMIT 20""", None),
        ("P5", "Q03 выручка филиалов за 3 месяца", query_file("q03_revenue_by_branch_month.sql"),
         "now() - interval '3 months', now()"),
        ("P6", "Q04 рейтинг мастеров за месяц", query_file("q04_barber_ranking.sql"),
         "date_trunc('month', now()) - interval '1 month', date_trunc('month', now())"),
    ]


SEARCH_BEFORE = "(c.full_name ILIKE '%петрова%' OR c.phone LIKE '%петрова%' OR c.email ILIKE '%петрова%')"
SEARCH_AFTER = "crm.client_search_text(c.full_name, c.phone, c.email) LIKE '%петрова%'"


def write_cost(url):
    """Время вставки 5 000 записей в откатываемой транзакции. Статус no_show не участвует
    в EXCLUDE (нет конфликтов с существующими записями), но попадает во все B-tree индексы."""
    out = psql(url, r"""
        BEGIN;
        SET LOCAL crm.history_load = 'on';
        SET LOCAL crm.audit_disabled = 'on';
        CREATE TEMP TABLE t0 AS SELECT clock_timestamp() AS ts;
        WITH ins AS (
          INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at, status)
          SELECT 1 + (g % 30000), ws.barber_id, crm.shift_start_ts(ws.work_date, '18:30'),
                 crm.shift_start_ts(ws.work_date, '18:55'), 'no_show'
          FROM generate_series(1, 5000) g
          JOIN LATERAL (SELECT * FROM crm.work_shifts w WHERE w.work_date > current_date - 700
                        ORDER BY w.shift_id OFFSET g LIMIT 1) ws ON true
          RETURNING appointment_id)
        SELECT count(*) FROM ins;
        SELECT round(extract(epoch FROM clock_timestamp() - ts) * 1000) FROM t0;
        ROLLBACK;""")
    return int(out.strip().split("\n")[-1])


def index_sizes(url):
    rows = psql(url, """SELECT indexrelid::regclass, pg_size_pretty(pg_relation_size(indexrelid))
        FROM pg_index i JOIN pg_class c ON c.oid = i.indrelid
        WHERE c.relname IN ('appointments', 'clients') ORDER BY 1""")
    return [r.split("|") for r in rows.strip().split("\n") if r]


def run_phase(url, phase, outdir):
    res = {}
    for code, title, sql, params in cases(url):
        if code == "P4":
            sql = sql.format(search=SEARCH_BEFORE if phase == "before" else SEARCH_AFTER)
        ms, plan, _ = bench(url, title, sql, params)
        res[code] = {"title": title, "ms": ms}
        with open(os.path.join(outdir, f"{code}_{phase}.txt"), "w", encoding="utf-8") as f:
            f.write(f"-- {code}. {title} ({phase})\n-- медиана Execution Time из {RUNS} прогонов: {ms:.2f} ms\n\n{plan}")
        print(f"  {code} {phase}: {ms:.2f} ms", flush=True)
    res["write_ms"] = write_cost(url)
    res["indexes"] = index_sizes(url)
    print(f"  insert 5000 ({phase}): {res['write_ms']} ms", flush=True)
    return res


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default="perf-results")
    args = ap.parse_args()
    url = os.environ["PERF_DB_URL"]
    os.makedirs(args.out, exist_ok=True)
    rows = psql(url, "SELECT count(*) FROM crm.appointments").strip()
    print(f"appointments: {rows}")

    before = run_phase(url, "before", args.out)
    env = dict(os.environ, CORE_MIGRATE_DB_URL=url)
    subprocess.run(["go", "run", "./cmd/migrate", "up", "-to", "10"], check=True, env=env,
                   cwd=os.path.join(os.path.dirname(__file__), "..", ".."))
    psql(url, "ANALYZE;")
    after = run_phase(url, "after", args.out)

    with open(os.path.join(args.out, "results.json"), "w", encoding="utf-8") as f:
        json.dump({"appointments": int(rows), "before": before, "after": after}, f, ensure_ascii=False, indent=2)
    print("\n| № | Запрос | До, мс | После, мс | Ускорение |\n|---|---|---|---|---|")
    for code in sorted(k for k in before if k.startswith("P")):
        b, a = before[code]["ms"], after[code]["ms"]
        print(f"| {code} | {before[code]['title']} | {b:.2f} | {a:.2f} | ×{b / a:.1f} |")
    print(f"| W | Вставка 5 000 записей | {before['write_ms']} | {after['write_ms']} | ×{before['write_ms'] / after['write_ms']:.2f} |")


if __name__ == "__main__":
    main()
