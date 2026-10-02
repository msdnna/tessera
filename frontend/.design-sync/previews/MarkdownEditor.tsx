import { MarkdownEditor } from '@tessera/design-system'

const text = `## План\n\n- [x] Токены поверхностей\n- [ ] Колонки без бордеров\n\nРазделяем блоки **поверхностями** и отступами, а не линиями.`
const Box = ({ children }) => <div style={{ width: 520 }}>{children}</div>

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
export const Composer = () => (
  <Box>
    <MarkdownEditor variant="boxed" send placeholder="Написать комментарий…" minRows={2} />
  </Box>
)
