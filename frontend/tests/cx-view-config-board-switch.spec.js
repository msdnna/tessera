import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { ref, defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'

import { useBoardViewConfig, VIEW_PERSIST_MS } from '@/composables/useBoardViewConfig'

// KanbanBoard does NOT remount when you switch boards: `watch(() => props.boardId)`
// re-calls `restoreView()` on the *same* `useBoardViewConfig` instance. Everything
// the composable holds between calls therefore belongs to the previous board until
// `restoreView` re-points it — and two things did not get re-pointed:
//
//   • `byLayout`, the per-layout slot map. `restoreView` only `Object.assign`s the
//     stored blob over it, so a layout the new board has no slot for kept the *old*
//     board's slot: a layout swap loaded another board's grouping/filters, and
//     `writeView` (which stores the whole map) copied that slot into the new
//     board's own localStorage key.
//   • the debounced `persistTimer`. Armed on board A, it fired 300 ms later under
//     board B's key: A's last toolbar change was lost and B's slot was overwritten.
//
// The toolbar snapshot here is just `{ groupMode }` — the composable is blind to the
// real shape, which stays in KanbanBoard (see cx-board-composables.spec.js).
describe('useBoardViewConfig across a board switch', () => {
  function mountView({ boardId = 'b1', layout: initial = 'board' } = {}) {
    const id = ref(boardId)
    const layout = ref(initial)
    const state = ref('')
    const load = vi.fn((s) => (state.value = s.groupMode))
    const defaults = vi.fn((l) => ({ groupMode: l === 'timeline' ? 'assignee' : 'status' }))
    const snapshot = vi.fn(() => ({ groupMode: state.value }))
    const migrate = vi.fn((v, l) => ({ ...defaults(l), groupMode: v.groupMode }))
    let api
    const wrapper = mount(
      defineComponent({
        setup() {
          api = useBoardViewConfig({ boardId: id, layout, defaults, snapshot, load, migrate })
          return () => h('div')
        },
      }),
    )
    return { api, wrapper, id, layout, state, load }
  }
  function stored(boardId) {
    const raw = localStorage.getItem(`tessera_view_${boardId}`)
    return raw === null ? null : JSON.parse(raw)
  }
  function put(boardId, blob) {
    localStorage.setItem(`tessera_view_${boardId}`, JSON.stringify(blob))
  }

  beforeEach(() => {
    localStorage.clear()
    vi.useFakeTimers()
  })
  afterEach(() => vi.useRealTimers())

  // Walk the caller's board switch: new id, then `restoreView`, then let the mutex
  // settle — exactly the order of KanbanBoard's boardId watcher.
  async function switchTo({ id, api }, boardId) {
    id.value = boardId
    await nextTick()
    api.restoreView()
    await nextTick()
  }

  it('a board with no stored slot for a layout gets the defaults, not the previous board', async () => {
    put('b1', {
      layout: 'board',
      toolbars: { board: { groupMode: 'tag' }, timeline: { groupMode: 'tag' } },
    })
    const h1 = mountView()
    h1.api.restoreView()
    await nextTick()
    expect(h1.state.value).toBe('tag')

    await switchTo(h1, 'b2') // never visited
    expect(h1.state.value).toBe('status') // board defaults
    h1.layout.value = 'timeline'
    await nextTick()
    // b1's timeline slot must not be what b2's timeline shows.
    expect(h1.state.value).toBe('assignee')
  })

  it('does not copy the previous board slots into the new board key', async () => {
    put('b1', {
      layout: 'board',
      toolbars: { board: { groupMode: 'tag' }, timeline: { groupMode: 'tag' } },
    })
    const h1 = mountView()
    h1.api.restoreView()
    await nextTick()

    await switchTo(h1, 'b2')
    h1.state.value = 'priority'
    h1.api.persistView()
    vi.advanceTimersByTime(VIEW_PERSIST_MS)
    expect(stored('b2')).toEqual({
      layout: 'board',
      toolbars: { board: { groupMode: 'priority' } },
    })
  })

  it('keeps the new board slots it does have', async () => {
    put('b2', { layout: 'board', toolbars: { board: { groupMode: 'priority' } } })
    const h1 = mountView()
    h1.api.restoreView()
    await nextTick()

    await switchTo(h1, 'b2')
    expect(h1.state.value).toBe('priority')
  })

  it('banks a pending write on the board it was made for', async () => {
    const h1 = mountView()
    h1.api.restoreView()
    await nextTick()
    h1.state.value = 'tag'
    h1.api.persistView() // armed, not yet fired

    await switchTo(h1, 'b2')
    expect(stored('b1')).toEqual({ layout: 'board', toolbars: { board: { groupMode: 'tag' } } })
  })

  it('does not let a pending write from the old board land on the new one', async () => {
    const h1 = mountView()
    h1.api.restoreView()
    await nextTick()
    h1.state.value = 'tag'
    h1.api.persistView()

    await switchTo(h1, 'b2')
    vi.advanceTimersByTime(VIEW_PERSIST_MS * 4)
    expect(stored('b2')).toBe(null)
  })

  it('still persists normally on the board it switched to', async () => {
    const h1 = mountView()
    h1.api.restoreView()
    await nextTick()
    await switchTo(h1, 'b2')
    h1.state.value = 'tag'
    h1.api.persistView()
    vi.advanceTimersByTime(VIEW_PERSIST_MS)
    expect(stored('b2').toolbars.board).toEqual({ groupMode: 'tag' })
    expect(stored('b1')).toBe(null) // nothing was pending when we left b1
  })

  it('a restore on the same board keeps that board per-layout slots', async () => {
    put('b1', {
      layout: 'board',
      toolbars: { board: { groupMode: 'tag' }, timeline: { groupMode: 'due' } },
    })
    const h1 = mountView()
    h1.api.restoreView()
    await nextTick()
    h1.api.restoreView() // same board (e.g. a scope reload)
    await nextTick()
    h1.layout.value = 'timeline'
    await nextTick()
    expect(h1.state.value).toBe('due')
  })

  it('flushes a pending write on unmount', async () => {
    const h1 = mountView()
    h1.api.restoreView()
    await nextTick()
    h1.state.value = 'tag'
    h1.api.persistView()
    h1.wrapper.unmount()
    expect(stored('b1').toolbars.board).toEqual({ groupMode: 'tag' })
  })
})
