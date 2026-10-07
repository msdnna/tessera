---
bump: minor
---
- **feat(api): этапы — дата закрытия, роллапы проекта, журнал смены этапа с id, метрики этапа (#2971: #2992, #3027).**
  Миграция `0080_milestone_metrics_tag_source` (аддитивная, повторяемая): `milestones.closed_at`
  (ставится при закрытии, снимается при открытии; уже закрытым — бэкфилл по `updated_at`),
  `tags.source` (`user` | `gitlab`, бэкфилл по `task_tags.source`), частичный индекс событий
  смены этапа. `GET /projects/:id/milestones` отдаёт `closed_at`, `gl_iid` и роллапы
  (`task_count`, `done_count`, `estimate_sum`, `done_estimate`). Записи журнала `milestone` /
  `milestone_removed` несут `milestone_id` (и `from_id`/`from_title` при переносе между
  этапами); смену этапа теперь пишут и синк GitLab (кроме первого импорта задачи), и повтор
  задачи. Новый `GET /milestones/:id/metrics` — прогресс по числу и оценке, сигналы
  (просрочено, без оценки, без исполнителя), статусы, исполнители, изменение объёма после
  старта, дневной ряд burn-up и темп трёх прошлых этапов.
