// The Get Started guide (#2753): the scenario a first-time user is walked
// through, in order. Pure data, like data/whatsNew.js — the engine that walks it
// lives in stores/tour.js, the layer that draws it in components/TourOverlay.vue.
//
// Anchors are `data-tour="<key>"` keys, or a raw CSS selector where the element
// already carries a usable attribute (kanban columns and cards do). Every key
// named here is in the markup — grep for it to find the element.
//
// Advancement on action steps follows the author's call: a step ends when the
// entity is actually created, not when a button was clicked (a modal can be
// cancelled, and the guide must not run ahead of the user). Steps that only open
// something — a dropdown, an inline input — have nothing to create, so those do
// advance on the click itself.
//
// Points 6–12 (#2759) show the task the user just created, then the board's own
// tools and the other sections. Two kinds of step there:
//
//   * "show, don't ask" (points 6, 9-tabs, 10, 11) — info steps with `extra`
//     anchors, several arrows at once out of one popover.
//   * "fill this in" (point 8) — action steps that end on the field carrying a
//     value (`advanceOn.set`), not on the picker being opened: opening the due
//     calendar and closing it empty leaves the step where it was, same rule as
//     the cancelled project modal above.
//
// Card anchors are scoped to the «К работе» column so a board that already has
// cards elsewhere can't pull the arrow onto some other task.

// The card the user creates in the last step of point 5.
const NEW_CARD = '[data-column-name="К работе"] [data-testid="task-card"]'

export const GET_STARTED = [
  {
    id: 'workspaces',
    anchor: 'ws-switch',
    title: 'Пространства',
    body: 'Здесь переключаются пространства: у каждого свои проекты, участники и теги. Рядом создаётся новое — но начнём с личного.',
    mode: 'info',
  },
  {
    id: 'tree-add',
    anchor: 'proj-add',
    title: 'Дерево проектов',
    body: 'Проекты живут слева. Нажмите «+», чтобы добавить первый.',
    mode: 'action',
    advanceOn: { click: 'proj-add' },
  },
  {
    id: 'menu-project',
    anchor: 'menu-project',
    title: 'Выберите «Проект»',
    body: 'Группа пригодится позже, когда проектов станет много.',
    mode: 'action',
    advanceOn: { click: 'menu-project' },
  },
  {
    id: 'project-create',
    anchor: 'project-name',
    // The address checkbox is manager-only; when it isn't rendered the extra
    // arrow is simply skipped — only the primary anchor gates the step.
    extra: ['project-slug'],
    // Un-dim the «Создать» button too, so the user isn't left staring at a
    // greyed-out control they're supposed to press (#2753 rework).
    cut: ['project-submit'],
    title: 'Назовите проект',
    body: 'Название решает и адрес проекта: /project/название. Адрес назначается один раз — если он важен, задайте его вручную.',
    mode: 'action',
    advanceOn: { count: 'project-row' },
  },
  {
    id: 'board-add',
    // Scoped to the project the user just created (`{project}` → its id), so the
    // arrow lands on its «+» and not the first project's (#2753 rework).
    anchor: '[data-tour-project="{project}"] [data-tour="board-add"]',
    title: 'Добавьте доску',
    body: 'Задачи живут на досках, у проекта их может быть несколько. Нажмите «+» у проекта.',
    mode: 'action',
    advanceOn: { click: '[data-tour-project="{project}"] [data-tour="board-add"]' },
  },
  {
    id: 'board-create',
    anchor: 'board-name',
    title: 'Назовите доску',
    body: 'Введите название и нажмите Enter — доска появится в дереве с четырьмя колонками.',
    mode: 'action',
    advanceOn: { count: 'board-row' },
  },
  {
    id: 'board-open',
    // The board just created (`{board}` → its id), not the first in the tree.
    anchor: '[data-tour-board="{board}"]',
    title: 'Откройте доску',
    body: 'Нажмите на доску — откроется канбан.',
    mode: 'action',
    advanceOn: { click: '[data-tour-board="{board}"]' },
  },
  {
    id: 'task-create',
    anchor: '[data-column-name="К работе"] [data-testid="add-task-button"]',
    title: 'Создайте задачу',
    body: 'Нажмите «Создать задачу» в колонке «К работе», введите название и нажмите Enter.',
    mode: 'action',
    advanceOn: { count: NEW_CARD },
  },

  // ── 6. Что можно задать прямо на карточке ────────────────────────────────
  {
    id: 'card-fields',
    anchor: `${NEW_CARD} [data-tour="card-priority"]`,
    extra: [
      `${NEW_CARD} [data-tour="card-due"]`,
      `${NEW_CARD} [data-tour="card-tags"]`,
      `${NEW_CARD} [data-tour="card-assignees"]`,
    ],
    title: 'Прямо на карточке',
    body: 'Приоритет, срок, теги и исполнители правятся не открывая задачу — по клику на значок карточки.',
    mode: 'info',
  },

  // ── 7. Открыть карточку ──────────────────────────────────────────────────
  {
    id: 'card-open',
    anchor: NEW_CARD,
    title: 'Откройте задачу',
    body: 'Нажмите на карточку — откроется полная форма задачи.',
    mode: 'action',
    // `click: true` = «клик по собственному якорю шага», чтобы не повторять
    // длинный селектор карточки.
    advanceOn: { click: true },
  },

  // ── 8. Заполнить поля задачи ─────────────────────────────────────────────
  {
    id: 'tm-due',
    anchor: 'tm-due',
    title: 'Срок',
    body: 'Задайте срок — в календаре же настраиваются повтор и напоминание.',
    mode: 'action',
    advanceOn: { set: '[data-tour="tm-due"][data-tour-set]' },
  },
  {
    id: 'tm-assignees',
    anchor: 'tm-assignees',
    title: 'Исполнители',
    body: 'Выберите, кто ведёт задачу. Пока вы в пространстве один — назначьте себя.',
    mode: 'action',
    advanceOn: { set: '[data-tour="tm-assignees"][data-tour-set]' },
  },
  {
    id: 'tm-priority',
    anchor: 'tm-priority',
    title: 'Приоритет',
    body: 'Приоритет красит точку на карточке и по нему же можно сортировать доску.',
    mode: 'action',
    advanceOn: { set: '[data-tour="tm-priority"][data-tour-set]' },
  },
  {
    id: 'tm-tags',
    anchor: 'tm-tags',
    title: 'Теги',
    body: 'Тег создаётся прямо отсюда. По тегам доска умеет группироваться в колонки — это главный способ разложить задачи по-своему.',
    mode: 'action',
    advanceOn: { set: '[data-tour="tm-tags"][data-tour-set]' },
  },
  {
    id: 'tm-description',
    anchor: 'tm-description',
    title: 'Описание',
    body: 'Опишите задачу: поддерживается Markdown, вложения и упоминания через @.',
    mode: 'action',
    advanceOn: { set: '[data-tour="tm-description"][data-tour-set]' },
  },

  // ── 9. Табы и сохранение ─────────────────────────────────────────────────
  {
    id: 'tm-tabs',
    anchor: '[data-name="comments"]',
    extra: ['[data-name="subtasks"]', '[data-name="relations"]', '[data-name="history"]'],
    title: 'Комментарии, подзадачи, связи',
    body: 'Здесь же — обсуждение, дерево подзадач, связи с другими задачами и полная история изменений.',
    mode: 'info',
  },
  {
    id: 'tm-save',
    anchor: 'tm-save',
    title: 'Сохраните задачу',
    body: 'Нажмите «Сохранить» — модалка закроется, а изменения появятся на карточке.',
    mode: 'action',
    advanceOn: { click: 'tm-save' },
  },

  // ── 10. Инструменты доски ────────────────────────────────────────────────
  {
    id: 'board-tools',
    anchor: 'board-layout',
    extra: ['ws-search', 'board-actions'],
    title: 'Виды и инструменты доски',
    body: 'Слева — канбан, список, календарь и гант: одни и те же задачи в разных разрезах. В центре — поиск по всему пространству, справа — теги, этапы и архив.',
    mode: 'info',
  },
  {
    id: 'board-composer',
    anchor: 'board-composer',
    extra: ['board-customize'],
    title: 'Группировка, сортировка, фильтры',
    body: 'Через «+» на этой панели добавляются группировка (в том числе по тегам), сортировка и фильтры, а справа — настройка вида карточек.',
    mode: 'info',
  },

  // ── 11. Остальные разделы ────────────────────────────────────────────────
  {
    id: 'nav-sections',
    anchor: '[data-nav="notes"]',
    extra: ['[data-nav="documents"]', '[data-nav="reminders"]'],
    title: 'Заметки, документы, напоминания',
    body: 'Кроме досок в пространстве живут заметки, совместные документы и напоминания.',
    mode: 'info',
  },
  {
    id: 'nav-footer',
    anchor: 'footer-settings',
    extra: ['footer-notifications'],
    title: 'Настройки и уведомления',
    body: 'Внизу — ваш профиль и настройки, рядом — колокольчик с уведомлениями.',
    mode: 'info',
  },

  // ── 12. Финал ────────────────────────────────────────────────────────────
  {
    id: 'done',
    anchor: 'sb-footer',
    title: 'Готово',
    body: 'Это всё, что нужно для старта. Обучение можно перезапустить отсюда же, из нижней части боковой панели.',
    mode: 'info',
  },
]
