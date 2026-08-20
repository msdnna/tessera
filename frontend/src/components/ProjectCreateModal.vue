<script setup>
import { ref, computed, watch } from 'vue'
import { NModal, NCard, NInput, NButton, NCheckbox, NText, useMessage } from 'naive-ui'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { makeSlug } from '@/utils/slug'

// Naming a project at creation time. Before this, every project was born
// "Проект" and got the address `proekt-<n>` — the name was fixable afterwards,
// the address was not (it's assigned once, on create).
const props = defineProps({
  show: { type: Boolean, default: false },
  // Group to create the project in; null/undefined creates it at tree root.
  groupId: { type: String, default: null },
})
const emit = defineEmits(['update:show', 'created'])

const store = useWorkspacesStore()
const message = useMessage()

const name = ref('')
const customSlug = ref(false)
const slugInput = ref('')
const slugError = ref('')
const busy = ref(false)

// Picking the address is manager-only server-side; hide the control for others
// rather than let them fill it in and collect a 403.
const canPickSlug = computed(() => store.canManage)

// What the address will be: what was typed when overriding, else derived from
// the name exactly as the server would derive it.
const preview = computed(() => makeSlug(customSlug.value ? slugInput.value : name.value))
const canSubmit = computed(
  () => !!name.value.trim() && !(customSlug.value && !preview.value) && !busy.value,
)

watch(
  () => props.show,
  (open) => {
    if (!open) return
    name.value = ''
    customSlug.value = false
    slugInput.value = ''
    slugError.value = ''
  },
)

// Typing a name updates the address suggestion until the user takes it over.
watch(customSlug, (on) => {
  if (on && !slugInput.value) slugInput.value = makeSlug(name.value)
  slugError.value = ''
})

async function submit() {
  if (!canSubmit.value) return
  busy.value = true
  slugError.value = ''
  try {
    const body = { name: name.value.trim() }
    if (props.groupId) body.group_id = props.groupId
    if (customSlug.value && preview.value) body.slug = preview.value
    const res = await wsApi.createProject(store.currentId, body)
    emit('update:show', false)
    emit('created', res.data)
  } catch (e) {
    // A taken address is the one failure the user can fix right here, so it
    // stays under the field instead of vanishing into a toast.
    if (e.status === 409) slugError.value = 'Такой адрес уже занят — выберите другой'
    else message.error(e.message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <n-card title="Новый проект" style="max-width: 420px" role="dialog" :bordered="false">
      <div class="pc-body">
        <n-input
          v-model:value="name"
          placeholder="Название проекта"
          autofocus
          :disabled="busy"
          data-tour="project-name"
          @keyup.enter="submit"
        />

        <n-checkbox
          v-if="canPickSlug"
          v-model:checked="customSlug"
          :disabled="busy"
          data-tour="project-slug"
        >
          Задать адрес вручную
        </n-checkbox>
        <n-input
          v-if="canPickSlug && customSlug"
          v-model:value="slugInput"
          placeholder="адрес-проекта"
          :status="slugError ? 'error' : undefined"
          :disabled="busy"
          @keyup.enter="submit"
        />
        <n-text v-if="slugError" type="error" class="pc-hint">{{ slugError }}</n-text>

        <n-text depth="3" class="pc-hint">
          Адрес:
          <span v-if="preview" class="pc-url">/project/{{ preview }}</span>
          <span v-else>будет назначен автоматически</span>
        </n-text>
      </div>
      <template #footer>
        <div class="pc-actions">
          <n-button size="small" :disabled="busy" @click="emit('update:show', false)">
            Отмена
          </n-button>
          <n-button
            type="primary"
            size="small"
            :disabled="!canSubmit"
            :loading="busy"
            data-tour="project-submit"
            @click="submit"
          >
            Создать
          </n-button>
        </div>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.pc-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.pc-hint {
  font-size: 12px;
}
.pc-url {
  font-family: var(--t-font-mono, ui-monospace, monospace);
}
.pc-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
