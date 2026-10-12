// The customize drawer (card size, pill visibility, board name/icon, auto-collapse
// of empty columns) had exactly one trigger: the gear in the composer's right-hand
// tools. Those tools are not merely narrow on a phone — `.bar-tools.hidden` sets
// `max-width: 0` AND `pointer-events: none`, and it flips on as soon as the chips
// plus the reserved search slice reach 75% of the bar, i.e. two or three facets on
// a 360px screen. BoardMobileMenu is the phone's whole board toolbar (Topbar renders
// it *instead of* BoardActions) and carried no customize entry, so from a phone the
// drawer was simply unreachable.
//
// Pinned here: the menu offers the entry, it is gated the same way the gear is
// (no customize panel on the time-axis layouts), it asks through the store, and
// KanbanBoard listens for that ask.
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
const entry = (w) => w.vm.items.find((o) => o.key === 'customize')

beforeEach(() => {
  setActivePinia(createPinia())
  route.query = {}
  push.mockClear()
  replace.mockClear()
})

describe('BoardMobileMenu «Настроить вид» entry', () => {
  it('is offered on the phone — the gear in the composer is not reachable there', () => {
    const w = mount(BoardMobileMenu, { global: { stubs: STUBS } })
    expect(entry(w)).toBeTruthy()
    w.unmount()
  })

  it('stays on every layout — the view panel works in all of them (#2986)', () => {
    for (const layout of ['board', 'list', 'timeline', 'gantt', 'calendar', 'matrix']) {
      const w = mount(BoardMobileMenu, { global: { stubs: STUBS } })
      useBoardViewStore().layout = layout
      expect(entry(w)).toBeTruthy()
      w.unmount()
    }
  })

  it('asks the board for the drawer through the store', () => {
    const w = mount(BoardMobileMenu, { global: { stubs: STUBS } })
    const store = useBoardViewStore()
    const before = store.customizeNonce
    w.vm.onSelect('customize')
    expect(store.customizeNonce).toBe(before + 1)
    w.unmount()
  })

  it('does not navigate — the drawer is board state, not a route', () => {
    const w = mount(BoardMobileMenu, { global: { stubs: STUBS } })
    w.vm.onSelect('customize')
    expect(push).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
    w.unmount()
  })
})

describe('boardView store', () => {
  it('openCustomize bumps its own nonce and leaves the reload nonce alone', () => {
    const store = useBoardViewStore()
    const reload = store.reloadNonce
    store.openCustomize()
    store.openCustomize()
    expect(store.customizeNonce).toBe(2)
    expect(store.reloadNonce).toBe(reload)
  })
})

// KanbanBoard is too heavy to mount (it loads a board over the API on create), so
// the other half of the wiring is read off the source.
const kanban = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

describe('KanbanBoard answers the ask', () => {
  it('watches the store nonce and opens the panel, whatever the view', () => {
    expect(kanban).toContain('() => boardViewStore.customizeNonce')
    expect(kanban).toContain('() => openPanel(),')
  })

  it('keeps the desktop gear — the phone menu is the other way in, not a replacement', () => {
    // v2 (#2954) folds the sub-toolbar adaptively instead of sliding its tools
    // away, but on a phone the toolbar is not where the customize panel is
    // reached: that is still the header menu's job (the cases above).
    expect(kanban).toContain('data-tour="board-customize"')
  })
})
