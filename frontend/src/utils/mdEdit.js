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

// ── auto-close pairs (typing with no selection) ──
// Only brackets, quotes and the backtick auto-close on a single keystroke. The
// emphasis marks (* _ ~) are deliberately left out of this set: they still wrap a
// selection (WRAP_PAIRS), but auto-pairing them on every keystroke turns "a * b"
// into "a ** b". `<` is out too — HTML like <details> is common and `<>` would
// fight it.
export const AUTOCLOSE_PAIRS = {
  '(': ')',
  '[': ']',
  '{': '}',
  '"': '"',
  "'": "'",
  '`': '`',
}
const CLOSERS = new Set(Object.values(AUTOCLOSE_PAIRS))

// autoClose decides what typing `ch` at a collapsed caret should do:
//   { type: 'over', caret }                    — step over the closing char we
//                                                 already inserted (so `()` typed
//                                                 as "(" then ")" stays "()"),
//   { type: 'insert', value, start, end }      — insert the pair, caret between,
//   null                                        — leave it to the browser.
export function autoClose(value, caret, ch) {
  const v = String(value)
  const before = caret > 0 ? v[caret - 1] : undefined
  const after = caret < v.length ? v[caret] : undefined
  // Type-over: the same closing char is already right after the caret.
  if (CLOSERS.has(ch) && after === ch) return { type: 'over', caret: caret + 1 }
  const close = AUTOCLOSE_PAIRS[ch]
  if (close == null) return null
  // Only pair when the char after the caret is nothing, whitespace or a closer —
  // otherwise "(" before existing text would trap it inside the pair.
  const okAfter = after === undefined || /\s/.test(after) || /[)\]}]/.test(after)
  if (!okAfter) return null
  // Symmetric marks (quote/backtick) additionally need a boundary before the caret
  // so an apostrophe inside a word (don't) or a closing quote stays literal.
  if (ch === close) {
    const boundaryBefore = before === undefined || /\s/.test(before) || /[([{]/.test(before)
    if (!boundaryBefore) return null
  }
  return {
    type: 'insert',
    value: v.slice(0, caret) + ch + close + v.slice(caret),
    start: caret + 1,
    end: caret + 1,
  }
}

// deletePair removes both halves of an empty auto-inserted pair when Backspace is
// pressed between them ("(|)" → ""), so auto-closing never leaves an orphan.
export function deletePair(value, caret) {
  const v = String(value)
  const before = caret > 0 ? v[caret - 1] : undefined
  const after = caret < v.length ? v[caret] : undefined
  if (before === undefined || after === undefined) return null
  if (AUTOCLOSE_PAIRS[before] === after) {
    return { value: v.slice(0, caret - 1) + v.slice(caret + 1), start: caret - 1, end: caret - 1 }
  }
  return null
}

// ── Enter handling: code fences and list continuation ──
function currentLine(value, caret) {
  const start = value.lastIndexOf('\n', caret - 1) + 1
  let end = value.indexOf('\n', caret)
  if (end === -1) end = value.length
  return { start, end, text: value.slice(start, end) }
}

// listItem parses the marker of a bullet / ordered / checkbox line.
function listItem(line) {
  let m = line.match(/^(\s*)([-*+])[ \t]+(\[[ xX]\][ \t]+)?(.*)$/)
  if (m) return { indent: m[1], bullet: m[2], checkbox: !!m[3], content: m[4], ordered: false }
  m = line.match(/^(\s*)(\d+)([.)])[ \t]+(.*)$/)
  if (m) return { indent: m[1], num: parseInt(m[2], 10), delim: m[3], content: m[4], ordered: true }
  return null
}

// handleEnter returns the next state for a plain Enter at a collapsed caret, or
// null to let the browser insert an ordinary newline. It covers two cases:
//   • an opening ``` fence with nothing closing it below → drop a closing fence
//     under the caret (GitHub-style), so code blocks self-close;
//   • a list item → carry the marker to the next line (numbers increment,
//     checkboxes reset to unchecked); an empty item ends the list instead.
export function handleEnter(value, caret) {
  const v = String(value)
  const line = currentLine(v, caret)
  // Code fence: only when the caret sits at the end of the opening line and no
  // fence closes it further down.
  const fence = line.text.match(/^(\s*)(`{3,})([^`]*)$/)
  if (fence && caret === line.end) {
    const closedBelow = /(^|\n)[ \t]*`{3,}/.test(v.slice(line.end))
    if (!closedBelow) {
      const indent = fence[1]
      const insert = `\n${indent}\n${indent}${'`'.repeat(fence[2].length)}`
      const inner = caret + 1 + indent.length
      return { value: v.slice(0, caret) + insert + v.slice(caret), start: inner, end: inner }
    }
  }
  const item = listItem(line.text)
  if (!item) return null
  // Empty item → end the list: clear the marker line, leave the caret on it.
  if (item.content.trim() === '') {
    return { value: v.slice(0, line.start) + v.slice(line.end), start: line.start, end: line.start }
  }
  const marker = item.ordered
    ? `${item.indent}${item.num + 1}${item.delim} `
    : item.checkbox
      ? `${item.indent}${item.bullet} [ ] `
      : `${item.indent}${item.bullet} `
  const insert = `\n${marker}`
  const at = caret + insert.length
  return { value: v.slice(0, caret) + insert + v.slice(caret), start: at, end: at }
}
