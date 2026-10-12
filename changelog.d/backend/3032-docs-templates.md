- **feat(api): документы и шаблоны v2 — этап документа, повестка встречи, категории шаблонов, шаблоны досок (#3032).**
  Миграция `0090`: `documents.milestone_id` и `documents.conference_id` (SET NULL), `document_templates.category`
  (sprint · product · engineering · meetings, '' — без категории). `PUT /documents/:id/milestone` и
  `PUT /documents/:id/conference` (оба конца — в пространстве документа, `updated_at` не двигается),
  `milestone_id`/`conference_id` при создании документа, `GET /milestones/:id/documents`,
  `GET /workspaces/:id/conference-documents`; перенос проекта снимает связи, оказавшиеся по разные стороны
  границы. Категория — в создании/правке/списке шаблонов. `POST /projects/:id/boards` принимает
  `template`: `sprint` (Канбан спринта), `bugs` (Баг-трекер, вид «Список»), `roadmap` (Роадмап, вид
  «Таймлайн») — свои колонки с `name_key` и теги проекта на языке создателя.
