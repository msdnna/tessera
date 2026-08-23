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

// done → check circle · first → empty circle · review → ⅔-pie · other → half.
export function columnStatusName({ isDone, first, name }) {
  if (isDone) return 'status-done'
  if (first) return 'status-todo'
  if (isReviewColumn(name)) return 'status-review'
  return 'status-progress'
}
