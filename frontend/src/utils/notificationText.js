// Renders a notification into the reader's language (#2801, stage 5 of #2796).
//
// The server stores two things per notification: `payload` — the facts of what
// happened (`{event, actor, task_number, …}`) — and `text`, the Russian sentence
// it used to build. This module turns the facts into a sentence here, in the UI,
// so the same row reads as Russian or English depending on the user's language.
//
// `text` stays the fallback and is not going away: rows written before migration
// 0065 have an empty payload, and a client that meets an event newer than its
// own bundle must still show something. Both cases land on the same branch.
import { i18n } from '@/i18n'
import { defaultFormatters } from '@/utils/format'

// Changed-task fields the server may list in a task_updated payload. An unknown
// field name (server ahead of client) is dropped rather than shown raw.
const FIELDS = [
  'title',
  'description',
  'priority',
  'due',
  'start',
  'estimate',
  'completed',
  'reopened',
]

// Title for a native (OS) notification, by notification kind.
export function notificationTitle(kind, t = i18n.global.t, te = i18n.global.te) {
  const key = `notifications.kind.${kind}`
  return te(key) ? t(key) : 'Tessera'
}

// The one-line sentence shown in the bell feed and as a native notification body.
export function notificationText(n, { t = i18n.global.t, te = i18n.global.te, formatters } = {}) {
  if (!n) return ''
  const p = n.payload || {}
  const event = p.event
  const key = `notifications.event.${event}`
  // No payload (pre-0065 row) or an event this bundle doesn't know: the server's
  // pre-rendered sentence is the only thing left, and it beats a blank line.
  if (!event || !te(key)) return n.text || ''

  // A reminder's body is the user's own text — content, not UI, so it is shown
  // verbatim; only the empty-message default is a translated string.
  if (event === 'reminder') return p.message || t(key)

  const f = formatters || defaultFormatters()
  return t(key, {
    actor: p.actor || '',
    ref: p.task_number ?? '?',
    ctx: p.title || p.excerpt ? t('notifications.ctx', { text: p.title || p.excerpt }) : '',
    column: p.column || '',
    title: p.title || '',
    label: p.label || '',
    reason: p.reason || '',
    created: p.created ?? 0,
    updated: p.updated ?? 0,
    took: formatDuration(p.seconds ?? 0, t),
    fields: fieldList(p.fields, t, f),
  })
}

// "название, срок" / "title, due date" — the changed fields, joined the way the
// reader's language joins a list (Intl.ListFormat, not a hard-coded comma).
function fieldList(fields, t, f) {
  if (!Array.isArray(fields)) return ''
  const words = fields.filter((k) => FIELDS.includes(k)).map((k) => t(`notifications.field.${k}`))
  return f.formatList(words)
}

// A sync duration, from the number of seconds the server measured. The server
// used to send this already worded ("3 м 20 с"), which no client could translate.
function formatDuration(seconds, t) {
  const total = Math.max(0, Math.round(Number(seconds) || 0))
  if (total <= 0) return t('notifications.duration.instant')
  if (total >= 3600)
    return t('notifications.duration.hm', {
      h: Math.floor(total / 3600),
      m: Math.floor((total % 3600) / 60),
    })
  if (total >= 60)
    return t('notifications.duration.ms', { m: Math.floor(total / 60), s: total % 60 })
  return t('notifications.duration.s', { s: total })
}
