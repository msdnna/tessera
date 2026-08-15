import {
  ChatboxOutline,
  CheckboxOutline,
  CodeSlashOutline,
  GridOutline,
  ImageOutline,
  LinkOutline,
  ListOutline,
  RemoveOutline,
  ReorderFourOutline,
  ReturnDownBackOutline,
  ReturnDownForwardOutline,
} from '@vicons/ionicons5'

// The toolbar is described as data rather than markup so that every button is
// reachable from a test. The commit that removed TipTap from this project the
// first time (ed63159) listed "dead toolbar buttons" among its reasons and the
// cause was never recorded — a button whose command does nothing is exactly the
// failure this shape makes testable: cx-doc-toolbar.spec.js walks these lists
// and asserts each `run` either changes the document or reports it could not.
//
// `external: true` marks the two buttons whose work happens outside the editor
// (file picker, link prompt); the test asserts the handler is invoked instead.

/**
 * Formatting tools — the "Главная" tab.
 * @param {object} editor TipTap editor instance
 * @param {object} [handlers]
 * @param {Function} [handlers.onSetLink] opens the link prompt
 */
export function homeTools(editor, handlers = {}) {
  const chain = () => editor.chain().focus()
  return [
    {
      key: 'bold',
      title: 'Жирный (Ctrl+B)',
      cls: 'b',
      text: 'B',
      run: () => chain().toggleBold().run(),
      isActive: () => editor.isActive('bold'),
    },
    {
      key: 'italic',
      title: 'Курсив (Ctrl+I)',
      cls: 'i',
      text: 'I',
      run: () => chain().toggleItalic().run(),
      isActive: () => editor.isActive('italic'),
    },
    {
      key: 'underline',
      title: 'Подчёркнутый (Ctrl+U)',
      cls: 'u',
      text: 'U',
      run: () => chain().toggleUnderline().run(),
      isActive: () => editor.isActive('underline'),
    },
    {
      key: 'strike',
      title: 'Зачёркнутый',
      cls: 's',
      text: 'S',
      run: () => chain().toggleStrike().run(),
      isActive: () => editor.isActive('strike'),
    },
    {
      key: 'code',
      title: 'Моноширинный',
      icon: CodeSlashOutline,
      run: () => chain().toggleCode().run(),
      isActive: () => editor.isActive('code'),
    },
    {
      key: 'h1',
      title: 'Заголовок 1',
      text: 'H1',
      run: () => chain().toggleHeading({ level: 1 }).run(),
      isActive: () => editor.isActive('heading', { level: 1 }),
    },
    {
      key: 'h2',
      title: 'Заголовок 2',
      text: 'H2',
      run: () => chain().toggleHeading({ level: 2 }).run(),
      isActive: () => editor.isActive('heading', { level: 2 }),
    },
    {
      key: 'h3',
      title: 'Заголовок 3',
      text: 'H3',
      run: () => chain().toggleHeading({ level: 3 }).run(),
      isActive: () => editor.isActive('heading', { level: 3 }),
    },
    {
      key: 'bulletList',
      title: 'Маркированный список',
      icon: ListOutline,
      run: () => chain().toggleBulletList().run(),
      isActive: () => editor.isActive('bulletList'),
    },
    {
      key: 'orderedList',
      title: 'Нумерованный список',
      icon: ReorderFourOutline,
      run: () => chain().toggleOrderedList().run(),
      isActive: () => editor.isActive('orderedList'),
    },
    {
      key: 'indent',
      title: 'Увеличить отступ',
      icon: ReturnDownForwardOutline,
      run: () => chain().indent().run(),
      isActive: () => false,
    },
    {
      key: 'outdent',
      title: 'Уменьшить отступ',
      icon: ReturnDownBackOutline,
      run: () => chain().outdent().run(),
      isActive: () => false,
    },
    {
      key: 'link',
      title: 'Ссылка',
      icon: LinkOutline,
      external: true,
      run: () => handlers.onSetLink?.(),
      isActive: () => editor.isActive('link'),
    },
  ]
}

/**
 * Insert tools — the "Вставка" tab.
 * @param {object} editor TipTap editor instance
 * @param {object} [handlers]
 * @param {Function} [handlers.onPickImage] opens the image picker
 */
export function insertTools(editor, handlers = {}) {
  const chain = () => editor.chain().focus()
  return [
    {
      key: 'table',
      title: 'Таблица 3×3',
      icon: GridOutline,
      run: () => chain().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(),
      isActive: () => editor.isActive('table'),
    },
    {
      key: 'image',
      title: 'Изображение',
      icon: ImageOutline,
      external: true,
      run: () => handlers.onPickImage?.(),
      isActive: () => editor.isActive('image'),
    },
    {
      key: 'taskList',
      title: 'Список задач',
      icon: CheckboxOutline,
      run: () => chain().toggleTaskList().run(),
      isActive: () => editor.isActive('taskList'),
    },
    {
      key: 'codeBlock',
      title: 'Блок кода',
      icon: CodeSlashOutline,
      run: () => chain().toggleCodeBlock().run(),
      isActive: () => editor.isActive('codeBlock'),
    },
    {
      key: 'blockquote',
      title: 'Цитата',
      icon: ChatboxOutline,
      run: () => chain().toggleBlockquote().run(),
      isActive: () => editor.isActive('blockquote'),
    },
    {
      key: 'horizontalRule',
      title: 'Разделитель',
      icon: RemoveOutline,
      run: () => chain().setHorizontalRule().run(),
      isActive: () => false,
    },
  ]
}

export const ALIGNMENTS = [
  { label: 'По левому краю', value: 'left' },
  { label: 'По центру', value: 'center' },
  { label: 'По правому краю', value: 'right' },
  { label: 'По ширине', value: 'justify' },
]
