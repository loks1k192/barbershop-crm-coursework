.PHONY: proto test lint build up down seed prod-up prod-down prod-pull prod-logs

SERVICES := staff booking analytics client api-gateway

proto:
	protoc --go_out=. --go_opt=module=github.com/RomanKovalev007/barber_crm \
		--go-grpc_out=. --go-grpc_opt=module=github.com/RomanKovalev007/barber_crm \
		-I api/proto \
		api/proto/staff/v1/staff.proto \
		api/proto/booking/v1/booking.proto \
		api/proto/analytics/v1/analytics.proto \
		api/proto/client/v1/client.proto

test:
	@for svc in $(SERVICES); do \
		echo "==> testing $$svc"; \
		(cd services/$$svc && go test ./... -race -count=1) || exit 1; \
	done

lint:
	@for svc in $(SERVICES); do \
		echo "==> linting $$svc"; \
		(cd services/$$svc && golangci-lint run ./...) || exit 1; \
	done

build:
	docker-compose build

up:
	docker-compose up -d

down:
	docker-compose down

seed:
	docker compose exec -T postgres psql -U postgres -f /docker-entrypoint-initdb.d/seed.sql

# ── Production ─────────────────────────────────────────────────────

prod-pull:
	docker compose -f docker-compose.prod.yml pull

prod-up:
	docker compose -f docker-compose.prod.yml up -d

prod-down:
	docker compose -f docker-compose.prod.yml down

prod-logs:
	docker compose -f docker-compose.prod.yml logs -f

# ── Курсовой проект: сервис core и БД barbershop ───────────────────

CORE := services/core
PG_ADMIN ?= postgres://postgres:postgres@localhost:5433/postgres?sslmode=disable

core-up:            ## поднять PostgreSQL + миграции + наполнение + API core (:8090)
	docker compose up -d --build postgres core-migrate core-seed core

core-test:          ## автотесты БД и API в контейнере
	docker compose --profile test run --rm core-tests

core-test-local:    ## автотесты против локального PostgreSQL
	cd $(CORE) && TEST_PG_ADMIN_URL=$(PG_ADMIN) go test ./... -count=1

core-demo-concurrency: ## демонстрация транзакций и конкурентного доступа
	cd $(CORE) && TEST_PG_ADMIN_URL=$(PG_ADMIN) go run ./cmd/concurrency-demo

core-perf-data:     ## набор данных ≥100 000 записей в БД barbershop_perf
	docker compose --profile perf run --rm core-perf

core-import-demo:   ## пакетная загрузка примера CSV
	docker compose run --rm --entrypoint importer core -file /opt/core/data/import/clients_legacy.csv

kpi-refresh:        ## обновить материализованное представление KPI
	docker compose exec -T postgres psql -U postgres -d barbershop -c "SELECT crm.refresh_kpi()"

psql-core:          ## psql в БД barbershop под ролью-владельцем
	docker compose exec postgres psql -U crm_owner -d barbershop
