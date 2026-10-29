-- Q01. Расписание мастера на день (операционный запрос администратора/мастера).
-- Параметры: $1 — barber_id, $2 — дата (date).
-- Конструкции: многотабличный JOIN, LEFT JOIN, агрегация string_agg, GROUP BY.
SELECT a.appointment_id,
       to_char(a.starts_at AT TIME ZONE crm.local_tz(), 'HH24:MI') AS time_from,
       to_char(a.ends_at   AT TIME ZONE crm.local_tz(), 'HH24:MI') AS time_to,
       a.status,
       c.full_name                               AS client_name,
       c.phone                                   AS client_phone,
       la.level                                  AS client_level,
       string_agg(s.name, ', ' ORDER BY s.name)  AS services,
       coalesce(sum(x.price), 0)                 AS total_price,
       a.comment
FROM crm.appointments a
JOIN crm.clients c           ON c.client_id = a.client_id
JOIN crm.loyalty_accounts la ON la.client_id = a.client_id
LEFT JOIN crm.appointment_services x ON x.appointment_id = a.appointment_id
LEFT JOIN crm.services s             ON s.service_id = x.service_id
WHERE a.barber_id = $1
  AND a.starts_at >= crm.shift_start_ts($2::date, time '00:00')
  AND a.starts_at <  crm.shift_start_ts($2::date + 1, time '00:00')
GROUP BY a.appointment_id, c.client_id, la.client_id
ORDER BY a.starts_at;
