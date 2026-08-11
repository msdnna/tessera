---
bump: minor
---
- **feat(api): явные флаги сброса сохранённых секретов (#2691).**
  `PUT /admin/oauth/gitlab` принимает `clear_client_secret` / `clear_service_token`,
  `PUT /notification-channels/:id` — `clear_secret`: стирают сохранённое значение
  (пустая строка по-прежнему значит «не менять»). Непустое значение в том же запросе
  имеет приоритет над флагом. Канал с обязательным секретом (telegram/shoutrrr)
  отвечает 400 на попытку очистки.
