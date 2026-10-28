-- 0007. Пакетная загрузка клиентской базы с валидацией и журналом ошибок.
--
-- Поток: приложение создаёт пакет (import_batches), копирует сырые строки CSV
-- в client_import_rows (COPY), затем вызывает staging.process_client_import().
-- Функция в одной транзакции нормализует и проверяет каждую строку, пишет
-- ошибки в import_errors, корректные строки добавляет/обновляет в crm.clients.
-- Некорректные строки не прерывают пакет; непредвиденная ошибка откатывает весь
-- пакет (частично загруженных данных не остаётся).

CREATE SCHEMA IF NOT EXISTS staging;

CREATE TABLE staging.import_batches (
    batch_id      bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_name   text        NOT NULL,
    status        text        NOT NULL DEFAULT 'loaded',
    created_at    timestamptz NOT NULL DEFAULT now(),
    processed_at  timestamptz,
    total_rows    integer,
    inserted_rows integer,
    updated_rows  integer,
    rejected_rows integer,
    CONSTRAINT import_batches_status_chk CHECK (status IN ('loaded', 'processed'))
);

CREATE TABLE staging.client_import_rows (
    batch_id          bigint  NOT NULL REFERENCES staging.import_batches (batch_id) ON DELETE CASCADE,
    line_no           integer NOT NULL,
    phone             text,
    last_name         text,
    first_name        text,
    middle_name       text,
    email             text,
    birth_date        text,
    notes             text,
    marketing_consent text,
    PRIMARY KEY (batch_id, line_no)
);

CREATE TABLE staging.import_errors (
    error_id bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    batch_id bigint  NOT NULL REFERENCES staging.import_batches (batch_id) ON DELETE CASCADE,
    line_no  integer NOT NULL,
    field    text    NOT NULL,
    message  text    NOT NULL
);
CREATE INDEX import_errors_batch_idx ON staging.import_errors (batch_id, line_no);

-- Безопасный разбор даты: ISO (2001-12-31) или русский формат (31.12.2001).
CREATE FUNCTION staging.try_parse_date(p text) RETURNS date
LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
    IF p IS NULL OR btrim(p) = '' THEN
        RETURN NULL;
    ELSIF p ~ '^\d{4}-\d{2}-\d{2}$' THEN
        RETURN to_date(p, 'YYYY-MM-DD');
    ELSIF p ~ '^\d{2}\.\d{2}\.\d{4}$' THEN
        RETURN to_date(p, 'DD.MM.YYYY');
    END IF;
    RETURN NULL;
EXCEPTION WHEN others THEN
    RETURN NULL;
END $$;

-- Нормализация телефона: 8 (916) 123-45-67 → +79161234567.
CREATE FUNCTION staging.normalize_phone(p text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE
        WHEN d ~ '^[78]\d{10}$' THEN '+7' || substr(d, 2)
        WHEN d ~ '^\d{10}$'     THEN '+7' || d
        ELSE NULL
    END
    FROM (SELECT regexp_replace(coalesce(p, ''), '\D', '', 'g') AS d) s
$$;

CREATE FUNCTION staging.process_client_import(p_batch_id bigint)
RETURNS TABLE (total integer, inserted integer, updated integer, rejected integer)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = staging, crm, pg_temp AS $$
DECLARE
    v_status text;
    v_total integer;
    v_inserted integer := 0;
    v_updated integer := 0;
    v_rejected integer;
BEGIN
    SELECT status INTO v_status FROM staging.import_batches WHERE batch_id = p_batch_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'CR001', MESSAGE = format('Пакет %s не найден', p_batch_id);
    END IF;
    IF v_status <> 'loaded' THEN
        RAISE EXCEPTION USING ERRCODE = 'CR010', MESSAGE = format('Пакет %s уже обработан', p_batch_id);
    END IF;

    CREATE TEMP TABLE tmp_norm ON COMMIT DROP AS
    SELECT r.line_no,
           r.phone                                   AS raw_phone,
           staging.normalize_phone(r.phone)          AS phone,
           nullif(btrim(r.last_name), '')            AS last_name,
           nullif(btrim(r.first_name), '')           AS first_name,
           nullif(btrim(r.middle_name), '')          AS middle_name,
           nullif(lower(btrim(r.email)), '')         AS email,
           r.birth_date                              AS raw_birth,
           staging.try_parse_date(btrim(r.birth_date)) AS birth_date,
           coalesce(btrim(r.notes), '')              AS notes,
           CASE WHEN lower(btrim(coalesce(r.marketing_consent, ''))) IN ('1', 'true', 'yes', 'да', 'y')
                THEN true ELSE false END             AS consent
    FROM staging.client_import_rows r
    WHERE r.batch_id = p_batch_id;

    SELECT count(*) INTO v_total FROM tmp_norm;

    -- Валидация: одна строка может получить несколько ошибок.
    INSERT INTO staging.import_errors (batch_id, line_no, field, message)
    SELECT p_batch_id, line_no, 'phone', format('Некорректный телефон «%s»', coalesce(raw_phone, ''))
    FROM tmp_norm WHERE phone IS NULL
    UNION ALL
    SELECT p_batch_id, line_no, 'first_name', 'Не указано имя'
    FROM tmp_norm WHERE first_name IS NULL
    UNION ALL
    SELECT p_batch_id, line_no, 'email', format('Некорректный e-mail «%s»', email)
    FROM tmp_norm WHERE email IS NOT NULL AND email !~* '^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$'
    UNION ALL
    SELECT p_batch_id, line_no, 'birth_date', format('Некорректная дата рождения «%s»', raw_birth)
    FROM tmp_norm
    WHERE (btrim(coalesce(raw_birth, '')) <> '' AND birth_date IS NULL)
       OR birth_date < date '1900-01-01' OR birth_date > current_date
    UNION ALL
    -- e-mail уже принадлежит другому клиенту
    SELECT p_batch_id, t.line_no, 'email', format('E-mail %s уже используется другим клиентом', t.email)
    FROM tmp_norm t
    WHERE t.email IS NOT NULL
      AND EXISTS (SELECT 1 FROM crm.clients c
                  WHERE lower(c.email) = t.email AND c.phone IS DISTINCT FROM t.phone);

    -- Дубликат телефона внутри файла (среди строк без других ошибок):
    -- принимается первая корректная строка.
    INSERT INTO staging.import_errors (batch_id, line_no, field, message)
    SELECT p_batch_id, t.line_no, 'phone',
           format('Телефон %s повторяется в файле (строка %s)', t.phone, t.first_line)
    FROM (
        SELECT v.line_no, v.phone,
               min(v.line_no) OVER (PARTITION BY v.phone) AS first_line
        FROM tmp_norm v
        WHERE NOT EXISTS (SELECT 1 FROM staging.import_errors e
                          WHERE e.batch_id = p_batch_id AND e.line_no = v.line_no)
    ) t
    WHERE t.line_no > t.first_line;

    -- Загрузка корректных строк (upsert по телефону; пустые поля не затирают данные).
    WITH valid AS (
        SELECT t.* FROM tmp_norm t
        WHERE NOT EXISTS (SELECT 1 FROM staging.import_errors e
                          WHERE e.batch_id = p_batch_id AND e.line_no = t.line_no)
    ), up AS (
        INSERT INTO crm.clients AS c (phone, last_name, first_name, middle_name, email, birth_date,
                                      notes, marketing_consent)
        SELECT phone, last_name, first_name, middle_name, email, birth_date, notes, consent FROM valid
        ON CONFLICT (phone) DO UPDATE
        SET last_name         = coalesce(EXCLUDED.last_name, c.last_name),
            first_name        = EXCLUDED.first_name,
            middle_name       = coalesce(EXCLUDED.middle_name, c.middle_name),
            email             = coalesce(EXCLUDED.email, c.email),
            birth_date        = coalesce(EXCLUDED.birth_date, c.birth_date),
            notes             = CASE WHEN EXCLUDED.notes <> '' THEN EXCLUDED.notes ELSE c.notes END,
            marketing_consent = EXCLUDED.marketing_consent
        RETURNING (xmax = 0) AS is_insert
    )
    SELECT count(*) FILTER (WHERE is_insert), count(*) FILTER (WHERE NOT is_insert)
    INTO v_inserted, v_updated
    FROM up;

    SELECT count(DISTINCT line_no) INTO v_rejected FROM staging.import_errors WHERE batch_id = p_batch_id;

    UPDATE staging.import_batches
    SET status = 'processed', processed_at = now(), total_rows = v_total,
        inserted_rows = v_inserted, updated_rows = v_updated, rejected_rows = v_rejected
    WHERE batch_id = p_batch_id;

    RETURN QUERY SELECT v_total, v_inserted, v_updated, v_rejected;
END $$;
