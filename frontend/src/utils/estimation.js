// Task-estimation helpers. The backend stores a single canonical `estimate`
// number per task whose *unit* lives in a two-level config (project override →
// workspace default → the built-in default below). This module resolves that
// config and does all input parsing / output formatting client-side:
//   • time   → canonical minutes; "3d 4h" / "1w" / "90m" parse via the working
//              day/week, and output compresses back ("3 дня" = 24 working hours);
//   • points → the point number on a scale (Fibonacci / T-shirt / linear);
//   • custom → a count of a named unit.
// Mirrors the backend canonicalisation in handlers/estimation.go.
import { defaultFormatters } from '@/utils/format'
import { i18n } from '@/i18n'

// Localised text is produced per call (never cached at module level), so a
// language switch reaches every label — pitfall 1 of #2799.
const t = (key, ...rest) => i18n.global.t(key, ...rest)

export const DEFAULT_ESTIMATION = { unit: 'time', hours_per_day: 8, days_per_week: 5 }

// Point scales. The canonical value is always the number; T-shirt maps a size
// label to its number so a board can show "M" while storing 3.
export const POINTS_SCALES = {
  fibonacci: [1, 2, 3, 5, 8, 13, 21],
  linear: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10],
  tshirt: [
    { label: 'XS', value: 1 },
    { label: 'S', value: 2 },
    { label: 'M', value: 3 },
    { label: 'L', value: 5 },
    { label: 'XL', value: 8 },
    { label: 'XXL', value: 13 },
  ],
}

// Resolve the effective config for a task's project: project override, else the
// workspace default, else the built-in default. Pass the project/workspace rows
// (each may carry a nullable `estimation`).
export function resolveEstimation(project, workspace) {
  return project?.estimation || workspace?.estimation || DEFAULT_ESTIMATION
}

function minutesPerDay(cfg) {
  return (cfg?.hours_per_day || 8) * 60
}
function minutesPerWeek(cfg) {
  return minutesPerDay(cfg) * (cfg?.days_per_week || 5)
}

// The same two lengths as whole minutes, for the week/day/hour/minute ladder the
// two formatters below walk.
//
// The working day is a free decimal — the settings field is an `n-input-number`
// with no precision, and the backend only clamps it to [1, 24] — and a day like
// 8.2h is not exactly representable, so `8.2 * 60` is 491.99999999999994 rather
// than 492. Dividing the (integer) estimate by that float is off by one in both
// directions: one working day came out as "1д 5.684341886080802e-14м", because
// the residue is not zero and the minute branch prints whatever it holds; and at
// 8.3h/day the same one day (498 min) lost the day unit entirely and read "8ч 18м",
// because 498 / 498.00000000000006 floors to 0. Android rounds these two lengths
// the same way (util/Estimation.kt `minutesPerDay(cfg).roundToInt()`), so the phone
// printed the plain "1д" the web could not.
function ladderMinutes(cfg) {
  return { mpd: Math.round(minutesPerDay(cfg)), mpw: Math.round(minutesPerWeek(cfg)) }
}

// Drop a trailing ".0" so 5 shows as "5" and 2.5 as "2.5".
function trimNum(v) {
  return Number.isInteger(v) ? String(v) : String(+Number(v).toFixed(2))
}

// The ceiling on a parsed estimate — minutes for time, a plain count for
// points/custom. Mirrors quickact.MaxEstimate on the backend, which explains
// the number; here it does one more job.
//
// Without it the time branch below could return Infinity. `total` is a sum of
// products, and JS overflows to Infinity silently, so a long enough run of
// digits (a pasted id, a key held down) came out of the field as Infinity —
// which Math.round leaves alone and `mins > 0` accepts. parseEstimateInput then
// reported `{ ok: true }`, the popover applied it, and JSON.stringify turned
// Infinity into `null` on the way out: a PATCH that reads as "clear the
// estimate". That is the exact outcome the comment on parseEstimateInput calls
// the worst of the three, reached through arithmetic rather than through a
// typo (#2932).
const MAX_ESTIMATE = 1e9

// Parse free-text input into the canonical number, or null when unparseable.
// Time accepts mixed tokens ("1w 2d 3h 30m"), English (w/d/h/m) or Russian
// (н/д/ч/м) units, decimals, and a bare number (taken as hours). Points/custom
// take a plain number; a T-shirt size label is accepted too.
export function parseEstimate(input, cfg) {
  if (input == null) return null
  // replaceAll, not replace: the comma is the decimal separator of *every*
  // token, and a single-occurrence swap left "2,5ч 30,5м" as "2.5ч 30,5м" —
  // the second number split at the comma and its tail was read as bare hours.
  const s = String(input).trim().toLowerCase().replaceAll(',', '.')
  if (!s) return null
  const unit = cfg?.unit || 'time'

  if (unit !== 'time') {
    if (unit === 'points' && cfg?.points_scale === 'tshirt') {
      const hit = POINTS_SCALES.tshirt.find((o) => o.label.toLowerCase() === s)
      if (hit) return hit.value
    }
    const n = parseFloat(s)
    return Number.isFinite(n) && n > 0 && n <= MAX_ESTIMATE ? n : null
  }

  const mpd = minutesPerDay(cfg)
  const unitMin = { w: mpd * (cfg?.days_per_week || 5), d: mpd, h: 60, m: 1 }
  let total = 0
  let matched = false
  // The unit a bare number takes: hours when it stands alone, otherwise a step
  // down from the unit in front of it (see SMALLER). Anything else is a refusal.
  let prev = null
  const re = /(\d+(?:\.\d+)?)\s*([a-zа-яё]*)/gi
  let m
  while ((m = re.exec(s))) {
    const num = parseFloat(m[1])
    if (!Number.isFinite(num)) continue
    const u = m[2] || ''
    let key
    if (u) {
      key = unitKey(u)
      // An unrecognised suffix is not hours. "5 попугаев" storing five hours is
      // the mistake the backend parser already refuses to make (quickact
      // `estimateUnit`), and a field that guesses is worse than one that asks.
      if (!key) return null
    } else if (prev == null && !matched) {
      key = 'h' // the lone bare number: hours
    } else if (prev) {
      key = SMALLER[prev] // "1ч 30" = an hour and a half, not thirty-one hours
    } else {
      return null // a second bare number in a row means nothing in particular
    }
    total += num * unitMin[key]
    matched = true
    prev = u ? key : null
  }
  if (!matched) return null
  const mins = Math.round(total)
  return Number.isFinite(mins) && mins > 0 && mins <= MAX_ESTIMATE ? mins : null
}

// The unit letter a written-out suffix reduces to — the first letter decides, so
// "день"/"дня"/"d" are one unit. null for anything that is not a unit at all.
function unitKey(word) {
  const c = word[0]
  if (c === 'w' || c === 'н') return 'w'
  if (c === 'd' || c === 'д') return 'd'
  if (c === 'h' || c === 'ч') return 'h'
  if (c === 'm' || c === 'м') return 'm'
  return null
}

// What a number with no suffix means when it follows one that had a suffix: the
// next unit down, the way a person writing "1ч 30" or "1д 4" means it. Minutes
// are the floor, so "30м 15" is 45 minutes rather than a new ladder.
//
// Reading EVERY bare number as hours (what this used to do) made "1ч 30" 1860
// minutes — thirty-one hours — and said nothing about it. The backend's own
// `1d 4 → 720` case was always this rule; for days it happens to coincide.
const SMALLER = { w: 'd', d: 'h', h: 'm', m: 'm' }

// Read what the user typed in the estimate field: `ok` separates "the box is
// empty, drop the estimate" from "I could not read this" — both of which the
// parser reports as a null value, and the popover used to apply as a clear. An
// unreadable estimate silently erasing the one already on the task is the worst
// of the three outcomes: the number came from a person's own plan, and nothing
// on screen said it had gone.
export function parseEstimateInput(raw, cfg) {
  if (raw == null || String(raw).trim() === '') return { value: null, ok: true }
  const value = parseEstimate(raw, cfg)
  return value == null ? { value: null, ok: false } : { value, ok: true }
}

// Format a canonical value for display. Time compresses minutes to working
// weeks/days/hours/minutes (e.g. 30h with an 8h day → "3д 6ч"). Returns '' for
// an empty estimate.
export function formatEstimate(value, cfg) {
  if (value == null || !(value > 0)) return ''
  const unit = cfg?.unit || 'time'

  if (unit === 'points') {
    if (cfg?.points_scale === 'tshirt') {
      const hit = POINTS_SCALES.tshirt.find((o) => o.value === value)
      return hit ? hit.label : trimNum(value)
    }
    return t('task.estimate.short.points', { n: trimNum(value) })
  }
  if (unit === 'custom') {
    const label = (cfg?.custom_label || '').trim()
    return label ? `${trimNum(value)} ${label}` : trimNum(value)
  }

  const { mpd, mpw } = ladderMinutes(cfg)
  let rem = Math.round(value)
  const w = Math.floor(rem / mpw)
  rem -= w * mpw
  const d = Math.floor(rem / mpd)
  rem -= d * mpd
  const h = Math.floor(rem / 60)
  rem -= h * 60
  const min = rem
  const parts = []
  if (w) parts.push(t('task.estimate.short.week', { n: w }))
  if (d) parts.push(t('task.estimate.short.day', { n: d }))
  if (h) parts.push(t('task.estimate.short.hour', { n: h }))
  if (min) parts.push(t('task.estimate.short.minute', { n: min }))
  return parts.join(' ') || t('task.estimate.short.minute', { n: 0 })
}

// Discrete options for the modal's point picker (empty for time/custom, which
// use a free-text field). T-shirt yields {label, value} sizes.
export function scaleOptions(cfg) {
  if (cfg?.unit !== 'points') return []
  const scale = POINTS_SCALES[cfg?.points_scale] || POINTS_SCALES.fibonacci
  if (cfg?.points_scale === 'tshirt') return scale.map((o) => ({ label: o.label, value: o.value }))
  return scale.map((v) => ({ label: String(v), value: v }))
}

// Human name of the unit, for settings labels and aggregates. A custom unit is
// named by the user, so it is content and stays untranslated.
export function unitName(cfg) {
  const u = cfg?.unit || 'time'
  if (u === 'points') return t('task.estimate.unit.points')
  if (u === 'custom') return (cfg?.custom_label || '').trim() || t('task.estimate.unit.custom')
  return t('task.estimate.unit.time')
}

// Input placeholder hinting the accepted syntax for the resolved unit. The time
// example is localised together with the suffixes it demonstrates — `parseEstimate`
// accepts both alphabets, so a user may keep typing either one.
export function estimatePlaceholder(cfg) {
  const u = cfg?.unit || 'time'
  if (u === 'points') return t('task.estimate.placeholder.points')
  if (u === 'custom') return t('task.estimate.placeholder.custom')
  return t('task.estimate.placeholder.time')
}

// Full spelled-out expansion of a time estimate, e.g. 30h with an 8h day →
// "3 дня 6 часов". Points/custom fall back to the compact formatEstimate.
export function formatEstimateFull(value, cfg) {
  if (value == null || !(value > 0)) return ''
  if ((cfg?.unit || 'time') !== 'time') return formatEstimate(value, cfg)
  const { mpd, mpw } = ladderMinutes(cfg)
  let rem = Math.round(value)
  const w = Math.floor(rem / mpw)
  rem -= w * mpw
  const d = Math.floor(rem / mpd)
  rem -= d * mpd
  const h = Math.floor(rem / 60)
  rem -= h * 60
  const m = rem
  // ICU pluralisation, not hand-rolled forms: Russian needs three branches and
  // English only two, and vue-i18n picks the right one per locale.
  const parts = []
  if (w) parts.push(t('task.estimate.full.week', w))
  if (d) parts.push(t('task.estimate.full.day', d))
  if (h) parts.push(t('task.estimate.full.hour', h))
  if (m) parts.push(t('task.estimate.full.minute', m))
  return parts.join(' ') || t('task.estimate.full.minute', 0)
}

// Free-form projected window for tooltips/hints: "18 мая → 13 июл." — day +
// short month, the year appended only when it isn't the current one (mirrors the
// due-date style). Returns '' without a start date or a time estimate.
export function estimateRangeShort(startISO, value, cfg, fmt = defaultFormatters()) {
  const days = estimateToDays(value, cfg)
  if (days == null || !startISO) return ''
  const start = new Date(startISO)
  if (Number.isNaN(start.getTime())) return ''
  // The window is a span of calendar days, so its end is the start's *date* —
  // as the user sees it — stepped by N days, never the instant plus N×86400000.
  // A day with a clock change is not 24 hours long: a window crossing the autumn
  // change landed at 23:00 the evening before and printed the previous date, so
  // seven days from "20 окт." read "26 окт.". Same rule as utils/recurrence.js
  // and utils/timeAxis.js. Date.UTC normalises the day overflow, and the UTC
  // midnight it returns is read back in UTC so no offset can move it again.
  const sp = fmt.parts(start)
  if (!sp) return ''
  const end = new Date(Date.UTC(sp.year, sp.month, sp.day + Math.round(days)))
  // "This year" is the user's year, in their timezone — not the browser's.
  const nowY = fmt.today().year
  const label = (dt, timeZone) => {
    const o = { day: '2-digit', month: 'short', timeZone }
    if (fmt.parts(dt, { timeZone }).year !== nowY) o.year = 'numeric'
    return fmt.formatDate(dt, o)
  }
  return `${label(start)} → ${label(end, 'UTC')}`
}

// One-line tooltip body: spelled-out estimate + free-form projected window in
// parens, e.g. "8 недель (18 мая → 13 июл.)". Without a start date (no window to
// project) it degrades to just the estimate. Returns '' for an empty estimate.
export function estimateTooltip(startISO, value, cfg, fmt = defaultFormatters()) {
  const full = formatEstimateFull(value, cfg)
  if (!full) return ''
  const range = estimateRangeShort(startISO, value, cfg, fmt)
  return range ? `${full} (${range})` : full
}

// Convert a canonical estimate into a calendar-day span for the timeline/Gantt
// "ghost" envelope. Only the time unit has a duration meaning (points/custom are
// dimensionless → null). The estimate is taken as a fraction of a working week and
// mapped onto 7 calendar days, so e.g. 4 working weeks of effort reads as ~4
// calendar weeks on the axis. Returns null for an empty/non-time estimate.
export function estimateToDays(value, cfg) {
  if (value == null || !(value > 0)) return null
  if ((cfg?.unit || 'time') !== 'time') return null
  return (value / minutesPerWeek(cfg)) * 7
}

// Sum the estimates of a task list (e.g. subtasks for a parent rollup, or a
// timeline lane). Returns null when none are estimated. A board speaks one unit
// (one project), so the sum is meaningful.
export function sumEstimates(tasks) {
  let sum = 0
  let any = false
  for (const t of tasks || []) {
    const e = t?.estimate
    if (e != null && e > 0) {
      sum += e
      any = true
    }
  }
  return any ? sum : null
}

// What ONE card is worth: its own estimate, or — when it has none — the rollup of
// its subtasks. This is the number the card itself prints (TaskCardPills: `own ??
// rollup`, with the «Σ …» rollup tooltip), so anything totalling a set of cards has
// to read them the same way.
//
// Only the direct children, matching the card: the rollup chip sums the child list
// handed to it, not the whole tree.
export function effectiveEstimate(task, subtasks) {
  return task?.estimate ?? sumEstimates(subtasks)
}

// Total effort a lane holds — a kanban column's «Σ» header, a Timeline/Gantt
// swimlane's effort chip (#2932).
//
// It used to be `sumEstimates(lane)`, i.e. the OWN estimates of the lane's rows, and
// a lane holds top-level cards only (buildLists buckets filteredTasks; the charts'
// `source` is the scheduled top-level tasks, their children drawn as sub-bars inside
// the parent's row). So a task whose work was planned entirely in its subtasks — no
// own estimate, every child estimated — showed «Σ 5д» on its own card and contributed
// exactly nothing to the total above it. The header is what a sprint's load is read
// off, and the tasks it silently dropped were the decomposed ones: the big ones.
//
// `subtasksByParent` must be the same child map the cards are drawn from (the
// FILTERED one on the board), or the header stops agreeing with the cards under it
// for the opposite reason.
export function sumLaneEffort(tasks, subtasksByParent = {}) {
  let sum = 0
  let any = false
  for (const t of tasks || []) {
    const e = effectiveEstimate(t, subtasksByParent[t?.id])
    if (e != null && e > 0) {
      sum += e
      any = true
    }
  }
  return any ? sum : null
}
