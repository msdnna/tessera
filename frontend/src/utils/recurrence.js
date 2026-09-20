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

// The interval row is rendered as three pieces — [everyLabel] [number box]
// [unitLabel] — so both text halves are declined for the same count.
const intervalCount = (n) => Math.round(Number(n) || 0)

const hasIntervalRow = (freq) => FREQ_VALUES.includes(freq) && freq !== '' && freq !== 'custom'

// "каждые 3 недели" — the unit half, declined for the count. Russian needs three
// forms and the accusative ("каждые 2 неделИ"), which ICU pluralisation in the
// catalogue handles; English gets its own two.
export function unitLabel(freq, n) {
  if (!hasIntervalRow(freq)) return ''
  return i18n.global.t(`task.recur.unit.${freq}`, intervalCount(n))
}

// "каждый 1 день" — the half *in front of* the number. It used to be one frozen
// word ("каждые"), which reads wrong for every count ending in 1: the interval
// box opens at 1 the moment a frequency is picked, so «каждые 1 день» was the
// first form a Russian user ever saw. The word agrees with the unit in number
// *and* in gender — неделя is feminine ("каждУЮ неделю") where день/месяц/год
// are masculine — hence one message per frequency rather than one shared. The
// English branches are deliberately identical: the word does not inflect there,
// and a flat message opposite a pluralised `ru` one is what cx-i18n forbids.
export function everyLabel(freq, n) {
  if (!hasIntervalRow(freq)) return ''
  return i18n.global.t(`task.recur.every.${freq}`, intervalCount(n))
}

// withStoredAnchor mirrors the backend's `recurrenceToStore` (handlers/tasks.go).
//
// A monthly/yearly rule carries a server-managed day-of-month anchor (`day`, plus
// `month` for yearly): a short month temporarily moves the due date, and without
// the anchor every later occurrence would inherit the clamped day and the series
// would walk backwards — a task repeating on the 31st would become a 28th task the
// first time it passed through February. The editor's controls never hold that
// anchor, so a rule rebuilt from them has to take it back from the stored one, on
// the same terms the server uses: same frequency, due date untouched. In every
// other case the server re-derives the anchor from the due date, which is exactly
// what `nextOccurrence` falls back to on its own.
//
// `dueChanged` means the due *day* moved, not the instant: the editor edits the
// calendar day and the time of day with separate controls, and the server keeps
// the anchor across a clock-only edit (`sameRuleDay`, handlers/tasks.go).
export function withStoredAnchor(rule, stored, dueChanged) {
  if (!rule || (rule.freq !== 'monthly' && rule.freq !== 'yearly')) return rule
  if (dueChanged || !stored || stored.freq !== rule.freq) return rule
  const out = { ...rule }
  if (stored.day) out.day = stored.day
  if (rule.freq === 'yearly' && stored.month) out.month = stored.month
  return out
}

// triggerColumnMissing reports a column-triggered rule that can never fire.
//
// The server advances such a rule in exactly one place — a move whose target
// column id equals `trigger_column` verbatim (handlers/task_ops.go) — so an
// unset column matches nothing and neither does one that has since been deleted
// from the board. Both states are reachable without any error: the trigger-column
// select opens on its placeholder and picking the event alone already commits the
// rule, and columns outlive nothing that points at them.
//
// `columnIds` is the board's current column ids. An empty/absent list means the
// caller cannot see the board (the editor's `columns` prop defaults to `[]`), and
// "I don't know the columns" is not "the column is gone" — only an unnamed column
// is knowable then.
export function triggerColumnMissing(rule, columnIds) {
  if (!rule || !rule.freq || rule.trigger !== 'column') return false
  if (!rule.trigger_column) return true
  if (!columnIds || !columnIds.length) return false
  return !columnIds.includes(rule.trigger_column)
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

// Step whole LOCAL calendar days, keeping the clock — the mirror of the backend's
// `AddDate(0, 0, n)`. Not `+ n * 86400000`: a day with a clock change is not 24
// hours long, and every field of a rule is a calendar statement (see timeAxis.js
// `msAtDayOffset`, which learnt this the same way).
function addDays(d, n) {
  return new Date(
    d.getFullYear(),
    d.getMonth(),
    d.getDate() + n,
    d.getHours(),
    d.getMinutes(),
    d.getSeconds(),
    0,
  )
}

// Push a Saturday/Sunday occurrence forward to Monday. Milliseconds used to do the
// pushing, and in the week the clocks go back (25-hour Sunday) +48h from Saturday
// lands at 23:00 on SUNDAY and +24h from Sunday stays on Sunday — the calendar
// highlighted the very weekend the rule excludes, on a day the server (stepping
// calendar days) never fires on.
function applySkip(d, skipWeekends) {
  if (!skipWeekends) return d
  const wd = d.getDay() // 0=Sun..6=Sat
  if (wd === 6) return addDays(d, 2)
  if (wd === 0) return addDays(d, 1)
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
//
// A custom rule walks its date list through the same loop as every other freq,
// and for the same reason: `once` is offered for custom rules too, and the server
// then fires the first date past the due one and drops the rule. Handing the whole
// picked list back here promised firings that were never going to happen — and lit
// up the dates the series had already walked past on top of that (#2932). The
// picked list is the *editor's* business to show; this function only answers
// "which days will fire".
export function occurrenceKeys(rule, fromTs, n = 24) {
  const keys = new Set()
  if (!fromTs || !rule || !rule.freq) return keys
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
