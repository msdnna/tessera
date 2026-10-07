import { InlineEdit } from '@tessera/design-system'

// A sidebar tree row / a column header around the field (v2 metrics).
const Row = ({ children }) => (
  <div
    style={{
      display: 'flex',
      alignItems: 'center',
      height: 32,
      padding: '0 4px 0 42px',
      width: 232,
      boxSizing: 'border-box',
      background: 'var(--t-sidebar)',
      borderRadius: 8,
    }}
  >
    {children}
  </div>
)
const Stack = ({ children }) => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: 48, padding: '8px 8px 48px' }}>
    {children}
  </div>
)

export const Rename = () => (
  <Stack>
    <Row>
      <InlineEdit defaultValue="Бэклог продукта" />
    </Row>
  </Stack>
)

export const NewRow = () => (
  <Stack>
    <Row>
      <InlineEdit placeholder="Название доски" hint="createView" />
    </Row>
    <Row>
      <InlineEdit placeholder="Название группы" hint="create" />
    </Row>
  </Stack>
)

export const ErrorAndBusy = () => (
  <Stack>
    <Row>
      <InlineEdit defaultValue="Баги" error="Доска «Баги» уже есть в проекте" />
    </Row>
    <Row>
      <InlineEdit defaultValue="Бэклог 2026" busy showHint={false} />
    </Row>
  </Stack>
)

export const ColumnHeader = () => (
  <Stack>
    <div
      style={{
        width: 256,
        padding: 8,
        borderRadius: 18,
        background: 'color-mix(in srgb, #2f80ed var(--t-col-tone), var(--t-col))',
        boxSizing: 'border-box',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', height: 30, paddingLeft: 6 }}>
        <InlineEdit variant="header" hint="column" defaultValue="В процессе" />
      </div>
    </div>
  </Stack>
)
