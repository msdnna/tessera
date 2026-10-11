# CLAUDE.md — Tessera

Гид по проекту для Claude Code. Загружается в каждую сессию — держим его **коротким**
(только то, что нужно почти в любой задаче). Глубина — в скиллах (`.claude/skills/`),
`ROADMAP.md` и auto-memory. **Если что-то здесь расходится с кодом — правь этот файл в
том же изменении.** Заметки по конкретной UI-области/фиче — в `tessera-v2-ui/areas.md`, не сюда.

**Язык общения:** финальные summary, объяснения и прочую важную информацию для
пользователя пишем **на русском** (код, коммиты, идентификаторы — как принято).

## Что это

**Tessera** — таск-трекер экосистемы **msdnna** (живёт рядом с `budget-go`, зеркалит
его структуру и конвенции): задачи, заметки, напоминания.

**Killer-фича:** канбан-доска с **группировкой по тегам** (колонки = теги). Этого
нет у аналогов — не ломать.

**UX:** слева дерево проектов (sidebar), по центру доска (6 видов), правка на месте и
поповерами (модель v2 — «меньше модалок»). Адаптив десктоп/телефон, светлая/тёмная темы,
7 акцентных схем (default — purple `#7c5cff`).

## Монорепо

```
backend/    Go + gin API (sqlc, pgx/v5, golang-migrate, WS-hub)
frontend/   Vue 3 SPA (Vite, Naive UI, Pinia, Vue Router)
desktop/    Tauri v2 оболочка web-клиента (Win/Linux; пилот Android — #2977)
android/    Kotlin + Compose — DEPRECATED (2026-10-12): новые фичи и паритет НЕ делать
mcp/        MCP-сервер Tessera (Go-модуль, PAT-токены)
tools/      bump-version.sh, сканеры, релиз-скрипты
design/     бренд (design/tessera-brand-v2/ — канон, не перегенерировать), макеты v2
```

Компоненты **независимо версионируются**, у каждого свой `VERSION` (semver), версии не
синхронизируются; текущие — `make version`.

## Стек

| Слой | Технологии |
|------|-----------|
| Backend | Go 1.26.9 + **gin**; PostgreSQL 17 + **JSONB**; `pgx/v5` + **sqlc** + `golang-migrate` (embed) |
| Realtime | WebSocket fan-out hub (`internal/realtime`), workspace-scoped события |
| Auth | JWT 15m access + ротируемый refresh (SHA-256), bcrypt; первый юзер = admin; refresh-on-401 |
| Frontend | Vue 3 + Vite + **Naive UI** + Pinia + Vue Router; **Yarn 4** (`corepack yarn`, не плейн!) |

### Backend-раскладка
- `handlers/` — ресурсные хендлеры; общий слой `API` в `handlers/api.go` (`requireMember` /
  `positionBetween` / `broadcast` / `broadcastAs` / `parseID` / `notFound` / `fail`).
- `db/queries/*.sql` → sqlc → `internal/db/*.sql.go` (`~/go/bin/sqlc generate`; схема — из
  `migrations/*.up.sql`). **`internal/db/` руками не редактировать.**
- `migrations/NNNN_name.{up,down}.sql` — embed через `migrations/embed.go`; `cmd/migrate` —
  раннер; `config/` fail-closed в проде.
- Порядок карточек/колонок — **float8 midpoint** (`positionBetween`), НЕ integer order.
- Цикл бэкенд-фичи — скилл **tessera-backend-feature**.

### Frontend-раскладка
- `src/components/` (kit/, task/, palette/, toast/, mobile/, …), `src/views/`,
  `src/stores/`, `src/api/index.js` (coalesced refresh-on-401), `src/composables/`, `src/utils/`,
  клавиши — каталог `src/shortcuts/`.
- Тема: `stores/theme.js` (Naive `themeOverrides` + CSS-vars `--t-*`), токены `styles/tokens.js`.
  Иконки — **ionicons5** (`@vicons/ionicons5`), не Material.
- localStorage: `tessera_token` / `tessera_refresh_token` / `tessera_user` / `tessera_dark` /
  `tessera_ws`. Не-критичный UX-стейт (сортировки/пресеты/свёрнутость) → localStorage, не БД.
- **Любая правка UI — сначала скилл `tessera-v2-ui`** (accent-gradient, kit-компоненты,
  тосты, телефон, клавиши; заметки по областям — его `areas.md`).

## Доменная модель

```
Workspace → ProjectGroup (вложенные, parent_id) → Project → Board → Column → Task
Task: subtasks (parent_id) · tags (M:N, per-project) · assignees (M:N) · priority · due/start
      · recurrence · milestone (этап) · estimate · completed
Note · Reminder («Входящие») · Document · Conference · User/Membership (owner|admin|member)
```
- ProjectGroup обязателен. Done-колонка — `boards.done_column_id` (иначе самая правая); вход
  в done → `completed_at` + лог `completed`, выход → снятие + `reopened`.
- Новая доска засевается колонками (`handlers/boards.go defaultColumns`, шаблоны —
  `handlers/board_templates.go`).

## Инварианты (нарушение = баг; подробности — `tessera-v2-ui/areas.md`)

- **Корзина/удаление:** новый запрос по `tasks`/`documents` фильтрует `deleted_at IS NULL`
  (храповик `handlers/trash_ratchet_test.go`), по задачам нескольких досок — ещё
  `b.deleted_at IS NULL`. Архивный проект (`projects.archived_at`) — запись → 409.
- **Заметки:** запрос по `notes` фильтрует `visibility = 'workspace' OR author_id = viewer`
  (и в сокете).
- **Правка задачи:** любой путь обязан нести `recurrence` (иначе стирается); смена
  `milestone_id` обязана журналить (`journalMilestoneActor`).
- **События:** user-initiated broadcast — `broadcastAs` (несёт `actor`; на нём тосты и
  чек-лист онбординга).
- **Уведомления:** новый вид — событие в `handlers/notification_v2.go` + ключи в
  `notify/locale.go` и `locales/*/notifications.json` (паритет — храповик-тест).
- **Web-сообщения:** только `useMessage` из `@/composables/useMessage`, не из naive-ui.
- **Новые мелкие модалки не заводить** (ярусы v2: на месте → поповер → крупное окно).
- **Интеграции/боты — без жёстких связей с ядром:** своя логика в отдельном пакете/таблицах,
  в ядро — только провайдеро-нейтрально (колонка `source` user|gitlab, `*_links`-таблицы с
  `provider`+`source_id`, пример — `task_code_links`). Плагин-рантайм не строим, но 2-я
  интеграция должна выноситься за интерфейс малой кровью.
- **Редактор** — собственный `MarkdownEditor.vue`, хранит Markdown. TipTap отвергнут.

## Конвенции (наследие budget-go)

- **Последние стабильные зависимости**, без legacy-воркэраундов.
- **CHANGELOG через фрагменты:** фича-ветка НЕ трогает `CHANGELOG.md`/`VERSION`, кладёт
  `changelog.d/<component>/<task#>.md`; бамп и сборка — на `develop` (`make changelog-release`).
  `docs:`/`chore:`/`refactor:` без поведения — без бампа. Скилл **tessera-ship**.
- **Ветки:** одна на **родительскую** задачу (`feat/<родитель#>-slug`), подзадачи — коммитами.
  Волну фич сливаем в `integration/*` → полные гейты → develop.
- **Миграции последовательны:** номер `NNNN_` занимать перед самым merge; коллизию чинить
  renumber'ом более позднего (`backend/e2e/migrate_test.go` падает на разрывах).
- **Quality gate:** `make lint-<comp>` + `make test-<comp>` зелёные перед коммитом. Lint **чиним**.
- **Conventional Commits**, коммит после каждого проверенного шага. **Пользователь сам пушит
  и ставит теги.** Git-identity: `msdnna` / `extracker0mail@gmail.com`. Ветка по умолчанию —
  `develop`.
- **JSON tri-state PATCH** (absent/null/value) через Go-Nullable / `optField` для nullable-полей.
- **Задачи — через MCP** (скилл **tessera-task-workflow**); несколько задач / агенты —
  скилл **tessera-orchestrate**.

## Команды и тулчейн (гочи этой среды)

```bash
make help / dev / migrate / up / down / logs / version
make lint / test                 # агрегаты; lint-backend/-frontend, test-*
make test-e2e-backend            # backend black-box, своя одноразовая БД
make e2e-backend-up && make test-e2e-frontend && make e2e-backend-down   # web Playwright
make scan-backend                # + scan-frontend/-desktop/-secrets
make bump-api BUMP=minor         # + bump-web
```

- **Go:** только `go1.26.9` (Makefile: `GO ?= go1.26.9`), не системный `go`. sqlc: `~/go/bin/sqlc`.
- **Frontend:** только `corepack yarn …` (`lint` / `build` / `test` (vitest) / `dev` — Vite :5174,
  `/api` → :8090).
- **БД:** боевая `tessera` (контейнер `tessera-postgres`, tessera/tessera, :5432) — **НИКОГДА
  не TRUNCATE/DROP/`down -v`**. Ручной e2e — только `tessera_test`; после новой миграции —
  migrate на обе. Детали e2e (порты, Playwright, браузеры) — скилл **tessera-e2e**.
- **Порты:** dev :8090 (budget — :8080), Vite :5174, docker-фронт :8083, throwaway e2e — **8092+**.
- **Скрипты:** `bash <<'SCRIPT'` (zsh ломается на uuid в арифметике); JSON — `python3`, не jq.
- **Сканы безопасности** — не часть lint/test (сеть, минуты); гонять на ветке слияния.
  Храповик `tools/security-baseline.json` (может только сокращаться; после починки —
  `make scan-baseline-update`). См. `docs/security-scanning.md`.

## Бренд и дизайн

- **Accent-gradient** на каждом non-neutral элементе (мягкий диагональный градиент того же
  оттенка, центр = базовый цвет); нейтральные — плоские. Web-инструментарий и грабли — скилл
  `tessera-v2-ui` + memory `reference-web-accent-gradient`.
- **Бренд** — `design/tessera-brand-v2/` (строчная «t» + плитка, Fredoka, `#6D5FE0→#7C6CFF→#9183FF`),
  сделан пользователем — **не перегенерировать**. Лоадер: `loader/loader-states.json` — общий
  источник для `src/utils/tesseraLoader.js`.

## Скиллы проекта (`.claude/skills/`)

- **tessera-v2-ui** — правила веб-UI v2 + заметки по областям (`areas.md`). Перед правкой UI.
- **tessera-orchestrate** — волна задач: разбор, группировка по общему контексту, агенты
  (≤2), worktree, интеграционная ветка, гейты, закрытие.
- **tessera-backend-feature** — sqlc-query → generate → handler → route → миграция → тест.
- **tessera-e2e** — ручной backend e2e против `tessera_test` + автоматические наборы.
- **tessera-ship** — changelog-фрагмент + conventional-commit; релиз на develop.
- **tessera-task-workflow** — работа с задачами через MCP (`tessera-mcp`).
- **tessera-sentry** — триаж ошибок через self-hosted Sentry (если жив).
- **tessera-android-release** — релиз Kotlin-APK (deprecated-клиент, только по запросу).
