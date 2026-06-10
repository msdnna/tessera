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

### 3. Бизнес-фичи — GitLab self-hosted Issues integration — ⏭ СЛЕДУЮЩЕЕ
- Тянуть рабочие задачи/issues из self-hosted GitLab прямо в Tessera вместо дублей.
  Дизайн TBD (направление синка, маппинг issues↔tasks).

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
