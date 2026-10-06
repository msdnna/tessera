---
bump: patch
---
- **fix(web): origin приложения отдаётся с заголовками безопасности (#2932).**
  Раньше их не было вовсе: `Content-Security-Policy`/`nosniff` ставил только бэкенд на своих
  файловых маршрутах, а Caddy впереди — обычный `reverse_proxy`, так что SPA-шелл приходил
  в браузер голым. Теперь каждый маршрут, который nginx обслуживает сам, несёт
  `frame-ancestors 'none'` + `X-Frame-Options: DENY` (встраивание в чужой iframe),
  `X-Content-Type-Options: nosniff` и `Referrer-Policy: strict-origin-when-cross-origin`.
  Набор вынесен в `frontend/security-headers.conf` и включается в каждый непроксирующий
  `location`: `add_header` в nginx не наследуется — location со своим `add_header` молча
  теряет все объявленные выше, а таких здесь четыре (`Cache-Control` PWA-шелла), и один из
  них — `= /index.html`, куда приходит `try_files` с главной страницы. `/api/` и `/livekit/`
  оставлены своим апстримам, чтобы не дублировать CSP бэкенда.
