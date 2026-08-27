<script setup>
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NCard, NInput, NButton } from 'naive-ui'

// A destructive-action guard: the user must type the exact entity name before
// the Delete button enables (GitHub-style). Used for project / workspace deletes.
//
// The `title`/`confirmText` defaults are empty rather than wording: a literal
// default would never let the catalogue answer (`props.title` is always set, so
// a fallback inside the template can't fire) and the dialog would stay Russian
// for every caller that relies on the default.
const props = defineProps({
  show: { type: Boolean, default: false },
  name: { type: String, default: '' },
  title: { type: String, default: '' },
  message: { type: String, default: '' },
  confirmText: { type: String, default: '' },
})
const emit = defineEmits(['update:show', 'confirm'])

const { t } = useI18n()
const cardTitle = computed(() => props.title || t('common.confirm.deleteTitle'))
const confirmLabel = computed(() => props.confirmText || t('common.action.delete'))

const typed = ref('')
const matches = computed(() => !!props.name && typed.value.trim() === props.name.trim())

watch(
  () => props.show,
  (s) => {
    if (s) typed.value = ''
  },
)

function confirm() {
  if (!matches.value) return
  emit('confirm')
  emit('update:show', false)
}
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <n-card :title="cardTitle" style="width: 420px; max-width: 92vw" role="dialog">
      <p v-if="message" class="msg">{{ message }}</p>
      <!-- <i18n-t> rather than concatenation: where the name sits in the sentence
           is the translator's call, not the template's. -->
      <i18n-t keypath="common.confirm.ask" tag="p" class="ask">
        <template #name
          ><b>{{ name }}</b></template
        >
      </i18n-t>
      <n-input
        v-model:value="typed"
        :placeholder="t('common.confirm.namePlaceholder')"
        autofocus
        @keyup.enter="confirm"
      />
      <template #footer>
        <div class="actions">
          <n-button size="small" @click="emit('update:show', false)">
            {{ t('common.action.cancel') }}
          </n-button>
          <n-button type="error" size="small" :disabled="!matches" @click="confirm">{{
            confirmLabel
          }}</n-button>
        </div>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.msg {
  color: var(--t-text2);
  margin: 0 0 10px;
}
.ask {
  color: var(--t-text3);
  font-size: 13px;
  margin: 0 0 8px;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
