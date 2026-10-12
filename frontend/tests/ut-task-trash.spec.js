import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// The task trash (#3004): «Удалить» sends the task to the board's trash at once
// and offers the undo in a toast; the undo is the trash's own restore (POST),
// not the archive's.
vi.mock('@/api', () => ({
  tasks: {
    remove: vi.fn().mockResolvedValue({}),
    untrash: vi.fn().mockResolvedValue({}),
    restore: vi.fn().mockResolvedValue({}),
  },
}))

import { tasks as tasksApi } from '@/api'
import { useUndoStore } from '@/stores/undo'
import { deleteTask, daysLeft, TRASH_DAYS } from '@/utils/taskTrash'
import { isTrashQuery, trashQuery, withoutTrashQuery } from '@/utils/boardScope'
import { buildNotificationToast } from '@/utils/notificationToast'

describe('deleteTask', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('deletes, re-reads and offers an undo that restores from the trash', async () => {
    const onChanged = vi.fn()
    await deleteTask({ id: 't1', number: 2949, title: 'Webhook' }, { onChanged })
    expect(tasksApi.remove).toHaveBeenCalledWith('t1')
    expect(onChanged).toHaveBeenCalledTimes(1)

    const undo = useUndoStore()
    expect(undo.toasts).toHaveLength(1)
    const toast = undo.toasts[0]
    expect(toast.icon).toBe('trash')
    expect(toast.text).toEqual({
      key: 'task.trash.deleted',
      params: { number: 2949, title: 'Webhook' },
    })

    await undo.undo(toast.id)
    expect(tasksApi.untrash).toHaveBeenCalledWith('t1')
    expect(tasksApi.restore).not.toHaveBeenCalled()
    expect(onChanged).toHaveBeenCalledTimes(2)
    expect(undo.toasts[0].status).toBe('done')
  })

  it('names a task without a number by its title', async () => {
    await deleteTask({ id: 't2', title: 'Без номера' })
    expect(useUndoStore().toasts[0].text.key).toBe('task.trash.deletedUntitled')
  })

  it('keeps the subtasks on the board when asked to detach them', async () => {
    await deleteTask({ id: 't3', number: 1 }, { detach: true })
    expect(tasksApi.remove).toHaveBeenCalledWith('t3', { subtasks: 'detach' })
  })

  it('offers no undo when the delete was refused', async () => {
    tasksApi.remove.mockRejectedValueOnce(new Error('409'))
    await expect(deleteTask({ id: 't4', number: 4 })).rejects.toThrow('409')
    expect(useUndoStore().toasts).toHaveLength(0)
  })
})

describe('daysLeft', () => {
  const day = 24 * 3600 * 1000
  const now = Date.parse('2026-10-10T12:00:00Z')

  it('counts the whole days the trash still keeps a row', () => {
    expect(daysLeft('2026-10-10T12:00:00Z', now)).toBe(TRASH_DAYS)
    expect(daysLeft(new Date(now - 1 * day).toISOString(), now)).toBe(TRASH_DAYS - 1)
    expect(daysLeft(new Date(now - 29.5 * day).toISOString(), now)).toBe(1)
  })

  it('never goes below zero and survives a bad date', () => {
    expect(daysLeft(new Date(now - 40 * day).toISOString(), now)).toBe(0)
    expect(daysLeft('nonsense', now)).toBe(TRASH_DAYS)
  })
})

describe('the trash query', () => {
  it('opens and closes the panel without touching the rest of the address', () => {
    const q = trashQuery({ milestone: 'm1', layout: 'list' }, 2949)
    expect(q).toEqual({ milestone: 'm1', layout: 'list', trash: '1', hl: '2949' })
    expect(isTrashQuery(q)).toBe(true)
    expect(withoutTrashQuery(q)).toEqual({ milestone: 'm1', layout: 'list' })
    expect(isTrashQuery({ trash: '0' })).toBe(false)
    expect(trashQuery({ hl: '5' })).toEqual({ trash: '1' })
  })
})

describe('the toast about a deleted task (#3120)', () => {
  it('«В корзину» is a live action, not a stub', () => {
    const trash = vi.fn()
    const n = {
      id: 'n1',
      kind: 'deleted',
      payload: { event: 'task_deleted', task_number: 7, title: 'X', board_id: 'b1' },
    }
    const spec = buildNotificationToast(n, { act: { trash } })
    const act = spec.actions.find((a) => a.id === 'trash')
    expect(act.stub).toBeUndefined()
    act.run()
    expect(trash).toHaveBeenCalledWith(n)
  })
})
