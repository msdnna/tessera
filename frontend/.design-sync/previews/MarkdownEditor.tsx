import { MarkdownEditor } from '@tessera/design-system'

const text = `## План\n\n- [x] Токены поверхностей\n- [ ] Колонки без бордеров\n\nРазделяем блоки **поверхностями** и отступами, а не линиями.`
const desc = `Сейчас блоки доски разделены 1px-линиями. Переходим на **лестницу поверхностей**.\n\n### Что сделать\n\n- Токены \`--t-*\` для обеих тем\n- [x] макет в Claude Design\n- [ ] проверка в тёмной теме — см. #9`
const Box = ({ children, w = 520 }) => <div style={{ width: w }}>{children}</div>

export const DescriptionEdit = () => (
  <Box w={640}>
    <MarkdownEditor variant="v2" defaultValue={desc} minRows={6} />
  </Box>
)
export const CommentComposer = () => (
  <Box w={640}>
    <MarkdownEditor
      variant="composer"
      send
      avatar={{ name: 'Аня Ковалёва' }}
      placeholder="Комментарий… @ — упомянуть, / — команды"
      minRows={1}
    />
  </Box>
)
export const Write = () => (
  <Box>
    <MarkdownEditor defaultValue={text} />
  </Box>
)
export const Preview = () => (
  <Box>
    <MarkdownEditor defaultValue={text} initialMode="preview" />
  </Box>
)
export const NoteBoxed = () => (
  <Box>
    <MarkdownEditor variant="boxed" send placeholder="Написать заметку…" minRows={2} />
  </Box>
)
