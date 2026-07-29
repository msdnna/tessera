# CLAUDE.md — Tessera

Гид по проекту для Claude Code. Загружается в каждую сессию — держим его коротким
и точным. Глубина — в `ROADMAP.md`, в скиллах (`.claude/skills/`) и в auto-memory.
**Если что-то здесь расходится с кодом — правь этот файл в том же изменении.**

**Язык общения:** финальные summary, объяснения и прочую важную информацию для
пользователя пишем **на русском** (код, коммиты, идентификаторы — как принято).

## Что это

**Tessera** — таск-трекер экосистемы **msdnna** (живёт рядом с `budget-go`, зеркалит
его структуру и конвенции): задачи, заметки, напоминания.

**Killer-фича:** канбан-доска с **группировкой по тегам** (колонки = теги). Этого
нет у аналогов — не ломать.

**UX:** слева дерево проектов (sidebar), по центру канбан,
управление единицами в **модальных окнах**. Адаптив десктоп/мобильный веб,
светлая/тёмная темы, 7 акцентных схем (default — purple `#7c5cff`).

## Монорепо

```
backend/    Go + gin API (sqlc, pgx/v5, golang-migrate, WS-hub)
frontend/   Vue 3 SPA (Vite, Naive UI, Pinia, Vue Router)
android/    Kotlin + Compose клиент (online-first; см. android/ARCHITECTURE.md)
tools/      bump-version.sh, build-android-release.sh
design/     бренд-ассеты (design/tessera-brand-v2/ — канон, не перегенерировать)
deploy/     прод-деплой (по мере необходимости)
```

Три **независимо версионируемых** компонента, у каждого свой `VERSION` (semver):
`backend/VERSION` · `frontend/VERSION` · `android/VERSION`. Текущие версии —
`make version`. Версии **не синхронизируются** между компонентами.

## Стек по слоям

| Слой | Технологии |
|------|-----------|
| Backend | Go 1.25.9 + **gin**; PostgreSQL 17 + **JSONB**; `pgx/v5` + **sqlc** (типобезопасный SQL) + `golang-migrate` (embed) |
| Realtime | WebSocket fan-out hub (`internal/realtime`), workspace-scoped события — заложено с Фазы 0 |
| Auth | JWT 15m access + ротируемый refresh (SHA-256), bcrypt; первый юзер = admin; refresh-on-401 |
| Frontend | Vue 3 + Vite + **Naive UI** + Pinia + Vue Router; **Yarn 4** (`corepack yarn`, не плейн!) |
| Mobile | Android Kotlin + Jetpack Compose; Retrofit 3/OkHttp 5/Gson; online-first (без Room) |

### Backend-раскладка
- `handlers/` — ресурсные хендлеры; общий слой `API` в `handlers/api.go` с хелперами
  `requireMember` / `positionBetween` / `broadcast` / `parseID` / `notFound` / `fail`.
- `db/queries/*.sql` — исходники sqlc → генерят `internal/db/*.sql.go` (`~/go/bin/sqlc generate`).
  Схема для sqlc берётся из `migrations/*.up.sql`. **Не редактировать `internal/db/` руками.**
- `migrations/NNNN_name.{up,down}.sql` — golang-migrate, embed через `migrations/embed.go`.
- `cmd/migrate` — раннер миграций. `config/` fail-closed в проде.
- Порядок карточек/колонок — **float8 midpoint** (`positionBetween`), НЕ integer order.

### Frontend-раскладка
- `src/components/` (Kanban, TaskCard, TaskModal, Sidebar*, MarkdownEditor, …),
  `src/views/`, `src/stores/` (auth/theme/workspaces/boardView/notifications),
  `src/api/index.js` (coalesced refresh-on-401), `src/composables/`, `src/utils/`.
- Тема: `stores/theme.js` (Naive `themeOverrides` + CSS-vars `--t-*` на :root),
  токены в `styles/tokens.js`. Иконки — **ionicons5** (`@vicons/ionicons5`), не Material.
- localStorage-ключи: `tessera_token` / `tessera_refresh_token` / `tessera_user` /
  `tessera_dark` / `tessera_ws`. Не-критичный UX-стейт (сортировки/пресеты) → localStorage, не БД.

## Доменная модель

```
Workspace → ProjectGroup (вложенные, parent_id) → Project → Board → Column → Task
Task: subtasks (parent_id, дерево) · tags (M:N) · assignees (M:N) · priority · due_date · completed
Note · Reminder (доставка push — только через Android) · User/Membership (роли в workspace)
```
- ProjectGroup обязателен (группировка проектов внутри workspace).
- Done-колонка доски — по `boards.done_column_id` (не по имени). Резолв: явная колонка,
  иначе самая правая. Вход в done → `completed_at` + лог `completed`; выход → снятие + `reopened`.
- Новая доска засевается 4 колонками (`handlers/boards.go defaultColumns`).

## Конвенции (наследие budget-go — соблюдать во всей экосистеме)

- **Последние стабильные зависимости** всегда, без legacy-воркэраундов.
- **Версионирование per-component + единый CHANGELOG** ведём с первого дня.
  Бамп на каждое содержательное изменение компонента (patch/minor/major по semver).
  `docs:`/`chore:`/`refactor:` без поведенческих изменений — **без** бампа. См. скилл **tessera-ship**.
- **CHANGELOG:** корневой `CHANGELOG.md` (Keep a Changelog) с секциями `## backend` /
  `## frontend`; у Android — отдельный `android/CHANGELOG.md`. `bump-version.sh` правит
  только файл VERSION — changelog и коммит делаются вручную.
- **Pre-commit quality gate:** `make lint-<comp>` + `make test-<comp>` зелёные перед коммитом.
  Lint **чиним**, не глушим.
- **Conventional Commits**; коммитим локально после каждого проверенного фикса/фичи.
  **Пользователь сам пушит и сам ставит теги** — за него не пушить.
- **Git-identity:** `user.name=msdnna`, `user.email=extracker0mail@gmail.com`
  (НЕ harness-email из системного контекста). Ветка по умолчанию — `develop`. Remote пока нет.
- **JSON tri-state PATCH** (absent/null/value) для nullable-полей через Go-Nullable —
  применять, когда нужны частичные правки (часть Update-ручек пока full-replace).
- **Интеграции/нотиферы/боты — без жёстких связей с ядром.** Подобные фичи (GitLab и
  будущие) держим за чистым швом: своя логика в отдельном пакете/таблицах, а в ядро
  «протекаем» только через провайдеро-нейтральные механизмы (напр. колонка `source`
  user|gitlab на M:N, отдельные `*_links`-таблицы), не GitLab-специфику. Полноценный
  плагин-рантайм пока НЕ строим (производительность/оверинжиниринг для self-hosted), но
  код пишем так, чтобы при 2-й интеграции вынести провайдер за интерфейс малой кровью.

## Команды и тулчейн (эта среда — гочи проверены многократно)

```bash
make help            # все цели
make dev             # Postgres в Docker + backend на :8090 (host)
make migrate         # применить миграции (host → localhost:5432)
make up / down / logs
make lint / test     # агрегаты; есть lint-backend/-frontend/-android, test-*
make version         # версии всех компонентов
make bump-api  BUMP=minor   # + bump-web / bump-android
```

- **Go:** только `go1.25.9` (Makefile зовёт его через `GO ?= go1.25.9`), НЕ системный `go`.
  sqlc-бинарь: `~/go/bin/sqlc`.
- **Frontend:** только `corepack yarn ...` (проект на `yarn@4.x`; плейн yarn падает).
  `corepack yarn lint` / `build` / `test` (vitest) / `dev` (Vite на :5174, проксирует `/api` → :8090).
- **БД:** боевая `tessera` (контейнер `tessera-postgres`, том `tessera_postgres_data`,
  tessera/tessera, :5432) — **НИКОГДА не TRUNCATE/DROP/`down -v`**. E2e — только против
  `tessera_test`. После новой миграции прогонять migrate И на `tessera`, И на `tessera_test`
  (аддитивны → безопасно). См. скилл **tessera-e2e**.
- **Порты:** dev backend :8090 (budget держит :8080), Vite :5174, docker-фронт :8083.
  Для throwaway-e2e бери порт **8092+** (на :8090 может висеть зомби со старым кодом).
- **E2e-скрипты:** `bash <<'SCRIPT'` (дефолтный zsh ломается на uuid в арифметике);
  JSON парсить `python3`, не jq-бинарём.

## Дизайн-язык (важно — легко нарушить)

- **Accent-gradient:** каждый non-neutral элемент (кнопки, чипы, теги, аватары, точки
  приоритета, акцентные бордеры/текст, лого, активные табы) несёт **мягкий диагональный
  градиент того же оттенка** (тёмный bottom-left → светлый top-right, **центр = базовый
  цвет**), едва заметный. Нейтральные серые — плоские. Сделано на Android **и портировано
  на web** (web 0.42.x). Web-инструментарий: `src/utils/gradient.js` (`hueGrad` /
  `hueGradVert` / `softFill` / `tagPillBg` / `swatchBg`), CSS-vars `--t-accent-grad[-subtle|-vert]`
  + хелперы `.accent-grad-text` / `.grad-icon` в `styles/main.css`, и SVG-дефиниция
  `#t-accent-grad-svg` в `App.vue`. **Грабли и приёмы — `reference-web-accent-gradient`
  (читать перед правкой градиентов на web!).** Теги — градиент на ТЕКСТЕ + бордере, фон не
  трогаем; левая полоса карточки / верх колонки — НЕ `::before`-полоска (обрезается на
  скруглении), а прозрачный бордер + градиент на `border-box` (заходит на скругление).
- **Редактор описаний/комментариев — собственный `MarkdownEditor.vue`** (textarea +
  тулбар + Написать/Просмотр + @-mentions), хранит **Markdown**. **TipTap пробовали и
  отвергли** (тема/бандл) — не возвращать без явного запроса.
- **Бренд:** канон в `design/tessera-brand-v2/` (одиночная строчная «t» + плитка
  tessera, Fredoka, градиент `#6D5FE0→#7C6CFF→#9183FF`; монограмма «mt» из v1 больше
  НЕ используется). Сделано пользователем — **не перегенерировать**. Интегрировано во
  все клиенты (web/Android/desktop) в рамках ребренда 2026-07-28. Анимированный лоадер
  живёт в `design/tessera-brand-v2/loader/` (`loader-states.json` — общий источник
  чисел для web `src/utils/tesseraLoader.js` и Android `TesseraLoader.kt`, держать в
  синхроне). v1 `design/tessera-brand/` оставлен в репо как история.

## Скиллы проекта (`.claude/skills/`)

- **tessera-ship** — bump версии + CHANGELOG + conventional-commit (не пушить/не тегать).
- **tessera-backend-feature** — цикл бэкенд-фичи: sqlc-query → generate → handler → route → миграция → тест.
- **tessera-e2e** — безопасный backend e2e против `tessera_test` (гочи bash/python/порты).
- **tessera-android-release** — сборка и публикация подписанного релиза + self-update (`latest.json`).
- **tessera-task-workflow** — работа с задачами через MCP (`tessera-mcp`): взять в работу, уточняющие вопросы, приложить результаты/скриншоты, вернуть на проверку. Юзер назначает задачи на бота; агент возвращает автору в «На рассмотрении».

## Где что искать / как поддерживать

- **`ROADMAP.md`** — план фаз и near-term дорожная карта (источник истины для «что дальше»).
- **Auto-memory** (`~/.claude/projects/.../memory/`) — накопленный опыт по фазам и решениям;
  читается как фон, может устаревать — сверяй с кодом.
- **Фиксируй новое:** durable-факты проекта → сюда (CLAUDE.md) или в ROADMAP.md;
  повторяемые процедуры → в скилл; точечные наблюдения → в memory. Не плодить дубли.
