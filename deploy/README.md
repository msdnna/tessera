<p align="right"><b>Русский</b> · <a href="README.en.md">English</a></p>

# Tessera — продакшн-развёртывание

Развёртывание в один хост через Docker за Caddy (автоматический HTTPS). Образы
собираются на dev-машине и едут на сервер тарболом — на сервере нет ни исходников,
ни сборочного тулчейна. (Позже: публикация в GHCR и `docker pull` вместо тарбола.)

```
┌────────── VDS (Ubuntu 24.04) ───────────┐
│  Caddy :80/:443  ──TLS──► frontend(nginx)│
│                            │ /api ─► backend (distroless) ─► postgres │
│  наружу открыты только 80/443; БД и backend — внутренние              │
└──────────────────────────────────────────┘
```

## Файлы

| Файл | Где выполняется | Назначение |
|------|-----------------|------------|
| `build-and-save.sh` | dev-машина | сборка прод-образов → `dist/*.tar.gz` |
| `server-bootstrap.sh` | сервер (однократно) | харденинг ОС + установка Docker |
| `docker-compose.yml` | сервер | прод-стек на готовых образах |
| `Caddyfile` | сервер | TLS-эдж + реверс-прокси |
| `.env.example` | сервер | скопировать в `.env`, заполнить секреты |

## Первое развёртывание

**0. DNS** — направьте A-запись (`tessera.example.com`) на IP VDS до шага 4
(Caddy нужно, чтобы имя резолвилось, для выпуска сертификата).

**1. Подготовка сервера** (на VDS, под sudo-пользователем):
```bash
scp deploy/server-bootstrap.sh user@server:/tmp/
ssh user@server 'sudo bash /tmp/server-bootstrap.sh'
# перезайдите в сессию, чтобы применилась группа docker
```

**2. Сборка и доставка образов** (dev-машина):
```bash
bash deploy/build-and-save.sh
scp deploy/dist/tessera-images-*.tar.gz user@server:/opt/tessera/
scp deploy/{docker-compose.yml,Caddyfile,.env.example} user@server:/opt/tessera/
```

**3. Настройка** (сервер, `/opt/tessera`):
```bash
cp .env.example .env && chmod 600 .env
# сгенерировать секреты:
#   openssl rand -hex 32      (JWT_SECRET, ENCRYPTION_KEY)
#   openssl rand -base64 24   (POSTGRES_PASSWORD)
nano .env          # задать DOMAIN, ACME_EMAIL, PUBLIC_URL, секреты, теги образов
docker load -i tessera-images-*.tar.gz
```

**4. Запуск**:
```bash
docker compose up -d
docker compose exec backend /migrate      # применить миграции БД
docker compose logs -f                     # смотреть, как Caddy берёт сертификат
```

Откройте `https://tessera.example.com` и **сразу зарегистрируйтесь** — первый
пользователь становится администратором.

## Обновление до новой версии

```bash
# dev-машина
bash deploy/build-and-save.sh
scp deploy/dist/tessera-images-*.tar.gz user@server:/opt/tessera/
# сервер
docker load -i tessera-images-*.tar.gz
# поднять теги BACKEND_IMAGE / FRONTEND_IMAGE в .env до новых версий
docker compose up -d
docker compose exec backend /migrate       # если релиз добавил миграции
```

## Бэкапы (делайте — без них конфиденциальность неполна)

```bash
# дамп БД (cron, например ночью)
docker compose exec -T postgres pg_dump -U tessera tessera | gzip > backup-$(date +%F).sql.gz
# вложения лежат в томе backend_uploads — его тоже в бэкап.
```
Шифруйте дампы (`gpg`) и храните их **вне хоста** (объектное хранилище / другой
сервер). Периодически проверяйте восстановление. Перед каждым обновлением снимайте
снапшот диска VDS.

## За корпоративным прокси

Если Tessera стоит за прокси/балансировщиком, которым вы не управляете (корпоративный
эдж, API-gateway), именно он — а не встроенные Caddy/nginx — определяет задержки и
работу WebSocket. Чтобы живая доска и быстрые ответы работали и там, нужно:

- **WebSocket-upgrade** на `/api/ws`: пробрасывать заголовки `Upgrade` / `Connection`
  и ходить к апстриму по HTTP/1.1. Без этого realtime-сокет не подключится, а клиенты
  скатываются в тихое устаревание данных + постоянные переподключения.
- **Idle-timeout ≥ 60с** на этом маршруте (лучше минуты). Бэкенд пингует каждые 25с,
  чтобы держать сокет открытым; прокси, обрывающий простаивающие соединения быстрее,
  будет раз за разом ронять живые обновления доски.
- **Сжатие ответов** (`gzip`/`br`) для `application/json`, ЛИБО пропускать собственный
  gzip бэкенда нетронутым (не срезать `Accept-Encoding` на входе и `Content-Encoding`
  на выходе). Tessera сама gzip-ует свой JSON; пейлоады доски / журнала синка
  сжимаются ~в 10 раз — это разница между суб-секундной и многосекундной загрузкой на
  узком канале.
- **Keep-alive до апстрима**: переиспользовать соединения к контейнеру фронтенда, а не
  открывать новый TCP + TLS на каждый запрос — открытие доски выстреливает ~10 вызовов
  разом, и посоединенный setup иначе доминирует.

Встроенный `frontend/nginx.conf` уже делает всё перечисленное для встроенного пути —
повторите эти настройки на внешнем прокси.

## Тюнинг Postgres под хост

`docker-compose.yml` едет с консервативными настройками Postgres под ~2ГБ-хост.
На более крупном VDS поднимите их в `.env` (затем `docker compose up -d postgres`):

```
PG_SHARED_BUFFERS=1GB           # ~25% RAM
PG_EFFECTIVE_CACHE_SIZE=3GB     # ~50-75% RAM
PG_WORK_MEM=64MB
PG_MAINTENANCE_WORK_MEM=512MB
PG_RANDOM_PAGE_COST=1.1         # SSD; оставляйте 4 только для «блинов»
```

## Модель безопасности (встроено)

- У Postgres и backend **нет host-портов** — недостижимы из интернета by design.
- Образ backend — **distroless, non-root**, статический бинарь.
- `APP_ENV=production` **fail-closed** без `JWT_SECRET` / `ENCRYPTION_KEY` /
  `DATABASE_URL` / `PUBLIC_URL`.
- TLS повсюду через Caddy (авто-обновление Let's Encrypt).
- SSH: только по ключу, root-логин выключен, fail2ban; ufw открывает лишь 22/80/443.
