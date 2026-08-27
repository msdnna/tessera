// Demo workspace for the help-centre screenshots (#2793).
//
// Separate from e2e/api.js's seedBoard on purpose: the e2e suite seeds for
// *assertions* (names carry the run id so parallel runs never collide), while a
// screenshot has to look like a real product — "Проект e2e-m3k1x9" in the
// sidebar would be in every picture of the manual. So everything visible here is
// a fixed, plausible Russian label, and the only place the run id survives is the
// registration email, which never appears on screen.
//
// Dates are relative to a base timestamp passed in by the caller, not to
// Date.now() at each call site, so one run cannot straddle midnight and produce
// "просрочено" in one shot and "сегодня" in the next.

import { api, register, newCredentials } from '../api.js'

const day = 86400000

function iso(base, days) {
  return new Date(base + days * day).toISOString()
}

// The board the screenshots are taken of. Columns come from the backend's
// defaults (К работе / В процессе / На рассмотрении / Готово), so the cards are
// listed per column name rather than per index.
const CARDS = {
  'К работе': [
    {
      title: 'Экран онбординга: тексты и иллюстрации',
      description:
        'Собрать финальные тексты трёх шагов и заменить временные иллюстрации на отрисованные.',
      priority: 2,
      start: 1,
      due: 6,
      tags: ['дизайн'],
    },
    {
      title: 'Импорт задач из CSV',
      description: 'Разобрать формат, показать предпросмотр перед импортом.',
      priority: 1,
      start: 5,
      due: 9,
      tags: ['бэкенд'],
    },
    {
      title: 'Ревизия пустых состояний',
      priority: 0,
      start: -3,
      due: 1,
      tags: ['дизайн'],
    },
  ],
  'В процессе': [
    {
      title: 'Push-уведомления о напоминаниях',
      description:
        'Доставка через мобильный клиент: регистрация токена, отправка, тихие часы.\n\nОсталось: тихие часы и повтор при ошибке доставки.',
      priority: 3,
      start: -5,
      due: 2,
      tags: ['мобильное', 'бэкенд'],
      assign: true,
      subtasks: ['Регистрация токена устройства', 'Отправка при наступлении срока', 'Тихие часы'],
      comment: 'Токены регистрируются, отправка уходит. Осталось окно тихих часов.',
    },
    {
      title: 'Фильтры на доске: сохранение пресетов',
      description: 'Пресеты фильтров хранятся локально и переживают перезагрузку.',
      priority: 2,
      start: -2,
      due: 4,
      tags: ['фронтенд'],
      assign: true,
    },
  ],
  'На рассмотрении': [
    {
      title: 'Экспорт документа в PDF',
      description: 'Печатная вёрстка с оглавлением и колонтитулами.',
      priority: 2,
      start: -8,
      due: -1,
      tags: ['фронтенд'],
      assign: true,
    },
  ],
  Готово: [
    {
      title: 'Тёмная тема: контраст карточек',
      priority: 1,
      tags: ['дизайн'],
      completed: true,
    },
    {
      title: 'Поиск по задачам в шапке',
      priority: 2,
      tags: ['фронтенд'],
      completed: true,
    },
  ],
}

const TAG_COLORS = {
  дизайн: '#e2b23a',
  фронтенд: '#7c5cff',
  бэкенд: '#2f80ed',
  мобильное: '#18a058',
}

// writeDoc fills a freshly created document with paragraphs. The content is
// ProseMirror JSON (the editor's own format) and the save is guarded by the
// document's `updated_at` — pass the one the create call just returned, or the
// backend rejects the write as a concurrent edit.
async function writeDoc(token, doc, paragraphs) {
  return api.patch(
    `/documents/${doc.id}/content`,
    {
      updated_at: doc.updated_at,
      content: {
        type: 'doc',
        content: paragraphs.map((text) => ({
          type: 'paragraph',
          content: [{ type: 'text', text }],
        })),
      },
    },
    token,
  )
}

// The admin screen (#2810) is the only shot that shows e-mail addresses, so the
// demo accounts ask for a presentable one first and fall back to the run-id
// address only when it is taken — that is, on a database that already holds a
// previous run. Domains are the ones RFC 2606 reserves for documentation.
const DEMO_EMAIL = 'a.kovaleva@example.com'

// Extra accounts so the admin list is a list and not a single row. They are
// instance accounts only — not members of the demo workspace — so they change
// nothing in the board, document or milestone shots.
const INSTANCE_USERS = [
  { name: 'Павел Дорохов', email: 'p.dorohov@example.com', admin: true },
  { name: 'Марина Кисляк', email: 'm.kislyak@example.com' },
  { name: 'Тимур Асланов', email: 't.aslanov@example.com', active: false },
]

// A GitLab OAuth application that does not exist, spelled out in full: the card
// on top of the admin screen is in the picture either way, and an empty one
// would document the setup form as a set of blank boxes.
const DEMO_OAUTH = {
  gl_base_url: 'https://gitlab.example.com',
  client_id: '7d1f9c2ab3e84f60a5c7d2e1b8f43a09c6d5e4b2a1f09876543210fedcba9876',
  client_secret: 'gloas-demo-secret-not-a-real-application',
  service_token: 'glpat-DEMOxxxxxxxxxxxxxxxx',
  enabled: true,
  sudo_writeback: false,
}

async function registerPresentable(runId, name, email, suffix = '') {
  const password = newCredentials(runId, suffix).password
  try {
    const creds = { email, name, password }
    return { creds, ...(await register(creds)) }
  } catch (e) {
    // 409 — the address is taken, i.e. this database has seen a run before.
    if (!e.message.includes('→ 409')) throw e
    const creds = { ...newCredentials(runId, suffix), name }
    return { creds, ...(await register(creds)) }
  }
}

// A binding to a GitLab project that does not exist either: the modal shows the
// selected binding's fields, and an unsaved blank form would document the screen
// as a column of placeholders. `enabled` is on because that is the state the
// article describes — the sync it schedules simply never reaches
// gitlab.example.com, which changes nothing in the picture.
const DEMO_BINDING = {
  name: 'Мобильное приложение → demo-project',
  project_path: 'demo-group/demo-project',
  enabled: true,
  sync_interval_sec: 900,
  full_sync_interval_sec: 86400,
  due_source: 'issue_milestone',
  start_source: 'created',
  scope: 'all',
  closed_policy: 'archive_closed_sprints',
  relations_sync: 'pull',
}

// Write-back actions for the demo binding (#2810). The pane documents a table of
// trigger→action rows, and an integration that never had one authored opens it
// synthesized from the legacy flags — a list without a column move, which is the
// row the article talks about first. Spelling the set out here keeps the picture
// stable and shows the qualifiers (a specific column, a marker on a comment).
// The labels are the project's own S:/P: taxonomy, the one the default parsing
// rules in the neighbouring shot read back.
function demoWriteback(columns) {
  const col = (name) => columns.find((c) => c.name === name)
  return {
    enabled: true,
    push_create: true,
    fetch_templates: true,
    bindings: [
      {
        enabled: true,
        trigger: { type: 'column', column_id: col('В процессе')?.id, column_name: 'В процессе' },
        action: { type: 'set_label', label: 'S: In progress', clear_prefix: true },
      },
      {
        enabled: true,
        trigger: { type: 'completion', completed: true },
        action: { type: 'set_state', state: '' },
      },
      {
        enabled: true,
        trigger: { type: 'due' },
        action: { type: 'set_due', date_kind: 'due' },
      },
      {
        enabled: true,
        trigger: { type: 'labels' },
        action: { type: 'reconcile_labels' },
      },
      {
        enabled: false,
        trigger: { type: 'comment' },
        action: { type: 'post_comment', add_marker: true },
      },
    ],
  }
}

// Delivery channels and routing rules for the notifications article (#2810).
// Nothing here can actually deliver: the addresses are documentation domains and
// the tokens are spelled out as demo, so the outbox retries into nowhere — which
// changes nothing in a picture of the settings screen. The browser adds its own
// «Браузер (Chrome)» row on top of these when the shot signs in: device channels
// register themselves per client.
const DEMO_CHANNELS = [
  { type: 'email', label: 'Рабочая почта', config: { address: DEMO_EMAIL } },
  {
    type: 'telegram',
    label: 'Мой телеграм',
    config: { chat_id: '123456789' },
    secret: { bot_token: '1234567:DEMO-not-a-real-bot-token' },
  },
  {
    type: 'shoutrrr',
    label: 'Дежурный чат',
    secret: { url: 'slack://demo/DEMO/not-a-real-token' },
  },
]

// The rules are ordered the way the article explains them: the narrow ones
// first, the mute last. First-match-wins, so a general "everything → mail" rule
// on top would make the rest of the list dead weight — and a screenshot of a
// list that cannot work is worse than no screenshot.
const DEMO_ROUTES = [
  { kinds: ['mention', 'assigned'], channels: ['Рабочая почта', 'Мой телеграм'] },
  { kinds: ['comment', 'updated'], channels: ['Мой телеграм'], scoped: true },
  { kinds: ['integration_sync'], mute: true },
]

// seedNotifications creates the demo user's channels, routing rules and schedule.
// Per-user state (no admin needed), so unlike the instance seed it runs on every
// database.
async function seedNotifications(token, workspaceId) {
  const byLabel = {}
  for (const ch of DEMO_CHANNELS) {
    const created = await api.post(
      '/notification-channels',
      { type: ch.type, label: ch.label, config: ch.config || {}, secret: ch.secret || {} },
      token,
    )
    byLabel[ch.label] = created.id
  }
  for (const r of DEMO_ROUTES) {
    await api.post(
      '/notification-routes',
      {
        matcher: { kinds: r.kinds, ...(r.scoped ? { workspace_id: workspaceId } : {}) },
        channel_ids: (r.channels || []).map((l) => byLabel[l]),
        options: { mute: r.mute === true },
        enabled: true,
      },
      token,
    )
  }
  // Quiet hours and a digest window on, so the schedule block below the rules is
  // a filled-in form rather than a column of defaults.
  await api.put(
    '/notification-prefs',
    {
      due_enabled: true,
      due_lead_minutes: 60,
      due_repeat_minutes: 0,
      reminder_enabled: true,
      digest_minutes: 15,
      quiet_enabled: true,
      quiet_start_minutes: 1320,
      quiet_end_minutes: 480,
      quiet_tz: 'Europe/Moscow',
    },
    token,
  )
}

// seedInstance fills the instance-wide state the admin articles document: a few
// accounts in different states, a GitLab OAuth application and a project binding.
// Only possible for a global admin — and the backend grants that to the *first*
// account of an instance, so it happens on a clean database and is skipped on a
// shared one (the admin shots skip with it; see help-shots.spec.js).
async function seedInstance(runId, token, workspaceId, boardId, columns) {
  for (const [i, u] of INSTANCE_USERS.entries()) {
    let created
    try {
      created = await registerPresentable(runId, u.name, u.email, `+u${i}`)
    } catch {
      continue // taken by an earlier run — the list is illustrative, not exact
    }
    const id = created.user.id
    if (u.admin) await api.patch(`/admin/users/${id}/admin`, { admin: true }, token)
    if (u.active === false) await api.patch(`/admin/users/${id}/active`, { active: false }, token)
  }
  await api.put(
    '/admin/oauth/gitlab',
    {
      ...DEMO_OAUTH,
      org_map: {
        'demo-group': { workspace_id: workspaceId, admins: ['p.dorohov'], users: true },
      },
    },
    token,
  )
  await api.post(
    `/workspaces/${workspaceId}/gitlab/integrations`,
    { ...DEMO_BINDING, board_id: boardId, writeback: demoWriteback(columns) },
    token,
  )
}

// seedDemo builds the whole demo workspace and returns what the shots spec needs
// to navigate: ids plus the credentials to sign in with.
export async function seedDemo(runId, base) {
  const { creds, token, user } = await registerPresentable(runId, 'Аня Ковалёва', DEMO_EMAIL)
  const t = token
  const post = (p, b) => api.post(p, b, t)
  const get = (p) => api.get(p, t)

  const ws = await post('/workspaces', { name: 'Тессера Демо' })
  const group = await post(`/workspaces/${ws.id}/groups`, { name: 'Продукт' })
  const project = await post(`/workspaces/${ws.id}/projects`, {
    name: 'Мобильное приложение',
    group_id: group.id,
  })
  // A second project so the sidebar tree is not a single row in the screenshot.
  await post(`/workspaces/${ws.id}/projects`, { name: 'Веб-клиент', group_id: group.id })
  await post(`/workspaces/${ws.id}/projects`, { name: 'Поддержка' })

  const board = await post(`/projects/${project.id}/boards`, { name: 'Спринт 14' })
  const columns = await get(`/boards/${board.id}/columns`)

  const tags = {}
  for (const [name, color] of Object.entries(TAG_COLORS)) {
    tags[name] = await post(`/projects/${project.id}/tags`, { name, color })
  }

  const created = []
  for (const [columnName, cards] of Object.entries(CARDS)) {
    const column = columns.find((c) => c.name === columnName)
    if (!column) throw new Error(`демо-сид: на доске нет колонки «${columnName}»`)
    for (const card of cards) {
      const task = await post(`/boards/${board.id}/tasks`, {
        column_id: column.id,
        title: card.title,
        description: card.description || '',
        priority: card.priority ?? 0,
        ...(card.due === undefined ? {} : { due_date: iso(base, card.due) }),
        ...(card.start === undefined ? {} : { start_date: iso(base, card.start) }),
      })
      for (const tag of card.tags || []) {
        await post(`/tasks/${task.id}/tags`, { tag_id: tags[tag].id })
      }
      if (card.assign) await post(`/tasks/${task.id}/assignees`, { user_id: user.id })
      for (const title of card.subtasks || []) {
        await post(`/boards/${board.id}/tasks`, {
          column_id: column.id,
          parent_id: task.id,
          title,
        })
      }
      if (card.comment) await post(`/tasks/${task.id}/comments`, { body: card.comment })
      // Creating a card straight into the done column does not close it — the
      // completion flag is set when a task *moves* there — so the finished ones
      // are marked explicitly, otherwise every board and milestone shot shows
      // 0 % progress next to a full «Готово» column.
      if (card.completed) await api.patch(`/tasks/${task.id}`, { completed: true }, t)
      created.push(task)
    }
  }

  // Task relations (#2823). Without a single blocking edge the Gantt shot shows
  // bars and no arrows — exactly the half of the view its article is about. The
  // pair is picked to read as real work: the empty-state pass gates the
  // onboarding screen, and the CSV import is merely adjacent to the PDF export.
  const byTitle = (title) => {
    const found = created.find((x) => x.title === title)
    if (!found) throw new Error(`демо-сид: нет задачи «${title}» для связи`)
    return found
  }
  const RELATIONS = [
    ['Ревизия пустых состояний', 'Экран онбординга: тексты и иллюстрации', 'blocks'],
    ['Импорт задач из CSV', 'Экспорт документа в PDF', 'relates'],
  ]
  for (const [from, to, kind] of RELATIONS) {
    await post(`/tasks/${byTitle(from).id}/relations`, { number: byTitle(to).number, kind })
  }

  const release = await post(`/projects/${project.id}/milestones`, {
    title: 'Релиз 2.4',
    description: 'Напоминания, экспорт документов и фильтры на доске.',
    start_date: iso(base, -14),
    due_date: iso(base, 12),
  })
  // The milestones screen shows each milestone's progress bar, and an empty one
  // reads «нет задач» — put the sprint's work behind the release so the picture
  // shows what the screen is actually for.
  for (const task of created.slice(0, 6)) {
    await post(`/tasks/${task.id}/milestone`, { milestone_id: release.id })
  }
  await post(`/projects/${project.id}/milestones`, {
    title: 'Релиз 2.3',
    description: 'Тёмная тема и поиск.',
    start_date: iso(base, -40),
    due_date: iso(base, -8),
    state: 'closed',
  })

  await post(`/workspaces/${ws.id}/notes`, {
    title: 'Итоги планирования',
    body: 'Берём напоминания и экспорт в PDF.\n\nИмпорт CSV переносим в следующий спринт.',
    project_id: project.id,
  })
  await post(`/workspaces/${ws.id}/notes`, {
    title: 'Вопросы к поддержке',
    body: 'Собрать частые обращения за месяц и свести в статьи справки.',
  })

  // Documents get real text, not just titles: the grid shows each document's
  // preview line, and three cards reading «Пустой документ» would be a picture
  // of an empty product. No `icon` is set for the same reason — the headless
  // browser has no emoji font, so an emoji icon comes out as a tofu box.
  await writeDoc(
    t,
    await post(`/workspaces/${ws.id}/documents`, {
      title: 'Спецификация напоминаний',
      project_id: project.id,
    }),
    [
      'Напоминание срабатывает в указанное время и приходит push-уведомлением на мобильный клиент.',
      'Тихие часы: с 22:00 до 8:00 уведомления откладываются до утра.',
    ],
  )
  await writeDoc(t, await post(`/workspaces/${ws.id}/documents`, { title: 'Регламент релиза' }), [
    'Релиз собирается в понедельник, выкатывается во вторник.',
    'Ветка замораживается за день до сборки; в заморозку попадают только исправления.',
  ])
  await writeDoc(t, await post(`/workspaces/${ws.id}/documents`, { title: 'Онбординг новичка' }), [
    'Первая неделя: доступы, обзор продукта, парное ревью.',
    'Вторая неделя: первая задача в спринте и знакомство с поддержкой.',
  ])

  await post('/reminders', {
    remind_at: iso(base, 1),
    message: 'Созвон по релизу 2.4 в 11:00',
  })
  await post('/reminders', {
    remind_at: iso(base, 3),
    message: 'Собрать обратную связь по онбордингу',
  })

  await seedNotifications(t, ws.id)

  // Instance-wide state for the admin articles (#2810). Gated on the flag the
  // backend itself set at registration, not on a guess about the database.
  if (user.is_admin) await seedInstance(runId, t, ws.id, board.id, columns)

  return {
    runId,
    creds,
    token,
    isAdmin: user.is_admin === true,
    userId: user.id,
    workspaceId: ws.id,
    projectId: project.id,
    boardId: board.id,
    columns: columns.map((c) => ({ id: c.id, name: c.name })),
  }
}
