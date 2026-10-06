import { Checkbox } from '@tessera/design-system'

export const States = () => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
    <Checkbox defaultChecked>Токены поверхностей</Checkbox>
    <Checkbox>Колонки без бордеров</Checkbox>
    <Checkbox indeterminate>Часть подзадач</Checkbox>
    <Checkbox disabled>Недоступно</Checkbox>
  </div>
)
