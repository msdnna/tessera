// Shared time-axis math for the Timeline and Gantt board views. Both render the
// same horizontal day-grid (months band + day/week/hour band + bars), so the pure
// geometry lives here to keep the two components in lock-step.
//
// Zoom changes not just the day density (px/day) but the axis GRANULARITY: zoomed
// far out we group by weeks, zoomed far in we add hour ticks and position bars at
// their real clock time (sub-day). A task date is "all-day" when it serialises to
// UTC midnight (GitLab/legacy or a user who left the time at 00:00); a non-zero UTC
// time means a real time-of-day. This mirrors useDateLocale (web) and
// Dates.isUtcMidnight (Android) — the same signal already used for labels.

import { defaultFormatters } from '@/utils/format'

export const DAY_MS = 86400000
export const HOUR_MS = 3600000

// Weekday and month names come from Intl, not from a table of Russian
// abbreviations (#2799): the axis has to speak the user's language, and the
// table would have had to grow a column per locale.
//
// Both are read in the BROWSER timezone (`timeZone: null`), because the axis is
// laid out on local calendar days — the same rule the cursor label follows.
const weekdayShort = (ms, fmt) => fmt.formatDate(ms, { weekday: 'short', timeZone: null })
const monthYear = (ms, fmt) =>
  fmt.formatDate(ms, { month: 'short', year: 'numeric', timeZone: null })

export const startOfDay = (ms) => {
  const d = new Date(ms)
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
}

// The instant at a fractional day offset along the axis: the whole part is a
// LOCAL calendar day, the fraction is the clock time inside it.
//
// Not `rangeStart + offset * DAY_MS` — the axis is laid out on local calendar
// days (startOfDay above, dayIndex in useChartTimeline), and a day with a clock
// change is not 24 hours long. An autumn day lasts 25, so ms arithmetic lands at
// 23:00 of the *previous* day and everything past the change carries its
// neighbour's date, while the bars — built from real local midnights — stay put.
// Outside a clock change the two agree exactly.
export function msAtDayOffset(rangeStart, offset) {
  const i = Math.floor(offset)
  const d = new Date(rangeStart)
  const dayStart = new Date(d.getFullYear(), d.getMonth(), d.getDate() + i).getTime()
  return offset === i ? dayStart : dayStart + (offset - i) * DAY_MS
}

// True when the instant lands on UTC midnight → a date-only (all-day) value with no
// meaningful time of day. Timed values carry a non-zero UTC time.
export function isAllDayMs(ms) {
  const d = new Date(ms)
  return d.getUTCHours() === 0 && d.getUTCMinutes() === 0 && d.getUTCSeconds() === 0
}

// The axis day a STORED date belongs to, as the local midnight that day's column
// starts at — the value-side counterpart of startOfDay.
//
// Not startOfDay for everything, because an all-day value names a calendar DATE,
// not a moment: milestones are written as pure UTC midnight (utils/milestones
// toDateOnlyISO) and GitLab hands over "2006-01-02" the same way (client.go
// parseDate). Reading that instant in the browser zone is only harmless east of
// UTC — west of it, UTC midnight is the EVENING OF THE DAY BEFORE, so startOfDay
// returns the previous local day and the marker/bar lands 24h early while the
// label beside it says the right date (format.js zoneOf reads date-only in UTC).
// Timed values keep their local reading: for them the clock time is the point.
export const dayStartOf = (ms) => {
  const d = new Date(ms)
  if (!isAllDayMs(ms)) return startOfDay(ms)
  return new Date(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()).getTime()
}

// Parse a serialised task date to epoch ms; null/'' → null (an unset endpoint).
export const parseDate = (s) => (s ? Date.parse(s) : null)

// Effective day-span endpoints of a task. A one-ended task (only start OR only due)
// collapses to a 1-day span — both views draw it as a single-day bar.
export function spanOf(t) {
  const s = parseDate(t.start_date)
  const d = parseDate(t.due_date)
  const a = s ?? d
  const b = d ?? s
  return { a: dayStartOf(a), b: dayStartOf(b), hasStart: s != null, hasDue: d != null }
}

// Zoom tier from the current px-per-day. Thresholds tuned by feel:
//  - below ~8px/day even a week range doesn't fit a cell → group by months (v2);
//  - below ~20px/day a 2-digit day number doesn't fit a cell → group by weeks;
//  - at/above ~140px/day a day is wide enough to show ≥3 hour ticks → hours.
export const MONTHS_MAX_DAYW = 8
export const WEEKS_MAX_DAYW = 20
export const HOURS_MIN_DAYW = 140
export function tierFor(dayW) {
  if (dayW < MONTHS_MAX_DAYW) return 'months'
  if (dayW < WEEKS_MAX_DAYW) return 'weeks'
  if (dayW >= HOURS_MIN_DAYW) return 'hours'
  return 'days'
}

// The scale presets of the chart toolbar (v2, #2959): «Дни / Недели / Месяцы».
// Each is a px-per-day landing inside its tier; the hours tier counts as «Дни».
export const SCALE_DAYW = { days: 24, weeks: 10, months: 4 }
export function scaleOfTier(tier) {
  if (tier === 'months') return 'months'
  if (tier === 'weeks') return 'weeks'
  return 'days'
}

// Working days (Mon–Fri) in an inclusive day span — the selected bar's tooltip
// («9 раб. дн.»). Holidays are not known here, only weekends.
export function workingDays(startMs, endMs) {
  if (startMs == null || endMs == null) return 0
  let a = startOfDay(Math.min(startMs, endMs))
  const b = startOfDay(Math.max(startMs, endMs))
  let n = 0
  while (a <= b) {
    const dow = new Date(a).getDay()
    if (dow !== 0 && dow !== 6) n++
    a = startOfDay(a + DAY_MS + HOUR_MS * 2) // +2h rides over a DST shift
  }
  return n
}

// Continuous time→x mapping (px from the axis origin). At a day boundary this equals
// dayIndex * dayW; between boundaries it gives sub-day precision for the hours tier.
export const xAt = (ms, rangeStart, dayW) => ((ms - rangeStart) / DAY_MS) * dayW

// Pixel span of a task bar. In the hours tier a timed endpoint honours its real clock
// time; an all-day endpoint (and EVERY endpoint in the day/week tiers) snaps to a day
// boundary — start → day start, due → day end — so an all-day task still fills its
// whole day exactly as before. `start`/`due` are epoch ms (either may be null).
export function barSpan({ start, due, tier, rangeStart, dayW, minW = 6 }) {
  const hasStart = start != null
  const hasDue = due != null
  const sMs = start ?? due
  const dMs = due ?? start
  if (sMs == null) return null
  const honor = tier === 'hours'
  const leftMs = honor && !isAllDayMs(sMs) ? sMs : dayStartOf(sMs)
  const rightMs = honor && !isAllDayMs(dMs) ? dMs : dayStartOf(dMs) + DAY_MS
  const left = xAt(leftMs, rangeStart, dayW)
  return {
    left,
    leftMs,
    width: Math.max(minW, xAt(rightMs, rangeStart, dayW) - left),
    hasStart,
    hasDue,
  }
}

// Left edge (epoch ms) a bar/ghost anchors to — honours the start's clock time in the
// hours tier, else the day start. Used so the estimate ghost lines up with the bar.
export function anchorMs(startMs, tier) {
  return tier === 'hours' && !isAllDayMs(startMs) ? startMs : dayStartOf(startMs)
}

// Day cells for the day/hours band. `fmt` is a formatter set from useFormat();
// callers outside a component (tests) fall back to the default preferences.
export function buildDays(rangeStart, count, todayMs, fmt = defaultFormatters()) {
  const out = []
  for (let i = 0; i < count; i++) {
    const ms = msAtDayOffset(rangeStart, i)
    const d = new Date(ms)
    const dow = d.getDay()
    out.push({
      ms,
      day: d.getDate(),
      dow: weekdayShort(ms, fmt),
      weekend: dow === 0 || dow === 6,
      isToday: startOfDay(ms) === todayMs,
    })
  }
  return out
}

// Month header bands: runs of consecutive same-month days. `short` drops the
// year — the months tier puts the years in a band of their own above.
export function buildMonthBands(days, fmt = defaultFormatters(), { short = false } = {}) {
  const out = []
  for (const d of days) {
    const dt = new Date(d.ms)
    const key = `${dt.getFullYear()}-${dt.getMonth()}`
    const last = out[out.length - 1]
    if (last && last.key === key) last.span++
    else
      out.push({
        key,
        label: short
          ? fmt.formatDate(d.ms, { month: 'short', timeZone: null })
          : monthYear(d.ms, fmt),
        span: 1,
      })
  }
  return out
}

// Year bands (months tier, v2): runs of consecutive same-year days.
export function buildYearBands(days) {
  const out = []
  for (const d of days) {
    const y = new Date(d.ms).getFullYear()
    const last = out[out.length - 1]
    if (last && last.key === y) last.span++
    else out.push({ key: y, label: String(y), span: 1 })
  }
  return out
}

// Week bands labelled with their date range — «28 сент. – 4 окт.» — the top band
// of the days tier (v2, #2959). Break on the user's first day of the week, like
// buildWeekBands below; the edge bands may be partial.
export function buildWeekRangeBands(days, fmt = defaultFormatters()) {
  const firstDow = fmt.prefs.weekStart === 0 ? 0 : 1
  const out = []
  for (const d of days) {
    const last = out[out.length - 1]
    if (last && new Date(d.ms).getDay() !== firstDow) {
      last.span++
      last.endMs = d.ms
    } else out.push({ key: d.ms, startMs: d.ms, endMs: d.ms, span: 1 })
  }
  const dm = (ms) => fmt.formatDate(ms, { day: 'numeric', month: 'short', timeZone: null })
  for (const b of out) {
    const sameMonth = new Date(b.startMs).getMonth() === new Date(b.endMs).getMonth()
    b.label =
      b.startMs === b.endMs
        ? dm(b.startMs)
        : `${sameMonth ? new Date(b.startMs).getDate() : dm(b.startMs)} – ${dm(b.endMs)}`
  }
  return out
}

// Week header bands (weeks tier): break on the user's first day of the week; the
// first band may be partial. Label = the band's day-of-month RANGE ("29–4"); the
// month is already shown by the band above, and a date+month label would overflow
// the narrow cell.
//
// The break day comes from the preferences rather than being pinned to Monday:
// the same setting already drives the due-date calendar, every Naive picker
// (useFormat's firstDayOfWeek) and formatRelative's "inside this week" window, so
// a hard-coded Monday made this header the one place that ignored it.
export function buildWeekBands(days, fmt = defaultFormatters()) {
  const firstDow = fmt.prefs.weekStart === 0 ? 0 : 1
  const out = []
  for (const d of days) {
    const dt = new Date(d.ms)
    const last = out[out.length - 1]
    if (last && dt.getDay() !== firstDow) {
      last.span++
      last.endDay = dt.getDate()
    } else {
      out.push({ key: d.ms, startDay: dt.getDate(), endDay: dt.getDate(), span: 1 })
    }
  }
  for (const b of out)
    b.label = b.startDay === b.endDay ? `${b.startDay}` : `${b.startDay}–${b.endDay}`
  return out
}

// Hour-tick step (in hours) that keeps labels at least ~34px apart, or 0 when even a
// 12h step is too tight (so the caller skips ticks). Returns one of 1/2/3/4/6/12.
export function hourStepFor(dayW) {
  const pxPerHour = dayW / 24
  for (const s of [1, 2, 3, 4, 6, 12]) if (s * pxPerHour >= 34) return s
  return 0
}

// One hour tick's label, in the user's 12/24h preference and language.
//
// The hour ruler was the one band of this header that never got the formatter
// treatment the two above it did (weekdayShort/monthYear): it built "13" by hand,
// so a user on a 12-hour clock read "13" on the ruler while the cursor pill
// hanging off that very tick — `formatTime` in useChartTimeline — said "1:00 PM".
//
// A tick names an hour of the day and nothing else, so the instant it formats is
// synthetic: a local-midnight date pushed to hour h, read in the browser zone
// (`timeZone: null`) because that is the zone the whole axis is laid out in.
// `hour: 'numeric'` is what keeps "1 PM" from coming out as "01 AM" under
// `2-digit`; the 24h side pads back to two digits afterwards so the ruler reads
// exactly as it always has for everyone who never changed the preference.
function hourLabel(h, fmt) {
  const s = fmt.formatTime(new Date(2026, 0, 1, h), {
    hour: 'numeric',
    minute: undefined,
    timeZone: null,
  })
  return /^\d+$/.test(s) ? s.padStart(2, '0') : s
}

// Hour ticks (hours tier) within a pixel window [loPx, hiPx] of the axis — only the
// visible slice is built so a multi-month board doesn't render thousands of nodes
// ("lazy lines"). { left, label }, skipping the day boundary (h=0, a major line).
export function hourTicksInWindow(dayCount, dayW, loPx, hiPx, fmt = defaultFormatters()) {
  const step = hourStepFor(dayW)
  if (!step) return []
  const out = []
  const d0 = Math.max(0, Math.floor(loPx / dayW))
  const d1 = Math.min(dayCount - 1, Math.ceil(hiPx / dayW))
  // Every day of the window repeats the same 24 labels, so they are formatted once.
  const labels = new Map()
  for (let h = step; h < 24; h += step) labels.set(h, hourLabel(h, fmt))
  for (let day = d0; day <= d1; day++) {
    for (let h = step; h < 24; h += step) {
      out.push({
        key: day * 24 + h,
        left: (day + h / 24) * dayW,
        label: labels.get(h),
      })
    }
  }
  return out
}
