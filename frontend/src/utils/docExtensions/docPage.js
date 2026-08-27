import { Extension } from '@tiptap/core'
import { isPageSetup } from '../docPage'

/**
 * DocPage puts the page geometry on the document itself (#2821).
 *
 * On the `doc` node rather than in a settings column beside it, because the
 * geometry is part of the document the way its text is: it travels with a
 * duplicate, survives the version journal, and comes back with a restore. That
 * also means no migration — `documents.content` is already jsonb — and one
 * validation path, since the content endpoint already checks every attribute a
 * body carries (checkDocPage in document_schema.go).
 *
 * The attribute is deliberately inert in HTML. `renderHTML` on the doc node
 * would put the geometry on the editor's own element, where it would style the
 * editing surface and be re-parsed on every paste; the sheet is sized from the
 * value in Vue instead (DocEditor.vue), and the export writes its @page rule on
 * the server from the stored attribute. So this extension only declares the
 * attribute and the command that sets it.
 */
export const DocPage = Extension.create({
  name: 'docPage',

  addGlobalAttributes() {
    return [
      {
        types: ['doc'],
        attributes: {
          page: {
            default: null,
            // The doc node is never parsed from HTML in a way that could carry a
            // geometry — an imported document's geometry comes from the source
            // file, not from the converted markup, precisely because LibreOffice
            // drops it (see docOffice.js). Both hooks are stated rather than
            // omitted so that neither is inherited from a future default.
            parseHTML: () => null,
            renderHTML: () => ({}),
          },
        },
      },
    ]
  },

  addCommands() {
    return {
      /**
       * Sets or clears the document's page geometry.
       *
       * Anything that is not a valid geometry clears the attribute rather than
       * being stored: the server would refuse it on save, and a document the
       * user cannot save is a worse outcome than one that fell back to A4.
       */
      setDocPage:
        (page) =>
        ({ tr, dispatch }) => {
          if (dispatch) tr.setDocAttribute('page', isPageSetup(page) ? { ...page } : null)
          return true
        },
    }
  },
})
