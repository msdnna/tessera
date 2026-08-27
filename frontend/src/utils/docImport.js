import { Editor } from '@tiptap/core'
import { renderMarkdown } from './markdown'
import { docExtensions, isDocJSON } from './docSchema'
import { ensureBlockIds } from './docExtensions/blockId'
import { formatFileSize } from './docPdf'
import { i18n } from '@/i18n'

// Turning an uploaded file into a document body (#2734, "загрузка готовых
// шаблонов"). Two formats, and both are deliberate:
//
//   .json — a body exported from Tessera. Round-trips exactly, which is what
//           moving a template between workspaces (or restoring one) needs.
//   .md   — the format the rest of Tessera already stores its text in
//           (MarkdownEditor), so a description or a note becomes a template
//           without a conversion service.
//
// Office formats (docx/odt) are D8's business: they need the LibreOffice
// sidecar, and doing them here would mean a second, worse converter.
//
// Parsing goes through TipTap rather than a hand-written HTML→JSON walk: the
// editor schema *is* the allow-list (docSchema.js), so anything it cannot
// represent is dropped at parse time instead of being rejected later by the
// server with a 400 the user cannot act on.

export const IMPORT_EXTENSIONS = ['.md', '.markdown', '.json']

// Bounds the file we are willing to read. The server caps a body at 4 MiB
// (maxDocContentSize) and Markdown expands when parsed, so a file past this is
// refused before the browser spends memory on it rather than after.
export const MAX_IMPORT_BYTES = 2 * 1024 * 1024

/**
 * Parses HTML into a document body using the editor's own schema.
 *
 * The editor is created detached and destroyed straight away: this is a pure
 * conversion, and leaving a live editor around would leak a ProseMirror view
 * per import.
 *
 * @param {string} html
 * @returns {object} ProseMirror document JSON
 */
export function htmlToDoc(html) {
  const editor = new Editor({ extensions: docExtensions(), content: html || '' })
  try {
    return ensureBlockIds(editor.getJSON())
  } finally {
    editor.destroy()
  }
}

/**
 * Converts Markdown into a document body.
 * @param {string} md
 * @returns {object} ProseMirror document JSON
 */
export function markdownToDoc(md) {
  return htmlToDoc(renderMarkdown(md || ''))
}

/**
 * Reads an exported body back.
 *
 * Accepts both a bare document (`{type:"doc"}`) and the full template envelope
 * the gallery exports (`{title, content}`) — the second is what a user gets
 * when they export a template, and refusing to read back our own export file
 * would be a trap.
 *
 * @param {string} text file contents
 * @returns {{content: object, title: string, description: string, icon: string}}
 * @throws {Error} when the file is not a document Tessera can read
 */
export function parseDocJSON(text) {
  let parsed
  try {
    parsed = JSON.parse(text)
  } catch {
    throw new Error(i18n.global.t('documents.file.notJson'))
  }
  const content = isDocJSON(parsed) ? parsed : parsed?.content
  if (!isDocJSON(content)) {
    throw new Error(i18n.global.t('documents.file.noTessera'))
  }
  return {
    content: ensureBlockIds(content),
    title: typeof parsed.title === 'string' ? parsed.title : '',
    description: typeof parsed.description === 'string' ? parsed.description : '',
    icon: typeof parsed.icon === 'string' ? parsed.icon : '',
  }
}

/**
 * Reads a picked file into a template draft.
 *
 * @param {File} file
 * @returns {Promise<{content: object, title: string, description: string, icon: string}>}
 * @throws {Error} on an unsupported extension, an oversized file or a body that
 *   does not parse
 */
export async function fileToTemplate(file) {
  if (!file) throw new Error(i18n.global.t('documents.file.notPicked'))
  const name = file.name || ''
  const ext = name.slice(name.lastIndexOf('.')).toLowerCase()
  if (!IMPORT_EXTENSIONS.includes(ext)) {
    throw new Error(
      i18n.global.t('documents.file.onlyFormats', { formats: IMPORT_EXTENSIONS.join(', ') }),
    )
  }
  if (file.size > MAX_IMPORT_BYTES) {
    // The limit is said in the reader's own units — "2 MB" beside "2 МБ" in the
    // viewer's file size would read as two different limits.
    throw new Error(
      i18n.global.t('documents.file.tooLarge', { limit: formatFileSize(MAX_IMPORT_BYTES) }),
    )
  }
  const text = await file.text()
  // Title falls back to the file name without its extension: an uploaded
  // template with an empty name is rejected by the server, and asking the user
  // to retype what they just picked is busywork.
  const fallbackTitle = name.slice(0, name.lastIndexOf('.')) || name
  if (ext === '.json') {
    const draft = parseDocJSON(text)
    return { ...draft, title: draft.title || fallbackTitle }
  }
  return {
    content: markdownToDoc(text),
    title: firstHeading(text) || fallbackTitle,
    description: '',
    icon: '',
  }
}

/**
 * First Markdown heading of a file, used as the template's name.
 * @param {string} md
 * @returns {string}
 */
export function firstHeading(md) {
  const line = String(md || '')
    .split('\n')
    .find((l) => /^\s{0,3}#{1,6}\s+\S/.test(l))
  return line ? line.replace(/^\s{0,3}#{1,6}\s+/, '').trim() : ''
}
