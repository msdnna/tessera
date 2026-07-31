# Contributing to Tessera

Спасибо за интерес к Tessera. Ниже — как поднять окружение и какие конвенции
соблюдаются в проекте. Более глубокий гид по архитектуре — в `CLAUDE.md` и
`ROADMAP.md`.

## Структура

Монорепо из независимо версионируемых компонентов (у каждого свой `VERSION`):

| Каталог     | Компонент                                                    |
|-------------|-------------------------------------------------------------|
| `backend/`  | Go 1.25 + gin + PostgreSQL 17 (sqlc, pgx/v5, golang-migrate) |
| `frontend/` | Vue 3 + Vite + Naive UI + Pinia (Yarn 4 через Corepack)      |
| `android/`  | Kotlin + Jetpack Compose (online-first)                     |
| `desktop/`  | Tauri v2 (Rust-оболочка вокруг того же Vue-фронтенда)        |
| `mcp/`      | Go — MCP-сервер для агентов                                  |

## Требования

- **Go 1.25.x** (в Makefile — переменная `GO`, по умолчанию `go1.25.9`; в CI — `go`).
- **Node 22+** и **Corepack** (Yarn 4 — только `corepack yarn ...`, не плейн `yarn`).
- **Docker** + Docker Compose (Postgres для dev/тестов).
- **JDK 21** + Android SDK (compileSdk 37) — для `android/`.
- **Rust stable** + системные Tauri-зависимости — для `desktop/`
  (`libwebkit2gtk-4.1-dev libgtk-3-dev libayatana-appindicator3-dev librsvg2-dev patchelf`).

## Быстрый старт

```bash
cp .env.example .env          # заполнить JWT_SECRET и пр.
make dev                      # Postgres в Docker + backend на :8090
cd frontend && corepack yarn install && corepack yarn dev   # Vite :5174

# Тесты и линтеры
make lint                     # backend + frontend + mcp (есть lint-android / lint-desktop)
make test                     # backend + frontend + mcp
make test-backend-cover       # покрытие backend (нужна БД tessera_test)
make coverage-report          # сводный HTML-отчёт покрытия по всем компонентам
```

БД для e2e/интеграционных тестов — **только `tessera_test`**, боевую `tessera`
не трогаем (её TRUNCATE/DROP запрещены).

## Конвенции

- **Свежие стабильные зависимости**, без legacy-воркэраундов.
- **Conventional Commits** (`feat:`, `fix:`, `docs:`, `chore:`, `test:`, `refactor:`, …).
- **Версионирование per-component**: каждое содержательное изменение компонента
  бампает его `VERSION` (semver) и добавляет запись в `CHANGELOG.md` (корневой,
  секции `## backend` / `## frontend`; Android — `android/CHANGELOG.md`).
  `docs:`/`chore:`/`refactor:` без поведенческих изменений — **без** бампа.
- **Pre-commit quality gate**: `make lint-<comp>` + `make test-<comp>` зелёные
  перед коммитом. Проблемы линтеров чиним, а не глушим.
- **Backend**: SQL — через sqlc (`db/queries/*.sql` → `~/go/bin/sqlc generate`,
  `internal/db/` руками не править); миграции — `migrations/NNNN_name.{up,down}.sql`.
- **Frontend**: тема/иконки — Naive UI + ionicons5; редактор описаний — собственный
  `MarkdownEditor.vue` (хранит Markdown), не TipTap.
- **Интеграции/нотиферы** держим слабо связанными с ядром (провайдеро-нейтральные швы).

## Pull requests

1. Форкни репозиторий и создай ветку от `develop`.
2. Убедись, что `make lint` и `make test` зелёные (CI прогонит их же + покрытие).
3. Опиши, что и зачем поменялось; приложи скриншоты для UI-изменений.
4. Один PR — одно логическое изменение.

## Лицензия

Отправляя вклад, вы соглашаетесь, что он лицензируется на условиях
[Apache License 2.0](LICENSE), под которой распространяется проект.
