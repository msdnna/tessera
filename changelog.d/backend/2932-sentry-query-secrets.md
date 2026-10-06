---
bump: patch
---
- **fix(api): секреты из query больше не уезжают в Sentry (#2932).** `middleware/accesslog.go`
  был написан ровно затем, чтобы `?code=…&state=…` не попали в лог контейнера — но лог был
  только одним из двух каналов, которыми query-строка покидает процесс. SDK кладёт в событие
  `QueryString: r.URL.RawQuery` **безусловно** (sentry-go `interfaces.go`): `SendDefaultPII`,
  выключенный по умолчанию, гейтит только заголовки, куки и адрес пира, но не URL. Хуков
  `BeforeSend`/`BeforeSendTransaction` не было, а `SENTRY_TRACES_SAMPLE_RATE` по умолчанию
  равен **1.0** — то есть при включённом Sentry транзакция каждого запроса несла сырой query:
  живой authorization code и `state` OAuth-колбэка (`oauth.go`), HMAC-подпись `sig`
  подписанных ссылок на вложения и ассеты документов/GitLab. Теперь список чувствительных
  параметров и сама замена живут в `internal/observability` — один источник и для лога, и
  для телеметрии (`sig` в список добавлен), и оба хука проставлены. `Request.URL` не трогаем:
  SDK собирает его из схемы, хоста и пути, без query.
