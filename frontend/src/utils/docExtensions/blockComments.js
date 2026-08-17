import { Extension } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'
import { blockRanges } from './blockLocks'

export const blockCommentsKey = new PluginKey('blockComments')

// Meta carrying the new per-block counts into the plugin state.
const SET_COMMENTS = 'blockComments$set'

/**
 * The top-level block the caret sits in, as both id and node.
 *
 * Annotating needs the node and not just the id: the quote stored with the
 * thread is taken from the text that is there at the moment the remark is made,
 * so that the discussion still says what it was about after the paragraph has
 * been rewritten.
 *
 * @param {object} state ProseMirror editor state
 * @returns {{id: string, node: object}|null}
 */
export function blockAtSelection(state) {
  const { from } = state.selection
  let pos = 0
  let found = null
  state.doc.forEach((node) => {
    const to = pos + node.nodeSize
    if (!found && from >= pos && from <= to) found = { id: node.attrs?.id || '', node }
    pos = to
  })
  return found
}

/** Pushes new per-block comment counts into a live editor. */
export function applyBlockComments(view, counts) {
  if (!view) return
  view.dispatch(view.state.tr.setMeta(SET_COMMENTS, counts || []).setMeta('addToHistory', false))
}

/**
 * BlockComments marks the blocks that carry an open discussion.
 *
 * It is a decoration and nothing else: no mark is written into the content, so
 * annotating a document does not modify it, does not start an autosave and does
 * not need the block to be free of someone else's lock. That separation is what
 * makes annotating an imported document work — it needs a block id, not a
 * schema that knows about comments.
 *
 * Only *unresolved* threads are painted. A settled discussion that goes on
 * highlighting the text would make a reviewed document look permanently
 * unfinished.
 */
export const BlockComments = Extension.create({
  name: 'blockComments',

  addOptions() {
    return {
      // Clicking a discussed block brings its thread into view in the panel.
      onSelect: () => {},
    }
  },

  addProseMirrorPlugins() {
    const options = this.options
    return [
      new Plugin({
        key: blockCommentsKey,
        state: {
          init: () => ({ counts: [] }),
          apply(tr, value) {
            const next = tr.getMeta(SET_COMMENTS)
            return next ? { counts: next } : value
          },
        },
        props: {
          decorations(state) {
            const { counts } = blockCommentsKey.getState(state) || { counts: [] }
            if (!counts.length) return DecorationSet.empty
            const byId = new Map(counts.map((c) => [c.block_id, c.count]))
            const decos = []
            for (const b of blockRanges(state.doc)) {
              const n = byId.get(b.id)
              if (!n) continue
              decos.push(
                Decoration.node(b.from, b.to, {
                  class: 'doc-block-commented',
                  'data-comment-count': String(n),
                }),
              )
            }
            return DecorationSet.create(state.doc, decos)
          },
          handleClick(view, pos) {
            const { counts } = blockCommentsKey.getState(view.state) || { counts: [] }
            if (!counts.length) return false
            const ids = new Set(counts.map((c) => c.block_id))
            for (const b of blockRanges(view.state.doc)) {
              if (pos >= b.from && pos <= b.to && ids.has(b.id)) {
                options.onSelect(b.id)
                break
              }
            }
            // Never handled: this only reveals a thread, and swallowing the click
            // would stop the caret from landing where the user clicked.
            return false
          },
        },
      }),
    ]
  },
})
