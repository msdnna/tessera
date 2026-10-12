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
//   → a reminder tomorrow at 10:00 titled «Созвон по релизу», tag «frontend»,
//     tied to task #2951.
//
// «#тег» is a tag of the record (#3040); «@человек» a member it mentions, read
// the way the quick entry reads a person (a word of the name, the e-mail login,
// the GitLab login). A handle that names nobody — or more than one person —
// stays in the title as typed rather than vanishing.
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
 *   people: object[],       // the members the line mentions
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
    kept.push(w)
  }
  const base = parseQuick(kept.join(' '), { now, people, tags })
  rest.push(...base.title.split(/\s+/).filter(Boolean))

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
    taskRef,
  }
}

// ── Срок словами (#3118, DuePicker) ──
//
//   «пт 18:00», «завтра», «через 3 дня», «14.10», «след. понедельник 9:00»,
//   «конец недели», «через 2 ч», «в 18», «вечером», «без срока»
//
// Every word has to be understood: a due is one moment, and a line that is half
// a date and half a title (the quick entry's job) is a typo here — the picker
// says «Не понял …» about the words it could not read instead of guessing.
//
// `now` is a WALL clock — a local Date whose fields are the reader's calendar
// (profile timezone) — and the answer is in the same terms: a calendar day and
// an optional 'HH:MM'. Turning that into an instant is the caller's business
// (utils/dueModel `writeEndpoint`), the one place that knows the zone.

const NEXT_WORDS = new Set(['след', 'след.', 'следующий', 'следующая', 'следующую', 'next'])
const FILLER = new Set(['в', 'во', 'на', 'at', 'on', 'by'])
const CLEAR_WORDS = ['без срока', 'нет срока', 'нет', 'убрать', 'no date', 'none', 'clear']
const END_OF_WEEK = ['конец недели', 'к концу недели', 'end of week', 'eow']
const PART_OF_DAY = {
  утром: '09:00',
  morning: '09:00',
  днём: '12:00',
  днем: '12:00',
  noon: '12:00',
  вечером: '18:00',
  evening: '18:00',
}
const IN_UNITS = [
  [/^(мин|минут[уы]?|m|min|mins|minutes?)$/, 'min'],
  [/^(ч|час|часа|часов|h|hr|hrs|hours?)$/, 'hour'],
  [/^(д|дн|день|дня|дней|d|days?)$/, 'day'],
  [/^(н|нед|неделю|недели|недель|w|wk|weeks?)$/, 'week'],
  [/^(мес|месяц|месяца|месяцев|mo|months?)$/, 'month'],
]
const inUnit = (w) => IN_UNITS.find(([re]) => re.test(w.replace(/\.$/, '')))?.[1] || null
const hhmm = (h, m = 0) => `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`

function clockWord(word, afterAt) {
  const t = timeFromWord(word)
  if (t) return t
  // «6pm», «9am», «12am»
  let m = /^(\d{1,2})(am|pm)$/.exec(word)
  if (m && Number(m[1]) <= 12) return hhmm((Number(m[1]) % 12) + (m[2] === 'pm' ? 12 : 0))
  // «18ч», «9h» — or a bare «18» right after «в» / «at»
  m = /^(\d{1,2})(?:ч|h)$/.exec(word) || (afterAt ? /^(\d{1,2})$/.exec(word) : null)
  if (m && Number(m[1]) <= 23) return hhmm(Number(m[1]))
  return null
}

// A clock typed on its own (the picker's «Время» field): «18:00», «9.30», «6pm»,
// «6:30 pm», «18». 'HH:MM' or null.
export function parseClock(text) {
  const s = String(text || '')
    .trim()
    .toLowerCase()
    .replace(/\s+/g, '')
  if (!s) return null
  const m = /^(\d{1,2})(?:[:.](\d{2}))?(am|pm)$/.exec(s)
  if (m) {
    const h = Number(m[1])
    const min = Number(m[2] || 0)
    if (h < 1 || h > 12 || min > 59) return null
    return hhmm((h % 12) + (m[3] === 'pm' ? 12 : 0), min)
  }
  return clockWord(s, true)
}

// «через 3 дня» / «через 3д» / «через неделю» / «in 2 hours» at words[i]:
// how many words it took and the moved wall clock, or null.
function readIn(words, i, now) {
  if (!IN_WORDS.has(words[i]) || i + 1 >= words.length) return null
  let n = 1
  let unit = inUnit(words[i + 1])
  let used = 1
  if (!unit) {
    const m = /^(\d{1,3})([^\d]*)$/.exec(words[i + 1])
    if (!m) return null
    n = Number(m[1])
    unit = m[2] ? inUnit(m[2]) : i + 2 < words.length ? inUnit(words[i + 2]) : null
    used = m[2] ? 1 : 2
  }
  if (!unit) return null
  const at = new Date(now)
  if (unit === 'min') at.setMinutes(at.getMinutes() + n)
  else if (unit === 'hour') at.setHours(at.getHours() + n)
  else if (unit === 'day') at.setDate(at.getDate() + n)
  else if (unit === 'week') at.setDate(at.getDate() + 7 * n)
  else at.setMonth(at.getMonth() + n)
  return { used, at, clock: unit === 'min' || unit === 'hour' }
}

/**
 * Read a due typed in words.
 * @returns {{ empty: true }
 *   | { ok: true, clear: true }
 *   | { ok: true, day: Date|null, time: string|null }   // day: wall start of day
 *   | { ok: false, bad: string }}                       // the words not understood
 */
export function parseDue(text, { now = new Date() } = {}) {
  let s = String(text || '')
    .trim()
    .toLowerCase()
    .replace(/,/g, ' ')
    .replace(/\s+/g, ' ')
  if (!s) return { empty: true }
  if (CLEAR_WORDS.includes(s)) return { ok: true, clear: true }
  const today = startOfDay(now)
  let day = null
  let time = null
  for (const phrase of END_OF_WEEK) {
    if (s === phrase || s.startsWith(`${phrase} `)) {
      // The coming Friday — today, when today is one.
      day = new Date(today.getTime() + ((5 - today.getDay() + 7) % 7) * DAY_MS + 3600000)
      s = s.slice(phrase.length).trim()
      break
    }
  }
  const words = s.split(' ').filter(Boolean)
  const bad = []
  for (let i = 0; i < words.length; i++) {
    const w = words[i]
    const moved = readIn(words, i, now)
    if (moved) {
      day = moved.at
      if (moved.clock) time = hhmm(moved.at.getHours(), moved.at.getMinutes())
      i += moved.used
      continue
    }
    if (FILLER.has(w)) continue
    // «след. понедельник» — the one after today, a week on when today is Monday.
    if (NEXT_WORDS.has(w) && WEEKDAY_OF.has(words[i + 1])) {
      const ahead = (WEEKDAY_OF.get(words[i + 1]) - today.getDay() + 7) % 7 || 7
      day = new Date(today.getTime() + ahead * DAY_MS + 3600000)
      i += 1
      continue
    }
    if (w in PART_OF_DAY) {
      time = PART_OF_DAY[w]
      continue
    }
    const d = dayFromWord(w, now)
    if (d) {
      day = d
      continue
    }
    const iso = /^(\d{4})-(\d{1,2})-(\d{1,2})$/.exec(w)
    if (iso) {
      const dd = new Date(Number(iso[1]), Number(iso[2]) - 1, Number(iso[3]))
      if (dd.getMonth() === Number(iso[2]) - 1 && dd.getDate() === Number(iso[3])) {
        day = dd
        continue
      }
    }
    const tm = clockWord(w, FILLER.has(words[i - 1]))
    if (tm) {
      time = tm
      continue
    }
    bad.push(w)
  }
  if (bad.length) return { ok: false, bad: bad.join(' ') }
  if (!day && !time) return { ok: false, bad: String(text).trim() }
  return { ok: true, day: day ? startOfDay(day) : null, time }
}
