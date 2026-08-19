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
// This file covers scenario points 1–5 (workspaces → project → board → first
// task); points 6–12 (task card, task modal, board tools, sections) are appended
// in #2759.

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
    title: 'Назовите проект',
    body: 'Название решает и адрес проекта: /project/название. Адрес назначается один раз — если он важен, задайте его вручную.',
    mode: 'action',
    advanceOn: { count: 'project-row' },
  },
  {
    id: 'board-add',
    anchor: 'board-add',
    title: 'Добавьте доску',
    body: 'Задачи живут на досках, у проекта их может быть несколько. Нажмите «+» у проекта.',
    mode: 'action',
    advanceOn: { click: 'board-add' },
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
    anchor: 'board-row',
    title: 'Откройте доску',
    body: 'Нажмите на доску — откроется канбан.',
    mode: 'action',
    advanceOn: { click: 'board-row' },
  },
  {
    id: 'task-create',
    anchor: '[data-column-name="К работе"] [data-testid="add-task-button"]',
    title: 'Создайте задачу',
    body: 'Нажмите «Создать задачу» в колонке «К работе», введите название и нажмите Enter.',
    mode: 'action',
    advanceOn: { count: '[data-column-name="К работе"] [data-testid="task-card"]' },
  },
]
