<script setup>
import { ref, watch } from 'vue'
import { NModal, NCard, NIcon, NButton, NInput, NDatePicker, NPopconfirm, useMessage } from 'naive-ui'
import { RibbonOutline, AddOutline, TrashOutline, CheckmarkOutline, RefreshOutline } from '@vicons/ionicons5'
import { projects as projApi, milestones as msApi } from '@/api'
import EmptyState from '@/components/EmptyState.vue'

const props = defineProps({
  show: { type: Boolean, default: false },
  projectId: { type: String, default: null },
  projectName: { type: String, default: '' },
})
const emit = defineEmits(['update:show'])

const message = useMessage()
const list = ref([])
const loading = ref(false)
const newTitle = ref('')
const newDue = ref(null) // epoch ms
const saving = ref(false)
const editingId = ref(null)
const editBuf = ref({ title: '', due_date: null, state: 'active' })

async function load() {
  if (!props.projectId) return
  loading.value = true
  try {
    const { data } = await projApi.milestones(props.projectId)
    list.value = data || []
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

async function create() {
  const title = newTitle.value.trim()
  if (!title) return
  saving.value = true
  try {
    const body = { title }
    if (newDue.value) body.due_date = new Date(newDue.value).toISOString()
    const { data } = await projApi.createMilestone(props.projectId, body)
    list.value.push(data)
    newTitle.value = ''
    newDue.value = null
  } catch (e) {
    message.error(e.message)
  } finally {
    saving.value = false
  }
}

function startEdit(m) {
  editingId.value = m.id
  editBuf.value = {
    title: m.title,
    due_date: m.due_date ? new Date(m.due_date).getTime() : null,
    state: m.state,
  }
}
function cancelEdit() {
  editingId.value = null
}
async function saveEdit(m) {
  const b = editBuf.value
  if (!b.title.trim()) return
  try {
    const body = {
      title: b.title.trim(),
      description: m.description || '',
      due_date: b.due_date ? new Date(b.due_date).toISOString() : null,
      start_date: m.start_date || null,
      state: b.state,
    }
    const { data } = await msApi.update(m.id, body)
    const i = list.value.findIndex((x) => x.id === m.id)
    if (i >= 0) list.value[i] = data
    editingId.value = null
  } catch (e) {
    message.error(e.message)
  }
}
async function toggleState(m) {
  try {
    const { data } = await msApi.update(m.id, {
      title: m.title,
      description: m.description || '',
      due_date: m.due_date || null,
      start_date: m.start_date || null,
      state: m.state === 'closed' ? 'active' : 'closed',
    })
    const i = list.value.findIndex((x) => x.id === m.id)
    if (i >= 0) list.value[i] = data
  } catch (e) {
    message.error(e.message)
  }
}
async function remove(m) {
  try {
    await msApi.remove(m.id)
    list.value = list.value.filter((x) => x.id !== m.id)
  } catch (e) {
    message.error(e.message)
  }
}

function fmtDue(iso) {
  if (!iso) return ''
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short', year: 'numeric' })
}

watch(
  () => props.show,
  (v) => {
    if (v) load()
  },
)
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <div class="m-wrap">
      <n-card class="m-card" style="width: 540px; max-width: 94vw" role="dialog">
        <template #header>
          <span class="m-title">
            <n-icon :component="RibbonOutline" class="grad-icon" /> Этапы — {{ projectName }}
          </span>
        </template>

        <div class="m-list">
          <empty-state
            v-if="!loading && !list.length"
            size="small"
            :icon="RibbonOutline"
            text="Этапов пока нет — создайте первый ниже"
          />
          <div v-for="m in list" :key="m.id" class="m-row" :class="{ closed: m.state === 'closed' }">
            <template v-if="editingId === m.id">
              <n-input v-model:value="editBuf.title" size="small" class="m-edit-title" />
              <n-date-picker v-model:value="editBuf.due_date" type="date" size="small" clearable />
              <n-button size="tiny" type="primary" @click="saveEdit(m)">
                <template #icon><n-icon :component="CheckmarkOutline" /></template>
              </n-button>
              <n-button size="tiny" tertiary @click="cancelEdit">Отмена</n-button>
            </template>
            <template v-else>
              <span class="m-name" @click="startEdit(m)">{{ m.title }}</span>
              <span v-if="m.due_date" class="m-due">{{ fmtDue(m.due_date) }}</span>
              <span class="m-spacer" />
              <n-button size="tiny" tertiary :title="m.state === 'closed' ? 'Открыть' : 'Закрыть'" @click="toggleState(m)">
                <template #icon><n-icon :component="m.state === 'closed' ? RefreshOutline : CheckmarkOutline" /></template>
              </n-button>
              <n-popconfirm @positive-click="remove(m)">
                <template #trigger>
                  <n-button size="tiny" tertiary type="error">
                    <template #icon><n-icon :component="TrashOutline" /></template>
                  </n-button>
                </template>
                Удалить этап «{{ m.title }}»? Задачи останутся, но потеряют этап.
              </n-popconfirm>
            </template>
          </div>
        </div>

        <div class="m-new">
          <n-input
            v-model:value="newTitle"
            size="small"
            placeholder="Название этапа"
            @keydown.enter.prevent="create"
          />
          <n-date-picker v-model:value="newDue" type="date" size="small" clearable placeholder="Срок" />
          <n-button size="small" type="primary" :loading="saving" :disabled="!newTitle.trim()" @click="create">
            <template #icon><n-icon :component="AddOutline" /></template>
            Создать
          </n-button>
        </div>
      </n-card>
    </div>
  </n-modal>
</template>

<style scoped>
.m-wrap {
  display: flex;
  justify-content: center;
}
.m-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}
.m-list {
  max-height: 50vh;
  overflow-y: auto;
  margin-bottom: 12px;
}
.m-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 4px;
  border-bottom: 1px solid var(--t-border);
}
.m-row.closed .m-name {
  opacity: 0.6;
  text-decoration: line-through;
}
.m-name {
  font-size: 13px;
  color: var(--t-text1);
  cursor: pointer;
}
.m-name:hover {
  color: var(--t-primary);
}
.m-due {
  font-size: 12px;
  color: var(--t-text3);
}
.m-spacer {
  flex: 1;
}
.m-edit-title {
  flex: 1;
}
.m-new {
  display: flex;
  gap: 8px;
  align-items: center;
  padding-top: 12px;
  border-top: 1px solid var(--t-border);
}
.m-new :deep(.n-input) {
  flex: 1;
}
</style>
