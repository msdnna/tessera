// Outline ("оглавление") and internal links for documents (#2733).
//
// The outline is derived from the live tree every time it is read, not stored
// as a block of its own. A stored table of contents is a copy of something that
// changes with every keystroke: it would need re-generating on save, would show
// up in the version diff (D6) as a change nobody made, and would be one more
// thing that can disagree with the document it describes.
//
// Anchors are block ids (D2), never slugs of the heading text. A slug reads
// nicer, but renaming a heading silently breaks every link pointing at it — and
// renaming headings is most of what editing a document's structure is. Block
// ids survive renames, moves (D3) and the paste de-duplication in blockId.js.
//
// Kept as pure functions on purpose: the interesting rules here (level
// normalisation, which section a block belongs to, what counts as an internal
// href) are worth testing without an editor or a mounted component.

import { quoteFromBlock } from './docComments'
import { i18n } from '@/i18n'

// Shown for a heading that exists but has no text yet — a heading is created
// empty and then typed into, and dropping it from the outline until the first
// character would make the list jump under the cursor.
export function untitledHeading() {
  return i18n.global.t('documents.toc.untitled')
}

/**
 * Collects the document's headings in reading order.
 *
 * Headings nested inside other blocks (a heading in a blockquote or a table
 * cell) are included: they are part of what the reader sees, and leaving them
 * out would make the outline skip a section that is visibly there.
 *
 * @param {object} json ProseMirror document JSON
 * @returns {Array<{id: string, level: number, text: string}>} headings in order
 */
export function docHeadings(json) {
  const out = []
  const walk = (node) => {
    if (!node || typeof node !== 'object') return
    if (node.type === 'heading' && node.attrs?.id) {
      out.push({
        id: node.attrs.id,
        level: clampLevel(node.attrs.level),
        text: quoteFromBlock(node, 120),
      })
    }
    if (Array.isArray(node.content)) node.content.forEach(walk)
  }
  walk(json)
  return out
}

/**
 * Nests a flat heading list for display.
 *
 * Levels are normalised rather than trusted: a document that starts at h2 and
 * drops to h4 would otherwise render with two empty indent steps in front of
 * every entry. What the outline shows is the *relative* structure the author
 * wrote, so a heading is nested under the nearest preceding heading of a
 * smaller level and indented one step from it, whatever the numbers say.
 *
 * @param {Array<object>} headings from docHeadings
 * @returns {Array<object>} the same entries with `depth` and `children`
 */
export function tocTree(headings) {
  const roots = []
  const stack = []
  for (const h of headings || []) {
    const node = { ...h, depth: 0, children: [] }
    while (stack.length && stack[stack.length - 1].level >= node.level) stack.pop()
    const parent = stack[stack.length - 1]
    if (parent) {
      node.depth = parent.depth + 1
      parent.children.push(node)
    } else {
      roots.push(node)
    }
    stack.push(node)
  }
  return roots
}

/**
 * Flattens the tree back into rows to render, keeping the computed depth.
 *
 * The panel draws a list, not a nested set of <ul>s: the entries are one column
 * of clickable rows and the nesting is a left offset, which keeps a deeply
 * nested heading from being squeezed to nothing on a narrow panel.
 *
 * @param {Array<object>} tree from tocTree
 * @returns {Array<object>} rows in reading order, each with `depth`
 */
export function tocRows(tree) {
  const out = []
  const walk = (nodes) => {
    for (const n of nodes || []) {
      out.push({ id: n.id, level: n.level, text: n.text, depth: n.depth })
      walk(n.children)
    }
  }
  walk(tree)
  return out
}

/**
 * The outline of a document in one call — what every caller actually wants.
 * @param {object} json ProseMirror document JSON
 */
export function docOutline(json) {
  return tocRows(tocTree(docHeadings(json)))
}

/**
 * The href an internal link carries.
 *
 * A bare fragment, so the link is still a link when the document is exported:
 * the renderer writes the same id as an anchor on the heading, and a `#id` in
 * the exported HTML/PDF resolves inside the file rather than pointing back at
 * this installation (which the reader of an exported document may not have).
 *
 * @param {string} blockId target block id
 */
export function internalHref(blockId) {
  return blockId ? `#${blockId}` : ''
}

/**
 * The block an href points at, or '' when the href leads outside the document.
 *
 * Anything that is not a bare fragment is external as far as the document is
 * concerned — including "/documents/x#block", which is a route change and must
 * stay the router's business.
 *
 * @param {string} href the link mark's href
 * @returns {string} block id, or '' if this is not an internal link
 */
export function internalTargetId(href) {
  const s = String(href || '').trim()
  if (!s.startsWith('#') || s.length < 2) return ''
  return s.slice(1)
}

/**
 * The heading whose section a block belongs to.
 *
 * Used to show where the reader is in the outline. It is the nearest heading
 * *before* the block in reading order, and a heading belongs to itself — the
 * cursor sitting on «Введение» should highlight «Введение», not the section
 * above it. Text before the first heading belongs to no section, which is
 * reported as ''.
 *
 * @param {object} json ProseMirror document JSON
 * @param {string} blockId the block the caret is in
 * @returns {string} heading block id, or '' when the block is above every
 *   heading or is not in the document at all
 */
export function headingForBlock(json, blockId) {
  if (!blockId) return ''
  let current = ''
  let found = ''
  let done = false
  const walk = (node) => {
    if (done || !node || typeof node !== 'object') return
    const id = node.attrs?.id
    if (node.type === 'heading' && id) current = id
    if (id && id === blockId) {
      found = current
      done = true
      return
    }
    if (Array.isArray(node.content)) node.content.forEach(walk)
  }
  walk(json)
  return found
}

/**
 * Label for an outline row, so an untitled heading is still clickable.
 * @param {object} row an outline row
 */
export function headingLabel(row) {
  return (row?.text || '').trim() || untitledHeading()
}

function clampLevel(level) {
  const n = Number(level)
  if (!Number.isFinite(n)) return 1
  return Math.min(6, Math.max(1, Math.round(n)))
}
