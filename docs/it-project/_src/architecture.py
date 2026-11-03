"""Архитектурные диаграммы Barber CRM (C4: контекст, контейнеры, компоненты;
развёртывание; последовательность обработки запроса).

Запуск:  python3 docs/it-project/_src/architecture.py [путь к dot]
Результат: docs/it-project/04-architecture/*.{svg,png}
Диаграммы 1–4 строятся Graphviz (dot), диаграмма 5 — svgkit.
"""
import os
import subprocess
import sys

sys.path.insert(0, os.path.dirname(__file__))
from svgkit import Canvas, text_width, wrap  # noqa: E402

OUT = os.path.join(os.path.dirname(__file__), "..", "04-architecture")
DOT = sys.argv[1] if len(sys.argv) > 1 else "dot"
FONT = "Liberation Sans"

PERSON = "#08427b"
SYSTEM = "#1168bd"
EXTERNAL = "#8a8a8a"
CONTAINER = "#438dd5"
DB = "#2f7d4f"
INFRA = "#7a5c99"
COMPONENT = "#85bbf0"


def pad(s):
    return s + "&#160;" * (2 + len(s) // 6)


def node(nid, title, tech, desc, color, shape="box", fc="white", w=None):
    tech_html = f'<br/><font point-size="10">[{tech}]</font>' if tech else ""
    desc_html = "".join(f'<br/><font point-size="10">{pad(line)}</font>' for line in wrap(desc, 230, 10)) if desc else ""
    label = f'<<b>{pad(title)}</b>{tech_html}{desc_html}>'
    extra = f', width={w}' if w else ""
    return (f'"{nid}" [label={label}, shape={shape}, style="rounded,filled", fillcolor="{color}", '
            f'fontcolor="{fc}", color="{color}", margin="0.25,0.12"{extra}];')


def cyl(nid, title, tech, desc, color=DB):
    return node(nid, title, tech, desc, color, shape="cylinder").replace('style="rounded,filled"', 'style="filled"')


def edge(a, b, label="", style="solid", both=False):
    lab = f', label=<<font point-size="10">{pad(label)}</font>>' if label else ""
    d = ', dir=both' if both else ""
    return f'"{a}" -> "{b}" [color="#555555", fontcolor="#333333", style={style}{lab}{d}];'


def graph(name, title, body, rankdir="TB", extra=""):
    src = "\n".join([
        f'digraph "{name}" {{',
        f'graph [rankdir={rankdir}, fontname="{FONT}", fontsize=22, labelloc=t, label=<<b>{title}</b>>, '
        f'nodesep=0.55, ranksep=0.8, pad=0.4, splines=spline, newrank=true {extra}];',
        f'node [fontname="{FONT}", fontsize=13];',
        f'edge [fontname="{FONT}", fontsize=10, arrowsize=0.8, penwidth=1.3];',
        *body, "}"])
    path = os.path.join(OUT, name)
    with open(path + ".dot", "w", encoding="utf-8") as f:
        f.write(src)
    for fmt, dpi in (("svg", 72), ("png", 110)):
        subprocess.run([DOT, f"-T{fmt}", f"-Gdpi={dpi}", "-o", f"{path}.{fmt}", path + ".dot"], check=True)


def legend(items):
    rows = "".join(f'<tr><td bgcolor="{c}" width="22"> </td><td align="left">{pad(t)}</td></tr>' for c, t in items)
    return (f'"legend" [shape=plain, label=<<table border="1" cellborder="0" cellspacing="4" cellpadding="3">'
            f'<tr><td colspan="2"><b>Обозначения</b></td></tr>{rows}</table>>];')


# ════════════════════════════════════════════════════════════════════════════
# 1. Контекст системы (C4, уровень 1)
# ════════════════════════════════════════════════════════════════════════════
def context():
    body = [
        node("client", "Клиент", "", "Записывается онлайн, приходит на визит, оплачивает, оставляет отзыв", PERSON, w=2.6),
        node("admin", "Администратор филиала", "", "Справочники, график смен, запись по телефону, расчёт, импорт клиентов", PERSON, w=2.6),
        node("barber", "Барбер", "", "Своё расписание, начало визита и оказание услуг", PERSON, w=2.6),
        node("manager", "Управляющий сетью", "", "Выручка, загрузка мастеров, RFM, отток, KPI", PERSON, w=2.6),
        node("crm", "Barber CRM", "веб-приложение + REST API + PostgreSQL",
             "CRM сети барбершопов: онлайн-запись, расписание, клиенты и лояльность, оплаты, аналитика, аудит", SYSTEM, w=4),
        node("legacy", "Прежняя CRM / таблицы", "внешний источник", "Выгрузка клиентской базы в CSV", EXTERNAL),
        node("ops", "Мониторинг", "Prometheus + Grafana", "Метрики сервисов", EXTERNAL),
        "{rank=same; client; admin; barber; manager;}",
        edge("client", "crm", "запись, отмена, слоты (HTTPS)"),
        edge("admin", "crm", "управление, расчёт, импорт (HTTPS)"),
        edge("barber", "crm", "расписание, визиты (HTTPS)"),
        edge("manager", "crm", "отчёты и KPI (HTTPS)"),
        edge("legacy", "crm", "CSV-файл", style="dashed"),
        edge("crm", "ops", "/metrics", style="dashed"),
        edge("crm", "client", "подтверждения, чеки, бонусы", style="dashed"),
        legend([(PERSON, "Пользователь"), (SYSTEM, "Проектируемая система"), (EXTERNAL, "Внешняя система")]),
    ]
    graph("arch-1-context", "Архитектура. Уровень 1: контекст системы (C4)", body)


# ════════════════════════════════════════════════════════════════════════════
# 2. Контейнеры (C4, уровень 2)
# ════════════════════════════════════════════════════════════════════════════
def containers():
    body = [
        node("users", "Пользователи", "браузер", "Клиенты, администраторы, барберы, управляющий", PERSON),
        'subgraph cluster_sys { label=<<b>Barber CRM (docker compose)</b>>; style="dashed,rounded"; color="#1168bd"; fontsize=15;',
        node("front", "Frontend", "React SPA, nginx", "Интерфейс записи и рабочего места", CONTAINER),
        node("gw", "API Gateway", "Go, chi, JWT, :8080", "Единая точка входа: аутентификация, rate limit, CORS, маршрутизация, метрики", CONTAINER, w=3.2),
        'subgraph cluster_ms { label=<<b>Микросервисы исходного проекта</b>>; style="rounded"; color="#999999";',
        node("staff", "staff", "Go, gRPC", "Мастера, услуги, график, вход", CONTAINER),
        node("booking", "booking", "Go, gRPC", "Бронирования, слоты", CONTAINER),
        node("clientsvc", "client", "Go, gRPC", "Профили клиентов", CONTAINER),
        node("analytics", "analytics", "Go, gRPC", "Статистика", CONTAINER),
        "}",
        'subgraph cluster_core { label=<<b>Курсовой проект: единая БД сети</b>>; style="rounded,filled"; fillcolor="#f2f8ff"; color="#1168bd";',
        node("core", "core", "Go, chi, pgx, :8090", "REST API: CRUD, бизнес-сценарии, отчёты, импорт, аудит; OpenAPI /docs", CONTAINER, w=3.2),
        node("coremig", "core-migrate / core-seed", "Go, psql", "Версионные миграции и воспроизводимое наполнение", CONTAINER),
        cyl("pgcore", "PostgreSQL 16: barbershop", "схемы crm, audit, staging",
            "15 таблиц, ограничения, триггеры, функции, представления, MV, роли"),
        "}",
        cyl("pgms", "PostgreSQL 16", "staff_db, booking_db, client_db", "Базы микросервисов"),
        cyl("redis", "Redis 7", "кэш", "Кэш расписания и сессий", INFRA),
        cyl("kafka", "Kafka (KRaft)", "bookings.events", "События бронирований", INFRA),
        cyl("ch", "ClickHouse", "analytics", "Аналитическое хранилище", INFRA),
        node("prom", "Prometheus + Grafana", "", "Сбор метрик и дашборды", EXTERNAL),
        "}",
        edge("users", "front", "HTTPS"),
        edge("front", "gw", "REST/JSON"),
        edge("users", "gw", "REST/JSON", style="dashed"),
        edge("gw", "staff", "gRPC"), edge("gw", "booking", "gRPC"), edge("gw", "clientsvc", "gRPC"),
        edge("gw", "analytics", "gRPC"),
        edge("gw", "core", "HTTP /api/core/* + X-User"),
        edge("core", "pgcore", "pgx, роль crm_app (мин. права)"),
        edge("coremig", "pgcore", "роль crm_owner"),
        edge("staff", "pgms", "SQL"), edge("booking", "pgms", "SQL"), edge("clientsvc", "pgms", "SQL"),
        edge("staff", "redis", ""), edge("booking", "redis", ""),
        edge("booking", "kafka", "publish"), edge("kafka", "analytics", "consume"), edge("kafka", "clientsvc", "consume"),
        edge("analytics", "ch", "SQL"),
        edge("prom", "gw", "/metrics", style="dashed"),
        legend([(PERSON, "Пользователь"), (CONTAINER, "Приложение/сервис"), (DB, "База данных PostgreSQL"),
                (INFRA, "Инфраструктура данных"), (EXTERNAL, "Наблюдаемость")]),
    ]
    graph("arch-2-containers", "Архитектура. Уровень 2: контейнеры (C4)", body)


# ════════════════════════════════════════════════════════════════════════════
# 3. Компоненты сервиса core и серверная логика БД (C4, уровень 3)
# ════════════════════════════════════════════════════════════════════════════
def components():
    C = COMPONENT
    body = [
        node("gw", "API Gateway", "JWT", "Проверяет токен, передаёт X-User", CONTAINER),
        'subgraph cluster_core { label=<<b>Сервис core (Go)</b>>; style="rounded"; color="#1168bd"; fontsize=15;',
        node("router", "Router и middleware", "chi", "RequestID, логирование, recover, таймаут 30 с, X-User → контекст", C, fc="black"),
        node("handlers", "HTTP-обработчики", "internal/api", "Справочники, клиенты, смены, записи, отчёты, импорт, аудит", C, fc="black"),
        node("validate", "Валидация и параметры", "internal/api", "Формат полей, белый список сортировки, пагинация ≤ 100", C, fc="black"),
        node("docs", "OpenAPI", "api/openapi.yaml", "/openapi.yaml и Swagger UI /docs", C, fc="black"),
        node("store", "Слой данных (store)", "pgx", "Параметризованный SQL, фильтры, страницы, вызов функций БД", C, fc="black"),
        node("reports", "Исполнитель отчётов", "embed sql/queries", "11 запросов, REPEATABLE READ READ ONLY", C, fc="black"),
        node("tx", "Транзакции", "internal/db", "InTx: SET LOCAL crm.app_user, повтор при 40001/40P01", C, fc="black"),
        node("apperr", "Маппинг ошибок", "internal/apperr", "SQLSTATE → HTTP: 23505/23P01→409, 23514/CR0xx→422, 42501→403", C, fc="black"),
        node("kpi", "Планировщик KPI", "goroutine", "Периодический REFRESH MV", C, fc="black"),
        node("cli", "CLI", "cmd/migrate, importer, concurrency-demo", "Миграции, пакетная загрузка, демонстрация", C, fc="black"),
        "}",
        'subgraph cluster_db { label=<<b>PostgreSQL 16 — БД barbershop</b>>; style="rounded,filled"; fillcolor="#f0f8f2"; color="#2f7d4f"; fontsize=15;',
        node("tables", "Схема crm: 15 таблиц", "PK, FK, UNIQUE, CHECK, EXCLUDE gist", "Целостность и инварианты BR-1, BR-2, BR-6, BR-8", DB),
        node("triggers", "Триггеры", "PL/pgSQL", "Смены, переходы статусов, оплаты, отзывы, бонусы, уровни, updated_at, автосоздание 1:1", DB),
        node("funcs", "Функции бизнес-операций", "SECURITY DEFINER", "free_slots, book/complete/cancel/reschedule_appointment, refresh_kpi", DB),
        node("views", "Представления и MV", "", "v_appointment_details, v_client_stats, mv_barber_monthly_kpi", DB),
        node("audit", "Схема audit", "триггер audit.tg_audit", "audit_log: операция, старые/новые данные, пользователь", DB),
        node("staging", "Схема staging", "process_client_import", "Пакеты, сырые строки, журнал ошибок", DB),
        node("roles", "Роли", "crm_owner / crm_app / crm_readonly", "Минимальные привилегии, столбцовые права", DB),
        "}",
        edge("gw", "router", "HTTP"),
        edge("router", "handlers"), edge("router", "docs"),
        edge("handlers", "validate"), edge("handlers", "store"), edge("handlers", "reports"),
        edge("handlers", "apperr", "ошибки"),
        edge("store", "tx"), edge("reports", "views", "SELECT"),
        edge("tx", "funcs", "SELECT crm.book_appointment(...)"),
        edge("tx", "tables", "INSERT/UPDATE/DELETE"),
        edge("store", "staging", "COPY + process_client_import"),
        edge("kpi", "funcs", "crm.refresh_kpi()"),
        edge("cli", "tables", "миграции, seed", style="dashed"),
        edge("tables", "triggers", "срабатывают", style="dashed"),
        edge("funcs", "tables"), edge("triggers", "audit", "запись аудита"),
        edge("views", "tables", "", style="dashed"),
        edge("roles", "tables", "GRANT", style="dotted"),
    ]
    graph("arch-3-core-components", "Архитектура. Уровень 3: компоненты сервиса core и серверная логика БД", body)


# ════════════════════════════════════════════════════════════════════════════
# 4. Развёртывание (docker compose)
# ════════════════════════════════════════════════════════════════════════════
def deployment():
    body = [
        node("dev", "Хост разработчика / сервер", "Docker Engine + Compose v2", "make core-up · docker compose up -d", EXTERNAL),
        'subgraph cluster_net { label=<<b>Сеть docker compose (bridge)</b>>; style="rounded"; color="#555555"; fontsize=15;',
        cyl("pg", "postgres", "postgres:16-alpine · 5433→5432",
            "init: init.sql, seed.sql, 10-barbershop.sh (роли и БД из .env) · том postgres_data"),
        node("mig", "core-migrate", "образ core · одноразовый", "migrate up (crm_owner)", CONTAINER),
        node("seed", "core-seed", "образ core · одноразовый", "run_seed.sh: наполнение, если БД пуста", CONTAINER),
        node("core", "core", "образ core · 8090→8090", "REST API (crm_app), healthcheck /health", CONTAINER),
        node("gw", "api-gateway", "8080→8080", "JWT, прокси /api/core/*", CONTAINER),
        node("ms", "staff · booking · client · analytics", "gRPC 5005x", "микросервисы исходного проекта", CONTAINER),
        cyl("infra", "redis · kafka · clickhouse", "тома kafka_data, clickhouse_data", "", INFRA),
        node("obs", "prometheus · grafana", "9090, 3000", "", EXTERNAL),
        'subgraph cluster_prof { label=<<b>Профили по запросу</b>>; style="dashed"; color="#999999";',
        node("tests", "core-tests", "golang:1.24 · profile test", "go test ./... против postgres", INFRA),
        node("perf", "core-perf", "образ core · profile perf", "БД barbershop_perf, ≥100 000 записей", INFRA),
        "}",
        "}",
        edge("dev", "gw", ":8080"), edge("dev", "core", ":8090"), edge("dev", "pg", ":5433 (psql)"),
        edge("pg", "mig", "service_healthy", style="dashed"),
        edge("mig", "seed", "completed_successfully", style="dashed"),
        edge("seed", "core", "completed_successfully", style="dashed"),
        edge("core", "gw", "depends_on", style="dashed"),
        edge("gw", "core", "HTTP"), edge("gw", "ms", "gRPC"),
        edge("core", "pg", "barbershop"), edge("mig", "pg", ""), edge("seed", "pg", ""),
        edge("ms", "pg", "staff_db, …"), edge("ms", "infra", ""),
        edge("tests", "pg", "временные БД"), edge("perf", "pg", "barbershop_perf"),
        edge("obs", "gw", "scrape", style="dotted"),
    ]
    graph("arch-4-deployment", "Архитектура: развёртывание (docker compose)", body)


# ════════════════════════════════════════════════════════════════════════════
# 5. Последовательность: запись клиента (успех и конфликт)
# ════════════════════════════════════════════════════════════════════════════
def sequence():
    lanes = [("Клиент\n(браузер)", 120), ("API Gateway", 400), ("core: handler\n+ валидация", 680),
             ("core: store\n+ InTx", 960), ("PostgreSQL:\nbook_appointment", 1260), ("PostgreSQL:\nтриггеры, EXCLUDE", 1560)]
    W, H = 1760, 1420
    c = Canvas(W, H)
    c.text(W / 2, 34, "Последовательность: запись клиента POST /api/core/api/v1/appointments", 22, bold=True)
    top = 70
    for name, x in lanes:
        lines = name.split("\n")
        c.rect(x - 110, top, 220, 56, rx=8, z=3, stroke="#1168bd", width=1.6, fill="#e8f0ff")
        c.mtext(x, top + 28, lines, 14, bold=True, z=5)
        c.line(x, top + 56, x, H - 40, z=0, stroke="#888", width=1.2, dash="6,5")
    X = dict((n.split("\n")[0].split(":")[0] if ":" in n else n.split("\n")[0], x) for n, x in lanes)
    xs = [x for _, x in lanes]
    y = top + 100

    def msg(i, j, text, ret=False, note=None):
        nonlocal y
        x1, x2 = xs[i], xs[j]
        c.polyline([(x1, y), (x2, y)], z=2, stroke="#333", width=1.5, end="arrow", dash="6,4" if ret else None)
        lines = wrap(text, abs(x2 - x1) - 20 if abs(x2 - x1) > 200 else 260, 12.5)
        for k, ln in enumerate(lines):
            tx = (x1 + x2) / 2
            c.text(tx, y - 8 - (len(lines) - 1 - k) * 15, ln, 12.5, z=4)
        y += 46 + (len(lines) - 1) * 12

    def self_note(i, text, color="#fff7d6"):
        nonlocal y
        lines = wrap(text, 270, 12)
        h = 12 + len(lines) * 15
        x0 = xs[i] + 12 if xs[i] + 310 < W else xs[i] - 302   # у правого края — слева от линии
        c.rect(x0, y - 14, 290, h, rx=4, z=3, stroke="#b59f3b", width=1, fill=color)
        for k, ln in enumerate(lines):
            c.text(x0 + 8, y + k * 15, ln, 12, anchor="start", z=4)
        y += h + 14

    def section(text):
        nonlocal y
        c.rect(30, y - 16, W - 60, 26, z=1, stroke="#999", width=1, fill="#f3f3f3")
        c.text(40, y + 2, text, 13, bold=True, anchor="start", z=4)
        y += 34

    msg(0, 1, "POST /api/core/api/v1/appointments, Authorization: Bearer JWT")
    self_note(1, "Проверка JWT, rate limit; X-User = barber:<id>")
    msg(1, 2, "POST /api/v1/appointments + X-User")
    self_note(2, "decode (DisallowUnknownFields), валидация: телефон, starts_at, 1–5 услуг, source")
    msg(2, 3, "store.Book(ctx, BookInput)")
    msg(3, 4, "BEGIN; SELECT set_config('crm.app_user', $1, true)")
    msg(3, 4, "SELECT crm.book_appointment($1…$7)")
    self_note(4, "Услуги из прайса мастера (CR007), окно онлайн-записи (CR008), upsert клиента по телефону")
    msg(4, 5, "INSERT appointments")
    self_note(5, "trg_appointments_validate: смена мастера FOR SHARE (CR003); EXCLUDE gist: пересечения (23P01)")
    self_note(5, "trg_appointments_status_history, audit.tg_audit; clients → trg_clients_create_loyalty")
    msg(4, 5, "INSERT appointment_services (снимок цен)")
    msg(4, 3, "appointment_id", ret=True)
    msg(3, 4, "COMMIT")
    msg(3, 2, "Appointment (из v_appointment_details)", ret=True)
    msg(2, 1, "201 Created", ret=True)
    msg(1, 0, "201 Created + JSON записи", ret=True)
    y += 6
    section("Альтернатива: параллельная транзакция уже заняла пересекающееся время")
    msg(4, 5, "INSERT appointments (другой клиент, пересекающийся интервал)")
    self_note(5, "Ожидание исхода конкурирующей транзакции; после её COMMIT — exclusion_violation 23P01", "#fde8e8")
    msg(5, 3, "ERROR 23P01 appointments_barber_no_overlap → ROLLBACK (ничего не сохранено)", ret=True)
    self_note(2, "apperr.FromDB: 23P01 → 409 time_slot_conflict «Мастер уже занят в это время»", "#fde8e8")
    msg(2, 0, "409 Conflict → клиенту предлагаются другие слоты", ret=True)
    section("Альтернатива: конфликт сериализации/взаимоблокировка (40001 / 40P01)")
    self_note(3, "db.InTx повторяет транзакцию до 3 раз с паузой; затем 503 concurrent_update", "#e8f4fd")
    c.h = y + 30
    c.save(os.path.join(OUT, "arch-5-sequence-booking"))


if __name__ == "__main__":
    os.makedirs(OUT, exist_ok=True)
    context()
    containers()
    components()
    deployment()
    sequence()
    print("ok")
