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
import { defaultFormatters, epochDay } from './format'
import { NO_MILESTONE } from './facetKeys'

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

// Both bounds and the due day itself are whole calendar days (epoch-day integers),
// never instants offset by milliseconds: a week is seven midnights, not 7×86400000
// ms, and across a DST change those are an hour apart — enough to move a task in or
// out of a window.
//
// The calendar is the USER'S, which is why a formatter set is threaded through every
// predicate here instead of reading the browser clock. A due date is bucketed in the
// same calendar it is printed in, because one pill carries both: TaskCardPills
// renders the caption with formatDue and paints it red with isTaskOverdue. Two rules
// come with that, both already inside `formatters.parts` — the `timezone` preference
// wins over the browser's zone, and a pure UTC midnight is a date-only value (a
// GitLab issue/milestone due) read in UTC in every zone. Buckets computed off
// `new Date().getDate()` ignored both, so a pill could read «вчера» and stay black,
// or read «сегодня» in red.
//
// The week window is half-open, [today, today+7): seven days counting today, the
// same range WorkspaceTaskSummary counts in SQL (`due < week_end`, pinned by
// TestWorkspaceSummaryBuckets) and Android's home filters on. It has to be the
// same one, because on the home screen the server's number is the caption of a
// list this module filters — an eighth day here was a card the count above it
// never included. The server counts in the caller's zone too (callerLocation), so
// passing the same preference here is what keeps the tile and the list in step.
function dueWindow(now, f) {
  const today = epochDay(f.today(now))
  return { today, weekEnd: today + 7 }
}

function dueDayOf(t, f) {
  return epochDay(f.parts(t.due_date))
}

// Is this task late? The one answer the whole app gives — the red due pill on a
// card, the red date in the home list, the Gantt/Timeline counters and the
// «Срок: Просрочено» facet all call this.
//
// It is a calendar-day comparison, not an instant one, because that is what a due
// date means everywhere else: the server buckets the home cards the same way
// (`due < day_start` in WorkspaceTaskSummary, midnights computed in Go), and a due
// date may or may not carry a time of day. Comparing instants made a task due
// *today* go red at 00:00 while the «Просрочено» card counted it as zero, the
// «Сегодня» card counted it as one, and clicking the «Просрочено» facet made the
// red card disappear from the board.
export function isTaskOverdue(t, now = new Date(), f = defaultFormatters()) {
  if (!t?.due_date || t.completed_at) return false
  return dueDayOf(t, f) < dueWindow(now, f).today
}

// Is this task due inside the next `days` calendar days — the urgency axis of the
// Eisenhower matrix (BoardMatrixView), whose default window is a week.
//
// Half-open, [today, today+days), and overdue counts as inside it: something that
// was due yesterday is not less urgent than something due tomorrow. With days = 7
// that is the very window `matchesDue(t, 'week')` uses, which is the point of
// putting it here instead of leaving the matrix its own arithmetic. The matrix
// used to ask `due <= Date.now() + days * 86400000`, an instant comparison, so a
// task due on day+7 landed in «Срочно» whenever its time of day was earlier than
// the current one — always, for a date-only due (a pure UTC midnight reads as
// 00:00) — while the «Срок: неделя» facet on the same board, the home «На неделе»
// tile and the server's SQL count all put it outside the week.
export function isDueWithinDays(t, days, now = new Date(), f = defaultFormatters()) {
  if (!t?.due_date) return false
  return dueDayOf(t, f) < dueWindow(now, f).today + days
}

// Due-date predicate for the "Срок" filter. `now` is injectable so tests are not
// clock-dependent; `f` is the caller's formatter set, whose `timezone` preference
// decides which calendar day the due date and "today" belong to.
export function matchesDue(t, mode, now = new Date(), f = defaultFormatters()) {
  if (mode === 'none') return !t.due_date
  if (!t.due_date) return false
  if (mode === 'has') return true
  const { today, weekEnd } = dueWindow(now, f)
  const dueDay = dueDayOf(t, f)
  if (mode === 'overdue') return dueDay < today && !t.completed_at
  if (mode === 'today') return dueDay === today
  if (mode === 'week') return dueDay >= today && dueDay < weekEnd
  return true
}

// The home screen's due predicate: the same windows as the summary cards above the
// list, minus completed tasks. A closed task is not work that is due, and the cards
// («Просрочено», «Сегодня», «На неделе») count it that way too — see the query.
//
// Kept apart from matchesDue rather than folded into it: the board's "Срок" facet
// is a date filter over cards that include the done column, so hiding completed
// tasks there would be a different feature, not the same one.
export function matchesHomeDue(t, mode, now = new Date(), f = defaultFormatters()) {
  if (!t.due_date || t.completed_at) return false
  const { today, weekEnd } = dueWindow(now, f)
  const dueDay = dueDayOf(t, f)
  if (mode === 'overdue') return dueDay < today
  if (mode === 'today') return dueDay === today
  if (mode === 'week') return dueDay >= today && dueDay < weekEnd
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
// considered (subtasks use SUBTASK_FACETS), `now` and `formatters` are passed
// through to matchesDue, `glLoginByUserId` (tessera user id → gl_username) lets the
// author facet match GitLab-synced tasks whose Tessera author is null.
export function matchesTask(
  t,
  filters,
  { facets = null, now = new Date(), glLoginByUserId = {}, formatters = null } = {},
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
    !filters.milestones.includes(t.milestone_id || NO_MILESTONE)
  )
    return false
  if (
    on('due') &&
    filters.due &&
    !matchesDue(t, filters.due, now, formatters || defaultFormatters())
  )
    return false
  const q = (filters.q || '').trim().toLowerCase()
  if (on('q') && q && !matchesQuery(t, q)) return false
  return true
}

// True when at least one facet a subtask may lift its parent by is active. With
// no such facet the subtask pass is a no-op and we can skip it entirely.
export function hasSubtaskFacets(filters) {
  return SUBTASK_FACETS.some((f) => (f === 'q' ? (filters.q || '').trim() : filters[f]?.length))
}

// True when any facet at all is active. The board spends most of its life with an
// empty composer, and in that state every task trivially passes — so the whole
// per-task predicate pass can be skipped.
export function hasAnyFacets(filters) {
  return hasSubtaskFacets(filters) || !!filters.statuses?.length
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
  formatters = null,
  now,
}) {
  const clock = now || new Date()
  const fmt = formatters || defaultFormatters()

  // Empty composer: everything passes and nothing is narrowed. Shaped exactly like
  // the general path below (fresh arrays, child lists only for parents that have
  // children), just without running the predicate over every task and subtask.
  if (!hasAnyFacets(filters)) {
    const outSubs = {}
    for (const t of tasks) {
      const subs = subtasksByParent[t.id]
      if (subs?.length) outSubs[t.id] = subs
    }
    return { tasks: tasks.slice(), subtasksByParent: outSubs, narrowedParents: new Set() }
  }

  const subFacets = hasSubtaskFacets(filters)

  const outTasks = []
  const outSubs = {}
  const narrowedParents = new Set()

  for (const t of tasks) {
    const subs = subtasksByParent[t.id] || []
    const selfOk = matchesTask(t, filters, { now: clock, glLoginByUserId, formatters: fmt })
    if (selfOk) {
      outTasks.push(t)
      if (subs.length) outSubs[t.id] = subs
      continue
    }
    if (!subFacets || !subs.length) continue
    // The parent itself failed. It may still be lifted by a matching subtask —
    // but only if the parent passes the facets a subtask cannot stand in for
    // (its board column), otherwise the card would appear in the wrong place.
    if (
      !matchesTask(t, filters, {
        facets: ['statuses'],
        now: clock,
        glLoginByUserId,
        formatters: fmt,
      })
    )
      continue
    const hits = subs.filter((s) =>
      matchesTask(s, filters, {
        facets: SUBTASK_FACETS,
        now: clock,
        glLoginByUserId,
        formatters: fmt,
      }),
    )
    if (!hits.length) continue
    outTasks.push(t)
    outSubs[t.id] = hits
    if (hits.length < subs.length) narrowedParents.add(t.id)
  }

  return { tasks: outTasks, subtasksByParent: outSubs, narrowedParents }
}
