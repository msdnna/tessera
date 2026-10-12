<script setup>
// «Шаблоны» of the commands window (#3098): the workspace's ready-made tasks
// («Баг-репорт», «Чек-лист релиза») — a title, a Markdown description (a
// checklist is just «- [ ]»), tags by name (matched to the board's project when
// a task is made), a priority, subtasks one per line. A command «Создать
// задачу» picks one; any member adds templates, the author or an admin edits.
import { computed, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NSelect } from 'naive-ui'
import { useMacrosStore } from '@/stores/macros'
import { useAuthStore } from '@/stores/auth'
import { useMessage } from '@/composables/useMessage'
import { humanizeError } from '@/utils/errors'
import ArmButton from '../kit/ArmButton.vue'
import '../kit/kit.css'

const { t } = useI18n()
const store = useMacrosStore()
const auth = useAuthStore()
const message = useMessage()

const PRI = ['none', 'low', 'normal', 'high', 'urgent']
const TAGS_EXAMPLE = 'type::bug' // an example value, not interface text
const priorityOptions = computed(() =>
  PRI.map((k, i) => ({ value: i, label: t(`common.priority.${k}`) })),
)
const canEdit = (tpl) => store.canManage || tpl.created_by === auth.user?.id

const editing = ref(null) // id | 'new' | null
const form = ref(null)
const error = ref('')
const busy = ref(false)
const nameEl = ref(null)

async function open(tpl) {
  error.value = ''
  editing.value = tpl ? tpl.id : 'new'
  form.value = tpl
    ? {
        name: tpl.name,
        title: tpl.title,
        description: tpl.description,
        tags: (tpl.tags || []).join(', '),
        priority: tpl.priority || 0,
        subtasks: (tpl.subtasks || []).join('\n'),
      }
    : { name: '', title: '', description: '', tags: '', priority: 0, subtasks: '' }
  await nextTick()
  nameEl.value?.focus()
}
function close() {
  editing.value = null
  form.value = null
}
async function save() {
  const f = form.value
  if (!f || busy.value) return
  busy.value = true
  error.value = ''
  try {
    const tpl = editing.value === 'new' ? null : store.templates.find((x) => x.id === editing.value)
    await store.saveTemplate(tpl, {
      name: f.name,
      title: f.title,
      description: f.description,
      tags: f.tags.split(',').map((s) => s.trim()),
      priority: f.priority,
      subtasks: f.subtasks.split('\n').map((s) => s.trim()),
    })
    close()
  } catch (e) {
    error.value = humanizeError(e)
  } finally {
    busy.value = false
  }
}
async function remove(tpl) {
  try {
    await store.removeTemplate(tpl)
    if (editing.value === tpl.id) close()
  } catch (e) {
    message.error(humanizeError(e))
  }
}
function onFormKey(e) {
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    close()
  } else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
    e.preventDefault()
    save()
  }
}
const rows = computed(() => [
  ...store.templates,
  ...(editing.value === 'new' ? [{ id: 'new' }] : []),
])
defineExpose({ open, editing, close })
</script>

<template>
  <div class="tp" data-testid="templates-pane">
    <div class="tp-title">
      <h2>{{ t('shell.commands.navTemplates') }}</h2>
      <span class="tp-note">{{ t('shell.commands.tpl.summary', store.templates.length) }}</span>
      <span class="tp-grow" />
      <button
        type="button"
        class="kit-btn primary"
        data-testid="template-new"
        :disabled="editing !== null"
        @click="open(null)"
      >
        {{ t('shell.commands.tpl.new') }}
      </button>
    </div>

    <template v-for="tpl in rows" :key="tpl.id">
      <div
        v-if="tpl.id !== 'new'"
        class="tp-row"
        data-testid="template-row"
        @click="canEdit(tpl) && editing === null && open(tpl)"
      >
        <span class="tp-name">{{ tpl.name }}</span>
        <span class="tp-what">
          {{ tpl.title || '—' }}
          <template v-if="tpl.tags?.length"> · {{ tpl.tags.join(', ') }}</template>
          <template v-if="tpl.subtasks?.length">
            · {{ t('shell.commands.tpl.subtasksN', tpl.subtasks.length) }}</template
          >
        </span>
        <span class="tp-acts" @click.stop>
          <ArmButton
            v-if="canEdit(tpl)"
            :title="t('shell.commands.tpl.remove')"
            :confirm="t('shell.commands.macro.removeConfirm')"
            testid="template-remove"
            @confirm="remove(tpl)"
          />
        </span>
      </div>
      <div
        v-if="editing === tpl.id && form"
        class="tp-form"
        data-testid="template-form"
        @keydown="onFormKey"
      >
        <div class="tp-grid">
          <label class="tp-f">
            <span>{{ t('shell.commands.tpl.name') }}</span>
            <input ref="nameEl" v-model="form.name" class="kf-input" data-testid="template-name" />
          </label>
          <label class="tp-f">
            <span>{{ t('shell.commands.tpl.taskTitle') }}</span>
            <input v-model="form.title" class="kf-input" data-testid="template-title" />
          </label>
          <label class="tp-f">
            <span>{{ t('shell.commands.macro.tags') }}</span>
            <input v-model="form.tags" class="kf-input" :placeholder="TAGS_EXAMPLE" />
          </label>
          <label class="tp-f">
            <span>{{ t('shell.commands.macro.priority') }}</span>
            <n-select v-model:value="form.priority" :options="priorityOptions" />
          </label>
        </div>
        <label class="tp-f">
          <span>{{ t('shell.commands.tpl.description') }}</span>
          <textarea
            v-model="form.description"
            class="kf-input kf-area"
            rows="5"
            :placeholder="t('shell.commands.tpl.descriptionPlaceholder')"
            data-testid="template-description"
          />
        </label>
        <label class="tp-f">
          <span>{{ t('shell.commands.tpl.subtasks') }}</span>
          <textarea
            v-model="form.subtasks"
            class="kf-input kf-area"
            rows="3"
            data-testid="template-subtasks"
          />
        </label>
        <div class="tp-foot">
          <span class="tp-note">{{ t('shell.commands.tpl.saveHint') }}</span>
          <span class="tp-grow" />
          <button type="button" class="kit-btn" @click="close">
            {{ t('common.action.cancel') }}
          </button>
          <button
            type="button"
            class="kit-btn primary"
            :disabled="busy || !form.name.trim()"
            data-testid="template-save"
            @click="save"
          >
            {{ t('common.action.save') }}
          </button>
        </div>
        <p v-if="error" class="tp-err">{{ error }}</p>
      </div>
    </template>
    <p v-if="!rows.length" class="tp-empty">{{ t('shell.commands.tpl.empty') }}</p>
  </div>
</template>

<style scoped>
.tp {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.tp-title {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
  min-height: 36px;
}
.tp-title h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
}
.tp-note {
  font-size: 13px;
  color: var(--t-text3);
}
.tp-grow {
  flex: 1;
}
.tp-row {
  display: grid;
  grid-template-columns: minmax(140px, 1fr) minmax(160px, 2.4fr) 40px;
  align-items: center;
  gap: 10px;
  min-height: 44px;
  padding: 0 10px;
  border-radius: 12px;
  background: var(--t-col);
  cursor: pointer;
}
.tp-row:hover {
  background: color-mix(in srgb, var(--t-col) 70%, var(--t-card));
}
.tp-name {
  font-weight: 600;
}
.tp-what {
  color: var(--t-text2);
  font-size: 12.5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tp-acts {
  display: flex;
  justify-content: flex-end;
}
.tp-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 2px 0 8px;
  padding: 12px;
  border-radius: 12px;
  background: var(--t-card);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--t-primary) 35%, transparent);
}
.tp-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 10px;
}
.tp-f {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--t-text3);
}
.tp-foot {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tp-err {
  margin: 0;
  font-size: 12.5px;
  color: var(--t-danger, #d03050);
}
.tp-empty {
  padding: 18px 10px;
  color: var(--t-text3);
  font-size: 13px;
}
</style>
