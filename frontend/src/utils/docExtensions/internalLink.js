import { Extension } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'
import { internalTargetId } from '../docToc'

export const internalLinkKey = new PluginKey('internalLink')

// Meta carrying "flash this block" into the plugin state.
const FLASH = 'internalLink$flash'

// How long the arrived-here cue stays. Long enough to be seen after a smooth
// scroll, short enough that it does not read as a permanent highlight.
export const FLASH_MS = 1400

/**
 * Finds a block by id and reports where it sits.
 *
 * @param {object} doc ProseMirror document node
 * @param {string} blockId
 * @returns {{pos: number, node: object}|null}
 */
export function blockPosById(doc, blockId) {
  if (!doc || !blockId) return null
  let found = null
  doc.descendants((node, pos) => {
    if (found) return false
    if (node.attrs?.id === blockId) {
      found = { pos, node }
      return false
    }
    return true
  })
  return found
}

/**
 * Brings a block into view and marks it as the one just arrived at.
 *
 * The scroll is done on the DOM rather than through a selection: a read-only
 * surface has no caret to move, and moving the selection in an editable one
 * would drag the user's cursor across the document because they clicked a link.
 *
 * @param {object} view ProseMirror editor view
 * @param {string} blockId target block
 * @returns {boolean} false when the document has no such block
 */
export function scrollToBlockId(view, blockId) {
  if (!view || view.isDestroyed || !blockId) return false
  const found = blockPosById(view.state.doc, blockId)
  if (!found) return false
  const dom = view.nodeDOM(found.pos)
  // scrollIntoView is absent in jsdom and on a detached node; the flash below
  // still runs, which is what the tests assert on.
  dom?.scrollIntoView?.({ block: 'center', behavior: 'smooth' })
  view.dispatch(view.state.tr.setMeta(FLASH, blockId).setMeta('addToHistory', false))
  setTimeout(() => {
    if (view.isDestroyed) return
    // Only clear our own flash: a second jump while this timer was pending owns
    // the highlight now, and clearing it would blank the cue the user is
    // currently looking at.
    if (internalLinkKey.getState(view.state)?.id !== blockId) return
    view.dispatch(view.state.tr.setMeta(FLASH, '').setMeta('addToHistory', false))
  }, FLASH_MS)
  return true
}

/**
 * InternalLink makes `#<block id>` links jump inside the document (#2733).
 *
 * Following a link is deliberately not the same gesture in both modes. On a
 * read-only surface a click follows it, which is what a reader expects from a
 * table of contents. In the editor a plain click keeps placing the caret —
 * a link in text you are writing has to be editable — and following needs
 * Ctrl/Cmd, the same intent the editor already asks for elsewhere.
 *
 * The default is prevented either way: a bare `#fragment` inside a read-only
 * surface is a real navigation, and letting it through would push a hash onto
 * the SPA's URL and leave the router looking at a route that no longer matches
 * what is on screen.
 */
export const InternalLink = Extension.create({
  name: 'internalLink',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: internalLinkKey,
        state: {
          init: () => ({ id: '' }),
          apply(tr, value) {
            const next = tr.getMeta(FLASH)
            return next === undefined ? value : { id: next }
          },
        },
        props: {
          decorations(state) {
            const { id } = internalLinkKey.getState(state) || { id: '' }
            if (!id) return DecorationSet.empty
            const found = blockPosById(state.doc, id)
            if (!found) return DecorationSet.empty
            return DecorationSet.create(state.doc, [
              Decoration.node(found.pos, found.pos + found.node.nodeSize, {
                class: 'doc-block-target',
              }),
            ])
          },
          handleDOMEvents: {
            click(view, event) {
              const anchor = event.target?.closest?.('a[href]')
              if (!anchor) return false
              const id = internalTargetId(anchor.getAttribute('href'))
              if (!id) return false
              const follow = !view.editable || event.metaKey || event.ctrlKey
              if (!follow) return false
              event.preventDefault()
              scrollToBlockId(view, id)
              return true
            },
          },
        },
      }),
    ]
  },
})
