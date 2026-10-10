// The conferences list as the v2 screen reads it (#3107, PackageE «Встречи»):
// the calls that are live go first as cards, the rest fall into day groups —
// planned ones by their day (soonest first), ones without a time together,
// finished ones last (latest first). Pure; the component hands in the
// formatters so labels follow the reader's language and time zone.
//
// A repeating meeting (#3052) is filed by its next occurrence (`next_at`, which
// the server computes), so it never falls into «Прошли, но не начались».
import { planAt } from '@/utils/confPlan'

// One day's label: «Сегодня, вт 21 окт.», «Завтра, ср 22 окт.», «пт 24 окт.».
export function dayLabel(d, f, now = new Date()) {
  const rel = f.formatRelative(d, { now, long: true, dateOnly: false })
  const date = f.formatDate(d, { weekday: 'short', day: 'numeric', month: 'short' })
  // A weekday inside this week comes back from formatRelative too; it is
  // already in the date, so only «today / tomorrow / yesterday» are kept.
  const near = rel && Math.abs(dayDiff(d, f, now)) <= 1
  return near ? `${rel}, ${date}` : date
}

function dayDiff(d, f, now) {
  const a = f.parts(d)
  const b = f.today(now)
  return Math.round(
    (Date.UTC(a.year, a.month, a.day) - Date.UTC(b.year, b.month, b.day)) / 86400000,
  )
}

// The planned call to offer «Войти» on: the earliest one that has not been
// over for more than a quarter of an hour.
export function nearestScheduled(list, now = new Date()) {
  const from = now.getTime() - 15 * 60000
  let best = null
  for (const c of list) {
    if (c.status !== 'scheduled' || !planAt(c)) continue
    const at = Date.parse(planAt(c))
    if (at < from) continue
    if (!best || at < Date.parse(planAt(best))) best = c
  }
  return best
}

export function groupConferences(list, f, { now = new Date(), t } = {}) {
  const live = list.filter((c) => c.status === 'live')
  const planned = list
    .filter((c) => c.status === 'scheduled' && planAt(c))
    .sort((a, b) => Date.parse(planAt(a)) - Date.parse(planAt(b)))
  const untimed = list.filter((c) => c.status === 'scheduled' && !planAt(c))
  const ended = list
    .filter((c) => c.status === 'ended')
    .sort((a, b) => Date.parse(b.ended_at || 0) - Date.parse(a.ended_at || 0))

  const groups = []
  // Planned for a day already gone and never held: after the days ahead.
  const today = f.today(now)
  const todayKey = Date.UTC(today.year, today.month, today.day)
  const past = []
  for (const c of planned) {
    const pp = f.parts(new Date(planAt(c)))
    if (Date.UTC(pp.year, pp.month, pp.day) < todayKey) {
      past.push(c)
      continue
    }
    const d = new Date(planAt(c))
    const p = f.parts(d)
    const key = `d-${p.year}-${p.month}-${p.day}`
    let g = groups.find((x) => x.key === key)
    if (!g) {
      g = { key, label: dayLabel(d, f, now), rows: [] }
      groups.push(g)
    }
    g.rows.push(c)
  }
  if (untimed.length)
    groups.push({ key: 'untimed', label: t('conferences.list.untimed'), rows: untimed })
  if (past.length)
    groups.push({ key: 'past', label: t('conferences.list.past'), rows: past.reverse() })
  if (ended.length) groups.push({ key: 'ended', label: t('conferences.list.ended'), rows: ended })
  return { live, groups }
}
