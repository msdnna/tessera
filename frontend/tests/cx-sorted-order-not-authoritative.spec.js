// A composer sort takes drag-reorder off the board, on both levels (#2932).
//
// `canSortWithinLane` already refused the gesture in a tag/milestone lane, where
// the lane has no stored order to write back. A manual sort breaks the same
// assumption from the other side: a status column DOES own its cards' order, but
// the board no longer draws that order — `lists` is built from `filteredTasks` =
// `sortByLevels(...)`, and each card's children come through
// `sortedSubtasksByParent`. The neighbours a drop names are then the SORT's
// neighbours, whose `position` values stand in arbitrary relative order, and the
// backend's midpoint of a swapped pair is a perfectly valid float pointing at a
// random slot of the stored ladder. Nothing showed on screen (the reload
// re-sorts); the scrambled order surfaced for everyone once the sort came off.
//
// Three halves, because any one of them alone can be green while the gesture is
// still live: the pure rule (ut-boardDnd-sorted), the bindings that carry it, and
// the child-row handler that reads its neighbours off the sorted array.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'
import { seedBoardStore } from './helpers/boardStore'

const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')
const card = readFileSync(resolve(root, 'src/components/TaskCard.vue'), 'utf8')
const subs = readFileSync(resolve(root, 'src/components/task/TaskCardSubtasks.vue'), 'utf8')

const move = vi.fn(() => Promise.resolve({ data: {} }))
const setParent = vi.fn(() => Promise.resolve({ data: {} }))
vi.mock('@/api', () => ({
  tasks: {
    move: (...a) => move(...a),
    setParent: (...a) => setParent(...a),
    update: vi.fn(() => Promise.resolve({ data: {} })),
  },
}))

const COLS = [{ id: 'c1', name: 'К работе', color: '#9aa0aa', position: 1 }]
const PARENT = { id: 't1', board_id: 'b1', column_id: 'c1', title: 'Родитель' }
// Drawn by priority ↓ — the stored order is s1, s2, s3.
const SUBS = [
  { id: 's3', title: 'Третья', column_id: 'c1', parent_id: 't1', priority: 3 },
  { id: 's1', title: 'Первая', column_id: 'c1', parent_id: 't1', priority: 2 },
  { id: 's2', title: 'Вторая', column_id: 'c1', parent_id: 't1', priority: 1 },
]

// Same stub as cx-subtask-reorder-keeps-column: SortableJS needs a real pointer
// device and its `updated` hook throws under jsdom, so the drop is emitted through
// the template binding instead. `sort` is kept as a prop so the binding itself can
// be read back (that vuedraggable forwards it to SortableJS is pinned in
// cx-lane-sort-scope).
const DraggableStub = {
  name: 'draggable',
  props: { list: { type: Array, default: () => [] }, sort: { type: Boolean, default: true } },
  emits: ['change'],
  template: '<div class="drag-stub"><slot name="item" v-for="s in list" :element="s" /></div>',
}

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  draggable: DraggableStub,
  TaskMiniCard: true,
}

let TaskCardSubtasks

beforeEach(async () => {
  move.mockClear()
  setParent.mockClear()
  setActivePinia(createPinia())
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
  seedBoardStore({ columns: COLS })
  TaskCardSubtasks = (await import('@/components/task/TaskCardSubtasks.vue')).default
})

function mountList(sorted) {
  return mount(TaskCardSubtasks, {
    props: { task: PARENT, subtasks: SUBS, sorted },
    global: { stubs },
  })
}

async function dropRow(wrapper, id, newIndex) {
  const list = [...SUBS]
  const from = list.findIndex((s) => s.id === id)
  const [el] = list.splice(from, 1)
  list.splice(newIndex, 0, el)
  await wrapper.setProps({ subtasks: list })
  const dr = wrapper.findComponent(DraggableStub)
  await dr.vm.$emit('change', { moved: { element: el, oldIndex: from, newIndex } })
  return dr
}

describe('child rows on a card, while a sort is active', () => {
  it('takes re-ordering off the list', async () => {
    const w = mountList(true)
    expect(w.findComponent(DraggableStub).props('sort')).toBe(false)
    w.unmount()
  })

  it('leaves it on when no sort is active', async () => {
    const w = mountList(false)
    expect(w.findComponent(DraggableStub).props('sort')).toBe(true)
    w.unmount()
  })

  it('persists nothing even if a drop reaches the handler', async () => {
    const w = mountList(true)
    await dropRow(w, 's2', 0)
    expect(move).not.toHaveBeenCalled()
    w.unmount()
  })

  it('but persists the same drop with the sort off', async () => {
    const w = mountList(false)
    await dropRow(w, 's2', 0)
    expect(move).toHaveBeenCalledTimes(1)
    expect(move.mock.calls[0][1]).toMatchObject({ before_id: null, after_id: 's3' })
    w.unmount()
  })

  it('still nests a card dragged in from the board', async () => {
    // The sort says nothing about WHOSE child a card is — nesting is a setParent
    // and carries no position, so it must survive.
    const w = mountList(true)
    const dr = w.findComponent(DraggableStub)
    await dr.vm.$emit('change', { added: { element: { id: 'x1' }, newIndex: 0 } })
    expect(setParent).toHaveBeenCalledWith('x1', 't1')
    expect(move).not.toHaveBeenCalled()
    w.unmount()
  })

  it('defaults to "no sort" for every other mounting of the card', () => {
    const w = mount(TaskCardSubtasks, {
      props: { task: PARENT, subtasks: SUBS },
      global: { stubs },
    })
    expect(w.findComponent(DraggableStub).props('sort')).toBe(true)
    w.unmount()
  })
})

describe('the bindings that carry the rule', () => {
  it('the board derives one flag from the composer’s sort levels', () => {
    expect(board).toMatch(/composerSorted\s*=\s*computed\(\(\)\s*=>\s*sortLevels\.value\.length/)
  })

  it('and hands it to the card-list sort rule', () => {
    expect(board).toMatch(/canSortWithinLane\(groupMode\.value,\s*composerSorted\.value\)/)
  })

  it('and to the planner, so a cross-column drop names no slot', () => {
    expect(board).toMatch(/sorted:\s*composerSorted\.value/)
  })

  it('and down to the cards, which forward it to their child list', () => {
    expect(board).toContain(':sorted="composerSorted"')
    expect(card).toContain(':sorted="sorted"')
    expect(subs).toContain(':sort="!sorted"')
  })

  it('keeps the filter lock the child list already had', () => {
    // `narrowed` disables the list outright (a partial list cannot be re-ordered
    // and must not accept a nest either); `sorted` only takes re-ordering off.
    expect(subs).toContain(':disabled="narrowed"')
  })
})
