import { getActivePinia } from 'pinia'
import { tasks as tasksApi } from '@/api'
import { useUndoStore } from '@/stores/undo'
import { useBoardViewStore } from '@/stores/boardView'

// The task trash (#3004, migration 0086). «Удалить» is reversible since the
// server keeps the task — and its subtasks and files — in the board's trash for
// TRASH_DAYS: the delete happens at once and offers its undo in a toast
// («Задача #N в корзине · Отменить», mockup CatalogA), the board's «Корзина»
// keeps «Вернуть» for the rest of the month.

export const TRASH_DAYS = 30
const DAY_MS = 24 * 3600 * 1000

// trashText names the task the way the toast does: by number when it has one.
function trashText(task, key) {
  const number = task?.number ?? null
  const title = task?.title || ''
  return number != null
    ? { key: `task.trash.${key}`, params: { number, title } }
    : { key: `task.trash.${key}Untitled`, params: { title } }
}

// deleteTask sends the task to the trash and offers the undo. onChanged runs
// after the delete and after a successful undo (the caller re-reads its list).
// Throws what the delete threw — the caller reports it.
export async function deleteTask(task, { onChanged, detach = false } = {}) {
  if (detach) await tasksApi.remove(task.id, { subtasks: 'detach' })
  else await tasksApi.remove(task.id)
  onChanged?.()
  if (!getActivePinia()) return
  useUndoStore().push({
    icon: 'trash',
    text: trashText(task, 'deleted'),
    doneText: trashText(task, 'restored'),
    undo: async () => {
      await tasksApi.untrash(task.id)
      onChanged?.()
      // The card that offered the delete is gone with it, so its own «changed»
      // reaches nobody: ask the open board to re-read its lists directly.
      useBoardViewStore().bumpReload()
    },
  })
}

// daysLeft is how many whole days a row deleted at `deletedAt` has before
// trash_purge takes it (never below 0). `now` is injectable for the tests.
export function daysLeft(deletedAt, now = Date.now()) {
  const at = Date.parse(deletedAt)
  if (Number.isNaN(at)) return TRASH_DAYS
  return Math.max(0, Math.ceil((at + TRASH_DAYS * DAY_MS - now) / DAY_MS))
}
