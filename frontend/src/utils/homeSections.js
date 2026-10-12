// The start screen's task lists and counters (v2, #2960), kept pure so the
// sorting rules are testable without mounting the screen.
//
// «Мои задачи» are the open tasks assigned to me. Sections follow the due date
// in the browser's calendar: overdue → today → this week (to Sunday) → later →
// no due date. The summary pills are counted here too, per user — the
// workspace summary endpoint counts the whole workspace.
import { isReviewColumn } from '@/utils/columnStatus'
import { epochDay } from '@/utils/format'

const DAY_MS = 86400000
const startOfDay = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()

// The local calendar day a due date falls on. A date-only due is stored as UTC
// midnight; reading it in local time would slide it to the previous day west of
// UTC, so such a value is taken by its UTC calendar date.
export function dueDay(iso) {
  if (!iso) return null
  const d = new Date(iso)
  if (d.getUTCHours() === 0 && d.getUTCMinutes() === 0 && d.getUTCSeconds() === 0)
    return new Date(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()).getTime()
  return startOfDay(d)
}

// Monday-based week: the days left to Sunday, today included.
function weekEnd(today, firstDayOfWeek = 1) {
  const dow = new Date(today).getDay()
  const last = (firstDayOfWeek + 6) % 7
  const ahead = (last - dow + 7) % 7
  return today + ahead * DAY_MS
}

export const SECTION_KEYS = ['overdue', 'today', 'week', 'later', 'nodate']

export function mineOf(tasks, meId) {
  return (tasks || []).filter((t) => !t.completed_at && (t.assignee_ids || []).includes(meId))
}

// `fmt` (useFormat's formatters) puts the day edges in the reader's calendar —
// the profile timezone over the browser's, a date-only due read in UTC — exactly
// as isTaskOverdue does, so a row filed under «Просрочено» is the row whose due
// pill is red (#2932). Without it the browser's local day is used.
export function buildSections(mine, { now = new Date(), firstDayOfWeek = 1, fmt = null } = {}) {
  let today, end, dayOf
  if (fmt) {
    today = epochDay(fmt.today(now))
    // Epoch day 0 (1 Jan 1970) was a Thursday: getDay() 4.
    const dow = (today + 4) % 7
    end = today + ((((firstDayOfWeek + 6) % 7) - dow + 7) % 7)
    dayOf = (iso) => (iso ? epochDay(fmt.parts(iso)) : null)
  } else {
    today = startOfDay(now)
    end = weekEnd(today, firstDayOfWeek)
    dayOf = dueDay
  }
  const out = Object.fromEntries(SECTION_KEYS.map((k) => [k, []]))
  for (const t of mine) {
    const d = dayOf(t.due_date)
    if (d == null) out.nodate.push(t)
    else if (d < today) out.overdue.push(t)
    else if (d === today) out.today.push(t)
    else if (d <= end) out.week.push(t)
    else out.later.push(t)
  }
  const byDue = (a, b) => (a.due_date || '').localeCompare(b.due_date || '')
  for (const k of SECTION_KEYS) if (k !== 'nodate') out[k].sort(byDue)
  return out
}

// Waiting for my review: an open task I created that sits in a review column.
export function reviewCount(tasks, meId) {
  return (tasks || []).filter(
    (t) =>
      !t.completed_at &&
      t.created_by === meId &&
      (t.column_name_key === 'review' || isReviewColumn(t.column_name)),
  ).length
}

// «Неделя»: per day of this week, my tasks due that day — done and still open.
export function weekBars(tasks, meId, { now = new Date(), firstDayOfWeek = 1 } = {}) {
  const today = startOfDay(now)
  const dow = new Date(today).getDay()
  const start = today - ((dow - firstDayOfWeek + 7) % 7) * DAY_MS
  const days = Array.from({ length: 7 }, (_, i) => ({
    ms: start + i * DAY_MS,
    done: 0,
    open: 0,
    today: start + i * DAY_MS === today,
  }))
  for (const t of tasks || []) {
    if (!(t.assignee_ids || []).includes(meId)) continue
    const d = dueDay(t.due_date)
    if (d == null) continue
    const i = Math.round((d - start) / DAY_MS)
    if (i < 0 || i > 6) continue
    if (t.completed_at) days[i].done++
    else days[i].open++
  }
  const done = days.reduce((s, d) => s + d.done, 0)
  const total = days.reduce((s, d) => s + d.done + d.open, 0)
  return { days, done, total }
}
