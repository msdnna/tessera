import {
  ArrowRedoOutline,
  ArrowUndoOutline,
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
// failure this shape makes testable: cx-doc-toolbar.spec.js walks these groups
// and asserts each `run` either changes the document or reports it could not.
// Laying the buttons out in the template instead would take them out from under
// that guard, which is why the flat panel is still built from data.
//
// `external: true` marks the two buttons whose work happens outside the editor
// (file picker, link prompt); the test asserts the handler is invoked instead.
// `disabled()` marks a button that is legitimately inert right now (undo with an
// empty history) — the guard skips those instead of treating them as dead.

/** Text alignment: four exclusive values, shown as icons rather than a select. */
export const ALIGNMENTS = [
  { label: 'По левому краю', value: 'left', vicon: 'align-left' },
  { label: 'По центру', value: 'center', vicon: 'align-center' },
  { label: 'По правому краю', value: 'right', vicon: 'align-right' },
  { label: 'По ширине', value: 'justify', vicon: 'align-justify' },
]

/**
 * Every toolbar command, grouped the way the single panel lays them out.
 *
 * The order is the reading order of the panel and the separators fall between
 * the groups — with the tabs gone they are the only thing telling the eye where
 * one kind of tool ends and the next begins.
 *
 * The 'selects' group carries no items: the font, size and line-height pickers
 * are naive-ui selects rendered by the component. They stay selects because
 * their value sets are open-ended, unlike the four alignments.
 *
 * @param {object} editor TipTap editor instance
 * @param {object} [handlers]
 * @param {Function} [handlers.onSetLink] opens the link prompt
 * @param {Function} [handlers.onPickImage] opens the image picker
 * @returns {Array<{key: string, kind?: string, items: Array<object>}>}
 */
export function toolbarGroups(editor, handlers = {}) {
  const chain = () => editor.chain().focus()
  return [
    {
      key: 'history',
      items: [
        {
          key: 'undo',
          title: 'Отменить (Ctrl+Z)',
          icon: ArrowUndoOutline,
          run: () => chain().undo().run(),
          isActive: () => false,
          // Without this the button looks live in a fresh document and silently
          // does nothing on click — the "dead toolbar button" failure again,
          // only self-inflicted.
          disabled: () => !editor.can().undo(),
        },
        {
          key: 'redo',
          title: 'Повторить (Ctrl+Shift+Z)',
          icon: ArrowRedoOutline,
          run: () => chain().redo().run(),
          isActive: () => false,
          disabled: () => !editor.can().redo(),
        },
      ],
    },
    {
      key: 'headings',
      items: [1, 2, 3].map((level) => ({
        key: `h${level}`,
        title: `Заголовок ${level}`,
        text: `H${level}`,
        run: () => chain().toggleHeading({ level }).run(),
        isActive: () => editor.isActive('heading', { level }),
      })),
    },
    { key: 'selects', kind: 'selects', items: [] },
    {
      key: 'format',
      items: [
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
      ],
    },
    {
      key: 'align',
      items: ALIGNMENTS.map((a) => ({
        key: `align-${a.value}`,
        title: a.label,
        vicon: a.vicon,
        run: () => chain().setTextAlign(a.value).run(),
        isActive: () => editor.isActive({ textAlign: a.value }),
      })),
    },
    {
      key: 'lists',
      items: [
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
        // Moved here from the "Вставка" tab: a checklist is a kind of list, and
        // splitting the three across two tabs made one choice into two screens.
        {
          key: 'taskList',
          title: 'Список задач',
          icon: CheckboxOutline,
          run: () => chain().toggleTaskList().run(),
          isActive: () => editor.isActive('taskList'),
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
      ],
    },
    {
      key: 'insert',
      items: [
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
          key: 'link',
          title: 'Ссылка',
          icon: LinkOutline,
          external: true,
          run: () => handlers.onSetLink?.(),
          isActive: () => editor.isActive('link'),
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
      ],
    },
  ]
}

/** Every command in the panel, flattened — the shape tests walk. */
export function toolbarCommands(editor, handlers = {}) {
  return toolbarGroups(editor, handlers).flatMap((g) => g.items)
}

/**
 * Line height for display: Russian decimals use a comma.
 *
 * Only the label is localised — the stored value stays '1.5', because the
 * lineHeight attribute is compared and written verbatim by blockStyle.
 */
export function lineHeightLabel(value) {
  return String(value ?? '').replace('.', ',')
}
