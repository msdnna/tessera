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
const IGNORED = new Set([
  // Not about this board's contents.
  'note.created',
  'note.updated',
  'note.deleted',
  'notification',
  'workspace.updated',
  'workspace.estimation',
  'group.created',
  'group.updated',
  'group.deleted',
  'group.moved',
  'project.created',
  'project.updated',
  'project.deleted',
  'project.moved',
  'project.estimation',
  'board.created',
  'board.deleted',
  // Handled by their own branch in the board's event handler, and neither needs
  // the board itself re-fetched.
  'workspace_commands.updated',
  'gitlab.conflict',
])

const META = new Set(['tag.created', 'tag.updated', 'tag.deleted', 'tag_prefixes.updated'])

// Milestone CRUD has its own reloadMilestones() branch; the board's grouping
// recomputes from that list via its watcher.
const MILESTONE_PREFIX = 'milestone.'

export function classifyEvent(ev, { boardId } = {}) {
  const type = ev?.type
  if (typeof type !== 'string') return 'ignore'
  if (IGNORED.has(type) || type.startsWith(MILESTONE_PREFIX)) return 'ignore'
  if (META.has(type)) return 'meta'
  if (type === 'integration.sync') return 'meta'
  if (type.startsWith('column.')) return 'columns'
  if (type === 'board.updated') return 'board'

  // Events that name a board tell us straight away whether they concern us.
  // (Only some do: the id-only payloads carry no board at all.)
  const evBoard = ev?.data?.board_id
  if (evBoard && boardId && evBoard !== boardId) return 'ignore'

  if (type.startsWith('task.')) {
    return isFullTask(ev?.data) ? 'patch' : 'tasks'
  }
  // Unknown type: re-fetch the tasks. A new event type added on the backend must
  // not silently stop updating the board — that failure would be invisible.
  return 'tasks'
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
