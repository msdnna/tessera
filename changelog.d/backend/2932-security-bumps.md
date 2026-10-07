---
bump: patch
---
- **chore(api): закрыты все достижимые уязвимости govulncheck (#2932).**
  Go toolchain `1.25.9` → `1.25.14` (13 находок в stdlib: `net/http`, `crypto/tls`,
  `net/mail`, `crypto/x509`, `encoding/xml`, …), `quic-go` `v0.59.0` → `v0.59.1`
  (GO-2026-5676), `golang.org/x/text` `v0.35.0` → `v0.39.0` (GO-2026-5970),
  `golang.org/x/sync` `v0.20.0` → `v0.21.0` (backend и mcp). `govulncheck` после
  бампов: 0 достижимых (было 15). Закреплённая версия Go в Makefile и документации —
  `go1.25.14`.
