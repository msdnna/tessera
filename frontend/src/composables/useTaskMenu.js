import { ref, computed, unref } from 'vue'
import { getActivePinia } from 'pinia'
import {
  OpenOutline,
  CheckmarkDoneOutline,
  FlagOutline,
  ArrowForwardOutline,
  ArchiveOutline,
  ArrowUndoOutline,
  TrashOutline,
  GitBranchOutline,
} from '@vicons/ionicons5'
import { tasks as tasksApi } from '@/api'
import { i18n } from '@/i18n'
import { priorityOptions } from '@/utils/priority'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { pressMoved } from '@/utils/dnd'
import { useToast } from '@/composables/useToast'
import { taskBasePatch } from '@/utils/taskPatch'
import { columnName } from '@/utils/defaultNames'
import { columnStatusName } from '@/utils/columnStatus'
import { sortedColumns } from '@/utils/status'
import { useBoardViewStore } from '@/stores/boardView'
import { useUndoStore } from '@/stores/undo'
import { deleteTask } from '@/utils/taskTrash'

// The menu is built by a computed that also runs outside a component (chart and
// list views call this composable from setup, its callers' specs directly), so
// the translator is the global one rather than useI18n() (#2799).
const tr = (key, params) => i18n.global.t(key, params)

// The task's row menu (#3112), shared by the board card and the list / calendar
// / timeline / gantt views (usable anywhere a task object is on hand): rows in
// kit/ActionMenu's shape — `options` — with «Приоритет ▸» and «Переместить ▸»
// as submenus and each row's key from the shortcut catalog. Draw it with
// task/TaskActionMenu.vue. Callbacks: onOpen(id), onChanged(). `columns`
// (ref/getter/array of {id,name,position}) fills «Переместить»; the done column
// is `doneColumnId` (getter) or the open board's.
//
// The v2 interaction model (#2969): «В архив» and «Удалить» both happen at once
// and offer their undo in a toast — the delete sends the task to the board's
// trash (#3004), where it stays restorable for 30 days.
//
// `extra(target)` returns caller-specific rows, inserted before the archive —
// the board card puts «Создать подзадачу» there. `onSelect(key, task)` gets any
// key the composable doesn't recognise, so a caller's extra rows act without
// re-implementing the menu.
//
// `readonly` (ref/getter/boolean) is the archive view: the menu simply never
// opens. Every row on it mutates the live task, and the archive is a read-only
// look at cards that are already out of the way. The gate lives here so the
// list and the chart views (which have no card of their own) inherit it instead
// of each repeating the check.
//
// `onRestore(id)` is the one exception, and it is opt-in. «Вернуть из архива» is
// the archive's OWN action rather than an edit of a live card — on the board
// and in the matrix the card carries it as the ↺ button — while Список /
// Календарь / Таймлайн / Гантт have no card of their own and nothing but this
// menu. A caller that passes `onRestore` therefore gets a menu under `readonly`
// too — exactly two rows, «Открыть» and «Вернуть из архива», neither of which
// writes to the task's own fields. A caller that does not keeps the flat refusal.
export function useTaskMenu({
  onOpen,
  onChanged,
  onSelect,
  onRestore,
  columns,
  doneColumnId,
  extra,
  readonly,
} = {}) {
  const show = ref(false)
  const x = ref(0)
  const y = ref(0)
  // The «⋯» that opened the menu, if a button did: it opens under it, not at
  // the pointer.
  const anchorEl = ref(null)
  const target = ref(null) // the task object the menu acts on
  // Every row on this menu writes to a live task, and a refusal used to end in a
  // bare resync: the card snapped back to where it was and said nothing, which
  // reads exactly like a click that missed. The card's OWN edits already report
  // through a toast (#2932) — the menu over that same card has to match.
  const toast = useToast()
  const reportFailure = (e) => toast?.error(e?.message || String(e))

  const read = (v) => (typeof v === 'function' ? v() : unref(v))
  // `columns` may be a getter function, a ref, or a plain array.
  const resolveColumns = () => read(columns) || []
  // Same three shapes for the archive flag — it is read per gesture, not captured.
  const isReadonly = () => !!read(readonly)
  function doneId() {
    if (doneColumnId !== undefined) return read(doneColumnId) || null
    return getActivePinia() ? useBoardViewStore().board?.done_column_id || null : null
  }

  const header = computed(() => {
    const t = target.value
    if (!t) return ''
    return t.number ? `#${t.number} ${t.title || ''}` : t.title || ''
  })

  // The archive's own menu. Built from the same reactive read as the gate below, so
  // a view whose scope flips without remounting (the archive chip is a route query)
  // swaps menus with it.
  const readonlyOptions = computed(() => [
    { key: 'open', label: tr('task.menu.open'), icon: OpenOutline, keys: 'card.open' },
    { key: 'restore', label: tr('task.action.restore'), icon: ArrowUndoOutline },
  ])

  // «Переместить ▸»: every status column with its header glyph, the task's own
  // one ticked.
  function moveRows(t) {
    const cols = sortedColumns(resolveColumns())
    const done = doneId()
    return cols.map((c, i) => ({
      key: 'col:' + c.id,
      label: columnName(c),
      glyph: columnStatusName({
        isDone: done ? c.id === done : i === cols.length - 1,
        first: i === 0,
        name: c.name,
        nameKey: c.name_key,
      }),
      checked: c.id === t?.column_id,
    }))
  }

  const options = computed(() => {
    const t = target.value
    if (isReadonly()) return readonlyOptions.value
    const cols = resolveColumns()
    return [
      { key: 'open', label: tr('task.menu.open'), icon: OpenOutline, keys: 'card.open' },
      {
        key: 'toggle',
        label: tr(t?.completed_at ? 'task.menu.uncomplete' : 'task.menu.complete'),
        icon: t?.completed_at ? ArrowUndoOutline : CheckmarkDoneOutline,
        keys: 'card.complete',
      },
      {
        key: 'prio',
        label: tr('task.menu.priority'),
        icon: FlagOutline,
        keys: 'card.priorityPick',
        subWidth: 220,
        children: priorityOptions().map((o) => ({
          key: 'prio:' + o.value,
          label: o.label,
          dot: PRIORITY_COLORS[o.value],
          caps: [String(o.value)],
          checked: (t?.priority || 0) === o.value,
        })),
      },
      ...(cols.length > 1
        ? [
            {
              key: 'move',
              label: tr('task.menu.move'),
              icon: ArrowForwardOutline,
              subWidth: 240,
              children: moveRows(t),
            },
          ]
        : []),
      ...(extra?.(t) || []),
      { gap: true },
      { key: 'archive', label: tr('task.menu.archive'), icon: ArchiveOutline },
      { key: 'delete', label: tr('task.menu.delete'), icon: TrashOutline, danger: true },
    ]
  })

  // At the pointer (a right-click), or under `el` (the card's «⋯»).
  function open(e, task, el = null) {
    if (pressMoved()) return
    if (isReadonly() && !onRestore) return
    target.value = task
    anchorEl.value = el
    show.value = false
    x.value = e?.clientX ?? 0
    y.value = e?.clientY ?? 0
    // Re-open on the next frame so the menu repositions if it was already showing.
    requestAnimationFrame(() => (show.value = true))
  }
  const placement = computed(() => (anchorEl.value ? 'bottom-end' : 'point'))
  const anchor = () => anchorEl.value || { x: x.value, y: y.value }
  function setShow(v) {
    show.value = v
  }

  async function select(key) {
    const t = target.value
    show.value = false
    if (!t) return
    if (key === 'open') return onOpen?.(t.id)
    if (key === 'restore') return onRestore?.(t.id)
    // Nothing else may fire while the scope is read-only. The archive menu offers
    // only the two keys above, but the flag can flip (route query) with the menu
    // already open, and a menu that was built live would still hand us «Удалить».
    if (isReadonly()) return
    try {
      // taskBasePatch omits the description on purpose — these views never load
      // it, and sending an empty one would wipe the stored text.
      if (key === 'toggle') {
        await tasksApi.update(t.id, { ...taskBasePatch(t), completed: !t.completed_at })
      } else if (key.startsWith('prio:')) {
        const priority = Number(key.slice(5))
        if (priority === (t.priority || 0)) return
        await tasksApi.update(t.id, { ...taskBasePatch(t), priority })
      } else if (key.startsWith('col:')) {
        if (key.slice(4) === t.column_id) return
        await tasksApi.move(t.id, { column_id: key.slice(4), before_id: null, after_id: null })
      } else if (key === 'archive') {
        await tasksApi.archive(t.id)
        offerUndo(t)
      } else if (key === 'delete') {
        // Into the trash, with the undo in a toast; deleteTask re-reads the list.
        await deleteTask(t, { onChanged })
        return
      } else {
        return onSelect?.(key, t)
      }
      onChanged?.()
    } catch (e) {
      // A row that stays where it was after a delete or a move reads as «it
      // didn't take» unless something says why.
      reportFailure(e)
      onChanged?.()
    }
  }

  // «В архив» is reversible: the undo toast brings the task back through the
  // archive's own restore.
  function offerUndo(t) {
    if (!getActivePinia()) return
    useUndoStore().push({
      icon: 'archive',
      text: { key: 'task.menu.archived', params: { title: t.title || '' } },
      doneText: { key: 'task.menu.archivedUndone', params: { title: t.title || '' } },
      undo: async () => {
        await tasksApi.restore(t.id)
        onChanged?.()
      },
    })
  }

  return {
    show,
    x,
    y,
    anchor,
    placement,
    header,
    options,
    open,
    select,
    setShow,
  }
}

// The card's own extra row (TaskCard): «Создать подзадачу» with its key.
export function subtaskRow() {
  return {
    key: 'subtask',
    label: tr('task.menu.addSubtask'),
    icon: GitBranchOutline,
    keys: 'card.subtask',
  }
}
