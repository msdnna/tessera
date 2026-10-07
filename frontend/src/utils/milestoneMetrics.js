// The stage page's chart and figure maths (#2993). Pure: the page hands in the
// answer of GET /milestones/:id/metrics (backend handlers/milestone_metrics.go)
// and draws what these return.

const DAY = 86400000

// 'YYYY-MM-DD' → epoch ms of that UTC midnight (the API's day keys are UTC days).
export function dayMs(key) {
  if (!key) return null
  const [y, m, d] = String(key).slice(0, 10).split('-').map(Number)
  if (!y || !m || !d) return null
  return Date.UTC(y, m - 1, d)
}

export function dayKey(ms) {
  return new Date(ms).toISOString().slice(0, 10)
}

export function isWeekend(ms) {
  const d = new Date(ms).getUTCDay()
  return d === 0 || d === 6
}

// Every day key from `from` to `to` inclusive.
export function daysRange(from, to) {
  const a = dayMs(from)
  const b = dayMs(to)
  if (a == null || b == null || b < a) return []
  const out = []
  for (let t = a; t <= b; t += DAY) out.push(dayKey(t))
  return out
}

// Which figure a series point carries for a mode: 'tasks' counts, 'estimate'
// sums the estimate.
export function pick(point, mode, which) {
  if (!point) return 0
  if (mode === 'estimate') return which === 'done' ? point.done_est : point.scope_est
  return which === 'done' ? point.done : point.scope
}

// The burn-up forecast (mockup: «прогноз пунктиром по темпу последних 5 рабочих
// дней»): the pace is what was done over the last five working days of the
// series, carried on over the working days left until `end`. Returns
// { pace, value } — done per working day, and done expected by `end` (capped at
// the scope) — or null when there is nothing to project from.
export function forecast(series, end, mode = 'tasks', window = 5) {
  if (!series?.length || !end) return null
  const last = series[series.length - 1]
  const lastMs = dayMs(last.date)
  const endMs = dayMs(end)
  if (lastMs == null || endMs == null) return null
  // Walk back over working days only.
  const work = series.filter((p) => !isWeekend(dayMs(p.date)))
  if (work.length < 2) return null
  const tail = work.slice(-(window + 1))
  const first = tail[0]
  const doneNow = pick(last, mode, 'done')
  const span = tail.length - 1
  const pace =
    span > 0 ? (pick(tail[tail.length - 1], mode, 'done') - pick(first, mode, 'done')) / span : 0
  let left = 0
  for (let t = lastMs + DAY; t <= endMs; t += DAY) if (!isWeekend(t)) left++
  const scope = pick(last, mode, 'scope')
  const value = Math.min(scope, doneNow + pace * left)
  return { pace, value: Math.max(doneNow, value) }
}

// A «nice» top for the value axis and its ticks (4 steps).
export function niceAxis(max) {
  const m = Math.max(1, max)
  const raw = m / 4
  const pow = 10 ** Math.floor(Math.log10(raw))
  const step = [1, 2, 2.5, 5, 10].map((k) => k * pow).find((s) => s >= raw) || raw
  const top = step * Math.ceil(m / step)
  const ticks = []
  for (let v = 0; v <= top + 1e-9; v += step) ticks.push(Math.round(v * 100) / 100)
  return { top, ticks }
}

// Burn-down: what is left (scope − done) per day, and the ideal straight line
// from the scope at the start to zero at the end.
export function burndown(series, mode = 'tasks') {
  return (series || []).map((p) => ({
    date: p.date,
    left: Math.max(0, pick(p, mode, 'scope') - pick(p, mode, 'done')),
  }))
}

// The stroke dash of a progress ring of radius r at `pct` (0–100).
export function ringDash(pct, r) {
  const c = 2 * Math.PI * r
  const p = Math.max(0, Math.min(100, pct || 0))
  return { dasharray: `${(c * p) / 100} ${c}`, circumference: c }
}

export function percent(done, total) {
  return total > 0 ? Math.round((done / total) * 100) : 0
}

// Average «done per stage» over the earlier stages of the pace list (the
// current one excluded) — the «Среднее за 3 этапа» line.
export function paceAverage(pace) {
  const prev = (pace || []).filter((p) => !p.current)
  if (!prev.length) return null
  return Math.round(prev.reduce((s, p) => s + (p.done || 0), 0) / prev.length)
}
