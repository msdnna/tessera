// Getting a task back out of the archive, from the layout the reader is actually on
// (#2932).
//
// «Вернуть из архива» was drawn by TaskCard alone — the ↺ button that replaces its
// hover action bar under `readonly`. So the board and the Eisenhower matrix (which
// renders the same card) could restore an archived task, and Список / Календарь /
// Таймлайн / Гантт — which have no card of their own and nothing but the shared
// right-click menu — could not: `useTaskMenu.open()` refused outright under the
// archive flag, because every item on the live menu writes to the task.
//
// The layout is per-board localStorage, so this is not an exotic state: a reader
// whose board sits on «Список» opens ?archived=1, sees the archived rows, and has no
// way at all to bring one back until they think to switch visualization — nothing on
// screen says the action lives in another layout.
//
// Restore is the archive's OWN action rather than an edit of a live card, so it is
// the one thing the flag may let through. Pinned here: with `onRestore` the menu
// opens under the flag carrying exactly «Открыть» + «Вернуть из архива»; without it
// the refusal stands (TaskCard gates its own contextmenu and must not grow a second
// affordance); and no mutating key can fire while the flag is up, even if it arrives
// from a menu built a moment earlier.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

vi.mock('@/api', () => ({
  tasks: {
    update: vi.fn(),
    move: vi.fn(),
    remove: vi.fn(),
    archive: vi.fn(),
  },
}))

const { tasks: tasksApi } = await import('@/api')
const { useTaskMenu } = await import('@/composables/useTaskMenu')

// open() defers the show flag by a frame (so a second right-click repositions an
// open menu), so every assertion about it has to let that frame pass.
const frame = () => new Promise((r) => requestAnimationFrame(() => r()))

const TASK = { id: 't1', number: 7, title: 'Архивная карточка', column_id: 'c1' }
const EVT = { clientX: 10, clientY: 20 }
const COLUMNS = [{ id: 'c2', name: 'В процессе' }]

describe('the archive menu', () => {
  beforeEach(() => vi.clearAllMocks())

  it('opens under the archive flag when the caller offers a restore', async () => {
    const menu = useTaskMenu({ readonly: () => true, onRestore: () => {} })
    menu.open(EVT, TASK)
    await frame()

    expect(menu.show.value).toBe(true)
  })

  it('carries the archive items only — no edit of the live task', async () => {
    const menu = useTaskMenu({
      readonly: () => true,
      onRestore: () => {},
      columns: () => COLUMNS,
      extra: () => [{ label: 'Подзадача', key: 'subtask' }],
    })
    menu.open(EVT, TASK)
    await frame()

    expect(menu.options.value.map((o) => o.key)).toEqual(['open', 'restore'])
  })

  it('hands the task id to the restore callback and closes', async () => {
    const onRestore = vi.fn()
    const menu = useTaskMenu({ readonly: () => true, onRestore })
    menu.open(EVT, TASK)
    await frame()
    await menu.select('restore')

    expect(onRestore).toHaveBeenCalledWith('t1')
    expect(menu.show.value).toBe(false)
  })

  // The live menu is unchanged: restore belongs to the archive and nowhere else.
  it('is not offered on a live board', async () => {
    const menu = useTaskMenu({ readonly: () => false, onRestore: () => {}, columns: () => COLUMNS })
    menu.open(EVT, TASK)
    await frame()
    const keys = menu.options.value.map((o) => o.key)

    expect(menu.show.value).toBe(true)
    expect(keys).not.toContain('restore')
    expect(keys).toContain('delete')
  })

  // TaskCard passes no `readonly` of its own but gates its contextmenu handler, and
  // any caller that has not opted in keeps the flat refusal.
  it('still refuses to open for a caller with no restore', async () => {
    const menu = useTaskMenu({ readonly: () => true })
    menu.open(EVT, TASK)
    await frame()

    expect(menu.show.value).toBe(false)
  })

  // The flag is a route query: it can turn on with a menu already built and open.
  it('lets no mutating key through once the flag is up', async () => {
    const readonly = ref(false)
    const menu = useTaskMenu({ readonly, onRestore: () => {} })
    menu.open(EVT, TASK)
    await frame()

    readonly.value = true
    await menu.select('toggle')
    await menu.select('prio:2')
    await menu.select('col:c2')
    await menu.select('delete')
    await menu.select('archive')

    expect(tasksApi.update).not.toHaveBeenCalled()
    expect(tasksApi.move).not.toHaveBeenCalled()
    expect(menu.deleteConfirmShow.value).toBe(false)
    expect(menu.archiveConfirmShow.value).toBe(false)
  })

  // One step further along: the popconfirms stay mounted, so a positive-click can
  // land after the scope changed under them.
  it('refuses a confirm that lands after the flag is up', async () => {
    const readonly = ref(false)
    const menu = useTaskMenu({ readonly, onRestore: () => {} })
    menu.open(EVT, TASK)
    await frame()
    await menu.select('delete')
    expect(menu.deleteConfirmShow.value).toBe(true)

    readonly.value = true
    await menu.confirmDelete()
    await menu.confirmArchive()

    expect(tasksApi.remove).not.toHaveBeenCalled()
    expect(tasksApi.archive).not.toHaveBeenCalled()
  })
})

// The views are too entangled with their charts to mount here (the board itself
// loads over the API on create), so the wiring is read off the sources — the same
// way cx-archive-readonly-views.spec.js pins the readonly flag. cwd is frontend/.
const src = (p) => readFileSync(resolve(process.cwd(), p), 'utf8')

describe('every cardless layout wires the restore through', () => {
  for (const file of [
    'src/components/BoardListView.vue',
    'src/components/BoardCalendarView.vue',
    'src/components/BoardTimelineView.vue',
    'src/components/BoardGanttView.vue',
  ]) {
    it(`${file} emits restore from its menu`, () => {
      const code = src(file)
      expect(code).toContain("defineEmits(['open', 'changed', 'restore'])")
      const call = code.slice(code.indexOf('useTaskMenu({'))
      expect(call.slice(0, call.indexOf('})'))).toContain("onRestore: (id) => emit('restore', id)")
    })
  }

  const kanban = src('src/components/KanbanBoard.vue')

  for (const tag of [
    'BoardListView',
    'BoardCalendarView',
    'BoardTimelineView',
    'BoardGanttView',
    'BoardMatrixView',
  ]) {
    it(`the board answers ${tag}'s restore`, () => {
      const at = kanban.indexOf(`<${tag}`)
      expect(at, `${tag} is gone from the board`).toBeGreaterThan(-1)
      const view = kanban.slice(at)
      expect(view.slice(0, view.indexOf('/>'))).toContain('@restore="restoreFromArchive"')
    })
  }
})
