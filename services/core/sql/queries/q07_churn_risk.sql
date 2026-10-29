-- Q07. Клиенты в зоне оттока: постоянные клиенты, давно не приходившие и не имеющие
-- будущих записей; с «любимым» мастером для персонального предложения.
-- Параметры: $1 — дата анализа, $2 — мин. число визитов, $3 — порог отсутствия (дней).
-- Конструкции: CTE, HAVING, NOT EXISTS, ROW_NUMBER OVER, LEFT JOIN.
WITH completed AS (
    SELECT a.client_id, a.barber_id, a.starts_at
    FROM crm.appointments a
    WHERE a.status = 'completed' AND a.starts_at <= $1
), per_client AS (
    SELECT client_id, count(*) AS visits, min(starts_at) AS first_visit, max(starts_at) AS last_visit
    FROM completed
    GROUP BY client_id
    HAVING count(*) >= $2
), favourite AS (
    SELECT client_id, barber_id,
           row_number() OVER (PARTITION BY client_id
                              ORDER BY count(*) DESC, max(starts_at) DESC) AS rn
    FROM completed
    GROUP BY client_id, barber_id
)
SELECT c.client_id, c.full_name, c.phone, c.marketing_consent,
       p.visits,
       p.last_visit,
       ($1::timestamptz::date - p.last_visit::date)                               AS days_since_last,
       round((extract(epoch FROM p.last_visit - p.first_visit) / 86400
              / nullif(p.visits - 1, 0))::numeric)                                AS avg_interval_days,
       b.full_name                                                                AS favourite_barber,
       la.bonus_balance
FROM per_client p
JOIN crm.clients c           ON c.client_id = p.client_id
JOIN crm.loyalty_accounts la ON la.client_id = p.client_id
LEFT JOIN favourite f        ON f.client_id = p.client_id AND f.rn = 1
LEFT JOIN crm.barbers b      ON b.barber_id = f.barber_id
WHERE p.last_visit < $1::timestamptz - make_interval(days => $3)
  AND NOT EXISTS (
        SELECT 1 FROM crm.appointments fut
        WHERE fut.client_id = p.client_id
          AND fut.status IN ('scheduled', 'confirmed')
          AND fut.starts_at > $1)
ORDER BY p.visits DESC, days_since_last DESC, c.client_id;
