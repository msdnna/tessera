- **fix(api): Go 1.26.9, golang.org/x/net 0.61 и x/text 0.43 — закрыты 14 новых advisory
  govulncheck (#3122).** net/http (HTTP/1 и HTTP/2: рассинхронизация после CONNECT, flow
  control, HPACK, лимит Range), net/textproto и mime/multipart, crypto/tls (ECH), паника в
  `x/text/secure/precis`. Ветка 1.25 больше не поддерживается; 1.27 не взят — gosec 2.29.0
  (последний релиз) не читает export data стандартной библиотеки go1.27. Тулчейн пинится
  в `go.mod` (backend, mcp), `Makefile` (`GO ?= go1.26.9`), образах `golang:1.26-alpine`;
  baseline сканов: −25 ушедших находок govulncheck.
