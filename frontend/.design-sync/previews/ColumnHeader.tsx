import { ColumnHeader } from '@tessera/design-system'

// A v2 lane (see conventions): the column colour washed over --t-col, no border.
const Col = ({ color = '#9aa0aa', children }) => (
  <div
    style={{
      width: 256,
      boxSizing: 'border-box',
      padding: 8,
      borderRadius: 18,
      background: `color-mix(in srgb, ${color} var(--t-col-tone), var(--t-col))`,
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
    <Col color="#0eb0a9">
      <ColumnHeader
        column={{ id: 'todo', name: 'К работе', color: '#0eb0a9' }}
        count={7}
        first
        editable
      />
    </Col>
    <Col color="#2f80ed">
      <ColumnHeader
        column={{ id: 'doing', name: 'В процессе', color: '#2f80ed' }}
        count={4}
        editable
      />
    </Col>
    <Col color="#f0a020">
      <ColumnHeader
        column={{ id: 'review', name: 'На рассмотрении', color: '#f0a020' }}
        count={2}
        editable
      />
    </Col>
    <Col color="#18a058">
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
    <Col color="#2f80ed">
      <ColumnHeader
        column={{ id: 'doing', name: 'В процессе', color: '#2f80ed' }}
        count={5}
        total={12}
        editable
      />
    </Col>
    <Col color="#0eb0a9">
      <ColumnHeader
        column={{ id: 'sprint', name: 'Релиз 0.150', color: '#0eb0a9' }}
        count={9}
        estimate="34 ч"
      />
    </Col>
  </Stack>
)

// Grouped by tag (the killer feature): each lane is a tag, titled with its pill;
// no status glyph, no column menu. The catch-all «Без тегов» lane stays neutral.
export const TagColumns = () => (
  <Stack>
    <Col color="#7c5cff">
      <ColumnHeader
        column={{
          id: 't1',
          name: 'App: frontend',
          color: '#7c5cff',
          tag: { name: 'app::frontend', color: '#7c5cff' },
        }}
        prefixNames={{ 'app::': 'App' }}
        count={6}
      />
    </Col>
    <Col color="#eb2f96">
      <ColumnHeader
        column={{
          id: 't4',
          name: 'design',
          color: '#eb2f96',
          tag: { name: 'design', color: '#eb2f96' },
        }}
        count={3}
      />
    </Col>
    <Col>
      <ColumnHeader column={{ id: '__none__', name: 'Без тегов' }} count={2} />
    </Col>
  </Stack>
)
