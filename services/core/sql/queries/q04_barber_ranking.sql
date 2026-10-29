-- Q04. Комплексный отчёт «Рейтинг мастеров за период» (многоэтапный).
-- Параметры: $1 — начало периода (timestamptz), $2 — конец периода (не включая).
-- Этапы: (1) записи периода → (2) показатели по мастеру: визиты, неявки, отработанное
-- время → (3) выручка → (4) рейтинг отзывов → (5) длительность смен → (6) сводка и
-- загрузка → (7) ранжирование в филиале и сравнение со средним по филиалу.
-- Конструкции: CTE, LEFT JOIN, агрегаты с FILTER, AVG/RANK/DENSE_RANK OVER.
WITH period_appts AS (
    SELECT a.appointment_id, a.barber_id, a.status, a.starts_at, a.ends_at
    FROM crm.appointments a
    WHERE a.starts_at >= $1 AND a.starts_at < $2
), barber_stats AS (
    SELECT pa.barber_id,
           count(*)                                        AS appointments_total,
           count(*) FILTER (WHERE pa.status = 'completed') AS completed,
           count(*) FILTER (WHERE pa.status = 'no_show')   AS no_shows,
           sum(extract(epoch FROM pa.ends_at - pa.starts_at) / 60)
               FILTER (WHERE pa.status = 'completed')      AS worked_min
    FROM period_appts pa
    GROUP BY pa.barber_id
), revenue AS (
    SELECT pa.barber_id, sum(x.price) AS revenue
    FROM period_appts pa
    JOIN crm.appointment_services x ON x.appointment_id = pa.appointment_id
    WHERE pa.status = 'completed'
    GROUP BY pa.barber_id
), ratings AS (
    SELECT pa.barber_id, avg(r.rating) AS avg_rating, count(*) AS reviews
    FROM period_appts pa
    JOIN crm.reviews r ON r.appointment_id = pa.appointment_id
    GROUP BY pa.barber_id
), shift_time AS (
    SELECT ws.barber_id, sum(extract(epoch FROM ws.ends_at - ws.starts_at) / 60) AS shift_min
    FROM crm.work_shifts ws
    WHERE ws.work_date >= ($1::timestamptz AT TIME ZONE crm.local_tz())::date
      AND ws.work_date <  ($2::timestamptz AT TIME ZONE crm.local_tz())::date
    GROUP BY ws.barber_id
), scored AS (
    SELECT b.barber_id, b.full_name, b.grade, b.branch_id,
           coalesce(bs.completed, 0)                                   AS completed,
           coalesce(rv.revenue, 0)                                     AS revenue,
           round(rv.revenue / nullif(bs.completed, 0), 2)              AS avg_check,
           round(100.0 * bs.no_shows / nullif(bs.appointments_total, 0), 1) AS no_show_pct,
           round(rt.avg_rating, 2)                                     AS avg_rating,
           coalesce(rt.reviews, 0)                                     AS reviews,
           round((100 * bs.worked_min / nullif(st.shift_min, 0))::numeric, 1) AS utilization_pct
    FROM crm.barbers b
    LEFT JOIN barber_stats bs ON bs.barber_id = b.barber_id
    LEFT JOIN revenue rv      ON rv.barber_id = b.barber_id
    LEFT JOIN ratings rt      ON rt.barber_id = b.barber_id
    LEFT JOIN shift_time st   ON st.barber_id = b.barber_id
    WHERE bs.barber_id IS NOT NULL OR st.barber_id IS NOT NULL
)
SELECT br.name AS branch,
       s.barber_id, s.full_name, s.grade, s.completed, s.revenue, s.avg_check,
       s.no_show_pct, s.avg_rating, s.reviews, s.utilization_pct,
       round(s.revenue - avg(s.revenue) OVER (PARTITION BY s.branch_id), 0) AS revenue_vs_branch_avg,
       rank()       OVER (PARTITION BY s.branch_id ORDER BY s.revenue DESC)    AS rank_in_branch,
       dense_rank() OVER (ORDER BY s.avg_rating DESC NULLS LAST)              AS rating_rank
FROM scored s
JOIN crm.branches br ON br.branch_id = s.branch_id
ORDER BY br.name, rank_in_branch, s.barber_id;
