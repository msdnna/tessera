// A notification as a toast «про меня» (v2, #3117 — PackageI): the same record
// the bell's feed draws as a row (NotifFeed), in the same words — the person,
// the sentence, the task as a chip, the comment as a quote — plus the actions
// the toast offers. Pure: text is resolved through `t` when the toast is drawn
// (a toast outliving a language switch speaks the new one), and what an action
// does comes in as `act` from the store's owner.
import { i18n } from '@/i18n'
import { notificationText } from '@/utils/notificationText'
import { columnName } from '@/utils/defaultNames'
import { columnStatusName, isTodoColumn } from '@/utils/columnStatus'
import { notificationEvent, notificationKind, notificationCategory } from '@/utils/toastModel'

// The small badge on the person's avatar, by event, and its tone: deletion is
// red, archiving orange, a call green, the rest the accent.
const BADGE = {
  task_assigned: ['PersonAddOutline', 'acc'],
  task_unassigned: ['PersonRemoveOutline', 'acc'],
  task_deleted: ['TrashOutline', 'red'],
  task_archived: ['ArchiveOutline', 'or'],
  task_moved: ['ArrowForwardOutline', 'acc'],
  task_completed: ['CheckmarkDoneOutline', 'acc'],
  task_reopened: ['ArrowUndoOutline', 'acc'],
  task_updated: ['CreateOutline', 'acc'],
  task_comment: ['ChatbubbleEllipsesOutline', 'acc'],
  task_thread_reply: ['ChatbubbleEllipsesOutline', 'acc'],
  task_mention: ['AtOutline', 'acc'],
  conference_mention: ['AtOutline', 'acc'],
  conference_invited: ['VideocamOutline', 'gr'],
}

// A notification without a person leads with a tile instead.
const TILE = {
  reminder: ['AlarmOutline', 'or'],
  task_due_soon: ['CalendarOutline', 'or'],
  integration_sync_ok: ['GitMergeOutline', 'neu'],
  integration_sync_partial: ['GitMergeOutline', 'or'],
  integration_sync_failed: ['GitMergeOutline', 'red'],
}

// The chip's status glyph: the column the task stands in, as the board draws
// it. The live event carries the column (#3117); a row from the list endpoint
// does not, and its chip goes without a glyph.
export function chipStatus(n) {
  if (n.task_done) return 'status-done'
  if (!n.task_column && !n.task_column_key) return ''
  return columnStatusName({
    isDone: false,
    first: isTodoColumn(n.task_column_key, n.task_column),
    name: n.task_column,
    nameKey: n.task_column_key,
  })
}

export function chipOf(n) {
  const p = n.payload || {}
  const ev = notificationEvent(n)
  if (
    ev === 'conference_invited' ||
    ev === 'conference_mention' ||
    ev === 'reminder' ||
    ev.startsWith('integration_sync')
  )
    return null
  const title = n.task_title || p.title || ''
  const ref = n.task_number ?? p.task_number ?? null
  if (!title && ref == null) return null
  return { ref, title, status: chipStatus(n), gone: ev === 'task_deleted' }
}

// The sentence split into the bold person and the rest, as the feed does.
export function sentenceParts(n, { t, te, chip = true } = {}) {
  const text = notificationText(n, { t, te, ctx: !chip })
  const actor = n.payload?.actor || ''
  if (actor && text.startsWith(actor)) return { bold: actor, rest: text.slice(actor.length).trim() }
  return { bold: '', rest: text }
}

// buildNotificationToast → the toast spec the store shows. `act` says what the
// buttons do — open(n), reply(n), joinCall(n), snooze(n, el), done(n),
// openGitlab(n) — wired by the store's owner to the router and the stores.
export function buildNotificationToast(
  n,
  { act = {}, t = i18n.global.t, te = i18n.global.te } = {},
) {
  const p = n.payload || {}
  const ev = notificationEvent(n)
  const kind = notificationKind(n)
  const chip = chipOf(n)
  const spec = {
    lane: 'top',
    kind,
    category: notificationCategory(n),
    event: ev,
    notificationId: n.id,
    taskId: n.task_id || null,
    time: n.created_at || null,
    chip,
    quote: p.quote || p.excerpt || '',
    actions: [],
  }
  const parts = () => sentenceParts(n, { t, te, chip: !!(chip || spec.quote) })
  spec.bold = () => parts().bold
  spec.rest = () => parts().rest
  spec.speech = () => notificationText(n, { t, te })

  const actor = p.actor || ''
  if (actor) {
    const [badge, tone] = BADGE[ev] || ['NotificationsOutline', 'acc']
    spec.lead = { avatar: { userId: n.actor_id || '', name: actor }, badge, tone }
  } else {
    const [tile, tone] = TILE[ev] || ['NotificationsOutline', 'acc']
    spec.lead = { tile, tone }
  }

  // Line three of a move: where the task went.
  if (ev === 'task_moved' && (p.column || p.column_key)) {
    spec.diff = {
      to: () => columnName({ name: p.column, name_key: p.column_key }, { t, te }),
    }
  }

  const open = {
    id: 'open',
    label: () => t('toasts.act.open'),
    caps: ['↵'],
    primary: true,
    run: () => act.open?.(n),
  }
  switch (ev) {
    case 'task_comment':
    case 'task_mention':
    case 'task_thread_reply':
      spec.actions = [
        open,
        {
          id: 'reply',
          label: () => t('toasts.act.reply'),
          caps: ['R'],
          key: 'r',
          run: () => act.reply?.(n),
        },
      ]
      break
    case 'task_deleted':
      // The task is gone for good: tasks have no trash yet — a visible stub.
      spec.actions = [
        {
          id: 'trash',
          label: () => t('toasts.act.trash'),
          caps: ['↵'],
          primary: true,
          stub: 'toasts.taskTrash',
        },
      ]
      break
    case 'conference_mention':
      // A mention in a call's chat (#3055) opens the call, whose chat has it.
      spec.actions = [open]
      break
    case 'conference_invited':
      spec.actions = [
        {
          id: 'join',
          label: () => t('toasts.act.join'),
          caps: ['↵'],
          primary: true,
          run: () => act.joinCall?.(n),
        },
        { id: 'later', label: () => t('toasts.act.notNow'), caps: ['Esc'], dismiss: true },
      ]
      spec.always = true
      break
    case 'reminder':
      spec.bold = () => p.message || t('notifications.event.reminder')
      spec.rest = ''
      spec.speech = spec.bold
      spec.line = () => t('toasts.reminder.line')
      spec.actions = [
        {
          id: 'snooze',
          label: () => t('toasts.act.snooze'),
          caps: ['Z'],
          key: 'z',
          keep: true,
          run: (el) => act.snooze?.(n, el),
        },
        {
          id: 'done',
          label: () => t('toasts.act.done'),
          caps: ['E'],
          key: 'e',
          primary: true,
          run: () => act.done?.(n),
        },
      ]
      spec.always = true
      break
    default:
      if (ev.startsWith('integration_sync')) {
        spec.actions = [
          { id: 'journal', label: () => t('toasts.act.journal'), run: () => act.openGitlab?.(n) },
        ]
        spec.always = true
      } else if (n.task_id) {
        spec.actions = [open]
      }
  }
  return spec
}
