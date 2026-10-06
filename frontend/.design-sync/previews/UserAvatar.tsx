import { UserAvatar } from '@tessera/design-system'

const Row = ({ children }) => (
  <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>{children}</div>
)

export const Initials = () => (
  <Row>
    <UserAvatar name="Мария Соколова" />
    <UserAvatar name="Илья Петров" />
    <UserAvatar name="Анна Ким" />
  </Row>
)
export const Sizes = () => (
  <Row>
    {[20, 24, 32, 48].map((s) => (
      <UserAvatar key={s} name="Мария Соколова" size={s} />
    ))}
  </Row>
)
export const Stack = () => (
  <div style={{ display: 'flex', alignItems: 'center' }}>
    <UserAvatar name="Мария Соколова" stacked />
    <UserAvatar name="Илья Петров" stacked />
    <UserAvatar name="Анна Ким" stacked />
  </div>
)
