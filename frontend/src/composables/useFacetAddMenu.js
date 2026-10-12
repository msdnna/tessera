import { ref, computed, unref } from 'vue'
import {
  AlbumsOutline,
  CalendarOutline,
  CreateOutline,
  FlagOutline,
  PeopleOutline,
  PricetagsOutline,
  SwapVerticalOutline,
  TrailSignOutline,
  EllipseOutline,
} from '@vicons/ionicons5'

// The composer's «+» menu (grouping / sort / every filter) as kit/ActionMenu
// rows (#3112): each section a submenu, an inline group (GitLab assignees) a
// heading inside it, the people with their faces, and whatever is already in
// force ticked. The phone gets the same rows — ActionMenu draws a submenu as
// the sheet's second screen with «‹ Назад» — so the drill this composable used
// to run by hand for the phone is gone.
//
// Shared by the composer's «+» (KanbanBoard) and the «Вид доски» panel's
// (BoardCustomizePanel), each with its own open state: one shared `show` would
// pop both menus at once, the hidden one anchored to a collapsed button.
//
// `options` is the nested menu (useBoardFacets' `addOptions`), `isOn(key)` says
// which rows to tick, `onPick` takes the key of a REAL facet.

const SECTION_ICONS = {
  group: AlbumsOutline,
  sort: SwapVerticalOutline,
  fs: EllipseOutline,
  fp: FlagOutline,
  fa: PeopleOutline,
  fc: CreateOutline,
  ft: PricetagsOutline,
  fm: TrailSignOutline,
  fd: CalendarOutline,
}

function row(o, isOn) {
  return {
    key: o.key,
    label: o.label,
    testid: o.props?.['data-testid'],
    checked: isOn(o.key),
    ...(o.avatarUserId || o.avatarSrc
      ? { avatar: { userId: o.avatarUserId, src: o.avatarSrc, name: o.label } }
      : {}),
  }
}

// One level of the menu: plain rows, a Naive `type: 'group'` turned into a
// heading followed by its rows.
export function facetRows(children = [], isOn = () => false) {
  const out = []
  for (const c of children) {
    if (c.type === 'group')
      out.push({ heading: c.label }, ...(c.children || []).map((x) => row(x, isOn)))
    else out.push(row(c, isOn))
  }
  return out
}

export function facetMenuItems(options = [], isOn = () => false) {
  return options.map((o) => ({
    key: o.key,
    label: o.label,
    icon: SECTION_ICONS[o.key],
    testid: o.props?.['data-testid'],
    subWidth: 260,
    children: facetRows(o.children, isOn),
  }))
}

export function useFacetAddMenu({ options, isOn = () => false, onPick }) {
  const show = ref(false)
  const items = computed(() => facetMenuItems(unref(options), isOn))
  function onSelect(key) {
    show.value = false
    onPick(key)
  }
  return { show, items, onSelect }
}
