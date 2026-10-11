- **feat(api): GitLab — подключения v2 (#3065, миграция 0097).**
  Несколько проектов GitLab на одну доску: снят `UNIQUE (board_id)`, карточка принадлежит одной
  привязке; создание issue из задачи на такой доске просит `integration_id` (409 `choose_integration`
  со списком) (#3082). Подключение аккаунта через GitLab OAuth: `POST /gitlab/connection/oauth`
  отдаёт адрес авторизации (scope `api`), грант возвращается на тот же `/api/auth/gitlab/callback`
  (подписанный state `c.`, новый redirect URI в приложении GitLab не нужен), access/refresh хранятся
  зашифрованными, access обновляется сам (#3083). Срок PAT из `/personal_access_tokens/self`
  (`token_expires_at` у подключения и привязки, у сервисного токена — при сохранении в админке);
  отказ GitLab (401) ставит синхронизацию привязки на паузу (`paused_at`, `pause_reason`), авто-синк
  её пропускает, переподключение или прошедший синк снимают паузу (#3084). Фильтр импорта по лейблам
  `import_labels` — в том же GraphQL-запросе `labelName` (#3085). REST с OAuth-токеном идёт
  `Authorization: Bearer`.
