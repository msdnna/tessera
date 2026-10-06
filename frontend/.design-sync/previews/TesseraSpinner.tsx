import { TesseraSpinner } from '@tessera/design-system'

export const Sizes = () => (
  <div style={{ display: 'flex', gap: 24, alignItems: 'center' }}>
    <TesseraSpinner size={24} />
    <TesseraSpinner size={40} />
    <TesseraSpinner size={64} />
  </div>
)
export const WithLabel = () => <TesseraSpinner size={48} label="Загрузка доски…" />
export const OnAccent = () => (
  <div
    style={{
      background: 'var(--t-accent-grad)',
      padding: 24,
      borderRadius: 12,
      display: 'inline-flex',
    }}
  >
    <TesseraSpinner size={48} variant="white" />
  </div>
)
