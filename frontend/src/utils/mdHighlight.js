// Syntax tint for the Markdown source while it is being typed (#3118, PackageJ
// «Описание · правка», «Пузырь в поле», «Полноэкранный»): the markers — `## `,
// `**`, `` ` ``, `- [ ] ` — fade to the third text colour, headings go bold,
// @-mentions and #N task references take the accent, and in the comment composer
// a `/command` at the start of a line and the half-typed @/#/ query under the
// caret are marked too.
//
// The editor stays a plain <textarea> (TipTap was tried and rejected): this HTML
// is drawn in a layer laid exactly over the field, whose own text is transparent.
// So the one hard rule here is that the tint never changes the width of a glyph:
// colour and background only, plus bold where the field is monospace (bold and
// regular advance alike there) — the editor passes `bold: false` for the
// proportional composer. Every character of the source is emitted exactly once
// and in order, so the layer wraps where the field wraps.

const ESC = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }
const esc = (s) => s.replace(/[&<>"]/g, (c) => ESC[c])

// What may stand right before an @-handle or a #N reference: the line start,
// whitespace or opening punctuation — so «user@mail» and «C#1» stay text.
const LEAD = /[\s([{«"'—,;:]/

function mark(cls, from, to, name) {
  for (let i = from; i < to; i++) cls[i] = name
}

// highlightMarkdown returns the HTML of the tint layer for `src`.
//   mentions  labels of the people the editor knows (`@Анна Ким` is two words)
//   commands  tint a `/key` that opens a line (the comment composer)
//   active    { start, end } of the open @ / # / / query — drawn as a soft chip
//   bold      headings in bold (monospace fields only)
export function highlightMarkdown(src, opts = {}) {
  const { mentions = [], commands = false, active = null, bold = true } = opts
  const text = String(src || '')
  const n = text.length
  const cls = new Array(n).fill('')

  const lines = text.split('\n')
  let at = 0
  let fence = false
  for (const line of lines) {
    const end = at + line.length
    if (/^\s{0,3}(```|~~~)/.test(line)) {
      mark(cls, at, end, 'mk')
      fence = !fence
    } else if (fence) {
      // A fenced block is code: no Markdown inside it means anything.
    } else {
      let m
      if ((m = /^(\s{0,3}#{1,6}\s+)(.*)$/.exec(line))) {
        mark(cls, at, at + m[1].length, 'mk')
        if (bold) mark(cls, at + m[1].length, end, 'h')
      } else if ((m = /^(\s*(?:[-*+]|\d+[.)])\s+(?:\[[ xX]\]\s+)?)/.exec(line))) {
        mark(cls, at, at + m[1].length, 'mk')
      } else if ((m = /^(\s*>+\s?)/.exec(line))) {
        mark(cls, at, at + m[1].length, 'mk')
      } else if (commands && (m = /^\/[a-z0-9_-]+/i.exec(line))) {
        mark(cls, at, at + m[0].length, 'cmd')
      }
      inline(text, at, end, cls, mentions)
    }
    at = end + 1
  }

  // Runs of one class → spans. The active query rides on top as a second class.
  const q = active && active.end > active.start ? active : null
  let out = ''
  let i = 0
  while (i < n) {
    const c = cls[i]
    const inQ = !!q && i >= q.start && i < q.end
    let j = i + 1
    while (j < n && cls[j] === c && (!!q && j >= q.start && j < q.end) === inQ) j++
    const piece = esc(text.slice(i, j))
    const names = [c && `mh-${c}`, inQ && 'mh-q'].filter(Boolean).join(' ')
    out += names ? `<span class="${names}">${piece}</span>` : piece
    i = j
  }
  // A trailing line break needs something after it, or the layer is one line
  // shorter than the field and the caret line sits on nothing.
  if (text.endsWith('\n') || !text) out += ' '
  return out
}

// Inline marks of one line [from, to): code spans first (nothing inside them is
// Markdown), then emphasis markers, link brackets, mentions and task refs.
function inline(text, from, to, cls, mentions) {
  const line = text.slice(from, to)
  const free = (i) => !cls[from + i]
  let m

  const code = /`+/g
  const spans = []
  while ((m = code.exec(line))) {
    const tick = m[0]
    const close = line.indexOf(tick, m.index + tick.length)
    if (close < 0) break
    mark(cls, from + m.index, from + m.index + tick.length, 'mk')
    mark(cls, from + close, from + close + tick.length, 'mk')
    spans.push([m.index, close + tick.length])
    code.lastIndex = close + tick.length
  }
  const inCode = (i) => spans.some(([a, b]) => i >= a && i < b)

  const pairs = /(\*\*|__|~~)(?=\S)(.+?\S)\1/g
  while ((m = pairs.exec(line))) {
    if (inCode(m.index)) continue
    const a = m.index
    const b = m.index + m[0].length
    mark(cls, from + a, from + a + 2, 'mk')
    mark(cls, from + b - 2, from + b, 'mk')
  }
  const single = /(^|[^*_\w])([*_])(?=\S)([^*_\n]*?\S)\2(?![*_\w])/g
  while ((m = single.exec(line))) {
    const a = m.index + m[1].length
    if (inCode(a) || !free(a)) continue
    const b = a + 1 + m[3].length
    mark(cls, from + a, from + a + 1, 'mk')
    mark(cls, from + b, from + b + 1, 'mk')
  }
  const link = /!?\[([^\]\n]*)\]\(([^)\s]*)\)/g
  while ((m = link.exec(line))) {
    if (inCode(m.index)) continue
    const a = m.index
    const textEnd = a + m[0].indexOf(']')
    mark(cls, from + a, from + a + (m[0][0] === '!' ? 2 : 1), 'mk')
    mark(cls, from + textEnd, from + a + m[0].length, 'mk')
  }

  const lower = line.toLowerCase()
  const labels = [...new Set((mentions || []).filter(Boolean))].sort((x, y) => y.length - x.length)
  for (let i = 0; i < line.length; i++) {
    const ch = line[i]
    if ((ch !== '@' && ch !== '#') || inCode(i) || !free(i)) continue
    if (i > 0 && !LEAD.test(line[i - 1])) continue
    let len = 0
    if (ch === '#') {
      const num = /^#\d+\b/.exec(line.slice(i))
      if (num) len = num[0].length
    } else {
      const hit = labels.find((l) => lower.startsWith(l.toLowerCase(), i + 1))
      if (hit && !/[\p{L}\p{N}_]/u.test(line[i + 1 + hit.length] || '')) len = hit.length + 1
      else {
        const handle = /^@[\p{L}\p{N}_.-]*[\p{L}\p{N}_]/u.exec(line.slice(i))
        if (handle) len = handle[0].length
      }
    }
    if (len) {
      mark(cls, from + i, from + i + len, 'ref')
      i += len - 1
    }
  }
}
