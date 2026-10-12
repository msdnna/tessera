// The document importer pulls the whole document editor (TipTap schema, the PDF
// embed with pdf.js) — ~3 MB — into the single-file design-system bundle, because
// «Входящие → в документ» imports it lazily and a library build inlines lazy
// imports. A design never converts a note into a document: the design system
// aliases the module to this stub, which keeps its names and refuses the work.
const refuse = () => {
  throw new Error('the document importer is not bundled in the Tessera design system')
}
export const IMPORT_EXTENSIONS = ['.md', '.markdown', '.json']
export const MAX_IMPORT_BYTES = 2 * 1024 * 1024
export const htmlToDoc = refuse
export const markdownToDoc = refuse
export const parseDocJSON = refuse
export const fileToTemplate = refuse
export const firstHeading = () => ''
