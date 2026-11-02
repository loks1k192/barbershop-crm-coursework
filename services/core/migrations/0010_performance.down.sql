DROP INDEX IF EXISTS crm.appointments_barber_starts_idx;
DROP INDEX IF EXISTS crm.appointments_client_starts_idx;
DROP INDEX IF EXISTS crm.appointments_completed_starts_idx;
DROP INDEX IF EXISTS crm.clients_search_trgm_idx;
DROP FUNCTION IF EXISTS crm.client_search_text(text, text, text);
-- представление и функция возвращаются к версиям из 0006/0003
CREATE OR REPLACE VIEW crm.v_client_stats AS
SELECT c.client_id, c.full_name, c.phone, c.email, la.level, la.bonus_balance, la.total_spent,
       count(a.appointment_id) FILTER (WHERE a.status = 'completed') AS visits_count,
       count(a.appointment_id) FILTER (WHERE a.status = 'no_show')   AS no_show_count,
       count(a.appointment_id) FILTER (WHERE a.status = 'cancelled') AS cancelled_count,
       max(a.starts_at) FILTER (WHERE a.status = 'completed')        AS last_visit_at,
       min(a.starts_at) FILTER (WHERE a.status IN ('scheduled', 'confirmed') AND a.starts_at > now()) AS next_visit_at,
       c.created_at
FROM crm.clients c
JOIN crm.loyalty_accounts la ON la.client_id = c.client_id
LEFT JOIN crm.appointments a ON a.client_id = c.client_id
GROUP BY c.client_id, la.client_id;
CREATE OR REPLACE FUNCTION crm.free_slots(p_barber_id integer, p_date date, p_duration_min integer,
    p_respect_lead boolean DEFAULT true, p_now timestamptz DEFAULT now())
RETURNS TABLE (slot_start timestamptz, slot_end timestamptz)
LANGUAGE sql STABLE AS $$
    WITH shift AS (
        SELECT crm.shift_start_ts(ws.work_date, ws.starts_at) AS sh_start,
               crm.shift_start_ts(ws.work_date, ws.ends_at) AS sh_end,
               bs.slot_step_min, bs.compact_slots, bs.min_lead_min
        FROM crm.work_shifts ws JOIN crm.barber_settings bs ON bs.barber_id = ws.barber_id
        WHERE ws.barber_id = p_barber_id AND ws.work_date = p_date
    ), grid AS (
        SELECT g AS slot_start, g + make_interval(mins => p_duration_min) AS slot_end, shift.*
        FROM shift, generate_series(shift.sh_start, shift.sh_end - make_interval(mins => p_duration_min),
                                    make_interval(mins => shift.slot_step_min)) AS g
    )
    SELECT grid.slot_start, grid.slot_end FROM grid
    WHERE (NOT p_respect_lead OR grid.slot_start >= p_now + make_interval(mins => grid.min_lead_min))
      AND NOT EXISTS (SELECT 1 FROM crm.appointments a WHERE a.barber_id = p_barber_id
              AND a.status NOT IN ('cancelled', 'no_show')
              AND a.starts_at < grid.slot_end AND a.ends_at > grid.slot_start)
      AND (NOT grid.compact_slots OR grid.slot_start = grid.sh_start OR grid.slot_end = grid.sh_end
           OR EXISTS (SELECT 1 FROM crm.appointments a WHERE a.barber_id = p_barber_id
                AND a.status NOT IN ('cancelled', 'no_show')
                AND (a.ends_at = grid.slot_start OR a.starts_at = grid.slot_end)))
    ORDER BY grid.slot_start
$$;
