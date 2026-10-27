-- 0004. Аудит изменений значимых сущностей.
-- Фиксируются: таблица, тип операции, ключ строки, старые/новые данные (jsonb),
-- список изменённых столбцов, пользователь БД и пользователь приложения
-- (crm.app_user, передаётся приложением через set_config), транзакция и время.
--
-- Функция SECURITY DEFINER: у прикладной роли есть только SELECT на журнал,
-- подделать или удалить записи аудита она не может.

CREATE SCHEMA IF NOT EXISTS audit;

CREATE TABLE audit.audit_log (
    audit_id        bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    table_name      text        NOT NULL,
    operation       text        NOT NULL,
    row_pk          jsonb       NOT NULL,
    old_data        jsonb,
    new_data        jsonb,
    changed_columns text[],
    db_user         text        NOT NULL DEFAULT session_user,
    app_user        text,
    txid            bigint      NOT NULL DEFAULT txid_current(),
    changed_at      timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT audit_operation_chk CHECK (operation IN ('INSERT', 'UPDATE', 'DELETE')),
    CONSTRAINT audit_data_chk CHECK (
        (operation = 'INSERT' AND old_data IS NULL AND new_data IS NOT NULL) OR
        (operation = 'UPDATE' AND old_data IS NOT NULL AND new_data IS NOT NULL) OR
        (operation = 'DELETE' AND old_data IS NOT NULL AND new_data IS NULL))
);
CREATE INDEX audit_log_table_time_idx ON audit.audit_log (table_name, changed_at);
CREATE INDEX audit_log_row_pk_idx     ON audit.audit_log USING gin (row_pk jsonb_path_ops);

-- Аргументы триггера — имена столбцов первичного ключа.
CREATE FUNCTION audit.tg_audit() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
    v_old  jsonb;
    v_new  jsonb;
    v_cols text[];
    v_pk   jsonb;
BEGIN
    -- массовая генерация тестовых данных владельцем схемы может отключить аудит
    IF coalesce(current_setting('crm.audit_disabled', true), '') = 'on' AND crm.is_schema_owner() THEN
        RETURN NULL;
    END IF;

    IF TG_OP <> 'INSERT' THEN v_old := to_jsonb(OLD); END IF;
    IF TG_OP <> 'DELETE' THEN v_new := to_jsonb(NEW); END IF;

    IF TG_OP = 'UPDATE' THEN
        SELECT array_agg(n.key ORDER BY n.key) INTO v_cols
        FROM jsonb_each(v_new) n
        WHERE n.value IS DISTINCT FROM v_old -> n.key
          AND n.key <> 'updated_at';
        IF v_cols IS NULL THEN   -- фактически ничего не изменилось
            RETURN NULL;
        END IF;
    END IF;

    SELECT jsonb_object_agg(k, coalesce(v_new, v_old) -> k) INTO v_pk
    FROM unnest(TG_ARGV) AS k;

    INSERT INTO audit.audit_log (table_name, operation, row_pk, old_data, new_data, changed_columns, app_user)
    VALUES (TG_TABLE_SCHEMA || '.' || TG_TABLE_NAME, TG_OP, v_pk, v_old, v_new, v_cols,
            nullif(current_setting('crm.app_user', true), ''));
    RETURN NULL;
END $$;

CREATE TRIGGER trg_audit AFTER INSERT OR UPDATE OR DELETE ON crm.appointments
    FOR EACH ROW EXECUTE FUNCTION audit.tg_audit('appointment_id');
CREATE TRIGGER trg_audit AFTER INSERT OR UPDATE OR DELETE ON crm.clients
    FOR EACH ROW EXECUTE FUNCTION audit.tg_audit('client_id');
CREATE TRIGGER trg_audit AFTER INSERT OR UPDATE OR DELETE ON crm.payments
    FOR EACH ROW EXECUTE FUNCTION audit.tg_audit('payment_id');
CREATE TRIGGER trg_audit AFTER INSERT OR UPDATE OR DELETE ON crm.barber_services
    FOR EACH ROW EXECUTE FUNCTION audit.tg_audit('barber_id', 'service_id');
CREATE TRIGGER trg_audit AFTER INSERT OR UPDATE OR DELETE ON crm.loyalty_accounts
    FOR EACH ROW EXECUTE FUNCTION audit.tg_audit('client_id');
CREATE TRIGGER trg_audit AFTER INSERT OR UPDATE OR DELETE ON crm.work_shifts
    FOR EACH ROW EXECUTE FUNCTION audit.tg_audit('shift_id');

COMMENT ON TABLE audit.audit_log IS 'Журнал аудита: appointments, clients, payments, barber_services, loyalty_accounts, work_shifts';
