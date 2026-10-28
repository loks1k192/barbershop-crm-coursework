-- 0008. Роли и минимально необходимые привилегии.
--
--   crm_owner    — владелец объектов, выполняет миграции (создаётся при развёртывании)
--   crm_app      — роль приложения: чтение, CRUD справочников/клиентов/смен,
--                  ограниченное изменение записей; финансовые таблицы — только
--                  через SECURITY DEFINER-функции
--   crm_readonly — роль для BI/аналитиков: только представления и отчёты
-- Пароли ролей задаются при развёртывании из переменных окружения
-- (infra/postgres/10-barbershop.sh), в миграциях секретов нет.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'crm_app') THEN
        CREATE ROLE crm_app NOLOGIN;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'crm_readonly') THEN
        CREATE ROLE crm_readonly NOLOGIN;
    END IF;
END $$;

REVOKE ALL ON SCHEMA crm, audit, staging FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA crm, audit, staging FROM PUBLIC;
-- функции по умолчанию исполняемы всеми (PUBLIC) — отзываем
REVOKE EXECUTE ON ALL FUNCTIONS IN SCHEMA crm, audit, staging FROM PUBLIC;

-- ── crm_app ─────────────────────────────────────────────────────────────────
GRANT USAGE ON SCHEMA crm, audit, staging TO crm_app;

GRANT SELECT ON ALL TABLES IN SCHEMA crm TO crm_app;          -- таблицы, представления, MV
GRANT SELECT ON audit.audit_log TO crm_app;

GRANT INSERT, UPDATE, DELETE ON
    crm.branches, crm.barbers, crm.service_categories, crm.services,
    crm.barber_services, crm.work_shifts, crm.clients, crm.reviews
TO crm_app;
GRANT UPDATE ON crm.barber_settings TO crm_app;
-- записи: создание/перенос — через функции; напрямую только статус и комментарии
GRANT UPDATE (status, comment, cancel_reason) ON crm.appointments TO crm_app;
GRANT DELETE ON crm.appointments TO crm_app;

GRANT SELECT, INSERT ON staging.import_batches, staging.client_import_rows TO crm_app;
GRANT SELECT ON staging.import_errors TO crm_app;

GRANT USAGE ON ALL SEQUENCES IN SCHEMA crm, staging TO crm_app;

GRANT EXECUTE ON FUNCTION
    crm.local_tz(), crm.shift_start_ts(date, time), crm.is_schema_owner(),
    crm.loyalty_level_for(numeric), crm.accrual_rate(crm.loyalty_level),
    crm.appointment_total(bigint), crm.split_person_name(text),
    crm.free_slots(integer, date, integer, boolean, timestamptz),
    crm.book_appointment(text, text, integer, timestamptz, integer[], crm.booking_source, text),
    crm.complete_appointment(bigint, crm.payment_method, numeric),
    crm.cancel_appointment(bigint, text),
    crm.reschedule_appointment(bigint, timestamptz),
    crm.refresh_kpi(),
    staging.process_client_import(bigint)
TO crm_app;
-- Триггерные функции отдельных прав не требуют: EXECUTE проверяется только при
-- CREATE TRIGGER, а не при срабатывании.

-- ── crm_readonly ────────────────────────────────────────────────────────────
GRANT USAGE ON SCHEMA crm TO crm_readonly;
GRANT SELECT ON crm.v_appointment_details, crm.v_client_stats, crm.mv_barber_monthly_kpi,
               crm.branches, crm.barbers, crm.services, crm.service_categories
TO crm_readonly;
GRANT EXECUTE ON FUNCTION crm.local_tz() TO crm_readonly;

-- Объекты, создаваемые будущими миграциями владельцем, читаемы приложением.
ALTER DEFAULT PRIVILEGES IN SCHEMA crm GRANT SELECT ON TABLES TO crm_app;
