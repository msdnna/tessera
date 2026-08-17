// Formatting for the task's activity feed — comment timestamps and journal lines.
// Extracted from TaskModal so the comments and history tabs share one implementation
// (and so the journal wording is unit-testable without mounting the modal).
import { PRIORITY_LABELS } from '@/styles/tokens'

// Short "12 янв, 14:03" stamp used on comment and journal rows.
export function fmtWhen(d) {
  return new Date(d).toLocaleString('ru-RU', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// Group the flat comment list the API returns into threads: a root plus the
// replies hanging off it. The server sends a flat array (so Android and the MCP
// server keep working) and the tree is assembled here.
//
// A reply whose parent is missing from the list — deleted, or filtered out —
// is promoted to a root of its own rather than dropped: losing someone's text
// because its parent went away is worse than showing it unindented.
export function groupThreads(comments) {
  const list = comments || []
  const byId = new Map(list.map((c) => [c.id, c]))
  const threads = []
  const byRoot = new Map()
  for (const c of list) {
    const parentId = c.parent_id && byId.has(c.parent_id) ? c.parent_id : null
    if (!parentId) {
      const t = { root: c, replies: [] }
      threads.push(t)
      byRoot.set(c.id, t)
      continue
    }
    const t = byRoot.get(parentId)
    if (t) t.replies.push(c)
    // Parent exists but its thread hasn't been seen yet (an out-of-order list):
    // fall back to a root so the comment stays visible.
    else threads.push({ root: c, replies: [] })
  }
  return threads
}

// Human sentence for a journal event, appended after the actor's name.
export function eventText(e) {
  const d = e.data || {}
  switch (e.kind) {
    case 'created':
      return 'создал(а) задачу'
    case 'renamed':
      return `переименовал(а) → «${d.to ?? ''}»`
    case 'description':
      return 'изменил(а) описание'
    case 'priority':
      return `изменил(а) приоритет → ${PRIORITY_LABELS[d.to] ?? d.to}`
    case 'due':
      return d.set ? 'установил(а) срок' : 'убрал(а) срок'
    case 'completed':
      return 'отметил(а) выполненной'
    case 'reopened':
      return 'вернул(а) в работу'
    case 'recurred':
      return 'перенёс(ла) повтор задачи'
    case 'moved':
      return `переместил(а)${d.to ? ` → «${d.to}»` : ''}`
    case 'assigned':
      return 'назначил(а) исполнителя'
    case 'unassigned':
      return 'снял(а) исполнителя'
    case 'archived':
      return 'отправил(а) в архив'
    case 'restored':
      return 'восстановил(а) из архива'
    case 'comment':
      return 'оставил(а) комментарий'
    case 'relation':
      return `добавил(а) связь с #${d.related ?? ''}`
    case 'attachment':
      return `прикрепил(а) файл${d.filename ? ` «${d.filename}»` : ''}`
    default:
      return e.kind
  }
}
