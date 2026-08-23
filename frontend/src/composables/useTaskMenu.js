import { ref, computed, unref, h } from 'vue'
import { NIcon } from 'naive-ui'
import {
  OpenOutline,
  CheckmarkDoneOutline,
  FlagOutline,
  ArrowForwardOutline,
  ArchiveOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { tasks as tasksApi } from '@/api'
import { i18n } from '@/i18n'
import { priorityOptions } from '@/utils/priority'
import { pressMoved } from '@/utils/dnd'
import { taskBasePatch } from '@/utils/taskPatch'

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
export function useTaskMenu({ onOpen, onChanged, onSelect, columns, extra, dangerDelete } = {}) {
  const show = ref(false)
  const x = ref(0)
  const y = ref(0)
  const target = ref(null) // the task object the menu acts on
  const deleteConfirmShow = ref(false)
  const archiveConfirmShow = ref(false)

  // `columns` may be a getter function, a ref, or a plain array.
  function resolveColumns() {
    const c = typeof columns === 'function' ? columns() : unref(columns)
    return c || []
  }

  const options = computed(() => {
    const t = target.value
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
              children: cols.map((c) => ({ label: c.name, key: 'col:' + c.id })),
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
    } catch {
      onChanged?.()
    }
  }

  async function confirmDelete() {
    const t = target.value
    if (!t) return
    try {
      await tasksApi.remove(t.id)
      onChanged?.()
    } catch {
      onChanged?.()
    }
  }

  async function confirmArchive() {
    const t = target.value
    if (!t) return
    try {
      await tasksApi.archive(t.id)
      onChanged?.()
    } catch {
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
