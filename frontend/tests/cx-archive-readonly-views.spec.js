// The archive view (?archived=1) is «look, don't touch», and the kanban layout
// says so everywhere: the column draggable, the card draggable, «+ задача»,
// «+ колонка», TaskCard, TaskModal and (since #2932) the Eisenhower matrix all
// take `archivedMode`. The other four layouts took nothing at all.
//
// `layout` lives in the boardView store and `?archived=1` in the route, so the two
// combine freely: switch to Список / Календарь / Таймлайн / Гантт while the archive
// chip is up and every archived card carries the full live right-click menu —
// «Удалить» (permanent, and it purges the task's uploads), «Архивировать» (on a
// card already archived), «Перенести в колонку», «Завершить», priority. The two
// chart views additionally let a bar be dragged to new dates, and the Gantt let a
// dependency edge be drawn or cut.
//
// Pinned here: the shared menu opens under `readonly` with nothing but the
// archive's own two items (and not at all for a caller that has not opted into
// «Вернуть из архива» — see cx-archive-restore-menu.spec.js), the shared bar drag
// never starts, and the board hands the flag to all four views.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ref } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

vi.mock('@/api', () => ({
  tasks: {
    update: vi.fn(),
    move: vi.fn(),
    remove: vi.fn(),
    archive: vi.fn(),
    addRelation: vi.fn(),
    removeRelation: vi.fn(),
  },
  boards: { dependencies: vi.fn(() => Promise.resolve({ data: [] })) },
}))

const { useTaskMenu } = await import('@/composables/useTaskMenu')
const { useChartBars } = await import('@/composables/useChartBars')
const { default: BoardListView } = await import('@/components/BoardListView.vue')

// open() defers the show flag by a frame (so a second right-click repositions an
// open menu), so every assertion about it has to let that frame pass.
const frame = () => new Promise((r) => requestAnimationFrame(() => r()))

const TASK = { id: 't1', number: 7, title: 'Архивная карточка', priority: 1 }
const EVT = { clientX: 10, clientY: 20 }

describe('useTaskMenu under the archive flag', () => {
  it('does not open the menu when readonly', async () => {
    const menu = useTaskMenu({ readonly: () => true })
    menu.open(EVT, TASK)
    await frame()

    expect(menu.show.value).toBe(false)
  })

  it('still opens it on a live board', async () => {
    const menu = useTaskMenu({ readonly: () => false })
    menu.open(EVT, TASK)
    await frame()

    expect(menu.show.value).toBe(true)
  })

  // The flag is read per gesture rather than captured at setup: a view that
  // switches scope without remounting (the archive chip is a route query) has to
  // start refusing immediately.
  it('follows a ref that flips after setup', async () => {
    const readonly = ref(false)
    const menu = useTaskMenu({ readonly })

    readonly.value = true
    menu.open(EVT, TASK)
    await frame()
    expect(menu.show.value).toBe(false)

    readonly.value = false
    menu.open(EVT, TASK)
    await frame()
    expect(menu.show.value).toBe(true)
  })

  // No flag at all is the live board — every caller that has not opted in yet
  // (TaskCard gates its own contextmenu handler) must keep its menu.
  it('defaults to a working menu', async () => {
    const menu = useTaskMenu({})
    menu.open(EVT, TASK)
    await frame()

    expect(menu.show.value).toBe(true)
  })
})

describe('useChartBars under the archive flag', () => {
  // useFormat() reads the theme store for the user's timezone.
  beforeEach(() => setActivePinia(createPinia()))

  const bars = (readonly) =>
    useChartBars({
      tier: ref('day'),
      range: ref({ start: new Date('2026-09-01'), end: new Date('2026-09-30') }),
      dayW: ref(24),
      estCfg: ref({}),
      vwindow: ref({ top: 0, bottom: 0, from: 0, to: 0 }),
      subBars: () => [],
      rowHeight: () => 32,
      findTask: () => null,
      onChanged: () => {},
      readonly,
    })

  const BAR = { id: 't1', start_date: '2026-09-10', due_date: '2026-09-12' }
  const down = () => ({
    button: 0,
    clientX: 100,
    preventDefault: () => {},
    stopPropagation: () => {},
  })

  it('starts no drag when readonly', () => {
    const { drag, onBarDown } = bars(() => true)
    onBarDown(down(), BAR, 'move')

    expect(drag.value).toBe(null)
  })

  it('still drags on a live board', () => {
    const { drag, onBarDown } = bars(() => false)
    onBarDown(down(), BAR, 'move')

    expect(drag.value?.id).toBe('t1')
  })
})

describe('BoardListView in the archive view', () => {
  beforeEach(() => setActivePinia(createPinia()))

  const mountList = (readonly) =>
    mount(BoardListView, {
      props: {
        columns: [{ key: 'c1', name: 'К работе', color: '' }],
        statusColumns: [{ id: 'c2', name: 'В процессе' }],
        lists: { c1: [TASK] },
        readonly,
      },
      global: { stubs: { UserAvatar: true, TagPill: true } },
    })

  // The row's menu is the only affordance this layout has, so under the archive it
  // opens with the archive's own items alone — never with an edit on it.
  it('offers nothing but open + restore on an archived row', async () => {
    const w = mountList(true)
    await w.find('.lv-row').trigger('contextmenu')
    await frame()

    expect(w.vm.menu.show.value).toBe(true)
    expect(w.vm.menu.options.value.map((o) => o.key)).toEqual(['open', 'restore'])
    w.unmount()
  })

  it('keeps the menu on a live board', async () => {
    const w = mountList(false)
    await w.find('.lv-row').trigger('contextmenu')
    await frame()

    expect(w.vm.menu.show.value).toBe(true)
    w.unmount()
  })
})

// KanbanBoard is too heavy to mount (it loads a board over the API on create), so
// the wiring is read off its template — the same way cx-archive-no-create.spec.js
// pins the matrix. cwd is frontend/ (vitest.config.js).
const src = (p) => readFileSync(resolve(process.cwd(), p), 'utf8')
const kanban = src('src/components/KanbanBoard.vue')

describe('KanbanBoard hands every layout its archive flag', () => {
  for (const tag of [
    'BoardListView',
    'BoardCalendarView',
    'BoardTimelineView',
    'BoardGanttView',
    'BoardMatrixView',
  ]) {
    it(`gives ${tag} :readonly="archivedMode"`, () => {
      const at = kanban.indexOf(`<${tag}`)
      expect(at, `${tag} is gone from the board`).toBeGreaterThan(-1)
      const view = kanban.slice(at)
      expect(view.slice(0, view.indexOf('/>'))).toContain(':readonly="archivedMode"')
    })
  }
})

describe('the chart views pass the flag on', () => {
  for (const file of [
    'src/components/BoardListView.vue',
    'src/components/BoardCalendarView.vue',
    'src/components/BoardTimelineView.vue',
    'src/components/BoardGanttView.vue',
  ]) {
    it(`${file} feeds readonly into useTaskMenu`, () => {
      const code = src(file)
      expect(code).toContain('readonly: { type: Boolean, default: false }')
      const call = code.slice(code.indexOf('useTaskMenu({'))
      expect(call.slice(0, call.indexOf('})'))).toContain('readonly: () => props.readonly')
    })
  }

  for (const file of [
    'src/components/BoardTimelineView.vue',
    'src/components/BoardGanttView.vue',
  ]) {
    it(`${file} feeds readonly into useChartBars`, () => {
      const code = src(file)
      const call = code.slice(code.indexOf('useChartBars({'))
      expect(call.slice(0, call.indexOf('})'))).toContain('readonly: () => props.readonly')
    })
  }
})

// The Gantt's dependency layer is its own: the knob that draws an edge and the ×
// that cuts one are drawn by this component, not by the shared bar engine.
describe('BoardGanttView dependency editing in the archive', () => {
  const gantt = src('src/components/BoardGanttView.vue')

  it('hides the link knob and gates the handler', () => {
    expect(gantt).toContain(':linkable="!readonly"')
    const fn = gantt.slice(gantt.indexOf('function onLinkDown'))
    expect(fn.slice(0, fn.indexOf('\n}'))).toContain('if (props.readonly) return')
  })

  it('hides the edge × and gates the delete', () => {
    const del = gantt.slice(gantt.indexOf('class="g-del"') - 200, gantt.indexOf('class="g-del"'))
    expect(del).toContain('v-if="!readonly"')
    const fn = gantt.slice(gantt.indexOf('async function removeEdge'))
    expect(fn.slice(0, fn.indexOf('\n}'))).toContain('if (props.readonly) return')
  })
})
