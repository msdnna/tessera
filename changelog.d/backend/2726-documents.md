---
bump: minor
---

- **feat(api): раздел «Документы» — хранилище и CRUD (#2726).**
  Новая таблица `documents`: блочное содержимое одним деревом в `content jsonb`,
  вложенность через `parent_id`, привязка к проекту через `project_id`.
  Ручки `POST/GET /workspaces/:id/documents`, резолв по слагу
  `GET /workspaces/:id/documents/by-slug/:slug` (отдаёт `workspace_id`, чтобы
  прямая ссылка переключала workspace), `GET/PATCH/DELETE /documents/:id`.
  Удаление контейнера с вложенными отвечает `409` и требует `?recursive=true`;
  перенос документа внутрь собственного поддерева отклоняется. Документы
  участвуют в поиске по workspace и переезжают вместе с проектом при его
  переносе в другой workspace.
