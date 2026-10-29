-- Q09. Загрузка мастеров по неделям: время смен vs занятое записями время.
-- Параметры: $1 — первая дата, $2 — последняя дата, $3 — branch_id или NULL (все).
-- Конструкции: CTE, LEFT JOIN по диапазону времени, двухуровневая агрегация.
WITH shifts AS (
    SELECT ws.barber_id, ws.branch_id,
           date_trunc('week', ws.work_date)::date              AS week,
           crm.shift_start_ts(ws.work_date, ws.starts_at)      AS s_ts,
           crm.shift_start_ts(ws.work_date, ws.ends_at)        AS e_ts
    FROM crm.work_shifts ws
    WHERE ws.work_date BETWEEN $1 AND $2
      AND ($3::integer IS NULL OR ws.branch_id = $3)
), per_shift AS (
    SELECT sh.barber_id, sh.branch_id, sh.week,
           extract(epoch FROM sh.e_ts - sh.s_ts) / 60                          AS shift_min,
           coalesce(sum(extract(epoch FROM a.ends_at - a.starts_at) / 60), 0) AS booked_min,
           count(a.appointment_id)                                            AS appointments
    FROM shifts sh
    LEFT JOIN crm.appointments a
           ON a.barber_id = sh.barber_id
          AND a.starts_at >= sh.s_ts AND a.starts_at < sh.e_ts
          AND a.status <> 'cancelled'
    GROUP BY sh.barber_id, sh.branch_id, sh.week, sh.s_ts, sh.e_ts
)
SELECT br.name                                              AS branch,
       b.full_name                                          AS barber,
       ps.week,
       count(*)                                             AS shifts,
       round(sum(ps.shift_min))::integer                    AS shift_min,
       round(sum(ps.booked_min))::integer                   AS booked_min,
       sum(ps.appointments)::integer                        AS appointments,
       round((100 * sum(ps.booked_min) / sum(ps.shift_min))::numeric, 1) AS utilization_pct
FROM per_shift ps
JOIN crm.barbers b   ON b.barber_id = ps.barber_id
JOIN crm.branches br ON br.branch_id = ps.branch_id
GROUP BY br.branch_id, b.barber_id, ps.week
ORDER BY ps.week, br.name, utilization_pct DESC;
