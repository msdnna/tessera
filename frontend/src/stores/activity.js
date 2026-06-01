import { defineStore } from 'pinia'
import { ref } from 'vue'

// Human-readable labels for the subset of realtime events worth surfacing in
// the activity bell (the rest — tagged/untagged/column moves — are too noisy).
const LABELS = {
  'task.created': 'Создана задача',
  'task.updated': 'Задача обновлена',
  'task.moved': 'Задача перемещена',
  'task.deleted': 'Задача удалена',
  'board.created': 'Создана доска',
  'project.created': 'Создан проект',
  'note.created': 'Создана заметка',
}

// activity store — an in-memory feed built from WebSocket events for the
// current workspace. Persistent server-side notifications are deferred.
export const useActivityStore = defineStore('activity', () => {
  const items = ref([])
  const unread = ref(0)

  function push(ev, meId) {
    let text = LABELS[ev.type]
    if (ev.type === 'task.assigned') {
      text = ev.data?.user_id === meId ? 'Вам назначили задачу' : 'Назначен исполнитель'
    }
    if (!text) return
    items.value.unshift({ id: `${Date.now()}-${Math.random()}`, text, at: new Date() })
    if (items.value.length > 30) items.value.pop()
    unread.value++
  }

  function markRead() {
    unread.value = 0
  }

  return { items, unread, push, markRead }
})
