import { ColumnHeader } from '@tessera/design-system'

const Col = ({ children }) => (
  <div
    style={{
      width: 280,
      boxSizing: 'border-box',
      padding: 10,
      borderRadius: 14,
      background: 'var(--t-surface-alt)',
    }}
  >
    {children}
  </div>
)
const Stack = ({ children }) => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>{children}</div>
)

export const Statuses = () => (
  <Stack>
    <Col>
      <ColumnHeader
        column={{ id: 'todo', name: 'К работе', color: '#9aa0aa' }}
        count={7}
        first
        editable
      />
    </Col>
    <Col>
      <ColumnHeader
        column={{ id: 'doing', name: 'В процессе', color: '#2f80ed' }}
        count={4}
        editable
      />
    </Col>
    <Col>
      <ColumnHeader
        column={{ id: 'review', name: 'На рассмотрении', color: '#f0a020' }}
        count={2}
        editable
      />
    </Col>
    <Col>
      <ColumnHeader
        column={{ id: 'done', name: 'Готово', color: '#18a058' }}
        count={23}
        isDone
        editable
      />
    </Col>
  </Stack>
)

export const CountsAndEstimate = () => (
  <Stack>
    <Col>
      <ColumnHeader
        column={{ id: 'doing', name: 'В процессе', color: '#7c5cff' }}
        count={5}
        total={12}
        editable
      />
    </Col>
    <Col>
      <ColumnHeader
        column={{ id: 'sprint', name: 'Релиз 0.150', color: '#0eb0a9' }}
        count={9}
        estimate="34 ч"
      />
    </Col>
  </Stack>
)
