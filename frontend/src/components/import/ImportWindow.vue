<script setup>
// «Импорт задач» (#3068, #3095) — a large window (tier 2, kit/LargeWindow): the
// file → the column mapping → the preview → the run. The file is read here for
// the mapping and the preview (utils/taskImport); on «Импортировать» it goes to
// the server with the mapping, which parses it again and creates the tasks as a
// background job — its progress, steps and log are drawn from the job
// (stores/jobs, the socket; a poll as the fallback).
//
// Opened from «⋯ Действия доски» → «Импорт задач…» and the palette (`>import`).
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NSelect, NSwitch } from 'naive-ui'
import {
  CloudUploadOutline,
  DocumentTextOutline,
  GitCompareOutline,
  EyeOutline,
  PlayCircleOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline,
  EllipseOutline,
  WarningOutline,
  StopCircleOutline,
} from '@vicons/ionicons5'
import LargeWindow from '@/components/kit/LargeWindow.vue'
import EmptyState from '@/components/EmptyState.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import { boards as boardsApi, workspaces as wsApi, jobs as jobsApi } from '@/api'
import { useBoardTransferStore } from '@/stores/boardTransfer'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useJobsStore } from '@/stores/jobs'
import { useMessage } from '@/composables/useMessage'
import { useShortcuts } from '@/composables/useShortcuts'
import { useResponsive } from '@/composables/useResponsive'
import { humanizeError } from '@/utils/errors'
import { columnName } from '@/utils/defaultNames'
import { priorityLabel } from '@/utils/priority'
import { jobStepLabel } from '@/utils/jobLabels'
import { isFinal, progressPct } from '@/utils/jobs'
import {
  IMPORT_FIELDS,
  PREVIEW_ROWS,
  parseImportFile,
  autoMapping,
  previewRows,
} from '@/utils/taskImport'
import '@/components/kit/kit.css'

const { t, te } = useI18n()
const message = useMessage()
const transfer = useBoardTransferStore()
const ws = useWorkspacesStore()
const jobsStore = useJobsStore()
const { isMobile } = useResponsive()

const show = computed({
  get: () => !!transfer.importBoard,
  set: (v) => {
    if (!v) transfer.closeImport()
  },
})
const board = computed(() => transfer.importBoard)

const section = ref('file')
const winRef = ref(null)
const fileInput = ref(null)
const dragOver = ref(false)
const file = ref(null)
const table = ref(null) // { columns, rows }
const parseError = ref('')
const mapping = ref({})
const defaultColumn = ref(null)
const createTags = ref(true)
const columns = ref([])
const members = ref([])
const submitting = ref(false)
const runId = ref('')
const pollJob = ref(null)
let pollTimer = 0

function reset() {
  section.value = 'file'
  file.value = null
  table.value = null
  parseError.value = ''
  mapping.value = {}
  defaultColumn.value = null
  createTags.value = true
  submitting.value = false
  runId.value = ''
  pollJob.value = null
  clearTimeout(pollTimer)
}

watch(
  () => board.value?.id,
  async (id) => {
    reset()
    if (!id) return
    const [cols, mem] = await Promise.allSettled([
      boardsApi.columns(id),
      ws.currentId ? wsApi.members(ws.currentId) : Promise.resolve({ data: [] }),
    ])
    columns.value = cols.status === 'fulfilled' ? cols.value.data || [] : []
    members.value = mem.status === 'fulfilled' ? mem.value.data || [] : []
  },
  { immediate: true },
)
onBeforeUnmount(() => clearTimeout(pollTimer))

// ── the file ──
function pickFile() {
  fileInput.value?.click()
}
async function takeFile(f) {
  if (!f) return
  parseError.value = ''
  try {
    const text = await f.text()
    const parsed = parseImportFile(text, f.name)
    if (!parsed.rows.length) throw Object.assign(new Error('rows'), { code: 'noRows' })
    file.value = f
    table.value = parsed
    mapping.value = autoMapping(parsed.columns)
    section.value = 'mapping'
  } catch (e) {
    file.value = f
    table.value = null
    parseError.value = te(`board.transfer.err.${e.code}`)
      ? t(`board.transfer.err.${e.code}`)
      : e.message
  }
}
function onFileChange(e) {
  takeFile(e.target.files?.[0])
  e.target.value = ''
}
function onDrop(e) {
  dragOver.value = false
  takeFile(e.dataTransfer?.files?.[0])
}
const sizeText = (n) =>
  n >= 1 << 20
    ? `${(n / (1 << 20)).toFixed(1)} MB`
    : n >= 1024
      ? `${Math.round(n / 1024)} KB`
      : `${n} B`

// ── the mapping ──
const NONE = '__none__'
const columnOptions = computed(() => [
  { label: t('board.transfer.skipField'), value: NONE },
  ...(table.value?.columns || []).map((c) => ({ label: c, value: c })),
])
function setMapping(field, value) {
  const next = { ...mapping.value }
  if (value === NONE || !value) delete next[field]
  else next[field] = value
  mapping.value = next
}
const sampleOf = (field) => {
  const col = mapping.value[field]
  const row = table.value?.rows?.find((r) => String(r[col] || '').trim())
  return col && row ? row[col] : ''
}
const boardColumnOptions = computed(() =>
  columns.value.map((c) => ({ label: columnName(c), value: c.id })),
)
const hasTitle = computed(() => !!mapping.value.title)

// ── the preview ──
const preview = computed(() =>
  table.value
    ? previewRows(table.value.rows, mapping.value, {
        columns: columns.value.map((c) => ({ name: c.name })),
        members: members.value,
      })
    : [],
)
const rowCount = computed(() => table.value?.rows?.length || 0)
const warnText = (w) => t(`board.transfer.warn.${w}`)

// ── the run ──
const job = computed(() => (runId.value && jobsStore.get(runId.value)?.job) || pollJob.value)
const jobLines = computed(() => (runId.value && jobsStore.get(runId.value)?.lines) || [])
const running = computed(() => !!runId.value && !isFinal(job.value))

async function submit() {
  if (!file.value || !hasTitle.value || submitting.value || !board.value) return
  submitting.value = true
  try {
    const fd = new FormData()
    fd.append('file', file.value)
    fd.append('mapping', JSON.stringify(mapping.value))
    const options = { create_tags: createTags.value }
    if (defaultColumn.value) options.column_id = defaultColumn.value
    fd.append('options', JSON.stringify(options))
    const { data } = await boardsApi.importTasks(board.value.id, fd)
    runId.value = data.job.run_id
    jobsStore.apply(data.job, [])
    section.value = 'run'
    poll()
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    submitting.value = false
  }
}
// The socket usually brings every change; the poll covers a missed one.
async function poll() {
  clearTimeout(pollTimer)
  if (!runId.value) return
  try {
    const { data } = await jobsApi.get(runId.value)
    pollJob.value = data
    jobsStore.apply(data, [])
    jobsStore.addLines(runId.value, data.log || [])
    if (isFinal(data)) return
  } catch {
    /* the next tick tries again */
  }
  pollTimer = setTimeout(poll, 2000)
}
async function cancelRun() {
  try {
    await jobsApi.cancel(runId.value)
  } catch (e) {
    message.error(humanizeError(e))
  }
}
function again() {
  const b = board.value
  reset()
  transfer.openImport(b)
}

const stepIcon = (s) =>
  s.status === 'done'
    ? CheckmarkCircleOutline
    : s.status === 'failed'
      ? CloseCircleOutline
      : EllipseOutline
const stepText = (s) => jobStepLabel(s, { t, te })
const resultText = computed(() => {
  const j = job.value
  if (!j) return ''
  if (j.status === 'done')
    return t('board.transfer.done', { created: j.created || 0, skipped: j.meta?.skipped || 0 })
  if (j.status === 'cancelled') return t('board.transfer.cancelled', { created: j.created || 0 })
  if (j.status === 'failed') return j.error || t('board.transfer.failed')
  return ''
})

const navItems = computed(() => [
  { key: 'file', icon: DocumentTextOutline, label: t('board.transfer.step.file') },
  { key: 'mapping', icon: GitCompareOutline, label: t('board.transfer.step.mapping') },
  {
    key: 'preview',
    icon: EyeOutline,
    label: t('board.transfer.step.preview'),
    count: rowCount.value || null,
  },
  { key: 'run', icon: PlayCircleOutline, label: t('board.transfer.step.run') },
])

const topmost = () => !!winRef.value?.topmost()
useShortcuts(
  {
    'import.next': () => {
      if (section.value === 'mapping' && hasTitle.value) section.value = 'preview'
      else if (section.value === 'preview' && !runId.value) submit()
    },
  },
  { when: () => show.value && topmost() },
)
</script>

<template>
  <LargeWindow
    ref="winRef"
    v-model:section="section"
    v-model:show="show"
    :crumbs="[board?.name || '']"
    :title="t('board.transfer.importTitle')"
    :icon="CloudUploadOutline"
    tone="#0eb0a9"
    :items="navItems"
    testid="import-window"
    tab-testid="import-tab"
  >
    <template #aside>
      <span v-if="file && !isMobile" class="iw-aside">{{ file.name }}</span>
    </template>

    <div class="iw-pane">
      <!-- 1. The file. -->
      <template v-if="section === 'file'">
        <div
          class="iw-drop"
          :class="{ over: dragOver }"
          data-testid="import-drop"
          @click="pickFile"
          @dragover.prevent="dragOver = true"
          @dragleave="dragOver = false"
          @drop.prevent="onDrop"
        >
          <n-icon :component="CloudUploadOutline" :size="34" class="grad-icon" />
          <span class="iw-drop-t">{{ t('board.transfer.dropTitle') }}</span>
          <span class="iw-drop-s">{{ t('board.transfer.dropSub') }}</span>
          <input
            ref="fileInput"
            type="file"
            accept=".csv,.tsv,.txt,.json,text/csv,application/json"
            hidden
            data-testid="import-file"
            @change="onFileChange"
          />
        </div>
        <div v-if="file" class="iw-file">
          <n-icon :component="DocumentTextOutline" :size="18" />
          <span class="iw-file-n">{{ file.name }}</span>
          <span class="iw-file-s">{{ sizeText(file.size) }}</span>
          <span v-if="table" class="iw-file-s">{{
            t('board.transfer.rows', { n: rowCount })
          }}</span>
        </div>
        <div v-if="parseError" class="iw-error" data-testid="import-error">
          <n-icon :component="WarningOutline" :size="16" />
          {{ parseError }}
        </div>
        <div class="iw-opts">
          <label class="iw-opt">
            <span class="iw-opt-l">{{ t('board.transfer.defaultColumn') }}</span>
            <n-select
              v-model:value="defaultColumn"
              :options="boardColumnOptions"
              :placeholder="t('board.transfer.defaultColumnHint')"
              clearable
              size="small"
              class="iw-sel"
            />
          </label>
          <label class="iw-opt">
            <span class="iw-opt-l">{{ t('board.transfer.createTags') }}</span>
            <n-switch v-model:value="createTags" size="small" />
          </label>
        </div>
      </template>

      <!-- 2. The mapping: a task field ← a column of the file. -->
      <template v-else-if="section === 'mapping'">
        <EmptyState
          v-if="!table"
          :icon="DocumentTextOutline"
          :text="t('board.transfer.needFile')"
        />
        <template v-else>
          <p class="iw-lead">{{ t('board.transfer.mappingLead') }}</p>
          <div class="iw-map" data-testid="import-mapping">
            <div v-for="f in IMPORT_FIELDS" :key="f" class="iw-map-row" :data-field="f">
              <span class="iw-map-f">
                {{ t(`board.transfer.field.${f}`) }}
                <span v-if="f === 'title'" class="iw-req">*</span>
              </span>
              <n-select
                :value="mapping[f] || NONE"
                :options="columnOptions"
                size="small"
                class="iw-sel"
                @update:value="setMapping(f, $event)"
              />
              <span class="iw-map-s">{{ sampleOf(f) }}</span>
            </div>
          </div>
          <div v-if="!hasTitle" class="iw-error">
            <n-icon :component="WarningOutline" :size="16" />
            {{ t('board.transfer.err.noTitle') }}
          </div>
          <div class="iw-actions">
            <button
              type="button"
              class="kit-btn primary"
              :disabled="!hasTitle"
              data-testid="import-to-preview"
              @click="section = 'preview'"
            >
              {{ t('board.transfer.next') }}
              <ShortcutKeys v-if="!isMobile" id="import.next" joined />
            </button>
          </div>
        </template>
      </template>

      <!-- 3. The preview: the first rows as the import will read them. -->
      <template v-else-if="section === 'preview'">
        <EmptyState
          v-if="!table"
          :icon="DocumentTextOutline"
          :text="t('board.transfer.needFile')"
        />
        <template v-else>
          <p class="iw-lead">
            {{
              t('board.transfer.previewLead', {
                n: Math.min(PREVIEW_ROWS, rowCount),
                total: rowCount,
              })
            }}
          </p>
          <div class="iw-table-wrap">
            <table class="iw-table" data-testid="import-preview">
              <thead>
                <tr>
                  <th>{{ t('board.transfer.field.title') }}</th>
                  <th>{{ t('board.transfer.field.column') }}</th>
                  <th>{{ t('board.transfer.field.tags') }}</th>
                  <th>{{ t('board.transfer.field.assignees') }}</th>
                  <th>{{ t('board.transfer.field.priority') }}</th>
                  <th>{{ t('board.transfer.field.due_date') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(r, i) in preview" :key="i" :class="{ warn: r.warn.length }">
                  <td class="iw-title">
                    <n-icon
                      v-if="r.warn.length"
                      :component="WarningOutline"
                      :size="14"
                      class="iw-warn-ic"
                      :title="r.warn.map(warnText).join('\n')"
                    />
                    {{ r.title || '—' }}
                  </td>
                  <td>{{ r.column || '—' }}</td>
                  <td>
                    <span v-for="tg in r.tags" :key="tg" class="iw-tag">{{ tg }}</span>
                  </td>
                  <td>{{ r.assignees.join(', ') }}</td>
                  <td>{{ r.priority == null ? '?' : priorityLabel(r.priority) }}</td>
                  <td>{{ r.due_date == null ? '?' : r.due_date }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="iw-notes">
            <div v-for="w in [...new Set(preview.flatMap((r) => r.warn))]" :key="w" class="iw-note">
              <n-icon :component="WarningOutline" :size="14" />
              {{ warnText(w) }}
            </div>
          </div>
          <div class="iw-actions">
            <button
              type="button"
              class="kit-btn primary"
              :disabled="submitting || !!runId || !hasTitle"
              data-testid="import-submit"
              @click="submit"
            >
              {{ t('board.transfer.importN', { n: rowCount }) }}
              <ShortcutKeys v-if="!isMobile" id="import.next" joined />
            </button>
          </div>
        </template>
      </template>

      <!-- 4. The run: the job's progress, steps and log. -->
      <template v-else>
        <EmptyState
          v-if="!runId"
          :icon="PlayCircleOutline"
          :text="t('board.transfer.notStarted')"
        />
        <template v-else-if="job">
          <div class="iw-run-head">
            <span class="iw-run-t">{{ t(`jobs.status.${job.status}`) }}</span>
            <span class="iw-grow" />
            <button
              v-if="running && job.cancelable"
              type="button"
              class="kit-btn"
              data-testid="import-cancel"
              @click="cancelRun"
            >
              <n-icon :component="StopCircleOutline" :size="15" class="iw-red" />
              {{ t('jobs.cancel') }}
            </button>
            <button v-else-if="!running" type="button" class="kit-btn" @click="again">
              {{ t('board.transfer.again') }}
            </button>
          </div>
          <div v-if="progressPct(job) != null" class="iw-progress">
            <span class="iw-bar"
              ><span class="iw-bar-fill" :style="{ width: `${progressPct(job)}%` }"
            /></span>
            <span class="iw-progress-t">{{
              t('jobs.progress', {
                done: job.progress_done || 0,
                total: job.progress_total,
                pct: progressPct(job),
              })
            }}</span>
          </div>
          <div v-if="job.queue_pos" class="iw-lead">
            {{ t('board.transfer.queued', { n: job.queue_pos }) }}
          </div>
          <div v-if="resultText" class="iw-result" :class="job.status" data-testid="import-result">
            {{ resultText }}
          </div>
          <div v-if="job.steps?.length" class="iw-steps">
            <div v-for="s in job.steps" :key="s.key" class="iw-step" :class="s.status">
              <n-icon :component="stepIcon(s)" :size="16" />
              <span class="iw-grow">{{ stepText(s) }}</span>
              <span class="iw-step-v">{{ s.total ? `${s.done || 0} / ${s.total}` : '' }}</span>
            </div>
          </div>
          <div class="iw-log" data-testid="import-log">
            <div v-for="l in jobLines.slice(-200)" :key="l.seq" class="iw-line" :class="l.level">
              {{ l.text }}
            </div>
          </div>
        </template>
      </template>
    </div>
  </LargeWindow>
</template>

<style scoped>
.iw-pane {
  display: flex;
  flex-direction: column;
  gap: 14px;
  height: 100%;
  min-height: 0;
  overflow-y: auto;
  padding: 6px 16px 16px 0;
}
.iw-aside {
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 260px;
}
.iw-drop {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 170px;
  border: 1.5px dashed var(--t-border);
  border-radius: 16px;
  background: var(--t-sub);
  cursor: pointer;
  text-align: center;
  padding: 18px;
}
.iw-drop.over,
.iw-drop:hover {
  border-color: var(--t-primary);
}
.iw-drop-t {
  font-weight: 650;
  font-size: 14px;
}
.iw-drop-s {
  font-size: 12.5px;
  color: var(--t-text3);
}
.iw-file {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
}
.iw-file-n {
  font-weight: 600;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.iw-file-s {
  color: var(--t-text3);
  font-size: 12.5px;
}
.iw-error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 12px;
  background: color-mix(in srgb, var(--t-error) 10%, transparent);
  color: var(--t-error);
  font-size: 13px;
}
.iw-opts {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 14px;
  background: var(--t-sub);
}
.iw-opt {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 13px;
}
.iw-opt-l {
  flex: 1;
}
.iw-sel {
  width: 240px;
  max-width: 100%;
}
.iw-lead {
  margin: 0;
  font-size: 13px;
  color: var(--t-text2);
}
.iw-map {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.iw-map-row {
  display: grid;
  grid-template-columns: 150px 240px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  min-height: 34px;
}
.iw-map-f {
  font-size: 13px;
  font-weight: 600;
}
.iw-req {
  color: var(--t-error);
}
.iw-map-s {
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.iw-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.iw-table-wrap {
  overflow-x: auto;
  border-radius: 12px;
  background: var(--t-sub);
}
.iw-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12.5px;
}
.iw-table th {
  text-align: left;
  font-weight: 600;
  color: var(--t-text3);
  padding: 8px 10px;
  white-space: nowrap;
}
.iw-table td {
  padding: 7px 10px;
  border-top: 1px solid var(--t-border);
  vertical-align: top;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.iw-title {
  font-weight: 600;
}
.iw-warn-ic {
  color: #f0a020;
  vertical-align: -2px;
}
.iw-tag {
  display: inline-block;
  margin: 0 4px 2px 0;
  padding: 0 6px;
  border-radius: 6px;
  border: 1px solid var(--t-border);
  font-size: 11.5px;
}
.iw-notes {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.iw-note {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  color: var(--t-text2);
}
.iw-note :deep(.n-icon) {
  color: #f0a020;
}
.iw-run-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.iw-run-t {
  font-weight: 700;
  font-size: 16px;
}
.iw-run-t::first-letter {
  text-transform: uppercase;
}
.iw-grow {
  flex: 1;
}
.iw-red {
  color: var(--t-error);
}
.iw-progress {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.iw-bar {
  display: flex;
  height: 8px;
  border-radius: 4px;
  background: var(--t-pill);
  overflow: hidden;
}
.iw-bar-fill {
  border-radius: 4px;
  background-image: var(--t-accent-grad);
  transition: width 0.3s ease;
}
.iw-progress-t {
  font-size: 12.5px;
  color: var(--t-text2);
  font-variant-numeric: tabular-nums;
}
.iw-result {
  padding: 10px 12px;
  border-radius: 12px;
  background: color-mix(in srgb, #18a058 12%, transparent);
  font-size: 13px;
}
.iw-result.failed {
  background: color-mix(in srgb, var(--t-error) 10%, transparent);
  color: var(--t-error);
}
.iw-result.cancelled {
  background: var(--t-sub);
}
.iw-steps {
  display: flex;
  flex-direction: column;
}
.iw-step {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 26px;
  font-size: 13px;
  color: var(--t-text3);
}
.iw-step.done :deep(.n-icon) {
  color: #18a058;
}
.iw-step.running,
.iw-step.done {
  color: var(--t-text1);
}
.iw-step.failed :deep(.n-icon) {
  color: var(--t-error);
}
.iw-step-v {
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.iw-log {
  max-height: 260px;
  overflow-y: auto;
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--t-sub);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
  color: var(--t-text2);
}
.iw-line.warn {
  color: #b7791f;
}
.iw-line.error {
  color: var(--t-error);
}
@media (max-width: 768px) {
  .iw-map-row {
    grid-template-columns: 1fr;
    gap: 4px;
  }
  .iw-pane {
    padding-right: 0;
  }
}
</style>
