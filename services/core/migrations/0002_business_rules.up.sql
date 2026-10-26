-- 0002. Триггеры: автоматизация и бизнес-правила, которые нельзя выразить
-- декларативными ограничениями (CHECK/UNIQUE/EXCLUDE).
--
-- Коды ошибок приложения (SQLSTATE класса 'CR'):
--   CR001 объект не найден               CR006 отзыв недопустим
--   CR002 недопустимый переход статуса   CR007 некорректный состав записи
--   CR003 нарушение графика работы       CR008 нарушение окна онлайн-записи
--   CR004 конфликт смены с записями      CR009 недостаточно бонусов / лимит списания
--   CR005 ошибка оплаты                  CR010 ошибка импорта

-- Часовой пояс сети (все филиалы в одном городе).
CREATE FUNCTION crm.local_tz() RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$ SELECT 'Europe/Moscow'::text $$;

-- Является ли текущий пользователь сеанса владельцем схемы crm (или членом
-- роли-владельца). Используется для служебных режимов (загрузка истории,
-- отключение аудита при массовой генерации данных) — прикладной роли они недоступны.
CREATE FUNCTION crm.is_schema_owner() RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT pg_has_role(session_user, n.nspowner, 'MEMBER')
    FROM pg_namespace n WHERE n.nspname = 'crm'
$$;

-- Границы смены как timestamptz.
CREATE FUNCTION crm.shift_start_ts(p_date date, p_time time) RETURNS timestamptz
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$ SELECT (p_date + p_time) AT TIME ZONE crm.local_tz() $$;

-- ── updated_at ──────────────────────────────────────────────────────────────
CREATE FUNCTION crm.tg_set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END $$;

CREATE TRIGGER trg_branches_updated_at BEFORE UPDATE ON crm.branches
    FOR EACH ROW EXECUTE FUNCTION crm.tg_set_updated_at();
CREATE TRIGGER trg_barbers_updated_at BEFORE UPDATE ON crm.barbers
    FOR EACH ROW EXECUTE FUNCTION crm.tg_set_updated_at();
CREATE TRIGGER trg_barber_settings_updated_at BEFORE UPDATE ON crm.barber_settings
    FOR EACH ROW EXECUTE FUNCTION crm.tg_set_updated_at();
CREATE TRIGGER trg_clients_updated_at BEFORE UPDATE ON crm.clients
    FOR EACH ROW EXECUTE FUNCTION crm.tg_set_updated_at();
CREATE TRIGGER trg_appointments_updated_at BEFORE UPDATE ON crm.appointments
    FOR EACH ROW EXECUTE FUNCTION crm.tg_set_updated_at();

-- ── Автосоздание зависимых 1:1 записей ──────────────────────────────────────
-- SECURITY DEFINER: прикладной роли не нужны права на INSERT в эти таблицы.
CREATE FUNCTION crm.tg_barbers_create_settings() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = crm, pg_temp AS $$
BEGIN
    INSERT INTO crm.barber_settings (barber_id) VALUES (NEW.barber_id);
    RETURN NULL;
END $$;
CREATE TRIGGER trg_barbers_create_settings AFTER INSERT ON crm.barbers
    FOR EACH ROW EXECUTE FUNCTION crm.tg_barbers_create_settings();

CREATE FUNCTION crm.tg_clients_create_loyalty() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = crm, pg_temp AS $$
BEGIN
    INSERT INTO crm.loyalty_accounts (client_id) VALUES (NEW.client_id);
    RETURN NULL;
END $$;
CREATE TRIGGER trg_clients_create_loyalty AFTER INSERT ON crm.clients
    FOR EACH ROW EXECUTE FUNCTION crm.tg_clients_create_loyalty();

-- ── BR-4: смены ─────────────────────────────────────────────────────────────
-- Смена лежит внутри часов работы активного филиала, мастер не уволен;
-- смену нельзя сократить/перенести/удалить, если активные записи окажутся вне её.
CREATE FUNCTION crm.tg_work_shifts_validate() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v_branch    crm.branches;
    v_fired     date;
    v_conflicts integer;
BEGIN
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        SELECT * INTO v_branch FROM crm.branches WHERE branch_id = NEW.branch_id;
        IF NOT v_branch.is_active THEN
            RAISE EXCEPTION USING ERRCODE = 'CR003',
                MESSAGE = format('Филиал «%s» не работает', v_branch.name);
        END IF;
        IF NEW.starts_at < v_branch.opens_at OR NEW.ends_at > v_branch.closes_at THEN
            RAISE EXCEPTION USING ERRCODE = 'CR003',
                MESSAGE = format('Смена %s–%s выходит за часы работы филиала «%s» (%s–%s)',
                                 NEW.starts_at, NEW.ends_at, v_branch.name,
                                 v_branch.opens_at, v_branch.closes_at);
        END IF;
        SELECT fired_at INTO v_fired FROM crm.barbers WHERE barber_id = NEW.barber_id;
        IF v_fired IS NOT NULL AND NEW.work_date >= v_fired THEN
            RAISE EXCEPTION USING ERRCODE = 'CR003',
                MESSAGE = format('Мастер %s уволен %s, смена на %s невозможна',
                                 NEW.barber_id, v_fired, NEW.work_date);
        END IF;
    END IF;

    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        SELECT count(*) INTO v_conflicts
        FROM crm.appointments a
        WHERE a.barber_id = OLD.barber_id
          AND a.status IN ('scheduled', 'confirmed', 'in_progress')
          AND a.starts_at >= crm.shift_start_ts(OLD.work_date, OLD.starts_at)
          AND a.ends_at   <= crm.shift_start_ts(OLD.work_date, OLD.ends_at)
          AND (TG_OP = 'DELETE'
               OR NEW.barber_id <> OLD.barber_id
               OR NEW.work_date <> OLD.work_date
               OR a.starts_at < crm.shift_start_ts(NEW.work_date, NEW.starts_at)
               OR a.ends_at   > crm.shift_start_ts(NEW.work_date, NEW.ends_at));
        IF v_conflicts > 0 THEN
            RAISE EXCEPTION USING ERRCODE = 'CR004',
                MESSAGE = format('Нельзя %s смену %s: %s активных записей окажутся вне смены',
                                 CASE TG_OP WHEN 'DELETE' THEN 'удалить' ELSE 'изменить' END,
                                 OLD.shift_id, v_conflicts),
                HINT = 'Сначала перенесите или отмените записи';
        END IF;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_work_shifts_validate BEFORE INSERT OR UPDATE OR DELETE ON crm.work_shifts
    FOR EACH ROW EXECUTE FUNCTION crm.tg_work_shifts_validate();

-- ── BR-3, BR-5: записи ──────────────────────────────────────────────────────
-- * новая запись создаётся в статусе scheduled/confirmed (исторические данные
--   загружаются в режиме crm.history_load = on, доступном только владельцу схемы);
-- * разрешённые переходы статусов, финальные статусы неизменны;
-- * completed / no_show — только после начала визита;
-- * запись целиком внутри смены мастера (строка смены блокируется FOR SHARE,
--   чтобы параллельное удаление/сокращение смены не прошло незамеченным).
CREATE FUNCTION crm.tg_appointments_validate() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v_history_load boolean := coalesce(current_setting('crm.history_load', true), '') = 'on'
                              AND crm.is_schema_owner();
    v_shift        crm.work_shifts;
    v_check_shift  boolean := false;
    v_allowed      boolean;
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status NOT IN ('scheduled', 'confirmed') AND NOT v_history_load THEN
            RAISE EXCEPTION USING ERRCODE = 'CR002',
                MESSAGE = format('Новая запись не может быть создана в статусе %s', NEW.status);
        END IF;
        v_check_shift := true;
    ELSE
        IF OLD.status IN ('completed', 'cancelled', 'no_show')
           AND (NEW.status <> OLD.status OR NEW.starts_at <> OLD.starts_at
                OR NEW.ends_at <> OLD.ends_at OR NEW.barber_id <> OLD.barber_id
                OR NEW.client_id <> OLD.client_id) THEN
            RAISE EXCEPTION USING ERRCODE = 'CR002',
                MESSAGE = format('Запись %s в финальном статусе «%s» и не может быть изменена',
                                 OLD.appointment_id, OLD.status);
        END IF;

        IF NEW.status <> OLD.status THEN
            v_allowed := CASE OLD.status
                WHEN 'scheduled'   THEN NEW.status IN ('confirmed', 'in_progress', 'completed',
                                                       'cancelled', 'no_show')
                WHEN 'confirmed'   THEN NEW.status IN ('in_progress', 'completed', 'cancelled', 'no_show')
                WHEN 'in_progress' THEN NEW.status = 'completed'
                ELSE false
            END;
            IF NOT v_allowed THEN
                RAISE EXCEPTION USING ERRCODE = 'CR002',
                    MESSAGE = format('Недопустимый переход статуса записи %s: %s → %s',
                                     OLD.appointment_id, OLD.status, NEW.status);
            END IF;
            IF NEW.status IN ('completed', 'no_show') AND NEW.starts_at > now() THEN
                RAISE EXCEPTION USING ERRCODE = 'CR002',
                    MESSAGE = format('Статус «%s» можно установить только после начала визита (%s)',
                                     NEW.status, NEW.starts_at);
            END IF;
            IF NEW.status = 'in_progress' AND NEW.starts_at > now() + interval '30 minutes' THEN
                RAISE EXCEPTION USING ERRCODE = 'CR002',
                    MESSAGE = 'Начать визит можно не ранее чем за 30 минут до записи';
            END IF;
        END IF;

        v_check_shift := NEW.status IN ('scheduled', 'confirmed', 'in_progress')
                         AND (NEW.starts_at <> OLD.starts_at OR NEW.ends_at <> OLD.ends_at
                              OR NEW.barber_id <> OLD.barber_id);
    END IF;

    IF v_check_shift THEN
        SELECT * INTO v_shift
        FROM crm.work_shifts
        WHERE barber_id = NEW.barber_id
          AND work_date = (NEW.starts_at AT TIME ZONE crm.local_tz())::date
        FOR SHARE;
        IF NOT FOUND THEN
            RAISE EXCEPTION USING ERRCODE = 'CR003',
                MESSAGE = format('У мастера %s нет смены на %s', NEW.barber_id,
                                 (NEW.starts_at AT TIME ZONE crm.local_tz())::date);
        END IF;
        IF NEW.starts_at < crm.shift_start_ts(v_shift.work_date, v_shift.starts_at)
           OR NEW.ends_at > crm.shift_start_ts(v_shift.work_date, v_shift.ends_at) THEN
            RAISE EXCEPTION USING ERRCODE = 'CR003',
                MESSAGE = format('Запись %s–%s выходит за границы смены мастера (%s–%s)',
                                 to_char(NEW.starts_at AT TIME ZONE crm.local_tz(), 'HH24:MI'),
                                 to_char(NEW.ends_at AT TIME ZONE crm.local_tz(), 'HH24:MI'),
                                 v_shift.starts_at, v_shift.ends_at);
        END IF;
    END IF;

    RETURN NEW;
END $$;
CREATE TRIGGER trg_appointments_validate BEFORE INSERT OR UPDATE ON crm.appointments
    FOR EACH ROW EXECUTE FUNCTION crm.tg_appointments_validate();

-- ── История статусов (автоматизация) ────────────────────────────────────────
CREATE FUNCTION crm.tg_appointments_status_history() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = crm, pg_temp AS $$
BEGIN
    IF TG_OP = 'INSERT' OR NEW.status <> OLD.status THEN
        INSERT INTO crm.appointment_status_history
            (appointment_id, old_status, new_status, changed_at, changed_by)
        VALUES (NEW.appointment_id,
                CASE WHEN TG_OP = 'UPDATE' THEN OLD.status END,
                NEW.status,
                NEW.updated_at,
                coalesce(nullif(current_setting('crm.app_user', true), ''), session_user));
    END IF;
    RETURN NULL;
END $$;
CREATE TRIGGER trg_appointments_status_history AFTER INSERT OR UPDATE OF status ON crm.appointments
    FOR EACH ROW EXECUTE FUNCTION crm.tg_appointments_status_history();

-- ── BR-7: оплаты ────────────────────────────────────────────────────────────
-- Строка записи блокируется FOR UPDATE: две параллельные оплаты одного визита
-- выполняются последовательно и не могут вместе превысить стоимость.
CREATE FUNCTION crm.tg_payments_validate() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v_status crm.appointment_status;
    v_total  numeric;
    v_paid   numeric;
BEGIN
    SELECT status INTO v_status
    FROM crm.appointments WHERE appointment_id = NEW.appointment_id
    FOR UPDATE;
    IF v_status NOT IN ('in_progress', 'completed') THEN
        RAISE EXCEPTION USING ERRCODE = 'CR005',
            MESSAGE = format('Оплата возможна только для идущего или завершённого визита (статус «%s»)',
                             v_status);
    END IF;
    SELECT coalesce(sum(price), 0) INTO v_total
    FROM crm.appointment_services WHERE appointment_id = NEW.appointment_id;
    SELECT coalesce(sum(amount), 0) INTO v_paid
    FROM crm.payments WHERE appointment_id = NEW.appointment_id AND payment_id <> NEW.payment_id;
    IF v_paid + NEW.amount > v_total THEN
        RAISE EXCEPTION USING ERRCODE = 'CR005',
            MESSAGE = format('Сумма оплат %s превышает стоимость визита %s', v_paid + NEW.amount, v_total);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_payments_validate BEFORE INSERT OR UPDATE ON crm.payments
    FOR EACH ROW EXECUTE FUNCTION crm.tg_payments_validate();

-- ── BR-9: отзывы ────────────────────────────────────────────────────────────
CREATE FUNCTION crm.tg_reviews_validate() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v_status crm.appointment_status;
    v_ends   timestamptz;
BEGIN
    SELECT status, ends_at INTO v_status, v_ends
    FROM crm.appointments WHERE appointment_id = NEW.appointment_id;
    IF v_status <> 'completed' THEN
        RAISE EXCEPTION USING ERRCODE = 'CR006',
            MESSAGE = format('Отзыв можно оставить только о завершённом визите (статус «%s»)', v_status);
    END IF;
    IF NEW.created_at > v_ends + interval '30 days' THEN
        RAISE EXCEPTION USING ERRCODE = 'CR006',
            MESSAGE = 'Срок для отзыва (30 дней после визита) истёк';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_reviews_validate BEFORE INSERT OR UPDATE ON crm.reviews
    FOR EACH ROW EXECUTE FUNCTION crm.tg_reviews_validate();

-- ── BR-8: бонусный баланс ───────────────────────────────────────────────────
-- Баланс — обоснованная денормализация суммы журнала bonus_transactions.
-- Изменяется атомарным UPDATE ... SET balance = balance + amount; отрицательный
-- результат отсекается CHECK (loyalty_balance_chk) и откатывает всю транзакцию.
CREATE FUNCTION crm.tg_bonus_tx_apply() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE crm.loyalty_accounts
    SET bonus_balance = bonus_balance + NEW.amount
    WHERE client_id = NEW.client_id;
    RETURN NULL;
END $$;
CREATE TRIGGER trg_bonus_tx_apply AFTER INSERT ON crm.bonus_transactions
    FOR EACH ROW EXECUTE FUNCTION crm.tg_bonus_tx_apply();

-- ── Уровень лояльности только повышается ───────────────────────────────────
CREATE FUNCTION crm.loyalty_level_for(p_total_spent numeric) RETURNS crm.loyalty_level
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE
        WHEN p_total_spent >= 50000 THEN 'gold'::crm.loyalty_level
        WHEN p_total_spent >= 15000 THEN 'silver'::crm.loyalty_level
        ELSE 'bronze'::crm.loyalty_level
    END
$$;

CREATE FUNCTION crm.tg_loyalty_level() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.level := greatest(NEW.level, crm.loyalty_level_for(NEW.total_spent));
    NEW.updated_at := now();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_loyalty_level BEFORE UPDATE ON crm.loyalty_accounts
    FOR EACH ROW EXECUTE FUNCTION crm.tg_loyalty_level();
