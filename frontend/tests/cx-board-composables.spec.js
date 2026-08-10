import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { ref, computed, defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'

import { useCardViewport, cardKey, VCARD_EST } from '@/composables/useCardViewport'
import { useBoardDragScroll } from '@/composables/useBoardDragScroll'
import { useBoardViewConfig, VIEW_PERSIST_MS } from '@/composables/useBoardViewConfig'

// jsdom ships neither observer, and both composables build them in onMounted —
// these fakes let a test drive the callbacks by hand.
class FakeObserver {
  constructor(cb, opts) {
    this.cb = cb
    this.opts = opts
    this.observed = []
    this.disconnected = false
    this.constructor.instances.push(this)
  }
  observe(el) {
    this.observed.push(el)
  }
  unobserve(el) {
    this.observed = this.observed.filter((x) => x !== el)
  }
  disconnect() {
    this.disconnected = true
  }
}
class FakeIO extends FakeObserver {}
class FakeRO extends FakeObserver {}
FakeIO.instances = []
FakeRO.instances = []

beforeEach(() => {
  FakeIO.instances = []
  FakeRO.instances = []
  vi.stubGlobal('IntersectionObserver', FakeIO)
  vi.stubGlobal('ResizeObserver', FakeRO)
})
afterEach(() => vi.unstubAllGlobals())

// Mount a composable inside a real component so its lifecycle hooks run.
function host(setup) {
  let api
  const wrapper = mount(
    defineComponent({
      setup() {
        api = setup()
        return () => h('div')
      },
    }),
  )
  return { api, wrapper }
}

describe('cardKey', () => {
  // Under tag grouping one task appears in several columns at once; a bare-id key
  // would make those instances share visibility and height.
  it('scopes a task to its column', () => {
    expect(cardKey('col-a', 't1')).toBe('col-a::t1')
    expect(cardKey('col-a', 't1')).not.toBe(cardKey('col-b', 't1'))
  })
})

describe('useCardViewport', () => {
  const wrapperEl = (placeholder = false) => {
    const el = document.createElement('div')
    if (placeholder) {
      const ph = document.createElement('div')
      ph.classList.add('card-ph')
      el.appendChild(ph)
    }
    return el
  }

  it('registering a card tags it and hands it to both observers', () => {
    const { api } = host(() => useCardViewport({ frozen: ref(false) }))
    const el = wrapperEl()
    api.regCard(el, 'col::t1')
    expect(el.dataset.cardId).toBe('col::t1')
    expect(FakeIO.instances[0].observed).toContain(el)
    expect(FakeRO.instances[0].observed).toContain(el)
  })

  it('ignores a null element (the ref callback fires with null on unmount)', () => {
    const { api } = host(() => useCardViewport({ frozen: ref(false) }))
    expect(() => api.regCard(null, 'col::t1')).not.toThrow()
    expect(FakeIO.instances[0].observed).toHaveLength(0)
  })

  it('intersection entries drive visibility', () => {
    const { api } = host(() => useCardViewport({ frozen: ref(false) }))
    const el = wrapperEl()
    api.regCard(el, 'col::t1')
    FakeIO.instances[0].cb([{ target: el, isIntersecting: true }])
    expect(api.vis['col::t1']).toBe(true)
    FakeIO.instances[0].cb([{ target: el, isIntersecting: false }])
    expect(api.vis['col::t1']).toBe(false)
  })

  it('freezes visibility swaps while a drag is in flight', () => {
    // SortableJS needs a stable DOM for the duration of a drag: collapsing a card
    // mid-drag would pull the drop target out from under it.
    const frozen = ref(false)
    const { api } = host(() => useCardViewport({ frozen }))
    const el = wrapperEl()
    api.regCard(el, 'col::t1')
    frozen.value = true
    FakeIO.instances[0].cb([{ target: el, isIntersecting: true }])
    expect(api.vis['col::t1']).toBeUndefined()
    frozen.value = false
    FakeIO.instances[0].cb([{ target: el, isIntersecting: true }])
    expect(api.vis['col::t1']).toBe(true)
  })

  it('records measured heights but never a placeholder height', () => {
    // Feeding a placeholder's own height back in would freeze the card at the
    // estimate forever.
    const { api } = host(() => useCardViewport({ frozen: ref(false) }))
    const real = wrapperEl()
    const collapsed = wrapperEl(true)
    api.regCard(real, 'col::t1')
    api.regCard(collapsed, 'col::t2')
    FakeRO.instances[0].cb([
      { target: real, contentRect: { height: 212.4 } },
      { target: collapsed, contentRect: { height: VCARD_EST } },
    ])
    expect(api.cardH['col::t1']).toBe(212)
    expect(api.cardH['col::t2']).toBeUndefined()
  })

  it('ignores a zero-height measurement', () => {
    const { api } = host(() => useCardViewport({ frozen: ref(false) }))
    const el = wrapperEl()
    api.regCard(el, 'col::t1')
    FakeRO.instances[0].cb([{ target: el, contentRect: { height: 0 } }])
    expect(api.cardH['col::t1']).toBeUndefined()
  })

  it('reset drops the previous board state without replacing the objects', () => {
    // The template holds these reactive objects by reference — reassigning them
    // would leave the render reading a detached copy.
    const { api } = host(() => useCardViewport({ frozen: ref(false) }))
    const el = wrapperEl()
    api.regCard(el, 'col::t1')
    FakeIO.instances[0].cb([{ target: el, isIntersecting: true }])
    FakeRO.instances[0].cb([{ target: el, contentRect: { height: 200 } }])
    const visRef = api.vis
    api.reset()
    expect(api.vis).toBe(visRef)
    expect(Object.keys(api.vis)).toHaveLength(0)
    expect(Object.keys(api.cardH)).toHaveLength(0)
  })

  it('disconnects both observers on unmount', () => {
    const { wrapper } = host(() => useCardViewport({ frozen: ref(false) }))
    wrapper.unmount()
    expect(FakeIO.instances[0].disconnected).toBe(true)
    expect(FakeRO.instances[0].disconnected).toBe(true)
  })
})

describe('useBoardDragScroll', () => {
  const mountDrag = () => {
    const el = document.createElement('div')
    el.scrollTo = vi.fn()
    const scrollEl = ref(el)
    const { api, wrapper } = host(() =>
      useBoardDragScroll({ scrollEl, colWidth: computed(() => 300), gap: 12 }),
    )
    return { api, wrapper, el }
  }

  it('a column drag raises the shared flag but not the card-only one', () => {
    // draggingCard gates the per-card nest dropzone hint: a column drag flipping
    // it would flash a dashed hint on every childless card.
    const { api } = mountDrag()
    api.onDragStart()
    expect(api.dragging.value).toBe(true)
    expect(api.draggingCard.value).toBe(false)
  })

  it('a card drag raises both', () => {
    const { api } = mountDrag()
    api.onCardDragStart()
    expect(api.dragging.value).toBe(true)
    expect(api.draggingCard.value).toBe(true)
  })

  it('suspends scroll-snap and smooth scrolling for the drag, then restores them', () => {
    // Both revert the per-frame scrollLeft nudges the autoscroll makes on mobile.
    const { api, el } = mountDrag()
    api.onCardDragStart()
    expect(el.style.scrollSnapType).toBe('none')
    expect(el.style.scrollBehavior).toBe('auto')
    api.onDragEnd()
    expect(el.style.scrollSnapType).toBe('')
    expect(el.style.scrollBehavior).toBe('')
    expect(api.dragging.value).toBe(false)
    expect(api.draggingCard.value).toBe(false)
  })

  it('drops the dragover listener when the drag ends', async () => {
    const add = vi.spyOn(window, 'addEventListener')
    const remove = vi.spyOn(window, 'removeEventListener')
    const { api } = mountDrag()
    api.onDragStart()
    await nextTick()
    expect(add).toHaveBeenCalledWith('dragover', expect.any(Function), { passive: true })
    api.onDragEnd()
    expect(remove).toHaveBeenCalledWith('dragover', expect.any(Function))
    add.mockRestore()
    remove.mockRestore()
  })

  it('unmounting mid-drag tears the drag down', () => {
    // Otherwise the rAF loop and the window listener keep running against a
    // detached element after navigating away.
    const remove = vi.spyOn(window, 'removeEventListener')
    const { api, wrapper } = mountDrag()
    api.onCardDragStart()
    wrapper.unmount()
    expect(api.dragging.value).toBe(false)
    expect(remove).toHaveBeenCalledWith('dragover', expect.any(Function))
    remove.mockRestore()
  })
})

describe('useBoardViewConfig', () => {
  // A toolbar snapshot is just `{ groupMode }` here — the composable is deliberately
  // blind to the real shape, which stays in KanbanBoard.
  function mountView({ boardId = 'b1', layout: initial = 'board' } = {}) {
    const id = ref(boardId)
    const layout = ref(initial)
    const state = ref('')
    const load = vi.fn((s) => (state.value = s.groupMode))
    const defaults = vi.fn((l) => ({ groupMode: l === 'timeline' ? 'assignee' : 'status' }))
    const snapshot = vi.fn(() => ({ groupMode: state.value }))
    const migrate = vi.fn((v, l) => ({ ...defaults(l), groupMode: v.groupMode }))
    const { api, wrapper } = host(() =>
      useBoardViewConfig({ boardId: id, layout, defaults, snapshot, load, migrate }),
    )
    return { api, wrapper, layout, state, load, defaults, migrate }
  }
  function stored(boardId = 'b1') {
    const raw = localStorage.getItem(`tessera_view_${boardId}`)
    return raw === null ? null : JSON.parse(raw)
  }

  beforeEach(() => {
    localStorage.clear()
    vi.useFakeTimers()
  })
  afterEach(() => vi.useRealTimers())

  it('restores the defaults of the current layout when nothing is stored', () => {
    const { api, state, defaults } = mountView({ layout: 'timeline' })
    api.restoreView()
    expect(defaults).toHaveBeenCalledWith('timeline')
    expect(state.value).toBe('assignee')
  })

  it('restores the stored layout and that layout slot', () => {
    localStorage.setItem(
      'tessera_view_b1',
      JSON.stringify({ layout: 'timeline', toolbars: { timeline: { groupMode: 'tag' } } }),
    )
    const { api, layout, state } = mountView()
    api.restoreView()
    expect(layout.value).toBe('timeline')
    expect(state.value).toBe('tag')
  })

  it('routes a pre-per-layout blob through migrate', () => {
    localStorage.setItem('tessera_view_b1', JSON.stringify({ groupMode: 'tag' }))
    const { api, state, migrate } = mountView()
    api.restoreView()
    expect(migrate).toHaveBeenCalledWith({ groupMode: 'tag' }, 'board')
    expect(state.value).toBe('tag')
  })

  it('falls back to the defaults when the stored blob is corrupt', () => {
    localStorage.setItem('tessera_view_b1', 'not json')
    const { api, state } = mountView()
    api.restoreView()
    expect(state.value).toBe('status')
  })

  it('debounces the write into one slot per layout', () => {
    const { api, state } = mountView()
    state.value = 'tag'
    api.persistView()
    api.persistView()
    expect(stored()).toBe(null)
    vi.advanceTimersByTime(VIEW_PERSIST_MS)
    expect(stored()).toEqual({ layout: 'board', toolbars: { board: { groupMode: 'tag' } } })
  })

  it('does not persist while a restore is in flight', async () => {
    // The deep watcher in KanbanBoard fires as `load` writes the very refs it
    // watches; without the mutex that write would be persisted straight back.
    const { api } = mountView()
    api.restoreView()
    api.persistView()
    vi.advanceTimersByTime(VIEW_PERSIST_MS)
    expect(stored()).toBe(null)
    await nextTick()
    api.persistView()
    vi.advanceTimersByTime(VIEW_PERSIST_MS)
    expect(stored().toolbars.board).toEqual({ groupMode: 'status' })
  })

  it('a layout swap banks the old layout and loads the new one', async () => {
    const { api, layout, state } = mountView()
    api.restoreView()
    await nextTick()
    state.value = 'tag'
    layout.value = 'timeline'
    await nextTick()
    expect(state.value).toBe('assignee') // the defaults of the layout we switched to
    await nextTick()
    vi.advanceTimersByTime(VIEW_PERSIST_MS)
    expect(stored()).toEqual({
      layout: 'timeline',
      toolbars: { board: { groupMode: 'tag' }, timeline: { groupMode: 'assignee' } },
    })
  })

  it('guard keeps the swap watcher off a bulk apply that sets the layout itself', async () => {
    const { api, layout, state, load } = mountView()
    api.restoreView()
    await nextTick()
    load.mockClear()
    // Applying a saved view sets layout AND the toolbar fields; the swap must not
    // fire on top and clobber them with the other layout's slot.
    api.guard(() => {
      layout.value = 'timeline'
      state.value = 'tag'
    })
    await nextTick()
    expect(load).not.toHaveBeenCalled()
    expect(state.value).toBe('tag')
  })

  it('flushes a pending write on unmount', () => {
    const { api, wrapper, state } = mountView()
    state.value = 'tag'
    api.persistView()
    wrapper.unmount()
    expect(stored().toolbars.board).toEqual({ groupMode: 'tag' })
    // The pending timer was cleared, not left to fire a second time.
    state.value = 'status'
    vi.advanceTimersByTime(VIEW_PERSIST_MS)
    expect(stored().toolbars.board).toEqual({ groupMode: 'tag' })
  })
})
