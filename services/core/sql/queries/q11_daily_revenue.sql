-- Q11. Ежедневная выручка месяца: нарастающий итог и скользящее среднее за 7 дней.
-- Параметры: $1 — любой день месяца (date), $2 — branch_id или NULL (вся сеть).
-- Дни без выручки присутствуют (generate_series + LEFT JOIN).
-- Конструкции: generate_series, LEFT JOIN, SUM OVER (нарастающий итог),
-- AVG OVER (ROWS BETWEEN 6 PRECEDING AND CURRENT ROW).
WITH days AS (
    SELECT d::date AS day
    FROM generate_series(date_trunc('month', $1::date),
                         date_trunc('month', $1::date) + interval '1 month - 1 day',
                         interval '1 day') AS d
), daily AS (
    SELECT p.paid_at_local::date AS day, sum(p.amount) AS revenue, count(DISTINCT p.appointment_id) AS visits
    FROM (
        SELECT pm.amount, pm.appointment_id, pm.paid_at AT TIME ZONE crm.local_tz() AS paid_at_local, a.barber_id, a.starts_at
        FROM crm.payments pm
        JOIN crm.appointments a ON a.appointment_id = pm.appointment_id
        WHERE pm.paid_at >= crm.shift_start_ts(date_trunc('month', $1::date)::date, time '00:00')
          AND pm.paid_at <  crm.shift_start_ts((date_trunc('month', $1::date) + interval '1 month')::date, time '00:00')
    ) p
    LEFT JOIN crm.work_shifts ws
           ON ws.barber_id = p.barber_id
          AND ws.work_date = (p.starts_at AT TIME ZONE crm.local_tz())::date
    WHERE $2::integer IS NULL OR ws.branch_id = $2
    GROUP BY p.paid_at_local::date
)
SELECT d.day,
       coalesce(dl.visits, 0)                                         AS visits,
       coalesce(dl.revenue, 0)                                        AS revenue,
       sum(coalesce(dl.revenue, 0)) OVER (ORDER BY d.day)             AS running_total,
       round(avg(coalesce(dl.revenue, 0)) OVER (ORDER BY d.day
             ROWS BETWEEN 6 PRECEDING AND CURRENT ROW), 0)            AS moving_avg_7d
FROM days d
LEFT JOIN daily dl ON dl.day = d.day
ORDER BY d.day;
