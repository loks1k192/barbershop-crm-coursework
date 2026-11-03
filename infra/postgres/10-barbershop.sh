#!/bin/sh
# Инициализация БД курсового проекта (выполняется один раз при создании тома).
# Пароли берутся из переменных окружения (.env), в репозитории их нет.
set -eu
: "${CORE_DB_OWNER_PASSWORD:?CORE_DB_OWNER_PASSWORD is required}"
: "${CORE_DB_APP_PASSWORD:?CORE_DB_APP_PASSWORD is required}"
: "${CORE_DB_READONLY_PASSWORD:=${CORE_DB_APP_PASSWORD}_ro}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
     -v owner_pw="$CORE_DB_OWNER_PASSWORD" -v app_pw="$CORE_DB_APP_PASSWORD" -v ro_pw="$CORE_DB_READONLY_PASSWORD" <<'SQL'
-- владелец схемы: миграции и наполнение
CREATE ROLE crm_owner LOGIN PASSWORD :'owner_pw';
-- роль приложения: минимальные права выдаёт миграция 0008
CREATE ROLE crm_app LOGIN PASSWORD :'app_pw' CONNECTION LIMIT 50;
-- роль для отчётов/BI: только чтение представлений
CREATE ROLE crm_readonly LOGIN PASSWORD :'ro_pw';
CREATE DATABASE barbershop OWNER crm_owner;
ALTER DATABASE barbershop SET timezone TO 'Europe/Moscow';
REVOKE ALL ON DATABASE barbershop FROM PUBLIC;
GRANT CONNECT ON DATABASE barbershop TO crm_app, crm_readonly;
\connect barbershop
-- в PG16 схема public принадлежит pg_database_owner; приложение не создаёт объектов
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
SQL
