-- 0010. Оптимизация производительности по результатам EXPLAIN (ANALYZE, BUFFERS)
-- на наборе ~170 000 записей (docs/db/06-performance.md).

-- (1) Расписание мастера за день/период (Q01, Q09, свободные слоты, KPI):
--     WHERE barber_id = ? AND starts_at >= ? AND starts_at < ?
CREATE INDEX appointments_barber_starts_idx ON crm.appointments (barber_id, starts_at);

-- (2) История и статистика клиента (Q02, v_client_stats, Q07); заодно индекс
--     внешнего ключа client_id (проверка ON DELETE RESTRICT без полного сканирования).
CREATE INDEX appointments_client_starts_idx ON crm.appointments (client_id, starts_at DESC);

-- (3) Периодные аналитические отчёты учитывают только завершённые визиты (Q03, Q04, Q06):
--     частичный индекс в 1.2 раза меньше полного и не обновляется для активных записей.
CREATE INDEX appointments_completed_starts_idx ON crm.appointments (starts_at) WHERE status = 'completed';

-- (4) Поиск клиента по подстроке ФИО/телефона/e-mail (ILIKE '%…%'): B-дерево не
--     применимо, используется триграммный GIN-индекс по выражению.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE FUNCTION crm.client_search_text(p_full_name text, p_phone text, p_email text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT lower(p_full_name || ' ' || p_phone || ' ' || coalesce(p_email, ''))
$$;
CREATE INDEX clients_search_trgm_idx ON crm.clients
    USING gin (crm.client_search_text(full_name, phone, email) gin_trgm_ops);

-- (5) Статистика клиента: агрегирование через LATERAL вместо GROUP BY по всем клиентам.
--     Прежняя версия вычисляла агрегаты для всех 30 000 клиентов даже при выборке
--     одной страницы; новая — только для отобранных строк (индекс (2)).
CREATE OR REPLACE VIEW crm.v_client_stats AS
SELECT c.client_id,
       c.full_name,
       c.phone,
       c.email,
       la.level,
       la.bonus_balance,
       la.total_spent,
       s.visits_count,
       s.no_show_count,
       s.cancelled_count,
       s.last_visit_at,
       s.next_visit_at,
       c.created_at
FROM crm.clients c
JOIN crm.loyalty_accounts la ON la.client_id = c.client_id
CROSS JOIN LATERAL (
    SELECT count(*) FILTER (WHERE a.status = 'completed')                          AS visits_count,
           count(*) FILTER (WHERE a.status = 'no_show')                            AS no_show_count,
           count(*) FILTER (WHERE a.status = 'cancelled')                          AS cancelled_count,
           max(a.starts_at) FILTER (WHERE a.status = 'completed')                  AS last_visit_at,
           min(a.starts_at) FILTER (WHERE a.status IN ('scheduled', 'confirmed')
                                      AND a.starts_at > now())                     AS next_visit_at
    FROM crm.appointments a
    WHERE a.client_id = c.client_id
) s;

-- (6) Свободные слоты: условие пересечения записано через && диапазонов —
--     так оно совпадает с выражением GiST-индекса ограничения
--     appointments_barber_no_overlap (тот же предикат частичного индекса), и
--     проверка каждого слота выполняется индексным поиском. Новых индексов не нужно.
CREATE OR REPLACE FUNCTION crm.free_slots(
    p_barber_id    integer,
    p_date         date,
    p_duration_min integer,
    p_respect_lead boolean     DEFAULT true,
    p_now          timestamptz DEFAULT now()
) RETURNS TABLE (slot_start timestamptz, slot_end timestamptz)
LANGUAGE sql STABLE AS $$
    WITH shift AS (
        SELECT crm.shift_start_ts(ws.work_date, ws.starts_at) AS sh_start,
               crm.shift_start_ts(ws.work_date, ws.ends_at)   AS sh_end,
               bs.slot_step_min, bs.compact_slots, bs.min_lead_min
        FROM crm.work_shifts ws
        JOIN crm.barber_settings bs ON bs.barber_id = ws.barber_id
        WHERE ws.barber_id = p_barber_id AND ws.work_date = p_date
    ), grid AS (
        SELECT g AS slot_start,
               g + make_interval(mins => p_duration_min) AS slot_end,
               shift.*
        FROM shift,
             generate_series(shift.sh_start,
                             shift.sh_end - make_interval(mins => p_duration_min),
                             make_interval(mins => shift.slot_step_min)) AS g
    )
    SELECT grid.slot_start, grid.slot_end
    FROM grid
    WHERE (NOT p_respect_lead OR grid.slot_start >= p_now + make_interval(mins => grid.min_lead_min))
      AND NOT EXISTS (
            SELECT 1 FROM crm.appointments a
            WHERE a.barber_id = p_barber_id
              AND a.status NOT IN ('cancelled', 'no_show')
              AND tstzrange(a.starts_at, a.ends_at, '[)') && tstzrange(grid.slot_start, grid.slot_end, '[)'))
      AND (NOT grid.compact_slots
           OR grid.slot_start = grid.sh_start
           OR grid.slot_end   = grid.sh_end
           OR EXISTS (
                SELECT 1 FROM crm.appointments a
                WHERE a.barber_id = p_barber_id
                  AND a.status NOT IN ('cancelled', 'no_show')
                  AND tstzrange(a.starts_at, a.ends_at, '[)') -|- tstzrange(grid.slot_start, grid.slot_end, '[)')))
    ORDER BY grid.slot_start
$$;

GRANT EXECUTE ON FUNCTION crm.client_search_text(text, text, text) TO crm_app;
GRANT SELECT ON crm.v_client_stats TO crm_app, crm_readonly;
