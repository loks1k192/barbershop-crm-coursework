-- Q10. Когортный анализ удержания: когорта — месяц первого завершённого визита,
-- для каждого следующего месяца — доля клиентов когорты, пришедших снова.
-- Параметры: $1 — первый месяц когорт (date).
-- Конструкции: многоэтапный CTE, DISTINCT, GROUP BY, FIRST_VALUE OVER.
WITH visits AS (
    SELECT DISTINCT a.client_id,
           date_trunc('month', a.starts_at AT TIME ZONE crm.local_tz())::date AS month
    FROM crm.appointments a
    WHERE a.status = 'completed'
), cohorts AS (
    SELECT client_id, min(month) AS cohort
    FROM visits
    GROUP BY client_id
), activity AS (
    SELECT c.cohort,
           ((extract(year FROM v.month) - extract(year FROM c.cohort)) * 12
             + extract(month FROM v.month) - extract(month FROM c.cohort))::integer AS month_no,
           count(DISTINCT v.client_id) AS active_clients
    FROM visits v
    JOIN cohorts c ON c.client_id = v.client_id
    GROUP BY c.cohort, month_no
)
SELECT cohort,
       month_no,
       active_clients,
       first_value(active_clients) OVER w                                  AS cohort_size,
       round(100.0 * active_clients / first_value(active_clients) OVER w, 1) AS retention_pct
FROM activity
WHERE cohort >= $1::date
WINDOW w AS (PARTITION BY cohort ORDER BY month_no)
ORDER BY cohort, month_no;
