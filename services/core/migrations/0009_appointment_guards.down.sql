-- Возврат прежних версий функций (из 0002 и 0003).
CREATE OR REPLACE FUNCTION crm.tg_appointments_validate() RETURNS trigger
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

CREATE OR REPLACE FUNCTION crm.cancel_appointment(p_appointment_id bigint, p_reason text)
RETURNS crm.appointments
LANGUAGE plpgsql AS $$
DECLARE
    v_appt crm.appointments;
BEGIN
    IF p_reason IS NULL OR btrim(p_reason) = '' THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Укажите причину отмены';
    END IF;
    UPDATE crm.appointments
    SET status = 'cancelled', cancel_reason = btrim(p_reason)
    WHERE appointment_id = p_appointment_id
    RETURNING * INTO v_appt;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'CR001', MESSAGE = format('Запись %s не найдена', p_appointment_id);
    END IF;
    RETURN v_appt;
END $$;

