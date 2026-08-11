---
bump: minor
---
- **feat(auth): refresh-токен в httpOnly-cookie для веба (#2684).**
  Клиент, пришедший с заголовком `X-Auth-Mode: cookie`, получает refresh-токен
  в cookie `tessera_refresh` (`HttpOnly`, `SameSite=Strict`, `Path=/api/auth`,
  `Secure` — по схеме публичного URL), а в теле ответа токена больше нет: XSS
  до него не дотянется. `/auth/refresh` принимает токен из cookie или из тела,
  cookie в приоритете. Для Android, desktop и скриптов не изменилось ничего —
  без заголовка ответ прежний, с токеном в теле.
- **feat(auth): ручка `POST /auth/logout` с ревокацией refresh-токена (#2684).**
  Раньше выхода на сервере не было вовсе: клиент чистил localStorage, а строка
  в `refresh_tokens` оставалась валидной все 30 дней. Теперь токен (из cookie
  или тела) ревокается, cookie затирается. Идемпотентна: неизвестный или
  протухший токен — тоже `204`.
- **fix(auth): OAuth-колбэк больше не светит refresh-токен в URL (#2684).**
  Веб-ветка `GET /auth/gitlab/callback` кладёт refresh в cookie, во фрагменте
  остаётся только access-токен. Фрагмент не уходит на сервер, но оседает в
  истории браузера и в `window.location`, откуда его читает тот же XSS.
  Мобильная ветка (custom-scheme) не изменилась.
