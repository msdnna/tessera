// The rules of the in-app toasts (v2, #3117 — PackageI): how long each kind
// lives, which one gives way to which, what the settings and «Не беспокоить»
// let through, how a burst of board activity folds into one toast. Pure — the
// store (stores/toasts.js) owns the state and the clock, this owns the rules, so
// they are tested without timers or a DOM.
//
// Two lanes: `bottom` — answers to my own actions (undo, errors, warnings,
// success, info), centred over the work area; `top` — what came from outside
// (about me, reminders, calls, the open board's activity, the system, the
// «Пока вас не было» digest), under the bell on the right.

// How long a toast stays, ms; 0 — until acted on or closed.
export const DURATION = {
  undo: 7000,
  success: 4000,
  info: 4000,
  activity: 5000,
  me: 8000,
  // Mentions and assignments are «про меня» too — only stronger in the deck.
  mention: 8000,
  assign: 8000,
  away: 8000,
  error: 8000,
  // An error with «Повторить» waits for the reader: Esc or × drop it unretried.
  errorRetry: 0,
  warning: 8000,
  reminder: 0,
  update: 0,
  call: 30000,
  system: 5000,
}

// Who gives way to whom in the top lane, strongest first: a call → a reminder →
// a mention or an assignment → the rest about me → the system → board activity.
export const RANK = {
  call: 6,
  reminder: 5,
  mention: 4,
  assign: 4,
  me: 3,
  system: 2,
  activity: 1,
}

// At most this many toasts are shown at once in the top lane; three and more
// lie as a deck until hovered.
export const TOP_MAX = 3
export const BOTTOM_MAX = 3
export const BATCH_WINDOW = 10000
// A toast «про меня» and the board's own event about the same task arrive a
// moment apart, in either order; activity waits this long for its twin.
export const ACTIVITY_DELAY = 400
export const ACTIVITY_TWIN_MS = 6000

// What the «Всплывающие в приложении» card stores in notification_prefs.toasts.
// Unknown keys from a newer client are kept by normalizePrefs' caller (the
// store merges over the server object), missing ones fall back to these.
export const ME_CATEGORIES = [
  'assign',
  'mentions',
  'comments',
  'changes',
  'removal',
  'due',
  'calls',
]
export const ACTIVITY_KINDS = ['created', 'moved', 'completed', 'deleted']
export const ACTIVITY_MODES = ['off', 'others', 'all']

export function defaultPrefs() {
  return {
    me: Object.fromEntries(ME_CATEGORIES.map((k) => [k, true])),
    activity: 'others',
    activityKinds: { created: false, moved: true, completed: true, deleted: true },
    batch: true,
    batchMin: 3,
    phone: true,
  }
}

export const BATCH_MIN = 2
export const BATCH_MAX = 9

// normalizePrefs reads the server's object, tolerating anything: a missing or
// mistyped key takes its default.
export function normalizePrefs(raw) {
  const out = defaultPrefs()
  const src = raw && typeof raw === 'object' && !Array.isArray(raw) ? raw : {}
  const me = src.me && typeof src.me === 'object' ? src.me : {}
  for (const k of ME_CATEGORIES) if (typeof me[k] === 'boolean') out.me[k] = me[k]
  if (ACTIVITY_MODES.includes(src.activity)) out.activity = src.activity
  const kinds = src.activityKinds && typeof src.activityKinds === 'object' ? src.activityKinds : {}
  for (const k of ACTIVITY_KINDS) if (typeof kinds[k] === 'boolean') out.activityKinds[k] = kinds[k]
  if (typeof src.batch === 'boolean') out.batch = src.batch
  const n = Math.round(Number(src.batchMin))
  if (Number.isFinite(n)) out.batchMin = Math.min(BATCH_MAX, Math.max(BATCH_MIN, n))
  if (typeof src.phone === 'boolean') out.phone = src.phone
  return out
}

// ── notifications ────────────────────────────────────────────────────────

// The payload event of a notification, falling back to its stored kind for a
// row older than the payload (migration 0065).
function eventOf(n) {
  const ev = n?.payload?.event
  if (ev) return ev
  const kind = n?.kind || ''
  const BY_KIND = {
    assigned: 'task_assigned',
    unassigned: 'task_unassigned',
    mention: 'task_mention',
    comment: 'task_comment',
    updated: 'task_updated',
    moved: 'task_moved',
    completed: 'task_completed',
    reopened: 'task_reopened',
    archived: 'task_archived',
    deleted: 'task_deleted',
    due_soon: 'task_due_soon',
    reminder: 'reminder',
    conference_invite: 'conference_invited',
    integration_sync: 'integration_sync_ok',
  }
  return BY_KIND[kind] || ''
}
export { eventOf as notificationEvent }

// Which switch of «Про меня» governs a notification; 'system' for the
// integration's report, which only the main switch and «Не беспокоить» govern.
export function notificationCategory(n) {
  const ev = eventOf(n)
  switch (ev) {
    case 'task_assigned':
    case 'task_unassigned':
      return 'assign'
    case 'task_mention':
    case 'note_mention':
    case 'task_thread_reply':
      return 'mentions'
    case 'task_comment':
      return 'comments'
    case 'task_updated':
    case 'task_moved':
    case 'task_completed':
    case 'task_reopened':
      return 'changes'
    case 'task_deleted':
    case 'task_archived':
      return 'removal'
    case 'task_due_soon':
    case 'reminder':
      return 'due'
    case 'conference_invited':
      return 'calls'
    default:
      return ev.startsWith('integration_sync') ? 'system' : 'changes'
  }
}

// The toast kind of a notification: decides its rank and how long it lives.
export function notificationKind(n) {
  const ev = eventOf(n)
  if (ev === 'conference_invited') return 'call'
  if (ev === 'reminder') return 'reminder'
  if (ev.startsWith('integration_sync')) return 'system'
  // A shared «Входящие» note naming the reader (#3040) is a mention too.
  if (ev === 'task_mention' || ev === 'note_mention' || ev === 'task_thread_reply') return 'mention'
  if (ev === 'task_assigned') return 'assign'
  return 'me'
}

// ── what silences a toast ────────────────────────────────────────────────

// Mentions, reminders and calls pass «Не беспокоить» and the quiet hours —
// the menu says so («Упоминания, напоминания и звонки проходят»).
export function passesQuiet(kind) {
  return kind === 'mention' || kind === 'reminder' || kind === 'call'
}

// Minutes since midnight of `now` in an IANA zone ('' — UTC).
export function minuteOfDay(now, tz) {
  try {
    const parts = new Intl.DateTimeFormat('en-GB', {
      timeZone: tz || 'UTC',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    }).formatToParts(now)
    const h = Number(parts.find((p) => p.type === 'hour')?.value)
    const m = Number(parts.find((p) => p.type === 'minute')?.value)
    if (Number.isFinite(h) && Number.isFinite(m)) return h * 60 + m
  } catch {
    /* an unknown zone reads as UTC, like the server's quietWindow */
  }
  return now.getUTCHours() * 60 + now.getUTCMinutes()
}

// Is `now` inside the quiet window — and does the window cover the toasts
// («И всплывающие в приложении»)? Mirrors the server's quietWindow: a window
// may wrap past midnight; start == end is no window at all.
export function inQuietHours(server, now = new Date()) {
  if (!server?.quiet_enabled || !server?.quiet_toasts) return false
  const start = Number(server.quiet_start_minutes)
  const end = Number(server.quiet_end_minutes)
  if (!Number.isFinite(start) || !Number.isFinite(end) || start === end) return false
  const m = minuteOfDay(now, server.quiet_tz)
  return start < end ? m >= start && m < end : m >= start || m < end
}

export function dndActive(dndUntil, now = new Date()) {
  if (!dndUntil) return false
  const d = new Date(dndUntil)
  return !Number.isNaN(d.getTime()) && d.getTime() > now.getTime()
}

// «До завтра, 9:00» and «На 1 час» of the moon's menu.
export function dndPreset(preset, now = new Date()) {
  if (preset === 'hour') return new Date(now.getTime() + 60 * 60 * 1000)
  const d = new Date(now)
  d.setDate(d.getDate() + 1)
  d.setHours(9, 0, 0, 0)
  return d
}

// ── the top lane: who gives way ──────────────────────────────────────────

// insertTop puts a new toast on top of the lane (newest first) and, past `max`,
// pushes out the weakest one — the oldest of the lowest rank. The newcomer
// itself is the one to go when everything shown outranks it. Returns the new
// list and the toasts pushed out.
export function insertTop(list, item, max = TOP_MAX) {
  const next = [item, ...list]
  const evicted = []
  while (next.length > max) {
    let weakest = -1
    for (let i = 0; i < next.length; i++) {
      // Ties go to the older one: the list is newest first, so the later index.
      if (weakest < 0 || next[i].rank <= next[weakest].rank) weakest = i
    }
    evicted.push(next.splice(weakest, 1)[0])
  }
  return { list: next, evicted }
}

// The bottom lane keeps the oldest first (it grows up, the newest at the
// bottom) and, past `max`, lets the oldest non-error go: errors stand above the
// undo toasts and nothing of somebody else's moves them.
export function insertBottom(list, item, max = BOTTOM_MAX) {
  const next = [...list, item]
  const evicted = []
  while (next.length > max) {
    let i = next.findIndex((x) => x.tone !== 'red')
    if (i < 0) i = 0
    evicted.push(next.splice(i, 1)[0])
  }
  return { list: next, evicted }
}

// The phone has one lane and shows one toast at a time; the rest wait in this
// order: errors, then the undo toasts, then the top lane by rank, newest first.
export function phoneOrder(items) {
  const weight = (x) => (x.lane === 'undo' ? 50 : x.tone === 'red' ? 100 : x.rank || 0)
  return [...items].sort((a, b) => weight(b) - weight(a) || (b.at || 0) - (a.at || 0))
}

// ── board activity ───────────────────────────────────────────────────────

// Does the settings card let this board event through? `self` — the reader did
// it; the mode «Включая мои» shows those too.
export function activityAllowed(prefs, a) {
  if (!prefs || prefs.activity === 'off') return false
  if (a.self && prefs.activity !== 'all') return false
  const kind = a.verb === 'reopened' ? 'moved' : a.verb
  return prefs.activityKinds?.[kind] !== false && ACTIVITY_KINDS.includes(kind)
}

// Events of one person doing one thing (moving to the same column) within the
// window fold into one toast past the threshold.
export function batchKey(a) {
  return [a.actorId || a.actorName || '?', a.verb, a.verb === 'moved' ? a.column || '' : ''].join(
    '|',
  )
}

// ── «Пока вас не было» ───────────────────────────────────────────────────

// The digest counts what arrived while the tab was hidden, by these groups.
export const AWAY_GROUPS = ['assign', 'comments', 'mentions', 'changes', 'reminders', 'calls']

export function awayGroup(n) {
  const kind = notificationKind(n)
  if (kind === 'call') return 'calls'
  if (kind === 'reminder') return 'reminders'
  const cat = notificationCategory(n)
  if (cat === 'assign') return 'assign'
  if (cat === 'mentions') return 'mentions'
  if (cat === 'comments') return 'comments'
  if (cat === 'system') return null
  return 'changes'
}

// The digest line: «2 назначения, 5 комментариев, 1 упоминание».
export function awayLine(counts, t, formatList = (xs) => xs.join(', ')) {
  const parts = []
  for (const g of AWAY_GROUPS) {
    const n = counts?.[g] || 0
    if (n) parts.push(t(`toasts.away.${g}`, n, { named: { n } }))
  }
  return formatList(parts)
}
