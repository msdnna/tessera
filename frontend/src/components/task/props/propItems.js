// The rows of every property picker (v2, #3118 — mockups PropPicker, PackageJ).
//
// One builder per field, shared by every place the field is edited — the task
// modal's «Свойства», a card's pills, the list's cells and the ⌘K palette's
// field lists — so the four never drift apart: same rows, same data, same order
// (PackageJ «Три вида одного свойства»). A builder takes plain data plus the
// host's `ops` (what picking a row does there) and returns the entries
// kit/PropPicker draws:
//
//   { kind: 'section', title, count }   { kind: 'gap' }   { kind: 'note', text }
//   { kind: 'row', id, label, sub, two, status | dot | avatar | icon, mono,
//     badge, lock, progress, meta, digit, caps, check: 'single'|'multi', on,
//     close, chev, disabled, run }
//
// `close` marks a single choice: the picker shuts once it ran.
import {
  AddOutline,
  ArchiveOutline,
  ArrowForwardOutline,
  ArrowUndoOutline,
  CheckmarkDoneOutline,
  GitNetworkOutline,
  MailOutline,
  OptionsOutline,
  PersonAddOutline,
  PersonRemoveOutline,
  RemoveCircleOutline,
  RemoveOutline,
  TrailSignOutline,
  UnlinkOutline,
} from '@vicons/ionicons5'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { priorityLabel } from '@/utils/priority'
import { columnName } from '@/utils/defaultNames'
import { columnStatusName } from '@/utils/columnStatus'
import { formatEstimate, scaleOptions } from '@/utils/estimation'
import {
  daysLeft,
  milestoneRangeShort,
  sortStages,
  stageProgress,
  stageState,
} from '@/utils/milestones'
import { uiCollator } from '@/i18n'
import { isMac } from '@/utils/keys'

const MOD_ENTER = isMac ? '⌘↵' : 'Ctrl ↵'

// The keys that, pressed again in an open picker, walk to the next field
// (PackageJ «Повтор клавиши поля»): S A P D L, round and round.
export const FIELD_CYCLE = ['status', 'assignees', 'priority', 'due', 'tags']
export function nextField(field) {
  const i = FIELD_CYCLE.indexOf(field)
  return i < 0 ? null : FIELD_CYCLE[(i + 1) % FIELD_CYCLE.length]
}

// Popover widths of the mockup (PackageJ «Спецификация · каркас»).
export const WIDTHS = {
  status: 300,
  priority: 280,
  assignees: 340,
  estimate: 300,
  estimateTime: 320,
  milestone: 360,
  relation: 380,
  parent: 420,
}

const row = (o) => ({ kind: 'row', ...o })
const section = (title, count) => ({ kind: 'section', id: `sec:${title}`, title, count })
const GAP = { kind: 'gap' }

// The four glyphs of a board column (done · first · review · other).
export function columnGlyph(col, cols, doneId) {
  if (!col) return { name: 'status-todo', color: 'var(--t-text3)' }
  const done = doneId ? col.id === doneId : col.id === cols[cols.length - 1]?.id
  return {
    name: columnStatusName({
      isDone: done,
      first: col.id === cols[0]?.id,
      name: col.name,
      nameKey: col.name_key,
    }),
    color: col.color || 'var(--t-text3)',
  }
}

// A task row's glyph for a task not on the open board: its column name and
// whether it is closed are all a cross-board list carries.
export function taskGlyph(task) {
  return {
    name: columnStatusName({
      isDone: !!task.completed_at,
      first: false,
      name: task.column_name,
      nameKey: task.column_name_key,
    }),
    color: task.column_color || 'var(--t-text3)',
  }
}

// ── Статус · S ──────────────────────────────────────────────
// columns — the board's columns in order; counts — tasks per column id.
// `actions` adds «Сдвинуть в …» (]) and «Выполнить» (E).
export function statusItems({
  t,
  columns,
  currentId,
  doneId,
  counts = null,
  completed = false,
  actions = true,
  query = '',
  ops,
}) {
  const cols = columns || []
  const doneCol = doneId ? cols.find((c) => c.id === doneId) : cols[cols.length - 1]
  const ql = query.trim().toLowerCase()
  const items = []
  cols.forEach((c, i) => {
    const name = columnName(c)
    if (ql && !name.toLowerCase().includes(ql)) return
    items.push(
      row({
        id: `col:${c.id}`,
        label: name,
        status: columnGlyph(c, cols, doneId),
        badge: c.id === doneCol?.id ? t('props.status.finishes') : '',
        meta: counts && counts[c.id] != null ? String(counts[c.id]) : '',
        digit: i < 9 ? String(i + 1) : '',
        check: 'single',
        on: c.id === currentId,
        close: true,
        testid: `prop-status-${c.id}`,
        run: () => ops.move(c.id),
      }),
    )
  })
  if (actions && !ql) {
    const at = cols.findIndex((c) => c.id === currentId)
    const next = at >= 0 ? cols[at + 1] : null
    const acts = []
    if (next && ops.next)
      acts.push(
        row({
          id: 'act:next',
          label: t('props.status.shift', { name: columnName(next) }),
          icon: ArrowForwardOutline,
          caps: [']'],
          close: true,
          testid: 'prop-status-next',
          run: () => ops.next(),
        }),
      )
    if (ops.complete)
      acts.push(
        row({
          id: 'act:complete',
          label: completed ? t('props.status.reopen') : t('props.status.complete'),
          icon: completed ? ArrowUndoOutline : CheckmarkDoneOutline,
          caps: ['E'],
          close: true,
          testid: 'prop-status-complete',
          run: () => ops.complete(),
        }),
      )
    if (acts.length) items.push(GAP, section(t('props.status.actions')), ...acts)
  }
  return { items, cursor: currentId ? `col:${currentId}` : '' }
}

// ── Приоритет · P ── 4…0, the same digits as P+digit.
export function priorityItems({ current = 0, ops }) {
  const items = [4, 3, 2, 1, 0].map((p) =>
    row({
      id: `prio:${p}`,
      label: priorityLabel(p),
      dot: PRIORITY_COLORS[p],
      digit: String(p),
      check: 'single',
      on: (current || 0) === p,
      close: true,
      testid: `prop-priority-${p}`,
      run: () => ops.set(p),
    }),
  )
  return { items, cursor: `prio:${current || 0}` }
}

// ── Исполнители · A ─────────────────────────────────────────
// members — the workspace's [{ user_id, name, email }]; assigned — Set of user
// ids; recent — MRU user ids; gitlab — the GitLab half of the picker
// ([{ gl_username, gl_name, gl_avatar_url, tessera_user_id }]); glAssigned — Set
// of GitLab logins on the task; me — the reader's user id; load — open tasks per
// user id on this board («6 в работе»).
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
export function assigneeItems({
  t,
  query = '',
  members = [],
  assigned = new Set(),
  recent = [],
  gitlab = [],
  glAssigned = new Set(),
  me = '',
  load = null,
  ops,
}) {
  const ql = query.trim().toLowerCase()
  const cmp = uiCollator()
  const byName = (a, b) => cmp.compare(a.name || a.email || '', b.name || b.email || '')
  const hit = (...xs) => !ql || xs.some((x) => (x || '').toLowerCase().includes(ql))
  const loadOf = (uid) => (load && load[uid] ? t('props.assignees.load', { n: load[uid] }) : '')
  const memberRow = (m) =>
    row({
      id: `u:${m.user_id}`,
      label: m.name || m.email,
      sub: m.name ? m.email : '',
      avatar: { userId: m.user_id, name: m.name || m.email },
      meta: loadOf(m.user_id),
      check: 'multi',
      on: assigned.has(m.user_id),
      testid: `prop-assignee-${m.user_id}`,
      run: () => ops.toggle(m.user_id),
    })
  const glRow = (g) =>
    row({
      id: `gl:${g.gl_username}`,
      label: g.gl_name || g.gl_username,
      sub: `@${g.gl_username}`,
      avatar: { src: g.gl_avatar_url, name: g.gl_name || g.gl_username, ext: true },
      meta: g.tessera_user_id ? '' : t('props.assignees.notInTessera'),
      check: 'multi',
      on: glAssigned.has(g.gl_username),
      testid: `prop-assignee-gl-${g.gl_username}`,
      run: () => ops.toggleGl(g),
    })

  const items = []
  const meRow = members.find((m) => m.user_id === me)
  if (meRow && !assigned.has(me) && !ql)
    items.push(
      row({
        id: 'self',
        label: t('props.assignees.self'),
        icon: PersonAddOutline,
        meta: meRow.name || meRow.email,
        caps: ['↵'],
        testid: 'prop-assignee-self',
        run: () => ops.toggle(me),
      }),
    )

  const on = members.filter((m) => assigned.has(m.user_id) && hit(m.name, m.email)).sort(byName)
  const glOn = gitlab.filter((g) => glAssigned.has(g.gl_username) && hit(g.gl_name, g.gl_username))
  if (on.length || glOn.length) {
    items.push(section(t('props.assignees.assigned'), on.length + glOn.length))
    items.push(...on.map(memberRow), ...glOn.map(glRow))
  }
  const rest = members.filter((m) => !assigned.has(m.user_id) && hit(m.name, m.email))
  const recentRows = ql
    ? []
    : recent
        .map((id) => rest.find((m) => m.user_id === id))
        .filter(Boolean)
        .slice(0, 3)
  if (recentRows.length) {
    items.push(section(t('props.assignees.recent')))
    items.push(...recentRows.map(memberRow))
  }
  const recentSet = new Set(recentRows.map((m) => m.user_id))
  const others = rest.filter((m) => !recentSet.has(m.user_id)).sort(byName)
  if (others.length) {
    items.push(section(t('props.assignees.members'), ql ? '' : members.length))
    items.push(...others.map(memberRow))
  }
  const glOff = gitlab.filter(
    (g) => !glAssigned.has(g.gl_username) && hit(g.gl_name, g.gl_username),
  )
  if (glOff.length) {
    items.push(section(t('props.assignees.gitlab')))
    items.push(...glOff.map(glRow))
  }
  const anyRow = items.some((i) => i.kind === 'row' && i.id !== 'self')
  let empty = null
  if (ql && !anyRow) {
    const email = query.trim()
    const mail = EMAIL_RE.test(email)
    empty = {
      title: t('props.assignees.none'),
      sub: mail ? t('props.assignees.noneMail') : t('props.assignees.noneName'),
    }
    if (mail && ops.invite)
      items.push(
        row({
          id: 'invite',
          label: t('props.assignees.invite', { email }),
          icon: MailOutline,
          meta: t('props.assignees.inviteWhere'),
          caps: ['↵'],
          close: true,
          testid: 'prop-assignee-invite',
          run: () => ops.invite(email),
        }),
      )
  }
  if ((assigned.size || glAssigned.size) && !ql && ops.clear)
    items.push(
      GAP,
      row({
        id: 'clear',
        label: t('props.assignees.clear'),
        icon: PersonRemoveOutline,
        caps: ['⇧', '⌫'],
        testid: 'prop-assignee-clear',
        run: () => ops.clear(),
      }),
    )
  return { items, cursor: '', empty }
}

// ── Оценка · O ──────────────────────────────────────────────
// Points: «Не задана» (0) and the scale (1–7…). Time / custom: the rows under
// the field — the picker's slot draws the field and its chips. Both end with
// «Σ подзадач» (puts the sum in) and the door to the project's scale.
export function estimateItems({ t, cfg, value = null, rollup = null, ops }) {
  const points = cfg?.unit === 'points'
  const items = []
  if (points) {
    items.push(
      row({
        id: 'est:none',
        label: t('props.estimate.unset'),
        icon: RemoveOutline,
        digit: '0',
        check: 'single',
        on: value == null,
        close: true,
        testid: 'prop-estimate-none',
        run: () => ops.set(null),
      }),
    )
    const tshirt = cfg?.points_scale === 'tshirt'
    scaleOptions(cfg).forEach((o, i) =>
      items.push(
        row({
          id: `est:${o.value}`,
          label: o.label,
          sub: tshirt || Array.isArray(cfg?.values) ? '' : t('props.estimate.points', o.value),
          digit: i < 9 ? String(i + 1) : '',
          check: 'single',
          on: value === o.value,
          close: true,
          testid: `prop-estimate-${o.value}`,
          run: () => ops.set(o.value),
        }),
      ),
    )
  }
  const tail = []
  if (rollup != null && rollup > 0)
    tail.push(
      row({
        id: 'est:rollup',
        label: t('props.estimate.rollup', { value: formatEstimate(rollup, cfg) }),
        icon: GitNetworkOutline,
        close: true,
        testid: 'prop-estimate-rollup',
        run: () => ops.set(rollup),
      }),
    )
  if (ops.openScale)
    tail.push(
      row({
        id: 'est:scale',
        label: t('props.estimate.scale'),
        icon: OptionsOutline,
        meta: t('props.estimate.scaleWhere'),
        chev: true,
        close: true,
        testid: 'prop-estimate-scale',
        run: () => ops.openScale(),
      }),
    )
  if (tail.length) items.push(GAP, ...tail)
  return {
    items,
    cursor: points ? (value == null ? 'est:none' : `est:${value}`) : '',
  }
}

// ── Этап · M ────────────────────────────────────────────────
// milestones — the project's stages (with the task_count / done_count rollups
// and gl_url); current — the task's milestone id; showClosed — «Закрытые»
// unfolded.
export function milestoneItems({
  t,
  query = '',
  milestones = [],
  current = null,
  showClosed = false,
  formatters,
  ops,
}) {
  const ql = query.trim().toLowerCase()
  const all = sortStages(milestones)
  const match = (m) => !ql || (m.title || '').toLowerCase().includes(ql)
  const byState = { active: [], planned: [], closed: [] }
  for (const m of all) if (match(m)) byState[stageState(m)].push(m)
  const items = []
  if (!ql)
    items.push(
      row({
        id: 'ms:none',
        label: t('props.milestone.none'),
        icon: RemoveCircleOutline,
        digit: '0',
        check: 'single',
        on: !current,
        close: true,
        testid: 'prop-milestone-none',
        run: () => ops.set(null),
      }),
    )
  let gl = false
  const stageRow = (m, state) => {
    const locked = !!m.gl_url
    if (locked) gl = true
    const range = formatters ? milestoneRangeShort(m, formatters) : ''
    const left = state === 'active' ? daysLeft(m) : null
    const sub = locked
      ? t('props.milestone.fromGitlab')
      : [range, left != null && left >= 0 ? t('props.milestone.left', left) : '']
          .filter(Boolean)
          .join(' · ')
    const p = stageProgress(m)
    return row({
      id: `ms:${m.id}`,
      label: m.title,
      sub,
      two: !!sub,
      icon: state === 'closed' ? ArchiveOutline : TrailSignOutline,
      lock: locked,
      progress: state === 'active' && p.total ? { done: p.done, total: p.total } : null,
      check: 'single',
      on: m.id === current,
      close: true,
      testid: `prop-milestone-${m.id}`,
      run: () => ops.set(m.id),
    })
  }
  if (byState.active.length) {
    items.push(section(t('props.milestone.active'), byState.active.length))
    items.push(...byState.active.map((m) => stageRow(m, 'active')))
  }
  if (byState.planned.length) {
    items.push(section(t('props.milestone.planned'), byState.planned.length))
    items.push(...byState.planned.map((m) => stageRow(m, 'planned')))
  }
  if (byState.closed.length) {
    if (ql || showClosed) {
      items.push(section(t('props.milestone.closed'), byState.closed.length))
      items.push(...byState.closed.map((m) => stageRow(m, 'closed')))
    } else {
      // The task's own closed stage stays in sight even folded.
      const own = byState.closed.find((m) => m.id === current)
      if (own) items.push(stageRow(own, 'closed'))
      items.push(
        row({
          id: 'ms:closed',
          label: t('props.milestone.closed'),
          icon: ArchiveOutline,
          meta: String(byState.closed.length),
          chev: true,
          testid: 'prop-milestone-closed',
          run: () => ops.toggleClosed?.(),
        }),
      )
    }
  }
  if (ops.create)
    items.push(
      GAP,
      row({
        id: 'ms:create',
        label: ql
          ? t('props.milestone.createNamed', { name: query.trim() })
          : t('props.milestone.create'),
        icon: AddOutline,
        caps: [MOD_ENTER],
        testid: 'prop-milestone-create',
        run: () => ops.create(query.trim()),
      }),
    )
  if (gl) items.push({ kind: 'note', id: 'ms:gl', text: t('props.milestone.gitlabNote') })
  return { items, cursor: current ? `ms:${current}` : 'ms:none' }
}

// ── Родитель · U ────────────────────────────────────────────
// boardTasks — top-level tasks of the task's board; projectTasks — every task
// of the workspace (with subtasks) that the project lists, carrying board_id /
// board_name / column_name; parent — the current parent (or null); excluded —
// ids never offered (the task itself and its descendants); recent — MRU ids.
export function parentItems({
  t,
  query = '',
  boardId = '',
  boardTasks = [],
  projectTasks = [],
  columns = [],
  doneId = null,
  parent = null,
  excluded = new Set(),
  recent = [],
  ops,
}) {
  const ql = query.trim().toLowerCase().replace(/^#/, '')
  const match = (x) =>
    !ql || (x.title || '').toLowerCase().includes(ql) || String(x.number || '').startsWith(ql)
  const skip = (x) => !x || excluded.has(x.id) || x.id === parent?.id
  const colOf = (x) => columns.find((c) => c.id === x.column_id)
  const boardRow = (x) =>
    row({
      id: `p:${x.id}`,
      label: x.title,
      mono: x.number ? `#${x.number}` : '',
      status: columnGlyph(colOf(x), columns, doneId),
      meta: colOf(x) ? columnName(colOf(x)) : '',
      close: true,
      testid: `prop-parent-${x.id}`,
      run: () => ops.set(x.id),
    })
  const otherRow = (x) =>
    row({
      id: `p:${x.id}`,
      label: x.title,
      mono: x.number ? `#${x.number}` : '',
      status: taskGlyph(x),
      meta: x.board_name || '',
      close: true,
      testid: `prop-parent-${x.id}`,
      run: () => ops.set(x.id),
    })
  const items = []
  if (parent && !ql) {
    items.push(section(t('props.parent.current')))
    items.push(
      row({
        id: `p:${parent.id}`,
        label: parent.title || '…',
        mono: parent.number ? `#${parent.number}` : '',
        status: parent.column_name
          ? taskGlyph(parent)
          : { name: 'status-todo', color: 'var(--t-text3)' },
        check: 'single',
        on: true,
        close: true,
        testid: 'prop-parent-current',
        run: () => ops.open?.(parent.id),
      }),
      row({
        id: 'p:detach',
        label: t('props.parent.detach'),
        icon: UnlinkOutline,
        caps: ['⌫'],
        close: true,
        testid: 'prop-parent-detach',
        run: () => ops.detach(),
      }),
    )
  }
  // The board's own list may be older than the project's (a task made a
  // moment ago in another tab): both, once each.
  const seen = new Set()
  const boardAll = [...boardTasks, ...projectTasks.filter((x) => x.board_id === boardId)].filter(
    (x) => !seen.has(x.id) && seen.add(x.id),
  )
  const onBoard = boardAll.filter((x) => !skip(x) && match(x))
  const shownIds = new Set()
  const cap = ql ? 30 : 8
  if (onBoard.length) {
    items.push(section(t('props.parent.board')))
    for (const x of onBoard.slice(0, cap)) {
      shownIds.add(x.id)
      items.push(boardRow(x))
    }
  }
  const elsewhere = projectTasks.filter((x) => x.board_id !== boardId)
  if (!ql) {
    const byId = new Map(projectTasks.map((x) => [x.id, x]))
    const rec = recent
      .map((id) => byId.get(id))
      .filter((x) => x && !skip(x) && !shownIds.has(x.id))
      .slice(0, 3)
    if (rec.length) {
      items.push(section(t('props.parent.recent')))
      for (const x of rec) {
        shownIds.add(x.id)
        items.push(x.board_id === boardId ? boardRow(x) : otherRow(x))
      }
    }
  }
  const other = elsewhere.filter((x) => !skip(x) && match(x) && !shownIds.has(x.id))
  if (other.length) {
    items.push(section(t('props.parent.project')))
    items.push(...other.slice(0, ql ? 30 : 5).map(otherRow))
  }
  items.push({ kind: 'note', id: 'p:note', text: t('props.parent.note') })
  return { items, cursor: parent && !ql ? 'p:detach' : '' }
}

// Every descendant of a task, from a flat list carrying parent_id — they can't
// become its parent (a cycle).
export function descendantsOf(id, tasks) {
  const kids = new Map()
  for (const x of tasks || []) {
    if (!x.parent_id) continue
    if (!kids.has(x.parent_id)) kids.set(x.parent_id, [])
    kids.get(x.parent_id).push(x.id)
  }
  const out = new Set([id])
  const stack = [id]
  while (stack.length) {
    const cur = stack.pop()
    for (const k of kids.get(cur) || []) {
      if (out.has(k)) continue
      out.add(k)
      stack.push(k)
    }
  }
  return out
}

// ── MRU lists in localStorage (non-critical UX state) ──
export const RECENT_ASSIGNEES_KEY = 'tessera_recent_assignees'
export const RECENT_PARENTS_KEY = 'tessera_recent_parents'
export function readRecent(key) {
  try {
    const v = JSON.parse(localStorage.getItem(key) || '[]')
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}
export function pushRecent(key, id, cap = 30) {
  const next = [id, ...readRecent(key).filter((x) => x !== id)].slice(0, cap)
  try {
    localStorage.setItem(key, JSON.stringify(next))
  } catch {
    /* storage disabled — non-fatal */
  }
  return next
}

// The footers of the mockup, field by field.
export function footerFor(field, t, { search = false, multi = false, n = 5 } = {}) {
  const pick = { caps: ['↑↓'], label: t('props.foot.pick') }
  const esc = { caps: ['Esc'] }
  switch (field) {
    case 'status':
      return [
        pick,
        { caps: [n > 1 ? `1–${Math.min(9, n)}` : '1'], label: t('props.foot.column') },
        '|',
        { caps: ['↵'], label: t('props.foot.apply') },
        esc,
      ]
    case 'priority':
      return [
        pick,
        { caps: ['0–4'], label: t('props.foot.priority') },
        '|',
        { caps: ['↵'], label: t('props.foot.apply') },
        esc,
      ]
    case 'assignees':
      return [
        pick,
        { caps: ['Space'], label: t('props.foot.toggle') },
        '|',
        { caps: ['↵'], label: t('props.foot.done') },
        esc,
      ]
    case 'estimatePoints':
      return [
        pick,
        { caps: ['0–7'], label: t('props.foot.value') },
        '|',
        { caps: ['↵'], label: t('props.foot.apply') },
        esc,
      ]
    case 'estimateTime':
      return [
        { caps: ['↵'], label: t('props.foot.apply') },
        { caps: ['Tab'], label: t('props.foot.chips') },
        '|',
        { caps: ['⌫'], label: t('props.foot.clear') },
        esc,
      ]
    case 'milestone':
      return [
        pick,
        { caps: ['0'], label: t('props.foot.noStage') },
        '|',
        { caps: ['↵'], label: t('props.foot.apply') },
        { caps: [MOD_ENTER], label: t('props.foot.create') },
      ]
    case 'parent':
      return [pick, '|', { caps: ['↵'], label: t('props.foot.makeSub') }, esc]
    case 'parentSet':
      return [
        pick,
        '|',
        { caps: ['↵'], label: t('props.foot.apply') },
        { caps: ['⌫'], label: t('props.foot.detach') },
      ]
    case 'relation':
      return [
        pick,
        { caps: [isMac ? '⌥1–4' : 'Alt 1–4'], label: t('props.foot.kind') },
        '|',
        { caps: ['↵'], label: t('props.foot.link') },
        esc,
      ]
    case 'readonly':
      return [{ caps: ['Esc'], label: t('props.foot.close') }]
    default:
      return search || multi
        ? [pick, '|', esc]
        : [pick, '|', { caps: ['↵'], label: t('props.foot.apply') }, esc]
  }
}
