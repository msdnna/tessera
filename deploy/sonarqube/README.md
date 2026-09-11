# Стенд SonarQube

Витрина качества кода для Tessera: тренд, дубли, покрытие, code smells по всему
монорепо. Поднимается **по требованию** — постоянно не крутится, потому что
встроенный Elasticsearch держит около 2 ГБ RAM.

Стенд живёт в отдельном compose-проекте (`-p tessera-sonar`) и в отдельной сети,
поэтому `make up` его не поднимает, а `make down` не гасит.

## Быстрый старт

```bash
make sonar-up        # поднять и дождаться status=UP (первый запуск — минуты)
make sonar-scan      # выпустить токен (один раз) и проанализировать рабочее дерево
make sonar-down      # погасить (данные остаются)
```

Логин в UI на `http://127.0.0.1:9001` — `admin` с паролем из
`~/.tessera/sonar-admin-password`.

| Цель | Что делает |
|---|---|
| `make sonar-up` | Поднимает стенд и **ждёт готовности** (`tools/sonar-wait.sh`) |
| `make sonar-down` | Гасит контейнеры, данные в `$SONAR_HOME` не трогает |
| `make sonar-logs` | Хвост логов |
| `make sonar-token` | Меняет дефолтный пароль admin и выпускает токен анализа |
| `make sonar-scan` | Прогоняет `sonar-scanner-cli` в контейнере по `sonar-project.properties` |

`make sonar-token` идемпотентен и вызывается из `sonar-scan` сам: действующий
токен переиспользуется (проверяется запросом к стенду, а не наличием файла).
Дефолтный пароль `admin/admin` меняется на случайный — он не зашит в
репозиторий, а сохраняется в `~/.tessera/sonar-admin-password` (0600).

## Где что лежит

Данные — **не в репозитории и не в docker-томах**, а bind-каталогами в
`$SONAR_HOME` (по умолчанию `~/sonarqube`), как стенд Sentry:

```
~/sonarqube/db          # Postgres стенда
~/sonarqube/data        # индексы и БД-файлы Sonar
~/sonarqube/extensions  # плагины
~/sonarqube/logs        # логи
```

Снести стенд целиком: `make sonar-down && rm -rf ~/sonarqube`.

Переменные: `SONAR_HOME`, `SONAR_PORT` (по умолчанию **9001**),
`SONAR_DB_PASSWORD`, `SONAR_IMAGE_TAG`. Порт слушается только на loopback.

**Почему 9001, а не 9000:** 9000 на этой машине держит стенд Sentry. Заранее
это не проверяется: `ss` на хосте не видит опубликованные docker-порты вообще
(докер живёт в своём netns), а `curl` к закрытому порту здесь не отвечает
`refused`, а висит до таймаута — то есть и свободный, и занятый порт выглядят
одинаково. Единственный честный ответ даёт сам `docker … up`.

## Ветки: почему `tessera-scratch`

**Community Build анализирует ровно одну ветку на проект** — веток и
PR-декорации в бесплатной редакции нет. Поэтому:

* `develop` → проект **`tessera`**, копит историю и тренд;
* любая другая ветка → проект **`tessera-scratch`**, перезаписываемая
  песочница.

`tools/sonar-scan.sh` выбирает ключ сам по текущей ветке; переопределить —
`make sonar-scan PROJECT_KEY=…`.

Из этого же следует, что Sonar — **витрина, а не гейт**: блокирующие проверки
в ночном цикле и CI делают быстрые CLI (`make scan`, см. #2936), они работают
по feature-ветке и без поднятого сервера.

## Покрытие

Импортируется, если отчёты уже сгенерированы; иначе анализ просто пройдёт без
метрики покрытия (`tools/sonar-scan.sh` не передаёт несуществующие пути).

```bash
make test-backend-cover    # backend/coverage.out
make test-frontend-cover   # frontend/coverage/lcov.info
make test-android-cover    # android/.../jacocoTestReport.xml
make sonar-scan
```

## Требования и грабли

* **`vm.max_map_count` ≥ 524288** — иначе Elasticsearch внутри Sonar не
  стартует, а контейнер уходит в рестарт-петлю. На этой машине (WSL2) уже
  `1048576`, проверить: `cat /proc/sys/vm/max_map_count`. Если мало —
  `sudo sysctl -w vm.max_map_count=524288` и строка в `/etc/wsl.conf`.
* **Первый `sonar-up` долгий**: образ ~1 ГБ, плюс миграция БД и старт
  Elasticsearch. `make sonar-up` ждёт `status=UP` и не возвращает ноль на ещё
  мёртвом стенде — на `DB_MIGRATION_RUNNING` ожидание продолжается.
* **Rust (`desktop/src-tauri`)**: анализатор Rust в Community Build появился не
  сразу; если в отчёте по desktop пусто — это ожидаемо, качество Rust-кода
  держит `make lint-desktop` (clippy `-D warnings`).
* `backend/db/**` исключён из анализа целиком: это вывод `sqlc`, править его
  руками нельзя.
