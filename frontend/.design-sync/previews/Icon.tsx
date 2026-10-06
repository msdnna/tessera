import { Icon } from '@tessera/design-system'

const names = [
  'AddOutline',
  'SearchOutline',
  'FilterOutline',
  'CalendarClearOutline',
  'FlagOutline',
  'PeopleOutline',
  'PricetagOutline',
  'ChatbubbleOutline',
  'EllipsisHorizontal',
  'TrashOutline',
  'SettingsOutline',
  'NotificationsOutline',
]
const Row = ({ children }) => (
  <div
    style={{
      display: 'flex',
      gap: 14,
      flexWrap: 'wrap',
      alignItems: 'center',
      color: 'var(--t-text2)',
    }}
  >
    {children}
  </div>
)

export const Glyphs = () => (
  <Row>
    {names.map((n) => (
      <Icon key={n} name={n} size={20} />
    ))}
  </Row>
)

export const AccentGradient = () => (
  <Row>
    {['FlashOutline', 'RocketOutline', 'StarOutline', 'CheckmarkCircleOutline'].map((n) => (
      <Icon key={n} name={n} size={24} gradient />
    ))}
  </Row>
)

export const Sizes = () => (
  <Row>
    {[12, 16, 20, 28].map((s) => (
      <Icon key={s} name="PricetagOutline" size={s} />
    ))}
  </Row>
)
