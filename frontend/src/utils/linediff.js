// Inline character-level diff for the conflict resolver: given a base text and a
// changed text, return the changed text split into segments, each flagged whether
// it diverged from the base. Uses the common prefix + common suffix (O(n)), so a
// localized edit (e.g. a digit appended to a paragraph) highlights only that edit,
// not the whole block. Multiple scattered edits collapse into one highlighted span
// between the shared ends — fine for the focused edits a conflict resolver shows.
export function diffSegments(base, cur) {
  const b = String(base ?? '')
  const c = String(cur ?? '')
  if (b === c) return [{ text: c, changed: false }]

  const minLen = Math.min(b.length, c.length)
  let p = 0
  while (p < minLen && b[p] === c[p]) p++
  let s = 0
  while (s < minLen - p && b[b.length - 1 - s] === c[c.length - 1 - s]) s++

  const segs = []
  if (p) segs.push({ text: c.slice(0, p), changed: false })
  const mid = c.slice(p, c.length - s)
  if (mid) segs.push({ text: mid, changed: true })
  if (s) segs.push({ text: c.slice(c.length - s), changed: false })
  return segs
}
