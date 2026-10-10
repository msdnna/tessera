// Where a notification takes you when you click it.
//
// Two kinds of destination exist, and they are picked in this order:
//
//   - a conference invitation (#2875) points at no task at all — the call's id
//     travels in the payload, because `notifications.task_id` is a foreign key
//     to tasks and a conference is not one. It wins the check because such a row
//     carries no board to fall back on.
//   - a mention in a shared «Входящие» note (#3040) points at the note: it
//     opens the panel on it, staying on the current screen (`?inbox=notes&note=`
//     is what AppLayout reveals a note by — the same address search uses).
//   - everything else is about a task, and opens the board with the task query
//     (BoardView canonicalises /board/:id → the slug URL and opens the modal).
//
// Returns null when the row points nowhere — an integration sync report, or a
// task notification whose board the list endpoint could not join in. The caller
// then only marks it read, which is what clicking such a row did before.
export function notificationRoute(n) {
  const confId = n?.payload?.conference_id
  if (confId) return `/conferences/${confId}`
  const noteId = n?.payload?.note_id
  if (noteId) return `?inbox=notes&note=${encodeURIComponent(noteId)}`
  if (n?.task_id && n?.task_board_id) return `/board/${n.task_board_id}?task=${n.task_id}`
  return null
}
