import { ref, computed, unref } from 'vue'
import { tasks as tasksApi } from '@/api'
import { PRIORITY_LABELS } from '@/styles/tokens'
import { pressMoved } from '@/utils/dnd'

// Reusable right-click menu for a task, shared by the list/calendar views (and
// usable anywhere a task object is on hand). Callbacks: onOpen(id), onChanged().
// `columns` (ref/array of {id,name}) adds a "move to column" submenu.
export function useTaskMenu({ onOpen, onChanged, columns } = {}) {
  const show = ref(false)
  const x = ref(0)
  const y = ref(0)
  const target = ref(null) // the task object the menu acts on

  const options = computed(() => {
    const t = target.value
    const cols = (unref(columns) || []).filter((c) => c.id !== t?.column_id)
    return [
      { label: 'Открыть', key: 'open' },
      { label: t?.completed_at ? 'Снять выполнение' : 'Отметить выполненной', key: 'toggle' },
      {
        label: 'Приоритет',
        key: 'prio',
        children: PRIORITY_LABELS.map((l, i) => ({ label: l, key: 'prio:' + i })),
      },
      ...(cols.length
        ? [
            {
              label: 'Переместить в колонку',
              key: 'move',
              children: cols.map((c) => ({ label: c.name, key: 'col:' + c.id })),
            },
          ]
        : []),
      { type: 'divider', key: 'd1' },
      { label: 'В архив', key: 'archive' },
      { label: 'Удалить', key: 'delete' },
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

  function base(t) {
    return {
      title: t.title,
      description: t.description || '',
      priority: t.priority || 0,
      due_date: t.due_date || null,
      completed: !!t.completed_at,
    }
  }

  async function select(key) {
    const t = target.value
    show.value = false
    if (!t) return
    if (key === 'open') return onOpen?.(t.id)
    try {
      if (key === 'toggle') {
        await tasksApi.update(t.id, { ...base(t), completed: !t.completed_at })
      } else if (key.startsWith('prio:')) {
        await tasksApi.update(t.id, { ...base(t), priority: Number(key.slice(5)) })
      } else if (key.startsWith('col:')) {
        await tasksApi.move(t.id, { column_id: key.slice(4), before_id: null, after_id: null })
      } else if (key === 'archive') {
        await tasksApi.archive(t.id)
      } else if (key === 'delete') {
        await tasksApi.remove(t.id)
      }
      onChanged?.()
    } catch {
      onChanged?.()
    }
  }

  return { show, x, y, options, open, select }
}
