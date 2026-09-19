// A write the server refused must say so — from the menu and the drag too (#2932).
//
// The card's own edits were taught to report a refusal with a toast
// (cx-card-edit-failure), and that spec cites `composables/useTaskMenu.js` as the
// rule already written down: «the right-click menu over the very same card wraps
// each of these calls and resyncs on failure». Half of it was true. The menu did
// wrap every call — and then swallowed the error:
//
//     } catch {
//       onChanged?.()
//     }
//
// `onChanged()` re-reads the board, so the card snaps back to exactly where it
// was. For the reader that is indistinguishable from a click that missed, and it
// is the same story for «Удалить» — a confirmed permanent delete leaving the row
// on screen with nothing said invites a second and third attempt at something
// already refused. Two more surfaces carried the same shape: the bar drag shared
// by Таймлайн and Гантт (`useChartBars`), and the on-card subtask list, where a
// dropped card or a reorder the server rejects just slides back.
//
// The toast is asserted through the injection naive's own `useMessage()` reads,
// so these also pin that the composables really are wired to the provider App.vue
// puts above them — a bus handed in by mocking the module would pass even if the
// wiring were gone.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { ref, computed, defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'
import { seedBoardStore } from './helpers/boardStore'

const update = vi.fn(() => Promise.resolve({ data: {} }))
const move = vi.fn(() => Promise.resolve({ data: {} }))
const remove = vi.fn(() => Promise.resolve({ data: {} }))
const archive = vi.fn(() => Promise.resolve({ data: {} }))
const setParent = vi.fn(() => Promise.resolve({ data: {} }))
vi.mock('@/api', () => ({
  tasks: {
    update: (...a) => update(...a),
    move: (...a) => move(...a),
    remove: (...a) => remove(...a),
    archive: (...a) => archive(...a),
    setParent: (...a) => setParent(...a),
  },
  boards: { dependencies: vi.fn() },
}))

import { useTaskMenu } from '@/composables/useTaskMenu'
import { useChartBars } from '@/composables/useChartBars'

// The message bus every mount below is given. `tests/setup.js` provides a silent
// one for specs that only need `useMessage()` not to throw; this spy wins over it.
const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }
const provide = { 'n-message-api': toast }

// The refusal `api/index.js` builds: a human sentence in `message`, the status
// alongside it (see `humanizeError`). What the reader must end up seeing.
const REFUSED = Object.assign(new Error('Недостаточно прав'), { status: 403 })
const refuse = (fn) => fn.mockImplementationOnce(() => Promise.reject(REFUSED))

// Both composables need the same thing from a mount: a component instance with
// the message provider above it, which is how all of their real callers build
// them. Nothing is rendered — the composable's return value is the surface.
const mountHost = (build) =>
  mount(
    defineComponent({
      setup() {
        build()
        return () => h('div')
      },
    }),
    { global: { provide } },
  )

const COLS = [
  { id: 'c1', name: 'К работе', color: '#9aa0aa', position: 1 },
  { id: 'c2', name: 'В процессе', color: '#2f80ed', position: 2 },
]
const TASK = { id: 't1', title: 'Задача', column_id: 'c1', completed_at: null, priority: 0 }

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
})

// ── the right-click menu ──────────────────────────────────────────────────────
describe('useTaskMenu refusals', () => {
  // Built inside a component, the way all six of its callers build it — that is
  // what puts the provider in reach.
  function host(opts = {}) {
    let api
    const onChanged = vi.fn()
    mountHost(() => (api = useTaskMenu({ onChanged, columns: COLS, ...opts })))
    api.open({ clientX: 0, clientY: 0 }, TASK)
    return { api, onChanged }
  }

  it('reports a refused «Выполнено» instead of resyncing in silence', async () => {
    refuse(update)
    const { api, onChanged } = host()
    await api.select('toggle')
    expect(toast.error).toHaveBeenCalledWith('Недостаточно прав')
    // The resync still happens: the card must not keep a tick the server rejected.
    expect(onChanged).toHaveBeenCalled()
  })

  it('reports a refused priority change', async () => {
    refuse(update)
    const { api } = host()
    await api.select('prio:3')
    expect(toast.error).toHaveBeenCalledWith('Недостаточно прав')
  })

  it('reports a refused move to another column', async () => {
    refuse(move)
    const { api } = host()
    await api.select('col:c2')
    expect(toast.error).toHaveBeenCalledWith('Недостаточно прав')
  })

  it('reports a refused delete — the loudest of the three', async () => {
    refuse(remove)
    const { api, onChanged } = host()
    await api.select('delete')
    expect(api.deleteConfirmShow.value).toBe(true)
    await api.confirmDelete()
    expect(toast.error).toHaveBeenCalledWith('Недостаточно прав')
    expect(onChanged).toHaveBeenCalled()
  })

  it('reports a refused archive', async () => {
    refuse(archive)
    const { api } = host()
    await api.select('archive')
    await api.confirmArchive()
    expect(toast.error).toHaveBeenCalledWith('Недостаточно прав')
  })

  // Control: a write that went through has nothing to report. Without this the
  // four above would pass just as well against a menu that shouts on every click.
  it('stays quiet when the write goes through', async () => {
    const { api, onChanged } = host()
    await api.select('toggle')
    expect(update).toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
    expect(onChanged).toHaveBeenCalled()
  })

  // Control: the composable is also called straight out of a spec, with no
  // instance to inject from (cx-composables does exactly that). A missing bus
  // must cost it nothing — least of all the action itself.
  it('still acts when built outside a component', async () => {
    refuse(remove)
    const onChanged = vi.fn()
    const api = useTaskMenu({ onChanged, columns: COLS })
    api.open({ clientX: 0, clientY: 0 }, TASK)
    await api.confirmDelete()
    expect(remove).toHaveBeenCalledWith('t1')
    expect(onChanged).toHaveBeenCalled()
  })
})

// ── the shared bar drag (Таймлайн / Гантт) ────────────────────────────────────
describe('useChartBars refusals', () => {
  const day = (n) => new Date(2026, 0, n).toISOString()

  function host(tasks, onChanged) {
    let api
    const vwindow = ref({
      rows: tasks.map((t) => ({ t: 'task', key: t.id, task: t })),
      top: 0,
      bottom: 0,
    })
    mountHost(
      () =>
        (api = useChartBars({
          tier: ref('days'),
          range: computed(() => ({ start: new Date(2026, 0, 1).getTime() })),
          dayW: ref(34),
          estCfg: computed(() => ({ unit: 'time', hoursPerDay: 8 })),
          vwindow,
          subBars: () => [],
          rowHeight: () => 36,
          findTask: (id) => tasks.find((t) => t.id === id),
          onChanged,
        })),
    )
    return api
  }

  // Two day-widths right, then let go — the same gesture cx-chart-parts drives.
  function dragTwoDays(api, task) {
    api.onBarDown(
      { button: 0, clientX: 0, preventDefault() {}, stopPropagation() {} },
      task,
      'move',
    )
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 68 }))
    window.dispatchEvent(new PointerEvent('pointerup'))
  }

  it('reports a reschedule the server refused', async () => {
    refuse(update)
    const t = { id: 'a', title: 'a', start_date: day(10), due_date: day(12) }
    const onChanged = vi.fn()
    const api = host([t], onChanged)
    dragTwoDays(api, t)
    await Promise.resolve()
    await Promise.resolve()
    expect(toast.error).toHaveBeenCalledWith('Недостаточно прав')
    // The bar still goes back to the dates the board knows.
    expect(onChanged).toHaveBeenCalled()
  })

  it('stays quiet when the reschedule persists', async () => {
    const t = { id: 'a', title: 'a', start_date: day(10), due_date: day(12) }
    const onChanged = vi.fn()
    const api = host([t], onChanged)
    dragTwoDays(api, t)
    await Promise.resolve()
    await Promise.resolve()
    expect(update).toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
  })
})

// ── the on-card subtask list ──────────────────────────────────────────────────
describe('on-card subtask drops the server refused', () => {
  const PARENT = { id: 't1', board_id: 'b1', column_id: 'c1', title: 'Родитель' }
  const SUBS = [
    { id: 's1', title: 'Первая', column_id: 'c1', parent_id: 't1' },
    { id: 's2', title: 'Вторая', column_id: 'c2', parent_id: 't1' },
  ]

  // Same stand-in cx-subtask-reorder-keeps-column uses: SortableJS needs a real
  // pointer device, so the drop is delivered through the template binding.
  const DraggableStub = {
    name: 'draggable',
    props: { list: { type: Array, default: () => [] } },
    emits: ['change'],
    template: '<div class="drag-stub"><slot name="item" v-for="s in list" :element="s" /></div>',
  }
  const stubs = {
    ...naive,
    ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
    draggable: DraggableStub,
    TaskMiniCard: true,
  }

  let TaskCardSubtasks
  beforeEach(async () => {
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    })
    seedBoardStore({ columns: COLS })
    TaskCardSubtasks = (await import('@/components/task/TaskCardSubtasks.vue')).default
  })

  const mountList = (subtasks = SUBS) =>
    mount(TaskCardSubtasks, {
      props: { task: PARENT, subtasks },
      global: { stubs, provide },
    })

  it('reports a card the parent refused to adopt', async () => {
    refuse(setParent)
    const w = mountList()
    const dropped = { id: 'x9', title: 'Чужая', column_id: 'c1' }
    await w.findComponent(DraggableStub).vm.$emit('change', { added: { element: dropped } })
    await Promise.resolve()
    expect(setParent).toHaveBeenCalledWith('x9', 't1')
    expect(toast.error).toHaveBeenCalledWith('Недостаточно прав')
    // The list is re-read either way — that is what puts the card back.
    expect(w.emitted('changed')).toBeTruthy()
  })

  it('reports a refused reorder', async () => {
    refuse(move)
    const w = mountList()
    const reordered = [SUBS[1], SUBS[0]]
    await w.setProps({ subtasks: reordered })
    await w
      .findComponent(DraggableStub)
      .vm.$emit('change', { moved: { element: SUBS[1], oldIndex: 1, newIndex: 0 } })
    await Promise.resolve()
    expect(toast.error).toHaveBeenCalledWith('Недостаточно прав')
  })

  it('stays quiet when the drop goes through', async () => {
    const w = mountList()
    const dropped = { id: 'x9', title: 'Чужая', column_id: 'c1' }
    await w.findComponent(DraggableStub).vm.$emit('change', { added: { element: dropped } })
    await Promise.resolve()
    expect(setParent).toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
  })
})
