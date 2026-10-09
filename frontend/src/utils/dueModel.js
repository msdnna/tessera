// The pure half of the due-date picker (v2, #3118 — DuePicker): reading a due
// into calendar terms and back, the presets, the human summaries of a repeat and
// of the due notification. Kept out of the components so it is unit-testable
// with a synthetic formatter set.
//
// Two rules, the app's everywhere (#2932, utils/calendarView.js):
//   • a date-only due is a pure UTC midnight — it reads as that day in every zone;
//   • a due with a time of day is an instant, its clock read in the user's
//     PROFILE timezone (`fmt.parts`), the zone every label of it is drawn in.
// The picker works on WALL values — calendar coordinates { year, month, day }
// and an 'HH:MM' — and only `writeEndpoint` turns one back into an instant.
import { i18n } from '@/i18n'
import { isDateOnlyMs } from '@/utils/calendarView'
import { nextOccurrence, occurrenceKeys, withStoredAnchor } from '@/utils/recurrence'
import { columnName } from '@/utils/defaultNames'

const t = (...a) => i18n.global.t(...a)

export const pad2 = (n) => String(n).padStart(2, '0')
export const dayKey = (p) => (p ? `${p.year}-${pad2(p.month + 1)}-${pad2(p.day)}` : '')
export const sameDay = (a, b) =>
  !!a && !!b && a.year === b.year && a.month === b.month && a.day === b.day
export const coordsOf = (d) => ({ year: d.getFullYear(), month: d.getMonth(), day: d.getDate() })
export const dateOf = (p) => new Date(p.year, p.month, p.day)
export const addDays = (p, n) => coordsOf(new Date(p.year, p.month, p.day + n))
export function addMonths(p, n) {
  const d = new Date(p.year, p.month + n, 1)
  const last = new Date(d.getFullYear(), d.getMonth() + 1, 0).getDate()
  return { year: d.getFullYear(), month: d.getMonth(), day: Math.min(p.day, last) }
}
export const epochOf = (p) => Math.round(Date.UTC(p.year, p.month, p.day) / 86400000)

// The reader's wall clock right now, as a local Date (profile timezone fields).
export function wallNow(fmt, now = new Date()) {
  const p = fmt.today(now)
  return new Date(p.year, p.month, p.day, p.hour, p.minute, 0, 0)
}

// One endpoint (due or start) in wall terms: { day, time } — `time` null for a
// date-only value. A legacy local midnight (what the old editor wrote for a date
// picked without a clock) reads as «без времени» too: it is labelled that way
// everywhere (`formatDue` shows no clock at 00:00).
export function readEndpoint(iso, fmt) {
  if (!iso) return null
  const ms = typeof iso === 'number' ? iso : Date.parse(iso)
  if (Number.isNaN(ms)) return null
  if (isDateOnlyMs(ms)) {
    const d = new Date(ms)
    return {
      day: { year: d.getUTCFullYear(), month: d.getUTCMonth(), day: d.getUTCDate() },
      time: null,
    }
  }
  const p = fmt.parts(new Date(ms))
  const day = { year: p.year, month: p.month, day: p.day }
  return { day, time: p.hour || p.minute ? `${pad2(p.hour)}:${pad2(p.minute)}` : null }
}

// The instant a wall clock reading names in `tz` ('' / undefined = the browser).
export function zonedInstant(p, h, m, tz) {
  if (!tz) return new Date(p.year, p.month, p.day, h, m, 0, 0).getTime()
  const f = new Intl.DateTimeFormat('en-US', {
    timeZone: tz,
    hourCycle: 'h23',
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: 'numeric',
  })
  const want = Date.UTC(p.year, p.month, p.day, h, m)
  const seen = (ms) => {
    const o = {}
    for (const x of f.formatToParts(new Date(ms)))
      if (x.type !== 'literal') o[x.type] = Number(x.value)
    return Date.UTC(o.year, o.month - 1, o.day, o.hour % 24, o.minute)
  }
  // Two rounds settle every offset, the hour a clock change skips included.
  let ms = want - (seen(want) - want)
  ms -= seen(ms) - want
  return ms
}

// A wall reading back to the wire. No clock → the day's UTC midnight (date-only).
// With one → that clock in `tz`; an instant that happens to land on a UTC
// midnight (20:00 in New York in summer, 03:00 in Moscow) gets one second, or
// every reader would take it for a date without a time and show the next day
// (#2932 — the second is invisible: no label shows seconds).
export function writeEndpoint(day, time, tz) {
  if (!day) return null
  if (!time) return new Date(Date.UTC(day.year, day.month, day.day)).toISOString()
  const [h, m] = time.split(':').map(Number)
  let ms = zonedInstant(day, h, m, tz)
  if (isDateOnlyMs(ms)) ms += 1000
  return new Date(ms).toISOString()
}

// The local Date the recurrence preview steps from: the wall reading itself.
export function wallDate(ep) {
  if (!ep) return null
  const [h, m] = (ep.time || '00:00').split(':').map(Number)
  return new Date(ep.day.year, ep.day.month, ep.day.day, h, m, 0, 0)
}

// The zone a rule's calendar arithmetic runs in (`recur.Rule.TZ`): the one its
// due is read in. A date-only due is a UTC day; a timed one, the profile's.
export function ruleZone(dueIso, profileTz) {
  if (!dueIso) return profileTz || ''
  return isDateOnlyMs(Date.parse(dueIso)) ? 'UTC' : profileTz || ''
}

// The grid's keys (#3118): arrows by days, PgUp / PgDn by months (⇧ — years),
// T — today (by the physical key, so «е» on a Russian layout works too). The
// new keyboard day, or null for a key that is not the grid's.
export function gridStep(e, focus, today) {
  const f = focus || today
  if (e.metaKey || e.ctrlKey || e.altKey) return null
  switch (e.key) {
    case 'ArrowLeft':
      return addDays(f, -1)
    case 'ArrowRight':
      return addDays(f, 1)
    case 'ArrowUp':
      return addDays(f, -7)
    case 'ArrowDown':
      return addDays(f, 7)
    case 'PageUp':
      return addMonths(f, e.shiftKey ? -12 : -1)
    case 'PageDown':
      return addMonths(f, e.shiftKey ? 12 : 1)
    case 'Home':
      return addDays(f, -((dateOf(f).getDay() + 6) % 7))
    case 'End':
      return addDays(f, 6 - ((dateOf(f).getDay() + 6) % 7))
  }
  if (e.code === 'KeyT' && !e.shiftKey) return today
  return null
}

// ── presets (1–5, 0) ──
export const PRESET_KEYS = ['today', 'tomorrow', 'weekEnd', 'nextMonday', 'inWeek']
export function presetDay(key, today) {
  const dow = dateOf(today).getDay()
  switch (key) {
    case 'today':
      return today
    case 'tomorrow':
      return addDays(today, 1)
    case 'weekEnd':
      return addDays(today, (5 - dow + 7) % 7)
    case 'nextMonday':
      return addDays(today, (1 - dow + 7) % 7 || 7)
    case 'inWeek':
      return addDays(today, 7)
    default:
      return null
  }
}

// ── labels ──
// «пт, 9 окт.» / «пт, 9 окт., 18:00» — a wall reading, so formatted in UTC from
// its own fields (the zone has already been applied).
export function wallLabel(ep, fmt, { weekday = true } = {}) {
  if (!ep) return ''
  const [h, m] = (ep.time || '00:00').split(':').map(Number)
  const d = new Date(Date.UTC(ep.day.year, ep.day.month, ep.day.day, h, m))
  const o = { day: 'numeric', month: 'short', timeZone: 'UTC' }
  if (weekday) o.weekday = 'short'
  const now = fmt.today()
  if (ep.day.year !== now.year) o.year = 'numeric'
  return ep.time ? fmt.formatDateTime(d, o) : fmt.formatDate(d, o)
}

// «через 6 дней» / «завтра» / «сегодня» / «3 дня назад».
export function relDays(day, today, locale) {
  const diff = epochOf(day) - epochOf(today)
  try {
    return new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(diff, 'day')
  } catch {
    return ''
  }
}

const weekdayShort = (dow, fmt) =>
  fmt.formatDate(new Date(Date.UTC(2026, 2, 1 + dow)), { weekday: 'short', timeZone: 'UTC' })

// The weekdays of the week in display order (0 = Sunday … 6 = Saturday).
export const weekOrder = (weekStart) =>
  weekStart === 0 ? [0, 1, 2, 3, 4, 5, 6] : [1, 2, 3, 4, 5, 6, 0]

// «Каждую неделю по пн и чт», «Каждые 2 дня, кроме выходных», «По датам: 12 окт., 15 окт.»…
export function ruleSummary(rule, { fmt, weekStart = 1, columns = [], due = null } = {}) {
  if (!rule || !rule.freq) return t('task.due.rep.none')
  const n = Math.max(1, Math.round(rule.interval || 1))
  const parts = []
  if (rule.freq === 'custom') {
    const ds = (rule.dates || []).slice().sort()
    const label = ds
      .slice(0, 3)
      .map((s) => {
        const [y, m, d] = s.split('-').map(Number)
        return fmt.formatDate(new Date(Date.UTC(y, m - 1, d)), {
          day: 'numeric',
          month: 'short',
          timeZone: 'UTC',
        })
      })
      .join(', ')
    parts.push(
      ds.length
        ? t('task.due.rep.dates', { list: label + (ds.length > 3 ? ` +${ds.length - 3}` : '') })
        : t('task.due.rep.datesNone'),
    )
  } else {
    let s =
      n === 1
        ? t(`task.due.rep.every1.${rule.freq}`)
        : t(`task.due.rep.every.${rule.freq}`, { n }, n)
    if (rule.freq === 'weekly' && rule.weekdays?.length) {
      const order = weekOrder(weekStart)
      const days = order.filter((d) => rule.weekdays.includes(d)).map((d) => weekdayShort(d, fmt))
      s = t('task.due.rep.onDays', { every: s, days: fmt.formatList(days) })
    } else if (rule.freq === 'monthly' && (rule.day || due)) {
      s = t('task.due.rep.onDay', { every: s, day: rule.day || due.day.day })
    } else if (rule.freq === 'yearly' && due) {
      const d = new Date(
        Date.UTC(2026, (rule.month || due.day.month + 1) - 1, rule.day || due.day.day),
      )
      s = t('task.due.rep.onDate', {
        every: s,
        date: fmt.formatDate(d, { day: 'numeric', month: 'long', timeZone: 'UTC' }),
      })
    }
    if (
      rule.skip_weekends &&
      (rule.freq === 'daily' || (rule.freq === 'weekly' && !rule.weekdays?.length))
    )
      s = t('task.due.rep.noWeekends', { every: s })
    parts.push(s)
  }
  if (rule.trigger === 'column') {
    const col = columns.find((c) => c.id === rule.trigger_column)
    parts.push(
      col ? t('task.due.rep.onColumn', { name: columnName(col) }) : t('task.due.rep.onColumnUnset'),
    )
  } else if (rule.trigger === 'schedule') parts.push(t('task.due.rep.onSchedule'))
  if (rule.once) parts.push(t('task.due.rep.once'))
  return parts.join(' · ')
}

// The next occurrence's day after `due` (a wall reading), or null.
export function nextDay(rule, due) {
  if (!rule?.freq || !due) return null
  const nx = nextOccurrence(rule, wallDate(due))
  return nx ? coordsOf(nx) : null
}

// Upcoming occurrence keys (YYYY-MM-DD, wall days) for the grid.
export function occurrenceDayKeys(rule, due, n = 24) {
  if (!rule?.freq || !due) return new Set()
  return occurrenceKeys(rule, wallDate(due).getTime(), n)
}

export { withStoredAnchor }

// The repeat in words for a tooltip (the card's due pill, the modal's «Срок»):
// «Повтор: каждую неделю по пн и чт · следующая — пн, 12 окт.».
export function recurrenceHint(rule, dueIso, { fmt, weekStart = 1, columns = [] } = {}) {
  if (!rule?.freq) return ''
  const due = readEndpoint(dueIso, fmt)
  const text = ruleSummary(rule, { fmt, weekStart, columns, due })
  const nx = nextDay(rule, due)
  const tail = nx
    ? ` · ${t('task.due.rep.next', { date: wallLabel({ day: nx, time: null }, fmt) })}`
    : ''
  return t('task.due.rep.hint', { rule: text + tail })
}

// ── due notification ──
export const LEAD_CHIPS = [0, 15, 60, 180, 1440]
export const REPEAT_CHIPS = [0, 60, 180, 1440]
const leadKey = (v) => (v === 0 ? 'leadAt' : `lead${v}`)
const repeatKey = (v) => (v === 0 ? 'repeatOnce' : `repeat${v}`)
export function leadLabel(v) {
  return LEAD_CHIPS.includes(v)
    ? t(`task.due.notify.${leadKey(v)}`)
    : t(`settings.notifications.lead.${v}`)
}
export function repeatLabel(v) {
  return REPEAT_CHIPS.includes(v)
    ? t(`task.due.notify.${repeatKey(v)}`)
    : t(`settings.notifications.repeat.${v}`)
}

// The values a task's due notification resolves to: its own, or the user's
// defaults (Настройки → Уведомления) where it says -1 / 'inherit'.
export function effectiveNotify(notify, defaults) {
  const d = defaults || { due_enabled: true, due_lead_minutes: 60, due_repeat_minutes: 0 }
  const enabled = notify.enabled === 'inherit' ? d.due_enabled !== false : notify.enabled === 'on'
  const lead = notify.lead === -1 || notify.lead == null ? (d.due_lead_minutes ?? 60) : notify.lead
  const repeat =
    notify.repeat === -1 || notify.repeat == null ? (d.due_repeat_minutes ?? 0) : notify.repeat
  return { enabled, lead, repeat }
}

// «По умолчанию — за час», «Включены — за 3 часа, каждый час», «Выключены».
export function notifySummary(notify, defaults) {
  const e = effectiveNotify(notify, defaults)
  const head =
    notify.enabled === 'inherit'
      ? t('task.due.notify.default')
      : notify.enabled === 'on'
        ? t('task.due.notify.on')
        : t('task.due.notify.off')
  if (!e.enabled)
    return notify.enabled === 'off' ? head : `${head} — ${t('task.due.notify.offLow')}`
  const what = [leadLabel(e.lead).toLowerCase()]
  if (e.repeat) what.push(repeatLabel(e.repeat).toLowerCase())
  return `${head} — ${what.join(', ')}`
}
