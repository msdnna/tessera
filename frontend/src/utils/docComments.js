// Threading and anchoring for document annotations (#2730).
//
// The server stores comments flat — roots and replies in one table, one list per
// document — and everything below turns that list into what the panel draws.
// Kept as pure functions on purpose: this is the part with the interesting
// rules (orphans, ordering, counts), and it is worth testing without an editor,
// a socket or a mounted component.

/**
 * Collects the ids of every block currently in the document.
 *
 * Anchors are matched against this set, so a thread whose block was deleted can
 * be told apart from one whose block is simply scrolled out of view.
 *
 * @param {object} json ProseMirror document JSON
 * @returns {Set<string>} the ids present in the tree
 */
export function blockIdsIn(json) {
  const out = new Set()
  const walk = (node) => {
    if (!node || typeof node !== 'object') return
    if (node.attrs?.id) out.add(node.attrs.id)
    if (Array.isArray(node.content)) node.content.forEach(walk)
  }
  walk(json)
  return out
}

/**
 * Groups a flat comment list into threads.
 *
 * A thread is a root plus its replies, oldest first. Replies whose root is
 * missing are dropped rather than promoted: the root carries the anchor and the
 * resolved state, so a reply on its own would render as an answer to a question
 * nobody can see. (The API deletes replies with their root, so this only ever
 * fires on a list that was filtered client-side.)
 *
 * @param {Array<object>} comments flat comments as returned by the API
 * @returns {Array<object>} threads: root fields plus `replies`
 */
export function buildThreads(comments) {
  const list = Array.isArray(comments) ? comments : []
  const roots = new Map()
  for (const c of list) {
    if (c && c.id && !c.parent_id) roots.set(c.id, { ...c, replies: [] })
  }
  for (const c of list) {
    if (!c || !c.parent_id) continue
    roots.get(c.parent_id)?.replies.push(c)
  }
  return [...roots.values()]
}

/**
 * Orders threads for the panel: unresolved before resolved, newest first within
 * each group.
 *
 * Unresolved first because the panel is a to-do list — a document with fifty
 * settled remarks would otherwise bury the one that still needs an answer.
 *
 * @param {Array<object>} threads threads from buildThreads
 * @returns {Array<object>} a new, sorted array
 */
export function sortThreads(threads) {
  return [...(threads || [])].sort((a, b) => {
    const ra = a.resolved_at ? 1 : 0
    const rb = b.resolved_at ? 1 : 0
    if (ra !== rb) return ra - rb
    return String(b.created_at || '').localeCompare(String(a.created_at || ''))
  })
}

/**
 * Splits threads by where they point: at a block still in the document, at the
 * document as a whole, or at a block that has since been deleted.
 *
 * Detached threads are kept and shown apart, never dropped. A paragraph being
 * rewritten is the normal course of a review, and deleting the discussion about
 * it — the very thing that asked for the rewrite — is not something the user
 * asked for.
 *
 * @param {Array<object>} threads threads from buildThreads
 * @param {Set<string>} blockIds ids present in the document, from blockIdsIn
 * @returns {{anchored: Array<object>, document: Array<object>, detached: Array<object>}}
 */
export function splitThreads(threads, blockIds) {
  const ids = blockIds instanceof Set ? blockIds : new Set(blockIds || [])
  const out = { anchored: [], document: [], detached: [] }
  for (const t of threads || []) {
    if (!t.block_id) out.document.push(t)
    else if (ids.has(t.block_id)) out.anchored.push(t)
    else out.detached.push(t)
  }
  return out
}

/**
 * Unresolved thread count per block, which is what the editor paints in the
 * margin. Resolved threads are deliberately absent: a settled discussion should
 * stop marking up the text.
 *
 * @param {Array<object>} threads threads from buildThreads
 * @returns {Map<string, number>} block id → open thread count
 */
export function openCountByBlock(threads) {
  const out = new Map()
  for (const t of threads || []) {
    if (!t.block_id || t.resolved_at) continue
    out.set(t.block_id, (out.get(t.block_id) || 0) + 1)
  }
  return out
}

/**
 * The plain text of a block, used as the quote stored with a new annotation.
 *
 * Walks the node's own text rather than the DOM: the selection may be collapsed
 * (the user clicked into the paragraph and hit "comment" without selecting
 * anything), and an empty quote would leave the thread with no context at all.
 *
 * @param {object} node ProseMirror node JSON, or a node with textContent
 * @param {number} limit maximum characters to keep
 * @returns {string}
 */
export function quoteFromBlock(node, limit = 160) {
  if (!node) return ''
  if (typeof node.textContent === 'string') return trim(node.textContent, limit)
  let text = ''
  const walk = (n) => {
    if (!n || typeof n !== 'object') return
    if (typeof n.text === 'string') text += n.text
    if (Array.isArray(n.content)) n.content.forEach(walk)
  }
  walk(node)
  return trim(text, limit)
}

function trim(s, limit) {
  const flat = String(s || '')
    .replace(/\s+/g, ' ')
    .trim()
  return flat.length > limit ? `${flat.slice(0, limit)}…` : flat
}

/**
 * Display name for a comment author, matching the fallback chain the document
 * socket uses for presence badges — the same person must not appear as "Иван" in
 * the roster and as their email in the thread right below it.
 *
 * @param {object} comment a comment row with author_name/author_email
 * @returns {string}
 */
export function authorLabel(comment) {
  const name = (comment?.author_name || '').trim()
  if (name) return name
  const email = comment?.author_email || ''
  const local = email.split('@')[0]
  return local || 'Участник'
}
