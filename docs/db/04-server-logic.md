# 4. Серверная логика и целостность данных

## 4.1. Функции и процедуры

| Функция | Тип | Назначение |
|---|---|---|
| `crm.book_appointment(phone, name, barber, starts_at, service_ids[], source, comment)` | бизнес-операция, `SECURITY DEFINER` | Запись клиента: проверка услуг и окна записи, поиск/создание клиента, запись, снимок цен. Возвращает `appointment_id` |
| `crm.complete_appointment(id, method, bonus)` | бизнес-операция, `SECURITY DEFINER` | Завершение визита: статус, оплаты (деньги + бонусы), списание/начисление бонусов, сумма покупок и уровень |
| `crm.cancel_appointment(id, reason)` | бизнес-операция | Отмена с причиной, проверка статуса (блокировка строки) |
| `crm.reschedule_appointment(id, new_start)` | бизнес-операция, `SECURITY DEFINER` | Перенос с сохранением длительности |
| `crm.free_slots(barber, date, duration, respect_lead, now)` | вычисление | Свободные слоты (сетка, смена, записи, компактный режим) |
| `staging.process_client_import(batch_id)` | бизнес-операция, `SECURITY DEFINER` | Пакетная загрузка с валидацией |
| `crm.refresh_kpi()` | служебная | Обновление MV |
| `crm.appointment_total`, `crm.accrual_rate`, `crm.loyalty_level_for`, `crm.split_person_name`, `crm.shift_start_ts`, `crm.client_search_text` | вычисления | Стоимость записи, ставка бонусов, уровень, разбор ФИО, перевод времени смены, выражение поиска |

## 4.2. Триггеры

| Триггер | Таблица | Событие | Что делает |
|---|---|---|---|
| `trg_appointments_validate` | appointments | BEFORE INSERT/UPDATE | Проверка BR-3 (запись внутри смены, смена `FOR SHARE`), BR-5 (переходы статусов, финальные статусы, `completed/no_show` только после начала) |
| `trg_appointments_status_history` | appointments | AFTER INSERT/UPDATE OF status | Автоматическая история статусов с пользователем приложения |
| `trg_work_shifts_validate` | work_shifts | BEFORE INSERT/UPDATE/DELETE | BR-4: смена в часах активного филиала, мастер не уволен, запрет «выбросить» активные записи из смены |
| `trg_payments_validate` | payments | BEFORE INSERT/UPDATE | BR-7: оплата только идущего/завершённого визита, Σ оплат ≤ стоимость (блокировка записи) |
| `trg_reviews_validate` | reviews | BEFORE INSERT/UPDATE | BR-9: только завершённый визит, не позже 30 дней |
| `trg_bonus_tx_apply` | bonus_transactions | AFTER INSERT | Атомарное изменение баланса счёта (CHECK ≥ 0) |
| `trg_loyalty_level` | loyalty_accounts | BEFORE UPDATE | Автоповышение уровня по сумме покупок |
| `trg_barbers_create_settings`, `trg_clients_create_loyalty` | barbers, clients | AFTER INSERT | Создание зависимых 1:1 записей |
| `trg_*_updated_at` | 5 таблиц | BEFORE UPDATE | Время изменения |
| `trg_audit` | appointments, clients, payments, barber_services, loyalty_accounts, work_shifts | AFTER INSERT/UPDATE/DELETE | Аудит (см. 4.3) |

## 4.3. Аудит изменений

Таблица `audit.audit_log`: `table_name`, `operation` (INSERT/UPDATE/DELETE),
`row_pk` (jsonb), `old_data` и `new_data` (полные образы строки, jsonb),
`changed_columns` (только реально изменённые столбцы; «пустые» UPDATE не пишутся),
`db_user`, `app_user` (из `crm.app_user`, который приложение устанавливает в каждой
транзакции по заголовку `X-User`), `txid`, `changed_at`.
Аудируется 6 значимых сущностей. Функция `audit.tg_audit` — `SECURITY DEFINER`:
приложение может только читать журнал (`GET /api/v1/audit`).

Пример (повышение цены мастера, записанное в seed):

```sql
SELECT operation, row_pk, old_data->>'price' AS old, new_data->>'price' AS new, changed_columns
FROM audit.audit_log WHERE table_name = 'crm.barber_services' LIMIT 1;
-- UPDATE | {"barber_id": 1, "service_id": 1} | 1730.00 | 1900.00 | {price}
```

## 4.4. Обработка исключительных ситуаций

* Бизнес-ошибки поднимаются `RAISE EXCEPTION USING ERRCODE = 'CR0xx'` с понятным
  сообщением; класс `CR` не пересекается со стандартными кодами PostgreSQL.
* Любое исключение откатывает всю транзакцию вызывающего — частичных изменений нет
  (демонстрация — раздел 5, тест `TestAtomicRollback`).
* В импорте ошибки отдельных строк не являются исключениями: они собираются в
  `staging.import_errors`, а исключение (сбой БД) откатывает пакет целиком; функция
  `staging.try_parse_date` перехватывает ошибку разбора даты блоком `EXCEPTION`.
* Приложение преобразует SQLSTATE в HTTP (`internal/apperr`):

| SQLSTATE | HTTP | Код ответа |
|---|---|---|
| 23505 unique_violation | 409 | `already_exists` |
| 23P01 exclusion_violation | 409 | `time_slot_conflict` |
| 23503 foreign_key_violation | 409 / 422 | `in_use` / `reference_not_found` |
| 23514 check_violation, 23502 | 422 | `constraint_violation` (`insufficient_bonus` для баланса) |
| CR001 | 404 | `not_found` |
| CR002, CR004, CR010 | 409 | `invalid_state`, `shift_has_appointments`, `import_error` |
| CR003, CR005–CR009 | 422 | `schedule_violation`, `payment_error`, `review_not_allowed`, `invalid_booking`, `booking_window`, `insufficient_bonus` |
| 22xxx | 400 | `invalid_input` |
| 40001, 40P01 | 503 (после 3 повторов) | `concurrent_update` |
| 42501 | 403 | `forbidden` |

## 4.5. Распределение логики между приложением и СУБД

В СУБД — всё, что определяет корректность данных независимо от клиента: ограничения,
межтабличные правила, составные операции, автоматизация (история, аудит, лояльность).
Причины: правила действуют и для API, и для импорта, и для ручной работы через psql;
межтабличные проверки и блокировки корректны только внутри транзакции СУБД;
`SECURITY DEFINER` позволяет не давать приложению прав на финансовые таблицы.

В приложении — то, что относится к взаимодействию: проверка формата входных данных с
сообщениями по полям, пагинация и сортировка, уровень изоляции и повтор транзакций,
HTTP-коды. Проверки формата (телефон, e-mail, диапазоны) намеренно есть и в API, и в
CHECK: в API — для удобных сообщений, в БД — как гарантия; бизнес-правила
(пересечения, смены, статусы, лимиты бонусов) реализованы **только** в СУБД.
