<script setup>
// One tool of the editor's bars (#3118, PackageJ «Строка Aa · тултип с
// клавишей»): a 28px glyph or icon button whose tooltip names the action and its
// key — «Жирный ⌘B». Used by the «Aa» strip, the v2 bar, the selection bubble
// and the fullscreen editor's toolbar.
//
// It fires on mousedown (default prevented), not on click: the caret and the
// selection must stay in the field the tool acts on. A keyboard press reaches
// the button as a click with no pointer behind it (detail 0) and fires too.
import { NIcon } from 'naive-ui'
import ShortcutKeys from '../ShortcutKeys.vue'

const props = defineProps({
  label: { type: String, required: true },
  // Catalog id of the key the tooltip shows (editor.bold …), or ''.
  keyId: { type: String, default: '' },
  icon: { type: [Object, Function], default: null },
  glyph: { type: String, default: '' },
  // Glyph style: 'b' | 'i' | 's' | 'num' | 'quote'.
  glyphClass: { type: String, default: '' },
  iconSize: { type: Number, default: 15 },
  on: { type: Boolean, default: false },
  busy: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  // Where the tooltip opens: above the bar or under the bubble.
  tip: { type: String, default: 'top' },
  // The phone's bigger targets (40px).
  big: { type: Boolean, default: false },
  testid: { type: String, default: '' },
})
const emit = defineEmits(['press'])

function fire(e) {
  if (props.disabled || props.busy) return
  emit('press', e)
}
</script>

<template>
  <button
    type="button"
    class="etb"
    :class="[{ on, busy, big, dis: disabled }, `tip-${tip}`]"
    :aria-label="label"
    :aria-pressed="on || undefined"
    :aria-disabled="disabled || undefined"
    :data-testid="testid || undefined"
    @mousedown.prevent="fire"
    @click="(e) => e.detail === 0 && fire(e)"
  >
    <n-icon v-if="icon" :component="icon" :size="big ? 18 : iconSize" />
    <span v-else class="etb-g" :class="glyphClass">{{ glyph }}</span>
    <slot />
    <span class="etb-tip" role="tooltip">
      <span class="etb-tip-l">{{ label }}</span>
      <ShortcutKeys v-if="keyId" :id="keyId" />
    </span>
  </button>
</template>

<style scoped>
.etb {
  position: relative;
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  min-width: 28px;
  height: 28px;
  padding: 0 6px;
  box-sizing: border-box;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  line-height: 1;
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.etb:hover,
.etb.on {
  background: var(--t-hover);
  color: var(--t-text1);
}
.etb:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: -2px;
}
.etb.busy {
  opacity: 0.5;
  pointer-events: none;
}
.etb.dis {
  color: var(--t-text3);
  cursor: default;
}
.etb.dis:hover {
  background: transparent;
}
.etb.big {
  min-width: 40px;
  height: 40px;
  border-radius: 9px;
  font-size: 15px;
}
.etb-g {
  font-size: 13.5px;
  font-weight: 700;
}
.big .etb-g {
  font-size: 15px;
}
.etb-g.i {
  font-style: italic;
  font-family: Georgia, serif;
}
.etb-g.s {
  font-weight: 600;
  text-decoration: line-through;
}
.etb-g.num {
  font-size: 12.5px;
}
.etb-g.quote {
  font-size: 17px;
  line-height: 1;
  margin-top: 6px;
}
/* «Жирный ⌘B»: a pop chip over (or under) the button after a short hover. */
.etb-tip {
  position: absolute;
  left: -6px;
  z-index: 3;
  display: flex;
  align-items: center;
  gap: 8px;
  height: 30px;
  padding: 0 6px 0 10px;
  border-radius: 10px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
  font-size: 12px;
  font-weight: 400;
  font-style: normal;
  text-decoration: none;
  white-space: nowrap;
  pointer-events: none;
  opacity: 0;
  visibility: hidden;
  transition:
    opacity 0.12s ease,
    visibility 0s linear 0.12s;
}
.tip-top .etb-tip {
  bottom: calc(100% + 6px);
}
.tip-bottom .etb-tip {
  top: calc(100% + 10px);
}
.etb:hover .etb-tip,
.etb:focus-visible .etb-tip {
  opacity: 1;
  visibility: visible;
  transition:
    opacity 0.12s ease 0.35s,
    visibility 0s linear 0.35s;
}
@media (hover: none) {
  .etb-tip {
    display: none;
  }
}
</style>
