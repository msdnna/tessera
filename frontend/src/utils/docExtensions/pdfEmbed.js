import { Node, mergeAttributes } from '@tiptap/core'
import { VueNodeViewRenderer } from '@tiptap/vue-3'
import DocPdf from '@/components/documents/DocPdf.vue'

/**
 * PdfEmbed is the block an imported or dropped PDF becomes (#2733).
 *
 * An atom, not a container: the pages are the file's, and there is nothing
 * inside the node for the editor to address. It is still a block like any other
 * — it takes an id from BlockId, so D5 annotations and D4 locks work on it, it
 * can be dragged by D3's handle, and D6 sees it in the version diff.
 *
 * The pages are drawn by a Vue node view rather than by renderHTML, because
 * pdf.js needs a canvas and a lifecycle. renderHTML below is therefore only the
 * *serialised* form — what a copy to the clipboard or an HTML paste sees — and
 * it deliberately produces a link rather than an <embed>: pasting a document
 * into an email should carry a reference to the file, not a viewer that will
 * not run there.
 */
export const PdfEmbed = Node.create({
  name: 'pdfEmbed',
  group: 'block',
  atom: true,
  draggable: true,
  selectable: true,

  addAttributes() {
    return {
      src: {
        default: null,
        parseHTML: (element) => element.getAttribute('data-src') || element.getAttribute('href'),
        renderHTML: (attributes) => (attributes.src ? { 'data-src': attributes.src } : {}),
      },
      name: {
        default: '',
        parseHTML: (element) => element.getAttribute('data-name') || element.textContent || '',
        renderHTML: (attributes) => (attributes.name ? { 'data-name': attributes.name } : {}),
      },
      size: {
        default: 0,
        parseHTML: (element) => Number(element.getAttribute('data-size')) || 0,
        renderHTML: (attributes) => (attributes.size ? { 'data-size': attributes.size } : {}),
      },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-pdf-embed]' }]
  },

  renderHTML({ HTMLAttributes, node }) {
    const label = node.attrs.name || 'документ.pdf'
    return [
      'div',
      mergeAttributes(HTMLAttributes, { 'data-pdf-embed': '' }),
      ['a', { href: node.attrs.src || '#' }, `PDF: ${label}`],
    ]
  },

  addNodeView() {
    return VueNodeViewRenderer(DocPdf)
  },

  addCommands() {
    return {
      /**
       * Inserts a stored PDF at the cursor.
       * @param {{src: string, name?: string, size?: number}} pdf
       */
      insertPdf:
        (pdf) =>
        ({ commands }) => {
          if (!pdf?.src) return false
          return commands.insertContent({
            type: this.name,
            attrs: { src: pdf.src, name: pdf.name || 'документ.pdf', size: Number(pdf.size) || 0 },
          })
        },
    }
  },
})
