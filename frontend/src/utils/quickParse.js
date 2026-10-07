// Quick entry on the start screen (v2, #2960): one line, read as you type.
//
//   «Позвонить Анне завтра 11:00 !высокий #звонки @анна»
//   → title «Позвонить Анне», due tomorrow 11:00, priority 3, tag «звонки»,
//     assignee Анна.
//
// Words are matched in both languages — a workspace is often bilingual, and a
// token the parser doesn't know stays in the title rather than vanishing. The
// words themselves are data the parser matches, not interface text, which is why
// this file sits on the Russian-literals allowlist next to columnStatus.js.

const DAY_MS = 86400000

const REL_DAYS = {
  сегодня: 0,
  today: 0,
  завтра: 1,
  tomorrow: 1,
  послезавтра: 2,
}

// Weekday words → JS getDay(): Sunday is 0.
const WEEKDAYS = [
  [0, ['вс', 'воскресенье', 'sun', 'sunday']],
  [1, ['пн', 'понедельник', 'mon', 'monday']],
  [2, ['вт', 'вторник', 'tue', 'tuesday']],
  [3, ['ср', 'среда', 'среду', 'wed', 'wednesday']],
  [4, ['чт', 'четверг', 'thu', 'thursday']],
  [5, ['пт', 'пятница', 'пятницу', 'fri', 'friday']],
  [6, ['сб', 'суббота', 'субботу', 'sat', 'saturday']],
]
const WEEKDAY_OF = new Map(WEEKDAYS.flatMap(([d, words]) => words.map((w) => [w, d])))

// `!word` → priority 0–4. Prefix matching, so «!срочно» and «!срочный» both land.
const PRIORITY_RULES = [
  [4, /^(срочн|urgent|4$)/],
  [3, /^(высок|high|3$)/],
  [2, /^(обычн|средн|норм|normal|medium|2$)/],
  [1, /^(низк|low|1$)/],
  [0, /^(без|none|0$)/],
]

const startOfDay = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate())

function dayFromWord(word, now) {
  const w = word.toLowerCase()
  if (w in REL_DAYS) return new Date(startOfDay(now).getTime() + REL_DAYS[w] * DAY_MS + 3600000)
  if (WEEKDAY_OF.has(w)) {
    // The coming one — today counts: «пт» typed on a Friday means today.
    const today = startOfDay(now)
    const ahead = (WEEKDAY_OF.get(w) - today.getDay() + 7) % 7
    return new Date(today.getTime() + ahead * DAY_MS + 3600000)
  }
  // 14.10, 14.10.26, 14.10.2026, 14/10
  const m = /^(\d{1,2})[./](\d{1,2})(?:[./](\d{2}|\d{4}))?$/.exec(w)
  if (m) {
    const day = Number(m[1])
    const month = Number(m[2]) - 1
    let year = m[3] ? Number(m[3]) : now.getFullYear()
    if (year < 100) year += 2000
    const d = new Date(year, month, day)
    if (d.getMonth() !== month || d.getDate() !== day) return null
    // A bare «14.10» already past this year means next year's.
    if (!m[3] && d < startOfDay(now)) d.setFullYear(year + 1)
    return d
  }
  return null
}

function timeFromWord(word) {
  const m = /^(\d{1,2})[:.](\d{2})$/.exec(word)
  if (!m) return null
  const h = Number(m[1])
  const min = Number(m[2])
  if (h > 23 || min > 59) return null
  return `${String(h).padStart(2, '0')}:${m[2]}`
}

// A person by an @handle: the start of any word of the name, the e-mail's local
// part, or the GitLab login. One match only — an ambiguous handle stays text.
function findPerson(handle, people) {
  const h = handle.toLowerCase()
  if (!h) return null
  const hits = people.filter((p) => {
    const words = (p.name || '').toLowerCase().split(/\s+/)
    const mail = (p.email || '').toLowerCase().split('@')[0]
    return (
      words.some((w) => w.startsWith(h)) || mail === h || (p.gl_username || '').toLowerCase() === h
    )
  })
  return hits.length === 1 ? hits[0] : null
}

function findTag(name, tags) {
  const n = name.toLowerCase()
  return tags.find((tg) => (tg.name || '').toLowerCase() === n) || null
}

/**
 * Read one quick-entry line.
 * @returns {{
 *   title: string,
 *   day: Date|null,        // the local day, when a date word was given
 *   time: string|null,     // 'HH:MM'
 *   due: string|null,      // ISO; an all-day date is UTC midnight (the app's rule)
 *   priority: number|null,
 *   tags: Array<{ name: string, tag: object|null }>,
 *   people: object[],
 * }}
 */
export function parseQuick(text, { now = new Date(), people = [], tags = [] } = {}) {
  const rest = []
  let day = null
  let time = null
  let priority = null
  const tagHits = []
  const personHits = []
  for (const word of String(text || '')
    .split(/\s+/)
    .filter(Boolean)) {
    if (word.length > 1 && word.startsWith('!')) {
      const rule = PRIORITY_RULES.find(([, re]) => re.test(word.slice(1).toLowerCase()))
      if (rule) {
        priority = rule[0]
        continue
      }
    }
    if (word.length > 1 && word.startsWith('#')) {
      const name = word.slice(1)
      if (!tagHits.some((x) => x.name.toLowerCase() === name.toLowerCase()))
        tagHits.push({ name, tag: findTag(name, tags) })
      continue
    }
    if (word.length > 1 && word.startsWith('@')) {
      const p = findPerson(word.slice(1), people)
      if (p) {
        if (!personHits.includes(p)) personHits.push(p)
        continue
      }
    }
    // «14.10» is a date and «14:10» a time; «11.45» is no date, so it is a time.
    const d = dayFromWord(word, now)
    if (d) {
      day = d
      continue
    }
    const t = timeFromWord(word)
    if (t) {
      time = t
      continue
    }
    rest.push(word)
  }

  let due = null
  if (day || time) {
    const base = day || startOfDay(now)
    if (time) {
      const [h, m] = time.split(':').map(Number)
      due = new Date(base.getFullYear(), base.getMonth(), base.getDate(), h, m).toISOString()
    } else {
      due = new Date(Date.UTC(base.getFullYear(), base.getMonth(), base.getDate())).toISOString()
    }
  }

  return {
    title: rest.join(' '),
    day: day ? startOfDay(day) : time ? startOfDay(now) : null,
    // Whether a date word was given (a bare time also fills `day` with today).
    dated: !!day,
    time,
    due,
    priority,
    tags: tagHits,
    people: personHits,
  }
}

// ── «Входящие» capture (#2996) ──
//
//   «завтра 10:00 созвон по релизу #frontend /2951»
//   → a reminder tomorrow at 10:00 titled «Созвон по релизу», tag «frontend»
//     (not stored yet, #3040), tied to task #2951.
//
// A date or a time makes the line a reminder, anything else a note. A date
// without a time means 09:00; a bare time already past today means tomorrow.
// «/123» names a task by number, «/слово» by a word of its title. «через 2 ч»,
// «через 30 мин», «через 3 дня» count from now.

export const CAPTURE_DEFAULT_HOUR = 9

const IN_WORDS = new Set(['через', 'in'])
const UNIT_MS = [
  [/^(мин|минут[уы]?|m|min|mins|minutes?)$/, 60000],
  [/^(ч|час|часа|часов|h|hr|hrs|hours?)$/, 3600000],
  [/^(д|дн|день|дня|дней|d|days?)$/, DAY_MS],
]

function unitMs(word) {
  const w = word.toLowerCase().replace(/\.$/, '')
  return UNIT_MS.find(([re]) => re.test(w))?.[1] || null
}

/**
 * Read one capture line.
 * @returns {{
 *   title: string,          // the rest, first letter raised
 *   kind: 'reminder'|'note',
 *   remindAt: string|null,  // ISO, when the line names a moment
 *   day: Date|null, time: string|null, relative: boolean,
 *   tags: Array<{ name: string, tag: object|null }>,
 *   people: object[], mentions: string[],
 *   taskRef: { number: number } | { word: string } | null,
 * }}
 */
export function parseCapture(text, { now = new Date(), people = [], tags = [] } = {}) {
  const words = String(text || '')
    .split(/\s+/)
    .filter(Boolean)
  const rest = []
  let taskRef = null
  let offset = null
  const kept = []
  const mentions = []
  for (let i = 0; i < words.length; i++) {
    const w = words[i]
    // «через 2 ч» / «через 2ч» / «in 30 min»
    if (IN_WORDS.has(w.toLowerCase()) && i + 1 < words.length) {
      const m = /^(\d{1,3})([^\d]*)$/.exec(words[i + 1])
      if (m) {
        const unit = m[2] ? unitMs(m[2]) : i + 2 < words.length ? unitMs(words[i + 2]) : null
        if (unit) {
          offset = Number(m[1]) * unit
          i += m[2] ? 1 : 2
          continue
        }
      }
    }
    if (w.length > 1 && w.startsWith('/') && !taskRef) {
      const ref = w.slice(1)
      taskRef = /^#?\d+$/.test(ref) ? { number: Number(ref.replace('#', '')) } : { word: ref }
      continue
    }
    if (w.length > 1 && w.startsWith('@')) mentions.push(w.slice(1))
    kept.push(w)
  }
  const base = parseQuick(kept.join(' '), { now, people, tags })
  // parseQuick keeps an unknown @handle in the title; the capture's people are
  // a stub (#3040), so a handle never lands in the title either way.
  for (const w of base.title.split(/\s+/).filter(Boolean)) {
    if (w.length > 1 && w.startsWith('@')) continue
    rest.push(w)
  }

  let remindAt = null
  let day = base.day
  if (offset != null) {
    remindAt = new Date(now.getTime() + offset).toISOString()
    day = startOfDay(new Date(remindAt))
  } else if (base.dated || base.time) {
    const d = base.day || startOfDay(now)
    const [h, m] = (base.time || `${CAPTURE_DEFAULT_HOUR}:00`).split(':').map(Number)
    let at = new Date(d.getFullYear(), d.getMonth(), d.getDate(), h, m)
    // «18:00» typed at 19:30 is tomorrow's — a reminder never starts in the past
    // unless a day was named for it.
    if (!base.dated && at <= now) at = new Date(at.getTime() + DAY_MS)
    day = startOfDay(at)
    remindAt = at.toISOString()
  }
  const title = rest.join(' ')
  return {
    title: title ? title[0].toLocaleUpperCase() + title.slice(1) : '',
    kind: remindAt ? 'reminder' : 'note',
    remindAt,
    day,
    time: base.time,
    relative: offset != null,
    tags: base.tags,
    people: base.people,
    mentions,
    taskRef,
  }
}
