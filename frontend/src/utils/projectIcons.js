// Curated ionicons5 set for project icons. project.icon stores the `key`;
// empty → render initials. (No emoji — themed vector icons only.)
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

// iconComponent returns the ionicon for a stored key, or null if none/unknown.
export function iconComponent(key) {
  return key ? BY_KEY[key] || null : null
}
