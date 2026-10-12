---
name: tessera-e2e
description: Run a safe end-to-end check of the Tessera backend against the tessera_test database, and run the automated e2e suites (backend black-box, web Playwright). Use to verify backend behavior live (auth, endpoints, WS) without touching the production tessera DB, or before running/writing Playwright specs. Covers the build/run/port, Playwright browser install and bash/json gotchas of this environment.
---

# tessera-e2e

Безопасная живая проверка бэкенда Tessera против тестовой БД `tessera_test`.
**Боевую `tessera` не трогаем** (никаких TRUNCATE/DROP/`down -v`).

## Критичные гочи этой среды
- **БД только `tessera_test`.** Боевая `tessera` (том `tessera_postgres_data`, контейнер
  `tessera-postgres`, tessera/tessera, :5432) — священна.
- **Порт 8092+.** На :8090 может висеть зомби со старым кодом; на :8080 — budget. Для
  e2e бери свободный 8092+. При надобности освободить: `fuser -k 8092/tcp`.
- **Бинарь:** только `go1.26.9` (не системный go).
- **Скрипты — `bash <<'SCRIPT'`** (дефолтный zsh ломается на uuid в арифметике).
- **JSON парсить `python3`**, не jq-бинарём.

## Подготовка
```bash
# Postgres поднят? (контейнер tessera-postgres)
docker compose up -d postgres

# tessera_test существует? если нет — создать (одноразово):
PGPASSWORD=tessera psql -h localhost -U tessera -d tessera -tAc \
  "SELECT 1 FROM pg_database WHERE datname='tessera_test'" | grep -q 1 || \
  PGPASSWORD=tessera createdb -h localhost -U tessera tessera_test

# применить миграции на tessera_test
cd backend && DATABASE_URL="postgres://tessera:tessera@localhost:5432/tessera_test?sslmode=disable" \
  go1.26.9 run ./cmd/migrate
# версия схемы:
PGPASSWORD=tessera psql -h localhost -U tessera -d tessera_test -tAc \
  "select max(version) from schema_migrations;"
```

## Сборка и запуск тест-инстанса
```bash
cd backend && go1.26.9 build -o /tmp/tessera-bin .
PORT=8092 UPLOAD_DIR=/tmp/tessera-uploads JWT_SECRET=devsecret \
  DATABASE_URL="postgres://tessera:tessera@localhost:5432/tessera_test?sslmode=disable" \
  /tmp/tessera-bin &
sleep 1
curl -s localhost:8092/api/health    # {"ok":true,"app":"tessera"}
```

## Прогон сценария (пример формы)
```bash
bash <<'SCRIPT'
set -e
B=http://localhost:8092/api
# register (первый юзер = admin) → токен
REG=$(curl -s -X POST $B/auth/register -H 'content-type: application/json' \
  -d '{"email":"e2e@test.local","password":"pass1234","name":"E2E"}')
TOKEN=$(python3 -c "import sys,json;print(json.load(sys.stdin)['access_token'])" <<<"$REG")
# authed-вызов
curl -s $B/workspaces -H "authorization: Bearer $TOKEN" | python3 -m json.tool
SCRIPT
```
Авторизация: ответ `{access_token, refresh_token, user{...}}`; защищённые ручки —
`Authorization: Bearer <access_token>`. Проверяй и happy-path, и authz (ожидай 403 для
не-членов workspace). WS — апгрейд на `/api/ws` (101).

## Уборка
```bash
fuser -k 8092/tcp 2>/dev/null || true     # погасить инстанс
```
Данные в `tessera_test` можно оставить или почистить — это тестовая БД. Боевую `tessera`
не касаться.

## Автоматические e2e-наборы
- **Backend black-box** (`backend/e2e/`, build-tag e2e): `make test-e2e-backend` — настоящие
  бинари + своя одноразовая БД `tessera_e2e_<runId>` (дропается в конце; делает `migrate down`,
  поэтому НЕ на общей `tessera_test`). `make test-e2e-backend-docker` — то же + образ (минуты).
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
- **БД агентов** `tessera_test_<N>` (своя на агента/задачу через `TEST_DATABASE_URL`) — после
  слияния задачи удалять: `docker exec tessera-postgres dropdb -U tessera tessera_test_<N>`.

## Android (deprecated с 2026-10-12 — новых спек не писать)
**Android e2e (#2711) — два яруса, харнесс общий** (`android/app/src/e2eShared`, он же
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
