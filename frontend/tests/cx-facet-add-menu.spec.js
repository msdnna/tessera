// The composer's «+» opens a two-level facet menu (grouping / sort / every filter).
// It used to be an n-dropdown with side submenus, plus a hand-made drill for the
// phone, where a side submenu opens past the edge of the screen. Since #3112 it is
// kit/ActionMenu: each section a submenu — on a phone the sheet's second screen
// with «‹ Назад» — the people with their faces, and what is in force ticked.
//
// The customize drawer renders the very same `addOptions` behind its own «+», and
// on a phone that drawer is the board's settings surface one can always reach.
// Pinned here: the conversion both call sites share, each with its own open state
// (one shared `show` would pop both menus at once, the hidden one anchored to a
// collapsed button), and that the drawer actually uses it.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { ref, nextTick } from 'vue'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { useFacetAddMenu, facetMenuItems, facetRows } from '@/composables/useFacetAddMenu'
import BoardCustomizePanel from '@/components/BoardCustomizePanel.vue'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

// Shaped like useBoardFacets' `addOptions`: sections with children, a Naive group
// among them, a testid on the grouping section the e2e suite reaches it by.
const OPTIONS = [
  {
    label: 'Группировка',
    key: 'group',
    props: { 'data-testid': 'facet-group' },
    children: [{ label: 'По статусу', key: 'g.status', props: { 'data-testid': 'facet-g-st' } }],
  },
  { label: 'Сортировка', key: 'sort', children: [{ label: 'По сроку', key: 's.due' }] },
  {
    label: 'Исполнитель',
    key: 'fa',
    children: [
      { label: 'Аня', key: 'fa.1', avatarUserId: '1' },
      {
        type: 'group',
        label: 'GitLab',
        key: 'fag',
        children: [{ label: 'bob', key: 'fa.gl:bob' }],
      },
    ],
  },
]

describe('facetMenuItems', () => {
  it('makes a submenu of every section, with its icon and testid', () => {
    const items = facetMenuItems(OPTIONS)
    expect(items.map((i) => i.key)).toEqual(['group', 'sort', 'fa'])
    expect(items.every((i) => Array.isArray(i.children) && i.icon)).toBe(true)
    expect(items[0].testid).toBe('facet-group')
    expect(items[0].children[0].testid).toBe('facet-g-st')
  })

  it('turns an inline group into a heading and keeps the faces', () => {
    const rows = facetRows(OPTIONS[2].children)
    expect(rows.map((r) => r.heading || r.key)).toEqual(['fa.1', 'GitLab', 'fa.gl:bob'])
    expect(rows[0].avatar).toEqual({ userId: '1', src: undefined, name: 'Аня' })
  })

  it('ticks what is already in force', () => {
    const items = facetMenuItems(OPTIONS, (k) => k === 's.due')
    expect(items[1].children[0].checked).toBe(true)
    expect(items[0].children[0].checked).toBe(false)
  })
})

describe('useFacetAddMenu', () => {
  it('hands a pick over and closes', () => {
    const picked = []
    const api = useFacetAddMenu({ options: ref(OPTIONS), onPick: (k) => picked.push(k) })
    api.show.value = true
    api.onSelect('s.due')
    expect(picked).toEqual(['s.due'])
    expect(api.show.value).toBe(false)
  })
})

describe('BoardCustomizePanel «+»', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.matchMedia = vi.fn().mockReturnValue({
      matches: true, // ≤768px — the viewport where the drawer is the only way in
      addEventListener: () => {},
      removeEventListener: () => {},
    })
  })

  const mountPanel = async () => {
    const w = mount(BoardCustomizePanel, {
      props: {
        show: true,
        defaultOpen: ['facets'],
        boardName: 'Доска релиза',
        boardIcon: '',
        boardColor: '#7c5cff',
        boardIconMode: 'badge',
        fieldVis: {},
        facetChips: [],
        addOptions: OPTIONS,
        facetIsOn: (k) => k === 'g.status',
      },
      // The drawer and the menu teleport to <body>, out of the wrapper's reach.
      global: { stubs: { teleport: true, TesseraIcon: true } },
    })
    await nextTick()
    await nextTick()
    return w
  }
  const menu = (w) =>
    w.findAllComponents({ name: 'ActionMenu' }).find((m) => m.props('testid') === 'facet')

  it('is the shared ActionMenu, with what is on ticked', async () => {
    const w = await mountPanel()
    const m = menu(w)
    expect(m.props('items').map((i) => i.key)).toEqual(['group', 'sort', 'fa'])
    expect(m.props('items')[0].children[0].checked).toBe(true)
    w.unmount()
  })

  it('emits a picked facet once', async () => {
    const w = await mountPanel()
    menu(w).vm.$emit('select', 's.due')
    expect(w.emitted('add-facet')).toEqual([['s.due']])
    w.unmount()
  })
})

// KanbanBoard is too heavy to mount (it loads a board over the API on create), so
// its half of the wiring is read off the source, like cx-archive-customize-board.
const kanban = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

describe('KanbanBoard keeps its own instance of the menu', () => {
  it('uses the shared composable rather than a second inline copy', () => {
    expect(kanban).toContain('useFacetAddMenu({')
    expect(kanban).not.toContain('<n-dropdown')
  })
})
