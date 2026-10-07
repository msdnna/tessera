// The composer's «+» opens a two-level facet menu (grouping / sort / every filter).
// On a desktop that is a plain n-dropdown with side submenus; on a phone a side
// submenu opens past the edge of the screen, so there the menu drills into one
// sub-list at a time behind a «‹ Назад» row. That drill lived inline in KanbanBoard
// and was wired to the composer's button alone.
//
// The customize drawer renders the very same `addOptions` behind its own «+», bound
// raw — nested `children`, no drill, no custom rows. And on a phone that drawer is
// the board's settings surface one can always reach: the composer's gear slides away
// as soon as the chips fill the bar (`.bar-tools.hidden`), and BoardMobileMenu's
// «Настроить вид» lands here. So the one add menu a phone could always open was the
// one that fanned submenus off the screen.
//
// Pinned here: the behaviour is a composable both call sites share, each with its
// own open/level state (one shared `show` would pop both dropdowns at once, the
// hidden one anchored to a collapsed button), and the drawer actually uses it.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { ref, computed, defineComponent, nextTick } from 'vue'
import { NDropdown } from 'naive-ui'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { useFacetAddMenu } from '@/composables/useFacetAddMenu'
import BoardCustomizePanel from '@/components/BoardCustomizePanel.vue'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

// Shaped like KanbanBoard's `addOptions`: groups with children, one leaf among them.
const OPTIONS = [
  { label: 'Группировка', key: 'group', children: [{ label: 'По статусу', key: 'group:status' }] },
  {
    label: 'Сортировка',
    key: 'sort',
    children: [{ label: 'По сроку', key: 'sort:due' }],
  },
  { label: 'Исполнитель', key: 'fa', children: [{ label: 'Аня', key: 'fa:1', avatarUserId: '1' }] },
]

// useI18n() needs an active instance with the plugin (installed globally in
// tests/setup.js), so the composable is exercised through a host component.
function host(isMobile, onPick = () => {}) {
  const api = {}
  const H = defineComponent({
    setup() {
      Object.assign(
        api,
        useFacetAddMenu({ options: computed(() => OPTIONS), isMobile: ref(isMobile), onPick }),
      )
      return () => null
    },
  })
  const w = mount(H)
  return { api, w }
}

describe('useFacetAddMenu on a desktop', () => {
  it('passes the nested menu through untouched — side submenus work there', () => {
    const { api, w } = host(false)
    expect(api.menuOptions.value).toBe(OPTIONS)
    expect(api.renderLabel({ label: 'Сортировка', key: 'sort' })).toBe('Сортировка')
    w.unmount()
  })
})

describe('useFacetAddMenu on a phone', () => {
  it('flattens the top level into drill entries instead of submenus', () => {
    const { api, w } = host(true)
    expect(api.menuOptions.value.map((o) => o.key)).toEqual(['nav.group', 'nav.sort', 'nav.fa'])
    expect(api.menuOptions.value.every((o) => !o.children)).toBe(true)
    w.unmount()
  })

  it('drills into a group and offers a way back', async () => {
    const { api, w } = host(true)
    api.onSelect('nav.sort')
    expect(api.level.value).toBe('sort')
    expect(api.menuOptions.value.map((o) => o.key)).toEqual(['nav.back', 'nav.div', 'sort:due'])
    api.onSelect('nav.back')
    expect(api.level.value).toBe(null)
    w.unmount()
  })

  // n-dropdown closes itself on every select; mid-drill that would snap the menu
  // shut instead of showing the sub-list.
  it('stays open while drilling and closes on a real pick', async () => {
    const picked = []
    const { api, w } = host(true, (k) => picked.push(k))
    api.show.value = true
    api.onSelect('nav.sort')
    api.onShow(false) // the select-triggered close
    expect(api.show.value).toBe(true)
    expect(picked).toEqual([])

    api.onSelect('sort:due')
    expect(picked).toEqual(['sort:due'])
    expect(api.show.value).toBe(false)
    expect(api.level.value).toBe(null)
    w.unmount()
  })

  // The drill's own nav.* keys are swallowed by the composable — a facet handler
  // that saw them would try to add a facet named `nav.group`.
  it('never leaks a nav key to the picker', () => {
    const picked = []
    const { api, w } = host(true, (k) => picked.push(k))
    for (const k of ['nav.group', 'nav.back', 'nav.div']) api.onSelect(k)
    expect(picked).toEqual([])
    w.unmount()
  })

  it('resets the drill level once the menu really closes', async () => {
    const { api, w } = host(true)
    api.onSelect('nav.fa')
    expect(api.level.value).toBe('fa')
    await nextTick() // past the swallowed select-close; this one is a real close
    api.onShow(false)
    expect(api.level.value).toBe(null)
    w.unmount()
  })
})

describe('BoardCustomizePanel «+» on a phone', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.matchMedia = vi.fn().mockReturnValue({
      matches: true, // ≤768px — the viewport where the drawer is the only way in
      addEventListener: () => {},
      removeEventListener: () => {},
    })
  })

  // useResponsive reads matchMedia in `onMounted`, so the phone flag lands one tick
  // after mount — read the dropdown's props before that and they are still desktop.
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
      },
      // n-drawer teleports to <body>, out of the wrapper's reach.
      global: { stubs: { teleport: true, TesseraIcon: true } },
    })
    await w.vm.$nextTick()
    await w.vm.$nextTick()
    return w
  }

  // IconColorPicker carries an n-dropdown of its own (the SVG/PNG upload menu), so
  // the facet menu is picked by its trigger.
  const addDropdown = (w) =>
    w.findAllComponents(NDropdown).find((d) => d.element.classList?.contains('cp-chips'))

  it('drills like the composer instead of fanning submenus off-screen', async () => {
    const w = await mountPanel()
    const dd = addDropdown(w)
    expect(dd.props('options').map((o) => o.key)).toEqual(['nav.group', 'nav.sort', 'nav.fa'])
    expect(dd.props('renderLabel')).toBeTypeOf('function')
    w.unmount()
  })

  it('picks a real facet through the drill and emits it once', async () => {
    const w = await mountPanel()
    const dd = addDropdown(w)
    dd.vm.$emit('select', 'nav.sort')
    await w.vm.$nextTick()
    expect(dd.props('options').map((o) => o.key)).toEqual(['nav.back', 'nav.div', 'sort:due'])
    expect(w.emitted('add-facet')).toBeFalsy() // the nav key is not a facet

    dd.vm.$emit('select', 'sort:due')
    expect(w.emitted('add-facet')).toEqual([['sort:due']])
    w.unmount()
  })
})

// KanbanBoard is too heavy to mount (it loads a board over the API on create), so
// its half of the wiring is read off the source, like cx-archive-customize-board.
const kanban = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

describe('KanbanBoard keeps its own instance of the menu', () => {
  it('uses the shared composable rather than a second inline copy', () => {
    expect(kanban).toContain('useFacetAddMenu({')
    expect(kanban).not.toContain('function renderAddLabel(')
  })
})
