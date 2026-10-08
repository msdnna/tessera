// v2 (#2970): saving moved into the board's view panel (a bottom sheet on a
// phone), so the phone menu only lists the views, ticks the loaded one and can
// drop it. The history below is why it lists them at all.
//
// Saved board views had exactly two triggers — the folder (load) and the disk
// (save) in the composer's right-hand tools. Those tools are the same strip that
// hides the customize gear: `.bar-tools.hidden` sets `max-width: 0` AND
// `pointer-events: none` as soon as the chips plus the reserved search slice reach
// 75% of the bar, i.e. one or two facets on a phone. The gear at least got a
// fallback entry in BoardMobileMenu (cx-board-customize-reach); the views had none
// anywhere in the app — no route, no sidebar menu, nothing — so a view saved on a
// desktop could neither be loaded nor re-saved from a phone.
//
// Pinned here: the phone menu lists this layout's views and offers a save entry, it
// ticks the loaded one, it asks through the store (nonced, so the same view can be
// re-applied twice), and KanbanBoard both publishes the list and answers the ask.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { useBoardViewStore } from '@/stores/boardView'

const route = { query: {} }
const push = vi.fn()
const replace = vi.fn()
vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ push, replace }),
}))

const { default: BoardMobileMenu } = await import('@/components/BoardMobileMenu.vue')

const STUBS = { TagManager: true, MilestoneManager: true, TesseraIcon: true }
const flat = (options) => options.flatMap((o) => (o.children ? [o, ...o.children] : [o]))
const keys = (w) => flat(w.vm.options).map((o) => o.key)

function menuWithViews(views, current = '') {
  const w = mount(BoardMobileMenu, { global: { stubs: STUBS } })
  const store = useBoardViewStore()
  store.savedViews = views
  store.currentViewId = current
  return { w, store }
}

beforeEach(() => {
  setActivePinia(createPinia())
  route.query = {}
  push.mockClear()
  replace.mockClear()
})

describe('BoardMobileMenu saved views', () => {
  it('lists every view of the board, whatever view kind it was saved in', () => {
    const { w } = menuWithViews([
      { id: 'v1', name: 'Мои', layout: 'board' },
      { id: 'v2', name: 'Гант', layout: 'gantt' },
    ])
    expect(keys(w)).toContain('view:v1')
    expect(keys(w)).toContain('view:v2')
    w.unmount()
  })

  it('has no views group and no save prompt of its own — saving lives in the panel', () => {
    const { w } = menuWithViews([])
    expect(keys(w)).not.toContain('views')
    expect(keys(w)).not.toContain('view-save')
    expect(keys(w)).toContain('customize')
    w.unmount()
  })

  it('asks the board to apply the chosen view, by id and with a fresh nonce', () => {
    const { w, store } = menuWithViews([{ id: 'v1', name: 'Мои', layout: 'board' }])
    w.vm.onSelect('view:v1')
    expect(store.viewRequest).toMatchObject({ op: 'apply', id: 'v1' })
    const first = store.viewRequest.nonce
    // Loading the same view twice is a real ask — it re-applies a toolbar the
    // reader has since changed — so the request has to look new every time.
    w.vm.onSelect('view:v1')
    expect(store.viewRequest.nonce).toBe(first + 1)
    w.unmount()
  })

  it('offers «Без представления» only while a view is loaded, and asks to clear', () => {
    const { w, store } = menuWithViews([{ id: 'v1', name: 'Мои', layout: 'board' }])
    expect(keys(w)).not.toContain('view-none')
    store.currentViewId = 'v1'
    expect(keys(w)).toContain('view-none')
    w.vm.onSelect('view-none')
    expect(store.viewRequest).toMatchObject({ op: 'clear' })
    w.unmount()
  })

  it('does not navigate — a view is board state, not a route', () => {
    const { w } = menuWithViews([{ id: 'v1', name: 'Мои', layout: 'board' }])
    w.vm.onSelect('view:v1')
    expect(push).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
    w.unmount()
  })

  it('marks the loaded view, like the active layout', () => {
    const { w } = menuWithViews([{ id: 'v1', name: 'Мои', layout: 'board' }], 'v1')
    expect(w.vm.isActive({ key: 'view:v1' })).toBe(true)
    expect(w.vm.isActive({ key: 'view:v2' })).toBe(false)
    expect(w.vm.items.find((i) => i.key === 'view:v1').active).toBe(true)
    w.unmount()
  })

  it('marks nothing when no view is loaded', () => {
    const { w } = menuWithViews([{ id: 'v1', name: '', layout: 'board' }], '')
    expect(w.vm.isActive({ key: 'view:v1' })).toBe(false)
    w.unmount()
  })
})

describe('boardView store', () => {
  it('reset clears the published views and any pending request', () => {
    const store = useBoardViewStore()
    store.savedViews = [{ id: 'v1', name: 'Мои' }]
    store.currentViewId = 'v1'
    store.requestApplyView('v1')
    store.reset()
    expect(store.savedViews).toEqual([])
    expect(store.currentViewId).toBe('')
    expect(store.viewRequest).toBe(null)
  })
})

// KanbanBoard is too heavy to mount (it loads a board over the API on create), so
// the other half of the wiring is read off the source.
const kanban = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

describe('KanbanBoard answers the ask', () => {
  it('publishes the whole list and the loaded view', () => {
    expect(kanban).toContain('boardViewStore.savedViews = bv.views.map')
    expect(kanban).toContain('boardViewStore.currentViewId = bv.loadedId')
  })

  it('applies and clears through useBoardViews, which owns the config', () => {
    expect(kanban).toContain('() => boardViewStore.viewRequest')
    expect(kanban).toContain('if (v) bv.apply(v)')
    expect(kanban).toContain("else if (req.op === 'clear') bv.clear()")
  })

  it('has no folder / disk buttons any more — the chip and the panel replaced them', () => {
    expect(kanban).not.toContain('showLoadView')
    expect(kanban).not.toContain('showSaveView')
    expect(kanban).toContain('<ViewChip')
    expect(kanban).toContain('<ViewsSection')
  })
})
