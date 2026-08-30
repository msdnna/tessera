import { Node, mergeAttributes } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'
import {
  DEFAULT_PAGE,
  SECTION_BREAK,
  bandStyle,
  isLandscape,
  isPageSetup,
  normalizePage,
  sectionIndexes,
  sectionPages,
  sizeKey,
  styleAttr,
  topBlocks,
} from '../docPage'
import { i18n } from '@/i18n'

/**
 * SectionBreak gives one document more than one page geometry (#2827).
 *
 * #2821 put a single geometry on the doc node, which is all an editor without
 * pagination needs — until a .docx arrives whose landscape section holds the
 * wide table the portrait one could not fit. Word calls that a section; here it
 * is a block: the break carries the geometry of everything *after* it, and the
 * doc node keeps the geometry of everything before the first one. That split is
 * what makes the feature free for existing documents — a document with no break
 * has exactly one section, and it is the attribute it already had.
 *
 * The geometry travels on the node rather than in a table beside the document
 * for the same reason the doc-level one does (docPage.js): it survives a
 * duplicate, a version restore and a copy-paste, and needs no migration.
 */

export const sectionBreakKey = new PluginKey('sectionBreak')

/** Reads a geometry out of the `data-page` attribute a pasted break carries. */
function parsePage(raw) {
  if (!raw) return null
  try {
    const page = JSON.parse(raw)
    return isPageSetup(page) ? page : null
  } catch {
    // A break whose geometry does not parse is still a break — losing the
    // section boundary would silently reflow the rest of the document.
    return null
  }
}

/**
 * The caption drawn on a break: what the section after it looks like.
 *
 * Translated at render time rather than baked into the node, so a language
 * switch reaches breaks that are already on screen (pitfall 1 of #2799).
 *
 * @param {*} page the break's stored geometry
 * @returns {string}
 */
export function sectionCaption(page) {
  const p = normalizePage(page)
  const t = i18n.global.t
  return t('documents.section.caption', {
    orientation: t(
      `documents.toolbar.page.${isLandscape(p) ? 'landscape' : 'portrait'}`,
    ).toLowerCase(),
    size: t(`documents.toolbar.page.sizes.${sizeKey(p) || 'custom'}`),
  })
}

/** True when the document has at least one break — i.e. more than one section. */
export function hasSections(doc) {
  return topBlocks(doc).some((b) => b.type === SECTION_BREAK)
}

/**
 * Which section the selection sits in, and that section's geometry.
 *
 * Both are wanted together by every caller (the toolbar shows the geometry and
 * writes it back to the same section), and deriving them separately is how the
 * two would drift.
 *
 * @param {object} state editor state
 * @returns {{index:number, page:object, pages:Array<object>}}
 */
export function sectionAt(state) {
  const doc = state.doc
  const blocks = topBlocks(doc)
  const pages = sectionPages(blocks, doc.attrs?.page)
  if (!blocks.length) return { index: 0, page: pages[0], pages }
  const at = Math.min(state.selection.$from.index(0), blocks.length - 1)
  const index = sectionIndexes(blocks)[at] || 0
  return { index, page: pages[index] || pages[0], pages }
}

/** Document position of the break that opens section `index` (1-based sections). */
function breakPos(doc, index) {
  let seen = 0
  let pos = -1
  doc.forEach((node, offset) => {
    if (node.type.name !== SECTION_BREAK) return
    seen += 1
    if (seen === index) pos = offset
  })
  return pos
}

/**
 * Node decorations that turn each top-level block into a band of its section.
 *
 * Only built when the document actually has a break: a single-section document
 * must render exactly as it did before this extension existed, and the cheapest
 * way to guarantee that is to emit nothing at all for it.
 *
 * @param {object} doc ProseMirror document
 * @returns {DecorationSet}
 */
export function bandDecorations(doc) {
  const blocks = topBlocks(doc)
  if (!blocks.some((b) => b.type === SECTION_BREAK)) return DecorationSet.empty
  const pages = sectionPages(blocks, doc.attrs?.page)
  const sections = sectionIndexes(blocks)
  const decos = []
  doc.forEach((node, pos, i) => {
    const page = pages[sections[i]] || pages[0]
    const first = i === 0 || sections[i] !== sections[i - 1]
    const last = i === blocks.length - 1 || sections[i] !== sections[i + 1]
    decos.push(
      Decoration.node(pos, pos + node.nodeSize, {
        class: 'doc-band',
        style: styleAttr(bandStyle(page, first, last)),
      }),
    )
  })
  return DecorationSet.create(doc, decos)
}

export const SectionBreak = Node.create({
  name: SECTION_BREAK,

  group: 'block',
  atom: true,
  selectable: true,
  // Not draggable: a break dragged into the middle of the document reassigns
  // the geometry of everything between its old and new place, which is a large
  // edit made by a small gesture. It is deleted and inserted instead.
  draggable: false,

  addAttributes() {
    return {
      page: {
        default: null,
        parseHTML: (el) => parsePage(el.getAttribute('data-page')),
        renderHTML: (attrs) =>
          isPageSetup(attrs.page) ? { 'data-page': JSON.stringify(attrs.page) } : {},
      },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-section-break]' }]
  },

  renderHTML({ node, HTMLAttributes }) {
    return [
      'div',
      mergeAttributes(HTMLAttributes, {
        'data-section-break': '',
        class: 'doc-section-break',
      }),
      ['span', { class: 'sb-caption' }, sectionCaption(node.attrs.page)],
    ]
  },

  addCommands() {
    return {
      /**
       * Inserts a break after the block the caret is in, opening a section that
       * starts as a copy of the current one. Copying rather than defaulting to
       * A4 is deliberate: the user reaches for this to change *one* thing
       * (usually the orientation), and a break that resets the margins as well
       * would undo work they did on the page dialog.
       */
      insertSectionBreak:
        (page) =>
        ({ state, chain }) => {
          const geometry = isPageSetup(page) ? { ...page } : { ...sectionAt(state).page }
          const { $from } = state.selection
          const pos = $from.depth ? $from.after(1) : state.selection.to
          const content = [{ type: SECTION_BREAK, attrs: { page: geometry } }]
          // A break as the last node leaves the new section with nothing in it
          // and no way to put a caret there.
          if (pos >= state.doc.content.size) content.push({ type: 'paragraph' })
          return chain().insertContentAt(pos, content).run()
        },

      /**
       * Sets the geometry of the section the selection is in — the doc
       * attribute for the first one, the opening break's for the rest.
       *
       * One command for both so that the toolbar has a single call and cannot
       * write a section's geometry onto the document by mistake.
       */
      setSectionPage:
        (page) =>
        ({ state, tr, dispatch }) => {
          const { index } = sectionAt(state)
          if (index === 0) {
            if (dispatch) tr.setDocAttribute('page', isPageSetup(page) ? { ...page } : null)
            return true
          }
          const pos = breakPos(state.doc, index)
          if (pos < 0) return false
          if (dispatch) {
            const node = state.doc.nodeAt(pos)
            tr.setNodeMarkup(pos, undefined, {
              ...node.attrs,
              // A break with no geometry would silently inherit the previous
              // section's and stop being a section boundary at all.
              page: isPageSetup(page) ? { ...page } : { ...DEFAULT_PAGE },
            })
          }
          return true
        },
    }
  },

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: sectionBreakKey,
        state: {
          init: (_config, state) => bandDecorations(state.doc),
          // Rebuilt only when the document changed: the bands depend on the
          // tree, not on the selection, and onTransaction fires on every
          // keystroke and every caret move.
          apply: (tr, prev) => (tr.docChanged ? bandDecorations(tr.doc) : prev),
        },
        props: {
          decorations(state) {
            return sectionBreakKey.getState(state)
          },
        },
      }),
    ]
  },
})
