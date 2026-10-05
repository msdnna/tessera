// ⌘K palette (v2, #2957): ranking, prefixes, date parsing, and the keyboard
// walk results → task actions → field options → apply, on a mounted palette.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { matchScore, highlight, rank } from '@/utils/fuzzy'
import { parseQuery } from '@/components/palette/usePaletteResults'
import { parseDay, nextMonday } from '@/components/palette/usePaletteTask'

const TASK = {
  id: 't1',
  number: 42,
  title: 'Редизайн доски',
  board_id: 'b1',
  column_id: 'c2',
  priority: 1,
  due_date: null,
  start_date: null,
  recurrence: null,
  completed_at: null,
  assignees: [],
  tags: [],
}
const update = vi.fn(() => Promise.resolve({ data: {} }))
const search = vi.fn(() =>
  Promise.resolve({
    data: {
      tasks: [{ ...TASK, project_slug: 'p', board_slug: 'b', column_name: 'В процессе' }],
      boards: [],
      notes: [],
      documents: [],
    },
  }),
)

vi.mock('@/api', () => ({
  workspaces: {
    search: (...a) => search(...a),
    members: vi.fn(() =>
      Promise.resolve({ data: [{ user_id: 'u1', name: 'Мария', email: 'm@x' }] }),
    ),
    milestones: vi.fn(() => Promise.resolve({ data: [] })),
  },
  tasks: {
    get: vi.fn(() => Promise.resolve({ data: TASK })),
    update: (...a) => update(...a),
    move: vi.fn(),
  },
  boards: {
    get: vi.fn(() => Promise.resolve({ data: { id: 'b1', project_id: 'p1', workspace_id: 'w1' } })),
    columns: vi.fn(() =>
      Promise.resolve({
        data: [
          { id: 'c1', name: 'К работе', position: 1 },
          { id: 'c2', name: 'В процессе', position: 2 },
        ],
      }),
    ),
    tasks: vi.fn(() => Promise.resolve({ data: [] })),
  },
  projects: { tags: vi.fn(() => Promise.resolve({ data: [] })) },
}))
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), resolve: (p) => ({ href: String(p) }) }),
}))
vi.mock('naive-ui', async (importOriginal) => ({
  ...(await importOriginal()),
  useMessage: () => ({ error: vi.fn(), success: vi.fn() }),
}))

describe('fuzzy', () => {
  it('prefers whole, prefix, word start, then substring', () => {
    const s = (t) => matchScore(t, 'ре')
    expect(s('ре')).toBeGreaterThan(s('редизайн'))
    expect(s('редизайн')).toBeGreaterThan(s('новый релиз'))
    expect(s('новый релиз')).toBeGreaterThan(s('перезагрузка'))
    expect(s('доска')).toBe(0)
  })

  it('matches every word of a multi-word query in any order', () => {
    expect(matchScore('экспорт документа в PDF', 'pdf экспорт')).toBeGreaterThan(0)
  })

  it('splits the title around the hit', () => {
    expect(highlight('Перерыв', 'ре')).toEqual([
      { text: 'Пе', hit: false },
      { text: 'ре', hit: true },
      { text: 'рыв', hit: false },
    ])
  })

  it('ranks best-first and keeps the incoming order among equals', () => {
    expect(rank(['б ре', 'а ре', 'ре'], (x) => [x], 'ре')).toEqual(['ре', 'б ре', 'а ре'])
  })
})

describe('palette query', () => {
  it('reads a prefix as a section', () => {
    expect(parseQuery('#2950')).toEqual({ section: 'tasks', term: '2950' })
    expect(parseQuery('@мар')).toEqual({ section: 'people', term: 'мар' })
    expect(parseQuery('> тема')).toEqual({ section: 'commands', term: 'тема' })
    expect(parseQuery(' ре ')).toEqual({ section: 'all', term: 'ре' })
  })

  it('parses typed due dates', () => {
    const now = new Date(2026, 9, 5)
    expect(parseDay('12.10', now)).toEqual(new Date(2026, 9, 12))
    expect(parseDay('1.2.27', now)).toEqual(new Date(2027, 1, 1))
    expect(parseDay('2026-11-03', now)).toEqual(new Date(2026, 10, 3))
    expect(parseDay('31.02', now)).toBeNull()
    expect(parseDay('завтра', now)).toBeNull()
    expect(nextMonday(new Date(2026, 9, 5))).toEqual(new Date(2026, 9, 12)) // Mon → next Mon
    expect(nextMonday(new Date(2026, 9, 8))).toEqual(new Date(2026, 9, 12))
  })
})

describe('CommandPalette', () => {
  let wrapper
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers({ shouldAdvanceTime: true })
    update.mockClear()
    // jsdom has no layout, so no scrollIntoView.
    Element.prototype.scrollIntoView = vi.fn()
  })
  afterEach(() => {
    wrapper?.unmount()
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  async function openWith(query) {
    const { default: CommandPalette } = await import('@/components/palette/CommandPalette.vue')
    const { usePaletteStore } = await import('@/stores/palette')
    const { useWorkspacesStore } = await import('@/stores/workspaces')
    useWorkspacesStore().currentId = 'w1'
    const palette = usePaletteStore()
    wrapper = mount(CommandPalette, { attachTo: document.body })
    palette.show(query)
    await flushPromises()
    vi.advanceTimersByTime(200)
    await flushPromises()
    return palette
  }
  const input = () => document.querySelector('[data-testid=palette-input]')
  const key = async (k, opts = {}) => {
    input().dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, ...opts }))
    await flushPromises()
  }

  it('lists the task hit and walks Tab → priority → apply', async () => {
    const palette = await openWith('ре')
    expect(search).toHaveBeenCalledWith('w1', 'ре')
    expect(document.querySelector('[data-row="task:t1"]')).toBeTruthy()

    await key('Tab') // narrow to the task
    expect(palette.scope).toMatchObject({ taskId: 't1', field: null })
    expect(document.querySelector('[data-row="priority"]')).toBeTruthy()

    // Down twice to «Приоритет» (status, assignees, priority), Tab opens it.
    await key('ArrowDown')
    await key('ArrowDown')
    await key('Tab')
    expect(palette.scope.field).toBe('priority')

    // The cursor starts on the current value (1 = low); ↑ is «normal» (2).
    await key('ArrowUp')
    await key('Enter')
    expect(update).toHaveBeenCalledWith('t1', expect.objectContaining({ priority: 2 }))
    // Reached via Tab, so applying steps back to the action list.
    expect(palette.open).toBe(true)
    expect(palette.scope.field).toBeNull()

    await key('Tab', { shiftKey: true })
    expect(palette.scope).toBeNull()
    expect(palette.query).toBe('ре')
  })

  it('closes on Escape', async () => {
    const palette = await openWith('')
    await key('Escape')
    expect(palette.open).toBe(false)
  })
})
