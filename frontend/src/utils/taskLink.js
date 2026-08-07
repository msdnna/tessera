// taskLink — a shareable URL for a task. The `/board/<board_id>?task=<n>` path is
// self-canonicalizing (BoardView resolves the id to project/board slugs), so the
// link is correct no matter which board/workspace the modal was opened from.
// Prefers the per-workspace number (#252) over the uuid for a readable query, and
// returns null when there is no board to point at (nothing worth copying).
export function taskLink(task, origin = window.location.origin) {
  const boardId = task?.board_id
  if (!boardId) return null
  const ref = task.number ?? task.id
  if (ref == null) return null
  return `${origin}/board/${boardId}?task=${ref}`
}
