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
