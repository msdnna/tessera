import { Input, Icon } from '@tessera/design-system'

const Col = ({ children }) => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: 10, width: 320 }}>{children}</div>
)

export const Text = () => (
  <Col>
    <Input placeholder="Название задачи" />
    <Input defaultValue="Редизайн доски" clearable />
    <Input placeholder="Поиск по задачам" prefix={<Icon name="SearchOutline" />} />
  </Col>
)
export const Sizes = () => (
  <Col>
    <Input size="small" placeholder="Small" />
    <Input placeholder="Medium" />
    <Input size="large" placeholder="Large" />
  </Col>
)
export const Textarea = () => (
  <Col>
    <Input type="textarea" placeholder="Комментарий" autosize={{ minRows: 3 }} />
  </Col>
)
export const Disabled = () => (
  <Col>
    <Input defaultValue="Только чтение" disabled />
  </Col>
)
