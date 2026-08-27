// Captions the entities whose default names are born on the server (доработки 1–2
// of #2800): the personal workspace every account is seeded with, and the four
// columns every new board is seeded with.
//
// Those names used to arrive as finished Russian strings, so an English UI had
// nothing to translate. The server now sends a stable key next to the string —
// `name_key` — and the caption is drawn here from the reader's catalogue. The
// string stays as the fallback: a key this bundle doesn't know yet (server newer
// than client) reads as a phrase, not as a bare key. Same shape as jobName (#2800)
// and notificationText (#2801).
//
// A name the user chose has no key — the server drops it on rename — so it is
// shown verbatim in every language, which is the whole point of the distinction.
import { i18n } from '@/i18n'

function caption(key, fallback, prefix, { t = i18n.global.t, te = i18n.global.te } = {}) {
  const path = `${prefix}.${key}`
  if (key && te(path)) return t(path)
  return fallback || ''
}

export function workspaceName(ws, opts) {
  if (!ws) return ''
  return caption(ws.name_key, ws.name, 'shell.workspace.defaultName', opts)
}

// A board column as the server sends it ({ name, name_key }).
export function columnName(col, opts) {
  if (!col) return ''
  return caption(col.name_key, col.name, 'board.column.defaultName', opts)
}

// The same column as it travels inside a task row (ListWorkspaceTasks flattens it
// into column_name / column_name_key).
export function taskColumnName(task, opts) {
  if (!task) return ''
  return caption(task.column_name_key, task.column_name, 'board.column.defaultName', opts)
}
