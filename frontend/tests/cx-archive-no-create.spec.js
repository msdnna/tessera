// The archive view (?archived=1) is read-only, and the kanban says so in five
// places: the column draggable, the card draggable, «+ задача», TaskCard and
// TaskModal all take `archivedMode`. Two creating affordances were left open.
//
//  * The «+ колонка» tile asked only `groupMode === 'status'`. Pressing it from
//    the archive creates a real, empty column on the LIVE board — for everyone.
//    (Android has always gated it: BoardViews.kt `!state.archivedMode`.)
//  * The Eisenhower matrix is the same board in another layout, and it was handed
//    no archive flag at all. Its quadrant «＋» creates a live task in the board's
//    first column — which vanishes on the next load, because the archive lists
//    archived cards only — the quadrant drag pins an archived card, and TaskCard
//    offered its live menu («Архивировать») over an already-archived task.
//
// Pinned here: `readonly` takes every creating/mutating affordance out of the
// matrix, and the column tile is gone from the archive.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import draggable from 'vuedraggable'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const eisenhower = vi.fn()
vi.mock('@/api', () => ({
  tasks: { eisenhower: (...a) => eisenhower(...a) },
}))

const { default: BoardMatrixView } = await import('@/components/BoardMatrixView.vue')
const { default: TaskCard } = await import('@/components/TaskCard.vue')

// One card, pinned to a quadrant (eisenhower_quadrant != null) so the «открепить»
// button has a reason to render, and open (no completed_at) so the grid keeps it.
const TASK = { id: 't1', title: 'Архивная карточка', eisenhower_quadrant: 0, priority: 1 }

const mountMatrix = (readonly) =>
  mount(BoardMatrixView, {
    props: { tasks: [TASK], readonly },
    global: { stubs: { TaskCard: true, TesseraIcon: true } },
  })

describe('BoardMatrixView in the archive view', () => {
  // useFormat() reads the theme store for the user's timezone (the urgency axis
  // is counted in calendar days there), so the grid needs a live pinia to mount.
  beforeEach(() => setActivePinia(createPinia()))

  it('offers no quadrant «＋», so no live task is created from the archive', () => {
    const live = mountMatrix(false)
    expect(live.findAll('.m-add').length).toBe(4) // one per quadrant
    live.unmount()

    const archived = mountMatrix(true)
    expect(archived.find('.m-add').exists()).toBe(false)
    archived.unmount()
  })

  // The input is opened by startAdd() alone; a stray call must not reach it
  // either, or a keyboard path would slip past the hidden button.
  it('does not open the quick-add input when asked directly', async () => {
    const w = mountMatrix(true)
    w.vm.startAdd(0)
    await w.vm.$nextTick()

    expect(w.find('.m-add-input').exists()).toBe(false)
    w.unmount()
  })

  it('disables the quadrant drag that would re-pin an archived card', () => {
    // `disabled` is not a declared prop of vuedraggable and it never reaches the
    // DOM either: the component picks the Sortable options out of $attrs and
    // hands them to Sortable. So $attrs is where the flag is observable.
    expect(mountMatrix(false).findComponent(draggable).vm.$attrs.disabled).toBe(false)
    expect(mountMatrix(true).findComponent(draggable).vm.$attrs.disabled).toBe(true)
  })

  it('hides the «открепить» button and passes readonly down to the card', () => {
    const live = mountMatrix(false)
    expect(live.find('.m-pin').exists()).toBe(true)
    expect(live.findComponent(TaskCard).props('readonly')).toBe(false)
    live.unmount()

    const archived = mountMatrix(true)
    expect(archived.find('.m-pin').exists()).toBe(false)
    expect(archived.findComponent(TaskCard).props('readonly')).toBe(true)
    archived.unmount()
  })
})

// KanbanBoard is too heavy to mount here (it loads a board over the API on
// create), so the two wirings are read off its template — the same way
// dt-get-started.spec.js pins the tour's anchors. cwd is frontend/ (vitest.config.js).
const kanban = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

describe('KanbanBoard wiring for the archive view', () => {
  it('drops the «+ колонка» tile in the archive', () => {
    const tile = kanban.match(/v-if="groupMode === 'status'[^"]*"\s*\n?\s*class="add-col"/)
    expect(tile, 'the add-col tile still has its own v-if').toBeTruthy()
    expect(tile[0]).toContain('!archivedMode')
  })

  it('hands the matrix its archive flag', () => {
    const view = kanban.slice(kanban.indexOf('<BoardMatrixView'))
    expect(view.slice(0, view.indexOf('/>'))).toContain(':readonly="archivedMode"')
  })
})
