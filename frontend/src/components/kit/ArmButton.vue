<script setup>
// The two-step danger item of package A (#2969), as a row's icon button (#3113):
// the first press arms it — the button turns red and says «Удалить?» — and the
// second press does it. It disarms after 4 s, when the pointer or the focus
// leaves, or on Esc (which it keeps from the dialog underneath). For the rows of
// the task modal (comments, files, documents, links), where a menu just to hold
// one destructive item would be a detour.
import { onBeforeUnmount, ref } from 'vue'
import { NIcon } from 'naive-ui'
import { TrashOutline } from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'

defineProps({
  // Tooltip and accessible name of the idle button.
  title: { type: String, default: '' },
  // What the armed button says («Удалить?»).
  confirm: { type: String, required: true },
  icon: { type: [Object, Function], default: () => TrashOutline },
  testid: { type: String, default: 'arm-button' },
})
const emit = defineEmits(['confirm'])

const armed = ref(false)
let timer = 0
function disarm() {
  armed.value = false
  clearTimeout(timer)
}
function press() {
  if (!armed.value) {
    armed.value = true
    clearTimeout(timer)
    timer = setTimeout(disarm, 4000)
    return
  }
  disarm()
  emit('confirm')
}
function onKey(e) {
  if (e.key === 'Escape' && armed.value) {
    e.preventDefault()
    e.stopPropagation()
    disarm()
  }
}
onBeforeUnmount(() => clearTimeout(timer))
defineExpose({ disarm })
</script>

<template>
  <button
    type="button"
    class="arm"
    :class="{ armed }"
    :title="armed ? '' : title"
    :aria-label="armed ? confirm : title"
    :data-testid="armed ? `${testid}-armed` : testid"
    @click.stop="press"
    @keydown="onKey"
    @blur="disarm"
    @mouseleave="disarm"
  >
    <n-icon :component="icon" :size="15" />
    <template v-if="armed">
      <span class="arm-text">{{ confirm }}</span>
      <Kbd class="arm-cap">{{ CAPS.enter }}</Kbd>
    </template>
  </button>
</template>

<style scoped>
.arm {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 26px;
  height: 26px;
  padding: 0;
  box-sizing: border-box;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  white-space: nowrap;
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.arm:hover {
  background: var(--t-hover);
  color: #e0533d;
}
.arm:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.arm.armed {
  padding: 0 4px 0 8px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 86%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 86%, #fff)
  );
  color: #fff;
  box-shadow: 0 4px 14px rgba(224, 83, 61, 0.32);
}
.arm-cap {
  height: 18px;
  min-width: 18px;
  background: rgba(255, 255, 255, 0.24);
  color: #fff;
  box-shadow: none;
}
</style>
