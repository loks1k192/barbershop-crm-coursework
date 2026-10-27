-- Обратная миграция: собрать ФИО обратно в одну строку (данные сохраняются).
ALTER TABLE crm.clients ADD COLUMN full_name_tmp text;
UPDATE crm.clients SET full_name_tmp = full_name;
ALTER TABLE crm.clients DROP COLUMN full_name;
ALTER TABLE crm.clients RENAME COLUMN full_name_tmp TO full_name;
ALTER TABLE crm.clients
    ALTER COLUMN full_name SET NOT NULL,
    ADD CONSTRAINT clients_name_chk CHECK (btrim(full_name) <> ''),
    DROP CONSTRAINT clients_first_name_chk,
    DROP CONSTRAINT clients_last_name_chk,
    DROP COLUMN last_name,
    DROP COLUMN first_name,
    DROP COLUMN middle_name;
-- функция book_appointment из 0003 снова корректна для full_name
CREATE OR REPLACE FUNCTION crm.split_person_name(p_name text, OUT last_name text, OUT first_name text, OUT middle_name text)
LANGUAGE sql IMMUTABLE AS $$ SELECT NULL::text, p_name, NULL::text $$;
DROP FUNCTION crm.split_person_name(text);
