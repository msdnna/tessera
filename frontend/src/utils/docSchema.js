import { getStyleProperty } from '@tiptap/core'
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
import { InternalLink } from './docExtensions/internalLink'
import { PdfEmbed } from './docExtensions/pdfEmbed'
import { SlashMenu } from './docExtensions/slashMenu'
import { darkSheetFill, darkSheetInk, darkSheetLine, hexColor } from './docColor'

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
  // A PDF stays a file and is read in place (#2733) — see docPdf.js for why it
  // is not converted into blocks like every other imported format.
  'pdfEmbed',
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
  'backgroundColor',
  'borderColor',
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
  'name',
  'rel',
  'rowspan',
  'size',
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

// Text colour, with the dark theme accounted for at render time (#2755).
//
// Upstream's Color extension writes `style="color: X"` and nothing else. That
// is fine for a colour picked inside Tessera — the picker offers colours that
// work on both sheets — but an imported Word document brings colours chosen
// against white paper, and #1f4e79 on the dark sheet is unreadable.
//
// The stored attribute stays exactly what the document said; the painted colour
// is chosen per theme (see themed() below). Doing it the other way round —
// normalising the colour on import — would bake one theme's correction into the
// document and export it back to .docx as a colour the author never chose.
const inkCache = new Map()

function darkInk(color) {
  if (!inkCache.has(color)) inkCache.set(color, darkSheetInk(color))
  return inkCache.get(color)
}

/**
 * Writes a declaration whose value the theme can take over.
 *
 * The obvious shape — a plain `color: X` inline plus a dark-theme rule that
 * repaints it — cannot work: an inline declaration beats any author rule, so
 * such a rule is inert, which is what happened to the first cut of the imported
 * colours. Handing the *value* over to a custom property inverts the priority:
 * the fallback holds the document's own colour and applies while nothing sets
 * the property, and the dark theme wins simply by defining it (DocEditor.vue).
 *
 * @param {string} prop CSS property to write
 * @param {string} slot custom property the theme may override the value with
 * @param {string} value colour stored in the document
 * @param {string} dark the same colour, adapted to the dark sheet
 * @returns {string} declarations for a style attribute
 */
function themed(prop, slot, value, dark) {
  return `${prop}: var(${slot}, ${value}); ${slot}-dark: ${dark}`
}

// The document's own value, read back out of a declaration themed() wrote.
// Round-trips matter here: copying a table or a coloured run between two
// documents goes through the clipboard as HTML, and without this the stored
// colour would become the literal string "var(--doc-ink, #1f4e79)".
const THEMED_RE = /^var\(\s*(--doc-[a-z-]+)\s*,\s*(.+?)\s*\)$/i

function unthemed(value) {
  const css = String(value ?? '').trim()
  const m = css.match(THEMED_RE)
  return m ? m[2] : css
}

export const DocColor = Color.extend({
  addGlobalAttributes() {
    return [
      {
        types: this.options.types,
        attributes: {
          color: {
            default: null,
            parseHTML: (element) =>
              unthemed(
                (getStyleProperty(element, 'color') ?? element.style.color)?.replace(/['"]+/g, ''),
              ) || null,
            renderHTML: (attributes) => {
              if (!attributes.color) return {}
              return {
                style: themed('color', '--doc-ink', attributes.color, darkInk(attributes.color)),
              }
            },
          },
        },
      },
    ]
  },
})

// Table fills and borders brought in from an imported document (#2756).
//
// The converter writes them on the cell — `<td bgcolor="#d9e2f3" style="background:
// #d9e2f3; border: 1px solid #000000">` — and upstream's TableCell declares no
// attribute for either, so the header band and the grid of every imported table
// were flattened to the sheet's own styling at parse time.
//
// Same contract as the text colour above: the document stores what the file
// said, and the dark theme repaints from a custom property the render emits
// beside it (DocEditor.vue). Word's fills are pale because they were drawn under
// near-black ink, so on the dark sheet they need to move — but in the *document*
// they stay the author's colours, and an export back to .docx carries those.
const fillCache = new Map()
const lineCache = new Map()

function cached(map, fn, color) {
  if (!map.has(color)) map.set(color, fn(color))
  return map.get(color)
}

/**
 * Reads a colour off a cell, ignoring the "no colour" spellings.
 *
 * `transparent` and a zero-alpha rgba are what a cell without a fill
 * reserializes to; storing them would put an attribute on every cell of every
 * table and make the parity between imported and typed tables noise.
 *
 * @param {string} value CSS colour
 * @returns {string|null}
 */
function cellColor(value) {
  const css = unthemed(String(value || '').replace(/['"]+/g, '')).toLowerCase()
  if (!css || css === 'transparent' || css === 'initial' || css === 'currentcolor') return null
  if (/^rgba\(.*[\s,]0(\.0+)?\)$/i.test(css)) return null
  // A `background:` shorthand only reaches us through the CSSOM, which hands
  // back rgb() — the document should say what the file said.
  return hexColor(css)
}

const CELL_STYLE_ATTRS = {
  backgroundColor: {
    default: null,
    // Three sources, in the order they are trustworthy: the declaration as the
    // document wrote it (which is also how a themed value survives a paste),
    // the longhand the CSSOM derives from a `background:` shorthand, and the
    // legacy bgcolor attribute — the sidecar writes both, but the .html files
    // the same import route accepts often carry only the attribute.
    parseHTML: (el) =>
      cellColor(
        getStyleProperty(el, 'background-color') ||
          el.style.backgroundColor ||
          el.getAttribute('bgcolor'),
      ),
    renderHTML: (attrs) => {
      if (!attrs.backgroundColor) return {}
      const fill = attrs.backgroundColor
      return {
        style: themed(
          'background-color',
          '--doc-fill',
          fill,
          cached(fillCache, darkSheetFill, fill),
        ),
      }
    },
  },
  borderColor: {
    default: null,
    // The shorthand-shaped borderColor is empty as soon as the four sides
    // differ, so the top edge is the fallback rather than the primary source.
    parseHTML: (el) =>
      cellColor(
        getStyleProperty(el, 'border-color') || el.style.borderColor || el.style.borderTopColor,
      ),
    renderHTML: (attrs) => {
      if (!attrs.borderColor) return {}
      const line = attrs.borderColor
      return {
        style: themed('border-color', '--doc-line', line, cached(lineCache, darkSheetLine, line)),
      }
    },
  },
}

function withCellStyle(extension) {
  return extension.extend({
    addAttributes() {
      return { ...this.parent?.(), ...CELL_STYLE_ATTRS }
    },
  })
}

export const DocTableCell = withCellStyle(TableCell)
export const DocTableHeader = withCellStyle(TableHeader)

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
      // The insertion line drawn while a block is dragged. `color: false` is
      // what stops prosemirror-dropcursor writing an inline background-color —
      // its default is `currentColor`, which resolved to the body text colour
      // and drew a black line no theme could reach. Colour comes from the class
      // instead (.doc-dropcursor in DocEditor.vue), so it follows the accent.
      dropcursor: { color: false, width: 2, class: 'doc-dropcursor' },
    }),
    TaskList,
    TaskItem.configure({ nested: true }),
    Table.configure({ resizable: true }),
    TableRow,
    DocTableHeader,
    DocTableCell,
    Image.configure({ inline: false, allowBase64: false }),
    PdfEmbed,
    TextStyle,
    FontFamily,
    FontSize,
    DocColor,
    TextAlign.configure({ types: STYLED_TYPES }),
    BlockId,
    BlockStyle,
    BlockMove,
    BlockLocks.configure({ onBlocked: opts.onBlocked || (() => {}) }),
    BlockComments.configure({ onSelect: opts.onSelectComments || (() => {}) }),
    // Internal links carry no schema of their own — they are ordinary link
    // marks whose href is a block id — so this only changes what a click does.
    InternalLink,
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
