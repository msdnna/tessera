import { StarterKit } from '@tiptap/starter-kit'
import { TaskItem, TaskList } from '@tiptap/extension-list'
import { Table, TableCell, TableHeader, TableRow } from '@tiptap/extension-table'
import { Image } from '@tiptap/extension-image'
import { Color, FontFamily, FontSize, TextStyle } from '@tiptap/extension-text-style'
import { TextAlign } from '@tiptap/extension-text-align'
import { Placeholder } from '@tiptap/extensions'
import { BlockId } from './docExtensions/blockId'
import { BlockComments } from './docExtensions/blockComments'
import { BlockLocks } from './docExtensions/blockLocks'
import { BlockMove } from './docExtensions/blockMove'
import { BlockStyle, STYLED_TYPES } from './docExtensions/blockStyle'
import { ImageDrop } from './docExtensions/imageDrop'
import { SlashMenu } from './docExtensions/slashMenu'

// The document is stored as the ProseMirror JSON tree the editor produces
// (documents.content, jsonb — chosen in D1). That makes the schema itself the
// allow-list: a node type the schema does not declare simply does not exist
// after parsing, which is stronger than a hand-maintained list of HTML tags.
//
// The same list is enforced server-side in backend/handlers/document_schema.go
// — the schema only protects clients that go through this frontend, and the
// PATCH endpoint accepts arbitrary JSON otherwise. The two lists must agree;
// ut-docSchema.spec.js checks this one against the extensions actually loaded,
// and documents_content_flow_test.go checks the Go one against the same names.

export const EMPTY_DOC = { type: 'doc', content: [] }

export const ALLOWED_NODES = [
  'doc',
  'paragraph',
  'text',
  'heading',
  'bulletList',
  'orderedList',
  'listItem',
  'taskList',
  'taskItem',
  'blockquote',
  'codeBlock',
  'horizontalRule',
  'hardBreak',
  'image',
  'table',
  'tableRow',
  'tableHeader',
  'tableCell',
]

export const ALLOWED_MARKS = ['bold', 'italic', 'strike', 'underline', 'code', 'link', 'textStyle']

// Attribute names the extensions above can put on a node or a mark. This list
// exists because the node/mark parity check was not enough: the server rejects
// unknown *attributes* too, and two of them ('align' on table cells, 'type' on
// ordered lists) are added by TipTap itself rather than by anything in this
// file. A document containing a table then failed to save with a 400, and
// nothing on either side noticed until an e2e run typed "/таблица" (#2728).
// ut-docSchema.spec.js derives the real set from the loaded extensions and
// compares it to this list, so the next such attribute cannot arrive silently.
export const ALLOWED_ATTRS = [
  'align',
  'alt',
  'checked',
  'class',
  'color',
  'colspan',
  'colwidth',
  'fontFamily',
  'fontSize',
  'height',
  'href',
  'id',
  'indent',
  'language',
  'level',
  'lineHeight',
  'rel',
  'rowspan',
  'src',
  'start',
  'target',
  'textAlign',
  'title',
  'type',
  'width',
]

export const FONT_FAMILIES = [
  { label: 'По умолчанию', value: '' },
  { label: 'Системный', value: 'system-ui, -apple-system, Segoe UI, Roboto, sans-serif' },
  { label: 'С засечками', value: 'Georgia, "Times New Roman", serif' },
  { label: 'Моноширинный', value: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace' },
]

export const FONT_SIZES = ['12px', '14px', '16px', '18px', '24px', '32px']

/**
 * Builds the extension set for the document editor.
 *
 * @param {object} [opts]
 * @param {string} [opts.placeholder] placeholder for the empty document
 * @param {Function} [opts.uploadImage] uploads a File, resolves to its URL
 * @param {Function} [opts.onUploadError] reports a failed upload to the user
 * @param {Function} [opts.onSlashExternal] runs a slash item handled outside
 *   the editor (currently only the image picker)
 * @param {Function} [opts.onBlocked] reports an edit refused by a block lock
 * @param {Function} [opts.onSelectComments] a discussed block was clicked, so
 *   the panel can bring its thread into view
 * @returns {Array} TipTap extensions
 */
export function docExtensions(opts = {}) {
  return [
    StarterKit.configure({
      // The link mark keeps TipTap's protocol validation; opening links from
      // inside the editor is the renderer's job, not the editor's.
      link: { openOnClick: false, autolink: true },
      // Trailing node would append an empty paragraph on every load, which
      // makes a freshly opened document dirty and triggers an autosave.
      trailingNode: false,
    }),
    TaskList,
    TaskItem.configure({ nested: true }),
    Table.configure({ resizable: true }),
    TableRow,
    TableHeader,
    TableCell,
    Image.configure({ inline: false, allowBase64: false }),
    TextStyle,
    FontFamily,
    FontSize,
    Color,
    TextAlign.configure({ types: STYLED_TYPES }),
    BlockId,
    BlockStyle,
    BlockMove,
    BlockLocks.configure({ onBlocked: opts.onBlocked || (() => {}) }),
    BlockComments.configure({ onSelect: opts.onSelectComments || (() => {}) }),
    ImageDrop.configure({ upload: opts.uploadImage || null, onError: opts.onUploadError || null }),
    SlashMenu.configure({ onExternal: opts.onSlashExternal || null }),
    Placeholder.configure({ placeholder: opts.placeholder || 'Начните писать…' }),
  ]
}

/**
 * Reports whether a stored value is a usable ProseMirror document.
 * @param {*} json parsed content
 */
export function isDocJSON(json) {
  return !!json && typeof json === 'object' && json.type === 'doc'
}

/**
 * Normalises whatever the API returned into a document the editor can load.
 * @param {*} json parsed content
 */
export function toDocJSON(json) {
  return isDocJSON(json) ? json : EMPTY_DOC
}

/**
 * Extracts plain text from a document tree, used for previews and for the
 * "is this document empty" check.
 *
 * @param {object} json ProseMirror document JSON
 * @param {number} [limit] stop once this many characters are collected
 * @returns {string} plain text with block boundaries collapsed to single spaces
 */
export function docPlainText(json, limit = 400) {
  const out = []
  let len = 0
  const walk = (node) => {
    if (!node || len >= limit) return
    if (node.type === 'text' && node.text) {
      out.push(node.text)
      len += node.text.length
      return
    }
    if (Array.isArray(node.content)) {
      for (const child of node.content) {
        walk(child)
        if (len >= limit) return
      }
      // Block boundary: without it "первый" + "второй" reads as one word.
      out.push(' ')
      len += 1
    }
  }
  walk(json)
  return out.join('').replace(/\s+/g, ' ').trim().slice(0, limit)
}

/**
 * True when the document has no text and no media — used to show the empty
 * state on a tile instead of a blank preview.
 * @param {object} json ProseMirror document JSON
 */
export function isEmptyDoc(json) {
  if (!isDocJSON(json)) return true
  if (docPlainText(json, 1)) return false
  let media = false
  const walk = (node) => {
    if (media || !node) return
    if (node.type === 'image' || node.type === 'table' || node.type === 'horizontalRule') {
      media = true
      return
    }
    if (Array.isArray(node.content)) node.content.forEach(walk)
  }
  walk(json)
  return !media
}
