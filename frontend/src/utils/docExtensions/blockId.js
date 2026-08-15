import { Extension } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'

// Block types that carry a stable id. Inline nodes (text, hardBreak) and table
// internals are deliberately absent: nothing addresses them.
export const ID_BEARING_TYPES = [
  'paragraph',
  'heading',
  'blockquote',
  'codeBlock',
  'bulletList',
  'orderedList',
  'taskList',
  'horizontalRule',
  'image',
  'table',
]

// newBlockId is short and collision-safe enough for identifiers scoped to one
// document. crypto.randomUUID is not used because the id ends up in the stored
// JSON for every block, and 36 bytes per paragraph adds up over a long page.
export function newBlockId() {
  const rnd = new Uint8Array(8)
  ;(globalThis.crypto || {}).getRandomValues?.(rnd)
  let out = ''
  for (const b of rnd) out += b.toString(16).padStart(2, '0')
  return out || Math.random().toString(16).slice(2, 18)
}

const blockIdPluginKey = new PluginKey('blockId')

/**
 * Fills in missing and de-duplicates repeated block ids on a plain JSON tree.
 *
 * Content handed to a fresh editor never passes through a transaction, and the
 * editor's own `create` hook runs a tick later — so documents written before
 * this extension existed would open with no ids at all for that first tick.
 * Doing it on the JSON keeps the invariant true from the moment the document is
 * loaded, and keeps it testable without an editor.
 *
 * @param {object} json ProseMirror document JSON
 * @returns {object} the same shape with every block carrying a unique id
 */
export function ensureBlockIds(json) {
  const types = new Set(ID_BEARING_TYPES)
  const seen = new Set()
  const walk = (node) => {
    if (!node || typeof node !== 'object') return node
    const next = { ...node }
    if (types.has(next.type)) {
      const id = next.attrs?.id
      next.attrs = { ...(next.attrs || {}), id: !id || seen.has(id) ? newBlockId() : id }
      seen.add(next.attrs.id)
    }
    if (Array.isArray(next.content)) next.content = next.content.map(walk)
    return next
  }
  return walk(json)
}

// Marks the transactions this extension dispatches on its own. Stamping ids is
// bookkeeping, not an edit: DocEditor skips these so that merely opening a
// document does not look dirty and kick off an autosave.
export const BLOCK_ID_META = 'blockIdStamp'

// stampIds returns a transaction that gives every block a unique id, or null if
// there is nothing to fix. Blocks without an id get one; an id that appears
// twice (paste of a copied block) is re-issued on the copy, because one anchor
// addressing two paragraphs would show D5's comments on both.
function stampIds(state) {
  const types = new Set(ID_BEARING_TYPES)
  const seen = new Set()
  const fixes = []
  state.doc.descendants((node, pos) => {
    if (!types.has(node.type.name)) return
    const id = node.attrs.id
    if (!id || seen.has(id)) {
      fixes.push(pos)
      return
    }
    seen.add(id)
  })
  if (!fixes.length) return null
  const tr = state.tr
  for (const pos of fixes) {
    const node = tr.doc.nodeAt(pos)
    if (!node) continue
    const id = newBlockId()
    seen.add(id)
    tr.setNodeMarkup(pos, undefined, { ...node.attrs, id })
  }
  return tr.setMeta('addToHistory', false).setMeta(BLOCK_ID_META, true)
}

/**
 * BlockId gives every block node a stable `id` attribute.
 *
 * It lives here, in D2, rather than in D4 (per-block locks) and D5 (per-block
 * annotations) that actually consume it: adding it later would mean every
 * document written in between has no ids, and D4 would have to start with a
 * backfill over every `content` in the database.
 *
 * The id survives parsing — copy/paste of a paragraph keeps it — with one
 * exception handled by the plugin below: when the *same* id appears twice in
 * one document (paste of a copied block), the duplicate is re-issued. Without
 * that, a comment anchored to a block would show up on its copy too.
 */
export const BlockId = Extension.create({
  name: 'blockId',

  addGlobalAttributes() {
    return [
      {
        types: ID_BEARING_TYPES,
        attributes: {
          id: {
            default: null,
            parseHTML: (element) => element.getAttribute('data-id'),
            renderHTML: (attributes) => (attributes.id ? { 'data-id': attributes.id } : {}),
            keepOnSplit: false,
          },
        },
      },
    ]
  },

  // Content loaded into a fresh editor never passes through a transaction, so
  // the plugin below would not see it: documents written before this extension
  // existed (and every freshly opened one) would stay id-less until the first
  // keystroke.
  onCreate() {
    const tr = stampIds(this.editor.state)
    if (tr) this.editor.view.dispatch(tr)
  },

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: blockIdPluginKey,
        // One appendTransaction pass stamps missing ids and de-duplicates the
        // ones a paste brought in. Doing it here rather than in a nodeView
        // keeps the invariant true for programmatic edits too (setContent,
        // collaborative patches in D4).
        appendTransaction: (_transactions, _oldState, newState) => stampIds(newState),
      }),
    ]
  },
})
