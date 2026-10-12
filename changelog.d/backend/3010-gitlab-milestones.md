- **feat(api): этапы GitLab — двусторонний синк: правка, закрытие и конфликт описания (#3010).**
  Этап с привязкой к GitLab больше не только для чтения: `PATCH /milestones/:id` пишет
  название, цели (описание), даты и состояние в milestone GitLab в том же запросе и
  отвечает `gl_sync` (`ok` | `failed` | `conflict`), когда у привязки включена запись
  (привязка и write-back включены; иначе `409 gitlab_readonly`). Сверка трёхсторонняя по
  базе `gitlab_milestone_links.gl_snapshot` (миграция 0087): поменянное только в Tessera
  уходит в GitLab, только в GitLab — приходит синком, в обоих местах — паркуется
  конфликтом (`GET /workspaces/:id/gitlab/conflicts?include=milestones`,
  `POST /milestones/:id/gitlab/conflict/resolve` ours|theirs|manual). Неудачная запись
  оставляет правку локально с отметкой `gl_push_error` — `POST /milestones/:id/gitlab/sync`
  повторяет, синк повторяет сам после pull. Клиент GitLab: `GetProjectMilestone` /
  `UpdateProjectMilestone` (#3022).
