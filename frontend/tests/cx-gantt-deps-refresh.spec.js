// The Gantt's dependency arrows against relations changed elsewhere (#2932).
//
// The edge list is its own fetch (/boards/:id/dependencies), separate from the
// board's task lists, and the view refetched it only when the *set* of task ids
// changed — which adding or dropping a relation never does. So an arrow created
// in the task modal, by a second browser or by a quick action stayed invisible
// (and a deleted one stayed drawn) until a full reload, even though the watcher
// comment claimed it covered "a link added elsewhere".
//
// The board now routes a `task.relation` socket event into a token prop; these
// pin both halves — the routing table leaves the event to that branch, and the
// chart re-reads its edges when the token moves.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

const dependencies = vi.fn()
vi.mock('@/api', () => ({
  tasks: { update: vi.fn(), addRelation: vi.fn(), removeRelation: vi.fn() },
  boards: { dependencies: (...a) => dependencies(...a) },
}))

import BoardGanttView from '@/components/BoardGanttView.vue'
import { classifyEvent, relationTouchesBoard } from '@/utils/boardEvents'

const stubs = {
  NIcon: { template: '<i />' },
  NTooltip: { template: '<div><slot name="trigger" /><slot /></div>' },
  NDropdown: { template: '<div><slot /></div>' },
  NPopover: { template: '<div><slot name="trigger" /><slot /></div>' },
}

const task = (id, number) => ({
  id,
  number,
  title: 'Задача ' + number,
  priority: 2,
  start_date: '2026-01-05T00:00:00Z',
  due_date: '2026-01-09T00:00:00Z',
})

function mountGantt(props = {}) {
  return mount(BoardGanttView, {
    props: {
      boardId: 'b1',
      tasks: [task('t1', 1), task('t2', 2)],
      statusColumns: [{ id: 'c1', name: 'К работе' }],
      ...props,
    },
    global: { stubs },
  })
}

describe('Gantt dependency refresh', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    dependencies.mockReset()
    dependencies.mockResolvedValue({ data: [] })
  })

  it('re-reads the edges when the relation token moves', async () => {
    const w = mountGantt()
    await nextTick()
    const onMount = dependencies.mock.calls.length
    expect(onMount).toBeGreaterThan(0)

    await w.setProps({ depsToken: 1 })
    await nextTick()
    expect(dependencies.mock.calls.length).toBe(onMount + 1)

    // A token that does not move must not cost a request — the board bumps it
    // only for relation events, but props churn on every task patch.
    await w.setProps({ depsToken: 1, tagPrefix: 'area/' })
    await nextTick()
    expect(dependencies.mock.calls.length).toBe(onMount + 1)
    w.unmount()
  })

  it('leaves task.relation to the board own branch instead of a task re-fetch', () => {
    // Relations live on no card (the board rows carry none), so the generic
    // unknown-type fallback would spend a task-list fetch on every link.
    expect(
      classifyEvent({ type: 'task.relation', data: { board_id: 'b1' } }, { boardId: 'b1' }),
    ).toBe('ignore')
  })

  it('routes a relation event by either endpoint board', () => {
    const ev = (data) => ({ type: 'task.relation', data })
    expect(relationTouchesBoard(ev({ board_id: 'b1', related_board_id: 'b2' }), 'b1')).toBe(true)
    // The far end counts too: that pair is how an arrow disappears from b1 when
    // the other task leaves the board.
    expect(relationTouchesBoard(ev({ board_id: 'b2', related_board_id: 'b1' }), 'b1')).toBe(true)
    expect(relationTouchesBoard(ev({ board_id: 'b2', related_board_id: 'b3' }), 'b1')).toBe(false)
    expect(relationTouchesBoard({ type: 'task.updated', data: { board_id: 'b1' } }, 'b1')).toBe(
      false,
    )
    expect(relationTouchesBoard(ev({ board_id: 'b1' }), '')).toBe(false)
  })
})
