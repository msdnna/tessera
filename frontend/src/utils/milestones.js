// Shared milestone helpers (used by the card chip, the task picker and the manager).
import { defaultFormatters, isDateOnly } from '@/utils/format'
import { i18n } from '@/i18n'

// Resolved per call, never cached at module level: this module is imported
// outside any setup context, so a table built at import time would freeze on the
// language of the first render (#2799).
const t = (key, ...rest) => i18n.global.t(key, ...rest)

// ── GitLab stages (#3010) ──
// A stage linked to a GitLab milestone is edited in Tessera only while its binding
// writes to GitLab (gl_writable: binding on, write-back on); otherwise it stays
// read-only, as GitLab would undo the edit on the next pull.
export const isGitlabStage = (m) => !!m?.gl_global_id
export const stageEditable = (m) => !!m && (!isGitlabStage(m) || !!m.gl_writable)
// The last write to GitLab failed: the edit is kept here, marked «not synced».
export const stageUnsynced = (m) => isGitlabStage(m) && !!m.gl_push_error
// Changed both here and in GitLab — waits in the conflicts window.
export const stageConflict = (m) => isGitlabStage(m) && !!m.gl_conflict
// What the «not synced» mark names: the goals alone (the mockup's case), or the stage.
export function unsyncedKey(m) {
  const f = m?.gl_push_fields || []
  return f.length === 1 && f[0] === 'description'
    ? 'milestones.gitlab.unsyncedGoals'
    : 'milestones.gitlab.unsynced'
}

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

/**
 * Which milestone a card typed on the board should be born with (#2932).
 *
 * `?milestone=` narrows the board SERVER-side (backend handlers/tasks.go →
 * ListBoardTasksWithMeta: `t.milestone_id = @milestone_id`), so on a sprint-scoped
 * board a card created without that milestone is not in the list the reload right
 * after it returns: the card the reader just typed into the column disappears, with
 * no error and no clue that it exists at all — it is sitting in the backlog. The
 * milestone GROUPING has always stamped a new card with its lane's milestone; the
 * sprint scope is the same promise made by the address bar instead of by a lane, and
 * it was the only one not kept.
 *
 * @param {Object} o
 * @param {string} [o.groupMode] board grouping. `'milestone'` means the lane the
 *        reader typed into decides — including its «Без этапа» lane, where saying
 *        "no milestone" out loud must beat the scope. Pass it only where lanes
 *        exist: the matrix quick-add has none, so there the scope alone decides.
 * @param {Object} [o.lane] that lane (`dcol`), carrying `milestone`
 * @param {string|null|undefined} [o.scopeId] the resolved sprint scope: an id, `null`
 *        for the backlog scope (a card with no milestone is exactly what it lists),
 *        `''` for a scope that cannot be resolved yet, `undefined` when unscoped
 * @returns {string|null} milestone id to stamp on the new card, or null for none
 */
export function newTaskMilestoneId({ groupMode, lane, scopeId } = {}) {
  if (groupMode === 'milestone') return lane?.milestone?.id || null
  return scopeId || null
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

// ── stage state, days left, progress (#2971) ──────────────────────

// A stage date as its UTC calendar day (stage dates are date-only), at midnight
// UTC — comparable with `todayUTC()`.
export function stageDay(iso) {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  return Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate())
}

// Today as the same kind of number, read on the reader's own calendar: «today»
// is the day the reader is living, whatever the UTC clock says.
export function todayUTC(now = new Date()) {
  return Date.UTC(now.getFullYear(), now.getMonth(), now.getDate())
}

const DAY = 86400000

// Two states are stored, as in GitLab (active / closed); «Запланирован» is
// derived — an active stage whose start is still ahead (decision in #2971).
export function stageState(m, today = todayUTC()) {
  if (!m) return 'active'
  if (m.state === 'closed') return 'closed'
  const start = stageDay(m.start_date)
  if (start != null && start > today) return 'planned'
  return 'active'
}

// Whole days until the due date (0 on the day itself, negative past it), or
// null without one.
export function daysLeft(m, today = todayUTC()) {
  const due = stageDay(m?.due_date)
  if (due == null) return null
  return Math.round((due - today) / DAY)
}

// Done / total of a stage row (the rollups on GET /projects/:id/milestones).
export function stageProgress(m) {
  const total = Number(m?.task_count) || 0
  const done = Number(m?.done_count) || 0
  return { done, total, pct: total ? Math.round((done / total) * 100) : 0 }
}

// Stage rows in the order the window, the selector and the sidebar list them:
// active first (by due date, the nearest first), then planned (by start), then
// closed (the most recently closed first).
export function sortStages(list, today = todayUTC()) {
  const rank = { active: 0, planned: 1, closed: 2 }
  const key = (m) => {
    const s = stageState(m, today)
    if (s === 'closed') return -(stageDay(m.closed_at) ?? stageDay(m.due_date) ?? 0)
    if (s === 'planned') return stageDay(m.start_date) ?? 0
    return stageDay(m.due_date) ?? Number.MAX_SAFE_INTEGER
  }
  return [...(list || [])].sort((a, b) => {
    const ra = rank[stageState(a, today)]
    const rb = rank[stageState(b, today)]
    if (ra !== rb) return ra - rb
    return key(a) - key(b)
  })
}

// The stage page's address: /project/<project>/stage/<stage> — slugs, like the
// board's (no UUIDs in URLs).
export function stagePath(projectSlug, m) {
  return `/project/${projectSlug}/stage/${m.slug || m.id}`
}

// The compact stage window the v2 surfaces print (mockup: «12 – 26 окт.»,
// «28 сент. – 11 окт.», «до 30 окт.»): the month once when both ends share it,
// the year only when it is not the current one.
export function milestoneRangeShort(m, fmt = defaultFormatters(), now = new Date()) {
  if (!m) return ''
  const s = m.start_date ? new Date(m.start_date) : null
  const d = m.due_date ? new Date(m.due_date) : null
  const year = (x) => (isDateOnly(x) ? x.getUTCFullYear() : x.getFullYear())
  const month = (x) => (isDateOnly(x) ? x.getUTCMonth() : x.getMonth())
  const thisYear = now.getFullYear()
  const opts = (x) => ({
    day: 'numeric',
    month: 'short',
    ...(year(x) !== thisYear ? { year: 'numeric' } : {}),
  })
  const f = (x, o) => fmt.formatDate(x.toISOString(), o)
  if (s && d) {
    if (year(s) === year(d) && month(s) === month(d)) {
      return `${f(s, { day: 'numeric' })} – ${f(d, opts(d))}`
    }
    return `${f(s, { day: 'numeric', month: 'short' })} – ${f(d, opts(d))}`
  }
  if (d) return t('milestones.range.until', { date: f(d, opts(d)) })
  if (s) return t('milestones.range.from', { date: f(s, opts(s)) })
  return ''
}
