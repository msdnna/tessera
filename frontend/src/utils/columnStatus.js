// Which status glyph a board column gets (#2799).
//
// There is no column type on the server — a "review" column is recognised by its
// name, and column names are user data seeded in Russian (handlers/boards.go),
// which is why this table is NOT interface text and never moves into the message
// catalogue: it has to keep matching the words the rows actually contain, in
// whatever language the workspace was created in. Hence the allowlist entry in
// tests/helpers/ruLiterals.js, next to the transliteration table.
const REVIEW_RE = /рассмотр|ревью|review|проверк/i

export function isReviewColumn(name) {
  return REVIEW_RE.test(name || '')
}

const TODO_RE = /^(к работе|to ?do)$/i

// Is this the «К работе» column, by its seeded key or its name — for rows that
// carry their column but not the board's column order (the start screen, #2960).
export function isTodoColumn(nameKey, name) {
  return nameKey === 'todo' || TODO_RE.test((name || '').trim())
}

// The «К работе» column of a board, where quick-created tasks go (#2956): the
// seeded todo column by its name_key, else one so named, else the first.
// `cols` must be in board order; works on board columns and on display columns
// (which carry the board column as `status` and its name as `rawName`).
export function todoColumn(cols) {
  const list = cols || []
  const col = (c) => c.status || c
  return (
    list.find((c) => col(c).name_key === 'todo') ||
    list.find((c) => TODO_RE.test((col(c).name || c.rawName || '').trim())) ||
    list[0] ||
    null
  )
}

// done → check circle · first → empty circle · review → ⅔-pie · other → half.
//
// A seeded column also carries a name_key (#2800), which says which of the four
// defaults it is without going through the name at all — so it decides first, and
// the name-matching above stays for every column that has no key: the ones a user
// added or renamed.
const REVIEW_KEYS = new Set(['review', 'verify'])

export function columnStatusName({ isDone, first, name, nameKey }) {
  if (isDone) return 'status-done'
  if (first) return 'status-todo'
  // «Проверка» of the bug-tracker template (#3047) is a review column too.
  if (nameKey) return REVIEW_KEYS.has(nameKey) ? 'status-review' : 'status-progress'
  if (isReviewColumn(name)) return 'status-review'
  return 'status-progress'
}

// The status a task shows outside the kanban — the glyph and colour of its
// column, as the column header draws them (the chart rows, the calendar pills).
// `cols` sorted left to right; the done column is the board's own, else the
// rightmost. A task outside the loaded columns reads as done or to-do by its flag.
export function taskStatus(task, cols, doneColumnId) {
  const list = cols || []
  const col = list.find((c) => c.id === task.column_id)
  if (!col) {
    const done = !!task.completed_at
    return { icon: done ? 'status-done' : 'status-todo', color: '', done }
  }
  const isDone = doneColumnId ? col.id === doneColumnId : col.id === list[list.length - 1]?.id
  const done = isDone || !!task.completed_at
  return {
    icon: columnStatusName({
      isDone: done,
      first: col.id === list[0]?.id,
      name: col.name,
      nameKey: col.name_key,
    }),
    color: col.color || '',
    done,
  }
}
