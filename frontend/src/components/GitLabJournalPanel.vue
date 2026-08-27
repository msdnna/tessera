<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NButton, useMessage } from 'naive-ui'
import { OpenOutline, RefreshOutline } from '@vicons/ionicons5'
import { gitlab as glApi } from '@/api'
import { useFormat } from '@/composables/useFormat'
import { useRealtime } from '@/composables/useRealtime'
import { runDuration, elapsedSince } from '@/utils/duration'
import { priorityLabel } from '@/utils/priority'
import EmptyState from '@/components/EmptyState.vue'
import LoaderOverlay from '@/components/LoaderOverlay.vue'

// Embeddable body of the GitLab sync journal — rendered inside the GitLab
// integration modal's right pane (no modal/card wrapper of its own).
const props = defineProps({
  wsId: { type: String, default: null },
})

const message = useMessage()
const { t } = useI18n()
const { formatDue, formatDateTime } = useFormat()

const runs = ref([])
const loading = ref(false)
const expandedRunId = ref(null)
// runId → { items: action[], hasMore: bool, nextSeq: number|null }. Actions are
// keyset-paginated (a run can hold thousands) and carry no before/after diff —
// that's fetched per row on select (see selectAction).
const actionsByRun = ref({})
const loadingActions = ref(false)
const selectedAction = ref(null) // { ...action, runId, detail? }
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
    await loadActions(run, true)
  }
}

// loadActions fetches one keyset page of a run's actions. reset=true starts over;
// otherwise it appends the next page from the stored cursor ("Показать ещё").
async function loadActions(run, reset) {
  const existing = actionsByRun.value[run.id]
  const afterSeq = reset ? undefined : (existing?.nextSeq ?? undefined)
  loadingActions.value = true
  try {
    const { data } = await glApi.syncRunActions(props.wsId, run.id, { afterSeq })
    const page = data?.items || []
    const prev = reset ? [] : existing?.items || []
    actionsByRun.value = {
      ...actionsByRun.value,
      [run.id]: {
        items: [...prev, ...page],
        hasMore: !!data?.has_more,
        nextSeq: data?.next_after_seq ?? null,
      },
    }
  } catch (e) {
    message.error(e.message)
  } finally {
    loadingActions.value = false
  }
}

// selectAction shows a row's detail, lazily fetching the before/after diff the
// list omits. The diff is cached back onto the row so re-selecting is instant,
// and a guard drops the response if the user moved on to another row meanwhile.
async function selectAction(run, a) {
  selectedAction.value = { ...a, runId: run.id, detail: a.detail || {} }
  if (a.has_detail && a.detail === undefined) {
    try {
      const { data } = await glApi.syncActionDetail(props.wsId, run.id, a.id)
      const detail = data?.detail || {}
      a.detail = detail
      if (selectedAction.value?.id === a.id) {
        selectedAction.value = { ...selectedAction.value, detail }
      }
    } catch (e) {
      message.error(e.message)
    }
  }
}

async function retry() {
  const a = selectedAction.value
  if (!a) return
  retrying.value = true
  try {
    await glApi.retryWriteback(props.wsId, a.runId, a.id)
    message.success(t('gitlab.journal.queuedForRetry'))
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
// One aggregated row per run (entity_type='relation'), so relation counts never
// disturb the per-task created/updated counters: {added, removed, deferred}.
const relations = computed(() => detail.value.relations || null)
const isPush = computed(() => selectedAction.value?.direction === 'push')
const canRetry = computed(() => isPush.value && selectedAction.value?.status === 'fail')

// FIELD_ORDER is the render order, not a label table — the wording is looked up
// per call so a language switch reaches it (pitfall 1 of the #2799 plan). A key
// the backend adds later renders raw instead of as a missing translation.
const FIELD_ORDER = ['title', 'description', 'priority', 'column', 'completed', 'due', 'start']

function orderedEntries(obj) {
  if (!obj) return []
  return FIELD_ORDER.filter((k) => k in obj).map((k) => [k, obj[k]])
}

function fieldLabel(key) {
  return FIELD_ORDER.includes(key) ? t(`gitlab.journal.field.${key}`) : key
}

function fmtVal(key, v) {
  if (v === null || v === undefined || v === '') return '—'
  if (key === 'priority') return priorityLabel(v) ?? String(v)
  if (key === 'completed') {
    return t(v ? 'gitlab.journal.value.completed' : 'gitlab.journal.value.notCompleted')
  }
  if (key === 'due' || key === 'start') return formatDue(v)
  return String(v)
}

function fmtTime(s) {
  if (!s) return ''
  return formatDateTime(s, { day: '2-digit', month: 'short' })
}

function runCounts(run) {
  if (run.kind === 'push') {
    return t('gitlab.journal.deliveries', { n: run.action_count })
  }
  const parts = []
  if (run.created_count) parts.push(`+${run.created_count}`)
  if (run.updated_count) parts.push(`~${run.updated_count}`)
  return parts.join(' ') || t('gitlab.journal.noChanges')
}

const STATUS_KEYS = ['running', 'ok', 'partial', 'error', 'fail']
const TRIGGER_KEYS = ['manual', 'auto']

function statusLabel(status) {
  return STATUS_KEYS.includes(status) ? t(`gitlab.journal.status.${status}`) : status
}
function triggerLabel(trigger) {
  return TRIGGER_KEYS.includes(trigger) ? t(`gitlab.journal.trigger.${trigger}`) : trigger
}

// A run opened by the backend but not yet finished. Manual syncs are detached, so
// this is the live state the user watches instead of a blocking overlay.
function isRunning(run) {
  return run.status === 'running' || !run.finished_at
}

// Ticks once a second so the "running for N s" caption on an in-flight run advances.
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
  if (isRunning(run)) {
    return t('gitlab.journal.runningFor', { elapsed: elapsedSince(run.started_at, now.value) })
  }
  return runDuration(run.started_at, run.finished_at)
}

// A background run reports itself over the workspace socket — refresh so the
// running row flips to its final status without the user reopening the panel.
useRealtime(
  (ev) => {
    if (ev.type !== 'integration.sync' || ev.scope !== props.wsId) return
    // Its actions were only written at the end, so drop the (empty) cached list.
    const id = ev.data?.run_id
    if (id && actionsByRun.value[id]) {
      const next = { ...actionsByRun.value }
      delete next[id]
      actionsByRun.value = next
    }
    loadRuns(false)
  },
  // Reconnect / resync: reload the run list from the top.
  () => loadRuns(true),
)

// Push payload, rendered compactly per change kind.
function pushPayloadText() {
  const d = detail.value
  const p = d.payload || {}
  switch (d.change_kind) {
    case 'state':
      return t(
        p.state === 'closed' ? 'gitlab.journal.push.closeIssue' : 'gitlab.journal.push.openIssue',
      )
    case 'priority':
      return t('gitlab.journal.push.priority', { to: priorityLabel(p.priority) ?? p.priority })
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
          :text="$t('gitlab.journal.empty')"
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
                {{ triggerLabel(run.trigger) }} ·
                <template v-if="!isRunning(run)">{{ runCounts(run) }} · </template>
                {{ runDurationText(run) }}
              </span>
            </span>
            <span
              class="j-dot"
              :class="isRunning(run) ? 'running' : run.status"
              :title="statusLabel(isRunning(run) ? 'running' : run.status)"
            />
          </button>
          <div v-if="expandedRunId === run.id" class="j-actions">
            <empty-state
              v-if="!loadingActions && !(actionsByRun[run.id]?.items || []).length"
              size="small"
              :text="$t('gitlab.journal.noActions')"
            />
            <button
              v-for="a in actionsByRun[run.id]?.items || []"
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
            <div v-if="loadingActions" class="j-muted">{{ $t('common.state.loading') }}</div>
            <button
              v-else-if="actionsByRun[run.id]?.hasMore"
              class="j-more"
              @click="loadActions(run, false)"
            >
              {{ $t('gitlab.journal.more') }}
            </button>
          </div>
        </div>
      </div>

      <!-- RIGHT: selected action detail / diff -->
      <div class="j-right">
        <empty-state
          v-if="!selectedAction"
          size="small"
          :text="$t('gitlab.journal.selectAction')"
        />
        <template v-else>
          <div class="j-d-head">
            <span class="j-d-dir" :class="selectedAction.direction">
              {{
                selectedAction.direction === 'pull'
                  ? $t('gitlab.journal.direction.pull')
                  : $t('gitlab.journal.direction.push')
              }}
            </span>
            <span class="j-d-sum">{{ selectedAction.summary }}</span>
          </div>

          <!-- pull: changed fields (update) -->
          <div v-if="fields" class="j-sec">
            <div v-for="[key, f] in orderedEntries(fields)" :key="key" class="j-field">
              <span class="j-fl">{{ fieldLabel(key) }}</span>
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
              <span class="j-fl">{{ fieldLabel(key) }}</span>
              <span class="j-after">{{ fmtVal(key, v) }}</span>
            </div>
          </div>

          <!-- tags -->
          <div v-if="tags" class="j-sec">
            <div class="j-fl">{{ $t('gitlab.journal.tags') }}</div>
            <!-- `tag`, not `t`: the loop variable would shadow the translation
                 function for the rest of the template. -->
            <div class="j-chips">
              <span v-for="tag in tags.added || []" :key="'a' + tag" class="j-chip add">
                + {{ tag }}
              </span>
              <span v-for="tag in tags.removed || []" :key="'r' + tag" class="j-chip rem">
                − {{ tag }}
              </span>
            </div>
          </div>

          <!-- comments -->
          <div v-if="comments && comments.added" class="j-sec">
            <div class="j-fl">{{ $t('gitlab.journal.newComments', { n: comments.added }) }}</div>
            <div v-for="(b, i) in comments.new || []" :key="i" class="j-comment">{{ b }}</div>
          </div>

          <!-- assignees -->
          <div v-if="assignees && assignees.length" class="j-sec">
            <div class="j-fl">{{ $t('gitlab.journal.assignees') }}</div>
            <div class="j-chips">
              <span v-for="a in assignees" :key="a" class="j-chip">{{ a }}</span>
            </div>
          </div>

          <!-- relations (aggregated per run) -->
          <div v-if="relations" class="j-sec">
            <div class="j-fl">{{ $t('gitlab.journal.relations') }}</div>
            <div class="j-chips">
              <span v-if="relations.added" class="j-chip add">
                {{ $t('gitlab.journal.relationsAdded', relations.added, { n: relations.added }) }}
              </span>
              <span v-if="relations.removed" class="j-chip rem">
                {{ $t('gitlab.journal.relationsRemoved', { n: relations.removed }) }}
              </span>
              <span v-if="relations.deferred" class="j-chip">
                {{ $t('gitlab.journal.relationsDeferred', { n: relations.deferred }) }}
              </span>
            </div>
          </div>

          <!-- push: payload + result/error -->
          <div v-if="isPush" class="j-sec">
            <div class="j-field">
              <span class="j-fl">{{ $t('gitlab.journal.action') }}</span>
              <span class="j-after">{{ pushPayloadText() }}</span>
            </div>
            <div v-if="selectedAction.status === 'fail'" class="j-error">
              {{ selectedAction.error || detail.error || $t('gitlab.journal.deliveryFailed') }}
            </div>
            <div v-else-if="detail.result" class="j-ok">{{ detail.result }}</div>
          </div>

          <div class="j-d-foot">
            <a
              v-if="detail.url"
              class="j-link"
              :href="detail.url"
              target="_blank"
              rel="noopener noreferrer"
            >
              <n-icon :component="OpenOutline" /> {{ $t('gitlab.journal.openInGitLab') }}
            </a>
            <n-button
              v-if="canRetry"
              size="small"
              type="primary"
              :loading="retrying"
              @click="retry"
            >
              <template #icon><n-icon :component="RefreshOutline" /></template>
              {{ $t('gitlab.journal.retry') }}
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
/* skip — the sync deliberately left something alone (e.g. a parent's children could
   not be listed, so its subtasks were kept as they were). */
.j-op.skip {
  color: #f0a020;
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
.j-more {
  align-self: flex-start;
  margin: 4px 0 2px 8px;
  padding: 4px 10px;
  font-size: 12px;
  color: var(--t-primary);
  background: none;
  border: 1px solid var(--t-border);
  border-radius: 6px;
  cursor: pointer;
}
.j-more:hover {
  background: var(--t-hover);
}
</style>
