#!/usr/bin/env python3
"""Физическая ER-диаграмма (нотация «вороньи лапки») по каталогу PostgreSQL.

    DOC_DB_URL=postgres://... python3 docs/db/_src/physical_er.py [путь к dot]

Таблицы, столбцы, типы, PK/FK/UNIQUE/NOT NULL и связи читаются из БД после
применения всех миграций, поэтому диаграмма всегда соответствует схеме.
"""
import os
import subprocess
import sys
from html import escape

URL = os.environ["DOC_DB_URL"]
DOT = sys.argv[1] if len(sys.argv) > 1 else "dot"
OUT = os.path.join(os.path.dirname(__file__), "..", "er-physical")

GROUPS = {
    "Справочники и персонал": ["branches", "barbers", "barber_settings", "service_categories", "services", "barber_services", "work_shifts"],
    "Клиенты и лояльность": ["clients", "loyalty_accounts", "bonus_transactions"],
    "Записи и расчёты": ["appointments", "appointment_services", "payments", "reviews", "appointment_status_history"],
}
COLORS = {"Справочники и персонал": "#e8f0ff", "Клиенты и лояльность": "#fff4e0", "Записи и расчёты": "#e6f6ea"}


def q(sql):
    r = subprocess.run(["psql", URL, "-X", "-At", "-F", "\t", "-c", sql], capture_output=True, text=True, check=True)
    return [line.split("\t") for line in r.stdout.strip().split("\n") if line]


cols = q("""
SELECT c.relname, a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull,
       coalesce((SELECT bool_or(a.attnum = ANY (i.indkey)) FROM pg_index i WHERE i.indrelid = c.oid AND i.indisprimary), false),
       EXISTS (SELECT 1 FROM pg_constraint f WHERE f.conrelid = c.oid AND f.contype = 'f' AND a.attnum = ANY (f.conkey)),
       EXISTS (SELECT 1 FROM pg_constraint u WHERE u.conrelid = c.oid AND u.contype = 'u' AND a.attnum = ANY (u.conkey)),
       a.attgenerated <> ''
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
WHERE n.nspname = 'crm' AND c.relkind = 'r'
ORDER BY c.relname, a.attnum""")
fks = q("""
SELECT c.relname, r.relname, f.confdeltype, (SELECT string_agg(attname, ',') FROM pg_attribute
        WHERE attrelid = c.oid AND attnum = ANY (f.conkey)),
       (SELECT bool_and(attnotnull) FROM pg_attribute WHERE attrelid = c.oid AND attnum = ANY (f.conkey)),
       EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid = c.oid AND (i.indisprimary OR i.indisunique)
               AND i.indkey::int2[] @> f.conkey AND i.indkey::int2[] <@ f.conkey)
FROM pg_constraint f JOIN pg_class c ON c.oid = f.conrelid JOIN pg_class r ON r.oid = f.confrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE f.contype = 'f' AND n.nspname = 'crm'""")

tables = {}
for t, name, typ, nn, pk, fk, uq, gen in cols:
    typ = typ.replace("timestamp with time zone", "timestamptz").replace("time without time zone", "time") \
             .replace("character varying", "varchar").replace("crm.", "")
    tables.setdefault(t, []).append((name, typ, nn == "t", pk == "t", fk == "t", uq == "t", gen == "t"))

def pad(text):
    # запас ширины: метрики шрифта при раскладке и при отрисовке могут отличаться
    return "&#160;" * (2 + len(text) // 4)


ACTION = {"a": "NO ACTION", "r": "RESTRICT", "c": "CASCADE", "n": "SET NULL", "d": "SET DEFAULT"}

lines = ['digraph ER {', 'graph [rankdir=LR, splines=true, nodesep=0.45, ranksep=1.3, fontname="Liberation Sans", '
         'label="Физическая модель данных БД barbershop (схема crm), нотация «вороньи лапки»", labelloc=t, fontsize=22, pad=0.4];',
         'node [shape=plain, fontname="Liberation Sans", fontsize=11];',
         'edge [fontname="Liberation Sans", fontsize=9, color="#555555", arrowsize=0.9];']
for gi, (g, members) in enumerate(GROUPS.items()):
    lines.append(f'subgraph cluster_{gi} {{ label="{g}"; fontsize=15; style="rounded,dashed"; color="#999999";')
    for t in members:
        rows = [f'<tr><td colspan="3" bgcolor="{COLORS[g]}"><b>{t}</b></td></tr>']
        for name, typ, nn, pk, fk, uq, gen in tables[t]:
            key = " ".join(k for k, f in (("PK", pk), ("FK", fk), ("U", uq and not pk)) if f)
            nm = f"<u>{escape(name)}</u>" if pk else escape(name)
            if gen:
                nm = f"<i>{nm}</i>"
            null = "" if nn else " ∅"
            keycell = f'<font color="#b0161b">{key}</font>' if key else " "
            rows.append(f'<tr><td align="left" port="{name}">{keycell}</td>'
                        f'<td align="left">{nm}{pad(name)}</td><td align="left"><font color="#555555">{escape(typ)}{null}</font>{pad(typ)}</td></tr>')
        lines.append(f'"{t}" [label=<<table border="1" cellborder="0" cellspacing="0" cellpadding="3">{"".join(rows)}</table>>];')
    lines.append("}")
for child, parent, deltype, col, notnull, unique in fks:
    tail = "teetee" if notnull == "t" else "teeodot"         # обязательная / необязательная ссылка
    head = ("tee" if unique == "t" else "crow") + ("tee" if notnull == "t" else "odot")
    lines.append(f'"{parent}" -> "{child}" [dir=both, arrowtail={tail}, arrowhead={head}, '
                 f'xlabel="{col}\\nON DELETE {ACTION[deltype]}"];')
lines.append("}")
dot_src = "\n".join(lines)
with open(OUT + ".dot", "w", encoding="utf-8") as f:
    f.write(dot_src)
for fmt in ("svg", "png"):
    subprocess.run([DOT, f"-T{fmt}", "-Gdpi=110" if fmt == "png" else "-Gdpi=72", "-o", f"{OUT}.{fmt}", OUT + ".dot"], check=True)
print("ok:", len(tables), "tables,", len(fks), "foreign keys")
