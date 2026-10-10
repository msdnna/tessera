<script setup>
// The confirmation code's cells (v2, #3057 — AuthSheet «Подтверждение email»):
// six 48px cells, the active one ringed in the accent with a caret.
//
// One real <input> lies over the cells, transparent, and the cells only draw
// its value. That is what makes the mockup's «код вставляется целиком и
// переходит к следующей ячейке» free: a paste, the OS's one-time-code autofill
// (`autocomplete="one-time-code"`) and typing all land in a single field, with
// no focus juggling between six inputs. Anything but digits is dropped as it
// arrives, so «482 913» from a mail client pastes as 482913.
import { computed, ref, watch, onMounted, nextTick } from 'vue'

const props = defineProps({
  modelValue: { type: String, default: '' },
  length: { type: Number, default: 6 },
  disabled: { type: Boolean, default: false },
  error: { type: Boolean, default: false },
  label: { type: String, default: '' },
  autofocus: { type: Boolean, default: true },
})
const emit = defineEmits(['update:modelValue', 'complete'])

const input = ref(null)
const focused = ref(false)
const digits = computed(() => (props.modelValue || '').replace(/\D/g, '').slice(0, props.length))
// The caret's cell: the next empty one, or the last when all are filled.
const active = computed(() => Math.min(digits.value.length, props.length - 1))

function onInput(e) {
  const clean = (e.target.value || '').replace(/\D/g, '').slice(0, props.length)
  if (e.target.value !== clean) e.target.value = clean
  emit('update:modelValue', clean)
  if (clean.length === props.length) emit('complete', clean)
}

// Keep the field's own value in step when the parent clears it (a wrong code).
watch(digits, (v) => {
  if (input.value && input.value.value !== v) input.value.value = v
})

function focus() {
  input.value?.focus()
}
defineExpose({ focus })

onMounted(async () => {
  if (!props.autofocus) return
  if (window.matchMedia?.('(pointer: coarse)')?.matches) return
  await nextTick()
  focus()
})
</script>

<template>
  <div class="cc" :class="{ 'is-error': error, 'is-disabled': disabled }" @click="focus">
    <span
      v-for="i in length"
      :key="i"
      class="cc-cell"
      :class="{ on: focused && active === i - 1, filled: i <= digits.length }"
      aria-hidden="true"
      data-testid="code-cell"
    >
      <template v-if="digits[i - 1]">{{ digits[i - 1] }}</template>
      <span v-else-if="focused && active === i - 1" class="cc-caret" />
    </span>
    <input
      ref="input"
      class="cc-input"
      type="text"
      inputmode="numeric"
      autocomplete="one-time-code"
      :maxlength="length"
      :value="digits"
      :disabled="disabled"
      :aria-label="label"
      :aria-invalid="error ? 'true' : undefined"
      spellcheck="false"
      data-testid="code-input"
      @input="onInput"
      @focus="focused = true"
      @blur="focused = false"
    />
  </div>
</template>

<style scoped>
.cc {
  position: relative;
  display: flex;
  gap: 8px;
  cursor: text;
}
.cc-cell {
  flex: 1;
  min-width: 0;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  background: var(--t-input-bg);
  color: var(--t-text1);
  font-size: 20px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  transition: box-shadow 0.12s ease;
}
.cc-cell.on {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.cc.is-error .cc-cell.filled {
  box-shadow: 0 0 0 1.5px color-mix(in srgb, #e0533d 70%, transparent);
}
.cc.is-disabled {
  opacity: 0.6;
  cursor: default;
}
.cc-caret {
  width: 2px;
  height: 22px;
  background: var(--t-text1);
  animation: cc-blink 1s steps(1) infinite;
}
@keyframes cc-blink {
  50% {
    opacity: 0;
  }
}
@media (prefers-reduced-motion: reduce) {
  .cc-caret {
    animation: none;
  }
}
/* The real field: over the cells, invisible, still the one that takes input,
   paste and autofill. Its own caret and selection are hidden — the cells draw
   them. */
.cc-input {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  opacity: 0;
  border: 0;
  padding: 0;
  font-size: 16px; /* no zoom-on-focus on iOS */
  color: transparent;
  caret-color: transparent;
  background: transparent;
}
</style>
