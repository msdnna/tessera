import {
  ChatboxOutline,
  CheckboxOutline,
  CodeSlashOutline,
  DocumentTextOutline,
  GridOutline,
  ImageOutline,
  ListOutline,
  RemoveOutline,
  ReorderFourOutline,
  TextOutline,
} from '@vicons/ionicons5'
import { i18n } from '@/i18n'

// The slash menu is described as data for the same reason the toolbar is
// (see docToolbar.js): every entry has to be reachable from a test, because a
// command that silently does nothing looks identical to one that worked.
//
// `apply` takes a chain rather than the editor so that deleting the typed "/…"
// and running the command land in one transaction — otherwise a single Ctrl+Z
// undoes the block and leaves the "/table" text behind.
//
// Keywords carry both languages: the interface is Russian, but "/table" is what
// hands trained on Notion actually type.
//
// The list stays FLAT even though the menu draws it in groups: the filter, the
// arrow-key cursor and the tests all index into it, and a nested shape would
// make the highlighted index mean two different things. `group` is a label on
// the item; grouping happens on the way out, in groupSlashItems.
//
// Declaration order therefore follows group order. That is not cosmetic either:
// the arrow keys walk this array, so an item drawn in the second group but
// declared first would make the cursor jump around the menu.
//
// The definitions below carry no text: `label`, `hint` and the group heading are
// produced by slashItems() on every call, because a module-level array would
// freeze the menu in whatever language was loaded first (pitfall 1 of #2799).
// The keywords are NOT translated — they are the typing aliases, and a Russian
// interface should still answer to "/table".

/**
 * Insertable blocks offered by the "/" menu, in menu order.
 * @type {Array<{key:string,group:string,icon:object,keywords:string[],external?:boolean,apply?:Function}>}
 */
const SLASH_DEFS = [
  {
    key: 'paragraph',
    group: 'text',
    icon: TextOutline,
    keywords: ['текст', 'абзац', 'параграф', 'text', 'paragraph', 'p'],
    apply: (chain) => chain.setParagraph().run(),
  },
  {
    key: 'h1',
    group: 'text',
    icon: TextOutline,
    keywords: ['заголовок', 'h1', 'heading', 'title'],
    apply: (chain) => chain.setNode('heading', { level: 1 }).run(),
  },
  {
    key: 'h2',
    group: 'text',
    icon: TextOutline,
    keywords: ['заголовок', 'h2', 'heading', 'подзаголовок'],
    apply: (chain) => chain.setNode('heading', { level: 2 }).run(),
  },
  {
    key: 'h3',
    group: 'text',
    icon: TextOutline,
    keywords: ['заголовок', 'h3', 'heading'],
    apply: (chain) => chain.setNode('heading', { level: 3 }).run(),
  },
  {
    key: 'blockquote',
    group: 'text',
    icon: ChatboxOutline,
    keywords: ['цитата', 'quote', 'blockquote'],
    apply: (chain) => chain.toggleBlockquote().run(),
  },
  {
    key: 'bulletList',
    group: 'lists',
    icon: ListOutline,
    keywords: ['список', 'маркированный', 'list', 'bullet', 'ul'],
    apply: (chain) => chain.toggleBulletList().run(),
  },
  {
    key: 'orderedList',
    group: 'lists',
    icon: ReorderFourOutline,
    keywords: ['список', 'нумерованный', 'номер', 'list', 'ordered', 'ol', 'number'],
    apply: (chain) => chain.toggleOrderedList().run(),
  },
  {
    key: 'taskList',
    group: 'lists',
    icon: CheckboxOutline,
    keywords: ['задачи', 'чеклист', 'чекбокс', 'todo', 'task', 'check'],
    apply: (chain) => chain.toggleTaskList().run(),
  },
  {
    key: 'table',
    group: 'insert',
    icon: GridOutline,
    keywords: ['таблица', 'table', 'grid'],
    apply: (chain) => chain.insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(),
  },
  {
    key: 'horizontalRule',
    group: 'insert',
    icon: RemoveOutline,
    keywords: ['разделитель', 'линия', 'hr', 'divider', 'rule'],
    apply: (chain) => chain.setHorizontalRule().run(),
  },
  {
    key: 'codeBlock',
    group: 'insert',
    icon: CodeSlashOutline,
    keywords: ['код', 'code', 'pre', 'блок кода'],
    apply: (chain) => chain.toggleCodeBlock().run(),
  },
  {
    key: 'image',
    group: 'upload',
    icon: ImageOutline,
    // Opens the file picker, which lives in DocEditor — the menu only deletes
    // the typed "/…" and hands off.
    external: true,
    keywords: ['изображение', 'картинка', 'фото', 'image', 'picture', 'img'],
  },
  {
    key: 'pdf',
    group: 'upload',
    icon: DocumentTextOutline,
    // Also external: a PDF is uploaded, not typed, so the menu hands off to the
    // picker in DocEditor exactly as the image entry does.
    external: true,
    keywords: ['pdf', 'пдф', 'файл', 'документ', 'скан'],
  },
]

/**
 * The menu items with their text filled in for the current language.
 *
 * Built on every call rather than memoised: the caller (slashMenu.js) rebuilds
 * the list on each keystroke anyway, and a cached copy would outlive a language
 * switch — the exact freeze SLASH_DEFS exists to avoid.
 *
 * @returns {Array<{key:string,label:string,hint:string,group:string,icon:object,keywords:string[],external?:boolean,apply?:Function}>}
 */
export function slashItems() {
  const t = i18n.global.t
  return SLASH_DEFS.map((item) => ({
    ...item,
    label: t(`documents.slash.${item.key}.label`),
    hint: t(`documents.slash.${item.key}.hint`),
    group: t(`documents.slash.group.${item.group}`),
  }))
}

/**
 * Narrows the menu to what the user typed after the slash.
 *
 * Substring rather than prefix matching: "/спис" and "/list" should both reach
 * "Маркированный список", and only one of those is a prefix of the label.
 *
 * @param {Array} items the full item list
 * @param {string} query text typed after "/"
 * @returns {Array} matching items, in the declared order
 */
export function filterSlashItems(items, query) {
  const q = (query || '').trim().toLowerCase()
  if (!q) return items
  return items.filter(
    (i) => i.label.toLowerCase().includes(q) || i.keywords.some((k) => k.includes(q)),
  )
}

/**
 * Buckets items for display, in first-seen group order.
 *
 * Display only — the flat list stays the source of truth. A group the filter
 * emptied is dropped rather than drawn headless, so "/спис" shows "Списки" and
 * nothing else instead of three empty headings.
 *
 * @param {Array} items items to draw, already filtered
 * @returns {Array<{group:string,items:Array}>}
 */
export function groupSlashItems(items) {
  const out = []
  const byName = new Map()
  for (const item of items || []) {
    const name = item.group || ''
    let bucket = byName.get(name)
    if (!bucket) {
      bucket = { group: name, items: [] }
      byName.set(name, bucket)
      out.push(bucket)
    }
    bucket.items.push(item)
  }
  return out
}
