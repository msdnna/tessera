import { ColorIconPicker } from '@tessera/design-system'

const Surface = ({ children }) => (
  <div style={{ width: 316, padding: 16, borderRadius: 16, background: 'var(--t-pop)' }}>
    {children}
  </div>
)

export const Project = () => (
  <Surface>
    <ColorIconPicker name="Tessera Web" icon="code" color="#7c5cff" set="project" />
  </Surface>
)

export const SpaceInitials = () => (
  <Surface>
    <ColorIconPicker name="Студия Север" icon="" color="#2f80ed" set="space" />
  </Surface>
)

export const Orange = () => (
  <Surface>
    <ColorIconPicker name="Маркетинг" icon="rocket" color="#f0a020" set="project" />
  </Surface>
)
