# ROADMAP — Tessera

Источник истины для «что сделано / что дальше». Обновлять по ходу работы (и в том же
изменении правда важнее красоты). Детали реализации — в коде, CHANGELOG и auto-memory.

Текущие версии — `make version`. На момент составления: backend `0.17.0` ·
frontend `0.43.0` · android `0.3.1`.

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
| 8 | **Android** (Kotlin Compose, online-first, повторяет web-моб; self-update) | 🔶 активная разработка |
| 9 | Деплой (multi-stage Dockerfile, nginx-прокси, distroless backend, prod-compose) | ✅ |
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
- **Бэклог фазы B+:** write-back (action-bindings + loop-guard через снапшот-хэши, уже в схеме);
  webhooks; OAuth/SSO.

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

## Сверка с budget-go (общие проектные аспекты)

Tessera зеркалит budget-go по структуре и конвенциям. Что есть у budget и стоит держать
в виду как возможные общие аспекты (НЕ обязательства — отметить осознанность):

- **Доставка напоминаний через Telegram-бот.** В budget push идёт через `telegram_bot/`.
  В Tessera напоминания пока только хранилище+UI; доставка завязана на Android (self-update
  уже есть; push-нотификации — следующий кандидат). Telegram-бот для Tessera пока не планируется.
- **`docs/`** (budget: `api/`, `E2E_PLAN.md`, `RPI_DEPLOY.md`, `TELEGRAM_BOT.md`) и
  **`CONTRIBUTING.md`** — у Tessera версионные конвенции живут в `CLAUDE.md`; отдельный
  CONTRIBUTING/docs-сайт пока не делали (Фаза 7 сознательно прагматична).
- **RPi / multi-arch деплой + systemd** (budget `deploy/systemd`, `docker-compose.rpi.yml`) —
  у Tessera не делали; добавить, если появится RPi-таргет.
- **Общий keystore:** Android-релизы Tessera подписываются ТЕМ ЖЕ ключом, что budget
  (`/home/msdnna/budget.jks`, alias `budget`) — см. скилл **tessera-android-release**.
- **Per-component VERSION + единый CHANGELOG + bump-скрипт** — уже на месте, идентично budget.
