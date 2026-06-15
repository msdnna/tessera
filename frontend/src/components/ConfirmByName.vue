<script setup>
import { ref, computed, watch } from 'vue'
import { NModal, NCard, NInput, NButton } from 'naive-ui'

// A destructive-action guard: the user must type the exact entity name before
// the Delete button enables (GitHub-style). Used for project / workspace deletes.
const props = defineProps({
  show: { type: Boolean, default: false },
  name: { type: String, default: '' },
  title: { type: String, default: 'Подтвердите удаление' },
  message: { type: String, default: '' },
  confirmText: { type: String, default: 'Удалить' },
})
const emit = defineEmits(['update:show', 'confirm'])

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
    <n-card :title="title" style="width: 420px; max-width: 92vw" role="dialog">
      <p v-if="message" class="msg">{{ message }}</p>
      <p class="ask">
        Введите <b>{{ name }}</b> для подтверждения:
      </p>
      <n-input v-model:value="typed" placeholder="Название" autofocus @keyup.enter="confirm" />
      <template #footer>
        <div class="actions">
          <n-button size="small" @click="emit('update:show', false)">Отмена</n-button>
          <n-button type="error" size="small" :disabled="!matches" @click="confirm">{{
            confirmText
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
