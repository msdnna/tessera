<p align="right"><b>Русский</b> · <a href="CONTRIBUTING.en.md">English</a></p>

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

## Документация и локализация

Документация в репозитории **двуязычная**, но у неё один канон — **русский**.

- **База — русский:** каноничный файл всегда `<имя>.md` на русском (именно его
  GitHub рендерит как homepage репозитория или каталога).
- **Английский — companion:** `<имя>.en.md` рядом, слово-в-слово перевод базы.
  Суффикс `.ru.md` **не используем** — русский и есть база.
- **Языковой свитчер** — короткая строка-навигация в самом верху каждого файла пары:
  `**Русский** · [English](<имя>.en.md)` в базе и `[Русский](<имя>.md) · **English**`
  в `.en.md`.
- **Двуязычны только «фасадные» документы** (README, CONTRIBUTING и компонентные
  README: `deploy/`, `mcp/`, `desktop/`, `android/ARCHITECTURE.md`). Внутренние
  документы (`CLAUDE.md`, `ROADMAP.md`, `CHANGELOG.md`, ADR, `.claude/skills/`,
  `changelog.d/`) ведём **только на русском** — перевод раздувает объём без пользы.
  Правя фасадный документ, синхронизируй обе версии в том же изменении.
- **Справочный центр** (`docs/help/`) следует той же схеме — русский `<slug>.md` +
  английский `<slug>.en.md` (+ платформенные `.android.md`), а индекс
  пересобирается `make help-index`. Решение зафиксировано в
  [`docs/adr/0001-help-center.md`](docs/adr/0001-help-center.md) и
  [`docs/adr/0002-doc-localization.md`](docs/adr/0002-doc-localization.md).

## MCP и наблюдаемость

- **MCP-сервер** (`mcp/`) — тонкий REST-клиент Tessera для агентной разработки;
  входит в `make lint` / `make test`. Локально: сборка `make build-mcp`, токен
  `go run ./cmd/token`. Детали — в [`mcp/README.md`](mcp/README.md).
- **Sentry** — опциональный self-hosted трекинг ошибок (выключен без DSN). В dev
  ошибки смотрим по логам; при поднятом Sentry — через него (см. skill
  `tessera-sentry`).

## Pull requests

1. Форкни репозиторий и создай ветку от `develop`.
2. Убедись, что `make lint` и `make test` зелёные (CI прогонит их же + покрытие).
3. Опиши, что и зачем поменялось; приложи скриншоты для UI-изменений.
4. Один PR — одно логическое изменение.

## Лицензия

Отправляя вклад, вы соглашаетесь, что он лицензируется на условиях
[Apache License 2.0](LICENSE), под которой распространяется проект.
