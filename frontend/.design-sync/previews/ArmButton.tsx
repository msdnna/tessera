import { ArmButton } from '@tessera/design-system'

const Row = ({ children }) => (
  <div
    style={{
      display: 'flex',
      alignItems: 'center',
      gap: 10,
      width: 360,
      padding: '8px 10px',
      borderRadius: 10,
      background: 'var(--t-card)',
      color: 'var(--t-text1)',
      fontSize: 13.5,
    }}
  >
    {children}
  </div>
)

export const Idle = () => (
  <Row>
    <span style={{ flex: 1 }}>макет-доски-v2.png · 412 КБ</span>
    <ArmButton title="Удалить файл" />
  </Row>
)
export const Armed = () => (
  <Row>
    <span style={{ flex: 1 }}>макет-доски-v2.png · 412 КБ</span>
    <ArmButton title="Удалить файл" armed />
  </Row>
)
