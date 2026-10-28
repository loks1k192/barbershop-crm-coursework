-- 0006. Представления и материализованное представление для отчётности.

-- Полная карточка записи: клиент, мастер, филиал смены, состав, суммы, отзыв.
CREATE VIEW crm.v_appointment_details AS
SELECT a.appointment_id,
       a.starts_at,
       a.ends_at,
       (a.starts_at AT TIME ZONE crm.local_tz())::date AS local_date,
       a.status,
       a.source,
       a.client_id,
       c.full_name                       AS client_name,
       c.phone                           AS client_phone,
       a.barber_id,
       b.full_name                       AS barber_name,
       coalesce(ws.branch_id, b.branch_id) AS branch_id,
       br.name                           AS branch_name,
       svc.services,
       coalesce(svc.total_price, 0)      AS total_price,
       coalesce(svc.total_duration, 0)   AS total_duration_min,
       coalesce(pay.paid, 0)             AS paid_amount,
       r.rating,
       a.comment,
       a.cancel_reason,
       a.created_at,
       a.updated_at
FROM crm.appointments a
JOIN crm.clients c ON c.client_id = a.client_id
JOIN crm.barbers b ON b.barber_id = a.barber_id
LEFT JOIN crm.work_shifts ws
       ON ws.barber_id = a.barber_id
      AND ws.work_date = (a.starts_at AT TIME ZONE crm.local_tz())::date
LEFT JOIN crm.branches br ON br.branch_id = coalesce(ws.branch_id, b.branch_id)
LEFT JOIN LATERAL (
    SELECT string_agg(s.name, ', ' ORDER BY s.name) AS services,
           sum(x.price)        AS total_price,
           sum(x.duration_min) AS total_duration
    FROM crm.appointment_services x
    JOIN crm.services s ON s.service_id = x.service_id
    WHERE x.appointment_id = a.appointment_id
) svc ON true
LEFT JOIN LATERAL (
    SELECT sum(p.amount) AS paid FROM crm.payments p WHERE p.appointment_id = a.appointment_id
) pay ON true
LEFT JOIN crm.reviews r ON r.appointment_id = a.appointment_id;

COMMENT ON VIEW crm.v_appointment_details IS 'Карточка записи; филиал определяется по смене мастера на дату визита';

-- Статистика клиента — производные атрибуты, которые в исходной системе
-- хранились в таблице clients (visits_count, last_visit) и нарушали 3НФ.
CREATE VIEW crm.v_client_stats AS
SELECT c.client_id,
       c.full_name,
       c.phone,
       c.email,
       la.level,
       la.bonus_balance,
       la.total_spent,
       count(a.appointment_id) FILTER (WHERE a.status = 'completed') AS visits_count,
       count(a.appointment_id) FILTER (WHERE a.status = 'no_show')   AS no_show_count,
       count(a.appointment_id) FILTER (WHERE a.status = 'cancelled') AS cancelled_count,
       max(a.starts_at) FILTER (WHERE a.status = 'completed')        AS last_visit_at,
       min(a.starts_at) FILTER (WHERE a.status IN ('scheduled', 'confirmed')
                                  AND a.starts_at > now())           AS next_visit_at,
       c.created_at
FROM crm.clients c
JOIN crm.loyalty_accounts la ON la.client_id = c.client_id
LEFT JOIN crm.appointments a ON a.client_id = c.client_id
GROUP BY c.client_id, la.client_id;

-- Ежемесячные KPI мастеров. Тяжёлая агрегация по всей истории, нужна
-- дашборду руководителя; актуальность «на момент обновления» допустима.
CREATE MATERIALIZED VIEW crm.mv_barber_monthly_kpi AS
WITH appt AS (
    SELECT a.appointment_id,
           a.barber_id,
           date_trunc('month', a.starts_at AT TIME ZONE crm.local_tz())::date AS month,
           a.status,
           a.ends_at - a.starts_at AS duration
    FROM crm.appointments a
), money AS (
    SELECT x.appointment_id, sum(x.price) AS amount
    FROM crm.appointment_services x
    GROUP BY x.appointment_id
), agg AS (
    SELECT ap.barber_id,
           ap.month,
           count(*)                                         AS appointments_total,
           count(*) FILTER (WHERE ap.status = 'completed')  AS completed_cnt,
           count(*) FILTER (WHERE ap.status = 'cancelled')  AS cancelled_cnt,
           count(*) FILTER (WHERE ap.status = 'no_show')    AS no_show_cnt,
           coalesce(sum(m.amount) FILTER (WHERE ap.status = 'completed'), 0) AS revenue,
           coalesce(sum(ap.duration) FILTER (WHERE ap.status = 'completed'), interval '0') AS worked,
           avg(r.rating)                                    AS avg_rating,
           count(r.rating)                                  AS reviews_cnt
    FROM appt ap
    LEFT JOIN money m       ON m.appointment_id = ap.appointment_id
    LEFT JOIN crm.reviews r ON r.appointment_id = ap.appointment_id
    GROUP BY ap.barber_id, ap.month
), shifts AS (
    SELECT ws.barber_id,
           date_trunc('month', ws.work_date)::date AS month,
           count(*)                    AS shifts_cnt,
           sum(ws.ends_at - ws.starts_at) AS shift_time
    FROM crm.work_shifts ws
    GROUP BY ws.barber_id, date_trunc('month', ws.work_date)
)
SELECT agg.barber_id,
       agg.month,
       agg.appointments_total,
       agg.completed_cnt,
       agg.cancelled_cnt,
       agg.no_show_cnt,
       agg.revenue,
       round(agg.revenue / nullif(agg.completed_cnt, 0), 2)                          AS avg_check,
       round(100.0 * agg.no_show_cnt / nullif(agg.appointments_total, 0), 1)         AS no_show_pct,
       coalesce(sh.shifts_cnt, 0)                                                    AS shifts_cnt,
       round(extract(epoch FROM coalesce(sh.shift_time, interval '0')) / 3600, 1)    AS shift_hours,
       round(extract(epoch FROM agg.worked) / 3600, 1)                               AS worked_hours,
       round(100 * extract(epoch FROM agg.worked)
                 / nullif(extract(epoch FROM sh.shift_time), 0), 1)                  AS utilization_pct,
       round(agg.avg_rating, 2)                                                      AS avg_rating,
       agg.reviews_cnt,
       now()                                                                         AS refreshed_at
FROM agg
LEFT JOIN shifts sh ON sh.barber_id = agg.barber_id AND sh.month = agg.month
WITH DATA;

-- Уникальный индекс обязателен для REFRESH ... CONCURRENTLY.
CREATE UNIQUE INDEX mv_barber_monthly_kpi_uq ON crm.mv_barber_monthly_kpi (barber_id, month);

-- Механизм обновления. REFRESH может выполнять только владелец, поэтому функция
-- SECURITY DEFINER. CONCURRENTLY не блокирует чтение отчёта во время обновления.
-- Вызывается: фоновым планировщиком приложения (CORE_KPI_REFRESH_INTERVAL),
-- эндпоинтом POST /api/v1/reports/kpi/refresh, командой `make kpi-refresh`.
CREATE FUNCTION crm.refresh_kpi() RETURNS timestamptz
LANGUAGE plpgsql SECURITY DEFINER SET search_path = crm, pg_temp AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY crm.mv_barber_monthly_kpi;
    RETURN clock_timestamp();
END $$;
