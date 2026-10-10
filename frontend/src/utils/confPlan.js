// A meeting's plan beyond its start (#3034): its length and repeat (#3052) and
// the stage it is tied to (#3053). Pure — the screens hand in `t`.
//
// The server owns the arithmetic of a repeat: it answers `next_at`, the next
// occurrence that is not over yet, computed in the meeting's own time zone. The
// client only names the rule and sends the reader's zone when it creates one.

// The repeat presets, in the order the menu offers them. '' is «не повторять».
export const RECURRENCES = ['', 'daily', 'weekdays', 'weekly', 'biweekly', 'monthly']

// The lengths the form offers, in minutes; the default is the mockup's 30.
export const DURATIONS = [15, 30, 45, 60, 90, 120]
export const DEFAULT_DURATION = 30

// The i18n key of a rule — '' has no key of its own.
export const recurrenceKey = (rule) => `conferences.plan.repeat.${rule || 'none'}`

export function recurrenceLabel(rule, t) {
  return t(recurrenceKey(rule))
}

// «15 мин», «1 ч», «1 ч 30 мин» — or '' when the meeting has no length.
export function durationLabel(minutes, t) {
  const n = Number(minutes)
  if (!n || n < 1) return ''
  if (n < 60) return t('conferences.plan.min', { n })
  const h = Math.floor(n / 60)
  const m = n % 60
  return m ? t('conferences.plan.hm', { h, m }) : t('conferences.plan.h', { h })
}

// The instant a row is filed and shown by: the next occurrence of a repeat,
// else the planned start.
export function planAt(c) {
  return c?.next_at || c?.scheduled_at || null
}

// The stage page of the meeting's stage, or null when it has none (or it is out
// of reach — the server then sends no slugs).
export function stageHref(c) {
  if (!c?.milestone_slug || !c?.milestone_project_slug) return null
  return `/project/${c.milestone_project_slug}/stage/${c.milestone_slug}`
}

// The reader's IANA zone, sent with a repeating meeting so «11:00» stays 11:00
// on the clock after a daylight-saving switch. '' when the runtime cannot say.
export function browserTimezone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || ''
  } catch {
    return ''
  }
}

// The stage menu: «Без этапа», then the open stages — under their project's
// name when the workspace has more than one project with stages. A closed stage
// is left out unless it is the current one (a meeting may stay tied to the stage
// it reviewed).
export function stageMenuItems(milestones, current, t) {
  const open = (milestones || []).filter((m) => m.state !== 'closed' || m.id === current)
  const items = [
    { key: '', label: t('conferences.plan.noStage'), checked: !current, testid: 'stage-none' },
  ]
  if (!open.length) {
    items.push({ hint: t('conferences.plan.noStages') })
    return items
  }
  const projects = [...new Set(open.map((m) => m.project_name || ''))]
  for (const p of projects) {
    if (projects.length > 1) items.push({ heading: p })
    for (const m of open.filter((x) => (x.project_name || '') === p))
      items.push({ key: m.id, label: m.title, checked: m.id === current, testid: 'stage-option' })
  }
  return items
}

// The repeat menu.
export function recurrenceMenuItems(current, t) {
  return RECURRENCES.map((r) => ({
    key: r || 'none',
    label: recurrenceLabel(r, t),
    checked: (current || '') === r,
    testid: `repeat-${r || 'none'}`,
  }))
}

// The plan fields of a create/update body. A repeat needs a start (the server
// refuses one without), so it is dropped when the time is cleared; the zone
// rides along only with a repeat — a one-off has no clock to keep.
export function planBody({ at, duration, recurrence, milestoneId }, tz = browserTimezone()) {
  const rule = at ? recurrence || '' : ''
  return {
    duration_minutes: duration || null,
    recurrence: rule,
    timezone: rule ? tz : '',
    milestone_id: milestoneId || null,
  }
}
