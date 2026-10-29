-- Q02. История визитов клиента: суммы, мастер, отзыв, интервал между визитами.
-- Параметры: $1 — client_id.
-- Конструкции: CTE, JOIN/LEFT JOIN, коррелированный подзапрос, оконная функция LAG.
WITH visits AS (
    SELECT a.appointment_id, a.starts_at, a.status, a.barber_id,
           (a.starts_at AT TIME ZONE crm.local_tz())::date AS visit_date
    FROM crm.appointments a
    WHERE a.client_id = $1
)
SELECT v.appointment_id,
       v.starts_at,
       v.status,
       b.full_name AS barber_name,
       (SELECT coalesce(sum(x.price), 0) FROM crm.appointment_services x
        WHERE x.appointment_id = v.appointment_id)               AS total_price,
       r.rating,
       CASE WHEN v.status = 'completed' THEN
           v.visit_date - lag(v.visit_date) OVER (PARTITION BY v.status = 'completed'
                                                  ORDER BY v.starts_at)
       END                                                       AS days_since_prev_visit
FROM visits v
JOIN crm.barbers b      ON b.barber_id = v.barber_id
LEFT JOIN crm.reviews r ON r.appointment_id = v.appointment_id
ORDER BY v.starts_at DESC;
