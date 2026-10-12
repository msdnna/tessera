// The «Календарь» view (v2, #3019): the pure half — the month and week grids,
// the day a due falls on, the due a drop on a day produces, how many pills a
// cell has room for. Kept out of BoardCalendarView so it stays unit-testable.
//
// Days here are CALENDAR COORDINATES — { year, month (0-based), day } — not
// instants. A due is turned into one through the formatter set (`fmt.parts`),
// which already knows the two rules (#2932): a timed due is read in the user's
// profile timezone, a pure UTC midnight (a date-only due) in UTC everywhere.
import { epochDay } from '@/utils/format'

export const DAY_MS = 86400000

export const dayKey = (p) => (p ? `${p.year}-${p.month}-${p.day}` : '')
export const sameDay = (a, b) =>
  !!a && !!b && a.year === b.year && a.month === b.month && a.day === b.day

// Calendar arithmetic on coordinates (local Date only as a day counter).
export function addDays(p, n) {
  const d = new Date(p.year, p.month, p.day + n)
  return { year: d.getFullYear(), month: d.getMonth(), day: d.getDate() }
}
export function addMonths(p, n) {
  const d = new Date(p.year, p.month + n, 1)
  const last = new Date(d.getFullYear(), d.getMonth() + 1, 0).getDate()
  return { year: d.getFullYear(), month: d.getMonth(), day: Math.min(p.day, last) }
}
export const dowOf = (p) => new Date(p.year, p.month, p.day).getDay()
const isWeekend = (p) => {
  const w = dowOf(p)
  return w === 0 || w === 6
}

// The first day of the week holding `p`, the week starting on `firstDow`
// (a getDay() value: 1 = Monday, 0 = Sunday).
export function weekStart(p, firstDow) {
  return addDays(p, -((dowOf(p) - firstDow + 7) % 7))
}

// The month grid around `p`: whole weeks from the one holding the 1st to the
// one holding the last day — 4 to 6 rows, as the month needs.
export function monthGrid(p, firstDow) {
  const first = { year: p.year, month: p.month, day: 1 }
  const lastDay = new Date(p.year, p.month + 1, 0).getDate()
  const start = weekStart(first, firstDow)
  const end = weekStart({ year: p.year, month: p.month, day: lastDay }, firstDow)
  const rows = Math.round((epochDay(end) - epochDay(start)) / 7) + 1
  const cells = []
  for (let i = 0; i < rows * 7; i++) {
    const d = addDays(start, i)
    cells.push({ ...d, key: dayKey(d), inMonth: d.month === p.month, weekend: isWeekend(d) })
  }
  return { cells, rows }
}

// The seven days of the week holding `p`.
export function weekGrid(p, firstDow) {
  const start = weekStart(p, firstDow)
  return Array.from({ length: 7 }, (_, i) => {
    const d = addDays(start, i)
    return { ...d, key: dayKey(d), inMonth: true, weekend: isWeekend(d) }
  })
}

// Is the instant a pure UTC midnight — a date-only due?
export function isDateOnlyMs(ms) {
  const d = new Date(ms)
  return d.getUTCHours() === 0 && d.getUTCMinutes() === 0 && d.getUTCSeconds() === 0
}

// The date-only value of a calendar day: its UTC midnight, the way the due
// editor and GitLab store an all-day due — it reads as that day in every zone.
export const dateOnlyISO = (p) => new Date(Date.UTC(p.year, p.month, p.day)).toISOString()

// One instant moved by whole calendar days, keeping what it is: a date-only
// value stays a UTC midnight; a timed one keeps its wall-clock time in the
// user's zone (`fmt.parts`), across a clock change too.
export function shiftInstant(iso, days, fmt) {
  const ms = Date.parse(iso)
  if (Number.isNaN(ms) || !days) return iso
  const moved = ms + days * DAY_MS
  if (isDateOnlyMs(ms)) return new Date(moved).toISOString()
  const a = fmt.parts(new Date(ms))
  const b = fmt.parts(new Date(moved))
  let delta = (a.hour - b.hour) * 60 + (a.minute - b.minute)
  if (delta > 720) delta -= 1440
  if (delta < -720) delta += 1440
  return new Date(moved + delta * 60000).toISOString()
}

// What a drop of `task` on day `to` writes (#3019). A task with a due moves by
// the days between its due's day and `to` — its start along with it, so the
// span stays; a task without one gets `to` as a date-only due.
export function movedDates(task, to, fmt) {
  if (!task.due_date) return { due_date: dateOnlyISO(to), start_date: task.start_date || null }
  const from = fmt.parts(task.due_date)
  const days = epochDay(to) - epochDay(from)
  if (!days) return null
  return {
    due_date: shiftInstant(task.due_date, days, fmt),
    start_date: task.start_date ? shiftInstant(task.start_date, days, fmt) : null,
  }
}

// How many pills a month cell shows (#3019): every task when they fit, else
// as many as fit with the «+N ещё» line under them — never more than three.
//   cellH — the cell's height; PAD 6+6, the day row 22, a pill 22 + gap 3,
//   the «+N» line 20.
export const PILL_MAX = 3
export function pillsShown(count, cellH) {
  const avail = cellH - 12 - 22
  const fit = Math.max(0, Math.floor(avail / 25))
  if (count <= Math.min(PILL_MAX, fit)) return count
  return Math.max(0, Math.min(PILL_MAX, Math.floor((avail - 20) / 25)))
}

// The day's tasks in the order a day reads: timed dues first, by their time,
// then the all-day ones in the order they came in.
export function dayOrder(tasks) {
  const timed = []
  const allDay = []
  for (const t of tasks) {
    const ms = Date.parse(t.due_date)
    if (!Number.isNaN(ms) && !isDateOnlyMs(ms)) timed.push([ms, t])
    else allDay.push(t)
  }
  timed.sort((a, b) => a[0] - b[0])
  return [...timed.map((x) => x[1]), ...allDay]
}
