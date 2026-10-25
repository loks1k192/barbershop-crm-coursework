-- 0001. Базовая схема БД «Сеть барбершопов».
--
-- Соглашения:
--  * суррогатные ключи — IDENTITY (GENERATED ALWAYS): неизменяемы, поэтому для всех
--    внешних ключей ON UPDATE NO ACTION (каскадное обновление не требуется);
--  * ON DELETE выбирается по смыслу связи (см. docs/db/02-data-model.md, раздел 2.6);
--  * деньги — numeric(10,2), время визитов — timestamptz, график смен — date + time
--    (локальное время сети, часовой пояс Europe/Moscow).

CREATE EXTENSION IF NOT EXISTS btree_gist;   -- '=' по скалярам внутри GiST для EXCLUDE

CREATE SCHEMA IF NOT EXISTS crm;

-- ── Перечислимые типы ────────────────────────────────────────────────────────
CREATE TYPE crm.barber_grade       AS ENUM ('junior', 'middle', 'senior', 'top');
CREATE TYPE crm.appointment_status AS ENUM ('scheduled', 'confirmed', 'in_progress',
                                            'completed', 'cancelled', 'no_show');
CREATE TYPE crm.booking_source     AS ENUM ('online', 'phone', 'walk_in', 'admin');
CREATE TYPE crm.payment_method     AS ENUM ('cash', 'card', 'online', 'bonus');
CREATE TYPE crm.loyalty_level      AS ENUM ('bronze', 'silver', 'gold');
CREATE TYPE crm.bonus_tx_kind      AS ENUM ('accrual', 'redeem', 'adjustment', 'expire');

-- ── Филиалы ─────────────────────────────────────────────────────────────────
CREATE TABLE crm.branches (
    branch_id   integer     GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        text        NOT NULL,
    address     text        NOT NULL,
    phone       text        NOT NULL,
    opens_at    time        NOT NULL DEFAULT '09:00',
    closes_at   time        NOT NULL DEFAULT '21:00',
    is_active   boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT branches_name_uq    UNIQUE (name),
    CONSTRAINT branches_name_chk   CHECK (btrim(name) <> ''),
    CONSTRAINT branches_addr_chk   CHECK (btrim(address) <> ''),
    CONSTRAINT branches_phone_chk  CHECK (phone ~ '^\+7[0-9]{10}$'),
    CONSTRAINT branches_hours_chk  CHECK (closes_at > opens_at)
);

-- ── Мастера ─────────────────────────────────────────────────────────────────
CREATE TABLE crm.barbers (
    barber_id     integer          GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    branch_id     integer          NOT NULL
                  REFERENCES crm.branches (branch_id) ON DELETE RESTRICT,
    full_name     text             NOT NULL,
    phone         text             NOT NULL,
    login         text             NOT NULL,
    password_hash text             NOT NULL,
    grade         crm.barber_grade NOT NULL DEFAULT 'junior',
    hired_at      date             NOT NULL DEFAULT current_date,
    fired_at      date,
    created_at    timestamptz      NOT NULL DEFAULT now(),
    updated_at    timestamptz      NOT NULL DEFAULT now(),
    CONSTRAINT barbers_phone_uq    UNIQUE (phone),
    CONSTRAINT barbers_login_uq    UNIQUE (login),
    CONSTRAINT barbers_name_chk    CHECK (btrim(full_name) <> ''),
    CONSTRAINT barbers_phone_chk   CHECK (phone ~ '^\+7[0-9]{10}$'),
    CONSTRAINT barbers_login_chk   CHECK (login ~ '^[a-z][a-z0-9_.]{2,31}$'),
    CONSTRAINT barbers_fired_chk   CHECK (fired_at IS NULL OR fired_at >= hired_at)
);
CREATE INDEX barbers_branch_idx ON crm.barbers (branch_id);

-- ── Настройки онлайн-записи мастера (1:1 с barbers) ─────────────────────────
CREATE TABLE crm.barber_settings (
    barber_id       integer     PRIMARY KEY
                    REFERENCES crm.barbers (barber_id) ON DELETE CASCADE,
    slot_step_min   smallint    NOT NULL DEFAULT 15,
    compact_slots   boolean     NOT NULL DEFAULT false,
    min_lead_min    integer     NOT NULL DEFAULT 60,
    max_days_ahead  smallint    NOT NULL DEFAULT 30,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT barber_settings_step_chk  CHECK (slot_step_min IN (5, 10, 15, 20, 30, 60)),
    CONSTRAINT barber_settings_lead_chk  CHECK (min_lead_min BETWEEN 0 AND 2880),
    CONSTRAINT barber_settings_days_chk  CHECK (max_days_ahead BETWEEN 1 AND 180)
);

-- ── Каталог услуг ───────────────────────────────────────────────────────────
CREATE TABLE crm.service_categories (
    category_id  integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         text    NOT NULL,
    description  text,
    CONSTRAINT service_categories_name_uq  UNIQUE (name),
    CONSTRAINT service_categories_name_chk CHECK (btrim(name) <> '')
);

CREATE TABLE crm.services (
    service_id        integer       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    category_id       integer       NOT NULL
                      REFERENCES crm.service_categories (category_id) ON DELETE RESTRICT,
    name              text          NOT NULL,
    description       text,
    base_price        numeric(10,2) NOT NULL,
    base_duration_min smallint      NOT NULL,
    is_active         boolean       NOT NULL DEFAULT true,
    CONSTRAINT services_name_uq      UNIQUE (category_id, name),
    CONSTRAINT services_name_chk     CHECK (btrim(name) <> ''),
    CONSTRAINT services_price_chk    CHECK (base_price > 0),
    CONSTRAINT services_duration_chk CHECK (base_duration_min BETWEEN 5 AND 480
                                            AND base_duration_min % 5 = 0)
);
CREATE INDEX services_category_idx ON crm.services (category_id);

-- ── Прайс мастера: связь M:N «мастер оказывает услугу» ─────────────────────
CREATE TABLE crm.barber_services (
    barber_id    integer       NOT NULL
                 REFERENCES crm.barbers (barber_id) ON DELETE CASCADE,
    service_id   integer       NOT NULL
                 REFERENCES crm.services (service_id) ON DELETE CASCADE,
    price        numeric(10,2) NOT NULL,
    duration_min smallint      NOT NULL,
    is_active    boolean       NOT NULL DEFAULT true,
    PRIMARY KEY (barber_id, service_id),
    CONSTRAINT barber_services_price_chk    CHECK (price > 0),
    CONSTRAINT barber_services_duration_chk CHECK (duration_min BETWEEN 5 AND 480
                                                   AND duration_min % 5 = 0)
);
CREATE INDEX barber_services_service_idx ON crm.barber_services (service_id);

-- ── Смены мастеров ──────────────────────────────────────────────────────────
CREATE TABLE crm.work_shifts (
    shift_id   bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    barber_id  integer NOT NULL REFERENCES crm.barbers (barber_id) ON DELETE CASCADE,
    branch_id  integer NOT NULL REFERENCES crm.branches (branch_id) ON DELETE RESTRICT,
    work_date  date    NOT NULL,
    starts_at  time    NOT NULL,
    ends_at    time    NOT NULL,
    CONSTRAINT work_shifts_barber_date_uq UNIQUE (barber_id, work_date),
    CONSTRAINT work_shifts_time_chk       CHECK (ends_at > starts_at),
    CONSTRAINT work_shifts_length_chk     CHECK (ends_at - starts_at >= interval '1 hour')
);
CREATE INDEX work_shifts_branch_date_idx ON crm.work_shifts (branch_id, work_date);

-- ── Клиенты ─────────────────────────────────────────────────────────────────
CREATE TABLE crm.clients (
    client_id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    phone             text        NOT NULL,
    full_name         text        NOT NULL,
    email             text,
    birth_date        date,
    notes             text        NOT NULL DEFAULT '',
    marketing_consent boolean     NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT clients_phone_uq   UNIQUE (phone),
    CONSTRAINT clients_phone_chk  CHECK (phone ~ '^\+7[0-9]{10}$'),
    CONSTRAINT clients_name_chk   CHECK (btrim(full_name) <> ''),
    CONSTRAINT clients_email_chk  CHECK (email ~* '^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$'),
    CONSTRAINT clients_birth_chk  CHECK (birth_date >= date '1900-01-01')
);
-- e-mail уникален без учёта регистра
CREATE UNIQUE INDEX clients_email_uq ON crm.clients (lower(email));

-- ── Бонусный счёт клиента (1:1 с clients) ───────────────────────────────────
CREATE TABLE crm.loyalty_accounts (
    client_id     bigint            PRIMARY KEY
                  REFERENCES crm.clients (client_id) ON DELETE CASCADE,
    level         crm.loyalty_level NOT NULL DEFAULT 'bronze',
    bonus_balance numeric(10,2)     NOT NULL DEFAULT 0,
    total_spent   numeric(12,2)     NOT NULL DEFAULT 0,
    updated_at    timestamptz       NOT NULL DEFAULT now(),
    CONSTRAINT loyalty_balance_chk CHECK (bonus_balance >= 0),
    CONSTRAINT loyalty_spent_chk   CHECK (total_spent >= 0)
);

-- ── Записи (визиты) ─────────────────────────────────────────────────────────
CREATE TABLE crm.appointments (
    appointment_id bigint                 GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    client_id      bigint                 NOT NULL
                   REFERENCES crm.clients (client_id) ON DELETE RESTRICT,
    barber_id      integer                NOT NULL
                   REFERENCES crm.barbers (barber_id) ON DELETE RESTRICT,
    starts_at      timestamptz            NOT NULL,
    ends_at        timestamptz            NOT NULL,
    status         crm.appointment_status NOT NULL DEFAULT 'scheduled',
    source         crm.booking_source     NOT NULL DEFAULT 'online',
    comment        text,
    cancel_reason  text,
    created_at     timestamptz            NOT NULL DEFAULT now(),
    updated_at     timestamptz            NOT NULL DEFAULT now(),
    CONSTRAINT appointments_time_chk
        CHECK (ends_at > starts_at AND ends_at - starts_at <= interval '8 hours'),
    CONSTRAINT appointments_minute_grid_chk
        CHECK (date_trunc('minute', starts_at) = starts_at),
    -- BR-6: причина обязательна ровно для отменённых записей
    CONSTRAINT appointments_cancel_reason_chk
        CHECK ((status = 'cancelled') = (cancel_reason IS NOT NULL AND btrim(cancel_reason) <> '')),
    -- BR-1: у мастера нет пересекающихся активных записей
    CONSTRAINT appointments_barber_no_overlap
        EXCLUDE USING gist (barber_id WITH =, tstzrange(starts_at, ends_at, '[)') WITH &&)
        WHERE (status NOT IN ('cancelled', 'no_show')),
    -- BR-2: у клиента нет пересекающихся активных записей
    CONSTRAINT appointments_client_no_overlap
        EXCLUDE USING gist (client_id WITH =, tstzrange(starts_at, ends_at, '[)') WITH &&)
        WHERE (status NOT IN ('cancelled', 'no_show'))
);

-- ── Состав записи: связь M:N «запись включает услугу» ───────────────────────
-- price/duration_min — снимок прайса мастера на момент записи (обоснованная
-- денормализация: прайс меняется, а история визитов — нет).
CREATE TABLE crm.appointment_services (
    appointment_id bigint        NOT NULL
                   REFERENCES crm.appointments (appointment_id) ON DELETE CASCADE,
    service_id     integer       NOT NULL
                   REFERENCES crm.services (service_id) ON DELETE RESTRICT,
    price          numeric(10,2) NOT NULL,
    duration_min   smallint      NOT NULL,
    PRIMARY KEY (appointment_id, service_id),
    CONSTRAINT appointment_services_price_chk    CHECK (price >= 0),
    CONSTRAINT appointment_services_duration_chk CHECK (duration_min BETWEEN 5 AND 480)
);
CREATE INDEX appointment_services_service_idx ON crm.appointment_services (service_id);

-- ── Оплаты ──────────────────────────────────────────────────────────────────
CREATE TABLE crm.payments (
    payment_id     bigint             GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    appointment_id bigint             NOT NULL
                   REFERENCES crm.appointments (appointment_id) ON DELETE RESTRICT,
    amount         numeric(10,2)      NOT NULL,
    method         crm.payment_method NOT NULL,
    paid_at        timestamptz        NOT NULL DEFAULT now(),
    CONSTRAINT payments_amount_chk CHECK (amount > 0)
);
CREATE INDEX payments_appointment_idx ON crm.payments (appointment_id);

-- ── Движение бонусов (журнал, только дописывается) ──────────────────────────
CREATE TABLE crm.bonus_transactions (
    bonus_tx_id    bigint            GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    client_id      bigint            NOT NULL
                   REFERENCES crm.loyalty_accounts (client_id) ON DELETE CASCADE,
    appointment_id bigint
                   REFERENCES crm.appointments (appointment_id) ON DELETE SET NULL,
    kind           crm.bonus_tx_kind NOT NULL,
    amount         numeric(10,2)     NOT NULL,
    comment        text,
    created_at     timestamptz       NOT NULL DEFAULT now(),
    CONSTRAINT bonus_tx_amount_chk CHECK (amount <> 0),
    CONSTRAINT bonus_tx_sign_chk CHECK (
        (kind = 'accrual' AND amount > 0)
        OR (kind IN ('redeem', 'expire') AND amount < 0)
        OR kind = 'adjustment')
);
CREATE INDEX bonus_tx_client_idx      ON crm.bonus_transactions (client_id, created_at);
CREATE INDEX bonus_tx_appointment_idx ON crm.bonus_transactions (appointment_id);

-- ── Отзывы (1:1 с appointments) ─────────────────────────────────────────────
CREATE TABLE crm.reviews (
    appointment_id bigint      PRIMARY KEY
                   REFERENCES crm.appointments (appointment_id) ON DELETE CASCADE,
    rating         smallint    NOT NULL,
    comment        text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT reviews_rating_chk CHECK (rating BETWEEN 1 AND 5)
);

-- ── История статусов записи ─────────────────────────────────────────────────
CREATE TABLE crm.appointment_status_history (
    history_id     bigint                 GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    appointment_id bigint                 NOT NULL
                   REFERENCES crm.appointments (appointment_id) ON DELETE CASCADE,
    old_status     crm.appointment_status,
    new_status     crm.appointment_status NOT NULL,
    changed_at     timestamptz            NOT NULL DEFAULT now(),
    changed_by     text                   NOT NULL DEFAULT current_user,
    CONSTRAINT status_history_change_chk CHECK (old_status IS DISTINCT FROM new_status)
);
CREATE INDEX status_history_appointment_idx
    ON crm.appointment_status_history (appointment_id, changed_at);

COMMENT ON SCHEMA crm IS 'Операционные данные CRM сети барбершопов';
COMMENT ON TABLE crm.appointments IS 'Записи клиентов к мастерам (визиты)';
COMMENT ON TABLE crm.appointment_services IS 'Услуги в составе записи; цена и длительность — снимок прайса мастера';
COMMENT ON TABLE crm.loyalty_accounts IS 'Бонусный счёт: bonus_balance = SUM(bonus_transactions.amount), поддерживается триггером';
