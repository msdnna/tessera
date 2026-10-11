# tessera-v2-ui — заметки по областям

Читать раздел своей области (grep по заголовку). Общие правила UI — `SKILL.md`.

- **Мягкое удаление досок и колонок (миграция 0078):** `deleted_at` + `POST …/restore`,
  воркер `trash_purge` удаляет окончательно через сутки. Удалённая доска невидима всем
  читателям — новый запрос по задачам нескольких досок обязан фильтровать
  `b.deleted_at IS NULL`; колонка при удалении отдаёт задачи соседу (`column_deletion_moves`).
- **Корзина задач/документов и архив проектов (#3001, миграция 0086):** `tasks.deleted_at` /
  `documents.deleted_at` — `DELETE` кладёт поддерево в корзину одной отметкой, `POST …/restore`
  возвращает (НЕ путать с `PATCH /tasks/:id/restore` архива), `DELETE …/purge` — навсегда,
  `trash_purge` — через 30 дней. Задача в корзине невидима всем: **новый запрос по `tasks`/
  `documents` обязан фильтровать `deleted_at IS NULL`** (храповик `handlers/trash_ratchet_test.go`;
  намеренно слепые — в его списке с причиной); `GetTask`/`GetDocument` уже фильтруют, сырьё —
  `…Any`. `projects.archived_at`: вне дерева/поиска/«Моих задач»/сканеров, запись → 409
  `{archived}` (гейт в `loadTask` для не-GET + `requireProjectWritable`), web `readOnly` доски.
- **«Вид доски» и представления (#2970, миграция 0079):** настройки доски — `kit/DockPanel`
  (pop) справа внутри доски во всех 6 видах (⇧V), на телефоне — нижний лист. Представления:
  `board_views.scope` user|board (у общих `user_id` NULL, автор в `created_by`, правят
  owner/admin пространства), `is_default` (одна звезда на список, личная > общей, применяется
  раз за сессию браузера), `position`; `PATCH /views/:id`. Автосохранения нет — «изменено» +
  Обновить ⌘S / Сбросить ⌘⌫; логика — `composables/useBoardViews.js`, чистая часть —
  `utils/boardViews.js`. Конфиг представления = состояние доски (`currentViewConfig`), поиск и
  этап в него не входят. `boards.default_layout` — вид новой доски.
- **Окно «Проект», этапы и теги (#2971, миграция 0080):** настройки проекта — крупное окно
  `components/project/ProjectWindow.vue` (стор `projectWindow.open(id, tab, {filter})`, ⌘,)
  с вкладками Общее · Теги · Этапы · Оценка; TagManager/MilestoneManager удалены. Таблица тегов
  (`utils/tagTable.js`): имя префикса — точечный `PATCH /projects/:id/tag-prefixes`, слияние
  `POST …/tags/merge`, массовое удаление `POST …/tags/delete` + undo `POST …/tags/restore`
  (теги возвращаются под прежними id), `tags.source` user|gitlab, замок — префиксы правил GitLab
  проекта. Этап: два хранимых состояния + `closed_at`, «Запланирован» = активный со стартом
  впереди (`utils/milestones.stageState`). Страница этапа `/project/<slug>/stage/<slug>` —
  `views/StageView.vue` на `GET /milestones/:id/metrics`; объём/burn-up считаются по журналу
  `milestone`/`milestone_removed` (несут `milestone_id`, при переносе `from_id`), поэтому
  **любой новый путь смены `milestone_id` обязан журналить** (`journalMilestoneActor`). На доске
  — селектор этапа в подбаре (⇧E) и полоса этапа (⇧H), выбор и свёрнутость — localStorage на
  доску. GitLab-этап (#3010, миграция 0087) правится, только пока привязка пишет в GitLab
  (`gl_writable` = binding + write-back, иначе `409 gitlab_readonly`): `PATCH /milestones/:id`
  пишет в веху синхронно и отвечает `gl_sync`; сверка трёхсторонняя по
  `gitlab_milestone_links.gl_snapshot` и в push, и в pull (`gitlab_milestone_sync.go`) —
  отказ GitLab = `push_error` («не синхронизировано · повторить», синк повторяет сам),
  правка с обеих сторон = `conflict` в окне конфликтов (`?include=milestones`, entity
  `milestone`), ответ записи на web — через `useStageSync`. Навигация из окна-оверлея — после отмотки его
  history-записи (`ProjectWindow.navigate`), иначе `back()` из `useOverlayBack` отменит push.
- **«Входящие» (#2972, миграция 0081):** заметки и напоминания — одна лента в `kit/DockPanel`
  (variant sidebar) справа под шапкой (`components/inbox/*`, стор `stores/inbox` — лента
  пространства из `GET /workspaces/:id/inbox`, сокет, часы; чистая часть — `utils/inbox.js`).
  Захват — `inbox/CapturePop` у FAB (N M / N R, разбор `parseCapture` в `utils/quickParse`), в
  модалке задачи — его inline-копия (фокус-ловушка NModal). Две таблицы, не одна сущность:
  `notes.visibility` private|workspace (личная видна только автору **во всех чтениях и в сокете**
  — новый запрос по заметкам обязан фильтровать `visibility = 'workspace' OR author_id = viewer`),
  `notes.task_id`; у `reminders` — `body`, `done_at` (идёт за `done`), `snoozed_from`,
  `workspace_id` (NULL — видно в любом пространстве). Разделов `/notes` `/reminders` больше нет
  (редирект в панель); пилл-колокольчик на карточке — из стора, без запроса на карточку.
  Теги и упоминания записи (#3040, миграция 0088): `tags text[]` — имена, не M:N (с задачей
  или проектом сервер пишет имя как тег проекта, цвет даёт клиент; иначе свободная метка),
  `mentions uuid[]` — уведомление `mention` (событие `note_mention`) только новым упомянутым
  **общей** заметки; личная заметка и напоминание не уведомляют никого.
- **Виды доски v2 (#2973, #3009):** Список (`BoardListView` + `list/ListRow`, геометрия и курсор —
  `utils/listView.js`; строки фиксированной высоты → оконная отрисовка, держит 500+ задач),
  Календарь (`BoardCalendarView` + `calendar/CalendarPill`, сетки и перенос срока —
  `utils/calendarView.js`: date-only остаётся UTC-полночью, время — в поясе профиля), Матрица
  (ручной квадрант — поле задачи `eisenhower_quadrant`, навигация — `utils/matrixNav.js`).
  Ширины колонок списка, режим календаря и «Шкала» таймлайна/Ганта (`День/Неделя/Месяц/Квартал`
  + переключатели) — ключи `list`/`calendar`/`chart` конфига представления (`utils/boardViews.js`).
  Клавиши видов — свои id в каталоге (`list.*`, `cal.*`, `matrix.*`) плюс `card.*` с `when` на
  свой вид; действия над задачами — общие функции KanbanBoard (`listActions`/`calendarActions`).
- **Избранные доски (#3115, миграция 0089):** `board_favorites(user_id, board_id, position)`, личные,
  читаются по пространству (`GET /workspaces/:id/favorites`, удалённые доски и чужие пространства
  отфильтрованы запросом), сокет `favorites.changed` — только себе. Web: стор `stores/favorites`,
  звезда у крошек (⇧F) + пункт «⋯ Действия доски» (на телефоне звезда только там), раздел
  `sidebar/FavoritesSection` над «Проектами», группа «Избранное» в ⌘K. Переход из палитры —
  `go()` через `unwindOverlay`, иначе `history.back()` палитры отменяет push на другую доску.
- **Документы и шаблоны v2 (#3032, миграция 0090):** у документа `milestone_id` и `conference_id`
  (чипы в шапке — `documents/DocPins`, `PUT /documents/:id/milestone|conference`, `updated_at` не
  двигают; перенос проекта снимает пины через границу), у шаблона — `category` (sprint · product ·
  engineering · meetings; «Мои» = `author_id`). Шаблоны досок пустого проекта — пресеты на сервере
  (`handlers/board_templates.go`, `POST /projects/:id/boards {template}`), новые `name_key` колонок
  подписываются на клиентах (`board.column.defaultName`, `default_column_*`) и в `seededColumnCaptions`;
  `/project/<slug>` — первая доска или экран «Пустой проект» (`views/ProjectView.vue`).
- **Экраны аккаунта, конференции, справка, колокольчик (#3107):** все экраны входа и 404 —
  `AuthLayout` (аврора «Моих задач» `HomeAurora frozen` + стеклянная карточка); корень обязан быть
  ОДНИМ элементом (`App.vue` анимирует маршрут `mode="out-in"`: фрагмент — абстрактный
  NConfigProvider или даже комментарий над корнем в dev — оставлял пустой экран после входа).
  401 от `/auth/login|register` — отказ формы, не истёкшая сессия. Общая кнопка v2 с клавишей —
  `.kit-btn` в `kit/kit.css`. Конференции: список по дням (`utils/confList`), звонок на всю
  рабочую область, док кнопок, клавиши `meet.*` (N V, N J, ⇧J, ⌘D, ⌘E, ⌘⇧H, ⌘⇧M), мини-окно с
  прилипанием к краям; на стенде нет SFU (токен 503) — экран звонка проверяется подменой рефов
  `session.transport`/`room` (драйвер `.design-sync/.cache/v2e2.mjs`, `mockCall`). «Статья
  помогла?» — ack `helpful:<slug>:<lang>:yes|no` без своей таблицы, итоги — `GET
  /admin/help-feedback` и раздел «Справка» админки. F1 — справка.
- **Уведомления v2 (#3033, миграция 0091):** `/notifications` — страница всей ленты и на десктопе
  (`NotifFeed all` + `composables/useNotificationPages`: `GET /notifications/page` с курсором и
  фильтром на сервере); голый `GET /notifications` (50 строк массивом) оставлен для Android и
  поповера. Новый вид — событие в `handlers/notification_v2.go` + ключи в трёх каталогах
  (`notify/locale.go`, `locales/*/notifications.json`, `NotificationText.kt`; паритет веба и бэкенда
  — храповик-тест). RSVP — `conference_participants.rsvp`, `PUT /conferences/:id/rsvp`;
  исключения тихих часов — `quietExempt`/`quietFor`, спрашивать везде, где спрашивается окно.
- **Встречи v2 (#3034, миграция 0092):** повтор — правило на одной переиспользуемой комнате, не
  копии: сервер отдаёт `next_at` (`internal/confsched`, время держится в `conferences.timezone`);
  новые поля PATCH встречи трёхзначные (`optField`) — старые клиенты их не стирают. Реакции — только
  data-канал LiveKit (тема `reaction`, палитра-белый список), не сокет комнаты. «/задача» в чате —
  `POST /conferences/:id/tasks` (задача + строка `kind=task` одной транзакцией).
- **Тур «Начало работы» и подсказки (#3000, #3070):** тур — главы (`data/getStarted.js`:
  «Основы» 10 шагов, главы по выбору, «Знакомство» для приглашённого, «Основы» телефона нижним
  листом, мини-тур «Что изменилось»), движок `stores/tour.js` (правила `click/count/set/appear/
  vanish/moved`, «Назад» по инфо-шагам, Esc → «Прервать?», «Засчитано»), слой `TourOverlay.vue`
  («Не видим …» с отсчётом вместо молчаливого пропуска). Чек-лист на «Моих задачах» —
  `stores/onboarding.js` считает **результаты** по событиям: свои события сокета (бэкенд кладёт
  `actor` в создание/перенос проекта и группы, доски, этапа, теги/исполнителей — **новый
  user-initiated broadcast делать `broadcastAs`**) и клиентские `tourEvent()` (`utils/tourEvents`).
  Подсказки — `data/hints.js` + `stores/hints.js` + `tour/SpotlightHint.vue` (любой якорь,
  «новое»/«при первой встрече», ≤2 за сессию, ждут свободный экран). Всё — в
  `user_acknowledgements`: `getstarted:*` (done/skipped, `ch:<глава>`, `item:<результат>[:self]`,
  `checklist:hidden`), `spotlight:<ключ>`, `hints:off:new|ctx`, `whatschanged:v2|later`;
  сброс — `DELETE /users/me/acknowledgements?key=|prefix=`. Новый якорь шага — в `MARKUP`
  `tests/dt-get-started.spec.js`; e2e-пользователи (`register()`) выключают тур и подсказки.
- **Вход и приглашения v2 (#3035, миграция 0093):** одноразовые коды (`user_tokens.kind`
  `verify_code`/`email_change`, `handlers/auth_codes.go`) — HMAC с солью, 15 мин, 5 попыток (счёт до
  сравнения), пауза 60 с на адрес и 5 кодов в час; смена почты не выдаёт, занят ли адрес. Превью/отказ
  приглашения — публичные `POST /invitations/lookup|decline`; ссылка-приглашение
  (`workspace_invite_links`, `/join/<token>` → `/invite`) хранится хешем, URL виден один раз. Коды в
  тестах: харнесс-мейлер `testMail` (backend), журнал `E2E_BACKEND_LOG` (`frontend/e2e/mailLog.js`).
- **Участники v2 (#3063, миграция 0094):** роли по-прежнему owner|admin|member; передача владения
  `POST /workspaces/:id/owner` (в tx, бывший → admin, ровно один owner), уход `…/leave` (не owner),
  запросы доступа `/access-requests` (ответ не раскрывает пространство; одобрение — только
  member/admin), группы участников `member_groups` (прав не дают — в пикерах раскрываются в людей).
  Активность: `memberships.last_active_at` штампует `lookupMembership` на изменяющих запросах,
  `users.last_seen_at` — выдача токенов и сокет. Матрица прав — `backend/members_roles_test.go`.
  Поле ввода во всплывашке внутри крупного окна (NModal) рисовать **на месте**, не телепортом.
- **Подписки на задачу (#2963, миграция 0083):** автор, исполнители и комментаторы подписаны
  неявно (список вычисляет `ListTaskSubscribers`, только участники пространства),
  `task_subscriptions` хранит лишь явный выбор (true — подписался, false — отписался).
  Новости задачи — `notifyTaskParticipants` → подписчикам; адресные уведомления (назначили,
  упомянули, удалили) от подписки не зависят.
- **Адреса проектов (#3007, миграция 0085):** адрес проекта (`projects.slug`) уникален на инстанс; смена оставляет старый алиасом на 30 дней
  (`project_slug_aliases`, миграция 0085, #3012). Проект по адресу — только через `projectBySlug`
  (живой slug важнее алиаса); подменить адрес в открытом URL — `utils/projectRoute.withProjectSlug`
  (`router.replace({ ...route, params })` vue-router молча игнорирует). 409 занятого адреса несёт
  `suggestion` и `taken_by` (`utils/slugTaken`, `kit/SlugSuggest`); счётчики для подтверждений
  удаления — `GET /projects|workspaces/:id/counts` (`utils/deleteCounts`).
- **GitLab MR и пайплайны (#2964, миграция 0084):** шов `task_code_links`: MR и пайплайны в «Разработке»
  задачи, `provider`+`source_id` без FK в GitLab-таблицы (чистит сам провайдер). GitLab-часть —
  `internal/gitlab/mergerequests.go` + `handlers/gitlab_mr.go`; вебхуки `merge_request`/`pipeline`
  применяются сразу из payload (issue/note — по-прежнему только через инкрементальный пул).
  MR из других проектов (#2964 rework, миграция 0095): берутся у issue (`related_merge_requests` +
  `closed_by`, пайплайн — из карточки MR), в синке и в фоне при открытии задачи (не чаще раза в 2 мин,
  ответ сокетом `task.code_links`), `handlers/gitlab_mr_foreign.go` + `internal/gitlab/related_mrs.go`;
  ключ строки включает `repo`, в ответе `ref` (`!12` / `group/proj!12`) — показывать его, а не `!number`.
- **GitLab подключения и правила v2 (#3065, #3066, #3105, миграция 0097):** несколько привязок на
  доску (карточка принадлежит одной привязке; task-действия через `boardIntegration`, без выбора —
  409 `choose_integration`); OAuth-подключение `POST /gitlab/connection/oauth` (общий callback, state
  `c.`, токен помечен `gitlab.MarkOAuthToken`, брать только через `credentialToken`); 401 → привязка
  на паузе (`paused_at`, `noteSyncOutcome`); `import_labels` → `labelName` GraphQL. Правила: действие
  `milestone` + `template`, `disabled`, `rules/preview`; `writeback.conflict_policy` ask|tessera|gitlab;
  поля конфликта assignees/labels, пакетный resolve, черновик «Отложить» — localStorage
  `tessera_conflict_drafts`. Полная сверка в две фазы (`NeedNotes`/`WithNotes`): при
  `Issue.NotesLoaded=false` комментарии не трогать.
