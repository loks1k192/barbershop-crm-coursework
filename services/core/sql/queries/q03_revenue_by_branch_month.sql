-- Q03. Выручка филиалов по месяцам: динамика к прошлому месяцу и доля в сети.
-- Параметры: $1 — начало периода (timestamptz), $2 — конец периода (не включая).
-- Филиал визита определяется по смене мастера на дату визита.
-- Конструкции: многоэтапный CTE, JOIN, GROUP BY, LAG, SUM() OVER (PARTITION BY).
WITH visit AS (
    SELECT a.appointment_id,
           date_trunc('month', a.starts_at AT TIME ZONE crm.local_tz())::date AS month,
           ws.branch_id
    FROM crm.appointments a
    JOIN crm.work_shifts ws
      ON ws.barber_id = a.barber_id
     AND ws.work_date = (a.starts_at AT TIME ZONE crm.local_tz())::date
    WHERE a.status = 'completed'
      AND a.starts_at >= $1 AND a.starts_at < $2
), monthly AS (
    SELECT v.branch_id, v.month,
           count(DISTINCT v.appointment_id) AS visits,
           sum(x.price)                     AS revenue
    FROM visit v
    JOIN crm.appointment_services x ON x.appointment_id = v.appointment_id
    GROUP BY v.branch_id, v.month
)
SELECT br.name                                   AS branch,
       m.month,
       m.visits,
       m.revenue,
       round(m.revenue / m.visits, 2)            AS avg_check,
       lag(m.revenue) OVER w                     AS prev_month_revenue,
       round(100.0 * (m.revenue - lag(m.revenue) OVER w)
             / nullif(lag(m.revenue) OVER w, 0), 1) AS mom_growth_pct,
       round(100.0 * m.revenue / sum(m.revenue) OVER (PARTITION BY m.month), 1) AS network_share_pct
FROM monthly m
JOIN crm.branches br ON br.branch_id = m.branch_id
WINDOW w AS (PARTITION BY m.branch_id ORDER BY m.month)
ORDER BY m.month, br.name;
