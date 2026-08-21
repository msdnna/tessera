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
      due: 6,
      tags: ['дизайн'],
    },
    {
      title: 'Импорт задач из CSV',
      description: 'Разобрать формат, показать предпросмотр перед импортом.',
      priority: 1,
      tags: ['бэкенд'],
    },
    {
      title: 'Ревизия пустых состояний',
      priority: 0,
      tags: ['дизайн'],
    },
  ],
  'В процессе': [
    {
      title: 'Push-уведомления о напоминаниях',
      description:
        'Доставка через мобильный клиент: регистрация токена, отправка, тихие часы.\n\nОсталось: тихие часы и повтор при ошибке доставки.',
      priority: 3,
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

// seedDemo builds the whole demo workspace and returns what the shots spec needs
// to navigate: ids plus the credentials to sign in with.
export async function seedDemo(runId, base) {
  const creds = {
    ...newCredentials(runId),
    // The user's name is on every avatar and in the sidebar footer, so it gets a
    // real one instead of "E2E <runid>".
    name: 'Аня Ковалёва',
  }
  const { token, user } = await register(creds)
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

  return {
    runId,
    creds,
    token,
    userId: user.id,
    workspaceId: ws.id,
    projectId: project.id,
    boardId: board.id,
    columns: columns.map((c) => ({ id: c.id, name: c.name })),
  }
}
