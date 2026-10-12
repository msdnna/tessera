// Ionicons of the tour's chapters (data/getStarted.js names them by string, the
// way the palette's catalog does) — a fixed map keeps tree-shaking honest.
import {
  DocumentTextOutline,
  FileTrayOutline,
  FlagOutline,
  FolderOutline,
  GiftOutline,
  GitBranchOutline,
  KeypadOutline,
  PeopleOutline,
  PhonePortraitOutline,
  PricetagsOutline,
  SparklesOutline,
} from '@vicons/ionicons5'

export const TOUR_ICONS = {
  DocumentTextOutline,
  FileTrayOutline,
  FlagOutline,
  FolderOutline,
  GiftOutline,
  GitBranchOutline,
  KeypadOutline,
  PeopleOutline,
  PhonePortraitOutline,
  PricetagsOutline,
  SparklesOutline,
}

export const tourIcon = (name) => TOUR_ICONS[name] || SparklesOutline
