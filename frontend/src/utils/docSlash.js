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

/**
 * Insertable blocks offered by the "/" menu.
 * @type {Array<{key:string,label:string,hint:string,icon:object,keywords:string[],external?:boolean,apply?:Function}>}
 */
export const SLASH_ITEMS = [
  {
    key: 'paragraph',
    label: 'Текст',
    hint: 'Обычный абзац',
    icon: TextOutline,
    keywords: ['текст', 'абзац', 'параграф', 'text', 'paragraph', 'p'],
    apply: (chain) => chain.setParagraph().run(),
  },
  {
    key: 'h1',
    label: 'Заголовок 1',
    hint: 'Крупный заголовок',
    icon: TextOutline,
    keywords: ['заголовок', 'h1', 'heading', 'title'],
    apply: (chain) => chain.setNode('heading', { level: 1 }).run(),
  },
  {
    key: 'h2',
    label: 'Заголовок 2',
    hint: 'Средний заголовок',
    icon: TextOutline,
    keywords: ['заголовок', 'h2', 'heading', 'подзаголовок'],
    apply: (chain) => chain.setNode('heading', { level: 2 }).run(),
  },
  {
    key: 'h3',
    label: 'Заголовок 3',
    hint: 'Мелкий заголовок',
    icon: TextOutline,
    keywords: ['заголовок', 'h3', 'heading'],
    apply: (chain) => chain.setNode('heading', { level: 3 }).run(),
  },
  {
    key: 'bulletList',
    label: 'Маркированный список',
    hint: 'Список с точками',
    icon: ListOutline,
    keywords: ['список', 'маркированный', 'list', 'bullet', 'ul'],
    apply: (chain) => chain.toggleBulletList().run(),
  },
  {
    key: 'orderedList',
    label: 'Нумерованный список',
    hint: 'Список по порядку',
    icon: ReorderFourOutline,
    keywords: ['список', 'нумерованный', 'номер', 'list', 'ordered', 'ol', 'number'],
    apply: (chain) => chain.toggleOrderedList().run(),
  },
  {
    key: 'taskList',
    label: 'Список задач',
    hint: 'Чекбоксы',
    icon: CheckboxOutline,
    keywords: ['задачи', 'чеклист', 'чекбокс', 'todo', 'task', 'check'],
    apply: (chain) => chain.toggleTaskList().run(),
  },
  {
    key: 'blockquote',
    label: 'Цитата',
    hint: 'Выделенный блок текста',
    icon: ChatboxOutline,
    keywords: ['цитата', 'quote', 'blockquote'],
    apply: (chain) => chain.toggleBlockquote().run(),
  },
  {
    key: 'codeBlock',
    label: 'Блок кода',
    hint: 'Моноширинный блок',
    icon: CodeSlashOutline,
    keywords: ['код', 'code', 'pre', 'блок кода'],
    apply: (chain) => chain.toggleCodeBlock().run(),
  },
  {
    key: 'table',
    label: 'Таблица 3×3',
    hint: 'С шапкой',
    icon: GridOutline,
    keywords: ['таблица', 'table', 'grid'],
    apply: (chain) => chain.insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(),
  },
  {
    key: 'horizontalRule',
    label: 'Разделитель',
    hint: 'Горизонтальная линия',
    icon: RemoveOutline,
    keywords: ['разделитель', 'линия', 'hr', 'divider', 'rule'],
    apply: (chain) => chain.setHorizontalRule().run(),
  },
  {
    key: 'image',
    label: 'Изображение',
    hint: 'Выбрать файл',
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
