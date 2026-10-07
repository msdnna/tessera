---
bump: minor
---
- **feat(api): общие представления доски, «по умолчанию», порядок и переименование; вид новой доски (#2970: #2988, #3015, #3016, #3017).**
  Миграция `0079_board_views_shared`: у `board_views` — `scope` (`user` | `board`, у общих
  `user_id` пуст, автор — в `created_by`), `is_default` (одна звезда в каждом списке, личная
  перекрывает общую), `position` (float-midpoint), уникальность имени — в пределах своего
  списка; у `boards` — `default_layout`. `GET /boards/:id/views` отдаёт свои + общие с полями
  `scope`/`is_default`/`position`/`creator_name`/`editable`; `POST` принимает `scope` и
  `is_default` (по-прежнему upsert по имени — старые клиенты не ломаются); новый
  `PATCH /views/:id` — имя, конфиг, звезда, `scope`, перенос (`before_id`/`after_id`).
  Общие создают и правят владелец и админы пространства; изменения общих рассылаются
  событием `board_views.changed`. `POST /projects/:id/boards` и `PATCH /boards/:id`
  принимают `default_layout`.
