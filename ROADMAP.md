# ROADMAP — Tessera

Источник истины для «что сделано / что дальше». Обновлять по ходу работы (и в том же
изменении правда важнее красоты). Детали реализации — в коде, CHANGELOG и auto-memory.

Текущие версии — `make version`. На 2026-06-27: backend `0.59.1` ·
frontend `0.102.0` · android `0.40.0`. Следующая миграция — `0037`.

## Статус фаз (0–10)

Порядок разработки: архитектура → каркас → оформление → функционал → коллаборация → мобилка.

| Фаза | Тема | Статус |
|------|------|--------|
| 0 | Фундамент (монорепо, docker-compose, gin-скелет, pgx-pool, WS-hub, migrate, Makefile, CHANGELOG) | ✅ |
| 1 | Backend модель + API (домен, sqlc, auth JWT+refresh, scope-авторизация, M:N, midpoint-позиции) | ✅ |
| 2 | Frontend каркас (Vue3+Vite+Naive+Pinia+Router, api refresh-on-401, sidebar, auth, read-only board) | ✅ |
| 3 | Оформление (7 акцентных схем + свет/тьма, `--t-*` токены, адаптив, a reference tracker-полировка) | ✅ |
| 4 | Канбан (DnD задач, realtime reload, модалка задачи, **killer-фича: статусы/теги-режим**) | ✅ |
| 5 | Богатый функционал (кастомизация колонок, заметки, напоминания — хранилище+UI) | ✅ |
| 6 | Коллаборация/realtime (участники+роли, инвайты, activity-лента, WS-уведомления) | ✅ |
| 7 | Тесты/CI (backend unit, vitest, GitHub Actions ci.yml) | ✅ |
| 8 | **Android** (Kotlin Compose, online-first, повторяет web-моб; self-update) | ✅ паритет с web (итеративно, 0.21.x) |
| 9 | Деплой (multi-stage Dockerfile, nginx-прокси, distroless backend, prod-compose) | ✅ **LIVE в проде** (tessera.msdnna.website, 2026-06-18) |
| 10 | a reference tracker UX-overhaul (вложенные группы, sidebar-дерево, inline-доска, теги на лету, редизайн карточек) | ✅ (итеративно) |

Подробные подпункты и гочи каждой фазы — в auto-memory `project-phase-plan`.

## Near-term дорожная карта (порядок задал пользователь 2026-06-09)

### 0. Tooling-гейт — `CLAUDE.md` + скиллы + `ROADMAP.md` — ✅ СДЕЛАНО (этот заход)
Память одна не тянет проект такого размера; без доков и скиллов теряли контекст между
тредами. Создано: корневой `CLAUDE.md`, этот `ROADMAP.md`, скиллы в `.claude/skills/`
(tessera-ship, tessera-backend-feature, tessera-e2e, tessera-android-release).
**Дальше — поддерживать эти файлы, фиксировать новое в них.**

### 1. Android — оставшиеся чистые app-фичи — ✅ device-verified (0.3.1)
**Git-трекинг `android/` — РЕШЁН 2026-06-10:** исходник Android (а также `CLAUDE.md`,
`ROADMAP.md`, `.claude/`, `design/`, `tools/build-android-release.sh`, `apks/`) ранее не
был `git add`-нут — релизы собираются с диска, поэтому это не мешало. Теперь всё
закоммичено и трекается. Follow-ups 0.3.0–0.3.1: @-mentions (пикер+подсветка),
создание колонки с доски, dashed-плейсхолдер, accent-gradient на ссылках/mentions.

- **State persistence** — DataStore хранит `expandedGroups`/`expandedProjects` + `lastDest`
  (восстановление Home/Notes/Reminders/board при запуске; deep-link напоминания приоритетнее).
- **Login-редизайн** — purple-gradient full-bleed, крупная белая «mt», frosted-инпуты,
  настройки сервера за шестерёнкой с live-URL (без Save).
- **In-app self-update** — `UpdateRepository` опрашивает `<server>/apks/latest.json`;
  sidebar-строка «Доступно обновление»; ставит через package installer. См. скилл **tessera-android-release**.
- **Урок (стоил release-бага):** любая Gson-модель ДОЛЖНА жить в `data.model` (иначе R8
  переименует поля → парсинг в дефолты). `latest.json` versionCode = major*10000+minor*100+patch.
- **Caveat:** `location /apks/` в `frontend/nginx.conf` закоммичен (отдельный commit), но
  активируется только после пересборки фронт-образа — иначе отсутствующий apk падает в
  SPA-index (200 HTML) вместо 404. Для happy-path self-update не критично.

### 2. Frontend (web) — дизайн-рефреш — ✅ СДЕЛАНО (web 0.42.x → 0.43.0)
- **Порт полировки Android ОБРАТНО на web.** Accent-gradient перенесён Android→web
  (web 0.42.0–0.42.8; memory `project-frontend-gradient-port`), + favicon/лого из
  бренд-бандла, кастомный лоадер, редизайн логина.
- **0.43.0:** destructive-действия переведены на inline `n-popconfirm`-поповеры
  (Android-стиль) вместо модалок; акцентные кнопки/иконки в поповерах.
- Точечные багфиксы — по мере появления.

### 3. Бизнес-фичи — GitLab self-hosted Issues integration — ✅ фаза A реализована (B+ в бэклоге)
- **Цель:** тянуть issues/work-items, назначенные на пользователя, из self-hosted
  GitLab (self-hosted) в Tessera вместо дублей.
- **Решено по дизайну (2026-06-11):** транспорт — **pull-first polling** (Tessera→GL,
  работает во всех топологиях с egress; webhooks — потом). GraphQL (issues+work-items
  единообразно, иерархия для `M:`-подзадач). Привязка: **per-user PAT** (credential) +
  **per-workspace integration** (project→board + правила лейблов). Лестница auth:
  PAT → OAuth2 → SSO (последнее за скобки). Двухсторонность (write-back), webhooks,
  OAuth — **в бэклог**.
- **Ядро кастомизации — движок правил по namespace-префиксам лейблов** (таксономия
  пользователя `S:`/`P:`/`T:`/`C:`/`Scope:`/`effort::`/`M:`/`B:`): эффект, а не «куда
  положить». `S:`→колонка, `P:`→priority, прочее→теги (с префиксом, additive — ручные
  scope-теги не затираются). `B: Future`→доска Backlog и `M:`→подзадачи — спроектировано,
  отложено.
- **Фаза A реализована целиком** (manual+авто pull, без write-back):
  - backend `0.19.0`: migration 0009, `internal/secrets` (AES-GCM, `ENCRYPTION_KEY`),
    `internal/gitlab` (GraphQL `project.issues(assigneeUsername:)` + rule engine, юнит-тесты),
    эндпоинты connection/integration/sync. **Живой sync против GL (self-hosted) подтверждён пользователем.**
  - backend `0.20.0`: автор issue на `gitlab_links` (отдаётся в `gitlab` на GET /tasks/:id;
    синканные задачи без `created_by`), **фоновый воркер автосинка** (owner_user_id +
    sync_interval_sec + last_synced_at; `runSync` вынесен из gin-хендлера), migration 0010.
  - backend `0.21.0`: `GET /boards/:id/tasks` отдаёт `gitlab_iid`/`gitlab_url` (для бейджа).
  - frontend `0.44.0`: модалка GitLab (аккаунт PAT + конфиг интеграции + редактор правил
    статус→колонка/приоритет→уровень/теги + Sync now), бейдж `!iid` на карточке, строка
    автора в TaskModal. Build/lint зелёные; **UI вживую против GL пользователем ещё не прогнан**.
- **Доработки после фазы A (2026-06-11, backend 0.20–0.23, web 0.45–0.46):** автор задачи
  (created_by/GL-автор) в модалке + автор→исполнитель на карточке; цвета тегов из GL +
  авто-цвет + читаемость по теме (`readableHue`); сроки из issue (milestone — следующий шаг);
  **синк исполнителей и комментов с внешними GL-юзерами** (migration 0011: `task_gitlab_assignees`,
  `task_comments.gl_*`); **mixed-sync** (`source` user|gitlab на тегах/ассайни — реконсайл только
  gitlab-набора); слинкованные задачи добираются по iid → переназначение отражается, синк не удаляет.
- **Доработки 2 (2026-06-11, backend 0.24–0.25, web 0.47–0.49) — ✅ СДЕЛАНО:**
  (2) сроки из **milestone** `End date` (issue.dueDate ?? milestone; ручной срок > GL через
  `due_overridden`; UI `due_source`); (3) **генерализация правил** — label_rules → список
  `{match: prefix|regex, action: status|priority|board|group|tag}` (board роутит на другую
  доску; group распознаётся, подзадачи — позже через GraphQL hierarchy); generic-редактор в UI;
  (4) **группировка по неймспейсу тега** на доске (авто-префиксы + кастом, persisted).
- **Доработки 3 (2026-06-11, backend 0.26–0.29, web 0.50–0.53):** баги (контраст тегов,
  тултипы, markdown с inline-HTML, closed→Done, инициалы Имя/Фамилия и a.fokin→AF, любые @-mentions);
  **сохранённые представления досок** per-user (миграция 0013); **многоуровневая сортировка**
  (список уровней поле+направление); **`M:`-подзадачи** — дети work-item'а из GL Hierarchy widget →
  Tessera subtasks (migration GraphQL, дедуп от top-level); **прокси вложений GL** (подписанный
  `/api/gitlab/asset`, переписывание `/uploads/…` на синке). Кнопка «Интеграции» (dropdown).
- **Доработки 4 (2026-06-11, backend 0.29.1, web 0.54.0):** **composer-bar** — группировка/
  сортировка/фильтры как удаляемые чипы в одном широком баре с «＋»-меню + инлайн-поиск
  (заменил кнопки Группировка/Сортировка/Фильтры); фикс **прокси вложений** (тянем через
  uploads API `/api/v4/projects/:id/uploads/:secret/:filename` по PAT, а не web-роут).
- **Фаза B — write-back — ✅ MVP СДЕЛАНО** (backend 0.53.0 / web 0.92.0, mig 0032, 2026-06-23):
  state/priority/comment, opt-in, async-outbox, loop-guard. Подробности — в «Что дальше» ниже +
  [[project-gitlab-writeback]]. Остаток (assignees/title-desc/`column_label_bindings`/webhooks/OAuth) — в бэклоге.
- **Сопутствующие фиксы (2026-06-23):** статус-маппинг сделан **регистронезависимым** (backend 0.53.1 —
  `S: In Progress`/`S: In Review` больше не уезжают в дефолт-колонку); HTTP-клиенты GL **fail-fast**
  (backend 0.53.2 — 3с dial + 8с/15с таймауты, чтобы падение GL не вешало UI на прокси вложений/аватаров).

### 5. UI-полировка клиентов + критфикс — ✅ (Android 0.8.0, web 0.55.0, 2026-06-15)
- **Анимации (web + Android):** login-aurora градиент, переходы маршрутов/поповеров/диалогов,
  Android `Modifier.popupAppear()` + `Crossfade` навигации/режимов доски (батч от 2026-06-12,
  отревьюен и зашипен в этом заходе).
- **Kanban perf (>100 карточек):** Android — виртуализация колонок + memoise lanes (0.7.0);
  web — дебаунс persist + общие flag-градиенты (0.54.3). Виртуализация web-колонок — в бэклоге.
- **Критбаг (Android):** группировка по namespace-тегу намертво вешала доску — weighted
  `BasicTextField` в `FlowRow` композера → intrinsic-measure ANR (диагноз по device-ANR с
  совпавшим r8 map-id). Фикс `Modifier.zeroIntrinsicWidth()`; + полировка композера (отступы 8,
  тап-в-любое-место → разворот) на обоих клиентах. См. [[project-android-composer-anr]].

### 6. Управление пользователями — ✅ СДЕЛАНО (U1+U2+U3, 2026-06-16)
- **U1** профиль/права/аватар (mig 0014): профиль (ФИО+bio/company/job_title), смена пароля,
  аватар (DB-blob), preferences→DB (тема/акцент/локализация), permission-матрица
  (owner/admin/member через `requireManager`), inline-роли, type-to-confirm удаление.
- **U2** аккаунт-лайфсайкл (mig 0015): инвайты (link/email), верификация почты, сброс пароля,
  деактивация. SMTP-креды не заданы → no-op mailer пишет ссылки в логи/ответ API.
- **U3** глобальная админ-панель (`handlers/admin.go`, префикс `/admin`): список юзеров,
  активация/деактивация, выдача/снятие admin, генерация reset-link (без SMTP). Web `/admin` +
  android AdminScreen, гейт по `is_admin`.
- **Аватары на карточках/в модалках** (web 0.58 / android 0.11) + **GitLab-аватары** синк-задач
  (mig 0016, backend 0.32). Версии: backend 0.33 / web 0.60 / android 0.13. След. mig — 0017.
- **Открытый долг:** pre-existing backend revive/lint (~39, старые файлы) — кандидат на отдельный
  `chore(backend)`. Детали — auto-memory `project-user-management-next`.

### 7. Полировка-раунд: GL-медиа прокси + анимации + теги — ✅ СДЕЛАНО (2026-06-16)
- **GL-медиа на клиентах без доступа к GitLab** (backend 0.33.1): аватары GL-юзеров шли
  абсолютными GL-URL (телефон до GL не достаёт), а прокси вложений делал редирект на GL-хост
  (работает только в браузере с GL-сессией). Добавлен подписанный прокси `GET /api/gitlab/avatar`
  (токен шлётся только на GL-хост, не на gravatar); прокси вложений теперь стримит файл на
  старых GitLab (<17.4) вместо редиректа. **URL переписываются при синке → нужен ре-синк для
  уже привязанных задач.**
- **Web-анимации** (web 0.61.0): плавное сворачивание сайдбара по даблклику разделителя
  (drag включается только после порога движения); плавный переход dim↔active у composer bar.
- **Теги** (web 0.61.0 / android 0.13.1): `+N` вместо переноса имён тегов в модалке (Android);
  читаемый текст на сплошных выбранных тегах через хелпер `onColor` (luminance по цвету тега) —
  оба пикера, web + Android.

### 8. UX-полировка раунд 2: back-навигация + splash/loading + dropdown — ✅ СДЕЛАНО (web 0.71.0 / android 0.16.0, 2026-06-17)
Пакет визуальных/поведенческих фиксов после прогона на устройстве (MIUI):
- **Back закрывает оверлеи (web):** новый composable `useOverlayBack` (pushState
  заглушки на открытии, раскрутка на UI-закрытии) — браузерный «Назад» закрывает
  таск-модалку / members / GitLab / архив доски / мобильный drawer / открытую заметку
  вместо ухода со страницы.
- **Back-навигация (Android):** в `MainScreen` добавлен стек `MainDest` — жест «Назад»
  с экрана, открытого через сайдбар, возвращает на предыдущий/начальный экран (а не
  закрывает приложение); в корне — хинт-тост и сворачивание (`moveTaskToBack`) по
  второму жесту. Back закрывает drawer, search-overlay и заметку; в таск-модалке Back
  с неактивной вкладки → первая вкладка, второй Back → закрытие. `BackHandler` per-overlay
  (Dialog-модалки уже глотают Back сами).
- **Splash (Android):** сначала пробовали скруглённый бейдж, но MIUI рисует квадрат
  при любом подходе (и baked rounded-rect, и `windowSplashScreenIconBackgroundColor`).
  Итог (android 0.17.0): **без бейджа вообще** — белая «mt»-марка на фиолетовом фоне
  (`mark-white.svg`). См. [[reference-android-splash-rounded-badge]].
- **Loading-gate + ошибки старта (Android, 0.17.0):** старт держит загрузочный сплэш
  (белая тессера на градиенте) пока идёт проверка доступности API + сессии (таймаут
  30с). Спустя 5с под лоадером — мягко сменяемые надписи. Исходы: сервер недоступен →
  экран на фиолетовом «Не удалось подключиться к серверу API» [Попробовать ещё раз]
  [Выход]; 401/403 (сессия истекла/блок) → [Выполнить повторный вход][Выход]; иначе →
  борда/auth. Текст/кнопки светлые (стиль логина). In-app загрузки (борда/Home/таск-
  модалка) — те же надписи + themed `ErrorState` с конкретным сообщением и одной
  кнопкой Retry (переиспользуемые `LoadingState`/`ErrorState`/`LoadingCaptions`).
- **Mobile dropdown (web):** drill-in пункты снова со стрелкой «›» справа, у «‹ Назад»
  больше зазор, переход между уровнями со слайдом (вниз при углублении, вверх при возврате).

### 9. Система уведомлений — каналы + роутинг + расписание — ✅ СДЕЛАНО (backend 0.34–0.41, web 0.62–0.70, android 0.14–0.15.1, 2026-06-16)
Крупнейший новый кусок после управления пользователями. Слабо связан с ядром (свой пакет
`internal/notify`, свои таблицы, `type` каналов — свободная строка, секреты шифруются тем же
sealer'ом, что GitLab). Миграции **0017–0021**.
- **Фаза A (backend 0.34, mig 0017):** пользовательские внешние **каналы** (`email`/`telegram`/
  `webhook`, per-user CRUD + `/:id/test`; не-секретный `config` JSONB, секреты AES-256-GCM,
  наружу только `has_secret`) + **правила роутинга** в стиле Alertmanager (JSONB-матчер по
  kind событий + workspace → набор каналов или `mute`; первое подходящее правило выигрывает,
  in-app уведомления безусловны) + **outbox-воркер** (claim `FOR UPDATE SKIP LOCKED`,
  квадратичный backoff, fail после 5 попыток — внешняя доставка асинхронна, ретраится, переживает рестарт).
- **shoutrrr-транспорт (0.35):** Telegram через библиотеку [shoutrrr] + generic-канал `shoutrrr`
  (slack/discord/ntfy/gotify/matrix/pushover/teams/… по service-URL) — универсальный escape-hatch
  без кода под каждый провайдер. + **редакция секретов из текста ошибок** (0.34.1: токен бота в
  URL утекал в `last_error`).
- **Шаблоны сообщений (0.36, mig 0018):** Go `text/template` per-channel (`{{.Text}}`/`{{.TaskTitle}}`/
  `{{.Actor}}`/…), endpoint предпросмотра; пустой шаблон = встроенный дефолт.
- **Due/reminder-уведомления (0.37, mig 0019):** фоновый сканер (60с) шлёт по `notification_prefs`
  (lead-минуты до due + repeat-N) с per-task override (`PATCH /tasks/:id/due-notify`); напоминания —
  один раз в `remind_at` рядом с локальным Android-алармом. Per-(task,user) state дедупит и пере-арм
  при правке due.
- **Тихие часы (0.38, mig 0020)** + **дайджест/группировка (0.39, mig 0021)**: окно тишины держит
  внешнюю доставку до конца окна; дайджест-окно объединяет доставки в один канал в одно сообщение.
- **Device-каналы (0.40):** клиент (browser/Android/desktop) — роутируемая цель по стабильному
  `device_id`; событие WS несёт `device_targets`, клиент поднимает нативное OS-уведомление.
  Per-device правила (только `assigned` на телефон, всё — в браузер+Telegram).
- **Больше kind'ов + умный контекст (0.41):** `archived`/`updated`/`moved` титулованы; короткий
  контекст (≤16 симв.) инлайнится в текст.
- **Web (0.62–0.70):** экран «Уведомления» — каналы/правила/расписание/шаблоны/предпросмотр.
- **Android (0.14–0.15.1):** экран «Уведомления» (bell в сайдбаре), авто-регистрация device-канала,
  **системные уведомления пока приложение открыто (C2)**, редактор шаблонов. **Background push (FCM)
  НЕ входит** — это остаётся в бэклоге (см. ниже).

### 10. Реальная доставка email — ✅ СДЕЛАНО (backend 0.45.1–0.45.6, 2026-06-18)
no-op mailer заменён рабочим SMTP. Долгая сага против Yandex-спам-фильтра:
- **0.45.2:** implicit TLS (465/SMTPS) вдобавок к STARTTLS (stdlib `smtp.SendMail` умеет только
  STARTTLS → 465-сервер вешал запрос); асинхронная отправка (`mail.SendAsync`) + логирование сбоев + таймаут 20с.
- **0.45.3–0.45.6:** RFC-совместимость (`Date`/`Message-ID`/RFC-2047 Subject/From-display-name),
  переименование reset-письма и URL `/reset-password`→`/recover` (подстрока «password» триггерила
  спам-фильтр), и финальный фикс — **quoted-printable вместо `8bit`** (net/smtp не договаривается о
  `8BITMIME`, кириллический UTF-8 как `8bit` Yandex резал как спам, `554`).
- Инвайты/верификация/сброс уже были реализованы — здесь добавлен живой транспорт (smtp.yandex.ru,
  support@msdnna.website). Прод-гоча с outbound-SMTP — см. [[project-prod-deploy]].

### 11. Теги per-project + человекочитаемые URL + имена префиксов — ✅ СДЕЛАНО (backend 0.43–0.45, web 0.72–0.74, android 0.18/0.21, 2026-06-18)
- **Теги per-project (mig 0023):** scope тегов = Project, не Workspace; create/list на
  `/projects/:id/tags`; `/workspaces/:id/tags` остаётся read-only для Home. GitLab-лейблы → проект
  интеграционной доски. См. [[project-tags-per-project]].
- **Slug-URL (mig 0024/0025):** вложенные роуты `/project/<slug>/board/<slug>?task=<number>`; project-slug
  глобально уникален, board-slug per-project; `internal/slug` транслитерирует кириллицу. См.
  [[project-human-readable-urls]]. **Не возвращать UUID в URL.**
- **Понятные имена префиксов тегов (mig 0026):** провайдеро-нейтральный per-project store
  (`projects/:id/tag-prefixes`); `S:`→«Статус» и т.п.; группирует меню composer-bar и **все** пикеры
  тегов на web и Android. Редактируется (пока) из GitLab-модалки. См. [[project-tag-prefix-names]].
  **Бэклог:** редактирование префиксов пользовательских тегов в TagManager (не только через GitLab).

### 12. Повторяющиеся задачи — полный паритет с a reference tracker — ✅ СДЕЛАНО (backend 0.46–0.47, web 0.77–0.78.2, android 0.19–0.20.1, 2026-06-19)
mig **0027** (jsonb `tasks.recurrence`, `internal/recur` + тесты).
- **Полный набор правил:** freq `daily|weekly|monthly|yearly|custom` (custom = выбранные на календаре
  даты) + `interval` + weekly `weekdays` + day/month-якорь (30-е переживает февраль).
- **Триггеры:** `complete` (закрытие), `column` (перенос в `trigger_column`), `schedule` (автопродление
  по прошествии due через фоновый `RunRecurrenceWorker`, пропускает пропущенные за простой).
  + `target_column` / `create_new` (дубликат vs reschedule) / `once` / `skip_weekends`.
- **Shared `DueEditor` (web):** общий компонент для card-due-popover и модалки; календарь с подсветкой
  ближайших вхождений + панель правила. `utils/recurrence.js` / `util/Recurrence.kt` зеркалят backend.
- **Настройки уведомлений задачи на карточке** (due lead/repeat/reminder) — web + Android.
- **Гоча:** ЛЮБОЙ task-update payload обязан нести `recurrence`, иначе затрёт правило. См.
  [[project-recurring-tasks]].

### 13. Прод-деплой LIVE + GL-медиа прокси + Android-полировка — ✅ СДЕЛАНО (2026-06-16…19)
- **Production LIVE (2026-06-18, backend 0.45.1 / web 0.74.1):** single-box Docker на Timeweb VDS
  `203.0.113.10`, `tessera.msdnna.website`. Caddy (auto-TLS) → frontend nginx → distroless backend →
  postgres; image-tarball workflow в `deploy/` (`build-and-save.sh` → `scp` → `docker load` →
  `compose up` → `exec backend /migrate`). Гочи (Hub rate-limit → ship base images по tar;
  DATABASE_URL escaping → передавать `POSTGRES_*`, не готовый URL; outbound-SMTP блок на IPv4 → Docker IPv6).
  Прод-БД `tessera` — **НИКОГДА не TRUNCATE/DROP**. См. [[project-prod-deploy]]. **Бэклог:** GHCR + `docker pull`.
- **CORS-lock + nginx body-size (backend 0.45.1 / web 0.74.1):** в проде `CORS_ORIGIN` дефолтит на
  `PUBLIC_URL` (не `*`); `client_max_body_size 50m` для аватаров/вложений за `/api`.
- **GL-медиа на клиентах без доступа к GitLab (backend 0.33.1–0.33.2):** подписанный прокси аватаров
  `GET /api/gitlab/avatar` (токен только на GL-хост, не на gravatar) + стрим вложений на старых GL
  (<17.4) вместо редиректа. URL переписываются на синке → нужен ре-синк уже привязанных задач.
- **Android-полировка (0.18–0.21.2):** due-time picker + GL-offset-фикс (date-only детект по UTC-инстансу,
  не по строке — см. [[reference-prod-timestamptz-offset]]), дефолтный server-URL, humanizeError-карта,
  фикс «Моя работа» 404 (workspace-scoped tags), drawer-header.

## 14. Новые представления доски — Гант / Таймлайн / Матрица Эйзенхауэра — ✅ СДЕЛАНО (все три, оба клиента)
Расширяем набор представлений (сейчас `board`/`list`/`calendar`) тремя новыми. **Делаем по одному
в полном функционале (по референсу a reference tracker 4 / a reference tracker), не «по минимуму»; на каждом клиенте (web +
Android).** Решение — 2026-06-19.

**Точки расширения (готовы, без переделок):**
- web: `boardView.layout` → диспетч `v-if/else-if` в `KanbanBoard.vue`; saved-views `config` JSONB
  уже несёт `layout` → новые типы без миграции `board_views`. Каждый layout-компонент ~250–350 строк.
- android: `enum BoardViewMode` (BoardViewModel) → `when()` в `BoardScreen.kt` (Crossfade); конфиг
  в DataStore + board_views теми же ключами. Свитчер — `MainScreen` `ViewSegments`.
- Общая инфра обоих клиентов: `useTaskMenu`/контекст-меню, `TaskCard`, фильтры/сортировка/группировка
  переиспользуются.

**Ключевое ограничение модели:** у задачи только `due_date`, **нет даты начала**. Гант и таймлайн
рисуют *отрезок* (start→end) → нужен **`start_date`** (миграция **0029**, nullable timestamptz —
матрица заняла 0028). **✅ СДЕЛАНО** (backend 0.49 / web 0.80 / android 0.23): start_date в API
(create/update, full-replace, несут все task-update пейлоады) и в UI обоих клиентов — редактируется
в **поповере срока** через пару вкладок «Начало»/«Срок» (карточка + модалка), с подсветкой отрезка
в календаре. Матрице Эйзенхауэра и существующим видам start не нужен.

**Рекомендованный порядок (по возрастанию объёма; каждый шаг готовит почву следующему):**

1. **Матрица Эйзенхауэра** — ✅ СДЕЛАНО (backend 0.48.0 / web 0.79.1 / android 0.22.1, 2026-06-19).
   2×2 (Важно×Срочно). Квадрант *выводится* из полей — ось **Важно** = priority≥high, ось
   **Срочно** = `due_date` в пределах окна (дефолт 7 дней / просрочено). **derive + ручной override**
   (полный a reference tracker-вариант): nullable `eisenhower_quadrant` (миграция **0028**), `PATCH
   /tasks/:id/eisenhower {quadrant:0-3|null}` (null = «вернуть на авто»). Показывает только **открытые**
   задачи. **Web** — полноразмерный `TaskCard`, drag между квадрантами (vuedraggable) + hover-пин
   «вручную» + per-quadrant «＋» (авто-фокус, бордер). **Android** — **компактная** карточка (полный
   TaskCard схлопывался под weight в тесном 2×2 → перенос по буквам); override через per-card меню;
   квадрант разворачивается на весь экран (анимация); перемещение анимируется (`animateItem`);
   индикатор/строки подзадач; рамки как у доски (`leftAccentFrame`/`topAccentFrame`). Иконки
   `apps`/`expand`. **DnD на Android сознательно НЕ делали** (drag-система доски заточена под
   1D-лейны; матрице нужен 2D-хит-тест + конфликт scroll-vs-drag в тесных квадрантах) — меню вместо
   него; можно добавить позже. Бэклог матрицы: web-виртуализация квадрантов при больших списках.
2. **Таймлайн** — ✅ СДЕЛАНО (web 0.81 / android 0.24, 2026-06-19). Горизонтальная шкала времени с
   отрезками-барами start→due. **Web** (`BoardTimelineView.vue`): свимлейны (группировка по
   исполнителю/тегу/статусу/без), фиксированная левая колонка + липкий заголовок месяцы/дни,
   вертикальная «сегодня», бар = priority-градиент (одноконцовая задача = 1-дневный бар, тянется в
   отрезок), счётчики overdue/без-дат, лента «Без дат», **drag-move бара (оба конца, span сохр.) +
   resize краёв (start/due с зажимом)** — проверено e2e через headless Chrome. **Android** (`BoardTimelineView`
   в BoardViews.kt): то же отображение (общий h-scroll заголовка/дорожек, v-scroll левой колонки/треков),
   перепланирование через поповер срока (тап→задача); **in-bar drag сознательно не делали** (как у
   матрицы: drag-движок доски 1D-лейновый, конфликт с h-scroll трека). **Без связей между задачами.**
   **Ревью-фиксы (web 0.82 / android 0.25):** убрана дублирующая группировка (свимлейны берут
   группировку из composer-bar); добавлены **сорт/фильтр по статусу** (только на таймлайне) — фильтр
   скрывает завершённые из «Готово»; **per-layout стейт тулбара + saved-views по визуализации** (web);
   **масштаб/панорама/hover-превью** (web). Android-бэклог паритета: группировка по исполнителю,
   zoom/pan/preview, per-layout стейт. См. [[project-board-views-next]].
3. **Гант** — ✅ СДЕЛАНО (backend 0.50 / web 0.83 / android 0.27, 2026-06-19). Переиспользует
   таймлайн-движок (ось/бары/зум/пан/today/свимлейны/drag-resize) и добавляет **зависимости задач**.
   **Без новой таблицы:** граф связей уже есть — `task_relations` несёт `blocks`/`blocked_by` с
   миграции 0006 (+ UI «Связи» в карточке). Добавлен только board-scoped read: `GET
   /boards/:id/dependencies` (все блокирующие рёбра, где оба конца на доске; raw-строки `{id,
   task_id, related_task_id, kind}`, клиент нормализует в blocker→blocked). **Web** (`BoardGanttView.vue`):
   SVG-стрелки finish-to-start (геометрия строк из модели лейнов), **drag-to-link** от узелка на
   правом крае бара к другой задаче (создаёт «блокирует»; дубли/прямые 2-циклы отсекаются), удаление
   стрелки крестиком при наведении; счётчик «N связей». Проверено e2e через headless Chrome
   (рендер, drag-to-link 2→3, удаление 3→2). **Android** (`BoardGanttView` в BoardViews.kt): те же
   стрелки одним Canvas над треком (левая фикс-колонка и трек делят `vScroll`; бары+Canvas — один
   вертикальный вьюпорт); **drag-to-link сознательно не делали** (как in-bar-drag — конфликт с
   h-скроллом), связи редактируются во вкладке «Связи» карточки. Собирается + ktlint/detekt, не на
   устройстве.

**Синергия с оценкой задач:** сайдбар таймлайна «No effort» и бейджи «⏱30h» — это и есть оценка
(см. бэклог ниже). Виды покажут оценку, как только она появится; делать виды можно и до оценки.

**Пост-релизные фиксы видов:**
- **Android Gantt/Timeline crash-cap (android 0.29.1):** широкая дата-ось (start_date=createdAt
  растягивала ось до ~66k px) + высокое тело трека → Compose не может упаковать `w+h` в `Constraints`
  (Long) → краш. Починено кэпом дневного окна в px (`TL_MAX_AXIS_PX`, якорь на недавний конец,
  `dayIndex coerceIn`). Кэп не убирать. См. [[reference-android-gantt-constraints-cap]].
- **GitLab → start_date sync (backend 0.52 / web 0.85 / android 0.29, mig 0031):** `start_source =
  created (дефолт, =createdAt issue) | milestone (startDate) | off`, зеркалит `due_source` 1:1; ручная
  правка → `gitlab_links.start_overridden` переживает ре-синки. Видам Гант/Таймлайн дало реальные
  отрезки на синканных задачах. См. [[project-gitlab-start-date-sync]].

**Доработки видов 2026-06-21…22 (web 0.86→0.87.2 / android 0.30→0.31.6) — СДЕЛАНО:**
- Ghost-оценка (пунктирный конверт оценки под баром) + значение оценки у края + ось включает
  ghost-конец; hover-аватарки+оценка (web); полная расшифровка длительности + тултип с датами
  (web, RU/EN ввод); иконка оценки ionicons5; сворачиваемая левая колонка (оба клиента);
  скруглённые углы рабочей области (android); фикс высоты лейн-хедера Ганта (web, content-driven
  + измеряемая высота для стрелок).
- **Android-паритет с web:** группировка «по исполнителю»/«без группировки» + per-layout тулбар-стейт ✅.
- **Android-перф (≈200 задач):** таймлайн+Гант виртуализированы (`LazyColumn` + Canvas-оверлей стрелок),
  скролл 8–16мс; заголовок дневной оси → один `TimelineAxisCanvas` (рисует видимое окно); мемоизация
  пер-кадровых пересчётов (sort/range/overdue) → зум 99-й перцентиль 250→65мс. Фикс «Сегодня» (корень —
  `viewportPx` мерился после `horizontalScroll`); фикс лоадер-ханга при открытии Ганта.
- **Android-зум-жест:** свой `Modifier.pinchZoom` (Initial-pass, consume только мультитач) вместо
  `transformable`; фокус на центроиде пальцев, вычисляется один раз за жест; отключены pull-to-refresh
  и свайп-сайдбара на TL/Ганте. См. [[reference-android-timeline-perf-viewport]].

**Item 5: масштаб меняет ГРАНУЛЯРНОСТЬ оси — ✅ СДЕЛАНО** (web 0.88.0 / android 0.32.0, 2026-06-22).
Зум меняет единицу оси, тиры по `dayW`: **недели** (`<20` — метка = диапазон чисел `8–14`, недельные
гридлайны) ↔ **дни** (`20–139`) ↔ **часы** (`≥140` — тики `06·12·18` через `hourStepFor`, минорные
часовые гридлайны, шапка выше, today на реальном времени). **Вариант A — под-дневные бары:** timed
start/due встают на свой час, all-day (UTC-полночь, `isAllDayMs`/`tlIsAllDay`) — на границы дня, так
что «весь день» по-прежнему занимает день; стрелки Ганта используют ту же геометрию (`barGeom`). Общая
матиматика web — `utils/timeAxis.js`; на Android — `TimelineAxisCanvas`/`tlGrid` (рисуют только видимое
окно). Web: часовые тики оконные («lazy lines»). Оба клиента, web проверен через CDP, Android — на
устройстве. Пороги/высоты тюнятся константами (`tierFor` / `TL_WEEKS_MAX_DAYW` / `TL_HOURS_MIN_DAYW`).

**Прочий бэклог видов:**
- **Виртуализация строк тела в таймлайне/Ганте на web — ✅ СДЕЛАНО** (web 0.89.0, 2026-06-22).
  Окно по вьюпорту (+`VMARGIN=600px`) + два спейсера, сохраняющих высоту прокрутки; в DOM ~30–50
  строк вместо всех (32 из 440, проверено CDP). Бары/стрелки — чистая математика оси, окно их не
  сдвигает; today-линия и SVG-оверлей стрелок Ганта остаются полноразмерными (зависимости к
  задачам вне экрана рисуются). Строка-задача 36px (фикс), лейн-хедер измеряется один раз.
  **Матрица виртуализации НЕ требует** (решено 2026-06-22): она рендерит только открытые задачи
  (`BoardMatrixView.vue` — «Open tasks only») обычными `TaskCard` в 4 квадрантах нормального потока,
  без широкой оси дат и полноширинных гридлайн-треков → списки ограничены, рендер быстрый. Вернуться
  к этому только если один квадрант когда-нибудь наберёт сотни открытых карточек (маловероятно).
- Гант-зум/сворачивание ещё ~42мс/шаг (inherent: ре-анкор скролла + лейаут видимых строк) — приемлемо.

**Гант «Авто» — порядок строк по графу зависимостей — ✅ СДЕЛАНО** (web 0.90.0–0.90.1 / android 0.33.0).
Кнопка «Авто» (только Гант) сбрасывает composer в «без группировки/сортировки» и упорядочивает строки
обходом графа блокирующих связей в глубину pre-order (корни в исходном порядке, задача перед своими
зависимыми, циклы — в конце ровно по разу). Алгоритм: web `utils/dependencyOrder.js` (+тесты) ↔ android
`topoByDeps`. Режим гаснет при любой группировке/сортировке (`autoActive`). Android-паритет также подтянул
свимлейны Ганта на все режимы composer (status/tag/assignee/none) и убрал кнопку раскрытия подзадач на
видах с осью времени (как web). Фикс web 0.90.1: бар `box-sizing: border-box` (стрелки выходят ровно из
края, бары не «перелетают» срок на 16px).

**Суб-бары подзадач + курсор-линия + полировка тултипов оценки — ✅ СДЕЛАНО** (web 0.91.0, 2026-06-23;
**android-паритет суб-баров — ✅ android 0.35.0**, см. раздел 15). Запланированные подзадачи (со
`start`/`due`) рисуются тонкими суб-барами под родительским баром в той же строке
(`rowHeight`/`SUB_STEP`/`findTask`-save; на Ганте без узелка-связи). Нейтральная пунктирная курсор-линия с приколотой к верху пилюлей даты (в тире «часы» — со
временем; `Teleport`+fixed). Тултипы оценки: `estimateDateRange`→свободный `estimateRangeShort`
(«18 мая → 13 июл.») + общий `estimateTooltip`; чип оценки и лейн-«⏱» переведены с `title` на `n-tooltip`.

## 15. Android-паритет с web/backend — ✅ СДЕЛАНО (android 0.35.0, 2026-06-26)
Аудит Android против последних web/backend-фич выявил гэпы; закрыты одним пакетом (один
коммит на `develop`, не запушен). Что НЕ применимо к тачу — сознательно опущено (курсор-линия,
hover-превью бара, hover-аватарки, колонка-1px-бордер/подсветка свёрнутого рельса — это
указательные/desktop-only; tag-prefix редактор вне GitLab — общий бэклог обоих клиентов).

- **GitLab журнал синхронизации** (паритет web 0.93–0.94 / backend 0.54.0): новый экран
  `GitLabJournalScreen` (мастер-деталь сложен в стек под мобильный) — прогоны pull/push с
  раскрытием действий и диалогом-диффом до/после + retry упавшего пуша; модели
  `GitlabSyncRun`/`GitlabSyncAction`, эндпоинты `sync-runs`/`…/actions`/`…/retry`, дест
  `MainDest.GitLabJournal`.
- **GitLab write-back настройки** (паритет web 0.92.0 / backend 0.53.0): `GitlabWriteback`
  jsonb (enabled/push_state/push_priority/push_comments) в модели интеграции + opt-in тумблеры
  в `GitLabSettingsScreen`.
- **icon_mode проекта/группы** (паритет web 0.94.0 / backend 0.55.0, mig 0034): сегментный
  переключатель «Бейдж/Иконка» в `IconColorPicker`; `icon_mode` в Domain-моделях + Update-запросах;
  `ProjectIcon` красит плашку ИЛИ глиф; проброшен через `SbNode` в сайдбар (узлы + drag-ghost).
- **Пикер исполнителя — поиск + MRU** (паритет web 0.95.0): поиск по имени + порядок
  назначенные→недавние→алфавит (кап 10) в on-card пикере; DataStore `recent_assignees` (зеркало
  web `tessera_recent_assignees`), пополняется в `BoardViewModel.toggleAssignee`.
- **Тема-тумблер на логине** (паритет web 0.95.0 AuthLayout): кнопка смены темы в углу `AuthScreen`
  (до входа — локальная тема `prefs.setDarkMode`).
- **Суб-бары подзадач на Таймлайне/Ганте** (паритет web 0.91.0): `tlSubBars`/`tlRowHeight`/`TL_SUB_STEP`;
  высота строки растёт, геометрия стрелок Ганта и виртуализация (`rowTops`/`itemTopsPx`) учитывают рост.

**Не device-verified** (собирается + ktlint/detekt/тесты зелёные) — пользователь ревьюит/проверяет на
устройстве и сам пушит/тегает.

## 16. Markdown-редактор — интерактивные чекбоксы + полировка ввода — ✅ СДЕЛАНО (web 0.96.x / android 0.36.x, 2026-06-26)
Доработки собственного `MarkdownEditor` (не TipTap — см. [[feedback-editor-markdown-not-tiptap]]):
- **Интерактивные GFM-чекбоксы в превью** (web 0.96.0 / android 0.36.0): кликабельные таск-чекбоксы
  в стиле приложения; тап по квадрату ИЛИ по тексту пункта → переписывает `[ ]`↔`[x]` и сохраняет;
  общий `toggleTaskMarker`. Моноширинный ввод, авто-рост textarea.
- **Анимация галочки + фиксы** (web 0.96.1–0.96.4 / android 0.36.1–0.36.4): центрирование/«поп»-анимация
  тика, убран градиентный «шов» на `appearance:none`-боксе (`background-image`+`border-box` origin),
  тик `translate -60%`, починка авто-роста textarea (`@after-enter` + сохранение скролла), стоп
  scroll-jump модалки при наборе. Грабли — перерисовка убивает CSS-анимацию → нативный тоггл + пропуск
  пересборки (web `skipBuild` / android `skipNextLoad`). См. [[project-markdown-interactive-checkboxes]].

## 17. Виртуализация списков карточек на канбане (web) — ✅ СДЕЛАНО (web 0.97.0, 2026-06-26)
Закрыт давний perf-долг web-канбана (>100 карточек; Android давно на `LazyColumn`). Подробности —
в бэклоге ниже («Виртуализация колонок на web») + CHANGELOG. Кратко: IntersectionObserver-оконность,
плейсхолдеры измеренной высоты, DnD не тронут, проверено через CDP.

## Что дальше (бэклог; приоритет не зафиксирован — выбирает пользователь)

Большие фазы (уведомления, email, теги per-project, URL, повторы, прод-деплой) закрыты. Активная
работа — новые представления (раздел 14). Прочие кандидаты (осознанность, не обязательства):

- **Оценка задач (estimation)** — ✅ **СДЕЛАНО** (backend 0.51 / web 0.84 / android 0.28, миграция
  **0030**). Канон `tasks.estimate` (nullable double) — число, единица которого резолвится из
  двухуровневого конфига (`projects.estimation` → `workspaces.estimation` jsonb → встроенный дефолт
  `время/8ч/5д`), как у префиксов тегов. Конфиг правится своими ручками `PUT /workspaces|projects/:id/estimation`
  (имя-edit не затирает). Клиенты делают весь парс/формат (`utils/estimation.js` / `util/Estimation.kt`):
  - **Время:** канон-минуты + рабочий день/неделя; ввод «3д 4ч»/«1н»/«90м» (EN+RU, голое число = часы)
    → минуты, вывод сжимается по рабочему дню (30h при 8-час. дне → «3д 6ч»).
  - **Стори-поинты** (Фибоначчи / футболки / линейная) и **кастом** (метка) — число без конверсии.
  - Роллап = сумма оценок подзадач (показывается «Σ …», когда у задачи есть дети). Поверхности: строка
    «Оценка» в TaskModal, чип на карточке, «⏱ N» в лейн-хедерах таймлайна/Ганта, редактор единиц из
    контекст-меню проекта и меню воркспейса. **Оценка ≠ трекинг времени** (учёт затраченного — отдельный
    поздний кандидат). Смешанные единицы на одной доске не делаем: доска = один проект = одна единица.

- **GitLab фаза B — write-back — ✅ MVP СДЕЛАНО** (backend 0.53.0 / web 0.92.0, миграция **0032**,
  2026-06-23). Opt-in per-integration (`gitlab_integrations.writeback` jsonb, всё выкл по умолчанию);
  пушим три вида: **state** (вход/выход из колонки «Готово» → close/reopen issue), **priority**
  (смена приоритета → swap метки `P:` через `add_labels`/`remove_labels`, только если маппинг 1:1),
  **comment** (комментарий Tessera → note в issue; note-id пишется на комментарий → следующий pull
  дедупит). Транспорт — REST (pull остаётся GraphQL). Async-outbox `gitlab_writebacks` + воркер
  `RunGitlabWriteBackWorker` (зеркало notification-outbox: claim SKIP LOCKED, квадратичный backoff,
  5 попыток) — пуш не блокирует мутацию, переживает падение GL. Loop-guard: enqueue только на
  user-мутациях (pull идёт отдельным путём), не пушим значение, которое в GL уже есть (`gl_last_state`
  + контент-хэши), после пуша ре-фетч issue переписывает снапшот. Пуш — PAT владельца интеграции
  (нужен scope `api`). Статус — **только open/close** (инверс колонка→`S:` лоссится; редактор явных
  привязок `column_label_bindings` отложен, шов пустой). См. [[project-gitlab-writeback]].
- **GitLab журнал синхронизации — ✅ СДЕЛАНО** (2026-06-23, backend 0.54.0 / web 0.93.0, миграция 0033).
  Видимая пользователю история прогонов pull+push воркера с диффом до/после на каждое действие.
  Сплит-кнопка «Синхронизировать» в GitLab-модалке (дропдаун → «Журнал синхронизации») открывает
  `GitLabJournalModal` (мастер-деталь): слева прогоны, справа дифф действия; у упавшей push-доставки —
  «Повторить». Свои таблицы `gitlab_sync_runs`/`gitlab_sync_actions` (слабая связанность, ядро не
  трогается); recorder = аккумулятор `syncJournal`; reconcile-запросы отдают дельты. Эндпоинты
  `sync-runs` / `.../actions` / `.../retry`. Подробности — [[project-gitlab-sync-journal-next]].
  Открытый хвост: визуальный прогон в живом UI (headless для NModal ненадёжен).
  **Android-паритет — ✅ СДЕЛАНО** (android 0.35.0, см. раздел 15).
- **GitLab фаза B+ — шаги 1/2/3 ✅ СДЕЛАНО** (2026-06-27, порядок `1→3→2`; детали и гочи —
  [[project-gitlab-phase-b-plus-plan]]). На готовом outbox/worker/журнале, каждый шаг зеркалит
  priority-путь. **Шаг 4 — на паузе (пользователь тестирует 1/2/3 против живого GL перед стартом).**
  1. **Теги + due → GL** — ✅ (backend 0.56 / web 0.99 / android 0.37): флаги `push_labels`/`push_due`
     (jsonb, без миграции); реконсайл меток только в тег-неймспейсах; только `due_date` issue
     (start у GL-issue нет).
  2. **timeEstimate ↔ задача (двусторонне)** — ✅ (backend 0.58 / web 0.101 / android 0.39, миг 0036):
     pull+push, только при единице оценки «время» (`estimation_unit` в ответе интеграции → тогл
     disabled иначе); `estimate_overridden`.
  3. **Members-sync → ассайни** — ✅ (backend 0.57 / web 0.100 / android 0.38, миг 0035): таблица
     `gitlab_project_members`, объединённый пикер Tessera+GL, `task_gitlab_assignees` writable
     (`source`), write-back `assignees` через `gl_user_id`. **OAuth-блокер ассайни снят.**
  4. **Создание issue из таски** (крупный, СЛЕДУЮЩИЙ): opt-in тогл в модалке задачи (как «Выполнено»),
     автор = owner PAT, guard от петель; **issue_templates** тянем из репо (`.gitlab/issue_templates/*.md`)
     в свой стор → префилл описания.
  - **OAuth/SSO** — не сейчас, но **ближний бэклог** (универсальный маппинг + атрибуция), не в долгий
    ящик. Ассайни от него больше не зависят (шаг 3).
  - Остаток прежнего бэклога: **title/description** (риск затирания, gated `push_title_desc`),
    редактор `column_label_bindings` (колонка→метка `S:`), webhooks (вместо/в дополнение к polling).
- **Android background push (FCM)** — device-канал поднимает уведомления только **пока приложение
  открыто** (C2); напоминания — локальный `AlarmManager`. Фоновый push при закрытом приложении (FCM)
  ещё не сделан — следующий кандидат для надёжной доставки (аналог Telegram-доставки в budget-go).
- **`chore(backend)` lint-cleanup — ✅ СДЕЛАНО** (2026-06-23). Было ~44 issue (golangci-lint обрезал
  повторы — revive exported/package-comments, `max` shadow builtin ×6, errcheck ×3, gocritic
  exitAfterDefer): добавлены doc-комментарии к экспортам (collab/attachments/auth/boards/version/ws/
  realtime/embed/token) и package-комментарии (main/config/middleware/database), `max`→`maxPos`,
  `cmd/migrate` вынесён в `run() error` (deferred Close теперь срабатывает и проверяется). Без
  изменения поведения, без бампа. `make lint-backend` = 0 issues.
- **Редактирование префиксов пользовательских тегов в TagManager** — ✅ **СДЕЛАНО** (web 0.98.0,
  2026-06-26). Секция «Имена префиксов» в поповере «Теги» (`TagManager.vue`): строка на каждый
  префикс, встречающийся среди тегов проекта (через `buildTagGroups`), поле имени, merge-save на
  blur/Enter в `projects/:id/tag-prefixes` (mig 0026) — зеркалит логику GitLab-модалки, но доступно
  и не-GitLab проектам. Проверено через CDP (рендер/правка/persist). См. [[project-tag-prefix-names]].
- **Виртуализация колонок на web** (kanban perf >100 карточек) — ✅ **СДЕЛАНО** (web 0.97.0,
  2026-06-26). IntersectionObserver-оконность: карточки дальше ~800px от вьюпорта сворачиваются в
  плейсхолдер их измеренной высоты, тяжёлый `TaskCard` монтируется только около вьюпорта (~4–22 из
  150 в DOM). DnD не тронут (обёртка `<div>` сохраняется → индексы vuedraggable/before-after
  идентичны, меняется лишь содержимое обёртки). Видимость — по реальному положению во вьюпорте,
  высота плейсхолдера — `ResizeObserver`-замером → нет рывков, низ всегда достижим; свопы заморожены
  на drag. Проверено через CDP (рендер/скролл до #150/реордер). См. [[project-kanban-perf-next]].
- **Деплой: GHCR + `docker pull`** — сейчас релиз = build-on-dev → tar → scp → `docker load`. Перейти на
  registry-pull, когда понадобится.
- **GitLab attachment на старых GL для приватных проектов** — текущий стрим-фолбэк (0.33.1) тянет
  публичные uploads; приватные на GL <17.4 без сессии (только PAT) сервер забрать не может —
  ограничение API GitLab, отметить если всплывёт.

## Сверка с budget-go (общие проектные аспекты)

Tessera зеркалит budget-go по структуре и конвенциям. Что есть у budget и стоит держать
в виду как возможные общие аспекты (НЕ обязательства — отметить осознанность):

- **Доставка напоминаний через Telegram.** В budget push идёт через отдельный `telegram_bot/`.
  В Tessera это закрыто иначе и шире (раздел 9): уведомления (включая due/reminder) роутятся через
  `internal/notify` в внешние каналы, Telegram — через **shoutrrr** (плюс slack/discord/ntfy/…),
  без отдельного бота. Остаётся только **Android background push (FCM)** при закрытом приложении.
- **`docs/`** (budget: `api/`, `E2E_PLAN.md`, `RPI_DEPLOY.md`, `TELEGRAM_BOT.md`) и
  **`CONTRIBUTING.md`** — у Tessera версионные конвенции живут в `CLAUDE.md`; отдельный
  CONTRIBUTING/docs-сайт пока не делали (Фаза 7 сознательно прагматична).
- **RPi / multi-arch деплой + systemd** (budget `deploy/systemd`, `docker-compose.rpi.yml`) —
  у Tessera не делали; добавить, если появится RPi-таргет.
- **Общий keystore:** Android-релизы Tessera подписываются ТЕМ ЖЕ ключом, что budget
  (`/home/msdnna/budget.jks`, alias `budget`) — см. скилл **tessera-android-release**.
- **Per-component VERSION + единый CHANGELOG + bump-скрипт** — уже на месте, идентично budget.
