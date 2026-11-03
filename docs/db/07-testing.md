# 7. Тестирование

Автотесты — интеграционные, выполняются против настоящего PostgreSQL:
`internal/testdb` создаёт временную БД, применяет все миграции, загружает контрольный
набор [testdata/fixture.sql](../../services/core/testdata/fixture.sql) с заранее
рассчитанными результатами и создаёт временную логин-роль — **члена `crm_app`**, т. е.
тесты работают с правами приложения. После тестов БД и роль удаляются.

```bash
make core-test          # в контейнере (docker compose --profile test run --rm core-tests)
make core-test-local    # против локального PostgreSQL (TEST_PG_ADMIN_URL)
```

| Группа (ТЗ, раздел 8) | Тесты | Что проверяется |
|---|---|---|
| Ограничения целостности и некорректные операции | `TestIntegrityConstraints` (21 случай), `TestExclusionAndShiftRules` (10) | CHECK, UNIQUE (в т. ч. без учёта регистра), FK RESTRICT, EXCLUDE и граница `[)`, все триггеры бизнес-правил, отказ в правах (42501) на прямую запись в `payments`, `loyalty_accounts`, `appointments.starts_at`, `audit_log`, маппинг в HTTP |
| CRUD | `TestBranchCRUD`, `TestClientCRUDAndListing`, `TestBarberAndPriceList`, `TestShiftCRUD` | Создание/чтение/изменение/удаление, автосоздание 1:1, каскады, аудит изменений, фильтры, сортировка, пагинация |
| Бизнес-операции | `TestBookingScenario`, `TestCompleteVisitScenario`, `TestImportClients` | Слоты → запись → конфликт → перенос → отмена → история; оплата с бонусами (суммы, начисление 3 %, уровень), отзыв (срок, единственность); импорт (нормализация, upsert, ошибки) |
| Атомарность и откат | `TestAtomicRollback` | Ошибка внутри функции и ошибка **после** успешной функции в той же транзакции приложения — состояние 5 таблиц не меняется; затем успешное выполнение |
| Аналитика на контрольных данных | `TestReportBarberRanking` (Q04), `TestReportTopServices` (Q06), `TestReportNoShows` (Q08), `TestReportDailyRevenue` (Q11), `TestReportChurnRisk` (Q07), `TestReportsSmoke` | Значения, рассчитанные вручную по фикстуре: выручка 6200/1700/5000, средний чек 1550, рейтинг 4.50, загрузка 7.2 %, доли 53.6/46.4 %, неявки 66.7 %, нарастающий итог 12 900, скользящее среднее 1 843 и т. д. |
| Миграции | `TestMigrationPreservesData` | Миграция 0005 на заполненной таблице, корректный разбор ФИО, откат и повторное применение |
| Конкурентный доступ | `TestDoubleBookingAnomalyNaive`, `TestConcurrentBookingSameSlot`, `TestConcurrentBonusRedeem` | Воспроизведение аномалии; 10 параллельных записей → 1 успех + 9×409; два параллельных списания → 1 успех, баланс = сумма журнала |
| API (e2e) | `TestValidationAndErrorMapping`, `TestBookingFlowHTTP`, `TestListingAndReportsHTTP` | HTTP-коды 400/404/409/422, битый JSON, лишние поля, белый список сортировки, лимит страницы, X-User в истории и аудите, отчёты и MV |

Результат последнего прогона (`go test ./... -count=1 -race`, трижды подряд):

```text
ok  github.com/RomanKovalev007/barber_crm/services/core/internal/api    1.5s
ok  github.com/RomanKovalev007/barber_crm/services/core/internal/store  2.5s
```
