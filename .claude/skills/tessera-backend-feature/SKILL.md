---
name: tessera-backend-feature
description: Add or change a Tessera backend endpoint/feature — the full loop of sqlc query → generate → handler → route → (optional) migration → vet/test. Use when implementing or modifying any backend API behavior, DB query, or schema in backend/.
---

# tessera-backend-feature

Цикл добавления/изменения бэкенд-фичи в Tessera. Стек: Go 1.25.9 + gin, pgx/v5,
**sqlc** (типобезопасный SQL), golang-migrate. Все команды — из `backend/` (или через `make`).

> Тулчейн: только `go1.25.9` (НЕ системный go), sqlc-бинарь — `~/go/bin/sqlc`.

## Слои и где что лежит

- `db/queries/*.sql` — **исходники sqlc** (пишем SQL руками с аннотациями `-- name: ... :one|:many|:exec`).
- `internal/db/*.sql.go` — **сгенерировано sqlc, руками НЕ трогать.**
- `handlers/*.go` — ресурсные хендлеры; общий слой `API` в `handlers/api.go`.
- `migrations/NNNN_name.{up,down}.sql` — схема (embed через `migrations/embed.go`).
  sqlc читает схему из `migrations/*.up.sql`.

## Шаги

### 1. (если меняется схема) — миграция
- Новый файл пары `migrations/NNNN_name.up.sql` + `.down.sql` (следующий номер по порядку).
- **up аддитивен** (ADD COLUMN nullable / новые таблицы) — безопасно для боевой БД.
  `down` — обратная операция.
- Применить на **обе** БД (боевую `tessera` И `tessera_test`):
  ```bash
  cd backend && go1.25.9 run ./cmd/migrate          # → боевая tessera (localhost:5432)
  DATABASE_URL="postgres://tessera:tessera@localhost:5432/tessera_test?sslmode=disable" \
    go1.25.9 run ./cmd/migrate                       # → tessera_test
  ```
  Проверка версии: `make migrate-version`. **Никогда не TRUNCATE/DROP/`down -v`** боевую БД.

### 2. SQL-запрос
- Добавить/поправить запрос в нужном `db/queries/<resource>.sql` с sqlc-аннотацией.
- Сгенерировать Go:
  ```bash
  cd backend && ~/go/bin/sqlc generate
  ```
  (Появятся/обновятся методы на `*db.Queries` в `internal/db/`.)

### 3. Handler + route
- Метод на `*API` в `handlers/<resource>.go`. Используй общие хелперы из `api.go`:
  - `h.requireMember(c, workspaceID)` — авторизация по членству в workspace (пишет 403, возвращает bool).
    Для вложенных ресурсов резолвь workspace через scope-запросы (`db/queries/scope.sql`).
  - `h.positionBetween(prev, next)` — float8-midpoint позиция (classic kanban-style) для порядка
    карточек/колонок; ручки move принимают `before_id`/`after_id`.
  - `h.broadcast(...)` — отправить domain-событие в WS-hub (scope = workspace id) для realtime.
  - `parseID` / `notFound` / `fail` — парсинг UUID и единообразные ошибки.
- Зарегистрировать маршрут там же, где регистрируются остальные (группа с `middleware.Auth`).
- Nullable-поля в PATCH — при необходимости частичных правок применяй tri-state
  (absent/null/value) через Go-Nullable; часть Update-ручек пока full-replace.

### 4. Проверка
```bash
cd backend && gofmt -w . && go1.25.9 vet ./... && go1.25.9 test ./...
make lint-backend     # golangci-lint — чинить, не глушить
```
Для проверки поведения вживую — скилл **tessera-e2e** (против `tessera_test`, порт 8092+).

### 5. Финал
- Bump + CHANGELOG + commit — скилл **tessera-ship** (`make bump-api BUMP=minor` для нового
  эндпоинта; `major` если ломаешь контракт — тогда подумай об Android-клиенте).

## Чеклист
- [ ] миграция (если схема), применена на tessera И tessera_test
- [ ] запрос в `db/queries/*.sql` + `sqlc generate` (не правил `internal/db/` руками)
- [ ] handler на `*API` + route под `middleware.Auth`, авторизация через `requireMember`
- [ ] WS-broadcast для realtime, где уместно
- [ ] `gofmt` + `vet` + `test` + `lint-backend` зелёные
- [ ] ship (bump/changelog/commit)
