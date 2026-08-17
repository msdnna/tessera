import { Extension } from '@tiptap/core'

// Blocks that can carry paragraph-level spacing and indentation.
export const STYLED_TYPES = ['paragraph', 'heading', 'blockquote']

export const LINE_HEIGHTS = ['1', '1.15', '1.5', '2']
export const MAX_INDENT = 8
const INDENT_STEP_PX = 32

/**
 * BlockStyle adds the two paragraph controls item 7 of #2718 asks for and the
 * MIT core does not have as block attributes: line spacing and indentation.
 *
 * `@tiptap/extension-text-style` does ship a LineHeight, but it is an attribute
 * of the inline textStyle mark — it styles a span inside the line, not the
 * block. Paragraph spacing set on a run of text and lost when the user retypes
 * the paragraph is not what "интервалы" means here, so the attribute lives on
 * the block node instead.
 */
export const BlockStyle = Extension.create({
  name: 'blockStyle',

  addOptions() {
    return { types: STYLED_TYPES }
  },

  addGlobalAttributes() {
    return [
      {
        types: this.options.types,
        attributes: {
          lineHeight: {
            default: null,
            parseHTML: (element) => element.style.lineHeight || null,
            renderHTML: (attributes) =>
              attributes.lineHeight ? { style: `line-height: ${attributes.lineHeight}` } : {},
          },
          indent: {
            default: null,
            parseHTML: (element) => {
              const px = parseInt(element.style.marginInlineStart || '', 10)
              return px > 0 ? Math.round(px / INDENT_STEP_PX) : null
            },
            renderHTML: (attributes) =>
              attributes.indent
                ? { style: `margin-inline-start: ${attributes.indent * INDENT_STEP_PX}px` }
                : {},
          },
        },
      },
    ]
  },

  addCommands() {
    const setOnSelection = (attr) => (value) => (params) => {
      const { state, tr, dispatch } = params
      const { from, to } = state.selection
      const types = new Set(this.options.types)
      let touched = false
      state.doc.nodesBetween(from, to, (node, pos) => {
        if (!types.has(node.type.name)) return
        tr.setNodeMarkup(pos, undefined, { ...node.attrs, [attr]: value })
        touched = true
      })
      if (touched && dispatch) dispatch(tr)
      return touched
    }

    const shiftIndent = (delta) => (params) => {
      const { state, tr, dispatch } = params
      const { from, to } = state.selection
      const types = new Set(this.options.types)
      let touched = false
      state.doc.nodesBetween(from, to, (node, pos) => {
        if (!types.has(node.type.name)) return
        const next = Math.min(MAX_INDENT, Math.max(0, (node.attrs.indent || 0) + delta))
        // A no-op at the boundary must not report success, or the toolbar
        // button would look like it did something at indent 0.
        if (next === (node.attrs.indent || 0)) return
        tr.setNodeMarkup(pos, undefined, { ...node.attrs, indent: next || null })
        touched = true
      })
      if (touched && dispatch) dispatch(tr)
      return touched
    }

    return {
      setLineHeight: setOnSelection('lineHeight'),
      unsetLineHeight: () => setOnSelection('lineHeight')(null),
      indent: () => shiftIndent(1),
      outdent: () => shiftIndent(-1),
    }
  },
})
