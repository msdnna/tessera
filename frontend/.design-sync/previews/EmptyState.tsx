import { EmptyState } from '@tessera/design-system'

const Box = ({ w = 360, children }) => (
  <div style={{ width: w, background: 'var(--t-surface)', borderRadius: 12, padding: 8 }}>
    {children}
  </div>
)

export const Medium = () => (
  <Box>
    <EmptyState text="В этой колонке пока нет задач" />
  </Box>
)
export const Small = () => (
  <Box w={260}>
    <EmptyState icon="SearchOutline" text="Ничего не найдено" size="small" />
  </Box>
)
