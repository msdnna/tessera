---
name: tessera-e2e
description: Run a safe end-to-end check of the Tessera backend against the tessera_test database. Use to verify backend behavior live (auth, endpoints, WS) without touching the production tessera DB. Covers the build/run/port and bash/json gotchas of this environment.
---

# tessera-e2e

Безопасная живая проверка бэкенда Tessera против тестовой БД `tessera_test`.
**Боевую `tessera` не трогаем** (никаких TRUNCATE/DROP/`down -v`).

## Критичные гочи этой среды
- **БД только `tessera_test`.** Боевая `tessera` (том `tessera_postgres_data`, контейнер
  `tessera-postgres`, tessera/tessera, :5432) — священна.
- **Порт 8092+.** На :8090 может висеть зомби со старым кодом; на :8080 — budget. Для
  e2e бери свободный 8092+. При надобности освободить: `fuser -k 8092/tcp`.
- **Бинарь:** только `go1.25.9` (не системный go).
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
  go1.25.9 run ./cmd/migrate
# версия схемы:
PGPASSWORD=tessera psql -h localhost -U tessera -d tessera_test -tAc \
  "select max(version) from schema_migrations;"
```

## Сборка и запуск тест-инстанса
```bash
cd backend && go1.25.9 build -o /tmp/tessera-bin .
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
