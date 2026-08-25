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
import { i18n } from '@/i18n'

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
const ALIGNMENT_DEFS = [
  { value: 'left', vicon: 'align-left' },
  { value: 'center', vicon: 'align-center' },
  { value: 'right', vicon: 'align-right' },
  { value: 'justify', vicon: 'align-justify' },
]

/** The four alignments with their labels in the current language. */
export function alignments() {
  return ALIGNMENT_DEFS.map((a) => ({
    ...a,
    label: i18n.global.t(`documents.toolbar.align.${a.value}`),
  }))
}

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
  // Read on every call, like the labels themselves: a caller that memoises the
  // panel does so in a computed, and calling `t` there is what makes the panel
  // re-render when the language changes.
  const t = i18n.global.t
  return [
    {
      key: 'history',
      items: [
        {
          key: 'undo',
          title: t('documents.toolbar.undo'),
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
          title: t('documents.toolbar.redo'),
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
        title: t('documents.toolbar.heading', { level }),
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
          title: t('documents.toolbar.bold'),
          cls: 'b',
          text: 'B',
          run: () => chain().toggleBold().run(),
          isActive: () => editor.isActive('bold'),
        },
        {
          key: 'italic',
          title: t('documents.toolbar.italic'),
          cls: 'i',
          text: 'I',
          run: () => chain().toggleItalic().run(),
          isActive: () => editor.isActive('italic'),
        },
        {
          key: 'underline',
          title: t('documents.toolbar.underline'),
          cls: 'u',
          text: 'U',
          run: () => chain().toggleUnderline().run(),
          isActive: () => editor.isActive('underline'),
        },
        {
          key: 'strike',
          title: t('documents.toolbar.strike'),
          cls: 's',
          text: 'S',
          run: () => chain().toggleStrike().run(),
          isActive: () => editor.isActive('strike'),
        },
        {
          key: 'code',
          title: t('documents.toolbar.code'),
          icon: CodeSlashOutline,
          run: () => chain().toggleCode().run(),
          isActive: () => editor.isActive('code'),
        },
      ],
    },
    {
      key: 'align',
      items: alignments().map((a) => ({
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
          title: t('documents.toolbar.bulletList'),
          icon: ListOutline,
          run: () => chain().toggleBulletList().run(),
          isActive: () => editor.isActive('bulletList'),
        },
        {
          key: 'orderedList',
          title: t('documents.toolbar.orderedList'),
          icon: ReorderFourOutline,
          run: () => chain().toggleOrderedList().run(),
          isActive: () => editor.isActive('orderedList'),
        },
        // Moved here from the "Вставка" tab: a checklist is a kind of list, and
        // splitting the three across two tabs made one choice into two screens.
        {
          key: 'taskList',
          title: t('documents.toolbar.taskList'),
          icon: CheckboxOutline,
          run: () => chain().toggleTaskList().run(),
          isActive: () => editor.isActive('taskList'),
        },
        {
          key: 'indent',
          title: t('documents.toolbar.indent'),
          icon: ReturnDownForwardOutline,
          run: () => chain().indent().run(),
          isActive: () => false,
        },
        {
          key: 'outdent',
          title: t('documents.toolbar.outdent'),
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
          title: t('documents.toolbar.table'),
          icon: GridOutline,
          run: () => chain().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(),
          isActive: () => editor.isActive('table'),
        },
        {
          key: 'image',
          title: t('documents.toolbar.image'),
          icon: ImageOutline,
          external: true,
          run: () => handlers.onPickImage?.(),
          isActive: () => editor.isActive('image'),
        },
        {
          key: 'link',
          title: t('documents.toolbar.link'),
          icon: LinkOutline,
          external: true,
          run: () => handlers.onSetLink?.(),
          isActive: () => editor.isActive('link'),
        },
        {
          key: 'codeBlock',
          title: t('documents.toolbar.codeBlock'),
          icon: CodeSlashOutline,
          run: () => chain().toggleCodeBlock().run(),
          isActive: () => editor.isActive('codeBlock'),
        },
        {
          key: 'blockquote',
          title: t('documents.toolbar.blockquote'),
          icon: ChatboxOutline,
          run: () => chain().toggleBlockquote().run(),
          isActive: () => editor.isActive('blockquote'),
        },
        {
          key: 'horizontalRule',
          title: t('documents.toolbar.horizontalRule'),
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
 * Line height for display, in the reader's number format.
 *
 * Was a hand-rolled `.` → `,` swap, which is right for Russian and wrong for
 * every locale that keeps the point. Intl decides instead; only the label is
 * localised — the stored value stays '1.5', because the lineHeight attribute is
 * compared and written verbatim by blockStyle.
 *
 * @param {string|number} value the stored line height
 * @param {string} [locale] override, for tests
 */
export function lineHeightLabel(value, locale = i18n.global.locale.value) {
  const n = Number(value)
  if (!Number.isFinite(n) || value === '' || value == null) return String(value ?? '')
  return new Intl.NumberFormat(locale).format(n)
}
