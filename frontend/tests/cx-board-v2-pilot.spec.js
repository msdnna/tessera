// The board's v2 review (#3112): the people of the board are the ones with tasks
// on it; the crumbs are doors; the task menu archives at once with an undo and
// deletes in two steps.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { nextTick } from 'vue'
import { seedBoardStore } from './helpers/boardStore'

const archive = vi.fn(() => Promise.resolve({ data: {} }))
const restore = vi.fn(() => Promise.resolve({ data: {} }))
const remove = vi.fn(() => Promise.resolve({ data: {} }))
vi.mock('@/api', () => ({
  tasks: {
    archive: (...a) => archive(...a),
    restore: (...a) => restore(...a),
    remove: (...a) => remove(...a),
    update: vi.fn(() => Promise.resolve({ data: {} })),
    move: vi.fn(() => Promise.resolve({ data: {} })),
  },
  projects: { boards: vi.fn(() => Promise.resolve({ data: [] })) },
  favorites: {
    list: vi.fn(() => Promise.resolve({ data: [] })),
    add: (...a) => favAdd(...a),
    remove: (...a) => favRemove(...a),
    move: vi.fn(() => Promise.resolve({ data: {} })),
  },
}))
const favAdd = vi.fn((id) => Promise.resolve({ data: { board_id: id, name: 'Спринт' } }))
const favRemove = vi.fn(() => Promise.resolve({ data: null }))
const push = vi.fn()
vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/project/web/board/sprint', params: {}, query: {} }),
  useRouter: () => ({ push, replace: vi.fn() }),
}))

const { default: BoardMembers } = await import('@/components/BoardMembers.vue')
const { default: Topbar } = await import('@/components/Topbar.vue')
const { useTaskMenu } = await import('@/composables/useTaskMenu')
const { useUndoStore } = await import('@/stores/undo')
const { useWorkspacesStore } = await import('@/stores/workspaces')
const { useBoardViewStore } = await import('@/stores/boardView')
const { useProjectWindowStore } = await import('@/stores/projectWindow')

const MEMBERS = [
  { user_id: 'u1', name: 'Анна', role: 'owner' },
  { user_id: 'u2', name: 'Борис', role: 'member' },
  { user_id: 'u3', name: 'Вера', role: 'admin' },
]

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  document.body.innerHTML = ''
})

describe('BoardMembers', () => {
  const mountPeople = (tasks) => {
    seedBoardStore({ members: MEMBERS })
    return mount(BoardMembers, { props: { tasks }, global: { stubs: { UserAvatar: true } } })
  }

  it('hides the cascade while no task has an assignee', () => {
    const w = mountPeople([{ id: 't1', assignee_ids: [] }])
    expect(w.find('[data-testid="board-members"]').exists()).toBe(false)
  })

  it('lists only the people with tasks here, the busiest first', () => {
    const w = mountPeople([
      { id: 't1', assignee_ids: ['u2'] },
      { id: 't2', assignee_ids: ['u2', 'u1'] },
    ])
    const people = w.vm.$.setupState.members
    expect(people.map((m) => [m.user_id, m.tasks])).toEqual([
      ['u2', 2],
      ['u1', 1],
    ])
  })
})

describe('the board crumbs', () => {
  function mountTopbar() {
    const ws = useWorkspacesStore()
    ws.projects = [{ id: 'p1', name: 'Tessera Web', slug: 'web' }]
    ws.boardsByProject = {
      p1: [
        { id: 'b1', name: 'Спринт', slug: 'sprint' },
        { id: 'b2', name: 'Бэклог', slug: 'backlog' },
      ],
    }
    const bv = useBoardViewStore()
    bv.setContext('b1', 'w1', 'p1')
    bv.active = true
    bv.board = { id: 'b1', name: 'Спринт' }
    return mount(Topbar, {
      global: {
        stubs: { SearchBar: true, NotificationsBell: true, ShortcutKeys: true },
      },
      attachTo: document.body,
    })
  }

  it('the project crumb opens the «Проект» window', async () => {
    const w = mountTopbar()
    await w.find('[data-testid="crumb-project"]').trigger('click')
    const win = useProjectWindowStore()
    expect(win.projectId).toBe('p1')
    expect(win.tab).toBe('general')
    w.unmount()
  })

  it('the board crumb lists the project’s boards and goes by address', async () => {
    const w = mountTopbar()
    await w.find('[data-testid="crumb-board"]').trigger('click')
    await nextTick()
    const cur = document.body.querySelector('[data-testid="crumb-boards-b1"]')
    expect(cur.classList).toContain('current')
    document.body.querySelector('[data-testid="crumb-boards-b2"]').click()
    expect(push).toHaveBeenCalledWith('/project/web/board/backlog')
    w.unmount()
  })

  // #3115: the star is real now — it stars the open board and takes it off.
  it('the favourite star toggles the open board', async () => {
    const w = mountTopbar()
    const star = () => w.find('[data-testid="crumb-favorite"]')
    expect(star().attributes('aria-pressed')).toBe('false')
    await star().trigger('click')
    await flushPromises()
    expect(favAdd).toHaveBeenCalledWith('b1')
    expect(star().attributes('aria-pressed')).toBe('true')
    expect(star().classes()).toContain('on')
    await star().trigger('click')
    await flushPromises()
    expect(favRemove).toHaveBeenCalledWith('b1')
    expect(star().attributes('aria-pressed')).toBe('false')
    w.unmount()
  })
})

describe('useTaskMenu v2', () => {
  const TASK = { id: 't1', number: 42, title: 'Починить вход', column_id: 'c1', priority: 3 }

  it('is titled «#N название»', () => {
    const m = useTaskMenu({})
    m.open({ clientX: 1, clientY: 2 }, TASK)
    expect(m.header.value).toBe('#42 Починить вход')
    expect(m.placement.value).toBe('point')
  })

  it('opens under the «⋯» that asked', () => {
    const m = useTaskMenu({})
    const btn = document.createElement('button')
    m.open({ clientX: 1, clientY: 2 }, TASK, btn)
    expect(m.placement.value).toBe('bottom-end')
    expect(m.anchor()).toBe(btn)
  })

  it('archives at once and offers the undo, which restores', async () => {
    const onChanged = vi.fn()
    const m = useTaskMenu({ onChanged })
    m.open({ clientX: 0, clientY: 0 }, TASK)
    await m.select('archive')
    expect(archive).toHaveBeenCalledWith('t1')
    const undo = useUndoStore()
    expect(undo.toasts).toHaveLength(1)
    expect(undo.toasts[0].icon).toBe('archive')
    await undo.undo(undo.toasts[0].id)
    expect(restore).toHaveBeenCalledWith('t1')
    expect(onChanged).toHaveBeenCalledTimes(2)
  })

  it('picks the priority by its digit row and ticks the current one', () => {
    const m = useTaskMenu({})
    m.open({ clientX: 0, clientY: 0 }, TASK)
    const prio = m.options.value.find((o) => o.key === 'prio')
    expect(prio.keys).toBe('card.priorityPick')
    expect(prio.children.find((c) => c.checked).key).toBe('prio:3')
    expect(prio.children.every((c) => c.dot)).toBe(true)
  })
})
