<script setup>
// The one tag chip. A scoped tag ("effort::small") renders as a GitLab-EE-style
// two-segment pill — «scope │ value» — with the scope muted and a hairline
// divider; an unscoped tag renders as a plain single-segment chip. The scope
// side shows the configured friendly prefix name when there is one, else the
// raw prefix.
//
// The pill owns only its *inner* layout plus the variant's colour treatment;
// size, padding, radius and font come from the call site's own class (.chip,
// .mchip, .tagchip, …), which falls through to the root. That keeps every
// existing chip geometry intact while the segment rendering lives in one place.
//
// NB for callers with a hidden measurement row (useTagFit): the measure copies
// must render <TagPill> with the same props, or the measured natural widths
// won't include the scope segment and the fit calculation will lie.
import { computed } from 'vue'
import { useThemeStore } from '@/stores/theme'
import { tagParts } from '@/utils/tagGroups'
import { hueGrad, tagPillBg, softFill, readableHue, onColor } from '@/utils/gradient'

const props = defineProps({
  // A tag row ({ name, color }) or a bare tag name string.
  tag: { type: [Object, String], default: null },
  // Colour override — used when `tag` is a string.
  color: { type: String, default: '' },
  // Canonical prefix → friendly label.
  prefixNames: { type: Object, default: () => ({}) },
  // Box + text treatment:
  //   outline   — gradient border + soft fill + gradient text (cards, task modal)
  //   solid     — filled hue gradient, contrast text (a selected picker chip)
  //   soft      — soft tint + coloured border (an unselected picker chip)
  //   ghost     — translucent tint, coloured text (hover previews, Home)
  //   grad-text — no box, gradient text; the call site's element *is* the pill
  //   plain     — no box, flat readable text; the call site styles the box
  //   inherit   — no box, no colour at all; inherits the call site's text colour
  //               (a picker chip that flips fill/text on selection)
  variant: { type: String, default: 'outline' },
  // 'auto' shows the scope segment when the tag has one; 'hide' suppresses it
  // (inside a picker already grouped by scope, where it would just repeat).
  scopeMode: { type: String, default: 'auto' },
})

const theme = useThemeStore()

const rawName = computed(() => (typeof props.tag === 'string' ? props.tag : props.tag?.name || ''))
const hue = computed(
  () => (typeof props.tag === 'string' ? props.color : props.tag?.color) || '#888',
)
// Tag colours come from GitLab too, where they may be unreadable as *text* on
// the active theme — clamp lightness into a legible band first.
const textHue = computed(() => readableHue(hue.value, theme.isDark))

const parts = computed(() => tagParts(rawName.value, props.prefixNames))
const showScope = computed(() => parts.value.hasScope && props.scopeMode !== 'hide')
const gradText = computed(() => props.variant === 'outline' || props.variant === 'grad-text')

const rootStyle = computed(() => {
  const c = hue.value
  // The divider is the tag's own hue, faded — a hairline, not a second fill.
  const base = { '--tp-sep': `color-mix(in srgb, ${textHue.value} 42%, transparent)` }
  switch (props.variant) {
    case 'solid':
      return { ...base, background: hueGrad(c), color: onColor(c), borderColor: 'transparent' }
    case 'soft':
      return { ...base, background: softFill(c), color: textHue.value, borderColor: `${c}66` }
    case 'ghost':
      return { ...base, background: `${c}22`, color: textHue.value }
    case 'plain':
      return { ...base, color: textHue.value }
    case 'grad-text':
      return base
    // Colour is whatever the call site set — so is the divider.
    case 'inherit':
      return { '--tp-sep': 'color-mix(in srgb, currentColor 42%, transparent)' }
    default:
      return { ...base, border: '1px solid transparent', background: tagPillBg(c, true) }
  }
})
// Each segment carries its own gradient span, so the muted scope can use plain
// `opacity` — nesting opacity *inside* one background-clip:text element would
// fight the clip instead of just dimming the glyphs.
const gradStyle = computed(() => ({ '--grad': hueGrad(textHue.value) }))
</script>

<template>
  <span class="tpill" :class="{ scoped: showScope }" :style="rootStyle" :title="rawName">
    <span
      v-if="showScope"
      class="tp-scope"
      :class="{ 'accent-grad-text': gradText }"
      :style="gradText ? gradStyle : null"
      >{{ parts.scope }}</span
    ><span
      class="tp-name"
      :class="{ 'accent-grad-text': gradText }"
      :style="gradText ? gradStyle : null"
      >{{ parts.label }}</span
    >
  </span>
</template>

<style scoped>
/* Inline-flex so the two segments and the divider share one baseline box; the
   call site still owns padding/radius/font-size via its own class. */
.tpill {
  display: inline-flex;
  align-items: center;
  min-width: 0;
}
.tp-scope,
.tp-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* Muted scope: lighter weight + a touch of transparency, so the value stays the
   thing the eye lands on. */
.tp-scope {
  font-weight: 500;
  opacity: 0.78;
}
.scoped .tp-name {
  border-left: 1px solid var(--tp-sep, var(--t-border));
  margin-left: 0.42em;
  padding-left: 0.42em;
}
</style>
