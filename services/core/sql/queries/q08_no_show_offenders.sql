-- Q08. Клиенты с систематическими неявками (кандидаты на предоплату).
-- Параметры: $1 — начало периода, $2 — минимальное число неявок.
-- Конструкции: JOIN LATERAL, GROUP BY + HAVING, агрегаты с FILTER, EXISTS.
SELECT c.client_id, c.full_name, c.phone,
       count(*)                                                AS visits_due,
       count(*) FILTER (WHERE a.status = 'no_show')            AS no_shows,
       round(100.0 * count(*) FILTER (WHERE a.status = 'no_show') / count(*), 1) AS no_show_pct,
       max(a.starts_at) FILTER (WHERE a.status = 'no_show')    AS last_no_show,
       coalesce(sum(t.total) FILTER (WHERE a.status = 'no_show'), 0) AS lost_revenue,
       EXISTS (SELECT 1 FROM crm.appointments f
               WHERE f.client_id = c.client_id
                 AND f.status IN ('scheduled', 'confirmed')
                 AND f.starts_at > now())                      AS has_upcoming_booking
FROM crm.appointments a
JOIN crm.clients c ON c.client_id = a.client_id
CROSS JOIN LATERAL (
    SELECT sum(x.price) AS total FROM crm.appointment_services x
    WHERE x.appointment_id = a.appointment_id
) t
WHERE a.starts_at >= $1
  AND a.status IN ('completed', 'no_show')
GROUP BY c.client_id
HAVING count(*) FILTER (WHERE a.status = 'no_show') >= $2
ORDER BY no_shows DESC, no_show_pct DESC, c.client_id;
