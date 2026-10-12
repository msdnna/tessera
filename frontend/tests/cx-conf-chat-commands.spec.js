import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import {
  commandMatches,
  defaultTaskBoard,
  mentionIds,
  mentionMatches,
  mentionQuery,
  parseTaskCommand,
  slashQuery,
  splitMentions,
  taskLineHref,
} from '@/utils/confChatCommands'

// The call chat's «/» and «@» (#3055).

const members = [
  { user_id: 'u-anna', name: 'Анна', email: 'anna@x.io' },
  { user_id: 'u-annak', name: 'Анна Ким', email: 'kim@x.io' },
  { user_id: 'u-ilya', name: 'Илья Петров', email: 'ilya@x.io' },
  { user_id: 'me', name: 'Мария', email: 'me@x.io' },
]

describe('the rules', () => {
  it('reads «/» only at the start, and the task command in either language', () => {
    expect(slashQuery('/за')).toBe('за')
    expect(slashQuery('/')).toBe('')
    expect(slashQuery('и/или')).toBe(null)
    expect(slashQuery('/задача x')).toBe(null)
    expect(commandMatches('за')).toEqual(['task'])
    expect(commandMatches('ta')).toEqual(['task'])
    expect(commandMatches('zz')).toEqual([])
    expect(parseTaskCommand('/задача   Прогноз  на burn-up ')).toEqual({
      title: 'Прогноз на burn-up',
    })
    expect(parseTaskCommand('/task Fix it')).toEqual({ title: 'Fix it' })
    expect(parseTaskCommand('/задача')).toEqual({ title: '' })
    expect(parseTaskCommand('/shrug')).toBe(null)
    expect(parseTaskCommand('привет /задача x')).toBe(null)
  })

  it('opens «@» after a space, not inside an e-mail', () => {
    expect(mentionQuery('привет @Ан')).toEqual({ start: 7, query: 'Ан' })
    expect(mentionQuery('@')).toEqual({ start: 0, query: '' })
    expect(mentionQuery('mail@x')).toBe(null)
    expect(mentionQuery('@Анна ')).toBe(null)
  })

  it('matches by any word of the name or the e-mail, never the reader', () => {
    expect(mentionMatches(members, 'ки', { me: 'me' }).map((m) => m.user_id)).toEqual(['u-annak'])
    expect(mentionMatches(members, 'ilya', { me: 'me' }).map((m) => m.user_id)).toEqual(['u-ilya'])
    expect(mentionMatches(members, '', { me: 'me' }).map((m) => m.user_id)).not.toContain('me')
  })

  it('draws the longest name and reads the ids off the chips', () => {
    const parts = splitMentions('@Анна Ким и @Анна, гляньте; mail@Анна нет', members)
    expect(parts.filter((p) => p.mention).map((p) => p.mention.user_id)).toEqual([
      'u-annak',
      'u-anna',
    ])
    expect(parts.map((p) => p.text).join('')).toBe('@Анна Ким и @Анна, гляньте; mail@Анна нет')
    expect(
      mentionIds('@Анна Ким, @Анна Кимова? — это Анна; @Илья Петров @Илья Петров', members),
    ).toEqual(['u-annak', 'u-anna', 'u-ilya'])
    expect(splitMentions('без упоминаний', members)).toEqual([{ text: 'без упоминаний' }])
  })

  it('picks the call’s board first, then the last one, then the first', () => {
    expect(defaultTaskBoard(['a', 'b', 'c'], { preferred: ['c', 'b'] })).toBe('c')
    expect(defaultTaskBoard(['a', 'b'], { preferred: ['', 'b'] })).toBe('b')
    expect(defaultTaskBoard(['a', 'b'], { preferred: ['zz'] })).toBe('a')
    expect(defaultTaskBoard([], {})).toBe(null)
  })

  it('links a task line to its board with the task open', () => {
    expect(taskLineHref({ number: 7, board_slug: 'web', project_slug: 'tessera' })).toBe(
      '/project/tessera/board/web?task=7',
    )
    expect(taskLineHref(null)).toBe(null)
  })
})

// ── the rail ──────────────────────────────────────────────────────────────
const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return { ...actual, useMessage: () => toast }
})
vi.mock('@/composables/useMessage', async () => ({
  useMessage: (await import('naive-ui')).useMessage,
}))
const api = {
  messages: vi.fn(),
  postMessage: vi.fn(),
  postMessageWithFiles: vi.fn(),
  removeMessage: vi.fn(),
  attachment: vi.fn(),
  createTask: vi.fn(),
}
const projApi = { boards: vi.fn() }
vi.mock('@/api', () => ({
  conferences: api,
  projects: projApi,
  getAccessToken: () => 'jwt',
  apiBaseURL: () => '/api',
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { id: 'me' }, isAdmin: false }),
}))
vi.mock('@/stores/workspaces', () => ({
  useWorkspacesStore: () => ({
    projects: [
      { id: 'p1', name: 'Web' },
      { id: 'p2', name: 'Android' },
    ],
  }),
}))

const { default: ConferenceChat } = await import('@/components/conference/ConferenceChat.vue')

beforeEach(() => {
  setActivePinia(createPinia())
  for (const fn of [...Object.values(api), projApi.boards]) fn.mockReset()
  api.messages.mockResolvedValue({ data: { messages: [], has_more: false } })
  api.postMessage.mockResolvedValue({ data: { id: 'n1', body: 'x', attachments: [] } })
  projApi.boards.mockImplementation(async (pid) => ({
    data: pid === 'p1' ? [{ id: 'b-web', name: 'Спринт' }] : [{ id: 'b-and', name: 'Релиз' }],
  }))
})
afterEach(() => vi.restoreAllMocks())

async function rail(props = {}) {
  const w = mount(ConferenceChat, {
    props: { conferenceId: 'c1', members, ...props },
    attachTo: document.body,
  })
  await flushPromises()
  return w
}

const input = (w) => w.find('[data-testid="conference-chat-input"] textarea')

describe('the rail', () => {
  it('sends the ids of the people it mentions', async () => {
    const w = await rail()
    await input(w).setValue('@Илья Петров посмотри')
    await input(w).trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(api.postMessage).toHaveBeenCalledWith('c1', '@Илья Петров посмотри', ['u-ilya'])
    w.unmount()
  })

  it('keeps the old call shape without mentions', async () => {
    const w = await rail()
    await input(w).setValue('просто текст')
    await input(w).trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(api.postMessage).toHaveBeenCalledWith('c1', 'просто текст')
    w.unmount()
  })

  it('offers «/задача» on «/» and inserts it on Enter', async () => {
    const w = await rail()
    await input(w).setValue('/')
    expect(w.find('[data-testid="conference-chat-suggest"]').exists()).toBe(true)
    await input(w).trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(input(w).element.value).toBe('/задача ')
    expect(api.postMessage).not.toHaveBeenCalled()
    w.unmount()
  })

  it('creates a task on the call’s board and shows the line', async () => {
    api.createTask.mockResolvedValue({
      data: {
        task: { id: 't1' },
        message: {
          id: 'm-task',
          kind: 'task',
          user_name: 'Мария',
          body: '#12 Прогноз',
          created_at: '2026-10-10T10:00:00Z',
          task: { id: 't1', number: 12, title: 'Прогноз', board_slug: 'b', project_slug: 'p' },
          attachments: [],
        },
      },
    })
    const w = await rail({ defaultBoardId: 'b-and' })
    await input(w).setValue('/задача Прогноз')
    await flushPromises()
    expect(w.find('[data-testid="conference-chat-task-strip"]').exists()).toBe(true)
    await input(w).trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(api.createTask).toHaveBeenCalledWith('c1', { title: 'Прогноз', board_id: 'b-and' })
    expect(api.postMessage).not.toHaveBeenCalled()
    const line = w.find('[data-testid="conference-chat-task"]')
    expect(line.text()).toContain('#12')
    expect(w.find('[data-testid="conference-chat-task-link"]').attributes('href')).toBe(
      '/project/p/board/b?task=12',
    )
    expect(input(w).element.value).toBe('')
    w.unmount()
  })

  it('does not send a bare command', async () => {
    const w = await rail()
    await input(w).setValue('/задача ')
    await flushPromises()
    await input(w).trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(api.createTask).not.toHaveBeenCalled()
    expect(api.postMessage).not.toHaveBeenCalled()
    w.unmount()
  })

  it('draws chips for mentions and a struck-through line for a gone task', async () => {
    api.messages.mockResolvedValue({
      data: {
        messages: [
          {
            id: 'a',
            user_id: 'u-ilya',
            user_name: 'Илья',
            body: '@Мария глянь',
            created_at: '2026-10-10T10:00:00Z',
            attachments: [],
          },
          {
            id: 'b',
            kind: 'task',
            user_id: 'u-ilya',
            user_name: 'Илья',
            body: '#3 Старое',
            task: null,
            created_at: '2026-10-10T10:01:00Z',
            attachments: [],
          },
        ],
        has_more: false,
      },
    })
    const w = await rail()
    const chip = w.find('[data-testid="conference-chat-mention"]')
    expect(chip.text()).toBe('@Мария')
    expect(chip.classes()).toContain('me')
    expect(w.find('.task-ref.gone').text()).toBe('#3 Старое')
    w.unmount()
  })
})
