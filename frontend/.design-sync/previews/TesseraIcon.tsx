import { TesseraIcon } from '@tessera/design-system'

const Row = ({ children }) => (
  <div
    style={{
      display: 'flex',
      gap: 16,
      flexWrap: 'wrap',
      alignItems: 'center',
      color: 'var(--t-text2)',
    }}
  >
    {children}
  </div>
)

export const Layouts = () => (
  <Row>
    {['kanban', 'list', 'calendar', 'timeline', 'gantt', 'matrix'].map((n) => (
      <TesseraIcon key={n} name={`layout-${n}`} size={22} />
    ))}
  </Row>
)

export const Statuses = () => (
  <Row>
    <span style={{ color: '#9aa0aa' }}>
      <TesseraIcon name="status-todo" size={22} />
    </span>
    <span style={{ color: '#2f80ed' }}>
      <TesseraIcon name="status-progress" size={22} />
    </span>
    <span style={{ color: '#f0a020' }}>
      <TesseraIcon name="status-review" size={22} />
    </span>
    <span style={{ color: '#18a058' }}>
      <TesseraIcon name="status-done" variant="filled" size={22} />
    </span>
  </Row>
)

export const Variants = () => (
  <Row>
    {['outline', 'sharp', 'filled'].map((v) => (
      <TesseraIcon key={v} name="status-review" variant={v} size={24} />
    ))}
  </Row>
)
