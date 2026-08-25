// Client-side mirror of the backend `internal/recur` advance logic, used to
// preview a recurrence on the calendar (highlight upcoming occurrences) and to
// label rules. The backend remains the source of truth; this only previews.
import { i18n } from '@/i18n'

// The option lists are functions, not arrays: a module-level array would be
// built once at import and keep the language of the first render forever
// (pitfall 1 of #2799). `values` stays a constant — it is the wire format.
export const FREQ_VALUES = ['', 'daily', 'weekly', 'monthly', 'yearly', 'custom']
export const TRIGGER_VALUES = ['complete', 'column', 'schedule']

export function freqOptions() {
  return FREQ_VALUES.map((value) => ({
    label: i18n.global.t(`task.recur.freq.${value || 'none'}`),
    value,
  }))
}

export function triggerOptions() {
  return TRIGGER_VALUES.map((value) => ({
    label: i18n.global.t(`task.recur.trigger.${value}`),
    value,
  }))
}

// "каждые 3 недели" — the unit half, declined for the count. Russian needs three
// forms and the accusative ("каждые 2 неделИ"), which ICU pluralisation in the
// catalogue handles; English gets its own two.
export function unitLabel(freq, n) {
  if (!FREQ_VALUES.includes(freq) || freq === '' || freq === 'custom') return ''
  return i18n.global.t(`task.recur.unit.${freq}`, Math.round(Number(n) || 0))
}

const dayKey = (d) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

const daysInMonth = (y, m) => new Date(y, m + 1, 0).getDate()

// Build a Date on y/m/d (d clamped to the month length), carrying base's time.
function dateOn(base, y, m, d) {
  const last = daysInMonth(y, m)
  const day = Math.min(d, last)
  return new Date(y, m, day, base.getHours(), base.getMinutes(), base.getSeconds(), 0)
}

function applySkip(d, skipWeekends) {
  if (!skipWeekends) return d
  const wd = d.getDay() // 0=Sun..6=Sat
  if (wd === 6) return new Date(d.getTime() + 2 * 86400000)
  if (wd === 0) return new Date(d.getTime() + 86400000)
  return d
}

function mondayOf(d) {
  const offset = (d.getDay() + 6) % 7 // Mon=0..Sun=6
  const m = new Date(d)
  m.setDate(d.getDate() - offset)
  return m
}

function nextWeekly(from, rule) {
  const days = (rule.weekdays || []).slice().sort((a, b) => a - b)
  if (!days.length) {
    const n = new Date(from)
    n.setDate(from.getDate() + 7 * (rule.interval || 1))
    return n
  }
  const set = new Set(days)
  const monday = mondayOf(from)
  for (let i = 1; i < 7; i++) {
    const c = new Date(from)
    c.setDate(from.getDate() + i)
    if (
      c < new Date(monday.getFullYear(), monday.getMonth(), monday.getDate() + 7) &&
      set.has(c.getDay())
    ) {
      return c
    }
  }
  const target = new Date(monday)
  target.setDate(monday.getDate() + 7 * (rule.interval || 1))
  for (let i = 0; i < 7; i++) {
    const c = new Date(target)
    c.setDate(target.getDate() + i)
    if (set.has(c.getDay())) return c
  }
  const n = new Date(from)
  n.setDate(from.getDate() + 7 * (rule.interval || 1))
  return n
}

// nextOccurrence returns the Date after `from` per the rule, or null when the
// recurrence has ended (custom dates exhausted / no rule).
export function nextOccurrence(rule, from) {
  if (!rule || !rule.freq) return null
  const iv = Math.max(1, rule.interval || 1)
  switch (rule.freq) {
    case 'daily': {
      const n = new Date(from)
      n.setDate(from.getDate() + iv)
      return applySkip(n, rule.skip_weekends)
    }
    case 'weekly':
      return applySkip(nextWeekly(from, { ...rule, interval: iv }), rule.skip_weekends)
    case 'monthly': {
      const total = from.getMonth() + iv
      const y = from.getFullYear() + Math.floor(total / 12)
      const m = ((total % 12) + 12) % 12
      return dateOn(from, y, m, rule.day || from.getDate())
    }
    case 'yearly': {
      const y = from.getFullYear() + iv
      const m = rule.month ? rule.month - 1 : from.getMonth()
      return dateOn(from, y, m, rule.day || from.getDate())
    }
    case 'custom': {
      const key = dayKey(from)
      for (const s of (rule.dates || []).slice().sort()) {
        if (s > key) {
          const [yy, mm, dd] = s.split('-').map(Number)
          return new Date(yy, mm - 1, dd, from.getHours(), from.getMinutes(), from.getSeconds(), 0)
        }
      }
      return null
    }
    default:
      return null
  }
}

// occurrences returns up to `n` date keys (YYYY-MM-DD) of upcoming occurrences,
// starting at `fromTs` itself — for highlighting the calendar. A one-off rule
// (`once`) shows just the current due + the single next occurrence.
export function occurrenceKeys(rule, fromTs, n = 24) {
  const keys = new Set()
  if (!fromTs || !rule || !rule.freq) return keys
  if (rule.freq === 'custom') {
    for (const s of rule.dates || []) keys.add(s)
    return keys
  }
  const limit = rule.once ? 1 : n
  let cur = new Date(fromTs)
  keys.add(dayKey(cur))
  for (let i = 0; i < limit; i++) {
    const nx = nextOccurrence(rule, cur)
    if (!nx) break
    keys.add(dayKey(nx))
    cur = nx
  }
  return keys
}
