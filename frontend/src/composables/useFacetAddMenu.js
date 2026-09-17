import { ref, computed, nextTick, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { ChevronBackOutline, ChevronForwardOutline } from '@vicons/ionicons5'
import UserAvatar from '@/components/UserAvatar.vue'

// The composer's «+» menu (grouping / sort / every filter) is two levels deep. On a
// desktop that is a normal n-dropdown with side submenus; on a phone a side submenu
// opens off the edge of the screen, so there the menu drills into one sub-list at a
// time with a «‹ Назад» row instead.
//
// This lived inline in KanbanBoard, wired to the composer's button alone — and the
// customize drawer, which renders the very same `addOptions` behind its own «+»,
// bound them raw: nested `children`, no drill, no custom rows. That drawer is the
// board's settings surface ON THE PHONE (the composer's gear slides away as soon as
// the chips fill the bar, and BoardMobileMenu's «Настроить вид» opens this drawer),
// so the one add menu a phone can always reach was the one that fanned submenus off
// the screen. Hence a composable: both call sites get the same behaviour, each with
// its own open/level state — one shared `show` ref would pop both dropdowns at once,
// the hidden one anchored to a collapsed button.
//
// `options` is the nested menu (KanbanBoard's `addOptions`), `isMobile` the viewport
// flag, `onPick` takes the key of a REAL facet — the drill's own `nav.*` keys are
// swallowed here and never reach it.
export function useFacetAddMenu({ options, isMobile, onPick }) {
  const { t } = useI18n()

  const show = ref(false)
  const level = ref(null) // null = top level; else the parent group's key
  const dir = ref('down') // slide direction for the level transition ('down' = deeper)

  const menuOptions = computed(() => {
    if (!isMobile.value) return options.value
    if (!level.value) {
      // Top level as drill-in entries (no side submenu).
      return options.value.map((o) => ({ label: o.label, key: `nav.${o.key}` }))
    }
    const parent = options.value.find((o) => o.key === level.value)
    return [
      { label: t('board.composer.back'), key: 'nav.back' },
      { type: 'divider', key: 'nav.div' },
      ...(parent?.children || []),
    ]
  })

  // Custom row rendering on mobile: right-side drill arrows, a spaced «‹ Назад»
  // header, and a vertical slide that plays whenever the level changes (keys differ
  // between levels → the option nodes remount → the animation replays).
  function renderLabel(option) {
    // Assignee-filter rows carry avatar hints — draw the user's face + name like the
    // on-card assignee picker (both desktop and mobile).
    if (option.avatarUserId || option.avatarSrc) {
      const av = h(UserAvatar, {
        class: 'flt-asgn-av',
        userId: option.avatarUserId,
        src: option.avatarSrc,
        name: option.label,
      })
      const txt = h('span', { class: 't-fdd-txt' }, option.label)
      if (!isMobile.value) return h('span', { class: 'flt-asgn' }, [av, txt])
      return h(
        'span',
        { class: 'flt-asgn t-fdd-row', style: { animation: `t-fdd-${dir.value} .17s ease` } },
        [av, txt],
      )
    }
    if (!isMobile.value) return option.label
    const anim = `t-fdd-${dir.value} .17s ease`
    const key = option.key
    if (key === 'nav.back') {
      return h('span', { class: 't-fdd-row is-back', style: { animation: anim } }, [
        h(NIcon, { component: ChevronBackOutline, size: 16 }),
        h('span', { class: 't-fdd-txt' }, option.label),
      ])
    }
    if (!level.value && typeof key === 'string' && key.startsWith('nav.')) {
      return h('span', { class: 't-fdd-row is-drill', style: { animation: anim } }, [
        h('span', { class: 't-fdd-txt' }, option.label),
        h(NIcon, { component: ChevronForwardOutline, size: 14, class: 't-fdd-arr' }),
      ])
    }
    return h('span', { class: 't-fdd-row', style: { animation: anim } }, [
      h('span', { class: 't-fdd-txt' }, option.label),
    ])
  }

  // n-dropdown auto-closes on every select. While drilling we ignore that close
  // (keep the controlled menu open) so the sub-list shows instead of snapping shut.
  let drilling = false
  function onSelect(key) {
    if (key === 'nav.back' || (typeof key === 'string' && key.startsWith('nav.'))) {
      dir.value = key === 'nav.back' ? 'up' : 'down'
      level.value = key === 'nav.back' ? null : key.slice(4)
      drilling = true
      nextTick(() => (drilling = false))
      return
    }
    onPick(key)
    show.value = false
    level.value = null
  }
  function onShow(v) {
    if (!v && drilling) return // swallow the select-triggered close mid-drill
    show.value = v
    if (!v) level.value = null // reset drill state when the menu actually closes
  }

  return { show, level, dir, menuOptions, renderLabel, onSelect, onShow }
}
