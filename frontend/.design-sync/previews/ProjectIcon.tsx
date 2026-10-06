import { ProjectIcon, hueGrad } from '@tessera/design-system'

// Sidebar badge recipe (ProjectRow .picon): 22px, 5px radius, hue gradient fill,
// white 13px glyph. In "icon" colour mode the box is transparent and the glyph is tinted.
const Badge = ({ color, children }) => (
  <span
    style={{
      width: 22,
      height: 22,
      borderRadius: 5,
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      color: '#fff',
      fontSize: 10,
      fontWeight: 700,
      background: color ? hueGrad(color) : 'transparent',
    }}
  >
    {children}
  </span>
)
const Row = ({ children }) => (
  <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>{children}</div>
)

export const Badges = () => (
  <Row>
    <Badge color="#7c5cff">
      <ProjectIcon icon="code" size={13} />
    </Badge>
    <Badge color="#2f80ed">
      <ProjectIcon icon="rocket" size={13} />
    </Badge>
    <Badge color="#18a058">
      <ProjectIcon icon="home" size={13} />
    </Badge>
    <Badge color="#f0a020">
      <ProjectIcon icon="wallet" size={13} />
    </Badge>
    <Badge color="#eb2f96">
      <ProjectIcon initials="ДР" size={13} />
    </Badge>
  </Row>
)

export const IconMode = () => (
  <Row>
    <Badge>
      <ProjectIcon icon="briefcase" size={14} color="#7c5cff" />
    </Badge>
    <Badge>
      <ProjectIcon icon="flask" size={14} color="#0eb0a9" />
    </Badge>
    <Badge>
      <ProjectIcon icon="star" size={14} color="#e0533d" />
    </Badge>
  </Row>
)
