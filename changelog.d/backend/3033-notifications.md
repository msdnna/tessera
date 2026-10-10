- **feat(api): уведомления v2 (#3033).**
  Страница «Все уведомления» (#3049): `GET /notifications/page?filter=&cursor=&limit=`
  читает ленту по курсору (created_at, id) с фильтром на сервере (all · unread ·
  mentions · assigned · gitlab) и отвечает `{items, next_cursor}`;
  `GET /notifications/unread-count` дополнительно считает непрочитанные по фильтрам
  (`by_group`). Миграция 0091.
  Исключения тихих часов (#3050): `quiet_allow_urgent` в `/notification-prefs` —
  «Только срочное и упоминания»: упоминания, задачи с приоритетом «Срочный» и
  встреча, которая вот-вот начнётся, уходят во внешние каналы и в тихие часы.
