<script setup>
// SecretInput — a password n-input that can *erase* an already-stored secret.
//
// The backend treats an empty value as "keep the stored secret", so wiping one
// needs an explicit signal. This component surfaces an eraser button (next to the
// eye) that, when a secret is saved, arms an erase: it clears the field and emits
// `cleared`. The parent sends the matching clear_* flag on save. Typing a new
// value cancels the armed erase (replace beats erase — same rule as the backend).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
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

const { t } = useI18n()

const showEraser = computed(() => props.stored && props.resettable)

const eraserLabel = computed(() =>
  props.cleared ? t('common.secret.undoClear') : t('common.secret.clear'),
)

const effectivePlaceholder = computed(() => {
  if (props.cleared) return t('common.secret.clearedPlaceholder')
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
      <!-- @mousedown.prevent: keep focus in the field so the eye doesn't flicker.
           tabindex + Enter/Space: this eraser is the *only* way to wipe a stored
           secret (an empty value means "keep"), so without them the whole feature
           is mouse-only — and arming disables the input, putting undo out of
           keyboard reach too. Same shape as VersionBadge.vue. -->
      <span
        class="secret-eraser"
        role="button"
        tabindex="0"
        :title="eraserLabel"
        :aria-label="eraserLabel"
        @mousedown.prevent
        @click="cleared ? undo() : arm()"
        @keydown.enter.prevent="cleared ? undo() : arm()"
        @keydown.space.prevent="cleared ? undo() : arm()"
      >
        <n-icon :size="16" :component="cleared ? ArrowUndoOutline : BackspaceOutline" />
      </span>
    </template>
  </n-input>
</template>

<style scoped>
.secret-eraser {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 16px; /* match the built-in eye so both icons line up */
  margin-right: 6px; /* gap between the eraser and the eye */
  cursor: pointer;
  color: var(--t-text-3, #999);
  transition: color 0.15s ease;
}
.secret-eraser:hover {
  color: var(--t-accent, #7c5cff);
}
/* Now that the eraser is tabbable it needs a visible stop for the keyboard. */
.secret-eraser:focus-visible {
  color: var(--t-accent, #7c5cff);
  outline: 2px solid var(--t-accent, #7c5cff);
  outline-offset: 2px;
  border-radius: 3px;
}
</style>
