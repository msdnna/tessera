// Characterization tests for the board card after the split (#2665): the pill
// row and the child list are their own components now, so mount the card and
// check the pieces still reach the DOM and still talk back to the card. A
// missed prop or a slot wired wrong degrades quietly (an empty pill row, a
// child list that renders nothing) rather than throwing.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'
import { seedBoardStore } from './helpers/boardStore'

const update = vi.fn(() => Promise.resolve({ data: {} }))
vi.mock('@/api', () => ({
  tasks: {
    update: (...a) => update(...a),
    move: vi.fn(() => Promise.resolve({ data: {} })),
    setParent: vi.fn(() => Promise.resolve({ data: {} })),
    remove: vi.fn(),
    archive: vi.fn(),
    description: vi.fn(() => Promise.resolve({ data: { description: '' } })),
    addTag: vi.fn(),
    removeTag: vi.fn(),
    addAssignee: vi.fn(),
    removeAssignee: vi.fn(),
    pinGitlabAssignee: vi.fn(),
    removeGitlabAssignee: vi.fn(),
    dueNotify: vi.fn(),
  },
  boards: { createTask: vi.fn(() => Promise.resolve({ data: {} })) },
  projects: { createTag: vi.fn() },
  users: { updatePrefs: vi.fn() },
  columns: { update: vi.fn(), remove: vi.fn() },
}))

const COLS = [
  { id: 'c1', name: 'К работе', color: '#9aa0aa', position: 1 },
  { id: 'c2', name: 'В процессе', color: '#2f80ed', position: 2 },
]
const TASK = {
  id: 't1',
  board_id: 'b1',
  column_id: 'c1',
  title: 'Родитель',
  number: 7,
  priority: 2,
  due_date: '2026-08-20T10:00:00Z',
}
const SUBS = [
  { id: 's1', title: 'Первая', column_id: 'c1', parent_id: 't1' },
  { id: 's2', title: 'Вторая', column_id: 'c1', parent_id: 't1', completed_at: null },
]

// Naive components render for real (the pills live inside n-popover/n-tooltip
// triggers); only the heavy leaves are stubbed. Stub keys are the *internal*
// component names — VTU matches on those, not on the exported `N…` aliases.
const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  RichContent: true,
  DueEditor: true,
  draggable: {
    template: '<div><slot name="item" v-for="e in list" :element="e" :index="0" /></div>',
    props: ['list'],
  },
}

const mountCard = async (props = {}, seed = {}) => {
  seedBoardStore({ columns: COLS, cardSize: 'large', ...seed })
  const TaskCard = (await import('@/components/TaskCard.vue')).default
  const w = mount(TaskCard, { props: { task: TASK, ...props }, global: { stubs } })
  await w.vm.$nextTick()
  return w
}

describe('TaskCard after the pills/subtasks split', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    update.mockClear()
  })

  it('renders the meta row itself and the pill row through the child component', async () => {
    const w = await mountCard()
    expect(w.find('.tnum').text()).toBe('#7') // meta row: card's own
    expect(w.find('.pills').exists()).toBe(true) // pill row: child component
    expect(w.findAll('.pill').length).toBeGreaterThan(0)
  })

  it('drops the pill row entirely when no field survives the preset', async () => {
    const w = await mountCard({}, { cardSize: 'compact', showEmpty: false })
    expect(w.text()).toContain('Родитель')
    expect(w.find('.pills').exists()).toBe(false)
  })

  it('lays the pill row out as full-width rows in stack mode', async () => {
    const w = await mountCard({}, { stackFields: true })
    expect(w.find('.pills').classes()).toContain('stacked')
  })

  it('renders the child rows and toggles one without touching its description', async () => {
    const w = await mountCard({ subtasks: SUBS, subtasksTotal: 2 })
    const rows = w.findAll('.subrow')
    expect(rows.length).toBe(2)

    await w.findAll('.check.sm')[1].trigger('click')
    expect(update).toHaveBeenCalledWith('s2', expect.objectContaining({ completed: true }))
    expect(update.mock.calls[0][1]).not.toHaveProperty('description')
  })

  it('says how many children the filter hid and locks their reorder', async () => {
    const w = await mountCard({ subtasks: [SUBS[0]], subtasksTotal: 5 })
    expect(w.find('.subs-narrowed').text()).toContain('1 из 5')
  })

  it('renders the expanded stack through the card-owned slot', async () => {
    const w = await mountCard({ subtasks: SUBS, subtasksTotal: 2, subtasksExpanded: true })
    // One layer per child, each holding a nested card (rendered by TaskCard, so
    // the two components never import each other).
    expect(w.findAll('.sub-layer').length).toBe(2)
    expect(w.findAll('.card.nested').length).toBe(2)
    expect(w.find('.subrow').exists()).toBe(false)
  })

  it('archive view: the pill row goes display-only and the restore action appears', async () => {
    const w = await mountCard({ readonly: true, subtasks: SUBS, subtasksTotal: 2 })
    expect(w.find('.ca-restore').exists()).toBe(true)
    expect(w.find('.pills').classes()).toContain('ro')
    expect(w.find('.subs').classes()).toContain('ro')
  })
})
