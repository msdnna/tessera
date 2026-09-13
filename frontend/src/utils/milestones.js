// Shared milestone helpers (used by the card chip, the task picker and the manager).
import { defaultFormatters, isDateOnly } from '@/utils/format'
import { i18n } from '@/i18n'

// Resolved per call, never cached at module level: this module is imported
// outside any setup context, so a table built at import time would freeze on the
// language of the first render (#2799).
const t = (key, ...rest) => i18n.global.t(key, ...rest)

// The reserved ?milestone= value for "tasks without a milestone".
export const BACKLOG_SCOPE = 'backlog'

// milestoneKey is what goes into ?milestone= — the slug, so sprint links read
// ?milestone=sprint-1. Falls back to the UUID for rows the startup backfill has
// not reached yet.
export function milestoneKey(m) {
  if (!m) return BACKLOG_SCOPE
  return m.slug || m.id
}

// matchesScope tells whether a ?milestone= value points at this milestone. Both
// forms count: links shared before slugs existed carry the UUID.
export function matchesScope(m, scope) {
  if (!scope) return false
  if (!m) return scope === BACKLOG_SCOPE
  return scope === m.slug || scope === m.id
}

// toDateOnlyISO turns an n-date-picker value into the date-only instant the rest
// of the stack expects: pure UTC midnight of the chosen calendar day.
//
// The picker hands back an epoch-ms at LOCAL midnight, so the day has to be
// re-read from the local calendar fields — `new Date(ms).toISOString()` keeps the
// instant and drops the day, which is a different value everywhere but UTC.
// Storing that moment breaks the date-only contract twice over: format.js
// (isDateOnly/zoneOf) then reads the milestone in the viewer's timezone, so two
// people see two different days; and the GitLab push formats it with
// `t.UTC().Format("2006-01-02")` (backend dateStr), so a milestone set to 15.07
// east of UTC arrives in GitLab as the 14th — after which the next pull writes
// that shifted day back over the native row for everyone.
//
// Milestones that come FROM GitLab are already date-only (parseDate of
// "2006-01-02"); this is what puts native ones on the same footing.
export function toDateOnlyISO(value) {
  if (value === null || value === undefined || value === '') return null
  const d = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(d.getTime())) return null
  return new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate())).toISOString()
}

// pickerValue is the inverse: a stored milestone date as the local-midnight
// epoch-ms n-date-picker works in.
//
// A date-only value is read in UTC — that is the whole point of it, and west of
// UTC its instant lands on the previous local day, which would show the wrong
// date in the picker and then save it back shifted. Anything else is a milestone
// written before this became date-only; it keeps its plain reading so the picker
// agrees with the label the list is already showing next to it.
export function pickerValue(iso) {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  if (!isDateOnly(d)) return d.getTime()
  return new Date(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()).getTime()
}

// Milestone dates are date-only, so formatDate reads them in UTC and they stay on
// the same calendar day in every timezone (#2798).
function fmtDate(iso, fmt) {
  if (!iso) return ''
  return fmt.formatDate(iso, { day: 'numeric', month: 'short', year: 'numeric' })
}

// Human-readable start–due window for a milestone (either side may be missing).
// `fmt` is a formatter set from useFormat(); without one the default preferences
// are used, which is what non-component callers (tests) get.
export function milestoneRange(m, fmt = defaultFormatters()) {
  if (!m) return ''
  const s = fmtDate(m.start_date, fmt)
  const d = fmtDate(m.due_date, fmt)
  if (s && d) return `${s} – ${d}`
  if (d) return t('milestones.range.until', { date: d })
  if (s) return t('milestones.range.from', { date: s })
  return ''
}
