# Barber CRM

CRM-система для барбершопов: расписание, запись клиентов, аналитика.

## Курсовой проект: CRM сети барбершопов на единой БД PostgreSQL

На базе исходного проекта добавлен сервис **core** ([services/core](services/core)) с
единой нормализованной БД `barbershop` (15 таблиц, ограничения целостности, триггеры,
функции бизнес-операций, аудит, представления и MV, роли с минимальными правами),
REST API с OpenAPI, автотестами, исследованием конкурентного доступа и
производительности. Документация и диаграммы — [docs/README.md](docs/README.md).

### Запуск

```bash
cp .env.example .env          # задайте пароли ролей БД barbershop
make core-up                  # postgres → core-migrate → core-seed → core (:8090)
docker compose up -d          # вся платформа (шлюз :8080 проксирует /api/core/* в core)
```

| Адрес | Что |
|---|---|
| http://localhost:8090/docs | Swagger UI сервиса core |
| http://localhost:8090/openapi.yaml | Спецификация OpenAPI 3 |
| http://localhost:8090/api/v1/... | REST API core (напрямую) |
| http://localhost:8080/api/core/api/v1/... | Тот же API через api-gateway (JWT) |
| `make psql-core` | psql в БД `barbershop` |

### Демонстрация

```bash
# бизнес-сценарий 1: свободные слоты и запись
curl "localhost:8090/api/v1/barbers/1/free-slots?date=$(date -d tomorrow +%F)&service_ids=1"
curl -X POST localhost:8090/api/v1/appointments -H 'Content-Type: application/json' -H 'X-User: demo' \
  -d '{"client_phone":"+79991234567","client_name":"Иванов Иван","barber_id":1,
       "starts_at":"<слот из ответа>","service_ids":[1]}'
# повтор того же запроса другим клиентом → 409 time_slot_conflict (EXCLUDE)

# сценарий 2: завершение визита с оплатой и бонусами (визит, начавшийся в прошлом)
curl -X POST localhost:8090/api/v1/appointments/<id>/complete -d '{"payment_method":"card","bonus_to_redeem":100}'

# коллекции: фильтры, сортировка, пагинация
curl "localhost:8090/api/v1/clients?search=петров&min_visits=3&sort=-total_spent&page=1&page_size=10"

# отчёты
curl "localhost:8090/api/v1/reports/barber-ranking?from=2026-10-01&to=2026-11-01"
curl "localhost:8090/api/v1/reports/rfm"
curl -X POST localhost:8090/api/v1/reports/kpi/refresh

# пакетная загрузка клиентов из CSV (строки с ошибками попадают в отчёт)
make core-import-demo
# журнал аудита
curl "localhost:8090/api/v1/audit?table=crm.appointments&page_size=5"

make core-test                 # автотесты БД и API
make core-demo-concurrency     # транзакции, аномалии и их предотвращение (2 подключения)
make core-perf-data            # набор ≥ 100 000 записей для замеров
```

### Структура курсового проекта

```
services/core/
├── migrations/           # 0001–0010: схема, правила, функции, аудит, миграция данных,
│                         #            представления, импорт, роли, исправление, индексы
├── sql/queries/          # 11 аналитических и операционных запросов (встроены в API)
├── sql/seed/             # воспроизводимое начальное наполнение
├── sql/perf/             # генератор ≥100 000 записей и сценарий замеров
├── internal/api          # HTTP: маршруты, валидация, обработчики
├── internal/store        # доступ к данным + интеграционные тесты
├── internal/db, apperr   # транзакции с повтором, SQLSTATE → HTTP
├── cmd/                  # core, migrate, importer, concurrency-demo
├── api/openapi.yaml
└── testdata/fixture.sql  # контрольные данные с известными результатами
docs/db/                  # документация по БД
docs/it-project/          # ER по Чену, IDEF0, BPMN, архитектура
```

---

## Исходный проект

CRM-система для частных барберов: расписание, запись клиентов, аналитика.

Репозиторий с frontend: https://github.com/K4rens/Barber-CRM/tree/main

Поднятый удаленно сервис: http://132.243.16.248/

## Архитектура

```
                        ┌─────────────┐
          HTTP :8080    │             │
Client ─────────────── │ api-gateway │
                        │             │
                        └──────┬──────┘
                               │ gRPC
              ┌────────────────┼────────────────┐
              │                │                │               │
              ▼                ▼                ▼               ▼
         ┌─────────┐    ┌─────────┐    ┌───────────┐    ┌──────────┐
         │  staff  │    │ booking │    │ analytics │    │  client  │
         │  :50051 │    │  :50052 │    │   :50053  │    │  :50054  │
         └────┬────┘    └────┬────┘    └─────┬─────┘    └────┬─────┘
              │              │               │                │
         ┌────▼────┐    ┌────▼────┐    ┌─────▼─────┐    ┌────▼─────┐
         │Postgres │    │Postgres │    │ClickHouse │    │ Postgres │
         │staff_db │    │booking_d│    │ analytics │    │ client_db│
         └─────────┘    └─────────┘    └───────────┘    └──────────┘
              │              │
         ┌────▼────┐    ┌────▼────┐
         │  Redis  │    │  Redis  │
         └─────────┘    └─────────┘

         Kafka ──► analytics (consumer)
               ──► client    (consumer)
         staff ──► Kafka (producer: события бронирований)
         booking ─► Kafka (producer: события бронирований)

         Prometheus ◄── все сервисы (/metrics)
         Grafana    ◄── Prometheus
```

## Стек

| Слой | Технологии |
|---|---|
| Язык | Go 1.24 |
| Transport | gRPC, HTTP (chi v5) |
| Авторизация | JWT (golang-jwt/jwt/v5) |
| БД | PostgreSQL 16, ClickHouse 24.3 |
| Кэш | Redis 7 |
| Очереди | Kafka (KRaft, без ZooKeeper) |
| Метрики | Prometheus, Grafana |
| Конфиг | cleanenv (env-переменные) |
| Миграции | собственный мигратор в каждом сервисе |

## Структура репозитория

```
.
├── api/                        # Protobuf-определения + сгенерированный Go-код
│   └── proto/
│       ├── staff/v1/
│       ├── booking/v1/
│       ├── analytics/v1/
│       └── client/v1/
├── pkg/                        # Общие пакеты
│   ├── auth/                   # JWT: генерация, валидация, claims
│   ├── config/                 # Структуры конфигурации (cleanenv)
│   ├── logger/                 # slog-обёртка
│   ├── postgres/               # pgxpool
│   └── redis/                  # go-redis клиент
├── services/
│   ├── api-gateway/            # HTTP :8080 → gRPC прокси
│   ├── staff/                  # gRPC :50051, авторизация барберов, расписание
│   ├── booking/                # gRPC :50052, бронирования, слоты
│   ├── analytics/              # gRPC :50053, статистика из ClickHouse
│   └── client/                 # gRPC :50054, профили клиентов
├── infra/
│   ├── postgres/init.sql       # Инициализация баз данных
│   ├── prometheus/             # prometheus.yml
│   └── grafana/                # Provisioning: datasources, dashboards
└── docker-compose.yml
```

## Быстрый старт

```bash
docker-compose up -d
```

Поднимаются все сервисы, инфраструктура и запускаются миграции. После старта:

| Сервис | Адрес |
|---|---|
| API Gateway | http://localhost:8080 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 (admin/admin) |
| ClickHouse HTTP | http://localhost:8123 |
| PostgreSQL | localhost:5433 |
| Kafka | localhost:9092 |

## API

Базовый путь: `/api/v1`

### Публичные (без авторизации)

```
GET  /barbers
GET  /barbers/{barber_id}/services
GET  /barbers/{barber_id}/free-slots
POST /bookings
```

### Авторизация

```
POST /auth/login
POST /auth/refresh
POST /auth/logout          [JWT]
```

### Staff (требуют JWT)

```
GET    /staff/barber

GET    /staff/services
POST   /staff/services
PUT    /staff/services/{service_id}
DELETE /staff/services/{service_id}

GET    /staff/schedule
PUT    /staff/schedule/{date}
DELETE /staff/schedule/{date}
GET    /staff/slots

POST   /staff/bookings
GET    /staff/bookings/{booking_id}
PUT    /staff/bookings/{booking_id}
PATCH  /staff/bookings/{booking_id}
DELETE /staff/bookings/{booking_id}

GET    /staff/clients
GET    /staff/clients/{client_id}
PUT    /staff/clients/{client_id}

GET    /staff/analytics
```

### Служебные

```
GET /health
GET /metrics
```

## Конфигурация

Все параметры передаются через переменные окружения.

### api-gateway

| Переменная | По умолчанию | Описание |
|---|---|---|
| `HTTP_PORT` | `:8080` | HTTP-порт |
| `JWT_SECRET` | `jwt-secret` | Секрет для валидации JWT |
| `STAFF_ADDR` | `localhost:50051` | Адрес staff-сервиса |
| `BOOKING_ADDR` | `localhost:50052` | Адрес booking-сервиса |
| `ANALYTICS_ADDR` | `localhost:50053` | Адрес analytics-сервиса |
| `CLIENT_ADDR` | `localhost:50054` | Адрес client-сервиса |

### staff

| Переменная | По умолчанию | Описание |
|---|---|---|
| `GRPC_PORT` | `:50051` | gRPC-порт |
| `JWT_SECRET` | `jwt-secret` | Секрет для выдачи JWT |
| `METRICS_PORT` | `:9091` | Порт Prometheus /metrics |
| `POSTGRES_HOST` | `db` | — |
| `POSTGRES_PORT` | `5432` | — |
| `POSTGRES_USER` | `postgres` | — |
| `POSTGRES_PASSWORD` | `postgres` | — |
| `POSTGRES_DB` | — | Имя базы данных |
| `REDIS_HOST` | `localhost` | — |
| `REDIS_PORT` | `6379` | — |
| `REDIS_TTL_MINUTE` | `43200` | TTL кэша в минутах (30 дней) |
| `KAFKA_BROKERS` | `localhost:9092` | — |

### booking

| Переменная | По умолчанию | Описание |
|---|---|---|
| `GRPC_PORT` | `:50051` | gRPC-порт |
| `JWT_SECRET` | `jwt-secret` | — |
| `STAFF_GRPC_ADDR` | `staff:50051` | Адрес staff-сервиса |
| `METRICS_PORT` | `:9092` | Порт Prometheus /metrics |
| `POSTGRES_*` | — | Аналогично staff |
| `REDIS_*` | — | Аналогично staff |
| `KAFKA_BROKERS` | `localhost:9092` | — |

### analytics

| Переменная | По умолчанию | Описание |
|---|---|---|
| `GRPC_PORT` | `:50052` | gRPC-порт |
| `METRICS_PORT` | `:9093` | Порт Prometheus /metrics |
| `CLICKHOUSE_HOST` | `localhost` | — |
| `CLICKHOUSE_PORT` | `9000` | Нативный протокол |
| `CLICKHOUSE_DATABASE` | `analytics` | — |
| `CLICKHOUSE_USERNAME` | `default` | — |
| `CLICKHOUSE_PASSWORD` | — | — |
| `KAFKA_BROKERS` | `localhost:9092` | — |

### client

| Переменная | По умолчанию | Описание |
|---|---|---|
| `GRPC_PORT` | `:50053` | gRPC-порт |
| `METRICS_PORT` | `:9094` | Порт Prometheus /metrics |
| `POSTGRES_*` | — | Аналогично staff |
| `KAFKA_BROKERS` | `localhost:9092` | — |

## Метрики и наблюдаемость

Каждый сервис отдаёт метрики на своём порту:

| Сервис | Endpoint | Метрики |
|---|---|---|
| api-gateway | `localhost:8080/metrics` | `http_requests_total`, `http_request_duration_seconds` |
| staff | `localhost:9091/metrics` | `grpc_server_*` (go-grpc-prometheus) |
| booking | `localhost:9092/metrics` | `grpc_server_*` |
| analytics | `localhost:9093/metrics` | `grpc_server_*` |
| client | `localhost:9094/metrics` | `grpc_server_*` |

Prometheus собирает метрики автоматически. Grafana доступна по `http://localhost:3000`, datasource Prometheus уже добавлен через provisioning.

Рекомендуемые дашборды для импорта (Dashboards → Import → ID):
- `10818` — gRPC метрики
- `13407` — Go runtime
