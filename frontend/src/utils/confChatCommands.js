// The call chat's «/» and «@» (#3055, CallScreen «Сообщение · / задача · @
// человек», PackageE §4 «В чате / создаёт задачу прямо из встречи»).
//
// Pure, so the composer's rules are pinned by tests without a textarea; the
// Android client (util/ConferenceChatCommands.kt) keeps the same rules.
//
// «/задача Название» creates a task on a board of the workspace and leaves a line
// «создал(а) задачу #N» in the chat (POST /conferences/:id/tasks). «@Имя» names a
// member of the workspace: the chip is drawn on the line, and the person is
// notified — the ids travel with the message, as for a comment.

// The command's names: the Russian word, the English one, and their one-letter
// shorthands. The first is what the picker inserts in Russian, the second in
// English.
export const TASK_COMMANDS = ['задача', 'task', 'з', 't']

// The open «/» query while the draft is still the command's name — «/», «/за» —
// or null. A slash anywhere but at the very start is text («и/или»).
export function slashQuery(draft) {
  const m = /^\/(\S*)$/.exec(String(draft || ''))
  return m ? m[1] : null
}

// The commands a «/» query offers. One command today; the list shape leaves room.
export function commandMatches(query) {
  const q = String(query || '').toLowerCase()
  return TASK_COMMANDS.some((c) => c.startsWith(q)) ? ['task'] : []
}

// The task a draft asks for: { title } when the draft is «/задача …» with a
// title, { title: '' } for the bare command (the composer then waits for one),
// null for an ordinary message.
export function parseTaskCommand(draft) {
  const m = /^\/(\S+)(?:\s+([\s\S]*))?$/.exec(String(draft || '').trim())
  if (!m || !TASK_COMMANDS.includes(m[1].toLowerCase())) return null
  return { title: (m[2] || '').replace(/\s+/g, ' ').trim() }
}

// The open «@» query at the caret: { start, query } or null. Opens after a line
// start, whitespace or an opening bracket, so «user@mail» stays text; spaces end
// it (names are matched by their beginning, then inserted whole).
export function mentionQuery(upto) {
  const m = /(^|[\s([{«"'])@([^\s@]*)$/.exec(String(upto || ''))
  if (!m) return null
  return { start: upto.length - m[2].length - 1, query: m[2] }
}

// The members a «@» query offers: by the start of any word of the name, or of
// the e-mail; at most `limit`, the reader themself left out.
export function mentionMatches(members, query, { me = '', limit = 8 } = {}) {
  const q = String(query || '').toLowerCase()
  const hit = (m) => {
    if (!q) return true
    const name = String(m.name || '').toLowerCase()
    if (name.split(/\s+/).some((w) => w.startsWith(q)) || name.startsWith(q)) return true
    return String(m.email || '')
      .toLowerCase()
      .startsWith(q)
  }
  return (members || []).filter((m) => m.user_id && m.user_id !== me && hit(m)).slice(0, limit)
}

// Members sorted for matching: the longest name first, so «@Анна Ким» is not
// read as «@Анна» when both are in the roster.
function byLongestName(members) {
  return (members || [])
    .filter((m) => m.user_id && m.name)
    .slice()
    .sort((a, b) => b.name.length - a.name.length)
}

// The pieces of a message: plain text and `{ mention: member }` chips — every
// «@Name» whose name is a member's, whole (followed by an end, a space or a
// punctuation mark).
export function splitMentions(body, members) {
  const text = String(body || '')
  const roster = byLongestName(members)
  if (!roster.length || !text.includes('@')) return [{ text }]
  const out = []
  let buf = ''
  let i = 0
  while (i < text.length) {
    if (text[i] === '@' && (i === 0 || /[\s([{«"']/.test(text[i - 1]))) {
      const rest = text.slice(i + 1)
      const m = roster.find(
        (r) =>
          rest.toLowerCase().startsWith(r.name.toLowerCase()) &&
          !/[\p{L}\p{N}_]/u.test(rest[r.name.length] || ''),
      )
      if (m) {
        if (buf) out.push({ text: buf })
        buf = ''
        out.push({ text: `@${rest.slice(0, m.name.length)}`, mention: m })
        i += 1 + m.name.length
        continue
      }
    }
    buf += text[i]
    i += 1
  }
  if (buf) out.push({ text: buf })
  return out
}

// The ids a message mentions, in order, once each — what the server is told to
// notify. Read off the same chips the line will show: a chip on screen is the
// promise that the person was told.
export function mentionIds(body, members) {
  const ids = []
  for (const p of splitMentions(body, members))
    if (p.mention && !ids.includes(p.mention.user_id)) ids.push(p.mention.user_id)
  return ids
}

// The board a task typed into the chat lands on by default: the call's own (the
// linked task's board, else its stage's project), else the board the reader was
// last on, else the first.
export function defaultTaskBoard(boardIds, { preferred = [] } = {}) {
  const all = boardIds || []
  return preferred.find((id) => id && all.includes(id)) || all[0] || null
}

// The link of a task line: the board's canonical address with the task open.
export function taskLineHref(task) {
  if (!task?.project_slug || !task?.board_slug || task.number == null) return null
  return `/project/${task.project_slug}/board/${task.board_slug}?task=${task.number}`
}
