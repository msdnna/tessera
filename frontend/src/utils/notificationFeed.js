// The bell feed's own reading of a notification (v2, #3107): which filter it
// belongs to, and how old it is in the feed's terse words. Pure — the
// component hands in `t` and the formatter set, so this stays testable.

// The filters across the top of the feed, in order. «all» shows every row.
export const NOTIFICATION_FILTERS = ['all', 'mentions', 'assigned', 'gitlab']

// The page of all notifications (#3049) adds «Непрочитанные»: there the filter
// is the server's, and the groups are days rather than «Новые / Ранее».
export const PAGE_FILTERS = ['all', 'unread', 'mentions', 'assigned', 'gitlab']

// Does a row belong under a filter of the page? Mirrors the server's
// ListNotificationsPage, so a row arriving over the socket lands where the
// next page load would put it.
export function inFilter(n, filter) {
  if (filter === 'all') return true
  if (filter === 'unread') return !n?.read_at
  return notificationGroup(n) === filter
}

// «Сегодня», «Вчера», then the date — the day groups of the page. `key` keeps
// rows of one day together however the label is worded.
export function dayGroup(value, { t, formatters, now = new Date() }) {
  const d = value ? new Date(value) : null
  if (!d || Number.isNaN(d.getTime())) return { key: '', label: '' }
  const f = formatters
  const p = f.parts(d)
  const key = `${p.year}-${p.month}-${p.day}`
  const today = f.today(now)
  const y = f.today(new Date(now.getTime() - 24 * 3600 * 1000))
  const same = (a, b) => a && b && a.year === b.year && a.month === b.month && a.day === b.day
  if (same(p, today)) return { key, label: t('notifications.pop.today') }
  if (same(p, y)) return { key, label: t('notifications.pop.yesterday') }
  return { key, label: f.formatDate(d, { day: 'numeric', month: 'long', weekday: 'short' }) }
}

// Rows in day groups, keeping their order (newest first).
export function groupByDay(rows, opts) {
  const out = []
  for (const n of rows) {
    const g = dayGroup(n.created_at, opts)
    const last = out[out.length - 1]
    if (last && last.key === g.key) last.rows.push(n)
    else out.push({ ...g, rows: [n] })
  }
  return out
}

// Which filter a row falls under. The kind is the stable name (the payload's
// event is newer and absent on old rows); a row of any other kind is only under
// «all».
export function notificationGroup(n) {
  const kind = n?.kind || ''
  const event = n?.payload?.event || ''
  if (kind === 'mention' || event === 'task_mention') return 'mentions'
  if (kind === 'assigned' || event === 'task_assigned') return 'assigned'
  if (
    kind === 'integration_sync' ||
    kind === 'integration_conflict' ||
    event.startsWith('integration_sync') ||
    event === 'integration_conflict'
  )
    return 'gitlab'
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
