// Routing for realtime events on an open board.
//
// The board used to answer EVERY workspace event with a full reload — 10 HTTP
// requests (4 for the board + 6 in loadWorkspaceMeta) — even though the backend
// broadcasts 40+ event types into the workspace scope, most of which cannot
// affect the open board at all (a note, a notification, a project rename).
//
// These are pure functions on purpose: the routing table is the part worth
// testing, and it must not need a mounted KanbanBoard to be checked.

// What the board has to do about an event:
//   ignore  — nothing (either unrelated, or already handled by its own branch)
//   patch   — the event carries a full task object; merge it locally, no requests
//   tasks   — re-fetch the task lists only
//   columns — re-fetch the column list only
//   board   — re-fetch the board record only
//   meta    — re-fetch the workspace meta only (tags, members, milestones, …)
//   column-purge — re-fetch columns AND tasks AND the board (see partialKinds)
const IGNORED = new Set([
  // Not about this board's contents.
  'note.created',
  'note.updated',
  'note.deleted',
  'notification',
  'workspace.updated',
  // The two estimation events do concern the board — they decide what its
  // estimate chips mean — but not by way of a re-fetch: AppLayout patches the
  // workspaces store and every consumer re-resolves through it (#2932).
  'workspace.estimation',
  'group.created',
  'group.updated',
  'group.deleted',
  'group.moved',
  'project.created',
  'project.updated',
  'project.deleted',
  'project.moved',
  'project.estimation', // see workspace.estimation above
  'board.created',
  // Handled by their own branch in the board's event handler, and neither needs
  // the board itself re-fetched.
  'workspace_commands.updated',
  'gitlab.conflict',
  // This board being deleted is not something a re-fetch can answer — there is
  // nothing left to fetch. The board's own branch compares the id and leaves for
  // Home; every other board's deletion really is none of our business (#2932).
  'board.deleted',
  // A link between two tasks changed. No card carries a relation (the board rows
  // have no such field), so a task-list re-fetch would buy nothing; the only
  // consumer is the Gantt's edge list, which the board pokes by its own branch
  // (#2932).
  'task.relation',
])

const META = new Set(['tag.created', 'tag.updated', 'tag.deleted', 'tag_prefixes.updated'])

// Milestone CRUD has its own reloadMilestones() branch; the board's grouping
// recomputes from that list via its watcher.
const MILESTONE_PREFIX = 'milestone.'

export function classifyEvent(ev, { boardId, hasTask } = {}) {
  const type = ev?.type
  if (typeof type !== 'string') return 'ignore'
  if (IGNORED.has(type) || type.startsWith(MILESTONE_PREFIX)) return 'ignore'
  if (META.has(type)) return 'meta'
  if (type === 'integration.sync') return 'meta'
  // Deleting a column takes every card standing in it with it (the server
  // cascades through tasks.column_id), yet it broadcasts a single
  // `column.deleted` — no task events at all. Re-reading just the column list
  // would leave those cards in `allTasks`: invisible when grouped by status
  // (their column_id no longer matches any rendered column), but fully alive in
  // the tag/milestone groupings and in the list/calendar/gantt views, which
  // bucket every task regardless of its column. Hence its own kind (#2932).
  if (type === 'column.deleted') return 'column-purge'
  if (type.startsWith('column.')) return 'columns'
  if (type === 'board.updated') return 'board'

  // Events that name a board tell us straight away whether they concern us.
  // (Only some do: the id-only payloads carry no board at all.)
  //
  // One exception, and it is the whole reason `hasTask` exists: a card LEAVING
  // this board is announced with the task as it is AFTER the move, so the event
  // names the board it went to — indistinguishable here from a stranger's. Both
  // doors onto another board (TransferTask from the modal's breadcrumb, and
  // setParent onto a card that lives elsewhere — backend task_ops.go applyParent)
  // broadcast exactly that `task.moved`. Ignoring it leaves the card, and its
  // whole subtree, drawn on a board it no longer belongs to for everyone but the
  // person who moved it: dragging it answers 400 ("column does not belong to this
  // board"), and opening it shows another board's task. So when the event names a
  // task we are still holding, re-read the lists — that is the card going away (#2932).
  const evBoard = ev?.data?.board_id
  if (evBoard && boardId && evBoard !== boardId) {
    const id = ev?.data?.id
    if (type.startsWith('task.') && typeof id === 'string' && hasTask?.(id)) return 'tasks'
    return 'ignore'
  }

  if (type.startsWith('task.')) {
    return isFullTask(ev?.data) ? 'patch' : 'tasks'
  }
  // Unknown type: re-fetch the tasks. A new event type added on the backend must
  // not silently stop updating the board — that failure would be invisible.
  return 'tasks'
}

// The partial re-fetches a classification asks for, in the order they should be
// scheduled. Every kind but `column-purge` maps to the one fetch it names; a
// purge needs the columns (the deleted one must go), the tasks (its cards died
// with it) and the board record — `boards.done_column_id` is an ON DELETE SET
// NULL reference, so deleting the completing column silently leaves the board
// without one, and a stale copy would keep pointing at a column that is gone.
const PARTIAL_KINDS = { 'column-purge': ['columns', 'tasks', 'board'] }

export function partialKinds(kind) {
  return PARTIAL_KINDS[kind] || [kind]
}

// Does a `task.relation` event change the arrows drawn on this board?
//
// The backend returns an edge only when BOTH its endpoints sit on the queried
// board, so a pair with one end here is still worth a re-read: that is exactly
// the event that takes an arrow away (a task re-parented onto another board) or
// brings one back. A pair with neither end here cannot move anything.
export function relationTouchesBoard(ev, boardId) {
  if (ev?.type !== 'task.relation' || !boardId) return false
  return ev?.data?.board_id === boardId || ev?.data?.related_board_id === boardId
}

// Is this event the open board's own retirement?
//
// `board.deleted` carries a bare `{id}`, and the board it names is usually not
// ours — the event is workspace-scoped, so every board in the workspace hears
// about every other. Only the match matters, and when it matches there is
// nothing to re-fetch: the server took the columns, the cards and their uploads
// with it (#2932).
export function boardGone(ev, boardId) {
  if (ev?.type !== 'board.deleted' || !boardId) return false
  return ev?.data?.id === boardId
}

// A full task payload (as opposed to `{id}` / `{task_id}`) is recognised by the
// fields the board actually needs to place a card.
export function isFullTask(data) {
  return !!(
    data &&
    typeof data === 'object' &&
    typeof data.id === 'string' &&
    typeof data.column_id === 'string' &&
    typeof data.title === 'string'
  )
}

// Merge a full task payload into an existing board row, returning a NEW object.
//
// Two things this must get right:
//   * `description` — board list payloads deliberately strip it (see backend
//     handlers/task_list_dto.go: `has_description` travels, the text does not),
//     while the event payload carries it in full. Copying it over would push
//     kilobytes of markdown into a card row and desync `has_description`.
//   * merge, not replace — `tag_ids`, `assignee_ids` and the `gitlab_*` columns
//     exist only on the board row, not in the payload. Replacing would drop the
//     card's tags and assignees.
export function mergeTaskRow(row, payload) {
  const out = { ...row }
  for (const [k, v] of Object.entries(payload)) {
    if (k === 'description') continue
    out[k] = v
  }
  if ('description' in payload) {
    out.has_description = !!payload.description
  }
  return out
}

// Apply a full-task event to the board's rows.
//
// Returns the new rows array, or null when the event cannot be applied locally
// and the caller must re-fetch instead:
//   * the task isn't on the board yet (it was just created, moved onto this
//     board, or was previously filtered out server-side by ?milestone=);
//   * its column or position changed, i.e. the card has to move. Card order
//     inside a column comes from the array order, not from `position`, so a
//     silent in-place merge would leave the card sitting in its old slot.
// Same idea for a subtask: patch it inside its parent's child list. Returns a new
// map, or null when the caller must re-fetch (unknown parent, the child isn't
// listed yet, or it moved — parent, column or position changed).
export function applySubtaskPatch(byParent, payload) {
  const pid = payload.parent_id
  if (!pid) return null
  const arr = byParent[pid]
  if (!arr) return null
  const i = arr.findIndex((t) => t.id === payload.id)
  if (i === -1) return null
  const row = arr[i]
  if (payload.column_id !== row.column_id) return null
  if (payload.position !== row.position) return null
  const next = arr.slice()
  next[i] = mergeTaskRow(row, payload)
  return { ...byParent, [pid]: next }
}

export function applyTaskPatch(rows, payload) {
  const i = rows.findIndex((t) => t.id === payload.id)
  if (i === -1) return null
  const row = rows[i]
  if (payload.column_id !== row.column_id) return null
  if (payload.position !== row.position) return null
  const next = rows.slice()
  next[i] = mergeTaskRow(row, payload)
  return next
}
