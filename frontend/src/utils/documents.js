// Tree assembly for the documents section. The API returns a flat list (the
// same shape serves Android and MCP), so nesting is rebuilt here.

import { i18n } from '@/i18n'

/**
 * Builds a nested tree from the flat document list.
 *
 * Orphans — documents whose parent is missing from the list (filtered out, or
 * not yet loaded) — are surfaced at the root rather than dropped: a document
 * the user can open by link must not be invisible in the tree. Cycles, which
 * the API rejects but a stale client could still hold, are broken the same way.
 *
 * @param {Array<object>} docs flat documents, each with id and parent_id
 * @returns {Array<object>} roots, each with a `children` array
 */
export function buildDocTree(docs) {
  const list = Array.isArray(docs) ? docs : []
  const byId = new Map()
  for (const d of list) {
    if (d && d.id) byId.set(d.id, { ...d, children: [] })
  }
  const roots = []
  for (const d of list) {
    const node = d && d.id ? byId.get(d.id) : null
    if (!node) continue
    const parent = d.parent_id ? byId.get(d.parent_id) : null
    if (parent && parent.id !== node.id && !isAncestor(byId, node.id, parent)) {
      parent.children.push(node)
    } else {
      roots.push(node)
    }
  }
  return roots
}

// isAncestor reports whether `candidate` sits below the node with id `nodeId`,
// walking up the raw parent_id chain. Guards buildDocTree against a cycle in
// stale data, which would otherwise make a subtree unreachable from any root.
function isAncestor(byId, nodeId, candidate) {
  const seen = new Set()
  let cur = candidate
  while (cur && !seen.has(cur.id)) {
    if (cur.id === nodeId) return true
    seen.add(cur.id)
    cur = cur.parent_id ? byId.get(cur.parent_id) : null
  }
  return false
}

/**
 * Flattens a document tree into n-tree options, preserving order.
 * @param {Array<object>} nodes tree roots from buildDocTree
 */
export function docTreeOptions(nodes) {
  return (nodes || []).map((n) => ({
    key: n.id,
    label: n.title || i18n.global.t('documents.view.untitled'),
    icon: n.icon || '',
    children: n.children && n.children.length ? docTreeOptions(n.children) : undefined,
  }))
}
