-- Масштабируемый набор данных для исследования производительности.
--
--   createdb barbershop_perf && migrate up -to 9   (схема без индексов оптимизации)
--   psql -d barbershop_perf -v scale=1 -f sql/perf/generate_perf_data.sql
--
-- scale = 1 → 40 мастеров, 30 000 клиентов, ~21 000 смен за 2 года,
--             ~170 000 записей, ~200 000 позиций, ~145 000 оплат (> 600 000 строк).
-- Записи генерируются множественными INSERT ... SELECT; непересечение
-- обеспечено построением (почасовые слоты, клиент не чаще раза в день),
-- поэтому EXCLUDE-ограничения и триггеры остаются включёнными.

\set ON_ERROR_STOP on
\if :{?scale}
\else
  \set scale 1
\endif
\timing on

BEGIN;
SET LOCAL crm.history_load = 'on';
SET LOCAL crm.audit_disabled = 'on';
SELECT setseed(0.2026);

INSERT INTO crm.branches (name, address, phone, opens_at, closes_at)
SELECT 'Филиал ' || i, 'Москва, ул. Нагрузочная, ' || i, '+7495' || lpad(i::text, 7, '0'), '09:00', '21:00'
FROM generate_series(1, 4 * :scale) i;

INSERT INTO crm.service_categories (name) VALUES ('Стрижки'), ('Борода'), ('Уход'), ('Комплексы');
INSERT INTO crm.services (category_id, name, base_price, base_duration_min)
SELECT 1 + (i % 4), 'Услуга ' || i, 500 + (i * 137) % 2500, (ARRAY[30, 45, 60])[1 + i % 3]
FROM generate_series(1, 20) i;

INSERT INTO crm.barbers (branch_id, full_name, phone, login, password_hash, grade, hired_at)
SELECT 1 + (i % (4 * :scale)), 'Мастер ' || i, '+7903' || lpad(i::text, 7, '0'), 'barber' || i, 'x',
       (ARRAY['junior','middle','senior','top'])[1 + i % 4]::crm.barber_grade, current_date - 800
FROM generate_series(1, 40 * :scale) i;

INSERT INTO crm.barber_services (barber_id, service_id, price, duration_min)
SELECT b.barber_id, s.service_id, round(s.base_price * (0.8 + (b.barber_id % 4) * 0.15) / 10) * 10,
       least(s.base_duration_min, 60)
FROM crm.barbers b CROSS JOIN crm.services s
WHERE (b.barber_id + s.service_id) % 5 <> 0;

INSERT INTO crm.clients (phone, last_name, first_name, created_at)
SELECT '+7916' || lpad(i::text, 7, '0'),
       (ARRAY['Иванов','Петров','Сидоров','Смирнов','Кузнецов','Попов','Васильев','Соколов','Михайлов','Новиков',
              'Фёдоров','Морозов','Волков','Алексеев','Лебедев','Семёнов','Егоров','Павлов','Козлов','Степанов'])[1 + i % 20]
         || CASE WHEN i % 7 = 0 THEN 'а' ELSE '' END,
       (ARRAY['Александр','Алексей','Андрей','Артём','Владимир','Дмитрий','Иван','Максим','Михаил','Никита',
              'Сергей','Роман','Павел','Олег','Егор','Кирилл','Илья','Тимур','Глеб','Антон'])[1 + (i * 7) % 20],
       now() - interval '800 days'
FROM generate_series(1, 30000 * :scale) i;

-- смены: 5/2, 10:00–19:00, 730 дней истории + 14 дней вперёд
INSERT INTO crm.work_shifts (barber_id, branch_id, work_date, starts_at, ends_at)
SELECT b.barber_id, b.branch_id, d::date, '10:00', '19:00'
FROM crm.barbers b
CROSS JOIN generate_series(current_date - 730, current_date + 14, interval '1 day') d
WHERE ((d::date - date '2024-01-01') + b.barber_id) % 7 < 5;

-- записи: почасовые слоты 10:00..18:00, занято ~85 %
CREATE TEMP TABLE gen AS
SELECT ws.barber_id, ws.work_date, k,
       crm.shift_start_ts(ws.work_date, '10:00') + make_interval(hours => k) AS starts_at,
       (SELECT array_agg(service_id ORDER BY service_id) FROM crm.barber_services bs WHERE bs.barber_id = ws.barber_id) AS svc,
       random() AS r_take, random() AS r_status, random() AS r_svc,
       -- клиент: не чаще одного визита в день → пересечения по клиенту невозможны
       1 + (((ws.work_date - (current_date - 730)) * 360 + (ws.barber_id % 40) * 9 + k) % (30000 * :scale)) AS client_id
FROM crm.work_shifts ws
CROSS JOIN generate_series(0, 8) k;
DELETE FROM gen WHERE r_take > 0.85;

INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at, status, source, cancel_reason, created_at, updated_at)
SELECT g.client_id, g.barber_id, g.starts_at,
       g.starts_at + make_interval(mins => bs.duration_min),
       CASE WHEN g.starts_at > now() THEN 'scheduled'
            WHEN g.r_status < 0.08 THEN 'cancelled'
            WHEN g.r_status < 0.15 THEN 'no_show'
            ELSE 'completed' END::crm.appointment_status,
       (ARRAY['online','online','phone','walk_in'])[1 + (g.k % 4)]::crm.booking_source,
       CASE WHEN g.starts_at <= now() AND g.r_status < 0.08 THEN 'Изменились планы' END,
       g.starts_at - interval '3 days', g.starts_at
FROM gen g
JOIN crm.barber_services bs
  ON bs.barber_id = g.barber_id
 AND bs.service_id = g.svc[1 + floor(g.r_svc * cardinality(g.svc))::int]
ORDER BY g.starts_at;

INSERT INTO crm.appointment_services (appointment_id, service_id, price, duration_min)
SELECT a.appointment_id, bs.service_id, bs.price, bs.duration_min
FROM crm.appointments a
JOIN crm.barber_services bs
  ON bs.barber_id = a.barber_id
 AND bs.duration_min = extract(epoch FROM a.ends_at - a.starts_at) / 60
 AND bs.service_id = (SELECT min(service_id) FROM crm.barber_services x
                      WHERE x.barber_id = a.barber_id
                        AND x.duration_min = extract(epoch FROM a.ends_at - a.starts_at) / 60);

-- оплаты завершённых визитов (триггер проверки оплат отключён только на время массовой загрузки:
-- сумма = стоимости по построению)
ALTER TABLE crm.payments DISABLE TRIGGER trg_payments_validate;
INSERT INTO crm.payments (appointment_id, amount, method, paid_at)
SELECT a.appointment_id, x.price, (ARRAY['card','card','cash','online'])[1 + a.appointment_id % 4]::crm.payment_method, a.ends_at
FROM crm.appointments a
JOIN crm.appointment_services x ON x.appointment_id = a.appointment_id
WHERE a.status = 'completed';
ALTER TABLE crm.payments ENABLE TRIGGER trg_payments_validate;

INSERT INTO crm.reviews (appointment_id, rating, created_at)
SELECT appointment_id, 3 + appointment_id % 3, ends_at + interval '2 hours'
FROM crm.appointments
WHERE status = 'completed' AND appointment_id % 5 = 0 AND ends_at > now() - interval '30 days';

DROP TABLE gen;
COMMIT;

VACUUM ANALYZE;
SELECT crm.refresh_kpi();

SELECT relname AS table_name, n_live_tup AS rows
FROM pg_stat_user_tables
WHERE schemaname = 'crm'
ORDER BY n_live_tup DESC;
