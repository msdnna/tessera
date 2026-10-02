import { TagPill, TesseraProvider } from '@tessera/design-system'

const tags = [
  { name: 'app::frontend', color: '#7c5cff' },
  { name: 'app::backend', color: '#2f80ed' },
  { name: 'bug', color: '#e0533d' },
  { name: 'design', color: '#eb2f96' },
  { name: 'ready', color: '#18a058' },
]

const Row = ({ children }) => (
  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>{children}</div>
)

export const Outline = () => (
  <Row>
    {tags.map((t) => (
      <TagPill key={t.name} tag={t} />
    ))}
  </Row>
)

export const Variants = () => (
  <Row>
    <TagPill tag={tags[3]} variant="outline" />
    <TagPill tag={tags[3]} variant="solid" />
    <TagPill tag={tags[3]} variant="soft" />
    <TagPill tag={tags[3]} variant="ghost" />
    <TagPill tag={tags[3]} variant="grad-text" />
    <TagPill tag={tags[3]} variant="plain" />
  </Row>
)

export const ScopeLabels = () => (
  <Row>
    <TagPill tag={tags[0]} prefixNames={{ 'app::': 'Приложение' }} />
    <TagPill tag={tags[1]} scopeMode="hide" />
  </Row>
)

export const Dark = () => (
  <TesseraProvider mode="dark">
    <div style={{ background: 'var(--t-surface)', padding: 12, borderRadius: 10 }}>
      <Row>
        {tags.map((t) => (
          <TagPill key={t.name} tag={t} />
        ))}
      </Row>
    </div>
  </TesseraProvider>
)
