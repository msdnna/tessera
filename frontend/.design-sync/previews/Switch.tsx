import { Switch } from '@tessera/design-system'

const Row = ({ label, children }) => (
  <label
    style={{
      display: 'flex',
      alignItems: 'center',
      gap: 10,
      color: 'var(--t-text2)',
      fontSize: 14,
    }}
  >
    {children}
    {label}
  </label>
)

export const States = () => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
    <Row label="Уведомлять о сроках">
      <Switch defaultValue />
    </Row>
    <Row label="Показывать пустые поля">
      <Switch defaultValue={false} />
    </Row>
    <Row label="Недоступно">
      <Switch defaultValue disabled />
    </Row>
  </div>
)
export const Sizes = () => (
  <div style={{ display: 'flex', gap: 14, alignItems: 'center' }}>
    <Switch size="small" defaultValue />
    <Switch defaultValue />
    <Switch size="large" defaultValue />
  </div>
)
