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

/**
 * Insertable blocks offered by the "/" menu.
 * @type {Array<{key:string,label:string,hint:string,group:string,icon:object,keywords:string[],external?:boolean,apply?:Function}>}
 */
export const SLASH_ITEMS = [
  {
    key: 'paragraph',
    label: 'Текст',
    hint: 'Обычный абзац',
    group: 'Текст',
    icon: TextOutline,
    keywords: ['текст', 'абзац', 'параграф', 'text', 'paragraph', 'p'],
    apply: (chain) => chain.setParagraph().run(),
  },
  {
    key: 'h1',
    label: 'Заголовок 1',
    hint: 'Крупный заголовок',
    group: 'Текст',
    icon: TextOutline,
    keywords: ['заголовок', 'h1', 'heading', 'title'],
    apply: (chain) => chain.setNode('heading', { level: 1 }).run(),
  },
  {
    key: 'h2',
    label: 'Заголовок 2',
    hint: 'Средний заголовок',
    group: 'Текст',
    icon: TextOutline,
    keywords: ['заголовок', 'h2', 'heading', 'подзаголовок'],
    apply: (chain) => chain.setNode('heading', { level: 2 }).run(),
  },
  {
    key: 'h3',
    label: 'Заголовок 3',
    hint: 'Мелкий заголовок',
    group: 'Текст',
    icon: TextOutline,
    keywords: ['заголовок', 'h3', 'heading'],
    apply: (chain) => chain.setNode('heading', { level: 3 }).run(),
  },
  {
    key: 'blockquote',
    label: 'Цитата',
    hint: 'Выделенный блок текста',
    group: 'Текст',
    icon: ChatboxOutline,
    keywords: ['цитата', 'quote', 'blockquote'],
    apply: (chain) => chain.toggleBlockquote().run(),
  },
  {
    key: 'bulletList',
    label: 'Маркированный список',
    hint: 'Список с точками',
    group: 'Списки',
    icon: ListOutline,
    keywords: ['список', 'маркированный', 'list', 'bullet', 'ul'],
    apply: (chain) => chain.toggleBulletList().run(),
  },
  {
    key: 'orderedList',
    label: 'Нумерованный список',
    hint: 'Список по порядку',
    group: 'Списки',
    icon: ReorderFourOutline,
    keywords: ['список', 'нумерованный', 'номер', 'list', 'ordered', 'ol', 'number'],
    apply: (chain) => chain.toggleOrderedList().run(),
  },
  {
    key: 'taskList',
    label: 'Список задач',
    hint: 'Чекбоксы',
    group: 'Списки',
    icon: CheckboxOutline,
    keywords: ['задачи', 'чеклист', 'чекбокс', 'todo', 'task', 'check'],
    apply: (chain) => chain.toggleTaskList().run(),
  },
  {
    key: 'table',
    label: 'Таблица 3×3',
    hint: 'С шапкой',
    group: 'Вставка',
    icon: GridOutline,
    keywords: ['таблица', 'table', 'grid'],
    apply: (chain) => chain.insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(),
  },
  {
    key: 'horizontalRule',
    label: 'Разделитель',
    hint: 'Горизонтальная линия',
    group: 'Вставка',
    icon: RemoveOutline,
    keywords: ['разделитель', 'линия', 'hr', 'divider', 'rule'],
    apply: (chain) => chain.setHorizontalRule().run(),
  },
  {
    key: 'codeBlock',
    label: 'Блок кода',
    hint: 'Моноширинный блок',
    group: 'Вставка',
    icon: CodeSlashOutline,
    keywords: ['код', 'code', 'pre', 'блок кода'],
    apply: (chain) => chain.toggleCodeBlock().run(),
  },
  {
    key: 'image',
    label: 'Изображение',
    hint: 'Выбрать файл',
    group: 'Загрузка',
    icon: ImageOutline,
    // Opens the file picker, which lives in DocEditor — the menu only deletes
    // the typed "/…" and hands off.
    external: true,
    keywords: ['изображение', 'картинка', 'фото', 'image', 'picture', 'img'],
  },
  {
    key: 'pdf',
    label: 'PDF',
    hint: 'Вставить файл для чтения',
    group: 'Загрузка',
    icon: DocumentTextOutline,
    // Also external: a PDF is uploaded, not typed, so the menu hands off to the
    // picker in DocEditor exactly as the image entry does.
    external: true,
    keywords: ['pdf', 'пдф', 'файл', 'документ', 'скан'],
  },
]

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
