// Pure formatting core for dates, times, numbers and lists (#2798, stage 2 of #2796).
//
// Everything here is a plain function over an explicit `prefs` object, so it is
// unit-testable without Pinia and safe to import from the theme store itself
// (the store normalizes its own date preset). The store-bound, reactive wrapper
// lives in composables/useFormat.js — components should use that one.
//
// Two rules this module exists to enforce:
//   1. The locale comes from the user's `language` preference, never from the
//      browser: `toLocaleDateString('ru-RU', …)` scattered across components was
//      the reason the UI stayed Russian regardless of the setting.
//   2. Instants are rendered in the user's `timezone` preference, not in the
//      browser's. Before this stage `timezone` only gated notification quiet
//      hours and had no effect on any rendered date.

// Named date presets replace the stored date-fns patterns ('dd.MM.yyyy', …).
// A pattern hard-codes a field order that contradicts the chosen language;
// a preset lets Intl pick the order and gives us one name that maps into both
// worlds — Intl options for text, a date-fns pattern for Naive UI pickers.
// `iso` stays explicit: it is locale-independent by definition.
export const DATE_PRESETS = ['short', 'medium', 'long', 'iso']

// Values written by older clients (and by Android, which still ships the
// pattern list) map onto the nearest preset.
const LEGACY_PATTERNS = {
  'dd.MM.yyyy': 'short',
  'dd/MM/yyyy': 'short',
  'MM/dd/yyyy': 'short',
  'yyyy-MM-dd': 'iso',
}

const PRESET_OPTIONS = {
  short: { day: '2-digit', month: '2-digit', year: 'numeric' },
  medium: { day: 'numeric', month: 'short', year: 'numeric' },
  long: { day: 'numeric', month: 'long', year: 'numeric' },
  // `iso` is rendered by hand (see isoDate) — Intl has no ISO-8601 preset.
  iso: { day: '2-digit', month: '2-digit', year: 'numeric' },
}

export const DEFAULT_PREFS = {
  language: 'ru',
  timezone: '',
  timeFormat: '24h',
  dateFormat: 'short',
  weekStart: 1,
}

export function normalizeDatePreset(value) {
  if (DATE_PRESETS.includes(value)) return value
  return LEGACY_PATTERNS[value] || DEFAULT_PREFS.dateFormat
}

// BCP-47 tag for a UI language. en-GB (not en-US) keeps day-before-month, which
// matches every other locale we ship and the shape users already see.
export function localeTag(language) {
  return language === 'en' ? 'en-GB' : 'ru-RU'
}

// Collator for sorting user data (tags, names, task titles) by the interface
// language (#2800). Two things this fixes at once: sites that hard-coded 'ru'
// kept Cyrillic-first order on an English UI, and sites that passed no locale at
// all sorted by the *environment* language — which differs between a browser and
// CI, so the same list could come out in two orders. `numeric` makes "Sprint 2"
// precede "Sprint 10"; `sensitivity: base` keeps case and accents from splitting
// otherwise equal names.
export function collator(language) {
  return cached('collator', localeTag(language), { numeric: true, sensitivity: 'base' })
}

export function normalizePrefs(prefs = {}) {
  return {
    language: prefs.language || DEFAULT_PREFS.language,
    timezone: prefs.timezone || '',
    timeFormat: prefs.timeFormat === '12h' ? '12h' : '24h',
    dateFormat: normalizeDatePreset(prefs.dateFormat),
    weekStart: prefs.weekStart === 0 ? 0 : (prefs.weekStart ?? DEFAULT_PREFS.weekStart),
  }
}

// ── Intl formatter cache ──────────────────────────────────────────────────────
// Constructing an Intl formatter is expensive relative to formatting with one,
// and these run inside computed properties on every board render.
const formatters = new Map()
function cached(kind, locale, options) {
  const key = `${kind}|${locale}|${JSON.stringify(options)}`
  let f = formatters.get(key)
  if (!f) {
    f =
      kind === 'date'
        ? new Intl.DateTimeFormat(locale, options)
        : kind === 'number'
          ? new Intl.NumberFormat(locale, options)
          : kind === 'list'
            ? new Intl.ListFormat(locale, options)
            : kind === 'collator'
              ? new Intl.Collator(locale, options)
              : new Intl.RelativeTimeFormat(locale, options)
    formatters.set(key, f)
  }
  return f
}

export function toDate(value) {
  if (value === null || value === undefined || value === '') return null
  const d = value instanceof Date ? value : new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}

// Calendar fields of an instant as seen in `timeZone` (undefined = browser tz).
function zonedParts(date, timeZone) {
  const f = cached('date', 'en-US', {
    timeZone: timeZone || undefined,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
  const p = {}
  for (const part of f.formatToParts(date)) {
    if (part.type !== 'literal') p[part.type] = Number(part.value)
  }
  return {
    year: p.year,
    month: p.month - 1,
    day: p.day,
    // Some ICU builds render midnight as hour 24 under hour12:false.
    hour: p.hour % 24,
    minute: p.minute,
    second: p.second,
  }
}

// Days since the epoch for a set of calendar fields — the unit relative labels
// compare in, so a comparison never depends on the offset of either instant.
function epochDay(parts) {
  return Math.round(Date.UTC(parts.year, parts.month, parts.day) / 86400000)
}

// A pure UTC-midnight instant is a date-only value (a GitLab issue/milestone
// date, an all-day due). It has no time of day, so it must be read in UTC in
// every timezone — otherwise a user east or west of UTC sees the previous or
// next day (pitfall 7 of the #2796 plan).
export function isDateOnly(date) {
  return date.getUTCHours() === 0 && date.getUTCMinutes() === 0 && date.getUTCSeconds() === 0
}

// Russian months and weekdays come out of Intl lowercase; English ones already
// carry a capital. Capitalize only when the string starts with a letter, so
// "31 дек." (starts with a digit) is left alone — the old blanket cap() helper
// was a no-op there and a source of confusion.
export function capitalizeFirst(s) {
  if (!s) return s
  const first = s.charAt(0)
  return first.toLowerCase() === first.toUpperCase() ? s : first.toUpperCase() + s.slice(1)
}

const DATE_FIELDS = ['weekday', 'day', 'month', 'year', 'era', 'dateStyle']
const hasDateField = (o) => DATE_FIELDS.some((k) => k in o)
const TIME_FIELDS = ['hour', 'minute', 'second', 'timeStyle']
const hasTimeField = (o) => TIME_FIELDS.some((k) => k in o)

function isoDate(parts) {
  const pad = (n) => String(n).padStart(2, '0')
  return `${parts.year}-${pad(parts.month + 1)}-${pad(parts.day)}`
}

// Builds every formatter bound to one set of preferences.
export function createFormatters(rawPrefs) {
  const prefs = normalizePrefs(rawPrefs)
  const locale = localeTag(prefs.language)
  const tz = prefs.timezone || undefined

  // Which timezone an instant is read in:
  //   • an explicit `timeZone` in the override always wins (`null` means "the
  //     browser's" — used by editing widgets, which build instants from local
  //     calendar fields);
  //   • a date-only value (pure UTC midnight — a GitLab issue/milestone date, an
  //     all-day due) is read in UTC, or every user east/west of UTC would see the
  //     neighbouring day (pitfall 7 of the #2796 plan);
  //   • everything else follows the user's timezone preference.
  const zoneOf = (o, date) => {
    if (o.timeZone !== undefined) return o.timeZone || undefined
    if (date && isDateOnly(date)) return 'UTC'
    return tz
  }

  // `preset` and `timeZone` are ours, not Intl's — strip them before an override
  // reaches a formatter.
  const intlFields = (o) =>
    Object.fromEntries(Object.entries(o).filter(([k]) => k !== 'preset' && k !== 'timeZone'))

  const dateOptions = (o) => {
    const preset = o.preset ? normalizeDatePreset(o.preset) : prefs.dateFormat
    const rest = intlFields(o)
    // An explicit field set replaces the preset rather than merging with it:
    // a caller asking for `{ day, month }` means "no year", not "plus a year".
    return hasDateField(rest) ? rest : { ...PRESET_OPTIONS[preset], ...rest }
  }

  const timeOptions = (o) => {
    const rest = intlFields(o)
    return {
      hour: '2-digit',
      minute: '2-digit',
      hour12: prefs.timeFormat === '12h',
      ...rest,
    }
  }

  function formatDate(value, override = {}) {
    const d = toDate(value)
    if (!d) return ''
    const zone = zoneOf(override, d)
    const preset = override.preset ? normalizeDatePreset(override.preset) : prefs.dateFormat
    const opts = dateOptions(override)
    if (preset === 'iso' && !hasDateField(override)) return isoDate(zonedParts(d, zone))
    return cached('date', locale, { ...opts, timeZone: zone }).format(d)
  }

  function formatTime(value, override = {}) {
    const d = toDate(value)
    if (!d) return ''
    // A clock reading is always about a moment, so no date-only shortcut here.
    const zone = override.timeZone === undefined ? tz : override.timeZone || undefined
    return cached('date', locale, { ...timeOptions(override), timeZone: zone }).format(d)
  }

  function formatDateTime(value, override = {}) {
    const d = toDate(value)
    if (!d) return ''
    const zone = zoneOf(override, d)
    const preset = override.preset ? normalizeDatePreset(override.preset) : prefs.dateFormat
    // ISO dates keep their canonical shape and get the clock appended, since
    // Intl cannot produce "2026-12-31" itself.
    if (preset === 'iso' && !hasDateField(override)) {
      const timeOnly = intlFields(override)
      // Same zone for both halves — otherwise a date-only value would print its
      // UTC day next to a clock read in the user's timezone.
      return `${isoDate(zonedParts(d, zone))} ${formatTime(d, { ...timeOnly, timeZone: zone || null })}`
    }
    const dateOpts = dateOptions(override)
    const timeOpts = hasTimeField(override) ? timeOptions(override) : timeOptions({})
    return cached('date', locale, { ...dateOpts, ...timeOpts, timeZone: zone }).format(d)
  }

  function formatNumber(value, options = {}) {
    if (value === null || value === undefined || value === '' || Number.isNaN(Number(value)))
      return ''
    return cached('number', locale, options).format(Number(value))
  }

  // "название, срок и приоритет" — the conjunction is the locale's, not ours.
  function formatList(items, options = {}) {
    const parts = (items || []).filter((s) => s !== null && s !== undefined && s !== '')
    if (!parts.length) return ''
    return cached('list', locale, { style: 'long', type: 'conjunction', ...options }).format(
      parts.map(String),
    )
  }

  // The user's "today" — a calendar day in their timezone, not the browser's.
  function todayParts(now = new Date()) {
    return zonedParts(now, tz)
  }

  // Relative day label ("сегодня" / "Tomorrow" / a weekday inside the current
  // week), or '' when the date is far enough out that an absolute one reads
  // better. Wording comes from Intl.RelativeTimeFormat instead of the hand-rolled
  // ru/en literals this replaces.
  function formatRelative(value, { now = new Date(), long = false, dateOnly = null } = {}) {
    const d = toDate(value)
    if (!d) return ''
    const only = dateOnly === null ? isDateOnly(d) : dateOnly
    const due = epochDay(zonedParts(d, only ? 'UTC' : tz))
    const today = epochDay(todayParts(now))
    const diff = due - today
    // English labels are capitalized even in pills ("Today"), Russian ones only
    // outside them, where they sit next to capitalized siblings.
    const cap = (s) => (long || prefs.language === 'en' ? capitalizeFirst(s) : s)
    if (Math.abs(diff) <= 1) {
      return cap(cached('relative', locale, { numeric: 'auto' }).format(diff, 'day'))
    }
    const firstDay = prefs.weekStart === 0 ? 0 : 1
    // First day of the week a given epoch-day belongs to: two dates in the same
    // week share it, so "inside this week" is one comparison.
    const weekOf = (day) => day - ((new Date(day * 86400000).getUTCDay() - firstDay + 7) % 7)
    if (Math.abs(diff) <= 6 && weekOf(due) === weekOf(today)) {
      // Short lowercase weekday in pills ("пн"), full capitalized elsewhere.
      const wd = cached('date', locale, {
        weekday: long ? 'long' : 'short',
        timeZone: 'UTC',
      }).format(new Date(due * 86400000))
      return long ? capitalizeFirst(wd) : wd
    }
    return ''
  }

  // Task due dates: relative when near, otherwise day + month (+ year when it
  // isn't the current one). The clock is appended only when the value carries a
  // real time of day — date-only dues stay terse.
  function formatDue(value, { long = false, now = new Date() } = {}) {
    const d = toDate(value)
    if (!d) return ''
    const only = isDateOnly(d)
    const zone = only ? 'UTC' : tz
    const parts = zonedParts(d, zone)
    const time = !only && (parts.hour || parts.minute) ? formatTime(d, { timeZone: zone }) : ''
    const rel = formatRelative(d, { now, long, dateOnly: only })
    if (rel) return time ? `${rel}, ${time}` : rel
    const o = { day: '2-digit', month: 'short', timeZone: zone }
    if (parts.year !== todayParts(now).year) o.year = 'numeric'
    const label = formatDate(d, o)
    return time ? `${label}, ${time}` : label
  }

  // Calendar fields (year, month 0-based, day, hour, minute, second) of a value
  // as the user sees them — the only correct way to ask "is this the current
  // year?" once rendering happens in a chosen timezone rather than the browser's.
  function parts(value, override = {}) {
    const d = toDate(value)
    return d ? zonedParts(d, zoneOf(override, d)) : null
  }

  return {
    prefs,
    locale,
    timeZone: tz,
    parts,
    today: (now = new Date()) => todayParts(now),
    formatDate,
    formatTime,
    formatDateTime,
    formatNumber,
    formatList,
    formatRelative,
    formatDue,
    isDateOnly,
  }
}

// Formatters for the default preferences (ru, browser timezone, 24h, short).
// The fallback for pure helpers that render a date but are called from outside a
// component — they take a formatter set as an argument and land here when a
// caller has none to hand (tests, mostly). Built once, on first use.
let fallback = null
export function defaultFormatters() {
  if (!fallback) fallback = createFormatters(DEFAULT_PREFS)
  return fallback
}

// ── date-fns patterns for Naive UI pickers ────────────────────────────────────
// Naive UI formats and parses with date-fns, so a preset has to be translated
// into a pattern. Deriving it from Intl.formatToParts keeps the picker and the
// rendered text in the same field order for every locale, with no table to
// maintain.
const PATTERN_REFERENCE = new Date(Date.UTC(2026, 11, 31, 12, 0, 0))

function quoteLiteral(text) {
  // date-fns treats bare letters as tokens; ru long dates carry " г." and other
  // locales carry words, so any literal with a letter has to be quoted.
  if (!/\p{L}/u.test(text)) return text
  return `'${text.replace(/'/g, "''")}'`
}

export function datePattern(prefs) {
  const p = normalizePrefs(prefs)
  if (p.dateFormat === 'iso') return 'yyyy-MM-dd'
  const opts = { ...PRESET_OPTIONS[p.dateFormat], timeZone: 'UTC' }
  const parts = cached('date', localeTag(p.language), opts).formatToParts(PATTERN_REFERENCE)
  // Field ORDER and literals come from Intl; the token WIDTH comes from the
  // preset's own options. Reading the width off the reference rendering would
  // pin it to that date — 31 December would make every day two-digit, and the
  // picker would then print "05 дек." where the text says "5 дек.".
  return parts
    .map((part) => {
      switch (part.type) {
        case 'day':
          return opts.day === '2-digit' ? 'dd' : 'd'
        case 'month':
          if (!/^\d+$/.test(part.value)) return opts.month === 'long' ? 'MMMM' : 'MMM'
          return opts.month === '2-digit' ? 'MM' : 'M'
        case 'year':
          return 'yyyy'
        case 'literal':
          return quoteLiteral(part.value)
        default:
          return ''
      }
    })
    .join('')
}

export function timePattern(prefs) {
  return normalizePrefs(prefs).timeFormat === '12h' ? 'hh:mm a' : 'HH:mm'
}

export function dateTimePattern(prefs) {
  return `${datePattern(prefs)} ${timePattern(prefs)}`
}

// A sample rendering of each preset, for the settings picker ("31.12.2026").
export function datePresetSamples(prefs, sample = PATTERN_REFERENCE) {
  const f = createFormatters(prefs)
  return DATE_PRESETS.map((preset) => ({
    value: preset,
    label: f.formatDate(sample, { preset, timeZone: 'UTC' }),
  }))
}
