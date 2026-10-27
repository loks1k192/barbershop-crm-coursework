-- 0005. Миграция, изменяющая структуру существующей БД с сохранением данных.
--
-- Было:  clients.full_name text NOT NULL — ФИО одной строкой (как в исходной
--        системе), что мешает сортировке по фамилии, поиску и персонализации
--        рассылок («Здравствуйте, Иван!»).
-- Стало: last_name / first_name / middle_name + full_name как вычисляемый
--        (GENERATED ... STORED) столбец — старые запросы на чтение full_name
--        продолжают работать.
--
-- Правило разбора существующих значений:
--   1 слово           → имя;
--   2 слова           → «Имя Фамилия» (формат ввода исходной системы: «Артём Волков»);
--   3 и более слов    → «Фамилия Имя Отчество…».
-- Миграция выполняется в одной транзакции: при любой ошибке данные не теряются.

CREATE FUNCTION crm.split_person_name(
    p_name text,
    OUT last_name text, OUT first_name text, OUT middle_name text)
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
    w text[] := regexp_split_to_array(btrim(regexp_replace(coalesce(p_name, ''), '\s+', ' ', 'g')), ' ');
BEGIN
    IF w = ARRAY[''] THEN
        RETURN;
    END IF;
    CASE cardinality(w)
        WHEN 1 THEN first_name := w[1];
        WHEN 2 THEN first_name := w[1]; last_name := w[2];
        ELSE last_name := w[1]; first_name := w[2]; middle_name := array_to_string(w[3:], ' ');
    END CASE;
END $$;

ALTER TABLE crm.clients
    ADD COLUMN last_name   text,
    ADD COLUMN first_name  text,
    ADD COLUMN middle_name text;

UPDATE crm.clients c
SET (last_name, first_name, middle_name) =
    (SELECT n.last_name, n.first_name, n.middle_name FROM crm.split_person_name(c.full_name) n);

ALTER TABLE crm.clients
    ALTER COLUMN first_name SET NOT NULL,
    ADD CONSTRAINT clients_first_name_chk CHECK (btrim(first_name) <> ''),
    ADD CONSTRAINT clients_last_name_chk  CHECK (last_name IS NULL OR btrim(last_name) <> '');

ALTER TABLE crm.clients DROP COLUMN full_name;   -- вместе с clients_name_chk

ALTER TABLE crm.clients
    ADD COLUMN full_name text GENERATED ALWAYS AS
        (coalesce(last_name || ' ', '') || first_name || coalesce(' ' || middle_name, '')) STORED;

-- book_appointment создавал клиента по full_name — переводим на новые столбцы.
CREATE OR REPLACE FUNCTION crm.book_appointment(
    p_client_phone text,
    p_client_name  text,
    p_barber_id    integer,
    p_starts_at    timestamptz,
    p_service_ids  integer[],
    p_source       crm.booking_source DEFAULT 'online',
    p_comment      text               DEFAULT NULL
) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = crm, pg_temp AS $$
DECLARE
    v_barber    crm.barbers;
    v_settings  crm.barber_settings;
    v_cnt       integer;
    v_duration  integer;
    v_client_id bigint;
    v_appt_id   bigint;
BEGIN
    IF p_service_ids IS NULL OR cardinality(p_service_ids) = 0 THEN
        RAISE EXCEPTION USING ERRCODE = 'CR007', MESSAGE = 'Не выбрано ни одной услуги';
    END IF;
    IF (SELECT count(DISTINCT s) FROM unnest(p_service_ids) s) <> cardinality(p_service_ids) THEN
        RAISE EXCEPTION USING ERRCODE = 'CR007', MESSAGE = 'Услуги в записи не должны повторяться';
    END IF;

    SELECT * INTO v_barber FROM crm.barbers WHERE barber_id = p_barber_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'CR001', MESSAGE = format('Мастер %s не найден', p_barber_id);
    END IF;
    IF v_barber.fired_at IS NOT NULL THEN
        RAISE EXCEPTION USING ERRCODE = 'CR007', MESSAGE = format('Мастер %s не работает', v_barber.full_name);
    END IF;

    SELECT count(*), sum(bsv.duration_min)
    INTO v_cnt, v_duration
    FROM crm.barber_services bsv
    JOIN crm.services s ON s.service_id = bsv.service_id
    WHERE bsv.barber_id = p_barber_id
      AND bsv.service_id = ANY (p_service_ids)
      AND bsv.is_active AND s.is_active;
    IF v_cnt <> cardinality(p_service_ids) THEN
        RAISE EXCEPTION USING ERRCODE = 'CR007',
            MESSAGE = 'Мастер не оказывает одну или несколько выбранных услуг';
    END IF;

    SELECT * INTO v_settings FROM crm.barber_settings WHERE barber_id = p_barber_id;
    IF p_source = 'online' THEN
        IF p_starts_at < now() + make_interval(mins => v_settings.min_lead_min) THEN
            RAISE EXCEPTION USING ERRCODE = 'CR008',
                MESSAGE = format('Онлайн-запись возможна не позднее чем за %s мин. до начала',
                                 v_settings.min_lead_min);
        END IF;
        IF p_starts_at > now() + make_interval(days => v_settings.max_days_ahead) THEN
            RAISE EXCEPTION USING ERRCODE = 'CR008',
                MESSAGE = format('Онлайн-запись открыта только на %s дн. вперёд', v_settings.max_days_ahead);
        END IF;
    ELSIF p_starts_at < now() - interval '15 minutes' THEN
        RAISE EXCEPTION USING ERRCODE = 'CR008', MESSAGE = 'Нельзя записать на прошедшее время';
    END IF;

    INSERT INTO crm.clients (phone, last_name, first_name, middle_name)
    SELECT p_client_phone, n.last_name, n.first_name, n.middle_name
    FROM crm.split_person_name(p_client_name) n
    ON CONFLICT (phone) DO NOTHING
    RETURNING client_id INTO v_client_id;
    IF v_client_id IS NULL THEN
        SELECT client_id INTO v_client_id FROM crm.clients WHERE phone = p_client_phone;
    END IF;

    INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at, source, comment)
    VALUES (v_client_id, p_barber_id, p_starts_at,
            p_starts_at + make_interval(mins => v_duration), p_source, p_comment)
    RETURNING appointment_id INTO v_appt_id;

    INSERT INTO crm.appointment_services (appointment_id, service_id, price, duration_min)
    SELECT v_appt_id, bsv.service_id, bsv.price, bsv.duration_min
    FROM crm.barber_services bsv
    WHERE bsv.barber_id = p_barber_id AND bsv.service_id = ANY (p_service_ids);

    RETURN v_appt_id;
END $$;
