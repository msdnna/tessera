// The bell feed's own reading of a notification (v2, #3107): which filter it
// belongs to, and how old it is in the feed's terse words. Pure — the
// component hands in `t` and the formatter set, so this stays testable.

// The filters across the top of the feed, in order. «all» shows every row.
export const NOTIFICATION_FILTERS = ['all', 'mentions', 'assigned', 'gitlab']

// Which filter a row falls under. The kind is the stable name (the payload's
// event is newer and absent on old rows); a row of any other kind is only under
// «all».
export function notificationGroup(n) {
  const kind = n?.kind || ''
  const event = n?.payload?.event || ''
  if (kind === 'mention' || event === 'task_mention') return 'mentions'
  if (kind === 'assigned' || event === 'task_assigned') return 'assigned'
  if (kind === 'integration_sync' || event.startsWith('integration_sync')) return 'gitlab'
  return 'other'
}

// «только что», «5 мин», «3 ч», «08:20» (earlier today), «вчера», «пн», «12 окт.» —
// the time column of a row.
export function notificationAge(value, { t, formatters, now = new Date() }) {
  const d = value ? new Date(value) : null
  if (!d || Number.isNaN(d.getTime())) return ''
  const mins = Math.floor((now.getTime() - d.getTime()) / 60000)
  if (mins < 1) return t('notifications.pop.now')
  if (mins < 60) return t('notifications.pop.minutes', { n: mins })
  if (mins < 12 * 60) return t('notifications.pop.hours', { n: Math.floor(mins / 60) })
  const f = formatters
  const sameDay = (a, b) => a && b && a.year === b.year && a.month === b.month && a.day === b.day
  if (sameDay(f.parts(d), f.today(now))) return f.formatTime(d)
  const rel = f.formatRelative(d, { now, dateOnly: false })
  if (rel) return rel
  return f.formatDate(d, { day: 'numeric', month: 'short' })
}
