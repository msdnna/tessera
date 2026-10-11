- **feat(api): экспорт и импорт задач доски в CSV и JSON через фоновые задания (#3068).**
  `POST /boards/:id/export {format: csv|json, task_ids?, include_subtasks?}` ставит
  задание экспорта (очередь, до 3 попыток) и отвечает 202 с ним; `task_ids` — карточки
  текущего вида (с подзадачами). Файл (CSV с BOM, колонки — стабильные ключи `number,
  title, description, column, priority, tags, assignees, start_date, due_date, estimate,
  milestone, parent, completed_at, created_at`; JSON — `{board, exported_at, tasks}`)
  скачивается через `GET /jobs/:id/download` автором или администратором 7 дней.
  `POST /boards/:id/import` (multipart: `file`, `mapping` — поле задачи → колонка файла,
  `options` — `column_id`, `create_tags`) разбирает CSV (`,`/`;`/таб) или JSON, отказывает
  сразу без колонки заголовка и ставит задание: строка — одна транзакция (задача, теги
  проекта — новые создаются, исполнители по почте/имени среди участников, приоритет
  `none…urgent`/рус./0–4, сроки ISO/`ДД.ММ.ГГГГ`, date-only — полночь UTC), колонка по
  имени или ключу засеянной колонки, иначе колонка по умолчанию; нераспознанное —
  предупреждения в логе задания, повтор попытки продолжает с первой несозданной строки.
  Маршрут импорта — с собственным бюджетом тела (храповик #2932). `GET /jobs/:id` и
  `POST /jobs/:id/cancel` — своё задание для участника.
