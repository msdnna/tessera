import { Select } from '@tessera/design-system'

const priorities = [
  { label: 'Без приоритета', value: 0 },
  { label: 'Низкий', value: 1 },
  { label: 'Обычный', value: 2 },
  { label: 'Высокий', value: 3 },
  { label: 'Срочный', value: 4 },
]
const people = [
  { label: 'Мария Соколова', value: 'u1' },
  { label: 'Илья Петров', value: 'u2' },
  { label: 'Анна Ким', value: 'u3' },
]
const Col = ({ children }) => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: 10, width: 300 }}>{children}</div>
)

export const Single = () => (
  <Col>
    <Select options={priorities} defaultValue={3} />
    <Select options={priorities} placeholder="Приоритет" />
  </Col>
)
export const Multiple = () => (
  <Col>
    <Select options={people} multiple defaultValue={['u1', 'u3']} />
  </Col>
)
