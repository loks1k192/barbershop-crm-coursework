# Документация курсового проекта «CRM сети барбершопов»

Курсовой проект по двум дисциплинам на базе проекта Barber CRM:

* **«Базы данных»** — единая БД PostgreSQL `barbershop` и сервис `core`
  ([services/core](../services/core)); описание — [docs/db](db);
* **«IT-проекты»** — ER-диаграмма по Чену, функциональная модель IDEF0, BPMN,
  архитектура — [docs/it-project](it-project/README.md).

## Базы данных

| Раздел | Документ |
|---|---|
| 1. Предметная область, бизнес-процессы и правила | [db/01-domain.md](db/01-domain.md) |
| 2. Модель данных: ER-диаграммы, ФЗ, нормализация, ограничения, ON DELETE | [db/02-data-model.md](db/02-data-model.md) |
| 3. SQL-запросы, представления, материализованное представление | [db/03-sql-queries.md](db/03-sql-queries.md) |
| 4. Функции, триггеры, аудит, обработка ошибок | [db/04-server-logic.md](db/04-server-logic.md) |
| 5. Транзакции и конкурентный доступ | [db/05-transactions.md](db/05-transactions.md) |
| 6. Оптимизация и производительность | [db/06-performance.md](db/06-performance.md) |
| 7. Тестирование | [db/07-testing.md](db/07-testing.md) |
| 8. Вклад участников | [db/08-contribution.md](db/08-contribution.md) |

### Соответствие техническому заданию

| Требование ТЗ | Реализация |
|---|---|
| ≥ 8 содержательных связанных таблиц | 15 таблиц схемы `crm` ([0001](../services/core/migrations/0001_init_schema.up.sql)) |
| Связи 1:1, 1:N, M:N | 1:1 — `barber_settings`, `loyalty_accounts`, `reviews`; M:N — `barber_services`, `appointment_services`; остальные 1:N |
| 3НФ, обоснованная денормализация | [db/02-data-model.md §2.3–2.4](db/02-data-model.md) |
| Типы, PK/FK, NOT NULL, UNIQUE, CHECK; ON DELETE/UPDATE | [§2.5–2.6](db/02-data-model.md) |
| ≥ 3 нетривиальных бизнес-ограничений | BR-1…BR-9: EXCLUDE, 5 триггеров-валидаторов, CHECK ([§2.7](db/02-data-model.md)) |
| ER-диаграмма, ФЗ, обоснование | [er-physical.png](db/er-physical.png), [ER по Чену](it-project/01-er-chen/README.md) |
| ≥ 1000 записей, разные состояния, история | [seed.sql](../services/core/sql/seed/seed.sql): ≈ 7 500 записей и ≈ 50 000 строк, все статусы, изменение цен |
| ≥ 100 000 записей для исследования | [generate_perf_data.sql](../services/core/sql/perf/generate_perf_data.sql): 163 000 записей, ≈ 710 000 строк |
| Воспроизводимое наполнение, пакетная загрузка с валидацией | `setseed`, `core-seed`; [0007](../services/core/migrations/0007_import_staging.up.sql), `cmd/importer`, `POST /import/clients` |
| Миграция с сохранением данных | [0005_split_client_name](../services/core/migrations/0005_split_client_name.up.sql) + тест |
| ≥ 8 запросов (JOIN, агрегаты, HAVING, подзапросы, EXISTS, CTE, оконные) | 11 запросов [sql/queries](../services/core/sql/queries), 2 комплексных отчёта (Q04, Q05) |
| ≥ 2 VIEW, 1 MATERIALIZED VIEW с обновлением | `v_appointment_details`, `v_client_stats`, `mv_barber_monthly_kpi` + `refresh_kpi()` |
| ≥ 2 функции/процедуры | `book/complete/cancel/reschedule_appointment`, `free_slots`, `process_client_import` и др. |
| ≥ 2 триггера | 11 функций-триггеров ([db/04-server-logic.md](db/04-server-logic.md)) |
| Аудит ≥ 2 сущностей | `audit.audit_log`: 6 таблиц |
| Обработка исключений | коды `CR001…CR010`, маппинг SQLSTATE → HTTP |
| ≥ 2 составные атомарные операции | запись клиента, завершение визита, импорт |
| Успех и полный откат; конкурентный доступ; аномалии; защита; конфликт | [db/05-transactions.md](db/05-transactions.md), `cmd/concurrency-demo`, `concurrency_test.go` |
| ≥ 3 запроса, EXPLAIN (ANALYZE, BUFFERS), индексы, сравнительная таблица | 6 запросов, [db/06-performance.md](db/06-performance.md), планы в [db/perf](db/perf) |
| Приложение: CRUD, ≥ 3 сценария, отчёты, ошибки СУБД, фильтрация/сортировка/пагинация | REST API core; 5 сценариев (запись, завершение визита, отмена/перенос, отзыв, импорт) |
| OpenAPI | [api/openapi.yaml](../services/core/api/openapi.yaml), Swagger UI `/docs` |
| Отдельная роль с минимальными правами, секреты не в коде | `crm_app`, [0008](../services/core/migrations/0008_roles_grants.up.sql), `.env` |
| Автотесты: ограничения, CRUD, атомарность, ≥ 3 запроса с известным результатом | [db/07-testing.md](db/07-testing.md) |
| Docker Compose | `make core-up` ([README](../README.md)) |
