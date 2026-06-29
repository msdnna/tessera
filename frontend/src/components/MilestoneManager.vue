<script setup>
import { ref, watch } from 'vue'
import { NModal, NCard, NIcon, NButton, NInput, NDatePicker, NPopconfirm, NTooltip, useMessage } from 'naive-ui'
import {
  RibbonOutline,
  AddOutline,
  TrashOutline,
  CheckmarkOutline,
  RefreshOutline,
  LogoGitlab,
} from '@vicons/ionicons5'
import { projects as projApi, milestones as msApi, gitlab as glApi } from '@/api'
import { milestoneRange } from '@/utils/milestones'
import EmptyState from '@/components/EmptyState.vue'

const props = defineProps({
  show: { type: Boolean, default: false },
  projectId: { type: String, default: null },
  projectName: { type: String, default: '' },
  wsId: { type: String, default: null },
})
const emit = defineEmits(['update:show', 'changed'])

const message = useMessage()
const list = ref([])
const loading = ref(false)
// Whether this project can push milestones to GitLab (integration on this project's
// board + enabled + milestone write-back on). Creation stays per-milestone opt-in.
const glLinkable = ref(false)
const pushing = ref(null) // milestone id currently being created in GitLab
function isLinked(m) {
  return !!m.gl_global_id
}
const newTitle = ref('')
const newStart = ref(null) // epoch ms
const newDue = ref(null) // epoch ms
const saving = ref(false)
const editingId = ref(null)
const editBuf = ref({ title: '', start_date: null, due_date: null, state: 'active' })

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
  // Can this project's milestones be pushed to GitLab? (integration on this project's
  // board + enabled + push_milestone). Best-effort; no GitLab → just hidden.
  glLinkable.value = false
  if (props.wsId) {
    try {
      const { data } = await glApi.getIntegration(props.wsId)
      glLinkable.value =
        data?.configured === true &&
        data?.enabled === true &&
        data?.writeback?.push_milestone === true &&
        data?.project_id === props.projectId
    } catch {
      glLinkable.value = false
    }
  }
}

async function pushToGitlab(m) {
  pushing.value = m.id
  try {
    await msApi.pushToGitlab(m.id)
    await load()
    emit('changed')
    message.success('Этап создан в GitLab')
  } catch (e) {
    message.error(e.response?.data?.error || e.message)
  } finally {
    pushing.value = null
  }
}

async function create() {
  const title = newTitle.value.trim()
  if (!title) return
  saving.value = true
  try {
    const body = { title }
    if (newStart.value) body.start_date = new Date(newStart.value).toISOString()
    if (newDue.value) body.due_date = new Date(newDue.value).toISOString()
    const { data } = await projApi.createMilestone(props.projectId, body)
    list.value.push(data)
    newTitle.value = ''
    newStart.value = null
    newDue.value = null
    emit('changed')
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
    start_date: m.start_date ? new Date(m.start_date).getTime() : null,
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
      start_date: b.start_date ? new Date(b.start_date).toISOString() : null,
      due_date: b.due_date ? new Date(b.due_date).toISOString() : null,
      state: b.state,
    }
    const { data } = await msApi.update(m.id, body)
    const i = list.value.findIndex((x) => x.id === m.id)
    if (i >= 0) list.value[i] = data
    editingId.value = null
    emit('changed')
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
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function remove(m) {
  try {
    await msApi.remove(m.id)
    list.value = list.value.filter((x) => x.id !== m.id)
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}

const fmtRange = milestoneRange

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
      <n-card class="m-card" style="width: 620px; max-width: 96vw" role="dialog">
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
              <n-date-picker
                v-model:value="editBuf.start_date"
                type="date"
                size="small"
                clearable
                placeholder="Начало"
              />
              <n-date-picker
                v-model:value="editBuf.due_date"
                type="date"
                size="small"
                clearable
                placeholder="Конец"
              />
              <n-button size="tiny" type="primary" @click="saveEdit(m)">
                <template #icon><n-icon :component="CheckmarkOutline" /></template>
              </n-button>
              <n-button size="tiny" tertiary @click="cancelEdit">Отмена</n-button>
            </template>
            <template v-else>
              <n-tooltip v-if="isLinked(m)" :disabled="false">
                <template #trigger>
                  <span class="m-name linked">{{ m.title }}</span>
                </template>
                Синхронизируется с GitLab — правьте в GitLab
              </n-tooltip>
              <span v-else class="m-name" @click="startEdit(m)">{{ m.title }}</span>
              <span v-if="fmtRange(m)" class="m-due">{{ fmtRange(m) }}</span>
              <span class="m-spacer" />
              <!-- explicit, per-milestone push to GitLab (native + linkable only) -->
              <n-button
                v-if="!isLinked(m) && glLinkable"
                size="tiny"
                tertiary
                :loading="pushing === m.id"
                title="Создать этот этап в GitLab"
                @click="pushToGitlab(m)"
              >
                <template #icon><n-icon :component="LogoGitlab" /></template>
                В GitLab
              </n-button>
              <!-- open the linked GitLab milestone (a button by the delete one) -->
              <n-button
                v-if="isLinked(m)"
                size="tiny"
                tertiary
                tag="a"
                :href="m.gl_url"
                target="_blank"
                rel="noopener"
                title="Открыть в GitLab"
              >
                <template #icon><n-icon :component="LogoGitlab" /></template>
              </n-button>
              <!-- state toggle: native only (GitLab-linked state is synced from GitLab) -->
              <n-button
                v-if="!isLinked(m)"
                size="tiny"
                tertiary
                :title="m.state === 'closed' ? 'Открыть' : 'Закрыть'"
                @click="toggleState(m)"
              >
                <template #icon><n-icon :component="m.state === 'closed' ? RefreshOutline : CheckmarkOutline" /></template>
              </n-button>
              <n-popconfirm @positive-click="remove(m)">
                <template #trigger>
                  <n-button size="tiny" tertiary type="error">
                    <template #icon><n-icon :component="TrashOutline" /></template>
                  </n-button>
                </template>
                {{
                  isLinked(m)
                    ? 'Удалить этап в Tessera? В GitLab он останется (связь будет снята).'
                    : 'Удалить этап «' + m.title + '»? Задачи останутся, но потеряют этап.'
                }}
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
          <n-date-picker v-model:value="newStart" type="date" size="small" clearable placeholder="Начало" />
          <n-date-picker v-model:value="newDue" type="date" size="small" clearable placeholder="Конец" />
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
.m-name.linked {
  cursor: default;
}
.m-name.linked:hover {
  color: var(--t-text1);
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
  min-width: 120px;
}
.m-row :deep(.n-date-picker),
.m-new :deep(.n-date-picker) {
  width: 132px;
  flex: none;
}
.m-new {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
  padding-top: 12px;
  border-top: 1px solid var(--t-border);
}
.m-new .n-button {
  margin-left: 4px;
}
.m-new :deep(.n-input) {
  flex: 1;
  min-width: 140px;
}
</style>
