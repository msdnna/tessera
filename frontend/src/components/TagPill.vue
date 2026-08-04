<script setup>
// The one tag chip. A scoped tag ("effort::small") renders as a GitLab-EE-style
// two-segment pill — «scope value» — where the scope is a *filled* accent
// segment with contrast text (like a selected picker chip) and the value is a
// soft-tinted segment with accent (gradient) text (like a normal tag); the whole
// pill is bordered in the accent hue. An unscoped tag renders as a plain
// single-segment chip. The scope side shows the configured friendly prefix name
// when there is one, else the raw prefix.
//
// The pill owns only its *inner* layout plus the variant's colour treatment;
// size, radius and font come from the call site's own class (.chip, .mchip,
// .tagchip, …), which falls through to the root. For the two-tone scoped pill the
// call site's padding yields to the pill's own segments (see the `.tt` styles).
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
// GitLab-EE two-segment treatment for a scoped tag. The picker (`inherit`)
// manages its own selected/unselected fill, so it opts out and falls back to the
// muted-scope + hairline-divider look.
const twoTone = computed(() => showScope.value && props.variant !== 'inherit')
const gradText = computed(() => props.variant === 'outline' || props.variant === 'grad-text')

const rootStyle = computed(() => {
  const c = hue.value
  // The divider is the tag's own hue, faded — a hairline, not a second fill.
  const base = { '--tp-sep': `color-mix(in srgb, ${textHue.value} 42%, transparent)` }
  if (twoTone.value) {
    // Two bordered segments form the pill (the root carries no box), so each
    // rounds its own outer corners — robust vs the border-box + overflow clip
    // that squared the value corner in the hover preview. The shared border is
    // the accent hue (= the scope fill).
    return {
      border: 'none',
      background: 'none',
      '--tp-scope-bg': hueGrad(c),
      '--tp-scope-fg': onColor(c),
      '--tp-name-bg': softFill(c),
      '--tp-bd': textHue.value,
      '--grad': hueGrad(textHue.value),
    }
  }
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
// A gradient span for accent text. Each segment carries its own so the muted
// scope can use plain `opacity` — nesting opacity *inside* one background-clip:text
// element would fight the clip instead of just dimming the glyphs.
const gradStyle = computed(() => ({ '--grad': hueGrad(textHue.value) }))
</script>

<template>
  <span
    class="tpill"
    :class="{ scoped: showScope, tt: twoTone }"
    :style="rootStyle"
    :title="rawName"
  >
    <span
      v-if="showScope"
      class="tp-scope"
      :class="{ 'accent-grad-text': gradText && !twoTone }"
      :style="gradText && !twoTone ? gradStyle : null"
      >{{ parts.scope }}</span
    ><span v-if="twoTone" class="tp-name"
      ><span class="tp-name-txt accent-grad-text" :style="gradStyle">{{ parts.label }}</span></span
    ><span
      v-else
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
/* Muted scope + hairline divider — the non two-tone scoped case (flat picker). */
.scoped:not(.tt) .tp-scope {
  font-weight: 500;
  opacity: 0.78;
}
.scoped:not(.tt) .tp-name {
  border-left: 1px solid var(--tp-sep, var(--t-border));
  margin-left: 0.42em;
  padding-left: 0.42em;
}
/* GitLab-EE two-tone scoped pill: two bordered segments (filled scope + soft
   value) that each round their own outer corners, so it matches the card's
   other pills (small 6px radius, full row height) with no overflow clipping.
   The call site's padding yields to the segments; the pill stretches to the row
   height — e.g. the 22px single-tag button — via align-self. */
.tt {
  padding: 0;
  align-self: stretch;
  align-items: stretch;
}
.tt .tp-scope,
.tt .tp-name {
  display: inline-flex;
  align-items: center;
  border: 1px solid var(--tp-bd, var(--t-border));
}
.tt .tp-scope {
  background: var(--tp-scope-bg);
  color: var(--tp-scope-fg);
  font-weight: 600;
  padding: 1px 7px;
  border-right: none;
  border-radius: 6px 0 0 6px;
}
.tt .tp-name {
  background: var(--tp-name-bg);
  padding: 1px 8px;
  border-left: none;
  border-radius: 0 6px 6px 0;
}
.tt .tp-name-txt {
  display: inline;
}
</style>
