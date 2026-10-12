// Shared row builders for the palette (v2, #2957). A row is a plain object the
// list renders and the keyboard drives:
//   { key, icon | status | avatar, title, hl, sub, value, keys, checked,
//     disabled, run(ctx), task? }
// `hl` is the query to highlight in the title (null: none). `task` marks a row
// Tab can narrow the palette to. `run` returns what the palette does next:
// 'close' (default), 'stay', or 'back'.
import { columnStatusName } from '@/utils/columnStatus'
import { taskColumnName } from '@/utils/defaultNames'

// The status glyph of a task row from what the search row carries: no column
// position, so a first column without a name_key reads as «in progress».
export function taskStatus(t) {
  const done = !!t.completed_at || t.column_name_key === 'done'
  return {
    name: columnStatusName({
      isDone: done,
      first: t.column_name_key === 'todo',
      name: t.column_name,
      nameKey: t.column_name_key,
    }),
    color: t.column_color || 'var(--t-text3)',
  }
}

export function taskPath(t) {
  const base =
    t.project_slug && t.board_slug
      ? `/project/${t.project_slug}/board/${t.board_slug}`
      : `/board/${t.board_id}`
  return `${base}?task=${t.number ?? t.id}`
}

// «#2950 · В процессе · Tessera Web / Спринт 0.150»
export function taskSub(t) {
  const parts = []
  if (t.number != null) parts.push(`#${t.number}`)
  const col = taskColumnName(t)
  if (col) parts.push(col)
  const where = [t.project_name, t.milestone_title || t.board_name].filter(Boolean).join(' / ')
  if (where) parts.push(where)
  return parts.join(' · ')
}
