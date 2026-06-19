# Team Manager

REST API для управления задачами в командах: пользователи, команды с ролями
(`owner` / `admin` / `member`), задачи с фильтрами и историей изменений,
аналитика и кеш.

Стек: **Go · MySQL · Redis · Docker**.

## Запуск

Всё окружение одной командой (MySQL + Redis + миграции + сервис):

```bash
make up        # docker compose up -d --build
make logs      # логи сервиса
make down      # остановить
```

Сервис поднимется на `http://localhost:8080`.

Локально (инфраструктура в Docker, сервис из исходников):

```bash
docker compose up -d mysql redis
make migrate-up        # нужен golang-migrate
make run               # TM_CONFIG_PATH=config/local.yml go run ./cmd/teammanager
```

Конфиг — `config/local.yml` (или переменные окружения с префиксом `TM_`).

## API

База: `/api/v1`. Защищённые ручки требуют заголовок `Authorization: Bearer <token>`.

| Метод | Путь | Что делает |
|-------|------|------------|
| POST | `/register` | регистрация |
| POST | `/login` | вход, возвращает JWT |
| POST | `/teams` | создать команду (создатель — owner) |
| GET  | `/teams` | команды пользователя |
| POST | `/teams/{id}/invite` | пригласить участника (owner/admin) |
| POST | `/tasks` | создать задачу (член команды) |
| GET  | `/tasks?team_id=&status=&assignee_id=&limit=&offset=` | список с фильтрами и пагинацией |
| PUT  | `/tasks/{id}` | обновить задачу (с проверкой прав) |
| GET  | `/tasks/{id}/history` | история изменений задачи |
| POST | `/tasks/{id}/comments` | добавить комментарий (член команды) |
| GET  | `/tasks/{id}/comments?limit=&offset=` | комментарии задачи с пагинацией |
| PUT  | `/comments/{id}` | редактировать комментарий (только автор) |
| DELETE | `/comments/{id}` | удалить комментарий (автор или owner/admin) |
| GET  | `/analytics/team-stats` | участники + задачи `done` за 7 дней по командам |
| GET  | `/analytics/top-creators` | топ-3 создателя задач в каждой команде за месяц |
| GET  | `/analytics/integrity-issues` | задачи, где исполнитель не в команде |
| GET  | `/health` · `/metrics` | health-check · Prometheus |

Пример:

```bash
TOKEN=$(curl -s localhost:8080/api/v1/login \
  -d '{"email":"a@b.io","password":"password123"}' | jq -r .token)

curl localhost:8080/api/v1/tasks -H "Authorization: Bearer $TOKEN" \
  -d '{"team_id":1,"title":"Build API","status":"todo","assignee_id":2}'
```

## Разработка

```bash
make test-unit          # unit-тесты (мок-зависимости)
make test-integration   # интеграционные на реальном MySQL (testcontainers, нужен Docker)
make cover              # покрытие
make lint               # go vet + golangci-lint
```

## Структура

```
cmd/teammanager      точка входа, graceful shutdown
internal/
  config             конфиг (YAML + ENV)
  models             доменные модели
  adapters/mysql     репозитории + пул соединений
  adapters/cache     Redis-кеш списков задач
  adapters/email     мок email-сервиса с circuit breaker
  modules            бизнес-логика: auth / teams / tasks
  app/http           роутинг, обработчики, middleware
  platform           инфраструктура: logger / jwt / metrics / httpx
migrations/mysql     схема и индексы
```
