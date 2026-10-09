// The archive view (?archived=1) is the same board behind a read-only lens, and
// #2932 walked its children one by one: the column and card draggables, «+ задача»,
// «+ колонка», TaskCard, TaskModal, the matrix and (last) the list/calendar/timeline/
// gantt context menus all take `archivedMode`. BoardCustomizePanel was missed — it is
// rendered outside the layout branch, at the very bottom of KanbanBoard's template,
// and was the one child of the board handed no archive flag at all.
//
// It is not a view-only drawer. Its first section is the LIVE board's branding: the
// name input commits on blur/Enter, the icon picker uploads SVG/PNG, and every one of
// them emits `update-board` → boards.update() → wsStore.upsertBoard(), i.e. a write
// for the whole workspace plus a re-stamped sidebar node. The gear that opens the
// drawer is gated only by `!timelineLike`, so it is right there in the archive — and
// since the previous pass the phone reaches the same drawer from BoardMobileMenu.
//
// Pinned here: `readonly` takes the branding section off (the view settings stay —
// card size, pills, auto-collapse, facets and autosave are this reader's own state),
// and KanbanBoard both passes the flag and refuses a stray updateBoard call.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import BoardCustomizePanel from '@/components/BoardCustomizePanel.vue'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

// n-drawer teleports its content to <body>, which puts it out of the wrapper's
// reach (`w.html()` is then a bare teleport marker, and every assertion below reads
// as "absent" — including the ones that must fail on the old code). VTU's teleport
// stub renders it in place instead.
const STUBS = { teleport: true, TesseraIcon: true }

// The panel asks useResponsive whether it is on a phone (its «+» drills through the
// facet menu there instead of fanning submenus off-screen), and jsdom ships no
// matchMedia — without this the mount throws before a single assertion runs.
beforeEach(() => {
  // The panel prints its due-date preview through useFormat (the theme store).
  setActivePinia(createPinia())
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
})

// Every section open at once (⇧-click does that in the app), so a gate is
// checked against the whole panel rather than whichever section happens to be open.
const ALL = ['views', 'cards', 'scale', 'facets', 'columns', 'board']
const mountPanel = (readonly) =>
  mount(BoardCustomizePanel, {
    props: {
      show: true,
      readonly,
      layout: 'board',
      defaultOpen: ALL,
      boardName: 'Доска релиза',
      boardIcon: '',
      boardColor: '#7c5cff',
      boardIconMode: 'badge',
      fieldVis: {},
      facetChips: [],
      addOptions: [],
    },
    global: { stubs: STUBS },
  })

describe('BoardCustomizePanel in the archive view', () => {
  // `.n-input`, not a bare `input`: naive's radio buttons carry hidden `<input>`
  // elements of their own, so the loose selector reads as present either way.
  it('offers the board name + icon picker on a live board', () => {
    const w = mountPanel(false)
    expect(w.find('.n-input').exists()).toBe(true)
    expect(w.find('.board-icp').exists()).toBe(true)
    // cards · facets · columns · board (views is the board's slot, not given here)
    expect(w.findAll('.cp-head').length).toBe(4)
    w.unmount()
  })

  it('drops the whole branding section in the archive', () => {
    const w = mountPanel(true)
    expect(w.find('.board-icp').exists()).toBe(false)
    expect(w.find('.n-input').exists()).toBe(false)
    w.unmount()
  })

  // The point of hiding only that one section: the drawer is still worth opening
  // from the archive. Card size / pills / auto-collapse / facets / autosave are all
  // local view state and must survive the gate.
  it('keeps the view settings, which are the reader’s own', () => {
    const w = mountPanel(true)
    // three sections left: cards, facets, columns
    expect(w.findAll('.cp-head').length).toBe(3)
    expect(w.findAll('.cp-size').length).toBe(3) // compact / medium / large
    expect(w.findAll('.cp-field').length).toBe(9) // the per-field pill switches
    expect(w.find('.cp-chip-add').exists()).toBe(true)
    w.unmount()
  })

  // Nothing may emit a board write from the archive even if a section were to leak
  // back in — the emit is what reaches boards.update().
  it('emits no update-board at all while readonly', async () => {
    const w = mountPanel(true)
    expect(w.emitted('update-board')).toBeFalsy()
    w.unmount()
  })
})

// KanbanBoard is too heavy to mount (it loads a board over the API on create), so
// the wiring is read off its source — the same way cx-archive-no-create.spec.js pins
// the «+ колонка» tile. cwd is frontend/ (vitest.config.js).
const kanban = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

describe('KanbanBoard wiring for the customize drawer', () => {
  it('hands the panel its archive flag', () => {
    const panel = kanban.slice(kanban.indexOf('<BoardCustomizePanel'))
    expect(panel.slice(0, panel.indexOf('</BoardCustomizePanel>'))).toContain(
      ':readonly="archivedMode"',
    )
  })

  // The name input commits on blur, so the call itself has to refuse too.
  it('refuses a board write from the archive', () => {
    const fn = kanban.slice(kanban.indexOf('async function updateBoard('))
    expect(fn.slice(0, fn.indexOf('\n}'))).toContain('archivedMode.value')
  })
})
