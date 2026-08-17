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
 * The same ids as blockIdsIn, in the order they appear in the document.
 *
 * Separate from blockIdsIn because a Set answers "is this block still here?"
 * and cannot answer "which of these two comes first" — and the panel needs the
 * second one to lay its cards out in reading order (#2730).
 *
 * @param {object} json ProseMirror document JSON
 * @returns {Array<string>} ids in document order, without duplicates
 */
export function blockIdsInOrder(json) {
  const out = []
  const seen = new Set()
  const walk = (node) => {
    if (!node || typeof node !== 'object') return
    const id = node.attrs?.id
    if (id && !seen.has(id)) {
      seen.add(id)
      out.push(id)
    }
    if (Array.isArray(node.content)) node.content.forEach(walk)
  }
  walk(json)
  return out
}

/**
 * Finds the block node carrying an id.
 *
 * The counterpart to blockIdsIn: anchoring something *new* to a block needs the
 * node itself, not just proof that it is still there — a task link records the
 * clause it hangs on so it can still say which one once that clause is rewritten
 * (#2732).
 *
 * @param {object} json ProseMirror document JSON
 * @param {string} blockId the anchor to look for
 * @returns {object|null} the node, or null when the block is gone
 */
export function blockNodeById(json, blockId) {
  if (!blockId) return null
  let found = null
  const walk = (node) => {
    if (found || !node || typeof node !== 'object') return
    if (node.attrs?.id === blockId) {
      found = node
      return
    }
    if (Array.isArray(node.content)) node.content.forEach(walk)
  }
  walk(json)
  return found
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
 * Anchored threads come out in document order when `blockIds` carries one (pass
 * the array from blockIdsInOrder rather than the Set from blockIdsIn). That is
 * what keeps the annotation lines from crossing: the cards and the blocks they
 * point at are then two monotonic sequences, and two monotonic sequences cannot
 * cross. The cost is that a settled thread on an early block now sits above an
 * open one further down — inside a block the unresolved-first order still holds,
 * and a resolved card is dimmed, so the panel keeps saying which is which.
 *
 * @param {Array<object>} threads threads from buildThreads
 * @param {Set<string>|Array<string>} blockIds ids present in the document; an
 *   array is read as document order, a Set only as membership
 * @returns {{anchored: Array<object>, document: Array<object>, detached: Array<object>}}
 */
export function splitThreads(threads, blockIds) {
  const order = Array.isArray(blockIds) ? blockIds : null
  const ids = blockIds instanceof Set ? blockIds : new Set(blockIds || [])
  const out = { anchored: [], document: [], detached: [] }
  for (const t of threads || []) {
    if (!t.block_id) out.document.push(t)
    else if (ids.has(t.block_id)) out.anchored.push(t)
    else out.detached.push(t)
  }
  if (order) {
    const rank = new Map(order.map((id, i) => [id, i]))
    // Stable sort: threads on the same block keep the order sortThreads gave
    // them (open before settled, newest first).
    out.anchored.sort((a, b) => (rank.get(a.block_id) ?? 0) - (rank.get(b.block_id) ?? 0))
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
