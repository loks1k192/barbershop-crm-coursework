-- Контрольный набор данных для автотестов с заранее известными результатами.
-- Исторические данные — март 2026 (фиксированные даты), для сценариев записи —
-- смены на ближайшие дни относительно current_date.
--
-- Ожидаемые результаты (период 2026-03-01 .. 2026-04-01):
--   Q04 рейтинг: Волков (b1) — 4 визита, выручка 6200, ср. чек 1550, рейтинг 4.50, загрузка 7.2 %;
--                Глебов (b2) — 1 визит, 1700, неявки 66.7 %; Алиев (b3) — 2 визита, 5000, рейтинг 4.00
--   Q06 услуги: «Стрижка» 4 продажи / 6000 (53.6 % категории), «Комплекс» 2 / 5200, «Борода» 2 / 1700
--   Q08 неявки: только клиент «Неявкин» — 2 неявки из 3, 66.7 %, потеряно 2000
--   Q11 выручка марта: 02.03 — 6700, 03.03 — 4700, 05.03 — 1500, итог 12900
--   Q07 отток (на 2026-05-01, ≥2 визитов, >30 дней): только «Первый Клиент»
SET crm.history_load = 'on';
SET crm.audit_disabled = 'on';

INSERT INTO crm.branches (name, address, phone, opens_at, closes_at) VALUES
    ('Филиал А', 'Москва, ул. Тестовая, 1', '+74950000001', '09:00', '21:00'),
    ('Филиал Б', 'Москва, ул. Тестовая, 2', '+74950000002', '10:00', '22:00');

INSERT INTO crm.service_categories (name) VALUES ('Стрижки'), ('Борода');
INSERT INTO crm.services (category_id, name, base_price, base_duration_min) VALUES
    (1, 'Стрижка', 1500, 45), (2, 'Борода', 1000, 30), (1, 'Комплекс', 2200, 75);

INSERT INTO crm.barbers (branch_id, full_name, phone, login, password_hash, grade, hired_at) VALUES
    (1, 'Артём Волков', '+79000000001', 'volkov', 'x', 'senior', '2025-01-01'),
    (1, 'Глеб Глебов',  '+79000000002', 'glebov', 'x', 'junior', '2025-06-01'),
    (2, 'Тимур Алиев',  '+79000000003', 'aliev',  'x', 'top',    '2024-01-01');
UPDATE crm.barber_settings SET min_lead_min = 0;

INSERT INTO crm.barber_services (barber_id, service_id, price, duration_min) VALUES
    (1, 1, 1500, 45), (1, 2, 1000, 30), (1, 3, 2200, 75),
    (2, 1, 1000, 60), (2, 2,  700, 40),
    (3, 1, 2000, 45), (3, 3, 3000, 75);

-- смены 02.03–06.03: b1, b2 в филиале А, b3 в филиале Б
INSERT INTO crm.work_shifts (barber_id, branch_id, work_date, starts_at, ends_at)
SELECT b.barber_id, b.branch_id, d::date, '10:00', '19:00'
FROM crm.barbers b, generate_series(date '2026-03-02', date '2026-03-06', interval '1 day') d;
INSERT INTO crm.work_shifts (barber_id, branch_id, work_date, starts_at, ends_at) VALUES
    (3, 2, '2026-05-10', '10:00', '19:00');
-- вчерашние и будущие смены для сценариев записи и отзывов
INSERT INTO crm.work_shifts (barber_id, branch_id, work_date, starts_at, ends_at)
SELECT b.barber_id, b.branch_id, current_date + d, '10:00', '19:00'
FROM crm.barbers b, generate_series(-1, 3) d WHERE d <> 0;

INSERT INTO crm.clients (phone, last_name, first_name, marketing_consent) VALUES
    ('+79160000001', 'Клиент', 'Первый', true),     -- c1
    ('+79160000002', 'Клиент', 'Второй', false),    -- c2
    ('+79160000003', 'Клиент', 'Третий', true),     -- c3
    ('+79160000004', 'Неявкин', 'Пётр', false),     -- c4
    ('+79160000005', 'Клиент', 'Пятый', true);      -- c5

CREATE TEMP TABLE fx (n int, client bigint, barber int, ts timestamptz, svc int[], st crm.appointment_status,
                      method crm.payment_method, rating int);
INSERT INTO fx VALUES
    ( 1, 1, 1, '2026-03-02 10:00+03', '{1}',   'completed', 'card',   5),
    ( 2, 2, 1, '2026-03-02 11:00+03', '{2}',   'completed', 'cash',   NULL),
    ( 3, 3, 1, '2026-03-02 12:00+03', '{3}',   'completed', 'card',   4),
    ( 4, 1, 2, '2026-03-03 10:00+03', '{1,2}', 'completed', 'card',   NULL),
    ( 5, 4, 2, '2026-03-03 12:00+03', '{1}',   'no_show',   NULL,     NULL),
    ( 6, 4, 2, '2026-03-04 12:00+03', '{1}',   'no_show',   NULL,     NULL),
    ( 7, 4, 1, '2026-03-05 12:00+03', '{1}',   'completed', 'card',   NULL),
    ( 8, 5, 3, '2026-03-02 10:00+03', '{1}',   'completed', 'online', 5),
    ( 9, 5, 3, '2026-03-03 10:00+03', '{3}',   'completed', 'card',   3),
    (10, 2, 3, '2026-03-04 10:00+03', '{1}',   'cancelled', NULL,     NULL),
    (11, 5, 3, '2026-05-10 11:00+03', '{1}',   'scheduled', NULL,     NULL),
    (12, 3, 1, '2026-03-06 10:00+03', '{1,2}', 'scheduled', NULL,     NULL);  -- не закрытый визит в прошлом

INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at, status, source, cancel_reason, created_at, updated_at)
SELECT fx.client, fx.barber, fx.ts,
       fx.ts + make_interval(mins => (SELECT sum(duration_min)::int FROM crm.barber_services
                                      WHERE barber_id = fx.barber AND service_id = ANY (fx.svc))),
       fx.st, 'online', CASE WHEN fx.st = 'cancelled' THEN 'Изменились планы' END,
       fx.ts - interval '1 day', fx.ts
FROM fx ORDER BY fx.n;

INSERT INTO crm.appointment_services (appointment_id, service_id, price, duration_min)
SELECT a.appointment_id, bs.service_id, bs.price, bs.duration_min
FROM fx
JOIN crm.appointments a ON a.client_id = fx.client AND a.starts_at = fx.ts
JOIN crm.barber_services bs ON bs.barber_id = fx.barber AND bs.service_id = ANY (fx.svc);

INSERT INTO crm.payments (appointment_id, amount, method, paid_at)
SELECT a.appointment_id, crm.appointment_total(a.appointment_id), fx.method, a.ends_at
FROM fx JOIN crm.appointments a ON a.client_id = fx.client AND a.starts_at = fx.ts
WHERE fx.st = 'completed';

INSERT INTO crm.reviews (appointment_id, rating, created_at)
SELECT a.appointment_id, fx.rating, a.ends_at + interval '1 hour'
FROM fx JOIN crm.appointments a ON a.client_id = fx.client AND a.starts_at = fx.ts
WHERE fx.rating IS NOT NULL;

-- бонусы: c3 — 500 для проверки списания
INSERT INTO crm.bonus_transactions (client_id, kind, amount, comment) VALUES
    (3, 'adjustment', 500, 'Тестовое начисление');

DROP TABLE fx;
RESET crm.history_load;
RESET crm.audit_disabled;
SELECT crm.refresh_kpi();
