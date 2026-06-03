import { ref, computed } from 'vue'
import { tasks as tasksApi } from '@/api'
import { PRIORITY_LABELS } from '@/styles/tokens'

// Reusable right-click menu for a task, shared by the list/calendar views (and
// usable anywhere a task object is on hand). Callbacks: onOpen(id), onChanged().
export function useTaskMenu({ onOpen, onChanged } = {}) {
  const show = ref(false)
  const x = ref(0)
  const y = ref(0)
  const target = ref(null) // the task object the menu acts on

  const options = computed(() => {
    const t = target.value
    return [
      { label: 'Открыть', key: 'open' },
      { label: t?.completed_at ? 'Снять выполнение' : 'Отметить выполненной', key: 'toggle' },
      {
        label: 'Приоритет',
        key: 'prio',
        children: PRIORITY_LABELS.map((l, i) => ({ label: l, key: 'prio:' + i })),
      },
      { type: 'divider', key: 'd1' },
      { label: 'В архив', key: 'archive' },
      { label: 'Удалить', key: 'delete' },
    ]
  })

  function open(e, task) {
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
