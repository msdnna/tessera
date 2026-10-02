import { Tooltip, Button, Icon } from '@tessera/design-system'

export const Pinned = () => (
  <div style={{ padding: '56px 120px 12px' }}>
    <Tooltip content="Срок истёк 2 дня назад" show placement="top">
      <Button size="small" icon={<Icon name="CalendarClearOutline" />}>
        28 сен
      </Button>
    </Tooltip>
  </div>
)
export const OnIcon = () => (
  <div style={{ padding: '56px 120px 12px' }}>
    <Tooltip content="Фильтры" show>
      <Button quaternary circle icon={<Icon name="FilterOutline" />} />
    </Tooltip>
  </div>
)
