<script setup>
// The v2 field for typing a new item in place (#3112): a card at the bottom of
// a column, a subtask under its card. A borderless textarea on the input
// fill, radius 10, the primary ring on focus, growing to four lines, with
// «↵ создать · Esc» under it. ↵ submits (⇧↵ is a line break the title will
// fold back), Esc cancels, a click away is the owner's call (`blur`).
import { nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  // 'card' — the column's field; 'small' — the tighter one under a card.
  size: { type: String, default: 'card' },
  maxRows: { type: Number, default: 4 },
  autofocus: { type: Boolean, default: true },
})
const emit = defineEmits(['update:modelValue', 'submit', 'cancel', 'blur'])
const { t } = useI18n()

const area = ref(null)

// Height follows the text up to maxRows; the line height is the CSS one.
function fit() {
  const el = area.value
  if (!el) return
  el.style.height = 'auto'
  const cs = getComputedStyle(el)
  const line = parseFloat(cs.lineHeight) || 19
  const pad = (parseFloat(cs.paddingTop) || 0) + (parseFloat(cs.paddingBottom) || 0)
  el.style.height = `${Math.min(el.scrollHeight, line * props.maxRows + pad)}px`
}
watch(
  () => props.modelValue,
  () => nextTick(fit),
)

function onKeydown(e) {
  if (e.isComposing) return
  if (e.key === 'Enter' && !e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey) {
    e.preventDefault()
    emit('submit')
  } else if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    emit('cancel')
  }
}

function focus() {
  area.value?.focus({ preventScroll: true })
}
defineExpose({ focus })
onMounted(() => {
  fit()
  if (props.autofocus) focus()
})
</script>

<template>
  <div class="qaf" :class="size">
    <textarea
      ref="area"
      class="qaf-input"
      rows="1"
      :value="modelValue"
      :placeholder="placeholder"
      @input="emit('update:modelValue', $event.target.value)"
      @keydown="onKeydown"
      @blur="emit('blur', $event)"
      @click.stop
    />
    <div class="qaf-hint" aria-hidden="true">
      <Kbd>{{ CAPS.enter }}</Kbd>
      <span>{{ t('common.inline.create') }}</span>
      <span class="qaf-dot">·</span>
      <Kbd>{{ CAPS.escape }}</Kbd>
    </div>
  </div>
</template>

<style scoped>
.qaf {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.qaf-input {
  display: block;
  width: 100%;
  box-sizing: border-box;
  min-height: 36px;
  padding: 8px 11px;
  border: 0;
  border-radius: 10px;
  outline: 0;
  resize: none;
  overflow-y: auto;
  background: var(--t-input-bg);
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  line-height: 19px;
  transition: box-shadow 0.12s ease;
}
.qaf-input::placeholder {
  color: var(--t-placeholder);
}
.qaf-input:focus {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.qaf.small .qaf-input {
  min-height: 30px;
  padding: 5px 9px;
  border-radius: 8px;
  font-size: 13px;
  line-height: 18px;
}
.qaf-hint {
  display: flex;
  align-items: center;
  gap: 5px;
  padding-left: 4px;
  font-size: 11.5px;
  color: var(--t-text3);
}
.qaf-dot {
  margin: 0 2px;
}
/* A phone has no Esc key and no hover hints (#2976). */
@media (hover: none) {
  .qaf-hint {
    display: none;
  }
}
</style>
