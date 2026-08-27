/**
 * Block-level comparison of two document versions (#2731, D6 of #2718).
 *
 * The diff is anchored on block ids — the stable ids BlockId stamps on every
 * top-level node (D2) — and not on text positions. That is the whole payoff of
 * the block model the task chose: a paragraph that was dragged three places up
 * is *moved*, not "deleted here and added there", and one edited word inside a
 * long paragraph does not repaint the paragraphs around it. A text-level LCS
 * would have to guess all of that back from the flattened string.
 *
 * Blocks without an id (imported content, or documents written before D2) fall
 * back to matching by position, which is the best available answer when there is
 * nothing stable to match on.
 */

import { i18n } from '@/i18n'

/** Status of a block in the comparison. */
export const DIFF_SAME = 'same'
export const DIFF_CHANGED = 'changed'
export const DIFF_ADDED = 'added'
export const DIFF_REMOVED = 'removed'
export const DIFF_MOVED = 'moved'

/**
 * Flattens one top-level node into the text the diff compares and renders.
 *
 * Nested structure (list items, table cells) is joined with spaces rather than
 * walked separately: the unit of comparison is the block, and a list whose third
 * item changed is a changed list. Splitting it further would mean anchors below
 * the level the ids are stamped at, which is exactly the guesswork the id-based
 * diff exists to avoid.
 *
 * @param {object} node ProseMirror node
 * @returns {string} plain text of the node
 */
export function blockText(node) {
  if (!node) return ''
  if (node.type === 'text') return node.text || ''
  if (node.type === 'image')
    return node.attrs?.alt || node.attrs?.src || i18n.global.t('documents.history.diffImage')
  if (node.type === 'horizontalRule') return '———'
  if (!Array.isArray(node.content)) return ''
  return node.content.map(blockText).filter(Boolean).join(' ').replace(/\s+/g, ' ').trim()
}

/**
 * The top-level blocks of a document, in order.
 * @param {object} doc ProseMirror document JSON
 * @returns {Array<{id: string, type: string, text: string, json: string}>}
 */
export function docBlocks(doc) {
  const nodes = Array.isArray(doc?.content) ? doc.content : []
  return nodes.map((node, i) => ({
    id: node?.attrs?.id || '',
    type: node?.type || 'paragraph',
    text: blockText(node),
    // Serialised node, so a change invisible in the flattened text (a heading
    // level, an alignment, a link target) still registers as an edit.
    json: JSON.stringify(node),
    index: i,
  }))
}

/**
 * Compares two document versions block by block.
 *
 * The result is a single ordered list rather than two columns: the panel renders
 * it as one readable document with removals struck through in place, which is
 * the view that answers "what changed here" without the reader diffing the diff.
 * Removed blocks are placed after the block they used to follow, so a deletion
 * stays where it happened.
 *
 * @param {object} oldDoc the earlier version's content
 * @param {object} newDoc the later version's content
 * @returns {Array<{status: string, type: string, text: string, prevText?: string}>}
 */
export function diffDocs(oldDoc, newDoc) {
  const before = docBlocks(oldDoc)
  const after = docBlocks(newDoc)

  // Ids are unique per document, but a copy-paste can duplicate one; keeping the
  // first occurrence makes the pairing deterministic instead of order-dependent.
  const beforeById = new Map()
  before.forEach((b) => {
    if (b.id && !beforeById.has(b.id)) beforeById.set(b.id, b)
  })
  const afterById = new Map()
  after.forEach((b) => {
    if (b.id && !afterById.has(b.id)) afterById.set(b.id, b)
  })

  // Blocks with no id are paired positionally among themselves — see the module
  // comment for why this fallback exists at all.
  const anonBefore = before.filter((b) => !b.id)
  const anonAfter = after.filter((b) => !b.id)
  const anonPairs = new Map()
  anonAfter.forEach((b, i) => {
    if (anonBefore[i]) anonPairs.set(b, anonBefore[i])
  })

  const matched = new Set()
  const rows = []

  for (const block of after) {
    const prev = block.id ? beforeById.get(block.id) : anonPairs.get(block)
    if (!prev) {
      rows.push({ status: DIFF_ADDED, type: block.type, text: block.text })
      continue
    }
    matched.add(prev)
    // `src` links the row back to the block it came from, so deletions can be
    // placed by identity below instead of by matching text — two identical
    // paragraphs are ordinary, and matching on their text would put a deletion
    // next to the wrong one.
    const src = prev
    if (prev.json !== block.json) {
      rows.push({
        status: DIFF_CHANGED,
        type: block.type,
        text: block.text,
        prevText: prev.text,
        src,
      })
      continue
    }
    // Same content, different neighbourhood: the block was dragged. Position is
    // compared among *surviving* blocks only — otherwise deleting a paragraph
    // would report everything below it as moved.
    const prevOrder = before.filter((b) => afterById.has(b.id) || anonPairs.has(b)).indexOf(prev)
    const nextOrder = after.filter((b) => beforeById.has(b.id) || anonPairs.has(b)).indexOf(block)
    rows.push({
      status: prevOrder !== nextOrder && prevOrder !== -1 ? DIFF_MOVED : DIFF_SAME,
      type: block.type,
      text: block.text,
      src,
    })
  }

  // Deletions, put back where they happened: right after the last surviving
  // block that preceded them. Walking `before` forwards keeps several deletions
  // in their original order relative to each other.
  for (let i = 0; i < before.length; i++) {
    const block = before[i]
    if (matched.has(block)) continue
    const anchor = before
      .slice(0, i)
      .reverse()
      .find((b) => matched.has(b))
    const row = { status: DIFF_REMOVED, type: block.type, text: block.text }
    let at = anchor ? rows.findIndex((r) => r.src === anchor) + 1 : 0
    // Skip past deletions already filed under the same anchor.
    while (rows[at]?.status === DIFF_REMOVED) at++
    rows.splice(at, 0, row)
  }

  // `src` was bookkeeping for placing the deletions; the panel gets rows only.
  rows.forEach((row) => delete row.src)
  return rows
}

/**
 * Counts what changed, for the one-line summary above the comparison.
 * @param {Array} rows result of diffDocs
 */
export function diffSummary(rows) {
  const count = (status) => rows.filter((r) => r.status === status).length
  return {
    added: count(DIFF_ADDED),
    removed: count(DIFF_REMOVED),
    changed: count(DIFF_CHANGED),
    moved: count(DIFF_MOVED),
    identical: rows.every((r) => r.status === DIFF_SAME),
  }
}
