// Composer-bar filtering for the board. Filtering is entirely client-side: the
// board loads its top-level tasks and every subtask, and this module decides
// what stays visible.
//
// Key rule (task #2602): a parent card stays on the board when IT matches OR when
// at least one of its subtasks matches — otherwise a subtask assigned to you was
// invisible under an "assignee = me" filter. When only a subtask matched, the
// parent's on-card subtask list is narrowed to the matching children. The task
// modal is untouched and always lists every subtask.

import { matchesAuthor } from './boardFilters'

// Facets that a subtask may "lift" its parent by. `statuses` (the board column)
// is deliberately excluded: a subtask can live in a different column, and letting
// it lift the parent would draw the parent into a column it isn't in. `authors`
// (task #2603) behaves like `assignees` — a subtask by the picked author lifts its
// parent, keeping the author facet consistent with the assignee facet.
export const SUBTASK_FACETS = [
  'q',
  'assignees',
  'authors',
  'tags',
  'priorities',
  'due',
  'milestones',
]

// Due-date predicate for the "Срок" filter. `now` is injectable so tests are
// not clock-dependent.
export function matchesDue(t, mode, now = new Date()) {
  if (mode === 'none') return !t.due_date
  if (!t.due_date) return false
  if (mode === 'has') return true
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const due = new Date(t.due_date)
  const dueDay = new Date(due.getFullYear(), due.getMonth(), due.getDate())
  const dayMs = 86400000
  if (mode === 'overdue') return dueDay < today && !t.completed_at
  if (mode === 'today') return dueDay.getTime() === today.getTime()
  if (mode === 'week') return dueDay >= today && dueDay - today <= 7 * dayMs
  return true
}

function matchesAssignees(t, wanted) {
  const ids = t.assignee_ids || []
  const logins = t.gitlab_assignee_logins || []
  return wanted.some((a) =>
    typeof a === 'string' && a.startsWith('gl:') ? logins.includes(a.slice(3)) : ids.includes(a),
  )
}

function matchesQuery(t, q) {
  return t.title.toLowerCase().includes(q) || (t.number != null && `#${t.number}`.includes(q))
}

// Does one task pass every active facet? `facets` limits which facets are
// considered (subtasks use SUBTASK_FACETS), `now` is passed through to matchesDue,
// `glLoginByUserId` (tessera user id → gl_username) lets the author facet match
// GitLab-synced tasks whose Tessera author is null.
export function matchesTask(
  t,
  filters,
  { facets = null, now = new Date(), glLoginByUserId = {} } = {},
) {
  const on = (f) => !facets || facets.includes(f)
  if (on('priorities') && filters.priorities?.length && !filters.priorities.includes(t.priority))
    return false
  if (on('assignees') && filters.assignees?.length && !matchesAssignees(t, filters.assignees))
    return false
  if (
    on('authors') &&
    filters.authors?.length &&
    !matchesAuthor(t, filters.authors, glLoginByUserId)
  )
    return false
  if (
    on('tags') &&
    filters.tags?.length &&
    !(t.tag_ids || []).some((id) => filters.tags.includes(id))
  )
    return false
  if (on('statuses') && filters.statuses?.length && !filters.statuses.includes(t.column_id))
    return false
  if (
    on('milestones') &&
    filters.milestones?.length &&
    !filters.milestones.includes(t.milestone_id || '__none__')
  )
    return false
  if (on('due') && filters.due && !matchesDue(t, filters.due, now)) return false
  const q = (filters.q || '').trim().toLowerCase()
  if (on('q') && q && !matchesQuery(t, q)) return false
  return true
}

// True when at least one facet a subtask may lift its parent by is active. With
// no such facet the subtask pass is a no-op and we can skip it entirely.
export function hasSubtaskFacets(filters) {
  return SUBTASK_FACETS.some((f) => (f === 'q' ? (filters.q || '').trim() : filters[f]?.length))
}

// Filter a board's tasks and subtasks together.
//
// Returns:
//   tasks            — visible top-level tasks (unsorted; the caller sorts)
//   subtasksByParent — per-parent child lists, narrowed for parents that only
//                      survived because a child matched
//   narrowedParents  — Set of parent ids whose child list was narrowed (the card
//                      shows an "N из M" hint and disables subtask drag-reorder)
export function filterBoardTasks({
  tasks = [],
  subtasksByParent = {},
  filters = {},
  glLoginByUserId = {},
  now,
}) {
  const clock = now || new Date()
  const subFacets = hasSubtaskFacets(filters)

  const outTasks = []
  const outSubs = {}
  const narrowedParents = new Set()

  for (const t of tasks) {
    const subs = subtasksByParent[t.id] || []
    const selfOk = matchesTask(t, filters, { now: clock, glLoginByUserId })
    if (selfOk) {
      outTasks.push(t)
      if (subs.length) outSubs[t.id] = subs
      continue
    }
    if (!subFacets || !subs.length) continue
    // The parent itself failed. It may still be lifted by a matching subtask —
    // but only if the parent passes the facets a subtask cannot stand in for
    // (its board column), otherwise the card would appear in the wrong place.
    if (!matchesTask(t, filters, { facets: ['statuses'], now: clock, glLoginByUserId })) continue
    const hits = subs.filter((s) =>
      matchesTask(s, filters, { facets: SUBTASK_FACETS, now: clock, glLoginByUserId }),
    )
    if (!hits.length) continue
    outTasks.push(t)
    outSubs[t.id] = hits
    if (hits.length < subs.length) narrowedParents.add(t.id)
  }

  return { tasks: outTasks, subtasksByParent: outSubs, narrowedParents }
}
