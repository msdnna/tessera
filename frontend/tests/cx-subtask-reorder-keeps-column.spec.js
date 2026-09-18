// Re-ordering the child list on a board card must not move the child (#2932).
//
// A subtask legitimately stands in another column than its parent: the row draws
// a divergence chip for it (utils/status `divergedColumn`), and the right-click
// menu on that very row is how it gets there (useTaskMenu's `col:` item →
// PATCH /tasks/:id/move). The on-card drag-reorder sent `column_id:
// props.task.column_id` with every `moved` event, so the reorder doubled as a
// move back into the parent's column — and PATCH move is anything but inert on
// the server (backend handlers/task_ops.go applyMove): it logs a «перемещена»
// journal entry, notifies the task's participants, and auto-toggles completion
// against the board's done column. A parent standing in «Готово» therefore
// CLOSED every child the reader reordered under it.
//
// Two halves, because either alone can be green while the feature is broken: the
// request must carry the child's OWN column, and it must still carry the sibling
// neighbours (without them the backend's positionBetween(nil, nil) returns a
// constant and the child jumps within the list — the reorder would persist as a
// no-op).
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'
import { seedBoardStore } from './helpers/boardStore'

const move = vi.fn(() => Promise.resolve({ data: {} }))
const setParent = vi.fn(() => Promise.resolve({ data: {} }))
vi.mock('@/api', () => ({
  tasks: {
    move: (...a) => move(...a),
    setParent: (...a) => setParent(...a),
    update: vi.fn(() => Promise.resolve({ data: {} })),
  },
}))

const COLS = [
  { id: 'c1', name: 'К работе', color: '#9aa0aa', position: 1 },
  { id: 'c2', name: 'В процессе', color: '#2f80ed', position: 2 },
]
// The parent sits in c1; the middle child was moved to c2 by hand and carries the
// divergence chip.
const PARENT = { id: 't1', board_id: 'b1', column_id: 'c1', title: 'Родитель' }
const SUBS = [
  { id: 's1', title: 'Первая', column_id: 'c1', parent_id: 't1' },
  { id: 's2', title: 'Вторая', column_id: 'c2', parent_id: 't1' },
  { id: 's3', title: 'Третья', column_id: 'c1', parent_id: 't1' },
]

// vuedraggable stands in as a stub that keeps its name, its `list` prop and its
// `change` event: SortableJS needs a real pointer device and its `updated` hook
// throws under jsdom, so the real one cannot be driven from a spec. What matters
// here is the template binding `@change="onSubChange"` — emitting on the stub
// goes through that binding exactly as the real drop does. That vuedraggable
// itself forwards its options is pinned elsewhere (cx-lane-sort-scope).
const DraggableStub = {
  name: 'draggable',
  props: { list: { type: Array, default: () => [] } },
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
  // The row asks useResponsive whether it is on a phone (the hover preview is off
  // there — #2893) and jsdom ships no matchMedia, so without this nothing mounts.
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
  seedBoardStore({ columns: COLS })
  TaskCardSubtasks = (await import('@/components/task/TaskCardSubtasks.vue')).default
})

function mountList(subtasks = SUBS) {
  return mount(TaskCardSubtasks, {
    props: { task: PARENT, subtasks },
    global: { stubs },
  })
}

// Sortable re-orders the bound list itself before emitting, so the spec does the
// same: the array the component holds is the NEW order, and `newIndex` points
// into it. Emitting on the real vuedraggable child is what keeps the wiring
// (`@change="onSubChange"`) under test, not just the handler.
async function dragTo(wrapper, id, newIndex, base = SUBS) {
  const list = [...base]
  const from = list.findIndex((s) => s.id === id)
  const [el] = list.splice(from, 1)
  list.splice(newIndex, 0, el)
  await wrapper.setProps({ subtasks: list })
  const dr = wrapper.findComponent(DraggableStub)
  // The component must have handed the stub the NEW order — that array is what
  // the neighbour arithmetic reads.
  expect(dr.props('list').map((s) => s.id)).toEqual(list.map((s) => s.id))
  await dr.vm.$emit('change', { moved: { element: el, oldIndex: from, newIndex } })
}

describe('on-card subtask reorder', () => {
  it('keeps a diverged child in its own column', async () => {
    const w = mountList()
    await dragTo(w, 's2', 0)
    expect(move).toHaveBeenCalledTimes(1)
    expect(move.mock.calls[0][0]).toBe('s2')
    expect(move.mock.calls[0][1].column_id).toBe('c2')
    w.unmount()
  })

  it('never sends the parent column for a child that is not in it', async () => {
    const w = mountList()
    await dragTo(w, 's2', 2)
    expect(move.mock.calls[0][1].column_id).not.toBe(PARENT.column_id)
    w.unmount()
  })

  it('still sends the child column for a child that agrees with the parent', async () => {
    const w = mountList()
    await dragTo(w, 's3', 0)
    expect(move.mock.calls[0][1].column_id).toBe('c1')
    w.unmount()
  })

  it('falls back to the parent column when the row carries none', async () => {
    const rows = [{ id: 's9', title: 'Без колонки', parent_id: 't1' }, ...SUBS]
    const w = mountList(rows)
    await dragTo(w, 's9', 1, rows)
    expect(move.mock.calls[0][1].column_id).toBe('c1')
    w.unmount()
  })

  it('sends the neighbours the child landed between', async () => {
    const w = mountList()
    // s2 → the end: [s1, s3, s2]
    await dragTo(w, 's2', 2)
    expect(move.mock.calls[0][1]).toMatchObject({ before_id: 's3', after_id: null })
    w.unmount()
  })

  it('takes neighbours from every sibling, whatever column each stands in', async () => {
    const w = mountList()
    // s3 → the middle: [s1, s3, s2]. The follower is the diverged s2 — a subtask's
    // float8 ladder is per parent (backend `sameList`), so it is a valid neighbour.
    await dragTo(w, 's3', 1)
    expect(move.mock.calls[0][1]).toMatchObject({ before_id: 's1', after_id: 's2' })
    w.unmount()
  })

  it('nests a card dropped in without touching the move endpoint', async () => {
    const w = mountList()
    const dr = w.findComponent(DraggableStub)
    await dr.vm.$emit('change', { added: { element: { id: 'x1' }, newIndex: 0 } })
    expect(setParent).toHaveBeenCalledWith('x1', 't1')
    expect(move).not.toHaveBeenCalled()
    w.unmount()
  })
})
