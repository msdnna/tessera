# Tessera

Таск-трекер экосистемы **msdnna** — задачи, заметки и напоминания. Замена a reference tracker
с killer-фичей, которой нет у аналогов: **канбан-доска с группировкой по тегам**.

Визуальный референс — a reference tracker v3: слева дерево проектов, по центру канбан-доска,
управление единицами в модальных окнах. Адаптив десктоп/мобильный веб,
светлая/тёмная темы.

## Стек

| Слой      | Технологии                                              |
|-----------|---------------------------------------------------------|
| Backend   | Go + gin, PostgreSQL + JSONB (`pgx/v5`, `sqlc`, `golang-migrate`) |
| Realtime  | WebSocket (fan-out hub) — заложен с самого начала       |
| Frontend  | Vue 3 + Vite + Naive UI + Pinia (Фаза 2)                |
| Mobile    | Android — Kotlin + Compose (Фаза 8)                      |

Иерархия: **Workspace → Project Group → Project → Board → Column → Task**
(подзадачи деревом, теги и исполнители M:N).

## Быстрый старт (dev)

```bash
cp .env.example .env          # при необходимости поправь креды/проксю
make dev                      # поднимает Postgres в Docker + backend на :8090 (host)
make migrate                  # применить миграции
cd frontend && corepack yarn dev   # фронт на :5174, проксирует /api → :8090
```

Проверка:

```bash
curl localhost:8090/api/health    # {"ok":true,"app":"tessera"}
curl localhost:8090/api/version
```

Всё в Docker (dev):

```bash
make up        # postgres + backend (:8090) + frontend (:8083)
make migrate   # миграции (с хоста на localhost:5432)
make logs
make down
```

Открыть приложение: <http://localhost:8083>.

## Production

```bash
# Обязательно задать в .env: POSTGRES_PASSWORD, JWT_SECRET (openssl rand -hex 32)
docker compose -f docker-compose.prod.yml up -d --build
# применить миграции внутри прод-образа (distroless backend несёт бинарь /migrate):
docker compose -f docker-compose.prod.yml exec backend /migrate
```

Фронт (nginx) слушает `${WEB_PORT:-8082}` и проксирует `/api` (+ WebSocket) на
backend; Postgres наружу не публикуется. Backend — distroless, `APP_ENV=production`
(fail-closed на пустые `JWT_SECRET`/`DATABASE_URL`).

## Команды

`make help` — список целей. Основное: `dev`, `up`/`down`/`logs`, `migrate`,
`migrate-down`, `migrate-version`, `tidy`, `lint-backend`, `test-backend`,
`version`, `bump-api BUMP=minor`.

## Структура

```
backend/    Go API (handlers, repository, models, middleware, internal, migrations, cmd)
frontend/   Vue SPA               (Фаза 2)
android/    Android-приложение    (Фаза 8)
deploy/     прод-деплой           (Фаза 9)
tools/      вспомогательные скрипты (bump-version.sh)
```

## Тулчейн

- Go `1.25.9` (бинарь `go1.25.9`; Makefile зовёт его через `GO ?= go1.25.9`)
- Docker + Docker Compose
- Node + Yarn 4 (с Фазы 2)
