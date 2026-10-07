import { useI18n } from 'vue-i18n'
import { useMessage } from 'naive-ui'
import { useRouter } from 'vue-router'
import { boards as boardsApi, documents as docsApi } from '@/api'
import { useInboxStore } from '@/stores/inbox'
import { useUndoStore } from '@/stores/undo'
import { useBoardViewStore } from '@/stores/boardView'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useFormat } from './useFormat'
import { sGet } from '@/utils/serverStorage'
import { sortedColumns } from '@/utils/status'
import { todoColumn } from '@/utils/columnStatus'
import { humanizeError } from '@/utils/errors'
import { taskPath } from '@/components/palette/rows'

// What can be done to a «Входящие» record (#2997) — shared by the panel, the
// start screen's block, the task modal's section and the card's bell pill, so a
// key, a menu row and a button all do the same thing. Reversible actions happen
// at once and offer their undo in a toast (#2969): complete, snooze, delete.

export function recordTitle(rec) {
  return (rec.kind === 'note' ? rec.title : rec.message) || ''
}

export function useInboxActions() {
  const { t } = useI18n()
  const message = useMessage()
  const router = useRouter()
  const inbox = useInboxStore()
  const undo = useUndoStore()
  const board = useBoardViewStore()
  const ws = useWorkspacesStore()
  const { formatDue } = useFormat()

  const fail = (e) => message.error(humanizeError(e))

  async function complete(rec) {
    if (rec.kind !== 'reminder') return
    const was = rec.done
    try {
      await inbox.setDone(rec, !was)
      if (was) return
      undo.push({
        icon: 'check',
        text: { key: 'inbox.toast.done', params: { title: recordTitle(rec) } },
        undo: () => inbox.setDone(rec, false),
      })
    } catch (e) {
      fail(e)
    }
  }

  async function completeAll(list) {
    const open = list.filter((r) => r.kind === 'reminder' && !r.done)
    if (!open.length) return
    try {
      await Promise.all(open.map((r) => inbox.setDone(r, true)))
      undo.push({
        icon: 'check',
        text: { key: 'inbox.toast.doneAll', params: { n: open.length } },
        undo: () => Promise.all(open.map((r) => inbox.setDone(r, false))),
      })
    } catch (e) {
      fail(e)
    }
  }

  async function snooze(rec, at) {
    if (rec.kind !== 'reminder') return
    const before = { remind_at: rec.remind_at, snoozed_from: rec.snoozed_from ?? null }
    try {
      const saved = await inbox.snooze(rec, at)
      undo.push({
        icon: 'moon',
        text: { key: 'inbox.toast.snoozed', params: { time: formatDue(saved.remind_at) } },
        undo: () => inbox.updateReminder(saved, before),
      })
    } catch (e) {
      fail(e)
    }
  }

  async function remove(rec) {
    try {
      await inbox.remove(rec)
      undo.push({
        text: { key: 'inbox.toast.deleted' },
        doneText: { key: 'inbox.toast.restored' },
        undo: () => inbox.restore(rec),
      })
    } catch (e) {
      fail(e)
    }
  }

  async function link(rec, task) {
    try {
      if (rec.kind === 'note') await inbox.updateNote(rec, { task_id: task?.id ?? null })
      else await inbox.updateReminder(rec, { task_id: task?.id ?? null })
      message.success(
        task ? t('inbox.toast.linked', { n: task.number }) : t('inbox.toast.unlinked'),
      )
    } catch (e) {
      fail(e)
    }
  }

  async function toggleVisibility(rec) {
    if (rec.kind !== 'note') return
    const next = rec.visibility === 'private' ? 'workspace' : 'private'
    try {
      await inbox.updateNote(rec, { visibility: next })
      message.success(t(next === 'private' ? 'inbox.toast.private' : 'inbox.toast.shared'))
    } catch (e) {
      fail(e)
    }
  }

  // «T — в задачу»: a task in «К работе» of the open board (else the last one),
  // the record's text as its description; a reminder's time becomes its due.
  // The record goes away — it has become the task.
  async function toTask(rec) {
    const boardId = (board.active && board.boardId) || sGet('tessera_last_board_id')
    if (!boardId) return message.warning(t('inbox.toast.needBoard'))
    try {
      const cols = (await boardsApi.columns(boardId)).data || []
      const col = todoColumn(sortedColumns(cols))
      if (!col) throw new Error(t('board.column.needStatus'))
      const res = await boardsApi.createTask(boardId, {
        column_id: col.id,
        title: recordTitle(rec) || t('inbox.item.untitled'),
        description: rec.body || '',
        due_date: rec.kind === 'reminder' && !rec.done ? rec.remind_at : undefined,
      })
      await inbox.remove(rec)
      message.success(t('inbox.toast.taskCreated', { n: res.data.number }))
      router.push(taskPath({ ...res.data, board_id: boardId }))
    } catch (e) {
      fail(e)
    }
  }

  // «⇧D — в документ»: a document of the workspace with the record's text.
  async function toDoc(rec) {
    try {
      const title = recordTitle(rec) || t('inbox.item.untitled')
      const res = await docsApi.create(ws.currentId, { title })
      if (rec.body?.trim()) {
        const { markdownToDoc } = await import('@/utils/docImport')
        await docsApi.updateContent(res.data.id, markdownToDoc(rec.body), res.data.updated_at)
      }
      await inbox.remove(rec)
      message.success(t('inbox.toast.docCreated', { title }))
      router.push(`/documents/${res.data.slug || res.data.id}`)
    } catch (e) {
      fail(e)
    }
  }

  function openTask(rec) {
    if (rec.task) router.push(taskPath(rec.task))
  }

  return {
    complete,
    completeAll,
    snooze,
    remove,
    link,
    toggleVisibility,
    toTask,
    toDoc,
    openTask,
  }
}
