<script setup>
// «Команды» of the commands window (#3069, mockup CmdsWin): the workspace's
// palette commands. A row: name, call (>bug), what it does, keys. A click opens
// the form under it — name, call, action and its parameters, keys (pressed, not
// typed; a chord the app already uses is refused with its name), shared or
// mine, category, on/off. Shared commands are edited by the owner and admins.
import { computed, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NSelect, NSwitch } from 'naive-ui'
import { documents as docsApi } from '@/api'
import { useMacrosStore } from '@/stores/macros'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useMessage } from '@/composables/useMessage'
import { humanizeError } from '@/utils/errors'
import { keyConflicts } from '@/shortcuts/dynamic'
import { capsOf, parseChord } from '@/shortcuts/keymap'
import { byId } from '@/shortcuts/catalog'
import ArmButton from '../kit/ArmButton.vue'
import Kbd from '../Kbd.vue'
import '../kit/kit.css'

const { t } = useI18n()
const store = useMacrosStore()
const ws = useWorkspacesStore()
const message = useMessage()

// Examples in the fields: values, not interface text.
const EX = {
  trigger: 'bug',
  tags: 'type::bug, web',
  url: 'https://',
  urlLabel: 'URL',
  ref: 'main',
  vars: 'ENV=staging',
}
const VIS = ['all', 'shared', 'mine']
const vis = ref('all')
const list = computed(() =>
  store.macros.filter(
    (m) => vis.value === 'all' || (vis.value === 'shared' ? m.shared : !m.shared),
  ),
)
const counts = computed(() => ({
  all: store.macros.length,
  shared: store.macros.filter((m) => m.shared).length,
  mine: store.macros.filter((m) => !m.shared).length,
}))

const ACTIONS = ['create_task', 'open_link', 'create_document', 'run_pipeline']
const actionOptions = computed(() =>
  ACTIONS.map((a) => ({ value: a, label: t(`shell.commands.macro.action.${a}`) })),
)
const PRIORITIES = [0, 1, 2, 3, 4]
const priorityOptions = computed(() => [
  { value: -1, label: t('shell.commands.macro.fromTemplate') },
  ...PRIORITIES.map((p) => ({
    value: p,
    label: t(`common.priority.${['none', 'low', 'normal', 'high', 'urgent'][p]}`),
  })),
])
const templateOptions = computed(() => [
  { value: '', label: t('shell.commands.macro.noTemplate') },
  ...store.templates.map((x) => ({ value: x.id, label: x.name })),
])
const docTemplates = ref([])
const docTemplateOptions = computed(() => [
  { value: '', label: t('shell.commands.macro.noTemplate') },
  ...docTemplates.value.map((x) => ({ value: x.id, label: x.name || x.title })),
])

// ── the form ──
const editing = ref(null) // macro id | 'new' | null
const form = ref(null)
const error = ref('')
const busy = ref(false)
const nameEl = ref(null)

function blank() {
  return {
    name: '',
    trigger: '',
    action: 'create_task',
    keys: '',
    category: '',
    enabled: true,
    shared: store.canManage,
    // params, flattened for the fields
    template_id: '',
    tags: '',
    priority: -1,
    assignMe: false,
    currentStage: false,
    url: '',
    title: '',
    doc_template_id: '',
    mode: 'run',
    ref: '',
    variables: '',
  }
}
function fromMacro(m) {
  const p = m.params || {}
  return {
    ...blank(),
    name: m.name,
    trigger: m.trigger,
    action: m.action,
    keys: m.keys || '',
    category: m.category || '',
    enabled: m.enabled,
    shared: m.shared,
    template_id: m.action === 'create_task' ? p.template_id || '' : '',
    tags: (p.tags || []).join(', '),
    priority: Number.isInteger(p.priority) ? p.priority : -1,
    assignMe: p.assignee === 'me',
    currentStage: p.milestone === 'current',
    url: p.url || '',
    title: p.title || '',
    doc_template_id: m.action === 'create_document' ? p.template_id || '' : '',
    mode: p.mode === 'retry_failed' ? 'retry_failed' : 'run',
    ref: p.ref || '',
    variables: Object.entries(p.variables || {})
      .map(([k, v]) => `${k}=${v}`)
      .join('\n'),
  }
}
function paramsOf(f) {
  switch (f.action) {
    case 'create_task': {
      const tags = f.tags
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      return {
        template_id: f.template_id || undefined,
        title: f.title.trim() || undefined,
        tags: tags.length ? tags : undefined,
        priority: f.priority >= 0 ? f.priority : undefined,
        assignee: f.assignMe ? 'me' : undefined,
        milestone: f.currentStage ? 'current' : undefined,
      }
    }
    case 'open_link':
      return { url: f.url.trim() }
    case 'create_document':
      return { template_id: f.doc_template_id || undefined, title: f.title.trim() || undefined }
    case 'run_pipeline': {
      const variables = {}
      for (const line of f.variables.split('\n')) {
        const at = line.indexOf('=')
        if (at > 0) variables[line.slice(0, at).trim()] = line.slice(at + 1).trim()
      }
      return { mode: f.mode, ref: f.ref.trim() || undefined, variables }
    }
  }
  return {}
}

async function open(m) {
  error.value = ''
  editing.value = m ? m.id : 'new'
  form.value = m ? fromMacro(m) : blank()
  if (!docTemplates.value.length && ws.currentId) {
    docsApi
      .templates(ws.currentId)
      .then((r) => (docTemplates.value = r.data || []))
      .catch(() => {})
  }
  await nextTick()
  nameEl.value?.focus()
}
function close() {
  editing.value = null
  form.value = null
  error.value = ''
}

// Keys: press the chord in the field; ⌫ clears it.
const keyConflict = computed(() => {
  const k = form.value?.keys
  if (!k) return ''
  const ids = keyConflicts(k, { except: editing.value === 'new' ? '' : editing.value })
  if (!ids.length) return ''
  const id = ids[0]
  const e = byId.get(id)
  if (e?.macro) return t('shell.commands.macro.keysTakenMacro')
  return t('shell.commands.macro.keysTaken', { what: t(`shell.keys.${id}`) })
})
function onKeysDown(e) {
  if (e.key === 'Tab') return
  e.preventDefault()
  e.stopPropagation()
  if (e.key === 'Backspace' || e.key === 'Delete') {
    form.value.keys = ''
    return
  }
  if (e.key === 'Escape' || ['Shift', 'Control', 'Alt', 'Meta'].includes(e.key)) return
  const code = e.code || ''
  let key = ''
  if (/^Key[A-Z]$/.test(code)) key = code.slice(3).toLowerCase()
  else if (/^Digit\d$/.test(code)) key = code.slice(5)
  else if (/^F\d{1,2}$/.test(code)) key = code.toLowerCase()
  if (!key) return
  const mods = []
  if (e.metaKey || e.ctrlKey) mods.push('mod')
  if (e.altKey) mods.push('alt')
  if (e.shiftKey) mods.push('shift')
  // A bare letter belongs to the app's one-key bindings — commands take a modifier.
  if (!mods.length && !/^f\d/.test(key)) {
    error.value = t('shell.commands.macro.keysNeedMod')
    return
  }
  error.value = ''
  form.value.keys = [...mods, key].join('+')
}
const keyCaps = (k) => (k ? capsOf(parseChord(k)) : [])

async function save() {
  const f = form.value
  if (!f || busy.value) return
  if (keyConflict.value) {
    error.value = keyConflict.value
    return
  }
  busy.value = true
  error.value = ''
  try {
    const m = editing.value === 'new' ? null : store.macros.find((x) => x.id === editing.value)
    await store.saveMacro(m, {
      name: f.name,
      trigger: f.trigger,
      action: f.action,
      params: paramsOf(f),
      keys: f.keys,
      category: f.category,
      enabled: f.enabled,
      shared: f.shared,
    })
    close()
  } catch (e) {
    error.value =
      e?.status === 409
        ? t('shell.commands.macro.triggerTaken', { trigger: f.trigger })
        : humanizeError(e)
  } finally {
    busy.value = false
  }
}
async function remove(m) {
  try {
    await store.removeMacro(m)
    if (editing.value === m.id) close()
  } catch (e) {
    message.error(humanizeError(e))
  }
}
async function toggle(m, on) {
  try {
    await store.saveMacro(m, { ...m, enabled: on, params: m.params })
  } catch (e) {
    message.error(humanizeError(e))
  }
}
function onFormKey(e) {
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    close()
  } else if (e.key === 'Enter' && !e.shiftKey && e.target?.tagName !== 'TEXTAREA') {
    e.preventDefault()
    save()
  }
}
function summary(m) {
  const p = m.params || {}
  const what = t(`shell.commands.macro.action.${m.action}`)
  if (m.action === 'create_task') {
    const tpl = store.templates.find((x) => x.id === p.template_id)
    return [what, tpl?.name, ...(p.tags || [])].filter(Boolean).join(' · ')
  }
  if (m.action === 'open_link')
    return `${what} · ${String(p.url || '').replace(/^https?:\/\//, '')}`
  if (m.action === 'run_pipeline')
    return [what, p.mode === 'retry_failed' ? t('shell.commands.macro.retryFailed') : p.ref]
      .filter(Boolean)
      .join(' · ')
  return what
}
defineExpose({ open, editing, close })
</script>

<template>
  <div class="mp" data-testid="macros-pane">
    <div class="mp-title">
      <h2>{{ t('shell.commands.navMacros') }}</h2>
      <span class="mp-note">{{ t('shell.commands.macro.summary', store.macros.length) }}</span>
      <span class="mp-grow" />
      <div class="mp-seg" role="tablist">
        <button
          v-for="v in VIS"
          :key="v"
          type="button"
          class="mp-seg-b"
          :class="{ on: vis === v }"
          @click="vis = v"
        >
          {{ t(`shell.commands.macro.vis.${v}`) }} <span class="mp-n">{{ counts[v] }}</span>
        </button>
      </div>
      <button
        type="button"
        class="kit-btn primary"
        data-testid="macro-new"
        :disabled="editing !== null"
        @click="open(null)"
      >
        {{ t('shell.commands.macro.new') }}
      </button>
    </div>

    <div class="mp-head">
      <span>{{ t('shell.commands.macro.col.name') }}</span>
      <span>{{ t('shell.commands.macro.col.call') }}</span>
      <span>{{ t('shell.commands.macro.col.action') }}</span>
      <span>{{ t('shell.commands.macro.col.keys') }}</span>
      <span />
    </div>

    <template v-for="m in [...list, ...(editing === 'new' ? [{ id: 'new' }] : [])]" :key="m.id">
      <div
        v-if="m.id !== 'new'"
        class="mp-row"
        :class="{ off: !m.enabled, open: editing === m.id }"
        data-testid="macro-row"
        :data-trigger="m.trigger"
        @click="m.can_edit && editing === null && open(m)"
      >
        <span class="mp-name">
          {{ m.name }}
          <span v-if="!m.shared" class="mp-mine">{{ t('shell.commands.macro.mine') }}</span>
        </span>
        <span class="mp-call">&gt;{{ m.trigger }}</span>
        <span class="mp-what">{{ summary(m) }}</span>
        <span class="mp-keys">
          <Kbd v-for="c in keyCaps(m.keys)" :key="c">{{ c }}</Kbd>
          <span v-if="!m.keys" class="mp-dim">—</span>
        </span>
        <span class="mp-acts" @click.stop>
          <n-switch
            v-if="m.can_edit"
            size="small"
            :value="m.enabled"
            data-testid="macro-enabled"
            @update:value="(v) => toggle(m, v)"
          />
          <ArmButton
            v-if="m.can_edit"
            :title="t('shell.commands.macro.remove')"
            :confirm="t('shell.commands.macro.removeConfirm')"
            testid="macro-remove"
            @confirm="remove(m)"
          />
        </span>
      </div>

      <div
        v-if="editing === m.id && form"
        class="mp-form"
        data-testid="macro-form"
        @keydown="onFormKey"
      >
        <div class="mp-grid">
          <label class="mp-f">
            <span>{{ t('shell.commands.macro.col.name') }}</span>
            <input ref="nameEl" v-model="form.name" class="kf-input" data-testid="macro-name" />
          </label>
          <label class="mp-f">
            <span>{{ t('shell.commands.macro.col.call') }}</span>
            <input
              v-model="form.trigger"
              class="kf-input mp-mono"
              :placeholder="EX.trigger"
              data-testid="macro-trigger"
            />
          </label>
          <label class="mp-f">
            <span>{{ t('shell.commands.macro.col.action') }}</span>
            <n-select
              v-model:value="form.action"
              :options="actionOptions"
              data-testid="macro-action"
            />
          </label>
          <label class="mp-f">
            <span>{{ t('shell.commands.macro.col.keys') }}</span>
            <input
              :value="keyCaps(form.keys).join(' ')"
              class="kf-input"
              readonly
              :placeholder="t('shell.commands.macro.keysPlaceholder')"
              data-testid="macro-keys"
              @keydown="onKeysDown"
            />
          </label>
        </div>

        <div class="mp-params">
          <template v-if="form.action === 'create_task'">
            <label class="mp-f">
              <span>{{ t('shell.commands.macro.template') }}</span>
              <n-select v-model:value="form.template_id" :options="templateOptions" />
            </label>
            <label class="mp-f">
              <span>{{ t('shell.commands.macro.tags') }}</span>
              <input v-model="form.tags" class="kf-input" :placeholder="EX.tags" />
            </label>
            <label class="mp-f">
              <span>{{ t('shell.commands.macro.priority') }}</span>
              <n-select v-model:value="form.priority" :options="priorityOptions" />
            </label>
            <div class="mp-checks">
              <label
                ><input v-model="form.assignMe" type="checkbox" />
                {{ t('shell.commands.macro.assignMe') }}</label
              >
              <label
                ><input v-model="form.currentStage" type="checkbox" />
                {{ t('shell.commands.macro.currentStage') }}</label
              >
            </div>
          </template>
          <label v-else-if="form.action === 'open_link'" class="mp-f wide">
            <span>{{ EX.urlLabel }}</span>
            <input
              v-model="form.url"
              class="kf-input"
              :placeholder="EX.url"
              data-testid="macro-url"
            />
          </label>
          <template v-else-if="form.action === 'create_document'">
            <label class="mp-f">
              <span>{{ t('shell.commands.macro.docTemplate') }}</span>
              <n-select v-model:value="form.doc_template_id" :options="docTemplateOptions" />
            </label>
            <label class="mp-f">
              <span>{{ t('shell.commands.macro.docTitle') }}</span>
              <input v-model="form.title" class="kf-input" />
            </label>
          </template>
          <template v-else-if="form.action === 'run_pipeline'">
            <div class="mp-checks">
              <label
                ><input v-model="form.mode" type="radio" value="run" />
                {{ t('shell.commands.macro.newPipeline') }}</label
              >
              <label
                ><input v-model="form.mode" type="radio" value="retry_failed" />
                {{ t('shell.commands.macro.retryFailed') }}</label
              >
            </div>
            <label class="mp-f">
              <span>{{ t('shell.commands.macro.ref') }}</span>
              <input v-model="form.ref" class="kf-input mp-mono" :placeholder="EX.ref" />
            </label>
            <label v-if="form.mode === 'run'" class="mp-f wide">
              <span>{{ t('shell.commands.macro.variables') }}</span>
              <textarea
                v-model="form.variables"
                class="kf-input kf-area mp-mono"
                rows="2"
                :placeholder="EX.vars"
              />
            </label>
          </template>
        </div>

        <div class="mp-foot">
          <label class="mp-f inline">
            <span>{{ t('shell.commands.macro.category') }}</span>
            <input v-model="form.category" class="kf-input" />
          </label>
          <div class="mp-seg">
            <button
              type="button"
              class="mp-seg-b"
              :class="{ on: form.shared }"
              :disabled="!store.canManage"
              @click="form.shared = true"
            >
              {{ t('shell.commands.macro.shared') }}
            </button>
            <button
              type="button"
              class="mp-seg-b"
              :class="{ on: !form.shared }"
              @click="form.shared = false"
            >
              {{ t('shell.commands.macro.onlyMine') }}
            </button>
          </div>
          <span class="mp-grow" />
          <button type="button" class="kit-btn" @click="close">
            {{ t('common.action.cancel') }}
          </button>
          <button
            type="button"
            class="kit-btn primary"
            :disabled="busy || !form.name.trim() || !form.trigger.trim()"
            data-testid="macro-save"
            @click="save"
          >
            {{ t('common.action.save') }}
          </button>
        </div>
        <p v-if="error || keyConflict" class="mp-err" data-testid="macro-error">
          {{ error || keyConflict }}
        </p>
      </div>
    </template>

    <p v-if="!list.length && editing !== 'new'" class="mp-empty">
      {{ t('shell.commands.macro.empty') }}
    </p>
  </div>
</template>

<style scoped>
.mp {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.mp-title {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 10px;
  margin-bottom: 10px;
  min-height: 36px;
}
.mp-title h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
}
.mp-note,
.mp-dim {
  font-size: 13px;
  color: var(--t-text3);
}
.mp-grow {
  flex: 1;
}
.mp-seg {
  display: inline-flex;
  gap: 2px;
  padding: 2px;
  border-radius: 10px;
  background: var(--t-ctl);
}
.mp-seg-b {
  height: 26px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.mp-seg-b.on {
  background: var(--t-pop);
  color: var(--t-text1);
  font-weight: 600;
}
.mp-seg-b:disabled {
  opacity: 0.5;
  cursor: default;
}
.mp-n {
  color: var(--t-text3);
}
.mp-head,
.mp-row {
  display: grid;
  grid-template-columns: minmax(140px, 1.2fr) 110px minmax(160px, 2fr) 110px 84px;
  align-items: center;
  gap: 10px;
  padding: 0 10px;
}
.mp-head {
  height: 28px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.mp-row {
  min-height: 44px;
  border-radius: 12px;
  background: var(--t-col);
  cursor: pointer;
}
.mp-row:hover,
.mp-row.open {
  background: color-mix(in srgb, var(--t-col) 70%, var(--t-card));
}
.mp-row.off {
  opacity: 0.55;
}
.mp-name {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mp-mine {
  margin-left: 6px;
  padding: 1px 6px;
  border-radius: 999px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 10.5px;
  font-weight: 600;
}
.mp-call,
.mp-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
}
.mp-call {
  color: var(--t-primary);
}
.mp-what {
  color: var(--t-text2);
  font-size: 12.5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mp-keys {
  display: flex;
  gap: 3px;
}
.mp-acts {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}
.mp-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 2px 0 8px;
  padding: 12px;
  border-radius: 12px;
  background: var(--t-card);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--t-primary) 35%, transparent);
}
.mp-grid,
.mp-params {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 10px;
}
.mp-f {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--t-text3);
}
.mp-f.wide {
  grid-column: 1 / -1;
}
.mp-f.inline {
  flex-direction: row;
  align-items: center;
  gap: 8px;
}
.mp-checks {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 14px;
  font-size: 13px;
}
.mp-checks label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.mp-foot {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.mp-err {
  margin: 0;
  font-size: 12.5px;
  color: var(--t-danger, #d03050);
}
.mp-empty {
  padding: 18px 10px;
  color: var(--t-text3);
  font-size: 13px;
}
</style>
