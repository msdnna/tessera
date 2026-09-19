import { ref, computed, unref, h } from 'vue'
import { NIcon } from 'naive-ui'
import {
  OpenOutline,
  CheckmarkDoneOutline,
  FlagOutline,
  ArrowForwardOutline,
  ArchiveOutline,
  ArrowUndoOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { tasks as tasksApi } from '@/api'
import { i18n } from '@/i18n'
import { priorityOptions } from '@/utils/priority'
import { pressMoved } from '@/utils/dnd'
import { useToast } from '@/composables/useToast'
import { taskBasePatch } from '@/utils/taskPatch'
import { columnName } from '@/utils/defaultNames'

// naive's dropdown `icon` option field wants a render fn — exported so callers
// can build their `extra` items with the same look.
export const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
// The menu is built by a computed that also runs outside a component (chart and
// list views call this composable from setup, its callers' specs directly), so
// the translator is the global one rather than useI18n() (#2799).
const tr = (key) => i18n.global.t(key)
const dangerIcon = (icon) => () => h(NIcon, { color: '#e0533d' }, { default: () => h(icon) })

// Reusable right-click menu for a task, shared by the board card and the
// list/calendar/timeline/gantt views (usable anywhere a task object is on hand).
// Callbacks: onOpen(id), onChanged(). `columns` (ref/array of {id,name}) adds a
// "move to column" submenu.
//
// `extra(target)` returns caller-specific items, inserted after the divider —
// the board card puts "create subtask" there. `dangerDelete` paints the delete
// item red (the card does; the other views keep the plain look).
// `onSelect(key, task)` gets any key the composable doesn't recognise, so a
// caller's extra items act without re-implementing the menu.
//
// `readonly` (ref/getter/boolean) is the archive view: the menu simply never
// opens. Every item on it mutates the live task — «Удалить» is permanent and
// takes the task's uploads with it — and the archive is a read-only look at
// cards that are already out of the way. TaskCard has always gated its own
// contextmenu handler this way; the gate lives here so the list and the chart
// views (which have no card of their own) inherit it instead of each repeating
// the check.
//
// `onRestore(id)` is the one exception, and it is opt-in. «Вернуть из архива» is
// the archive's OWN action rather than an edit of a live card, and TaskCard has
// always carried it as the ↺ button that replaces its action bar there — so on the
// board and in the matrix (which draws the same card) an archived task can be
// restored, while Список / Календарь / Таймлайн / Гантт have no card of their own
// and nothing but this menu. The layout is per-board localStorage, so a reader
// whose board sits on «Список» opens ?archived=1, sees the archived rows and has no
// way at all to bring one back until they think to switch visualization. A caller
// that passes `onRestore` therefore gets a menu under `readonly` too — exactly two
// items, «Открыть» and «Вернуть из архива», neither of which writes to the task's
// own fields. A caller that does not (TaskCard, anyone yet to opt in) keeps the
// flat refusal.
export function useTaskMenu({
  onOpen,
  onChanged,
  onSelect,
  onRestore,
  columns,
  extra,
  dangerDelete,
  readonly,
} = {}) {
  const show = ref(false)
  const x = ref(0)
  const y = ref(0)
  const target = ref(null) // the task object the menu acts on
  const deleteConfirmShow = ref(false)
  const archiveConfirmShow = ref(false)
  // Every item on this menu writes to a live task, and a refusal used to end in a
  // bare resync: the card snapped back to where it was and said nothing, which
  // reads exactly like a click that missed. The card's OWN edits already report
  // through a toast (#2932) — the menu over that same card has to match, or the
  // two surfaces disagree about whether the server answered at all.
  const toast = useToast()
  const reportFailure = (e) => toast?.error(e?.message || String(e))

  // `columns` may be a getter function, a ref, or a plain array.
  function resolveColumns() {
    const c = typeof columns === 'function' ? columns() : unref(columns)
    return c || []
  }
  // Same three shapes for the archive flag — it is read per gesture, not captured.
  function isReadonly() {
    return !!(typeof readonly === 'function' ? readonly() : unref(readonly))
  }

  // The archive's own menu. Built from the same reactive read as the gate below, so
  // a view whose scope flips without remounting (the archive chip is a route query)
  // swaps menus with it.
  const readonlyOptions = computed(() => [
    { label: tr('task.menu.open'), key: 'open', icon: menuIcon(OpenOutline) },
    { label: tr('task.action.restore'), key: 'restore', icon: menuIcon(ArrowUndoOutline) },
  ])

  const options = computed(() => {
    const t = target.value
    if (isReadonly()) return readonlyOptions.value
    const cols = resolveColumns().filter((c) => c.id !== t?.column_id)
    return [
      { label: tr('task.menu.open'), key: 'open', icon: menuIcon(OpenOutline) },
      {
        label: tr(t?.completed_at ? 'task.menu.uncomplete' : 'task.menu.complete'),
        key: 'toggle',
        icon: menuIcon(CheckmarkDoneOutline),
      },
      {
        label: tr('task.menu.priority'),
        key: 'prio',
        icon: menuIcon(FlagOutline),
        children: priorityOptions().map((o) => ({ label: o.label, key: 'prio:' + o.value })),
      },
      ...(cols.length
        ? [
            {
              label: tr('task.menu.move'),
              key: 'move',
              icon: menuIcon(ArrowForwardOutline),
              children: cols.map((c) => ({ label: columnName(c), key: 'col:' + c.id })),
            },
          ]
        : []),
      { type: 'divider', key: 'd1' },
      ...(extra?.(t) || []),
      { label: tr('task.menu.archive'), key: 'archive', icon: menuIcon(ArchiveOutline) },
      {
        label: tr('task.menu.delete'),
        key: 'delete',
        icon: dangerDelete ? dangerIcon(TrashOutline) : menuIcon(TrashOutline),
        ...(dangerDelete ? { props: { style: 'color:#e0533d' } } : {}),
      },
    ]
  })

  function open(e, task) {
    if (pressMoved()) return
    if (isReadonly() && !onRestore) return
    target.value = task
    show.value = false
    x.value = e.clientX
    y.value = e.clientY
    // Re-open on next tick so the menu repositions if it was already showing.
    requestAnimationFrame(() => (show.value = true))
  }

  async function select(key) {
    const t = target.value
    show.value = false
    if (!t) return
    if (key === 'open') return onOpen?.(t.id)
    if (key === 'restore') return onRestore?.(t.id)
    // Nothing else may fire while the scope is read-only. The archive menu offers
    // only the two keys above, but the flag can flip (route query) with the menu
    // already open, and a dropdown that was built live would still hand us «Удалить».
    if (isReadonly()) return
    if (key === 'delete') {
      deleteConfirmShow.value = true
      return
    }
    if (key === 'archive') {
      archiveConfirmShow.value = true
      return
    }
    try {
      // taskBasePatch omits the description on purpose — these views never load
      // it, and sending an empty one would wipe the stored text.
      if (key === 'toggle') {
        await tasksApi.update(t.id, { ...taskBasePatch(t), completed: !t.completed_at })
      } else if (key.startsWith('prio:')) {
        await tasksApi.update(t.id, { ...taskBasePatch(t), priority: Number(key.slice(5)) })
      } else if (key.startsWith('col:')) {
        await tasksApi.move(t.id, { column_id: key.slice(4), before_id: null, after_id: null })
      } else {
        return onSelect?.(key, t)
      }
      onChanged?.()
    } catch (e) {
      reportFailure(e)
      onChanged?.()
    }
  }

  // Both confirms are bound to popconfirms the view keeps mounted, so they can be
  // reached by a positive-click landing after the scope turned read-only — the same
  // race `select` guards, one step further along.
  async function confirmDelete() {
    const t = target.value
    if (!t || isReadonly()) return
    try {
      await tasksApi.remove(t.id)
      onChanged?.()
    } catch (e) {
      // The loudest of the three: the reader confirmed a permanent delete and the
      // row is still there afterwards. Silence there reads as «the confirm didn't
      // take» and invites a second, third attempt at something already refused.
      reportFailure(e)
      onChanged?.()
    }
  }

  async function confirmArchive() {
    const t = target.value
    if (!t || isReadonly()) return
    try {
      await tasksApi.archive(t.id)
      onChanged?.()
    } catch (e) {
      reportFailure(e)
      onChanged?.()
    }
  }

  return {
    show,
    x,
    y,
    options,
    open,
    select,
    deleteConfirmShow,
    confirmDelete,
    archiveConfirmShow,
    confirmArchive,
  }
}
