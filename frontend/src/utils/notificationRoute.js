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
//
// The v2 notifications (#3048) about something other than a task open it: a
// document someone mentioned the reader in, the page of a closed stage (the
// page takes the stage's id in place of its slug; the project slug is the one
// the stage was closed under — a later rename lands on the 404 of the stage
// page, which leads back).
export function notificationRoute(n) {
  const p = n?.payload || {}
  const confId = p.conference_id
  if (confId) return `/conferences/${confId}`
  if (p.document_slug) return `/documents/${encodeURIComponent(p.document_slug)}`
  if (p.milestone_id && p.project_slug)
    return `/project/${encodeURIComponent(p.project_slug)}/stage/${p.milestone_id}`
  if (n?.task_id && n?.task_board_id) return `/board/${n.task_board_id}?task=${n.task_id}`
  return null
}
