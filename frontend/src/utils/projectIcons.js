// project.icon can be: '' (→ initials), a curated key (below), raw '<svg…>'
// markup (an ionicon picked from the full set, or an uploaded SVG), or a
// 'data:image/…' URL (an uploaded raster icon).
import DOMPurify from 'dompurify'
import {
  BriefcaseOutline,
  HomeOutline,
  CodeSlashOutline,
  RocketOutline,
  SchoolOutline,
  CartOutline,
  HeartOutline,
  StarOutline,
  FlagOutline,
  ConstructOutline,
  BookOutline,
  FlaskOutline,
  BulbOutline,
  GameControllerOutline,
  AirplaneOutline,
  WalletOutline,
} from '@vicons/ionicons5'

export const PROJECT_ICONS = [
  { key: 'briefcase', component: BriefcaseOutline },
  { key: 'home', component: HomeOutline },
  { key: 'code', component: CodeSlashOutline },
  { key: 'rocket', component: RocketOutline },
  { key: 'school', component: SchoolOutline },
  { key: 'cart', component: CartOutline },
  { key: 'heart', component: HeartOutline },
  { key: 'star', component: StarOutline },
  { key: 'flag', component: FlagOutline },
  { key: 'construct', component: ConstructOutline },
  { key: 'book', component: BookOutline },
  { key: 'flask', component: FlaskOutline },
  { key: 'bulb', component: BulbOutline },
  { key: 'game', component: GameControllerOutline },
  { key: 'airplane', component: AirplaneOutline },
  { key: 'wallet', component: WalletOutline },
]

const BY_KEY = Object.fromEntries(PROJECT_ICONS.map((i) => [i.key, i.component]))

// iconComponent returns the ionicon for a curated key, or null otherwise.
export function iconComponent(key) {
  return key && BY_KEY[key] ? BY_KEY[key] : null
}

// Classify a stored icon value so the renderer knows how to draw it.
export function iconKind(icon) {
  if (!icon) return 'none'
  if (icon.startsWith('data:image')) return 'img'
  if (icon.trimStart().startsWith('<svg')) return 'svg'
  if (BY_KEY[icon]) return 'curated'
  return 'none'
}

// Sanitise SVG markup (uploaded or extracted) before it's rendered via v-html.
export function sanitizeIconSvg(svg) {
  return DOMPurify.sanitize(svg, { USE_PROFILES: { svg: true, svgFilters: true } })
}
