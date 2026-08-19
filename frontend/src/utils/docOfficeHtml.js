// Normalising the HTML the LibreOffice sidecar returns before it is parsed
// with the editor schema (#2755).
//
// The converter emits *legacy* HTML: colours as <font color>, sizes as <font
// size>, centring as <center>, paragraph styles as classes resolved in a
// <style> block, and a rule under a heading as a paragraph border rather than
// an <hr>. TipTap reads none of that — it looks at inline styles — so an
// imported document arrived with its colours, sizes, rules and code blocks
// silently flattened, which is what the task reported.
//
// The fix belongs here rather than in the schema: the editor's parsing rules
// are the same ones a paste or a template goes through, and teaching them
// about <font> would mean carrying Word's markup into every other import path.
// This module translates one converter's dialect into the styles the schema
// already understands, and everything downstream stays as it was.
//
// What is deliberately NOT carried over:
//   * the body font ("Calibri, serif" on every paragraph). It is not installed
//     here, it is not a choice the author made per paragraph, and pinning it
//     would override the sheet's typography for the whole document. Only
//     monospace survives, because there it carries meaning — it marks code.
//   * the default ink (<body text="…">, usually #222222) and the base font
//     size. Both are "Word's default", not formatting: keeping them would
//     freeze the document to the light theme and to one size forever.
//   * table fills and borders — they need a schema attribute that does not
//     exist yet (подзадача #2756).

// Properties worth lifting out of the <style> block. Everything else there is
// print geometry (@page, margins, orphans/widows) that a block document has no
// use for.
const CARRIED_PROPS = [
  'color',
  'font-family',
  'font-size',
  'font-weight',
  'font-style',
  'text-align',
  'text-decoration',
]

// Font stack a monospace run is mapped onto — the same one the editor's own
// font picker offers (FONT_FAMILIES in docSchema.js), so imported code and
// typed code look alike instead of depending on Consolas being installed.
const MONO_STACK = 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace'

const MONO_RE = /consolas|courier|menlo|monaco|liberation mono|dejavu sans mono|monospace/i

// <font size="N"> when the tag carries no font-size of its own. LibreOffice
// writes both, but hand-written HTML and older converters only write the
// attribute.
const LEGACY_SIZES = { 1: '10px', 2: '13px', 3: '16px', 4: '18px', 5: '24px', 6: '32px', 7: '48px' }

const ALIGNMENTS = ['left', 'center', 'right', 'justify']

const BLOCK_TAGS = new Set([
  'P',
  'DIV',
  'H1',
  'H2',
  'H3',
  'H4',
  'H5',
  'H6',
  'LI',
  'TD',
  'TH',
  'PRE',
  'BLOCKQUOTE',
])

/**
 * Rewrites converter HTML into the inline-styled HTML the editor schema reads.
 *
 * Pure and idempotent-ish: HTML that has none of the legacy constructs comes
 * back unchanged, so this is safe on the .html/.htm files the same import route
 * accepts.
 *
 * @param {string} html HTML as returned by the import endpoint
 * @returns {string} HTML for htmlToDoc
 */
export function normalizeOfficeHtml(html) {
  const src = String(html || '')
  if (!src.trim()) return src
  const doc = new DOMParser().parseFromString(src, 'text/html')
  const body = doc.body
  if (!body) return src

  const defaultInk = String(body.getAttribute('text') || '').trim()
  expandStyleBlocks(doc)
  inlineFontTags(doc)
  applyAlignment(doc)
  dropDefaultInk(body, defaultInk)
  dropBaseFontSize(body)
  monospaceToCode(doc)
  bordersToRules(doc)
  return body.innerHTML
}

/**
 * Resolves the class-based rules LibreOffice puts in <style> into inline
 * styles.
 *
 * Only the declarations in CARRIED_PROPS travel, and an inline style always
 * wins — the stylesheet is the weaker source in CSS too, and a paragraph that
 * says `font-size: 24pt` on itself means it.
 *
 * @param {Document} doc parsed document
 */
function expandStyleBlocks(doc) {
  const rules = []
  doc.querySelectorAll('style').forEach((el) => {
    rules.push(...parseRules(el.textContent || ''))
    el.remove()
  })
  if (!rules.length) return

  // Accumulated per element so a later, more specific rule (h1.western after
  // h1 — the order LibreOffice writes them in) overrides an earlier one.
  const pending = new Map()
  for (const { selector, decls } of rules) {
    let matched
    try {
      matched = doc.body.querySelectorAll(selector)
    } catch {
      continue // a selector this browser cannot parse is not worth failing over
    }
    matched.forEach((el) => {
      pending.set(el, { ...(pending.get(el) || {}), ...decls })
    })
  }
  for (const [el, decls] of pending) {
    for (const [prop, value] of Object.entries(decls)) {
      setStyle(el, prop, value)
    }
  }
}

/**
 * Splits a stylesheet into flat `selector { declarations }` pairs.
 *
 * At-rules and pseudo-class selectors are skipped: @page is print geometry, and
 * `a:link` describes a state a static document has no way to express.
 *
 * @param {string} css stylesheet text
 * @returns {Array<{selector: string, decls: object}>}
 */
function parseRules(css) {
  const out = []
  const text = String(css).replace(/\/\*[\s\S]*?\*\//g, '')
  for (const match of text.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const decls = parseDecls(match[2])
    if (!Object.keys(decls).length) continue
    for (const selector of match[1].split(',')) {
      const sel = selector.trim()
      if (!sel || sel.startsWith('@') || sel.includes(':')) continue
      out.push({ selector: sel, decls })
    }
  }
  return out
}

/**
 * Parses a declaration list, keeping only the properties worth carrying.
 * @param {string} text declarations between the braces
 * @returns {object} property → value
 */
function parseDecls(text) {
  const out = {}
  for (const decl of String(text).split(';')) {
    const at = decl.indexOf(':')
    if (at < 0) continue
    const prop = decl.slice(0, at).trim().toLowerCase()
    const value = decl.slice(at + 1).trim()
    if (!value || !CARRIED_PROPS.includes(prop)) continue
    out[prop] = value
  }
  return out
}

/**
 * Writes one declaration onto an element unless it already has that property
 * inline, applying the two conversions the whole module depends on: a font
 * family only survives if it is monospace, and a size is normalised to px.
 *
 * @param {Element} el target
 * @param {string} prop CSS property
 * @param {string} value CSS value
 */
function setStyle(el, prop, value) {
  if (el.style.getPropertyValue(prop)) return
  if (prop === 'font-family') {
    if (!MONO_RE.test(value)) return
    el.style.setProperty(prop, MONO_STACK)
    return
  }
  el.style.setProperty(prop, prop === 'font-size' ? toPx(value) : value)
}

/**
 * Converts a CSS length to px so every size in the document is comparable.
 *
 * Word works in points and the editor's size picker in pixels; leaving both in
 * the document would mean "16px" and "12pt" sorting as different sizes in a UI
 * that shows one list.
 *
 * @param {string} value CSS length
 * @returns {string} px value, or the input when it is not a length we convert
 */
function toPx(value) {
  const m = String(value)
    .trim()
    .match(/^(-?[\d.]+)(pt|px|in|cm|mm)$/i)
  if (!m) return String(value).trim()
  const n = parseFloat(m[1])
  const factor = { pt: 96 / 72, px: 1, in: 96, cm: 96 / 2.54, mm: 96 / 25.4 }[m[2].toLowerCase()]
  return Math.round(n * factor) + 'px'
}

/**
 * Replaces every <font> with a styled <span>.
 *
 * A <font> that carries nothing we keep is unwrapped instead of becoming an
 * empty span: the schema turns a styleless span into a textStyle mark with no
 * attributes, and this document has 297 of them.
 *
 * @param {Document} doc parsed document
 */
function inlineFontTags(doc) {
  doc.body.querySelectorAll('font').forEach((el) => {
    const span = doc.createElement('span')
    const color = el.getAttribute('color') || el.style.color
    if (color) span.style.color = color
    const face = el.getAttribute('face') || el.style.fontFamily
    if (face && MONO_RE.test(face)) span.style.fontFamily = MONO_STACK
    const size = el.style.fontSize || LEGACY_SIZES[el.getAttribute('size')]
    if (size) span.style.fontSize = toPx(size)
    for (const prop of ['font-weight', 'font-style', 'text-decoration']) {
      const value = el.style.getPropertyValue(prop)
      if (value) span.style.setProperty(prop, value)
    }

    const target = span.getAttribute('style') ? span : doc.createDocumentFragment()
    while (el.firstChild) target.appendChild(el.firstChild)
    el.replaceWith(target)
  })
}

/**
 * Turns <center> and align="…" into text-align, the only form TipTap reads.
 * @param {Document} doc parsed document
 */
function applyAlignment(doc) {
  doc.body.querySelectorAll('center').forEach((el) => {
    let blocks = 0
    for (const child of Array.from(el.children)) {
      if (!BLOCK_TAGS.has(child.tagName) && child.tagName !== 'TABLE') continue
      blocks += 1
      setStyle(child, 'text-align', 'center')
    }
    // A <center> holding bare text has no block to carry the alignment, so it
    // becomes the paragraph it was standing in for.
    if (!blocks && el.textContent.trim()) {
      const p = doc.createElement('p')
      p.style.textAlign = 'center'
      while (el.firstChild) p.appendChild(el.firstChild)
      el.replaceWith(p)
      return
    }
    const frag = doc.createDocumentFragment()
    while (el.firstChild) frag.appendChild(el.firstChild)
    el.replaceWith(frag)
  })

  doc.body.querySelectorAll('[align]').forEach((el) => {
    const value = String(el.getAttribute('align')).toLowerCase()
    el.removeAttribute('align')
    // align="bottom" on an image is vertical alignment, not text alignment.
    if (ALIGNMENTS.includes(value)) setStyle(el, 'text-align', value)
  })
}

/**
 * Drops the colour that is merely the document's default ink.
 *
 * Word writes it onto the body and repeats it on every paragraph rule, so
 * keeping it would paint the entire document in a near-black that the dark
 * theme then has to fight. A colour the author actually chose differs from it.
 *
 * @param {HTMLElement} body document body
 * @param {string} defaultInk value of <body text="…">
 */
function dropDefaultInk(body, defaultInk) {
  const ink = normaliseHex(defaultInk)
  body.querySelectorAll('[style*="color"]').forEach((el) => {
    const color = normaliseHex(el.style.color)
    if (!color) return
    if (color === ink || color === '#000000') el.style.removeProperty('color')
  })
}

/**
 * Drops the size that is merely the document's base size.
 *
 * Every run in a Word document carries an explicit size, so without this the
 * body text is pinned to 15px and only ever renders at the size the .docx was
 * written for. What matters for the reported bug is the *relative* order —
 * title above heading above body — and that survives untouched.
 *
 * @param {HTMLElement} body document body
 */
function dropBaseFontSize(body) {
  const sized = Array.from(body.querySelectorAll('[style*="font-size"]')).filter(
    (el) => el.style.fontSize,
  )
  if (sized.length < 4) return
  const counts = new Map()
  for (const el of sized) counts.set(el.style.fontSize, (counts.get(el.style.fontSize) || 0) + 1)
  let base = null
  let top = 0
  for (const [size, n] of counts) {
    if (n > top) {
      top = n
      base = size
    }
  }
  // A size that is not actually dominant is a real choice, not a default.
  if (!base || top * 2 < sized.length) return
  for (const el of sized) {
    if (el.style.fontSize === base) el.style.removeProperty('font-size')
  }
}

/**
 * Turns monospace runs into code.
 *
 * Word has no code block, so a code listing arrives as one of two things: a
 * single-cell table with a grey fill (what the attached document uses), or a
 * paragraph set in Consolas. Both become a real codeBlock; a monospace run
 * inside a normal sentence becomes an inline `code` mark.
 *
 * @param {Document} doc parsed document
 */
function monospaceToCode(doc) {
  doc.body.querySelectorAll('table').forEach((table) => {
    const cells = table.querySelectorAll('td, th')
    if (cells.length !== 1 || !isAllMonospace(cells[0])) return
    table.replaceWith(codeBlock(doc, cells[0]))
  })

  doc.body.querySelectorAll('p, div').forEach((el) => {
    if (!el.isConnected || !isAllMonospace(el)) return
    el.replaceWith(codeBlock(doc, el))
  })

  doc.body.querySelectorAll('span').forEach((el) => {
    if (!el.isConnected || !MONO_RE.test(el.style.fontFamily || '')) return
    if (!el.textContent.trim()) return
    const code = doc.createElement('code')
    el.style.removeProperty('font-family')
    while (el.firstChild) code.appendChild(el.firstChild)
    el.appendChild(code)
  })
}

/**
 * True when every piece of text under an element is monospace.
 * @param {Element} el candidate block
 */
function isAllMonospace(el) {
  const walker = el.ownerDocument.createTreeWalker(el, NodeFilter.SHOW_TEXT)
  let seen = false
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    if (!node.textContent.trim()) continue
    seen = true
    let mono = false
    for (let cur = node.parentElement; cur && cur !== el.parentElement; cur = cur.parentElement) {
      if (MONO_RE.test(cur.style?.fontFamily || '')) {
        mono = true
        break
      }
    }
    if (!mono) return false
  }
  return seen
}

/**
 * Builds a <pre><code> holding an element's text, with line structure kept.
 * @param {Document} doc parsed document
 * @param {Element} el source block
 * @returns {HTMLElement} pre element
 */
function codeBlock(doc, el) {
  const pre = doc.createElement('pre')
  const code = doc.createElement('code')
  code.textContent = blockText(el)
  pre.appendChild(code)
  return pre
}

/**
 * Text of a block with <br> and nested blocks as newlines.
 *
 * Source-level line wrapping is collapsed to single spaces: LibreOffice breaks
 * long lines in the markup for readability, and taking those breaks literally
 * would chop every command in the listing in half.
 *
 * @param {Element} el source block
 * @returns {string} plain text
 */
function blockText(el) {
  const parts = []
  const walk = (node) => {
    for (const child of Array.from(node.childNodes)) {
      if (child.nodeType === 3) {
        parts.push(child.textContent.replace(/\s+/g, ' '))
        continue
      }
      if (child.nodeType !== 1) continue
      if (child.tagName === 'BR') {
        parts.push('\n')
        continue
      }
      walk(child)
      if (BLOCK_TAGS.has(child.tagName)) parts.push('\n')
    }
  }
  walk(el)
  return parts
    .join('')
    .split('\n')
    .map((line) => line.trim())
    .join('\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

/**
 * Turns a bordered paragraph into a paragraph plus a horizontal rule.
 *
 * Word draws the rule under a title as a paragraph border, which is why the
 * lines went missing: there is no <hr> in the converted HTML at all. An empty
 * bordered paragraph *is* the rule and is replaced by it; one with text keeps
 * its text and gains the rule on the side the border was on.
 *
 * @param {Document} doc parsed document
 */
function bordersToRules(doc) {
  doc.body.querySelectorAll('p, div').forEach((el) => {
    const style = el.getAttribute('style') || ''
    const top = hasBorder(style, 'top')
    const bottom = hasBorder(style, 'bottom')
    if (!top && !bottom) return
    if (top) el.before(doc.createElement('hr'))
    if (!bottom) return
    if (el.textContent.trim() || el.querySelector('img')) el.after(doc.createElement('hr'))
    else el.replaceWith(doc.createElement('hr'))
  })
}

/**
 * Whether a style attribute declares a visible border on one side.
 * @param {string} style raw style attribute
 * @param {string} side 'top' or 'bottom'
 */
function hasBorder(style, side) {
  const m = style.match(new RegExp('border-' + side + '\\s*:\\s*([^;]+)', 'i'))
  if (!m) return false
  const value = m[1].trim().toLowerCase()
  if (!value || value.includes('none') || value.includes('hidden')) return false
  // A zero-width border is written as `0in` as often as it is omitted.
  const width = value.match(/(-?[\d.]+)\s*(px|pt|in|cm|mm|em)?/)
  return !width || parseFloat(width[1]) > 0
}

/**
 * Normalises a colour to lowercase #rrggbb for comparison, when it is a form we
 * can compare at all.
 * @param {string} value CSS colour
 * @returns {string} normalised hex, or ''
 */
function normaliseHex(value) {
  const css = String(value || '')
    .trim()
    .toLowerCase()
  const short = css.match(/^#([0-9a-f]{3})$/)
  if (short) return '#' + [...short[1]].map((c) => c + c).join('')
  if (/^#[0-9a-f]{6}$/.test(css)) return css
  const rgb = css.match(/^rgba?\(\s*(\d+)\D+(\d+)\D+(\d+)/)
  if (!rgb) return ''
  return (
    '#' +
    rgb
      .slice(1, 4)
      .map((n) => Number(n).toString(16).padStart(2, '0'))
      .join('')
  )
}
