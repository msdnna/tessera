<script setup>
// SecretInput — a password n-input that can *erase* an already-stored secret.
//
// The backend treats an empty value as "keep the stored secret", so wiping one
// needs an explicit signal. This component surfaces an eraser button (next to the
// eye) that, when a secret is saved, arms an erase: it clears the field and emits
// `cleared`. The parent sends the matching clear_* flag on save. Typing a new
// value cancels the armed erase (replace beats erase — same rule as the backend).
import { computed } from 'vue'
import { NInput, NIcon } from 'naive-ui'
import { BackspaceOutline, ArrowUndoOutline } from '@vicons/ionicons5'

const props = defineProps({
  value: { type: String, default: '' }, // v-model:value — typed replacement
  cleared: { type: Boolean, default: false }, // v-model:cleared — erase armed
  stored: { type: Boolean, default: false }, // a secret is already saved on the server
  resettable: { type: Boolean, default: true }, // allow erasing at all
  placeholder: { type: String, default: '' }, // shown when nothing is stored
  storedPlaceholder: { type: String, default: '' }, // shown when a secret is stored
  size: { type: String, default: undefined },
  inputProps: { type: Object, default: undefined },
})
const emit = defineEmits(['update:value', 'update:cleared'])

const showEraser = computed(() => props.stored && props.resettable)

const effectivePlaceholder = computed(() => {
  if (props.cleared) return 'будет очищено при сохранении'
  if (props.stored) return props.storedPlaceholder || props.placeholder
  return props.placeholder
})

function onInput(v) {
  // Typing after arming an erase means "replace", not "keep erased" — cancel it.
  if (props.cleared) emit('update:cleared', false)
  emit('update:value', v)
}
function arm() {
  emit('update:value', '')
  emit('update:cleared', true)
}
function undo() {
  emit('update:cleared', false)
}
</script>

<template>
  <n-input
    :value="cleared ? '' : value"
    type="password"
    show-password-on="click"
    :size="size"
    :disabled="cleared"
    :placeholder="effectivePlaceholder"
    :input-props="inputProps"
    @update:value="onInput"
  >
    <template v-if="showEraser" #suffix>
      <!-- @mousedown.prevent: keep focus in the field so the eye doesn't flicker -->
      <span
        class="secret-eraser"
        role="button"
        :title="cleared ? 'Отменить очистку' : 'Очистить сохранённое значение'"
        @mousedown.prevent
        @click="cleared ? undo() : arm()"
      >
        <n-icon :component="cleared ? ArrowUndoOutline : BackspaceOutline" />
      </span>
    </template>
  </n-input>
</template>

<style scoped>
.secret-eraser {
  display: inline-flex;
  align-items: center;
  cursor: pointer;
  color: var(--t-text-3, #999);
  transition: color 0.15s ease;
}
.secret-eraser:hover {
  color: var(--t-accent, #7c5cff);
}
</style>
