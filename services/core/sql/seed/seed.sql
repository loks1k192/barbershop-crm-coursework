-- Воспроизводимое начальное наполнение БД barbershop.
--
-- Запуск (роль-владелец схемы, после миграций):
--   psql "$CORE_DB_OWNER_URL" -v ON_ERROR_STOP=1 -f sql/seed/seed.sql
--
-- Детерминированность: setseed(0.42) — при одинаковой дате запуска результат
-- идентичен; даты строятся относительно current_date, поэтому в БД всегда есть
-- и история (≈150 дней), и будущие записи (14 дней).
--
-- Визиты проводятся через бизнес-функции (complete_appointment, cancel_appointment),
-- поэтому оплаты, бонусы, уровни лояльности и история статусов согласованы.
--
-- Отражённые состояния и пограничные случаи:
--   * все статусы записей: scheduled, confirmed, in_progress, completed, cancelled, no_show;
--   * историческое изменение цен: за 60 дней до текущей даты прайс мастеров повышен
--     на 10 % (старые визиты хранят старые цены; изменение есть в audit_log);
--   * уволенный мастер с историей визитов; неактивные услуга и филиал;
--   * клиенты без визитов, «ушедшие» клиенты, клиенты с несколькими неявками,
--     ручное присвоение уровня gold, приветственные бонусы, списания бонусов;
--   * мастер, работающий сменами в другом филиале.

\set ON_ERROR_STOP on

BEGIN;
SET LOCAL crm.history_load = 'on';     -- разрешает вставку исторических записей
SET LOCAL crm.audit_disabled = 'on';   -- начальная загрузка не засоряет журнал аудита
SET LOCAL crm.app_user = 'seed';
SELECT setseed(0.42);

-- ── Филиалы ─────────────────────────────────────────────────────────────────
INSERT INTO crm.branches (name, address, phone, opens_at, closes_at, is_active) VALUES
    ('Тверская', 'Москва, ул. Тверская, 12',          '+74951110001', '09:00', '21:00', true),
    ('Арбат',    'Москва, ул. Арбат, 35',             '+74951110002', '10:00', '22:00', true),
    ('Сокол',    'Москва, Ленинградский пр-т, 75',    '+74951110003', '09:00', '21:00', true),
    ('Химки',    'Химки, ул. Московская, 4',          '+74951110004', '10:00', '20:00', false);

-- ── Каталог услуг ───────────────────────────────────────────────────────────
INSERT INTO crm.service_categories (name, description) VALUES
    ('Стрижки',          'Мужские и детские стрижки'),
    ('Борода и бритьё',  'Моделирование бороды, бритьё опасной бритвой'),
    ('Уход',             'Камуфляж седины, уход за кожей'),
    ('Комплексы',        'Комбинированные услуги со скидкой');

INSERT INTO crm.services (category_id, name, base_price, base_duration_min, is_active)
SELECT c.category_id, s.name, s.price, s.dur, s.active
FROM (VALUES
    (1, 'Стрижки',         'Мужская стрижка',         1500, 45, true),
    (2, 'Стрижки',         'Стрижка машинкой',         900, 30, true),
    (3, 'Стрижки',         'Детская стрижка',         1100, 30, true),
    (4, 'Стрижки',         'Стрижка длинных волос',   2000, 60, true),
    (5, 'Борода и бритьё', 'Моделирование бороды',    1000, 30, true),
    (6, 'Борода и бритьё', 'Королевское бритьё',      1400, 45, true),
    (7, 'Борода и бритьё', 'Оформление усов',          500, 15, true),
    (8, 'Уход',            'Камуфляж седины',         1300, 30, true),
    (9, 'Уход',            'Мытьё и укладка',          700, 20, true),
    (10, 'Уход',            'Уход за кожей лица',      1600, 40, true),
    (11, 'Уход',            'Тонирование бровей',       600, 15, false),
    (12, 'Комплексы',       'Стрижка + борода',        2300, 75, true),
    (13, 'Комплексы',       'Отец и сын',              2400, 75, true),
    (14, 'Комплексы',       'Стрижка + камуфляж',      2600, 75, true)
) AS s(ord, cat, name, price, dur, active)
JOIN crm.service_categories c ON c.name = s.cat
ORDER BY s.ord;

-- ── Мастера (пароль у всех: password123, bcrypt) ────────────────────────────
INSERT INTO crm.barbers (branch_id, full_name, phone, login, password_hash, grade, hired_at, fired_at)
SELECT br.branch_id, b.name, b.phone, b.login,
       '$2a$04$izYDHmkQIgebbOWzMKX49OjKPa1rb5Qx/px.p7xluJs7/8weJkLOG',
       b.grade::crm.barber_grade, current_date - b.hired_days,
       CASE WHEN b.fired_days IS NOT NULL THEN current_date - b.fired_days END
FROM (VALUES
    (1, 'Тверская', 'Артём Волков',       '+79030000001', 'artem',    'top',    1200, NULL),
    (2, 'Тверская', 'Максим Орлов',       '+79030000002', 'maksim',   'senior',  900, NULL),
    (3, 'Тверская', 'Илья Соболев',       '+79030000003', 'ilya',     'middle',  400, NULL),
    (4, 'Тверская', 'Глеб Корнилов',      '+79030000004', 'gleb',     'junior',  170, NULL),
    (5, 'Арбат',    'Тимур Алиев',        '+79030000005', 'timur',    'top',    1500, NULL),
    (6, 'Арбат',    'Роман Ковалёв',      '+79030000006', 'roman',    'senior',  700, NULL),
    (7, 'Арбат',    'Денис Гусев',        '+79030000007', 'denis',    'middle',  350, NULL),
    (8, 'Арбат',    'Егор Белов',         '+79030000008', 'egor',     'junior',  120, NULL),
    (9, 'Сокол',    'Никита Зуев',        '+79030000009', 'nikita',   'senior', 1000, NULL),
    (10, 'Сокол',    'Павел Крылов',       '+79030000010', 'pavel',    'middle',  500, NULL),
    (11, 'Сокол',    'Степан Лавров',      '+79030000011', 'stepan',   'junior',  200, NULL),
    (12, 'Сокол',    'Олег Миронов',       '+79030000012', 'oleg',     'middle',  800, 40)
) AS b(ord, branch, name, phone, login, grade, hired_days, fired_days)
JOIN crm.branches br ON br.name = b.branch
ORDER BY b.ord;

-- Прайс мастеров: цена зависит от грейда, у junior длительность больше.
INSERT INTO crm.barber_services (barber_id, service_id, price, duration_min, is_active)
SELECT b.barber_id, s.service_id,
       round(s.base_price * CASE b.grade WHEN 'junior' THEN 0.80 WHEN 'middle' THEN 0.90
                                         WHEN 'senior' THEN 1.00 ELSE 1.25 END / 50) * 50 / 1.10,
       s.base_duration_min + CASE WHEN b.grade = 'junior' AND s.base_duration_min >= 30 THEN 10 ELSE 0 END,
       NOT (b.barber_id = 3 AND s.name = 'Королевское бритьё')
FROM crm.barbers b
CROSS JOIN crm.services s
WHERE s.is_active
  AND (b.barber_id + s.service_id) % 6 <> 0
  AND NOT (b.grade = 'junior' AND s.name IN ('Королевское бритьё', 'Уход за кожей лица'));
-- (цена делится на 1.10: через 60 дней «назад» прайс будет повышен на 10 %)
UPDATE crm.barber_services SET price = round(price / 10) * 10;

UPDATE crm.barber_settings
SET slot_step_min = CASE WHEN barber_id % 3 = 0 THEN 30 ELSE 15 END,
    compact_slots = (barber_id % 4 = 0),
    min_lead_min  = CASE WHEN barber_id % 2 = 0 THEN 120 ELSE 60 END,
    max_days_ahead = CASE WHEN barber_id IN (1, 5) THEN 60 ELSE 30 END;

-- ── Смены: график 5/2, чередование утро/вечер по неделям ────────────────────
-- Иногда мастер выходит в соседний филиал (каждый 11-й день).
INSERT INTO crm.work_shifts (barber_id, branch_id, work_date, starts_at, ends_at)
SELECT b.barber_id, br.branch_id, d::date,
       CASE WHEN ((d::date - date '2026-01-05') / 7 + b.barber_id) % 2 = 0
            THEN br.opens_at ELSE br.closes_at - interval '9 hours' END,
       CASE WHEN ((d::date - date '2026-01-05') / 7 + b.barber_id) % 2 = 0
            THEN br.opens_at + interval '9 hours' ELSE br.closes_at END
FROM crm.barbers b
CROSS JOIN generate_series(current_date - 150, current_date + 14, interval '1 day') AS d
JOIN crm.branches br
  ON br.branch_id = CASE WHEN (d::date - date '2026-01-05' + b.barber_id) % 11 = 0
                         THEN b.branch_id % 3 + 1 ELSE b.branch_id END
WHERE ((d::date - date '2026-01-05') + b.barber_id) % 7 < 5
  AND d::date >= b.hired_at
  AND (b.fired_at IS NULL OR d::date < b.fired_at);

-- ── Клиенты ─────────────────────────────────────────────────────────────────
CREATE TEMP TABLE tmp_client_life (
    client_id   bigint PRIMARY KEY,
    active_from date,
    active_to   date,
    fav_barber  integer,
    loyalty     numeric          -- вероятность выбрать «своего» мастера
) ON COMMIT DROP;

WITH names AS (
    SELECT ARRAY['Александр','Алексей','Андрей','Антон','Артём','Борис','Вадим','Виктор','Владимир',
                 'Глеб','Дмитрий','Евгений','Егор','Иван','Игорь','Илья','Кирилл','Константин',
                 'Максим','Матвей','Михаил','Никита','Николай','Олег','Павел','Роман','Руслан',
                 'Сергей','Степан','Тимур','Фёдор','Ярослав'] AS fn,
           ARRAY['Иванов','Смирнов','Кузнецов','Попов','Васильев','Петров','Соколов','Михайлов',
                 'Новиков','Фёдоров','Морозов','Волков','Алексеев','Лебедев','Семёнов','Егоров',
                 'Павлов','Козлов','Степанов','Николаев','Орлов','Андреев','Макаров','Никитин',
                 'Захаров','Зайцев','Соловьёв','Борисов','Яковлев','Григорьев','Романов','Воробьёв'] AS ln,
           ARRAY['Александрович','Алексеевич','Андреевич','Дмитриевич','Иванович','Михайлович',
                 'Николаевич','Сергеевич','Викторович','Олегович'] AS mn
), gen AS (
    SELECT i,
           names.ln[1 + (i * 7) % 32]  AS last_name,
           names.fn[1 + (i * 13) % 32] AS first_name,
           CASE WHEN i % 3 = 0 THEN names.mn[1 + (i * 3) % 10] END AS middle_name,
           '+7916' || lpad(((1000000 + i * 7919) % 9000000 + 1000000)::text, 7, '0') AS phone,
           random() AS r1, random() AS r2, random() AS r3, random() AS r4
    FROM generate_series(1, 650) AS i, names
), ins AS (
    INSERT INTO crm.clients (phone, last_name, first_name, middle_name, email, birth_date, notes,
                             marketing_consent, created_at, updated_at)
    SELECT phone, last_name, first_name, middle_name,
           CASE WHEN r1 < 0.4 THEN 'client' || i || '@mail.example' END,
           CASE WHEN r2 < 0.6 THEN date '1965-01-01' + (r2 * 1000 * 15)::int END,
           CASE WHEN i % 37 = 0 THEN 'Аллергия на средства с ментолом'
                WHEN i % 41 = 0 THEN 'Предпочитает короткие стрижки'
                ELSE '' END,
           r3 < 0.55,
           now() - make_interval(days => 160 - (i % 160)),
           now() - make_interval(days => 160 - (i % 160))
    FROM gen
    RETURNING client_id, created_at
)
INSERT INTO tmp_client_life
SELECT client_id,
       -- «окно активности» клиента: часть клиентов пришла недавно, часть ушла
       created_at::date + 3,
       CASE WHEN client_id % 5 = 0 THEN created_at::date + 30 + (client_id % 50)::int   -- ушедшие
            WHEN client_id % 23 = 0 THEN current_date - 400                        -- без визитов
            ELSE current_date + 30 END,
       1 + (client_id % 12)::int,
       CASE WHEN client_id % 9 = 0 THEN 0.95 ELSE 0.6 END
FROM ins;

-- Приветственный бонус за согласие на рассылку.
INSERT INTO crm.bonus_transactions (client_id, kind, amount, comment, created_at)
SELECT client_id, 'adjustment', 200, 'Приветственный бонус за согласие на рассылку', created_at
FROM crm.clients WHERE marketing_consent;

-- ── Записи и визиты ─────────────────────────────────────────────────────────
DO $$
DECLARE
    sh         record;
    t          timestamptz;
    v_svc      integer[];
    v_dur      integer;
    v_cl       bigint;
    v_appt     bigint;
    v_total    numeric;
    v_balance  numeric;
    v_bonus    numeric;
    r          double precision;
    k          integer;
    i          integer;
    n          integer;
    a_clients  bigint[];
    a_from     date[];
    a_to       date[];
    a_fav      integer[];
    a_loyal    numeric[];
    v_repriced boolean := false;
    v_sources  crm.booking_source[] := ARRAY['online','online','online','online','online','phone',
                                             'phone','phone','walk_in','walk_in','admin']::crm.booking_source[];
    v_reasons  text[] := ARRAY['Клиент заболел', 'Изменились планы', 'Перенос на другой день',
                               'Не успевает с работы', 'Мастер заболел'];
    v_comments text[] := ARRAY['Отличная стрижка, спасибо!', 'Всё понравилось', 'Быстро и аккуратно',
                               'Немного ждал, но результат хороший', 'Лучший мастер', 'Нормально',
                               'Буду приходить ещё', 'Не понравилась укладка'];
BEGIN
    SELECT array_agg(client_id ORDER BY client_id), array_agg(active_from ORDER BY client_id),
           array_agg(active_to ORDER BY client_id), array_agg(fav_barber ORDER BY client_id),
           array_agg(loyalty ORDER BY client_id)
    INTO a_clients, a_from, a_to, a_fav, a_loyal
    FROM tmp_client_life;
    n := cardinality(a_clients);

    FOR sh IN
        SELECT ws.*, crm.shift_start_ts(ws.work_date, ws.starts_at) AS s_ts,
               crm.shift_start_ts(ws.work_date, ws.ends_at) AS e_ts
        FROM crm.work_shifts ws
        ORDER BY ws.work_date, ws.barber_id
    LOOP
        -- историческое изменение цен: +10 % за 60 дней до текущей даты (попадает в аудит)
        IF NOT v_repriced AND sh.work_date >= current_date - 60 THEN
            SET LOCAL crm.audit_disabled = 'off';
            UPDATE crm.barber_services SET price = round(price * 1.10 / 10) * 10;
            SET LOCAL crm.audit_disabled = 'on';
            v_repriced := true;
        END IF;

        t := sh.s_ts + make_interval(mins => 15 * floor(random() * 3)::int);
        WHILE t < sh.e_ts LOOP
            SELECT array_agg(x.service_id), sum(x.duration_min)
            INTO v_svc, v_dur
            FROM (SELECT service_id, duration_min FROM crm.barber_services
                  WHERE barber_id = sh.barber_id AND is_active
                  ORDER BY random()
                  LIMIT CASE WHEN random() < 0.25 THEN 2 ELSE 1 END) x;
            EXIT WHEN t + make_interval(mins => v_dur) > sh.e_ts;

            IF random() < 0.72 THEN
                v_cl := NULL;
                FOR k IN 1..8 LOOP
                    i := 1 + floor(random() * n)::int;
                    IF a_from[i] <= sh.work_date AND a_to[i] >= sh.work_date
                       AND (a_fav[i] = sh.barber_id OR random() > a_loyal[i]) THEN
                        v_cl := a_clients[i];
                        EXIT;
                    END IF;
                END LOOP;

                IF v_cl IS NOT NULL THEN
                    BEGIN
                        INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at, status,
                                                      source, created_at, updated_at)
                        VALUES (v_cl, sh.barber_id, t, t + make_interval(mins => v_dur), 'scheduled',
                                v_sources[1 + floor(random() * 11)::int],
                                t - make_interval(hours => 2 + floor(random() * 240)::int),
                                t - make_interval(hours => 2 + floor(random() * 240)::int))
                        RETURNING appointment_id INTO v_appt;

                        INSERT INTO crm.appointment_services (appointment_id, service_id, price, duration_min)
                        SELECT v_appt, service_id, price, duration_min
                        FROM crm.barber_services
                        WHERE barber_id = sh.barber_id AND service_id = ANY (v_svc);

                        r := random();
                        IF t + make_interval(mins => v_dur) <= now() THEN          -- прошлое
                            IF r < 0.10 THEN
                                PERFORM crm.cancel_appointment(v_appt, v_reasons[1 + floor(random() * 5)::int]);
                            ELSIF r < 0.16 OR (v_cl % 47 = 0 AND r < 0.6) THEN    -- «злостные» неявки
                                UPDATE crm.appointments SET status = 'no_show' WHERE appointment_id = v_appt;
                            ELSE
                                IF random() < 0.5 THEN
                                    UPDATE crm.appointments SET status = 'confirmed' WHERE appointment_id = v_appt;
                                END IF;
                                v_total := crm.appointment_total(v_appt);
                                SELECT bonus_balance INTO v_balance FROM crm.loyalty_accounts WHERE client_id = v_cl;
                                v_bonus := CASE WHEN random() < 0.3 AND v_balance >= 100
                                                THEN least(v_balance, floor(v_total * 0.30)) ELSE 0 END;
                                PERFORM crm.complete_appointment(
                                    v_appt,
                                    (ARRAY['card','card','card','cash','cash','online'])[1 + floor(random() * 6)::int]::crm.payment_method,
                                    v_bonus);
                                IF random() < 0.35 THEN
                                    INSERT INTO crm.reviews (appointment_id, rating, comment, created_at)
                                    VALUES (v_appt,
                                            least(5, greatest(1, round(random_normal(4.7 - (sh.barber_id % 4) * 0.3, 0.7))))::smallint,
                                            CASE WHEN random() < 0.6 THEN v_comments[1 + floor(random() * 8)::int] END,
                                            least(now(), t + make_interval(hours => 1 + floor(random() * 72)::int)));
                                END IF;
                            END IF;
                        ELSIF t <= now() THEN                                      -- идёт сейчас
                            UPDATE crm.appointments SET status = 'in_progress' WHERE appointment_id = v_appt;
                        ELSE                                                       -- будущее
                            IF r < 0.06 THEN
                                PERFORM crm.cancel_appointment(v_appt, v_reasons[1 + floor(random() * 5)::int]);
                            ELSIF r < 0.40 THEN
                                UPDATE crm.appointments SET status = 'confirmed' WHERE appointment_id = v_appt;
                            END IF;
                        END IF;
                    EXCEPTION WHEN exclusion_violation THEN
                        NULL;   -- клиент уже записан на пересекающееся время — слот остаётся свободным
                    END;
                END IF;
                t := t + make_interval(mins => v_dur);
            ELSE
                t := t + make_interval(mins => 15 * (1 + floor(random() * 3)::int));
            END IF;
        END LOOP;
    END LOOP;
END $$;

-- Ручное присвоение уровня gold постоянным клиентам (уровень только повышается).
UPDATE crm.loyalty_accounts SET level = 'gold'
WHERE client_id IN (SELECT client_id FROM crm.loyalty_accounts ORDER BY total_spent DESC LIMIT 3);

-- ── Реалистичные отметки времени ────────────────────────────────────────────
-- Бизнес-функции ставят now(); переносим события на момент визита.
-- Триггеры записей временно отключаются (только для правки служебных дат).
ALTER TABLE crm.appointments DISABLE TRIGGER USER;
ALTER TABLE crm.payments DISABLE TRIGGER USER;

UPDATE crm.payments p SET paid_at = a.ends_at
FROM crm.appointments a WHERE a.appointment_id = p.appointment_id;

UPDATE crm.bonus_transactions b SET created_at = a.ends_at
FROM crm.appointments a WHERE a.appointment_id = b.appointment_id;

UPDATE crm.appointment_status_history h
SET changed_at = CASE h.new_status
        WHEN 'confirmed'   THEN a.created_at + (a.starts_at - a.created_at) / 2
        WHEN 'cancelled'   THEN a.created_at + (a.starts_at - a.created_at) * 0.8
        WHEN 'no_show'     THEN a.starts_at + interval '20 minutes'
        WHEN 'in_progress' THEN a.starts_at
        WHEN 'completed'   THEN a.ends_at
        ELSE h.changed_at END,
    changed_by = 'seed'
FROM crm.appointments a
WHERE a.appointment_id = h.appointment_id;

UPDATE crm.appointments a
SET updated_at = (SELECT max(h.changed_at) FROM crm.appointment_status_history h
                  WHERE h.appointment_id = a.appointment_id);

ALTER TABLE crm.appointments ENABLE TRIGGER USER;
ALTER TABLE crm.payments ENABLE TRIGGER USER;

SELECT crm.refresh_kpi();
COMMIT;

ANALYZE;

-- Итоги наполнения
SELECT 'branches' AS table_name, count(*) FROM crm.branches
UNION ALL SELECT 'barbers', count(*) FROM crm.barbers
UNION ALL SELECT 'services', count(*) FROM crm.services
UNION ALL SELECT 'barber_services', count(*) FROM crm.barber_services
UNION ALL SELECT 'work_shifts', count(*) FROM crm.work_shifts
UNION ALL SELECT 'clients', count(*) FROM crm.clients
UNION ALL SELECT 'appointments', count(*) FROM crm.appointments
UNION ALL SELECT 'appointment_services', count(*) FROM crm.appointment_services
UNION ALL SELECT 'payments', count(*) FROM crm.payments
UNION ALL SELECT 'bonus_transactions', count(*) FROM crm.bonus_transactions
UNION ALL SELECT 'reviews', count(*) FROM crm.reviews
UNION ALL SELECT 'appointment_status_history', count(*) FROM crm.appointment_status_history;
