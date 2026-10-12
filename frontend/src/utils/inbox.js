// «Входящие» (#2972): the pure part of the feed — what state a record is in,
// which group it falls into, and the moments «Отложить» offers. The store and
// the components read the clock once and pass it in, so all of this is testable
// with a fixed `now`.
//
// A record is a reminder or a note as GET /workspaces/:id/inbox returns it,
// tagged with `kind` by the store: { kind: 'reminder' | 'note', ...row }.

const DAY_MS = 86400000
const HOUR_MS = 3600000

export const startOfDay = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate())

// 'done' · 'fired' (its time has come) · 'snoozed' (moved and still ahead) ·
// 'upcoming' — and 'note' for a note.
export function recordState(r, now = new Date()) {
  if (r.kind === 'note') return 'note'
  if (r.done) return 'done'
  if (Date.parse(r.remind_at) <= now.getTime()) return 'fired'
  if (r.snoozed_from) return 'snoozed'
  return 'upcoming'
}

// A done reminder stays in the feed, struck through, until the reader's
// midnight (#3039): «Выполнено сегодня».
export function doneToday(r, now = new Date()) {
  if (!r.done) return false
  const at = r.done_at ? Date.parse(r.done_at) : NaN
  return Number.isFinite(at) && at >= startOfDay(now).getTime()
}

export const FILTERS = ['all', 'reminders', 'notes', 'board']
export const GROUPS = ['fired', 'today', 'later', 'notes']

// Does the record pass the segment filter? «К этой доске» keeps the records
// tied to a task of the open board. `tag` (a tagKey) narrows any segment to the
// records wearing that tag (#3040).
export function passes(r, filter, boardId, tag = '') {
  if (tag && !hasTag(r, tag)) return false
  if (filter === 'reminders') return r.kind === 'reminder'
  if (filter === 'notes') return r.kind === 'note'
  if (filter === 'board') return !!boardId && r.task?.board_id === boardId
  return true
}

// ── tags and mentions of a record (#3040) ──
//
// A record's `tags` are names. Tied to a task (or, a note, to a project) it
// wears that project's tags — the server already spells a matching name the
// project's way, and the pill takes the tag's colour here; any other name is
// the record's free label, drawn neutral. `mentions` are member ids.

// How tag names compare everywhere: trimmed, «#» optional, case-insensitive.
export const tagKey = (name) =>
  String(name || '')
    .trim()
    .replace(/^#+/, '')
    .toLowerCase()

export function hasTag(r, key) {
  return (r?.tags || []).some((n) => tagKey(n) === key)
}

// The project whose tags the record wears: its task's, else its own.
export function recordProjectId(r) {
  return r?.task?.project_id || r?.project_id || null
}

// The workspace's tags (GET /workspaces/:id/tags) by project.
export function tagsByProject(tags = []) {
  const map = new Map()
  for (const tg of tags) {
    if (!tg?.project_id) continue
    if (!map.has(tg.project_id)) map.set(tg.project_id, [])
    map.get(tg.project_id).push(tg)
  }
  return map
}

/**
 * The record's tag pills, in the record's order.
 * @returns {Array<{ name: string, tag: object|null }>} `tag` — the project's
 *   tag of that name (colour and all), null for a free label
 */
export function recordTags(r, byProject = new Map(), projectId = recordProjectId(r)) {
  const own = (projectId && byProject.get(projectId)) || []
  return (r?.tags || []).map((name) => ({
    name,
    tag: own.find((tg) => tagKey(tg.name) === tagKey(name)) || null,
  }))
}

// The members a record mentions, in its order; ids nobody in the roster owns
// any more are left out (the person left the workspace).
export function recordPeople(r, members = []) {
  const out = []
  for (const id of r?.mentions || []) {
    const m = members.find((x) => x.user_id === id)
    if (m && !out.includes(m)) out.push(m)
  }
  return out
}

// «#релиз, frontend #идеи» → ['релиз', 'frontend', 'идеи']: the tag line of an
// opened record. Repeats fold onto the first spelling.
export function parseTagLine(text) {
  const out = []
  for (const w of String(text || '').split(/[\s,]+/)) {
    const name = w.replace(/^#+/, '').trim()
    if (name && !out.some((x) => tagKey(x) === tagKey(name))) out.push(name)
  }
  return out
}

export const tagLine = (tags = []) => tags.map((n) => `#${n}`).join(' ')

/**
 * Group the feed: «Сработало» (open reminders whose time has come, oldest
 * first), «Сегодня» (the rest of today, then the ones done today), «Позже»,
 * «Заметки» (newest edit first). Done reminders from before today drop out.
 * @returns {Array<{ key: string, items: object[] }>} non-empty groups in order
 */
export function groupInbox(
  { reminders = [], notes = [] },
  { now = new Date(), filter = 'all', boardId = null, tag = '' } = {},
) {
  const tomorrow = startOfDay(now).getTime() + DAY_MS
  const out = { fired: [], today: [], later: [], notes: [] }
  const doneHere = []
  for (const r of reminders) {
    const rec = r.kind ? r : { ...r, kind: 'reminder' }
    if (!passes(rec, filter, boardId, tag)) continue
    const st = recordState(rec, now)
    if (st === 'done') {
      if (doneToday(rec, now)) doneHere.push(rec)
      continue
    }
    if (st === 'fired') out.fired.push(rec)
    else if (Date.parse(rec.remind_at) < tomorrow) out.today.push(rec)
    else out.later.push(rec)
  }
  const byTime = (a, b) => Date.parse(a.remind_at) - Date.parse(b.remind_at)
  out.fired.sort(byTime)
  out.today.sort(byTime)
  out.later.sort(byTime)
  doneHere.sort(byTime)
  out.today.push(...doneHere)
  for (const n of notes) {
    const rec = n.kind ? n : { ...n, kind: 'note' }
    if (passes(rec, filter, boardId, tag)) out.notes.push(rec)
  }
  out.notes.sort((a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at))
  return GROUPS.map((key) => ({ key, items: out[key] })).filter((g) => g.items.length)
}

// Open reminders whose time has come — the counter on the top bar and the
// sidebar row.
export function firedCount(reminders = [], now = new Date()) {
  return reminders.filter((r) => !r.done && Date.parse(r.remind_at) <= now.getTime()).length
}

export function doneTodayCount(reminders = [], now = new Date()) {
  return reminders.filter((r) => doneToday(r, now)).length
}

// The moment a task's due date stands for: its own time when it has one, 18:00
// of the day for a date-only due (UTC midnight — the app's all-day rule).
export function dueMoment(due) {
  if (!due) return null
  const d = new Date(due)
  if (Number.isNaN(d.getTime())) return null
  if (d.getUTCHours() === 0 && d.getUTCMinutes() === 0 && d.getUTCSeconds() === 0) {
    return new Date(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate(), 18, 0)
  }
  return d
}

const at = (base, days, h, m = 0) =>
  new Date(base.getFullYear(), base.getMonth(), base.getDate() + days, h, m)

/**
 * The «Отложить» menu (#2997, mockup SnoozeMenu): every option counted from now.
 * «Сегодня вечером» only while the evening is ahead; «К сроку задачи» only for a
 * record tied to a task whose due moment is still ahead.
 * @returns {Array<{ key: string, at: Date }>}
 */
export function snoozeOptions(now = new Date(), task = null) {
  const out = [{ key: 'hour', at: new Date(now.getTime() + HOUR_MS) }]
  if (now.getHours() < 17) out.push({ key: 'evening', at: at(now, 0, 18) })
  out.push({ key: 'morning', at: at(now, 1, 9) })
  // The coming Monday (a Monday morning means next week's).
  const ahead = (8 - now.getDay()) % 7 || 7
  out.push({ key: 'monday', at: at(now, ahead, 9) })
  const due = dueMoment(task?.due_date)
  if (due && due > now) out.push({ key: 'due', at: due })
  return out
}

/**
 * «+ Напомнить» in the task modal (mockup TaskModalB): through an hour, tomorrow
 * morning, at the task's due moment.
 */
export function quickRemindOptions(now = new Date(), task = null) {
  const out = [
    { key: 'hour', at: new Date(now.getTime() + HOUR_MS) },
    { key: 'morning', at: at(now, 1, 9) },
  ]
  const due = dueMoment(task?.due_date)
  if (due && due > now) out.push({ key: 'due', at: due })
  return out
}

/**
 * A short distance between two moments, for «40 мин назад» / «через 1 ч 20 м».
 * @returns {{ key: 'now'|'min'|'hm'|'h'|'d', past: boolean, n?: number, h?: number, m?: number }}
 */
export function distance(target, now = new Date()) {
  const diff = Date.parse(target) - now.getTime()
  const past = diff < 0
  const mins = Math.round(Math.abs(diff) / 60000)
  if (mins < 1) return { key: 'now', past }
  if (mins < 60) return { key: 'min', past, n: mins }
  const h = Math.floor(mins / 60)
  const m = mins % 60
  if (h < 24) return m && h < 6 ? { key: 'hm', past, h, m } : { key: 'h', past, n: h }
  return { key: 'd', past, n: Math.round(h / 24) }
}

// The record's line in the panel and on the start screen: the first line of
// the body with Markdown marks stripped.
export function firstLine(body) {
  const line = String(body || '')
    .split('\n')
    .map((l) => l.trim())
    .find(Boolean)
  if (!line) return ''
  return line
    .replace(/^(#{1,6}\s+|[-*+]\s+(\[[ xX]\]\s+)?|\d+\.\s+|>\s*)/, '')
    .replace(/[*_`~]/g, '')
    .trim()
}

// The open reminders of a task, soonest first — the card's bell pill and the
// task modal's section.
export function remindersByTask(reminders = []) {
  const map = new Map()
  for (const r of reminders) {
    if (r.done || !r.task_id) continue
    if (!map.has(r.task_id)) map.set(r.task_id, [])
    map.get(r.task_id).push(r)
  }
  for (const list of map.values())
    list.sort((a, b) => Date.parse(a.remind_at) - Date.parse(b.remind_at))
  return map
}
