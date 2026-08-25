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
//
// A seeded column also carries a name_key (#2800), which says which of the four
// defaults it is without going through the name at all — so it decides first, and
// the name-matching above stays for every column that has no key: the ones a user
// added or renamed.
export function columnStatusName({ isDone, first, name, nameKey }) {
  if (isDone) return 'status-done'
  if (first) return 'status-todo'
  if (nameKey) return nameKey === 'review' ? 'status-review' : 'status-progress'
  if (isReviewColumn(name)) return 'status-review'
  return 'status-progress'
}
