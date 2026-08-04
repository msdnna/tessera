<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { NIcon, NButton, useMessage } from 'naive-ui'
import { OpenOutline, RefreshOutline } from '@vicons/ionicons5'
import { gitlab as glApi } from '@/api'
import { useThemeStore } from '@/stores/theme'
import { useDateLocale } from '@/composables/useDateLocale'
import { useRealtime } from '@/composables/useRealtime'
import { runDuration, elapsedSince } from '@/utils/duration'
import { PRIORITY_LABELS } from '@/styles/tokens'
import EmptyState from '@/components/EmptyState.vue'
import LoaderOverlay from '@/components/LoaderOverlay.vue'

// Embeddable body of the GitLab sync journal — rendered inside the GitLab
// integration modal's right pane (no modal/card wrapper of its own).
const props = defineProps({
  wsId: { type: String, default: null },
})

const message = useMessage()
const theme = useThemeStore()
const { formatDue } = useDateLocale()

const runs = ref([])
const loading = ref(false)
const expandedRunId = ref(null)
const actionsByRun = ref({}) // runId → actions[]
const loadingActions = ref(false)
const selectedAction = ref(null) // { ...action, runId }
const retrying = ref(false)

// reset=false is the live-refresh form (a background run finished): the list is
// re-fetched in place, keeping whatever the user has expanded/selected and
// skipping the overlay so the panel doesn't flash under them.
async function loadRuns(reset = true) {
  if (!props.wsId) return
  if (reset) {
    runs.value = []
    actionsByRun.value = {}
    expandedRunId.value = null
    selectedAction.value = null
    loading.value = true
  }
  try {
    const { data } = await glApi.syncRuns(props.wsId)
    runs.value = data || []
  } catch (e) {
    if (reset) message.error(e.message)
  } finally {
    loading.value = false
  }
}

async function toggleRun(run) {
  if (expandedRunId.value === run.id) {
    expandedRunId.value = null
    return
  }
  expandedRunId.value = run.id
  if (!actionsByRun.value[run.id]) {
    loadingActions.value = true
    try {
      const { data } = await glApi.syncRunActions(props.wsId, run.id)
      actionsByRun.value = { ...actionsByRun.value, [run.id]: data || [] }
    } catch (e) {
      message.error(e.message)
    } finally {
      loadingActions.value = false
    }
  }
}

function selectAction(run, a) {
  selectedAction.value = { ...a, runId: run.id }
}

async function retry() {
  const a = selectedAction.value
  if (!a) return
  retrying.value = true
  try {
    await glApi.retryWriteback(props.wsId, a.runId, a.id)
    message.success('Поставлено в очередь на повтор')
  } catch (e) {
    message.error(e.message)
  } finally {
    retrying.value = false
  }
}

// ── detail helpers ──
const detail = computed(() => selectedAction.value?.detail || {})
const fields = computed(() => detail.value.fields || null)
const after = computed(() => detail.value.after || null)
const tags = computed(() => detail.value.tags || null)
const comments = computed(() => detail.value.comments || null)
const assignees = computed(() => detail.value.assignees || null)
const isPush = computed(() => selectedAction.value?.direction === 'push')
const canRetry = computed(() => isPush.value && selectedAction.value?.status === 'fail')

const FIELD_LABELS = {
  title: 'Заголовок',
  description: 'Описание',
  priority: 'Приоритет',
  column: 'Колонка',
  completed: 'Статус',
  due: 'Срок',
  start: 'Начало',
}
const FIELD_ORDER = ['title', 'description', 'priority', 'column', 'completed', 'due', 'start']

function orderedEntries(obj) {
  if (!obj) return []
  return FIELD_ORDER.filter((k) => k in obj).map((k) => [k, obj[k]])
}

function fmtVal(key, v) {
  if (v === null || v === undefined || v === '') return '—'
  if (key === 'priority') return PRIORITY_LABELS[v] ?? String(v)
  if (key === 'completed') return v ? 'Выполнено' : 'Не выполнено'
  if (key === 'due' || key === 'start') return formatDue(v)
  return String(v)
}

function fmtTime(s) {
  if (!s) return ''
  const locale = theme.language === 'en' ? 'en-GB' : 'ru-RU'
  return new Date(s).toLocaleString(locale, {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    hour12: theme.timeFormat === '12h',
  })
}

function runCounts(run) {
  if (run.kind === 'push') {
    return `${run.action_count} дост.`
  }
  const parts = []
  if (run.created_count) parts.push(`+${run.created_count}`)
  if (run.updated_count) parts.push(`~${run.updated_count}`)
  return parts.join(' ') || 'без изменений'
}

const STATUS_LABEL = {
  running: 'выполняется',
  ok: 'успех',
  partial: 'частично',
  error: 'ошибка',
  fail: 'ошибка',
}
const TRIGGER_LABEL = { manual: 'вручную', auto: 'авто' }

// A run opened by the backend but not yet finished. Manual syncs are detached, so
// this is the live state the user watches instead of a blocking overlay.
function isRunning(run) {
  return run.status === 'running' || !run.finished_at
}

// Ticks once a second so the "идёт N с" caption on an in-flight run advances.
// Only armed while something is actually running, so an idle journal costs nothing.
const now = ref(Date.now())
let ticker = null
function syncTicker() {
  const live = runs.value.some(isRunning)
  if (live && !ticker) {
    ticker = setInterval(() => (now.value = Date.now()), 1000)
  } else if (!live && ticker) {
    clearInterval(ticker)
    ticker = null
  }
}
onBeforeUnmount(() => clearInterval(ticker))

// Duration cell: the finished span, or a live ticker while the run is in flight.
function runDurationText(run) {
  if (isRunning(run)) return `идёт ${elapsedSince(run.started_at, now.value)}`
  return runDuration(run.started_at, run.finished_at)
}

// A background run reports itself over the workspace socket — refresh so the
// running row flips to its final status without the user reopening the panel.
useRealtime((ev) => {
  if (ev.type !== 'integration.sync' || ev.scope !== props.wsId) return
  // Its actions were only written at the end, so drop the (empty) cached list.
  const id = ev.data?.run_id
  if (id && actionsByRun.value[id]) {
    const next = { ...actionsByRun.value }
    delete next[id]
    actionsByRun.value = next
  }
  loadRuns(false)
})

// Push payload, rendered compactly per change kind.
function pushPayloadText() {
  const d = detail.value
  const p = d.payload || {}
  switch (d.change_kind) {
    case 'state':
      return p.state === 'closed' ? 'закрыть issue' : 'открыть issue'
    case 'priority':
      return `приоритет → ${PRIORITY_LABELS[p.priority] ?? p.priority}`
    case 'comment':
      return p.body || ''
    default:
      return JSON.stringify(p)
  }
}

onMounted(loadRuns)
watch(
  () => props.wsId,
  () => loadRuns(),
)
watch(runs, syncTicker, { immediate: true })

defineExpose({ reload: () => loadRuns() })
</script>

<template>
  <div class="jp-wrap">
    <div class="j-body">
      <!-- LEFT: runs + their actions -->
      <div class="j-left">
        <empty-state
          v-if="!loading && !runs.length"
          size="small"
          text="Журнал пуст — синхронизация ещё не запускалась"
        />
        <div
          v-for="run in runs"
          :key="run.id"
          class="j-run"
          :class="{ open: expandedRunId === run.id }"
        >
          <button class="j-run-head" @click="toggleRun(run)">
            <span class="j-kind" :class="run.kind">{{
              run.kind === 'pull' ? 'Pull' : 'Push'
            }}</span>
            <span class="j-run-main">
              <span class="j-run-time">{{ fmtTime(run.started_at) }}</span>
              <span class="j-run-meta">
                {{ TRIGGER_LABEL[run.trigger] }} ·
                <template v-if="!isRunning(run)">{{ runCounts(run) }} · </template>
                {{ runDurationText(run) }}
              </span>
            </span>
            <span
              class="j-dot"
              :class="isRunning(run) ? 'running' : run.status"
              :title="STATUS_LABEL[isRunning(run) ? 'running' : run.status]"
            />
          </button>
          <div v-if="expandedRunId === run.id" class="j-actions">
            <div v-if="loadingActions" class="j-muted">Загрузка…</div>
            <empty-state
              v-else-if="!(actionsByRun[run.id] || []).length"
              size="small"
              text="Нет записанных действий"
            />
            <button
              v-for="a in actionsByRun[run.id] || []"
              :key="a.id"
              class="j-action"
              :class="{
                sel: selectedAction && selectedAction.id === a.id,
                fail: a.status === 'fail',
              }"
              @click="selectAction(run, a)"
            >
              <span class="j-op" :class="a.op">{{ a.op }}</span>
              <span class="j-action-sum">{{ a.summary }}</span>
            </button>
          </div>
        </div>
      </div>

      <!-- RIGHT: selected action detail / diff -->
      <div class="j-right">
        <empty-state
          v-if="!selectedAction"
          size="small"
          text="Выберите действие, чтобы увидеть детали"
        />
        <template v-else>
          <div class="j-d-head">
            <span class="j-d-dir" :class="selectedAction.direction">
              {{ selectedAction.direction === 'pull' ? 'GitLab → Tessera' : 'Tessera → GitLab' }}
            </span>
            <span class="j-d-sum">{{ selectedAction.summary }}</span>
          </div>

          <!-- pull: changed fields (update) -->
          <div v-if="fields" class="j-sec">
            <div v-for="[key, f] in orderedEntries(fields)" :key="key" class="j-field">
              <span class="j-fl">{{ FIELD_LABELS[key] || key }}</span>
              <div class="j-diff">
                <span class="j-before">{{ fmtVal(key, f.before) }}</span>
                <span class="j-arrow">→</span>
                <span class="j-after">{{ fmtVal(key, f.after) }}</span>
              </div>
            </div>
          </div>

          <!-- pull: created task snapshot -->
          <div v-if="after" class="j-sec">
            <div v-for="[key, v] in orderedEntries(after)" :key="key" class="j-field">
              <span class="j-fl">{{ FIELD_LABELS[key] || key }}</span>
              <span class="j-after">{{ fmtVal(key, v) }}</span>
            </div>
          </div>

          <!-- tags -->
          <div v-if="tags" class="j-sec">
            <div class="j-fl">Теги</div>
            <div class="j-chips">
              <span v-for="t in tags.added || []" :key="'a' + t" class="j-chip add">+ {{ t }}</span>
              <span v-for="t in tags.removed || []" :key="'r' + t" class="j-chip rem"
                >− {{ t }}</span
              >
            </div>
          </div>

          <!-- comments -->
          <div v-if="comments && comments.added" class="j-sec">
            <div class="j-fl">Новые комментарии ({{ comments.added }})</div>
            <div v-for="(b, i) in comments.new || []" :key="i" class="j-comment">{{ b }}</div>
          </div>

          <!-- assignees -->
          <div v-if="assignees && assignees.length" class="j-sec">
            <div class="j-fl">Исполнители (GitLab)</div>
            <div class="j-chips">
              <span v-for="a in assignees" :key="a" class="j-chip">{{ a }}</span>
            </div>
          </div>

          <!-- push: payload + result/error -->
          <div v-if="isPush" class="j-sec">
            <div class="j-field">
              <span class="j-fl">Действие</span>
              <span class="j-after">{{ pushPayloadText() }}</span>
            </div>
            <div v-if="selectedAction.status === 'fail'" class="j-error">
              {{ selectedAction.error || detail.error || 'Ошибка доставки' }}
            </div>
            <div v-else-if="detail.result" class="j-ok">{{ detail.result }}</div>
          </div>

          <div class="j-d-foot">
            <a v-if="detail.url" class="j-link" :href="detail.url" target="_blank" rel="noopener">
              <n-icon :component="OpenOutline" /> Открыть в GitLab
            </a>
            <n-button
              v-if="canRetry"
              size="small"
              type="primary"
              :loading="retrying"
              @click="retry"
            >
              <template #icon><n-icon :component="RefreshOutline" /></template>
              Повторить
            </n-button>
          </div>
        </template>
      </div>
    </div>
    <loader-overlay :show="loading" contained />
  </div>
</template>

<style scoped>
.jp-wrap {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 340px;
  /* Fill the pane height when embedded (parent passes .gl-pane-fill); harmless
     as a standalone block otherwise. */
  height: 100%;
}
.j-body {
  display: grid;
  grid-template-columns: minmax(0, 300px) minmax(0, 1fr);
  gap: 0;
  flex: 1 1 auto;
  min-height: 0;
}
.j-left {
  overflow-y: auto;
  border-right: 1px solid var(--t-border);
  padding-right: 10px;
}
.j-right {
  overflow-y: auto;
  padding-left: 16px;
}

/* runs */
.j-run + .j-run {
  margin-top: 2px;
}
.j-run-head {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 8px;
  background: none;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  text-align: left;
  color: var(--t-text1);
}
.j-run-head:hover {
  background: var(--t-hover);
}
.j-kind {
  flex: none;
  font-size: 11px;
  font-weight: 700;
  padding: 2px 7px;
  border-radius: 6px;
  letter-spacing: 0.02em;
}
.j-kind.pull {
  color: #2b6cb0;
  background: rgba(43, 108, 176, 0.14);
}
.j-kind.push {
  color: #805ad5;
  background: rgba(128, 90, 213, 0.14);
}
.j-run-main {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  line-height: 1.25;
}
.j-run-time {
  font-size: 13px;
}
.j-run-meta {
  font-size: 11px;
  color: var(--t-text3);
}
.j-dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #18a058;
}
.j-dot.partial {
  background: #f0a020;
}
.j-dot.error {
  background: #d03050;
}
/* In-flight run: the accent gradient (design language — non-neutral elements
   carry the soft diagonal), softly pulsing. This is the only progress affordance
   a detached sync gets, so it has to read as alive. */
.j-dot.running {
  background-image: var(--t-accent-grad);
  animation: j-dot-pulse 1.4s ease-in-out infinite;
}
@keyframes j-dot-pulse {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.45;
    transform: scale(0.7);
  }
}
@media (prefers-reduced-motion: reduce) {
  .j-dot.running {
    animation: none;
  }
}
.j-actions {
  padding: 2px 0 6px 8px;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.j-action {
  display: flex;
  align-items: baseline;
  gap: 7px;
  padding: 5px 8px;
  background: none;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  text-align: left;
  color: var(--t-text2);
  font-size: 12.5px;
}
.j-action:hover {
  background: var(--t-hover);
}
.j-action.sel {
  /* Soft same-hue tint (not the saturated accent gradient) so the coloured op
     label and the dark/light summary text keep contrast on both themes. */
  background: color-mix(in srgb, var(--t-primary) 14%, var(--t-surface));
  color: var(--t-text1);
}
.j-action.fail .j-action-sum {
  color: #d03050;
}
.j-op {
  flex: none;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  color: var(--t-text3);
  width: 46px;
}
.j-op.create {
  color: #18a058;
}
.j-op.update {
  color: #2080f0;
}
.j-op.delete {
  color: #d03050;
}
.j-action-sum {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* detail */
.j-d-head {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 14px;
}
.j-d-dir {
  font-size: 11px;
  font-weight: 700;
  align-self: flex-start;
  padding: 2px 8px;
  border-radius: 6px;
}
.j-d-dir.pull {
  color: #2b6cb0;
  background: rgba(43, 108, 176, 0.14);
}
.j-d-dir.push {
  color: #805ad5;
  background: rgba(128, 90, 213, 0.14);
}
.j-d-sum {
  font-size: 14px;
  font-weight: 600;
  color: var(--t-text1);
}
.j-sec {
  padding: 10px 0;
  border-top: 1px solid var(--t-border);
}
.j-field {
  display: flex;
  gap: 10px;
  align-items: baseline;
  padding: 3px 0;
}
.j-fl {
  flex: none;
  width: 110px;
  font-size: 12px;
  color: var(--t-text3);
}
.j-diff {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
  min-width: 0;
}
.j-before {
  color: #d03050;
  text-decoration: line-through;
  opacity: 0.85;
  word-break: break-word;
}
.j-after {
  color: var(--t-text1);
  word-break: break-word;
}
.j-arrow {
  color: var(--t-text3);
}
.j-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 4px;
}
.j-chip {
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 10px;
  background: var(--t-surface-alt);
  color: var(--t-text2);
}
.j-chip.add {
  color: #18a058;
  background: rgba(24, 160, 88, 0.13);
}
.j-chip.rem {
  color: #d03050;
  background: rgba(208, 48, 80, 0.13);
}
.j-comment {
  font-size: 12.5px;
  color: var(--t-text2);
  background: var(--t-surface-alt);
  border-radius: 8px;
  padding: 7px 9px;
  margin-top: 5px;
  white-space: pre-wrap;
  word-break: break-word;
}
.j-error {
  margin-top: 6px;
  font-size: 12.5px;
  color: #d03050;
  background: rgba(208, 48, 80, 0.1);
  border-radius: 8px;
  padding: 7px 9px;
  word-break: break-word;
}
.j-ok {
  margin-top: 6px;
  font-size: 12.5px;
  color: #18a058;
}
.j-d-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: 16px;
}
.j-link {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 13px;
  color: var(--t-primary);
  text-decoration: none;
}
.j-link:hover {
  text-decoration: underline;
}
.j-muted {
  font-size: 12px;
  color: var(--t-text3);
  padding: 6px 8px;
}
</style>
