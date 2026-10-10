// Where a notification takes you when you click it.
//
// Two kinds of destination exist, and they are picked in this order:
//
//   - a conference invitation (#2875) points at no task at all — the call's id
//     travels in the payload, because `notifications.task_id` is a foreign key
//     to tasks and a conference is not one. It wins the check because such a row
//     carries no board to fall back on.
//   - everything else is about a task, and opens the board with the task query
//     (BoardView canonicalises /board/:id → the slug URL and opens the modal).
//
// Returns null when the row points nowhere — an integration sync report, or a
// task notification whose board the list endpoint could not join in. The caller
// then only marks it read, which is what clicking such a row did before.
//   - the members notifications (#3063) point at a workspace: an approved access
//     request opens the space it let you into, a transfer and a new request
//     open the «Участники» window (`?members=` — AppLayout opens it).
export function notificationRoute(n) {
  const confId = n?.payload?.conference_id
  if (confId) return `/conferences/${confId}`
  const ev = n?.payload?.event
  if (ev === 'access_approved' && n.payload.workspace_id) return `/space/${n.payload.workspace_id}`
  if ((ev === 'access_requested' || ev === 'workspace_owner_transferred') && n.workspace_id)
    return { path: '/', query: { members: n.workspace_id } }
  if (n?.task_id && n?.task_board_id) return `/board/${n.task_board_id}?task=${n.task_id}`
  return null
}
