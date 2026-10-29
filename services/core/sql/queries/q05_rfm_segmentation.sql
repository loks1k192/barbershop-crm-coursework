-- Q05. Комплексный отчёт «RFM-сегментация клиентов» (многоэтапный).
-- Параметры: $1 — дата анализа (timestamptz); учитываются визиты за 365 дней до неё.
-- Этапы: (1) Recency/Frequency/Monetary по завершённым визитам → (2) баллы 1..5
-- квинтилями NTILE → (3) сегмент по правилам → (4) размер и средний чек сегмента.
-- Конструкции: CTE, LATERAL-подзапрос, GROUP BY, NTILE/COUNT/AVG OVER.
WITH base AS (
    SELECT a.client_id,
           max(a.starts_at)  AS last_visit,
           count(*)          AS frequency,
           sum(t.total)      AS monetary
    FROM crm.appointments a
    CROSS JOIN LATERAL (
        SELECT sum(x.price) AS total
        FROM crm.appointment_services x
        WHERE x.appointment_id = a.appointment_id
    ) t
    WHERE a.status = 'completed'
      AND a.starts_at >  $1::timestamptz - interval '365 days'
      AND a.starts_at <= $1::timestamptz
    GROUP BY a.client_id
), scores AS (
    SELECT b.*,
           ($1::timestamptz::date - b.last_visit::date)                  AS recency_days,
           ntile(5) OVER (ORDER BY b.last_visit, b.client_id)            AS r_score,
           ntile(5) OVER (ORDER BY b.frequency, b.client_id)             AS f_score,
           ntile(5) OVER (ORDER BY b.monetary, b.client_id)              AS m_score
    FROM base b
), segmented AS (
    SELECT s.*,
           CASE
               WHEN s.r_score >= 4 AND s.f_score >= 4 AND s.m_score >= 4 THEN 'Чемпионы'
               WHEN s.r_score >= 3 AND s.f_score >= 3                    THEN 'Лояльные'
               WHEN s.r_score >= 4 AND s.f_score <= 2                    THEN 'Новички'
               WHEN s.r_score <= 2 AND s.f_score >= 3                    THEN 'Под угрозой ухода'
               WHEN s.r_score <= 2 AND s.f_score <= 2                    THEN 'Потерянные'
               ELSE 'Требуют внимания'
           END AS segment
    FROM scores s
)
SELECT sg.client_id, c.full_name, c.phone,
       sg.recency_days, sg.frequency, sg.monetary,
       sg.r_score, sg.f_score, sg.m_score,
       concat(sg.r_score, sg.f_score, sg.m_score)                  AS rfm_code,
       sg.segment,
       count(*) OVER (PARTITION BY sg.segment)                     AS segment_size,
       round(avg(sg.monetary) OVER (PARTITION BY sg.segment), 0)   AS segment_avg_monetary
FROM segmented sg
JOIN crm.clients c ON c.client_id = sg.client_id
ORDER BY sg.segment, sg.monetary DESC, sg.client_id;
