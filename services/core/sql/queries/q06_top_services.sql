-- Q06. Популярность услуг по категориям за период.
-- Параметры: $1 — начало, $2 — конец периода, $3 — минимальное число продаж.
-- Конструкции: JOIN 4 таблиц, GROUP BY + HAVING, SUM(SUM()) OVER, RANK OVER.
SELECT cat.name                                   AS category,
       s.name                                     AS service,
       count(*)                                   AS times_sold,
       sum(x.price)                               AS revenue,
       round(avg(x.price), 0)                     AS avg_price,
       round(100.0 * sum(x.price) / sum(sum(x.price)) OVER (PARTITION BY cat.category_id), 1)
                                                  AS share_in_category_pct,
       rank() OVER (PARTITION BY cat.category_id ORDER BY sum(x.price) DESC) AS rank_in_category
FROM crm.appointment_services x
JOIN crm.appointments a          ON a.appointment_id = x.appointment_id
JOIN crm.services s              ON s.service_id = x.service_id
JOIN crm.service_categories cat  ON cat.category_id = s.category_id
WHERE a.status = 'completed'
  AND a.starts_at >= $1 AND a.starts_at < $2
GROUP BY cat.category_id, s.service_id
HAVING count(*) >= $3
ORDER BY cat.name, rank_in_category, s.name;
