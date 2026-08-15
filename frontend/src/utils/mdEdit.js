// Pure text operations behind the markdown editor's keyboard handling. Each takes
// the current textarea value plus its selection and returns `{ value, start, end }`
// for the next state — the fiddly cases (multi-line indent, outdent that must not
// run past column 0, ordered-list numbering) are then unit-testable without
// mounting a textarea. Returning `null` means "nothing to do".

// Typing one of these characters with text selected wraps the selection instead of
// replacing it (the browser default destroys it). Brackets close with their pair,
// the rest are symmetric.
export const WRAP_PAIRS = {
  '(': ')',
  '[': ']',
  '{': '}',
  '<': '>',
  '"': '"',
  "'": "'",
  '`': '`',
  '*': '*',
  _: '_',
  '~': '~',
}

// Two spaces: markdown list nesting in this project counts by two, and the
// textarea is monospace so the indent lines up with the rendered nesting.
export const INDENT = '  '

export function wrapSelection(value, start, end, ch) {
  const close = WRAP_PAIRS[ch]
  if (close == null || start === end) return null
  const v = String(value)
  return {
    value: v.slice(0, start) + ch + v.slice(start, end) + close + v.slice(end),
    start: start + ch.length,
    end: end + ch.length,
  }
}

// lineSpan returns [from, to) covering every whole line the selection touches.
function lineSpan(value, start, end) {
  const from = value.lastIndexOf('\n', start - 1) + 1
  let to = value.indexOf('\n', end)
  if (to === -1) to = value.length
  return [from, to]
}

export function indentLines(value, start, end, unit = INDENT) {
  const v = String(value)
  const [from, to] = lineSpan(v, start, end)
  const lines = v.slice(from, to).split('\n')
  const block = lines.map((l) => unit + l).join('\n')
  return {
    value: v.slice(0, from) + block + v.slice(to),
    // `start` sits on the first line, `end` on the last — each line before them
    // added `unit`, so the two ends shift by different amounts.
    start: start + unit.length,
    end: end + unit.length * lines.length,
  }
}

export function outdentLines(value, start, end, unit = INDENT) {
  const v = String(value)
  const [from, to] = lineSpan(v, start, end)
  const strip = new RegExp(`^ {1,${unit.length}}`)
  let removedBeforeStart = 0
  let removedTotal = 0
  const block = v
    .slice(from, to)
    .split('\n')
    .map((l, i) => {
      const m = l.match(strip)
      const n = m ? m[0].length : 0
      if (i === 0) removedBeforeStart = n
      removedTotal += n
      return l.slice(n)
    })
    .join('\n')
  if (!removedTotal) return null // every line already at column 0
  const nextStart = Math.max(from, start - removedBeforeStart)
  return {
    value: v.slice(0, from) + block + v.slice(to),
    start: nextStart,
    end: Math.max(nextStart, end - removedTotal),
  }
}

// orderedListPrefix numbers the lines of a selection 1., 2., 3. — passed to the
// editor's applyLinePrefix, which accepts a function as well as a literal prefix.
export function orderedListPrefix(_line, i) {
  return `${i + 1}. `
}
