.PHONY: build run test test-unit test-integration cover lint tidy \
        up down logs migrate-up migrate-down docker-build

CONFIG ?= config/local.yml
DB_URL ?= mysql://tm:tm@tcp(localhost:3306)/team_manager
MIGRATIONS ?= migrations/mysql

## build: собрать бинарник сервиса
build:
	go build -o bin/teammanager ./cmd/teammanager

## run: запустить сервис локально с config/local.yml
run:
	TM_CONFIG_PATH=$(CONFIG) go run ./cmd/teammanager

## test: запустить все тесты
test:
	go test ./... -count=1

## test-unit: только unit-тесты (без интеграционных/testcontainers)
test-unit:
	go test ./... -short -count=1

## test-integration: интеграционные тесты (требуют Docker для testcontainers)
test-integration:
	go test ./internal/adapters/mysql/... -run Integration -count=1

## cover: покрытие тестами критичных модулей
cover:
	go test ./internal/modules/... ./internal/pkg/... -coverprofile=coverage.out -count=1
	go tool cover -func=coverage.out | tail -n 20

## tidy: привести в порядок go.mod/go.sum
tidy:
	go mod tidy

## lint: статический анализ
lint:
	go vet ./...

## up: поднять всё окружение (mysql, redis, migrate, app)
up:
	docker compose up -d --build

## down: остановить окружение
down:
	docker compose down

## logs: логи приложения
logs:
	docker compose logs -f app

## docker-build: собрать образ приложения
docker-build:
	docker build -t team-manager:latest .

## migrate-up: применить миграции (нужен golang-migrate)
migrate-up:
	migrate -path $(MIGRATIONS) -database "$(DB_URL)" up

## migrate-down: откатить миграции
migrate-down:
	migrate -path $(MIGRATIONS) -database "$(DB_URL)" down
