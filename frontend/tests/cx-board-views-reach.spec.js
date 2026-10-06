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
const entry = (w, key) => flat(w.vm.options).find((o) => o.key === key)

function menuWithViews(views, current = '') {
  const w = mount(BoardMobileMenu, { global: { stubs: STUBS } })
  const store = useBoardViewStore()
  store.savedViews = views
  store.currentViewName = current
  return { w, store }
}

beforeEach(() => {
  setActivePinia(createPinia())
  route.query = {}
  push.mockClear()
  replace.mockClear()
})

describe('BoardMobileMenu saved views', () => {
  it('lists the board views the phone otherwise cannot reach', () => {
    const { w } = menuWithViews([
      { id: 'v1', name: 'Мои' },
      { id: 'v2', name: 'Спринт' },
    ])
    expect(keys(w)).toContain('view:v1')
    expect(keys(w)).toContain('view:v2')
    w.unmount()
  })

  it('offers the save entry even with no views saved yet', () => {
    const { w } = menuWithViews([])
    expect(entry(w, 'view-save')).toBeTruthy()
    w.unmount()
  })

  it('asks the board to apply the chosen view, by id and with a fresh nonce', () => {
    const { w, store } = menuWithViews([{ id: 'v1', name: 'Мои' }])
    w.vm.onSelect('view:v1')
    expect(store.viewRequest).toMatchObject({ op: 'apply', id: 'v1' })
    const first = store.viewRequest.nonce
    // Loading the same view twice is a real ask — it re-applies a toolbar the
    // reader has since changed — so the request has to look new every time.
    w.vm.onSelect('view:v1')
    expect(store.viewRequest.nonce).toBe(first + 1)
    w.unmount()
  })

  it('does not navigate — a view is board state, not a route', () => {
    const { w } = menuWithViews([{ id: 'v1', name: 'Мои' }])
    w.vm.onSelect('view:v1')
    expect(push).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
    w.unmount()
  })

  it('opens a name prompt for saving instead of saving blind, prefilled with the loaded view', () => {
    const { w, store } = menuWithViews([{ id: 'v1', name: 'Мои' }], 'Мои')
    w.vm.onSelect('view-save')
    expect(w.vm.saveOpen).toBe(true)
    expect(w.vm.saveName).toBe('Мои')
    expect(store.viewRequest).toBe(null) // nothing sent until confirmed
    w.unmount()
  })

  it('sends the trimmed name on confirm and closes the prompt', () => {
    const { w, store } = menuWithViews([])
    w.vm.onSelect('view-save')
    w.vm.saveName = '  Спринт  '
    w.vm.confirmSave()
    expect(store.viewRequest).toMatchObject({ op: 'save', name: 'Спринт' })
    expect(w.vm.saveOpen).toBe(false)
    w.unmount()
  })

  it('refuses an empty name when no view is loaded, and allows it when one is', () => {
    const { w, store } = menuWithViews([])
    w.vm.saveName = '   '
    expect(w.vm.canSave).toBe(false)
    w.vm.confirmSave()
    expect(store.viewRequest).toBe(null)
    // With a view loaded an empty name means "save over it" — the same fallback
    // the desktop disk button's empty input has.
    store.currentViewName = 'Мои'
    expect(w.vm.canSave).toBe(true)
    w.unmount()
  })

  it('ticks the loaded view, like the active layout', () => {
    const { w } = menuWithViews([{ id: 'v1', name: 'Мои' }], 'Мои')
    expect(w.vm.renderLabel({ key: 'view:v1', label: 'Мои' })).not.toBe('Мои')
    expect(w.vm.renderLabel({ key: 'view:v1', label: 'Мои' })).toBeTypeOf('object')
    w.unmount()
  })

  it('leaves an unloaded view untouched', () => {
    const { w } = menuWithViews(
      [
        { id: 'v1', name: 'Мои' },
        { id: 'v2', name: 'Спринт' },
      ],
      'Мои',
    )
    expect(w.vm.renderLabel({ key: 'view:v2', label: 'Спринт' })).toBe('Спринт')
    w.unmount()
  })

  it('ticks nothing when no view is loaded — an empty name must not match', () => {
    const { w } = menuWithViews([{ id: 'v1', name: '' }], '')
    expect(w.vm.renderLabel({ key: 'view:v1', label: '' })).toBe('')
    w.unmount()
  })
})

describe('boardView store', () => {
  it('reset clears the published views and any pending request', () => {
    const store = useBoardViewStore()
    store.savedViews = [{ id: 'v1', name: 'Мои' }]
    store.currentViewName = 'Мои'
    store.requestApplyView('v1')
    store.reset()
    expect(store.savedViews).toEqual([])
    expect(store.currentViewName).toBe('')
    expect(store.viewRequest).toBe(null)
  })

  it('an empty save name is carried as-is (the board falls back to the loaded view)', () => {
    const store = useBoardViewStore()
    store.requestSaveView('')
    expect(store.viewRequest).toMatchObject({ op: 'save', name: '' })
  })
})

// KanbanBoard is too heavy to mount (it loads a board over the API on create), so
// the other half of the wiring is read off the source.
const kanban = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

describe('KanbanBoard answers the ask', () => {
  it('publishes only the current layout views plus the loaded one', () => {
    expect(kanban).toContain('boardViewStore.savedViews = viewsForLayout.value.map')
    expect(kanban).toContain('boardViewStore.currentViewName = currentViewName.value')
  })

  it('applies and saves through its own functions, which own the config', () => {
    expect(kanban).toContain('() => boardViewStore.viewRequest')
    expect(kanban).toMatch(/savedViews\.value\.find\(\(x\) => x\.id === req\.id\)/)
    expect(kanban).toContain('if (v) applyView(v)')
    expect(kanban).toContain('newViewName.value = req.name')
  })

  it('keeps both desktop view buttons — the phone menu is the other way in', () => {
    // See cx-board-customize-reach: v2 (#2954) no longer slides the tools away,
    // and on a phone the header menu remains the entry (the cases above).
    expect(kanban).toContain('showLoadView')
    expect(kanban).toContain('showSaveView')
  })
})
