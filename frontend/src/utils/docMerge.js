// Merging a colleague's saved document into the one on screen (#2729 rework).
//
// The naive answer — replace the editor's content with the server's — is what
// D4 avoided in the first place: it throws away the caret, the selection and
// whatever the user has typed since the last debounce. The whole reason blocks
// carry stable ids (D2) is so a remote save can be taken *per block* instead.
//
// The rule is: the server decides which blocks exist and in what order; this
// client decides what is inside the blocks it is holding. That is the same
// boundary the block lock draws, just applied to data rather than to keystrokes.

/** Top-level blocks of a ProseMirror document JSON, or null when it is not one. */
function blocksOf(doc) {
  if (!doc || doc.type !== 'doc' || !Array.isArray(doc.content)) return null
  return doc.content
}

function idOf(block) {
  const id = block?.attrs?.id
  return typeof id === 'string' && id ? id : ''
}

/**
 * Merges the server's version of a document into the local one.
 *
 * Blocks listed in `keepIds` (the block this client holds, and the one the caret
 * sits in) keep their local content. A kept block the server has never seen is
 * kept too, in its local place: it is a paragraph the user just started, and
 * dropping it because the last save predates it would delete text in front of
 * the person typing it.
 *
 * @param {object} local the tree currently in the editor
 * @param {object} remote the tree just fetched from the server
 * @param {object} [opts]
 * @param {string[]} [opts.keepIds] blocks whose local content wins
 * @returns {object|null} the merged document, or null when it cannot be merged
 *   safely — an unaddressable block (no id, e.g. from an old import) means the
 *   caller should reload instead, because guessing here silently loses text
 */
export function mergeRemoteBlocks(local, remote, { keepIds = [] } = {}) {
  const localBlocks = blocksOf(local)
  const remoteBlocks = blocksOf(remote)
  if (!localBlocks || !remoteBlocks) return null
  if ([...localBlocks, ...remoteBlocks].some((b) => !idOf(b))) return null

  const keep = new Set(keepIds.filter(Boolean))
  const localById = new Map(localBlocks.map((b) => [idOf(b), b]))

  const merged = remoteBlocks.map((b) => {
    const id = idOf(b)
    return keep.has(id) && localById.has(id) ? localById.get(id) : b
  })

  // Kept blocks the server does not know about yet, spliced back where they sit
  // locally: after the nearest preceding block that survived, or at the front.
  const present = new Set(merged.map(idOf))
  localBlocks.forEach((block, i) => {
    const id = idOf(block)
    if (!keep.has(id) || present.has(id)) return
    let at = 0
    for (let j = i - 1; j >= 0; j -= 1) {
      const anchor = merged.findIndex((b) => idOf(b) === idOf(localBlocks[j]))
      if (anchor >= 0) {
        at = anchor + 1
        break
      }
    }
    merged.splice(at, 0, block)
    present.add(id)
  })

  return { ...remote, content: merged }
}

/**
 * Whether two documents differ in which top-level blocks they have, or in what
 * order — as opposed to differing only inside blocks.
 *
 * The editor applies the two cases differently: same shape means a handful of
 * single-block replacements that leave every other position untouched, while a
 * changed shape needs the content swapped wholesale and the caret remapped.
 *
 * @param {object} a first document
 * @param {object} b second document
 * @returns {boolean}
 */
export function blockOrderChanged(a, b) {
  const left = (blocksOf(a) || []).map(idOf)
  const right = (blocksOf(b) || []).map(idOf)
  return left.length !== right.length || left.some((id, i) => id !== right[i])
}
