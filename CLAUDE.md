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
| Backend | Go 1.26.9 + **gin**; PostgreSQL 17 + **JSONB**; `pgx/v5` + **sqlc** (типобезопасный SQL) + `golang-migrate` (embed) |
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
- **Подписки на задачу (#2963, миграция 0083):** автор, исполнители и комментаторы подписаны
  неявно (список вычисляет `ListTaskSubscribers`, только участники пространства),
  `task_subscriptions` хранит лишь явный выбор (true — подписался, false — отписался).
  Новости задачи — `notifyTaskParticipants` → подписчикам; адресные уведомления (назначили,
  упомянули, удалили) от подписки не зависят.
- Адрес проекта (`projects.slug`) уникален на инстанс; смена оставляет старый алиасом на 30 дней
  (`project_slug_aliases`, миграция 0085, #3012). Проект по адресу — только через `projectBySlug`
  (живой slug важнее алиаса); подменить адрес в открытом URL — `utils/projectRoute.withProjectSlug`
  (`router.replace({ ...route, params })` vue-router молча игнорирует). 409 занятого адреса несёт
  `suggestion` и `taken_by` (`utils/slugTaken`, `kit/SlugSuggest`); счётчики для подтверждений
  удаления — `GET /projects|workspaces/:id/counts` (`utils/deleteCounts`).

## Конвенции (наследие budget-go — соблюдать во всей экосистеме)

- **Последние стабильные зависимости** всегда, без legacy-воркэраундов.
- **Версионирование per-component + единый CHANGELOG** ведём с первого дня.
  Бамп на каждое содержательное изменение компонента (patch/minor/major по semver).
  `docs:`/`chore:`/`refactor:` без поведенческих изменений — **без** бампа. См. скилл **tessera-ship**.
- **CHANGELOG через фрагменты (важно — против merge-конфликтов).** Фича-ветка НЕ трогает
  `CHANGELOG.md`/`VERSION`, а кладёт запись фрагментом `changelog.d/<component>/<task#>.md`
  (уникальное имя на задачу → не конфликтует при merge). **Бамп версий и сборку changelog
  делаем на `develop`:** `make changelog-release` поднимает `VERSION` (уровень = максимум
  по фрагментам) и вставляет запись в нужную секцию. Корневой `CHANGELOG.md` (Keep a Changelog,
  секции `## backend`/`## frontend`), Android — `android/CHANGELOG.md`. Формат — `changelog.d/README.md`.
- **Модель веток:** одна ветка на **родительскую** задачу (`feat/<родитель#>-slug`); подзадачи —
  отдельными conventional-коммитами внутри неё (одна поверхность на ревью, погранульность
  по коммитам). Не плодить ветку на подзадачу.
- **Миграции последовательны** (`golang-migrate` линеен): номер `NNNN_` занимать **перед
  самым merge**; миграционные задачи разбирать **после** безмиграционных, чтобы не плодить
  параллельные номера. Коллизию номеров чинить renumber'ом более позднего.
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
  Пример шва — `task_code_links` (#2964, миграция 0084): MR и пайплайны в «Разработке»
  задачи, `provider`+`source_id` без FK в GitLab-таблицы (чистит сам провайдер). GitLab-часть —
  `internal/gitlab/mergerequests.go` + `handlers/gitlab_mr.go`; вебхуки `merge_request`/`pipeline`
  применяются сразу из payload (issue/note — по-прежнему только через инкрементальный пул).

## Команды и тулчейн (эта среда — гочи проверены многократно)

```bash
make help            # все цели
make dev             # Postgres в Docker + backend на :8090 (host)
make migrate         # применить миграции (host → localhost:5432)
make up / down / logs
make lint / test     # агрегаты; есть lint-backend/-frontend/-android, test-*
make test-e2e-backend       # чёрный ящик: настоящие бинари + одноразовая БД (build-tag e2e)
make test-e2e-backend-docker # то же + сборка и старт образа (минуты)
make version         # версии всех компонентов
make bump-api  BUMP=minor   # + bump-web / bump-android
make scan-backend    # SAST/SCA компонента (есть scan-frontend/-android/-desktop/-secrets)
```

- **Сканы безопасности — не часть `lint`/`test`** (ходят в сеть, идут минутами), гонять
  перед сдачей по затронутому компоненту. Гейт — **храповик**: находки заморожены в
  `tools/security-baseline.json`, список может сокращаться, но не расти; после починки —
  `make scan-baseline-update` и диff в тот же коммит. Подробности —
  `docs/security-scanning.md`.

- **Go:** только `go1.26.9` (Makefile зовёт его через `GO ?= go1.26.9`), НЕ системный `go`.
  sqlc-бинарь: `~/go/bin/sqlc`.
- **Frontend:** только `corepack yarn ...` (проект на `yarn@4.x`; плейн yarn падает).
  `corepack yarn lint` / `build` / `test` (vitest) / `dev` (Vite на :5174, проксирует `/api` → :8090).
- **БД:** боевая `tessera` (контейнер `tessera-postgres`, том `tessera_postgres_data`,
  tessera/tessera, :5432) — **НИКОГДА не TRUNCATE/DROP/`down -v`**. E2e — только против
  `tessera_test`. После новой миграции прогонять migrate И на `tessera`, И на `tessera_test`
  (аддитивны → безопасно). См. скилл **tessera-e2e**. Автоматический e2e
  (`backend/e2e/`, `make test-e2e-backend`) создаёт **свою** одноразовую БД
  `tessera_e2e_<runId>` рядом и дропает её в конце — он делает `migrate down`,
  которому нельзя на общей `tessera_test`.
- **Порты:** dev backend :8090 (budget держит :8080), Vite :5174, docker-фронт :8083.
  Для throwaway-e2e бери порт **8092+** (на :8090 может висеть зомби со старым кодом).
- **E2e-скрипты:** `bash <<'SCRIPT'` (дефолтный zsh ломается на uuid в арифметике);
  JSON парсить `python3`, не jq-бинарём.
- **Web e2e (Playwright, #2710):** `make e2e-backend-up` (бэкенд :8092 против `tessera_test`)
  → `make test-e2e-frontend` (сборка + прогон против `vite preview` :4174) →
  `make e2e-backend-down`. Спеки в `frontend/e2e/`, **не** в `frontend/tests/` (там vitest).
  `@playwright/test` пинуется **ровно 1.62.0** под уже скачанный chromium в
  `~/.cache/ms-playwright`; `playwright install` не запускать — его node-клиент режется
  средой (как yarn на npm-реестре). Сам CDN доступен: `curl -L cdn.playwright.dev/dbazure/
  download/playwright/builds/<браузер>/<rev>/<браузер>-ubuntu-24.04.zip` качается, ревизию
  брать из `node_modules/playwright-core/browsers.json`, распаковывать в
  `~/.cache/ms-playwright/<браузер>-<rev>/` + пустые `INSTALLATION_COMPLETE`/
  `DEPENDENCIES_VALIDATED`. Так поставлен **firefox** (rev 1538) для #2863.
  Гоча headless Firefox: он **не рисует скроллбары вообще** — `scrollbar-width` там всегда
  computed `none`, независимо от CSS. Скроллбар-баги в FF проверяются по computed
  `scrollbar-color`, кадром — нет (дисплея/Xvfb на машине тоже нет).
- **Android e2e (#2711) — два яруса, харнесс общий** (`android/app/src/e2eShared`, он же
  в `src/test` и в `src/androidTest`). Ярус A: Compose UI Test под **Robolectric** против
  того же бэкенда на :8092 — `make e2e-backend-up` → `make test-e2e-android`. Спеки в
  `app/src/test/java/.../e2e/`, из `make test-android` отфильтрованы (`-Pe2e`). Ярус B:
  инструментальные smoke-тесты (`app/src/androidTest/.../smoke/`) — `make test-android-instrumented`,
  нужен девайс/эмулятор; без него компиляцию проверяет `make build-android-instrumented`.
  Адрес бэкенда `E2eBackend` выбирает сам: Robolectric → `localhost`, устройство → `10.0.2.2`.
  Якоря — `Modifier.testTag` из реестра `ui/TestTags.kt`, **не** тексты (локализованы и
  переписываются). Новый Kotlin-сорс-сет надо руками добавлять в `ktlintSources`/`detekt.source`
  **и** регистрировать через `sourceSets { ….kotlin.srcDir(…) }` — `java.srcDirs` Kotlin
  больше не компилирует, каталог молча выпадает из сборки.

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
- **Модель взаимодействия v2 (#2969) — меньше модалок.** Ярусы: 0 — правка на месте
  (`kit/InlineEdit`), 1 — поповер-форма у якоря (`kit/PopoverForm` + `FormField`, на
  телефоне — `BottomSheet`), 1′ — та же форма в ⌘K (`palette/usePaletteForm`), 2 — крупное
  окно (задача, GitLab, участники, задания, команды, конфликты, справка, шпаргалка, «Проект»).
  Меню строк — `kit/ActionMenu` (клавиша у каждого пункта из каталога `shortcuts/`);
  обратимое удаление — сразу + `stores/undo` (тост 7 с, ⌘Z), необратимое — двухшаговый
  пункт или ввод имени. **Новые мелкие модалки не заводить.** Функции из макетов, которых
  ещё нет, — видимые неактивные заглушки «Скоро · #N» через реестр `utils/featureStubs.js`.
  Подменю — `children` у пункта ActionMenu (→ ← ↵; на телефоне — второй экран листа). Слой
  внутри NModal (меню, поповер-форма, лист) — через `kit/AnchorLayer` (+ `inline`) или
  `in-place`: телепорт в body фокус-ловушка модалки обесточивает (клавиатура и ввод мертвы).
  Двухшаговая кнопка вне меню — `kit/ArmButton`. Меню задачи во всех видах — `useTaskMenu` +
  `task/TaskActionMenu`, меню «⋯ Действия доски» — `useBoardActions` (и на телефоне).
- **Ревью пилотов (#3109):** «Входящие» — только кнопка в шапке + G I (строки в сайдбаре
  нет); справка — «?» рейла (Справочный центр · Обучение) и F1 на статье текущего экрана
  (`utils/helpContext`), `HelpHint` в шапках не ставить. Крошки доски — двери (проект →
  окно «Проект», доска → список досок). Уведомления «про меня» на бэкенде: `deleted`,
  `unassigned`, `completed`, `reopened` (#3114).
- **Тосты (#3117, пакет I, миграция 0082):** один каркас `toast/Toast`, две полосы
  `toast/ToastLanes` — низ: ответы на мои действия + undo (`stores/undo`, правила пакета A);
  верх справа под колокольчиком: «про меня» из сокета `notification` (те же слова, что строка
  ленты), напоминания, звонки, активность доски (`toasts.activity()` из KanbanBoard, только
  чужие, без дубля «про меня»), система, сводка. Правила — `utils/toastModel.js`, стор —
  `stores/toasts.js`; на телефоне одна полоса с очередью. **Сообщения — только `useMessage`
  из `@/composables/useMessage`, не из naive-ui** (`<n-message-provider>` нет; опции
  `{ retry, line, action, duration, class }`); успех тостом — только если результата не видно.
  Новое событие задачи, дающее тост, обязано нести `actor`. Настройки тостов и «Не беспокоить» —
  `notification_prefs` (`toasts` jsonb, `toasts_enabled`, `quiet_toasts`, `dnd_until`; поля
  PUT необязательны) + `PUT /notification-prefs/dnd`. Слой поверх карточки (пульс) — отдельный
  fixed-элемент, не класс на `.card`. e2e-якоря: `toast` + `data-kind`, `toast-lane-top|bottom|phone`.
- **Свойства задачи и редактор (#3118, пакет J):** все свойства — `kit/PropPicker` (поповер /
  лист на телефоне / `shell="palette"`), строки полей — один модуль
  `components/task/props/propItems.js` для модалки, карточки, «Списка» и палитры (новое поле —
  построитель там, не своё меню). Хосты: модалка — `task/TaskPropPickers` (`openField`,
  `fieldOpen`), карточка/Список — `task/props/TaskFieldPopover`; placement `below-or-side`
  (слева от `[data-pop-side]`). Клавиши модалки: S A P D L, O оценка, M этап, U родитель,
  ⇧L связь (форма остаётся открытой), повтор клавиши — следующее поле. Срок —
  `due/DuePicker` (DueEditor удалён; модель `composables/useDueModel`, чистая часть
  `utils/dueModel`, разбор `quickParse.parseDue`): шлёт `{ due_date, start_date, recurrence }`
  целиком — **любой путь правки задачи обязан нести `recurrence`**; date-only — UTC-полночь,
  время — пояс профиля, `recurrence.tz` следует за этим. Редактор — `MarkdownEditor`
  `v2` | `composer` («Aa» помнит `tessera_composer_fmt`) | `full`; подсветка — прозрачная
  textarea над `utils/mdHighlight` (в пропорциональном поле — только цвет). @ # / —
  `utils/edSuggest` + `editor/EdSuggest` (на месте, не телепорт), клавиши поля — группа
  `editor.*` (по физической клавише; ссылка ⌘⇧K — ⌘K у палитры). Описание — I (⇧E скрытый
  алиас), «Отменить правки?» по Esc. Пузырь над текстом для чтения — `editor/EdReadBubble`
  («Цитировать в ответ» через `utils/quoteBus`). Полноэкранный — `editor/FullscreenEditor` на
  LargeWindow (⌘1–3, Esc свернуть с сохранением). Черновик комментария — на задачу
  (`tessera_comment_draft:<taskId>`).
- **Телефон (≤768px, #2976 — пакет G, модель A):** своя оболочка в `AppLayout` — шапка
  `mobile/MobileTopbar` (на доске: проект/доска → дровер; иначе «‹ Ещё»), страница и нижняя
  навигация `mobile/MobileTabBar` (Мои задачи · Доска · Входящие · Поиск · Ещё). Дерево —
  `n-drawer` (тап по названию доски, повторный тап «Доска», свайп от края `useEdgeSwipe`),
  гамбургера нет. «Ещё» — `/more`, лента уведомлений — `/notifications` (`NotifFeed`; на
  десктопе оба адреса уводят в настройки / поповер). CSS-переменные `--safe-top/-bottom`,
  `--mnav-h` (высота навигации, 0 без неё), `--m-head-h`, `--app-vh`, `--kb-h` (клавиатура,
  `useVisualViewport`) — всё, что стоит над навигацией или клавиатурой, считается от них. На
  таче ActionMenu/SnoozeMenu — лист снизу, `BottomSheet` — снапы и `page`, `kit/SwipeRow` —
  свайп строк, kbd скрыты глобально (`kbd.kbd`, `.sk`, `.kbd-hint`). Закрывая оверлей ради
  перехода — `utils/overlayNav.unwindOverlay`, иначе его `history.back()` отменит push.
- **Мягкое удаление досок и колонок (миграция 0078):** `deleted_at` + `POST …/restore`,
  воркер `trash_purge` удаляет окончательно через сутки. Удалённая доска невидима всем
  читателям — новый запрос по задачам нескольких досок обязан фильтровать
  `b.deleted_at IS NULL`; колонка при удалении отдаёт задачи соседу (`column_deletion_moves`).
- **Корзина задач/документов и архив проектов (#3001, миграция 0086):** `tasks.deleted_at` /
  `documents.deleted_at` — `DELETE` кладёт поддерево в корзину одной отметкой, `POST …/restore`
  возвращает (НЕ путать с `PATCH /tasks/:id/restore` архива), `DELETE …/purge` — навсегда,
  `trash_purge` — через 30 дней. Задача в корзине невидима всем: **новый запрос по `tasks`/
  `documents` обязан фильтровать `deleted_at IS NULL`** (храповик `handlers/trash_ratchet_test.go`;
  намеренно слепые — в его списке с причиной); `GetTask`/`GetDocument` уже фильтруют, сырьё —
  `…Any`. `projects.archived_at`: вне дерева/поиска/«Моих задач»/сканеров, запись → 409
  `{archived}` (гейт в `loadTask` для не-GET + `requireProjectWritable`), web `readOnly` доски.
- **«Вид доски» и представления (#2970, миграция 0079):** настройки доски — `kit/DockPanel`
  (pop) справа внутри доски во всех 6 видах (⇧V), на телефоне — нижний лист. Представления:
  `board_views.scope` user|board (у общих `user_id` NULL, автор в `created_by`, правят
  owner/admin пространства), `is_default` (одна звезда на список, личная > общей, применяется
  раз за сессию браузера), `position`; `PATCH /views/:id`. Автосохранения нет — «изменено» +
  Обновить ⌘S / Сбросить ⌘⌫; логика — `composables/useBoardViews.js`, чистая часть —
  `utils/boardViews.js`. Конфиг представления = состояние доски (`currentViewConfig`), поиск и
  этап в него не входят. `boards.default_layout` — вид новой доски.
- **Крупные окна (#2975):** общий каркас — `kit/LargeWindow` (шапка 56px с крошками и `#aside`,
  колонка разделов `items` со счётчиками/заглушками `stub`/дверями `out`, `escape()` раздела,
  цифры и Esc — общие `win.section`/`win.close` каталога, Back и возврат фокуса). На нём окно
  «Проект», GitLab, конфликты, фоновые задания, участники, команды — новое крупное окно строить
  только на нём. Карточка окна снимается сразу (`v-if`), без затухания: иначе клавиша сразу после
  закрытия глоталась ещё видимой модалкой. `useOverlayBack` метит запись истории своим id — окна
  можно ставить друг на друга (конфликты поверх GitLab). GitLab: состояние формы —
  `gitlab/useGitlabForm` (provide/inject в разделы), привязка — один PUT всей формы, поэтому
  явное сохранение панелью «Изменения не сохранены ⌘S» (как у конфликтов «Применить ⌘↵»);
  выбор версии в конфликтах — по полю, на сервер уходит theirs / ours / manual со значениями.
- **Окно «Проект», этапы и теги (#2971, миграция 0080):** настройки проекта — крупное окно
  `components/project/ProjectWindow.vue` (стор `projectWindow.open(id, tab, {filter})`, ⌘,)
  с вкладками Общее · Теги · Этапы · Оценка; TagManager/MilestoneManager удалены. Таблица тегов
  (`utils/tagTable.js`): имя префикса — точечный `PATCH /projects/:id/tag-prefixes`, слияние
  `POST …/tags/merge`, массовое удаление `POST …/tags/delete` + undo `POST …/tags/restore`
  (теги возвращаются под прежними id), `tags.source` user|gitlab, замок — префиксы правил GitLab
  проекта. Этап: два хранимых состояния + `closed_at`, «Запланирован» = активный со стартом
  впереди (`utils/milestones.stageState`). Страница этапа `/project/<slug>/stage/<slug>` —
  `views/StageView.vue` на `GET /milestones/:id/metrics`; объём/burn-up считаются по журналу
  `milestone`/`milestone_removed` (несут `milestone_id`, при переносе `from_id`), поэтому
  **любой новый путь смены `milestone_id` обязан журналить** (`journalMilestoneActor`). На доске
  — селектор этапа в подбаре (⇧E) и полоса этапа (⇧H), выбор и свёрнутость — localStorage на
  доску. GitLab-этап (#3010, миграция 0087) правится, только пока привязка пишет в GitLab
  (`gl_writable` = binding + write-back, иначе `409 gitlab_readonly`): `PATCH /milestones/:id`
  пишет в веху синхронно и отвечает `gl_sync`; сверка трёхсторонняя по
  `gitlab_milestone_links.gl_snapshot` и в push, и в pull (`gitlab_milestone_sync.go`) —
  отказ GitLab = `push_error` («не синхронизировано · повторить», синк повторяет сам),
  правка с обеих сторон = `conflict` в окне конфликтов (`?include=milestones`, entity
  `milestone`), ответ записи на web — через `useStageSync`. Навигация из окна-оверлея — после отмотки его
  history-записи (`ProjectWindow.navigate`), иначе `back()` из `useOverlayBack` отменит push.
- **«Входящие» (#2972, миграция 0081):** заметки и напоминания — одна лента в `kit/DockPanel`
  (variant sidebar) справа под шапкой (`components/inbox/*`, стор `stores/inbox` — лента
  пространства из `GET /workspaces/:id/inbox`, сокет, часы; чистая часть — `utils/inbox.js`).
  Захват — `inbox/CapturePop` у FAB (N M / N R, разбор `parseCapture` в `utils/quickParse`), в
  модалке задачи — его inline-копия (фокус-ловушка NModal). Две таблицы, не одна сущность:
  `notes.visibility` private|workspace (личная видна только автору **во всех чтениях и в сокете**
  — новый запрос по заметкам обязан фильтровать `visibility = 'workspace' OR author_id = viewer`),
  `notes.task_id`; у `reminders` — `body`, `done_at` (идёт за `done`), `snoozed_from`,
  `workspace_id` (NULL — видно в любом пространстве). Разделов `/notes` `/reminders` больше нет
  (редирект в панель); пилл-колокольчик на карточке — из стора, без запроса на карточку.
- **Виды доски v2 (#2973, #3009):** Список (`BoardListView` + `list/ListRow`, геометрия и курсор —
  `utils/listView.js`; строки фиксированной высоты → оконная отрисовка, держит 500+ задач),
  Календарь (`BoardCalendarView` + `calendar/CalendarPill`, сетки и перенос срока —
  `utils/calendarView.js`: date-only остаётся UTC-полночью, время — в поясе профиля), Матрица
  (ручной квадрант — поле задачи `eisenhower_quadrant`, навигация — `utils/matrixNav.js`).
  Ширины колонок списка, режим календаря и «Шкала» таймлайна/Ганта (`День/Неделя/Месяц/Квартал`
  + переключатели) — ключи `list`/`calendar`/`chart` конфига представления (`utils/boardViews.js`).
  Клавиши видов — свои id в каталоге (`list.*`, `cal.*`, `matrix.*`) плюс `card.*` с `when` на
  свой вид; действия над задачами — общие функции KanbanBoard (`listActions`/`calendarActions`).
- **Экраны с колонкой разделов (#3106):** Настройки и Администрирование — утопленная колонка
  `--t-sub` слева, открытый раздел в `?s=`; секции настроек — `components/settings/*` (общие
  классы `st-*` в `settings.css`). Документы — дерево `documents/DocTree` слева (прячется, если
  не помещается рядом с текстом) и шапка документа `doc-head`: управление документом больше НЕ
  телепортируется в topbar, topbar называет экран (`page-crumbs`). Меню `NSelect` подняты над
  поповерами в `kit/kit.css` — селект внутри `PopoverForm` иначе рисуется под формой.
- **Экраны аккаунта, конференции, справка, колокольчик (#3107):** все экраны входа и 404 —
  `AuthLayout` (аврора «Моих задач» `HomeAurora frozen` + стеклянная карточка); корень обязан быть
  ОДНИМ элементом (`App.vue` анимирует маршрут `mode="out-in"`: фрагмент — абстрактный
  NConfigProvider или даже комментарий над корнем в dev — оставлял пустой экран после входа).
  401 от `/auth/login|register` — отказ формы, не истёкшая сессия. Общая кнопка v2 с клавишей —
  `.kit-btn` в `kit/kit.css`. Конференции: список по дням (`utils/confList`), звонок на всю
  рабочую область, док кнопок, клавиши `meet.*` (N V, N J, ⇧J, ⌘D, ⌘E, ⌘⇧H, ⌘⇧M), мини-окно с
  прилипанием к краям; на стенде нет SFU (токен 503) — экран звонка проверяется подменой рефов
  `session.transport`/`room` (драйвер `.design-sync/.cache/v2e2.mjs`, `mockCall`). «Статья
  помогла?» — ack `helpful:<slug>:<lang>:yes|no` без своей таблицы, итоги — `GET
  /admin/help-feedback` и раздел «Справка» админки. F1 — справка.
- **Тур «Начало работы» и подсказки (#3000, #3070):** тур — главы (`data/getStarted.js`:
  «Основы» 10 шагов, главы по выбору, «Знакомство» для приглашённого, «Основы» телефона нижним
  листом, мини-тур «Что изменилось»), движок `stores/tour.js` (правила `click/count/set/appear/
  vanish/moved`, «Назад» по инфо-шагам, Esc → «Прервать?», «Засчитано»), слой `TourOverlay.vue`
  («Не видим …» с отсчётом вместо молчаливого пропуска). Чек-лист на «Моих задачах» —
  `stores/onboarding.js` считает **результаты** по событиям: свои события сокета (бэкенд кладёт
  `actor` в создание/перенос проекта и группы, доски, этапа, теги/исполнителей — **новый
  user-initiated broadcast делать `broadcastAs`**) и клиентские `tourEvent()` (`utils/tourEvents`).
  Подсказки — `data/hints.js` + `stores/hints.js` + `tour/SpotlightHint.vue` (любой якорь,
  «новое»/«при первой встрече», ≤2 за сессию, ждут свободный экран). Всё — в
  `user_acknowledgements`: `getstarted:*` (done/skipped, `ch:<глава>`, `item:<результат>[:self]`,
  `checklist:hidden`), `spotlight:<ключ>`, `hints:off:new|ctx`, `whatschanged:v2|later`;
  сброс — `DELETE /users/me/acknowledgements?key=|prefix=`. Новый якорь шага — в `MARKUP`
  `tests/dt-get-started.spec.js`; e2e-пользователи (`register()`) выключают тур и подсказки.
- **Редактор описаний/комментариев — собственный `MarkdownEditor.vue`** (textarea +
  тулбар + Написать/Просмотр + @-mentions), хранит **Markdown**. **TipTap пробовали и
  отвергли** (тема/бандл) — не возвращать без явного запроса.
- **Бренд:** канон в `design/tessera-brand-v2/` (одиночная строчная «t» + плитка
  tessera, Fredoka, градиент `#6D5FE0→#7C6CFF→#9183FF`; монограмма «mt» из v1 больше
  НЕ используется). Сделано пользователем — **не перегенерировать**. Интегрировано во
  все клиенты (web/Android/desktop) в рамках ребренда 2026-07-28. Анимированный лоадер
  живёт в `design/tessera-brand-v2/loader/` (`loader-states.json` — общий источник
  чисел для web `src/utils/tesseraLoader.js` и Android `TesseraLoader.kt`, держать в
  синхроне).

## Скиллы проекта (`.claude/skills/`)

- **tessera-ship** — bump версии + CHANGELOG + conventional-commit (не пушить/не тегать).
- **tessera-backend-feature** — цикл бэкенд-фичи: sqlc-query → generate → handler → route → миграция → тест.
- **tessera-e2e** — безопасный backend e2e против `tessera_test` (гочи bash/python/порты).
- **tessera-android-release** — сборка и публикация подписанного релиза + self-update (`latest.json`).
- **tessera-task-workflow** — работа с задачами через MCP (`tessera-mcp`): взять в работу, уточняющие вопросы, приложить результаты/скриншоты, вернуть на проверку. Юзер назначает задачи на бота; агент возвращает автору в «На рассмотрении».
- **tessera-sentry** — триаж/анализ ошибок и верификация фиксов через self-hosted Sentry (MCP `sentry-self-hosted`), **только если Sentry настроен и жив**; иначе — по логам. Побочные/незнакомые ошибки → заводить отдельные задачи в Tessera.

## Где что искать / как поддерживать

- **`ROADMAP.md`** — план фаз и near-term дорожная карта (источник истины для «что дальше»).
- **Auto-memory** (`~/.claude/projects/.../memory/`) — накопленный опыт по фазам и решениям;
  читается как фон, может устаревать — сверяй с кодом.
- **Фиксируй новое:** durable-факты проекта → сюда (CLAUDE.md) или в ROADMAP.md;
  повторяемые процедуры → в скилл; точечные наблюдения → в memory. Не плодить дубли.
