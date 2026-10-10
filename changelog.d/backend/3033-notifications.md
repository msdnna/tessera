- **feat(api): уведомления v2 (#3033).**
  Страница «Все уведомления» (#3049): `GET /notifications/page?filter=&cursor=&limit=`
  читает ленту по курсору (created_at, id) с фильтром на сервере (all · unread ·
  mentions · assigned · gitlab) и отвечает `{items, next_cursor}`;
  `GET /notifications/unread-count` дополнительно считает непрочитанные по фильтрам
  (`by_group`). Миграция 0091.
