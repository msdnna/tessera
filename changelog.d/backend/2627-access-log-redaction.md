---
bump: patch
---
- **fix(api): не писать OAuth-код и токены в access-лог (#2627).** Роутер
  поднимался через `gin.Default()`, а штатный `gin.Logger` печатает полный путь
  **вместе с query** — то есть `GET /api/auth/gitlab/callback?code=…&state=…`
  ложился в stdout и оттуда в логи контейнера в открытом виде. Теперь
  `gin.New()` + `gin.Recovery()` + собственный `middleware.AccessLog()`: та же
  строка доступа, но значения параметров `code`, `state`, `token`, `secret`,
  `access_token`, `refresh_token` заменяются на `***`. Остальные параметры
  проходят как есть и в исходном порядке, чтобы лог оставался пригодным для
  разбора маршрутизации.
