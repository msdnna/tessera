// One stacking authority for every floating layer (#3034, #3063).
//
// Naive UI hands its layers (modals, drawers, select menus, date-picker panels,
// popovers) a growing z-index from vdirs' manager: 2000, 2001, … — and resets
// the counter only when no layer at all is registered. Our own layers used to
// sit on constants (PopoverForm 2050, BottomSheet 2100, select menus pinned at
// 2200), so their order against Naive's depended on how many layers had been
// opened since the page loaded: a date panel came up under a PopoverForm, and
// after a long session a modal outgrew 2200 and its role selects opened behind
// it (a reload "fixed" it by resetting the counter).
//
// `v-layer` registers the element in the same manager on mount and releases it
// on unmount, so whatever opens last is on top — ours or Naive's. Put it on the
// v-if'd root of a layer: `<div v-if="show" v-layer …>`; pass `{ zIndex }` to pin
// an explicit value instead.
import { zindexable } from 'vdirs'

export const vLayer = {
  mounted(el, binding) {
    zindexable.mounted(el, { value: { enabled: true, ...binding.value } })
  },
  updated() {},
  unmounted(el, binding) {
    zindexable.unmounted(el, { value: { ...binding.value } })
  },
}
