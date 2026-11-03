#!/bin/sh
# Наполнение БД, если она пуста (идемпотентный запуск из docker compose).
set -eu
n=$(psql "$CORE_MIGRATE_DB_URL" -tAc "SELECT count(*) FROM crm.branches")
if [ "$n" = "0" ]; then
  echo "seed: loading /opt/core/sql/seed/seed.sql"
  psql "$CORE_MIGRATE_DB_URL" -v ON_ERROR_STOP=1 -q -f /opt/core/sql/seed/seed.sql
else
  echo "seed: database already contains data ($n branches), skipped"
fi
