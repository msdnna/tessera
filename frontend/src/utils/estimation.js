// Task-estimation helpers. The backend stores a single canonical `estimate`
// number per task whose *unit* lives in a two-level config (project override →
// workspace default → the built-in default below). This module resolves that
// config and does all input parsing / output formatting client-side:
//   • time   → canonical minutes; "3d 4h" / "1w" / "90m" parse via the working
//              day/week, and output compresses back ("3 дня" = 24 working hours);
//   • points → the point number on a scale (Fibonacci / T-shirt / linear);
//   • custom → a count of a named unit.
// Mirrors the backend canonicalisation in handlers/estimation.go.

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

// Drop a trailing ".0" so 5 shows as "5" and 2.5 as "2.5".
function trimNum(v) {
  return Number.isInteger(v) ? String(v) : String(+Number(v).toFixed(2))
}

// Parse free-text input into the canonical number, or null when unparseable.
// Time accepts mixed tokens ("1w 2d 3h 30m"), English (w/d/h/m) or Russian
// (н/д/ч/м) units, decimals, and a bare number (taken as hours). Points/custom
// take a plain number; a T-shirt size label is accepted too.
export function parseEstimate(input, cfg) {
  if (input == null) return null
  const s = String(input).trim().toLowerCase().replace(',', '.')
  if (!s) return null
  const unit = cfg?.unit || 'time'

  if (unit !== 'time') {
    if (unit === 'points' && cfg?.points_scale === 'tshirt') {
      const hit = POINTS_SCALES.tshirt.find((o) => o.label.toLowerCase() === s)
      if (hit) return hit.value
    }
    const n = parseFloat(s)
    return Number.isFinite(n) && n > 0 ? n : null
  }

  const mpd = minutesPerDay(cfg)
  const unitMin = { w: mpd * (cfg?.days_per_week || 5), d: mpd, h: 60, m: 1 }
  let total = 0
  let matched = false
  const re = /(\d+(?:\.\d+)?)\s*([a-zа-яё]*)/gi
  let m
  while ((m = re.exec(s))) {
    const num = parseFloat(m[1])
    if (!Number.isFinite(num)) continue
    const u = m[2] || ''
    let key = 'h' // a bare number is hours
    if (u) {
      const c = u[0]
      if (c === 'w' || c === 'н') key = 'w'
      else if (c === 'd' || c === 'д') key = 'd'
      else if (c === 'h' || c === 'ч') key = 'h'
      else if (c === 'm' || c === 'м') key = 'm'
    }
    total += num * unitMin[key]
    matched = true
  }
  if (!matched) return null
  const mins = Math.round(total)
  return mins > 0 ? mins : null
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
    return `${trimNum(value)} SP`
  }
  if (unit === 'custom') {
    const label = (cfg?.custom_label || '').trim()
    return label ? `${trimNum(value)} ${label}` : trimNum(value)
  }

  const mpd = minutesPerDay(cfg)
  const mpw = minutesPerWeek(cfg)
  let rem = Math.round(value)
  const w = Math.floor(rem / mpw)
  rem -= w * mpw
  const d = Math.floor(rem / mpd)
  rem -= d * mpd
  const h = Math.floor(rem / 60)
  rem -= h * 60
  const min = rem
  const parts = []
  if (w) parts.push(`${w}н`)
  if (d) parts.push(`${d}д`)
  if (h) parts.push(`${h}ч`)
  if (min) parts.push(`${min}м`)
  return parts.join(' ') || '0м'
}

// Discrete options for the modal's point picker (empty for time/custom, which
// use a free-text field). T-shirt yields {label, value} sizes.
export function scaleOptions(cfg) {
  if (cfg?.unit !== 'points') return []
  const scale = POINTS_SCALES[cfg?.points_scale] || POINTS_SCALES.fibonacci
  if (cfg?.points_scale === 'tshirt') return scale.map((o) => ({ label: o.label, value: o.value }))
  return scale.map((v) => ({ label: String(v), value: v }))
}

// Human name of the unit, for settings labels and aggregates.
export function unitName(cfg) {
  const u = cfg?.unit || 'time'
  if (u === 'points') return 'Стори-поинты'
  if (u === 'custom') return (cfg?.custom_label || '').trim() || 'Единицы'
  return 'Время'
}

// Input placeholder hinting the accepted syntax for the resolved unit.
export function estimatePlaceholder(cfg) {
  const u = cfg?.unit || 'time'
  if (u === 'points') return 'напр. 5'
  if (u === 'custom') return 'напр. 8'
  return 'напр. 3д 4ч, 90м, 1н'
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
