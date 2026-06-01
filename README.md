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
make dev                      # поднимает Postgres в Docker + backend на :8080
make migrate                  # применить миграции
```

Проверка:

```bash
curl localhost:8080/api/health    # {"ok":true,"app":"tessera"}
curl localhost:8080/api/version   # {"api":"0.1.0"}
```

Всё в Docker:

```bash
make up        # postgres + backend
make logs
make down
```

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
