// Rows of the editor's @ / # / / popup (#3118, PackageJ «@, # и /»): what each
// list shows and in what order. Pure, so the order rules are tested without a
// textarea; MarkdownEditor only renders the result (components/editor/EdSuggest).
//
// Every list is a flat array of `{ head }` section titles and rows; the keyboard
// cursor counts rows only (rowsOf).
import { matchCommands } from '@/utils/commands'
import { columnStatusName } from '@/utils/columnStatus'

// At most this many rows are offered — the popup is a pointer, not a browser.
export const SUGGEST_LIMIT = 8

export const rowsOf = (list) => (list || []).filter((x) => !x.head)

// detectTrigger finds the open @ or # query in the text up to the caret:
// { kind: 'mention' | 'task', start, query } (start = index of the trigger), or
// null. Both open after a line start, whitespace or an opening bracket, so
// «user@mail» and «C#» stay text. A `## heading` closes the # query on its
// second `#`, and the space after it ends any query.
export function detectTrigger(upto, { mentions = true, tasks = true } = {}) {
  const m = /(^|[\s([{«"'])([@#])([^\s@#]*)$/.exec(String(upto || ''))
  if (!m) return null
  const kind = m[2] === '@' ? 'mention' : 'task'
  if ((kind === 'mention' && !mentions) || (kind === 'task' && !tasks)) return null
  return { kind, start: upto.length - m[3].length - 1, query: m[3] }
}

// mentionRows: people in the task first («В задаче», with their part in it),
// then the workspace, then GitLab-only logins — each section filtered by the
// query over the name and the inserted login.
//   items   buildMentionItems rows
//   people  [{ id, role }] — role: 'assignee' | 'author' | 'commenter'
//   labels  { inTask, members, gitlab, role: (role) => string, gitlabAside }
//   groups  member groups (#3074): [{ id, name, member_ids }] — a group row
//           inserts the handles of its people who are among `items`.
export function mentionRows(items, query, people = [], labels = {}, groups = []) {
  const q = String(query || '').toLowerCase()
  const hit = (m) =>
    !q || (m.label || '').toLowerCase().includes(q) || (m.display || '').toLowerCase().includes(q)
  const roleOf = new Map((people || []).map((p) => [p.id, p.role]))
  const inTask = []
  const members = []
  const gitlab = []
  for (const m of items || []) {
    if (!hit(m)) continue
    if (m.gitlab) gitlab.push(m)
    else if (m.id && roleOf.has(m.id)) inTask.push(m)
    else members.push(m)
  }
  // The task's own people in the order of their part: assignees, the author,
  // then whoever wrote in the thread.
  const RANK = { assignee: 0, author: 1, commenter: 2 }
  inTask.sort((a, b) => (RANK[roleOf.get(a.id)] ?? 9) - (RANK[roleOf.get(b.id)] ?? 9))
  const row = (m, aside) => ({
    key: m.gitlab ? `gl:${m.label}` : m.id || m.label,
    item: m,
    name: m.display || m.label,
    login: m.hint || (m.gitlab ? `@${m.label}` : ''),
    aside: aside || '',
    avatarUserId: m.avatarUserId || '',
    avatarSrc: m.avatarSrc || null,
  })
  let left = SUGGEST_LIMIT
  const out = []
  const section = (head, list, asideOf) => {
    const take = list.slice(0, left)
    if (!take.length) return
    left -= take.length
    if (head) out.push({ head })
    for (const m of take) out.push(row(m, asideOf(m)))
  }
  // Section titles only when there is more than one kind on screen.
  const kinds = [inTask, members, gitlab].filter((l) => l.length).length
  const titled = kinds > 1 || inTask.length > 0
  section(titled ? labels.inTask : '', inTask, (m) => labels.role?.(roleOf.get(m.id)) || '')
  section(titled ? labels.members : '', members, () => '')
  // GitLab logins are always titled: a bare login reads like a typo otherwise.
  section(labels.gitlab, gitlab, () => labels.gitlabAside || '')
  // Groups last, titled: «@Фронтенд» writes out everyone in it, so each person
  // gets their own chip and their own notification — a group grants nothing.
  const byId = new Map((items || []).filter((m) => m.id && !m.gitlab).map((m) => [m.id, m]))
  const groupHits = (groups || [])
    .map((g) => ({ g, people: (g.member_ids || []).map((id) => byId.get(id)).filter(Boolean) }))
    .filter(({ g, people }) => people.length && (!q || (g.name || '').toLowerCase().includes(q)))
    .slice(0, Math.max(left, 0))
  if (groupHits.length) {
    out.push({ head: labels.groups || '' })
    for (const { g, people } of groupHits)
      out.push({
        key: `grp:${g.id}`,
        item: { group: true, labels: people.map((m) => m.label) },
        name: g.name,
        login: labels.groupCount ? labels.groupCount(people.length) : '',
        aside: '',
        avatarUserId: '',
        avatarSrc: null,
      })
  }
  return out
}

// taskStatusIcon: the status glyph of a search hit — the search row carries the
// column's key and name, not the board's column order.
export function taskStatusIcon(t) {
  if (t.completed_at || t.column_name_key === 'done') return 'status-done'
  if (t.column_name_key === 'todo' || t.column_name_key === 'backlog') return 'status-todo'
  return columnStatusName({
    isDone: false,
    first: false,
    name: t.column_name || '',
    nameKey: t.column_name_key || '',
  })
}

// taskRows: the workspace search hits as popup rows — number, title, and
// «project · stage» (or the board when the task has no stage) on a second line.
// The task the editor belongs to is left out: a reference to itself is noise.
export function taskRows(hits, selfId = null) {
  return (hits || [])
    .filter((t) => t && t.id !== selfId && t.number)
    .slice(0, SUGGEST_LIMIT)
    .map((t) => ({
      key: t.id,
      item: t,
      number: t.number,
      title: t.title || '',
      sub: [t.project_name, t.milestone_title || t.board_name].filter(Boolean).join(' · '),
      icon: taskStatusIcon(t),
      color: t.column_color || '',
    }))
}

// The groups of the / list (PackageJ «/ · группы»). A command missing here
// (a new registry key) lands in «Поля», never nowhere.
export const COMMAND_GROUPS = [
  [
    'fields',
    [
      'due',
      'assign',
      'estimate',
      'priority',
      'start',
      'unassign',
      'remove_due',
      'remove_start',
      'remove_estimate',
      'title',
      'tag',
      'untag',
      'milestone',
      'remove_milestone',
    ],
  ],
  ['status', ['move', 'close', 'reopen', 'archive']],
  ['links', ['blocks', 'blocked_by', 'relate', 'duplicates', 'unlink']],
  ['subtasks', ['subtask', 'parent', 'unparent']],
]
// What an empty `/` offers before anything is typed — the mockup's eight.
export const COMMAND_FEATURED = [
  'due',
  'assign',
  'estimate',
  'priority',
  'move',
  'close',
  'blocks',
  'subtask',
]

const groupOf = (key) => COMMAND_GROUPS.find(([, keys]) => keys.includes(key))?.[0] || 'fields'
const orderOf = (key) => {
  for (const [, keys] of COMMAND_GROUPS) {
    const i = keys.indexOf(key)
    if (i >= 0) return i
  }
  return 99
}

// commandRows: the / list grouped (Поля · Статус · Связи · Подзадачи · Свои),
// featured commands first while nothing is typed, then whatever matches.
//   labels  { group: (g) => string, arg: (kind) => string, custom: string }
export function commandRows(items, query, labels = {}) {
  const list = items || []
  const q = String(query || '')
  let picked
  if (!q) {
    const featured = COMMAND_FEATURED.map((k) => list.find((c) => c.builtin && c.key === k)).filter(
      Boolean,
    )
    picked = [...featured, ...list.filter((c) => !c.builtin)]
  } else {
    picked = matchCommands(list, q, 64)
  }
  const builtin = picked.filter((c) => c.builtin)
  const custom = picked.filter((c) => !c.builtin)
  const groups = new Map()
  for (const c of builtin) {
    const g = groupOf(c.key)
    if (!groups.has(g)) groups.set(g, [])
    groups.get(g).push(c)
  }
  const out = []
  let left = SUGGEST_LIMIT
  for (const [g] of COMMAND_GROUPS) {
    const rows = (groups.get(g) || []).sort((a, b) => orderOf(a.key) - orderOf(b.key))
    const take = rows.slice(0, left)
    if (!take.length) continue
    left -= take.length
    out.push({ head: labels.group?.(g) || g })
    for (const c of take) {
      const arg = c.arg && c.arg !== 'none' ? labels.arg?.(c.arg) || '' : ''
      out.push({ key: c.key, item: c, cmd: `/${c.key}`, desc: c.description, aside: arg })
    }
  }
  // A workspace's own commands always get a place, even past the cap: they are
  // the ones nobody remembers.
  if (custom.length) {
    out.push({ head: labels.group?.('custom') || 'custom' })
    for (const c of custom.slice(0, Math.max(1, left)))
      out.push({ key: `c:${c.key}`, item: c, cmd: `/${c.key}`, desc: c.description, custom: true })
  }
  return out
}
