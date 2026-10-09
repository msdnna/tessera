<script setup>
// «Фоновые задания» — a large window (v2, #2975, mockup JobsWin): the instance's
// background work as the jobs registry reports it (internal/jobs, GET
// /admin/jobs). The navigation filters by status and by kind; the middle column
// is the list (52px rows), the right one the picked job. Only the job types
// that really exist are drawn — the tick-loop workers and the GitLab syncs; the
// mockup's history, steps, progress and live log are #3067.
//
// Opened from the footer «⋯» and by G J (administrators only).
import { ref, computed, watch, onBeforeUnmount, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NTooltip } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  ChevronBackOutline,
  PlayOutline,
  StopCircleOutline,
  ServerOutline,
  LayersOutline,
  PlayCircleOutline,
  TimeOutline,
  AlertCircleOutline,
  CheckmarkCircleOutline,
  SearchOutline,
  HourglassOutline,
  LogoGitlab,
  ListOutline,
  CopyOutline,
  RefreshOutline,
} from '@vicons/ionicons5'
import EmptyState from '@/components/EmptyState.vue'
import LargeWindow from './kit/LargeWindow.vue'
import ShortcutKeys from './ShortcutKeys.vue'
import Kbd from './Kbd.vue'
import { admin as adminApi } from '@/api'
import { runDuration, elapsedSince } from '@/utils/duration'
import { jobName as jobLabel, jobOpText } from '@/utils/jobLabels'
import { stubTask } from '@/utils/featureStubs'
import { cap } from '@/utils/keyCaps'
import { useFormat } from '@/composables/useFormat'
import { useResponsive } from '@/composables/useResponsive'
import { useShortcuts } from '@/composables/useShortcuts'
import './kit/kit.css'

const props = defineProps({ show: { type: Boolean, default: false } })
const emit = defineEmits(['update:show'])
const message = useMessage()
const { t, te } = useI18n()
const { formatTime } = useFormat()

const jobs = ref([])
const selectedKey = ref(null)
const loading = ref(false)
const refreshing = ref(false)
const section = ref('all')
const query = ref('')
const winRef = ref(null)
const searchEl = ref(null)
// A 1s ticker drives the live "processing"/next-run clocks; polling reloads the
// list every 3s while the window is open.
const now = ref(Date.now())
let pollTimer = null
let tickTimer = null

// Phone layout (#2893): list and detail are two screens, not two stacked panes.
const { isMobile } = useResponsive()

const isWorker = (j) => j.kind === 'worker'
const STATUS_FILTERS = ['running', 'pending', 'failed', 'done']
const KIND_FILTERS = { workers: 'worker', syncs: 'sync' }
const countBy = (pred) => jobs.value.filter(pred).length
const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  return jobs.value.filter((j) => {
    if (STATUS_FILTERS.includes(section.value) && j.status !== section.value) return false
    if (KIND_FILTERS[section.value] && j.kind !== KIND_FILTERS[section.value]) return false
    if (!q) return true
    return `${jobName(j)} ${opText(j)}`.toLowerCase().includes(q)
  })
})
const selected = computed(() => jobs.value.find((j) => j.key === selectedKey.value) || null)
const mobileDetail = computed(() => isMobile.value && !!selected.value)

async function load() {
  loading.value = true
  try {
    const { data } = await adminApi.jobs()
    jobs.value = data || []
    // Pre-selecting the first job is a convenience for the side-by-side layout;
    // on a phone it would open the detail screen unasked — and, because load()
    // runs on a 3s poll, it would also drag the user back into it a beat after
    // they tapped "back".
    if (!selected.value && shown.value.length && !isMobile.value)
      selectedKey.value = shown.value[0].key
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

// Manual refresh: spin the icon for at least one full turn so the feedback is
// visible even when the request returns almost instantly.
async function refresh() {
  refreshing.value = true
  const started = Date.now()
  await load()
  const rest = 600 - (Date.now() - started)
  setTimeout(() => (refreshing.value = false), rest > 0 ? rest : 0)
}

function start() {
  load()
  pollTimer = setInterval(load, 3000)
  tickTimer = setInterval(() => (now.value = Date.now()), 1000)
}
function stop() {
  clearInterval(pollTimer)
  clearInterval(tickTimer)
  pollTimer = tickTimer = null
}
watch(
  () => props.show,
  (v) => (v ? start() : stop()),
)
onBeforeUnmount(stop)

// A filter that hides the picked job moves the pick to the first one shown.
watch(section, () => {
  if (isMobile.value) return
  if (!shown.value.some((j) => j.key === selectedKey.value))
    selectedKey.value = shown.value[0]?.key || null
})

async function runJob(key) {
  try {
    await adminApi.runJob(key)
    message.info(t('jobs.started'))
    await load()
  } catch (e) {
    message.error(e.message)
  }
}
async function cancelJob(key) {
  try {
    await adminApi.cancelJob(key)
    message.info(t('jobs.cancelling'))
    await load()
  } catch (e) {
    message.error(e.message)
  }
}

// Names and current operations arrive as catalog keys with the server's Russian
// wording as fallback — rendered in utils/jobLabels.
const jobName = (j) => jobLabel(j, { t, te })
const opText = (j) => jobOpText(j, { t, te })
// A status the backend adds later shows raw rather than as a missing translation.
const statusLabel = (s) => (STATUS_FILTERS.includes(s) ? t(`jobs.status.${s}`) : s)
// "worker" is a term of art and stays as it is in every locale; a discrete run
// is a sync.
const kindLabel = (j) => t(isWorker(j) ? 'jobs.kind.worker' : 'jobs.kind.sync')
const MODES = ['incremental', 'full']
const modeLabel = (j) => (MODES.includes(j.mode) ? t(`jobs.mode.${j.mode}`) : j.mode || '')
const TRIGGERS = ['manual', 'auto']
const triggerLabel = (j) =>
  TRIGGERS.includes(j.trigger) ? t(`jobs.trigger.${j.trigger}`) : j.trigger || ''
// "run now" is only for the tick-loop workers; a per-integration sync is started
// from the GitLab window.
const canRun = (j) => isWorker(j)
const canCancel = (j) => j.cancelable && j.status === 'running'

// A running job's live elapsed; a finished one's total duration.
function processingText(j) {
  if (!j) return ''
  if (j.status === 'running' && j.started_at) return elapsedSince(j.started_at, now.value)
  return runDuration(j.started_at, j.finished_at)
}
// When a worker (ticker) next fires: last_tick + interval.
function nextRunText(j) {
  if (!isWorker(j) || !j.last_tick_at || !j.interval_sec) return ''
  const next = new Date(j.last_tick_at).getTime() + j.interval_sec * 1000
  const delta = Math.round((next - now.value) / 1000)
  if (delta <= 0) return t('jobs.next.soon')
  return delta >= 60
    ? t('jobs.next.inMinutes', { m: Math.floor(delta / 60), s: delta % 60 })
    : t('jobs.next.inSeconds', { s: delta })
}
function fmtTime(iso) {
  return iso ? formatTime(iso, { second: '2-digit' }) : '—'
}
// The row's tile: GitLab orange for a sync, the accent for a worker.
const tileOf = (j) => (isWorker(j) ? ServerOutline : LogoGitlab)
const toneOf = (j) => (isWorker(j) ? 'var(--t-primary)' : '#fc6d26')
function rowSub(j) {
  if (isWorker(j)) {
    const next = nextRunText(j)
    return [opText(j), next && t('jobs.nextRun', { when: next })].filter(Boolean).join(' · ')
  }
  return [modeLabel(j), triggerLabel(j)].filter(Boolean).join(' · ') || opText(j)
}

const historyTask = stubTask('jobs.history')
const navItems = computed(() => [
  { key: 'all', icon: LayersOutline, label: t('jobs.nav.all'), count: jobs.value.length || null },
  {
    key: 'running',
    icon: PlayCircleOutline,
    label: t('jobs.nav.running'),
    count: countBy((j) => j.status === 'running') || null,
  },
  {
    key: 'pending',
    icon: TimeOutline,
    label: t('jobs.nav.pending'),
    count: countBy((j) => j.status === 'pending') || null,
  },
  {
    key: 'failed',
    icon: AlertCircleOutline,
    label: t('jobs.nav.failed'),
    count: countBy((j) => j.status === 'failed') || null,
    tone: 'danger',
  },
  {
    key: 'done',
    icon: CheckmarkCircleOutline,
    label: t('jobs.nav.done'),
    count: countBy((j) => j.status === 'done') || null,
  },
  { heading: t('jobs.nav.type') },
  {
    key: 'workers',
    icon: ServerOutline,
    label: t('jobs.nav.workers'),
    count: countBy(isWorker) || null,
  },
  {
    key: 'syncs',
    icon: LogoGitlab,
    label: t('jobs.nav.syncs'),
    count: countBy((j) => !isWorker(j)) || null,
  },
])
const runningCount = computed(() => countBy((j) => j.status === 'running' && !isWorker(j)))
const pendingCount = computed(() => countBy((j) => j.status === 'pending'))

// ── keys ──
const listEl = ref(null)
function pick(j) {
  selectedKey.value = j.key
}
function onListKey(e) {
  if (e.isComposing || e.target?.tagName === 'INPUT') return
  if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
  e.preventDefault()
  const list = shown.value
  if (!list.length) return
  const i = list.findIndex((j) => j.key === selectedKey.value)
  const next = list[Math.max(0, Math.min(list.length - 1, i + (e.key === 'ArrowDown' ? 1 : -1)))]
  selectedKey.value = next.key
  nextTick(() =>
    listEl.value
      ?.querySelector(`[data-key="${CSS.escape(next.key)}"]`)
      ?.focus({ preventScroll: false }),
  )
}
function paneEscape() {
  if (query.value) {
    query.value = ''
    return true
  }
  if (isMobile.value && selected.value) {
    selectedKey.value = null
    return true
  }
  return false
}
const topmost = () => !!winRef.value?.topmost()
useShortcuts(
  {
    'win.filter': () => searchEl.value?.focus(),
    'jobs.stop': () => selected.value && canCancel(selected.value) && cancelJob(selected.value.key),
    'jobs.run': () => selected.value && canRun(selected.value) && runJob(selected.value.key),
  },
  { when: topmost },
)
</script>

<template>
  <LargeWindow
    ref="winRef"
    v-model:section="section"
    :show="show"
    :crumbs="[t('jobs.crumb')]"
    :title="t('jobs.title')"
    :icon="HourglassOutline"
    tone="#3b82f6"
    :items="navItems"
    :escape="paneEscape"
    testid="jobs-window"
    tab-testid="jobs-tab"
    @update:show="emit('update:show', $event)"
  >
    <template #aside>
      <span v-if="!isMobile" class="bj-summary">
        {{ t('jobs.summary', { running: runningCount, pending: pendingCount }) }}
      </span>
      <button
        type="button"
        class="bj-refresh"
        :class="{ spinning: refreshing }"
        :title="t('jobs.refresh')"
        data-testid="jobs-refresh"
        @click="refresh"
      >
        <n-icon :component="RefreshOutline" :size="16" />
      </button>
    </template>
    <template #nav-foot>
      <n-tooltip :delay="300" placement="right">
        <template #trigger>
          <div class="lw-foot-row bj-stub">{{ t('jobs.historyStub') }}</div>
        </template>
        {{ t('common.stub.soon', { n: historyTask }) }}
      </n-tooltip>
      <div class="lw-foot-row">
        <ShortcutKeys id="nav.goJobs" bare />
        <span>{{ t('jobs.openKey') }}</span>
      </div>
    </template>

    <div class="bj-panes bj-modal" :class="{ mobile: isMobile }">
      <!-- The list. On a phone the list and the detail are two screens — v-show
           keeps the list's scroll position across a round trip into a job. -->
      <div v-show="!mobileDetail" class="bj-list-col" data-testid="jobs-list">
        <label class="bj-search">
          <n-icon :component="SearchOutline" :size="15" class="bj-search-ic" />
          <input
            ref="searchEl"
            v-model="query"
            spellcheck="false"
            :placeholder="t('jobs.search')"
            data-testid="jobs-search"
            @keydown.esc.stop="((query = ''), $event.target.blur())"
          />
          <Kbd v-if="!isMobile" soft>/</Kbd>
        </label>
        <div ref="listEl" class="bj-list t-hoverscroll" @keydown="onListKey">
          <EmptyState
            v-if="!shown.length"
            :icon="ServerOutline"
            :text="query ? t('jobs.noMatch') : t('jobs.empty')"
          />
          <button
            v-for="j in shown"
            :key="j.key"
            type="button"
            class="bj-row"
            :class="{ active: j.key === selectedKey }"
            :data-key="j.key"
            @click="pick(j)"
          >
            <span class="bj-tile" :style="{ '--tone': toneOf(j) }">
              <n-icon :component="tileOf(j)" :size="16" />
            </span>
            <span class="bj-row-main">
              <span class="bj-row-name">{{ jobName(j) }}</span>
              <span class="bj-row-sub">{{ rowSub(j) }}</span>
            </span>
            <span v-if="j.status === 'failed'" class="bj-pill err">{{
              statusLabel(j.status)
            }}</span>
            <span v-else-if="j.status === 'pending'" class="bj-pill">{{
              statusLabel(j.status)
            }}</span>
            <span v-else-if="j.status === 'done'" class="bj-done">
              <n-icon :component="CheckmarkCircleOutline" :size="14" />
              {{ fmtTime(j.finished_at) }}
            </span>
            <span v-else-if="!isWorker(j)" class="bj-live">
              <span class="bj-dot" />
              {{ processingText(j) }}
            </span>
            <span v-else class="bj-alive" :title="statusLabel(j.status)">
              <span class="bj-dot calm" />
            </span>
          </button>
        </div>
      </div>

      <!-- The picked job. Hidden until a row is picked on a phone — «выберите
           задание» prompts for a second pane that isn't there. -->
      <div v-show="!isMobile || mobileDetail" class="bj-detail" data-testid="jobs-detail">
        <EmptyState v-if="!selected" :icon="ServerOutline" :text="t('jobs.select')" />
        <template v-else>
          <button v-if="isMobile" type="button" class="bj-back" @click="selectedKey = null">
            <n-icon :component="ChevronBackOutline" :size="15" />
            {{ t('jobs.backToList') }}
          </button>
          <div class="bj-detail-head">
            <span class="bj-detail-name">{{ jobName(selected) }}</span>
            <span class="bj-status" :class="selected.status">
              <span class="bj-dot" />
              {{ statusLabel(selected.status) }}
            </span>
            <span class="bj-grow" />
            <button
              v-if="canCancel(selected)"
              type="button"
              class="kit-btn"
              data-testid="jobs-stop"
              @click="cancelJob(selected.key)"
            >
              <n-icon :component="StopCircleOutline" :size="15" class="bj-red" />
              {{ t('jobs.cancel') }}
              <ShortcutKeys v-if="!isMobile" id="jobs.stop" joined />
            </button>
            <button
              v-if="canRun(selected)"
              type="button"
              class="kit-btn"
              data-testid="jobs-run"
              @click="runJob(selected.key)"
            >
              <n-icon :component="PlayOutline" :size="15" />
              {{ t('jobs.run') }}
              <Kbd v-if="!isMobile">{{ cap('r') }}</Kbd>
            </button>
          </div>
          <div class="bj-kindline">
            <span class="bj-kind">{{ kindLabel(selected) }}</span>
            <span v-if="selected.persisted" class="bj-kind bj-journal">{{
              t('jobs.journal')
            }}</span>
            <span v-if="opText(selected)" class="bj-op">{{ opText(selected) }}</span>
          </div>

          <div class="bj-facts">
            <template v-if="isWorker(selected)">
              <div class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.lastActivity') }}</span>
                <span class="bj-fv">{{ fmtTime(selected.last_tick_at) }}</span>
              </div>
              <div v-if="nextRunText(selected)" class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.nextRun') }}</span>
                <span class="bj-fv">{{ nextRunText(selected) }}</span>
              </div>
            </template>
            <template v-else>
              <div v-if="modeLabel(selected)" class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.mode') }}</span>
                <span class="bj-fv">{{ modeLabel(selected) }}</span>
              </div>
              <div v-if="triggerLabel(selected)" class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.trigger') }}</span>
                <span class="bj-fv">{{ triggerLabel(selected) }}</span>
              </div>
              <div class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.startedAt') }}</span>
                <span class="bj-fv">{{ fmtTime(selected.started_at) }}</span>
              </div>
              <div class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.finishedAt') }}</span>
                <span class="bj-fv">{{
                  selected.finished_at ? fmtTime(selected.finished_at) : '—'
                }}</span>
              </div>
              <div class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.duration') }}</span>
                <span class="bj-fv">{{ processingText(selected) || '—' }}</span>
              </div>
              <div class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.createdUpdated') }}</span>
                <span class="bj-fv">+{{ selected.created }} / ~{{ selected.updated }}</span>
              </div>
            </template>
          </div>

          <div v-if="selected.error" class="bj-error">
            <n-icon :component="AlertCircleOutline" :size="16" />
            <span class="bj-err">{{ selected.error }}</span>
          </div>

          <!-- Steps, progress and the live log of the mockup (#3067): drawn,
               not yet fed. -->
          <n-tooltip :delay="300">
            <template #trigger>
              <div class="bj-logstub" aria-disabled="true" data-testid="jobs-log-stub">
                <div class="bj-log-head">
                  <n-icon :component="ListOutline" :size="15" />
                  <span class="bj-log-t">{{ t('jobs.log') }}</span>
                  <span class="bj-grow" />
                  <span class="bj-switch"><span class="bj-knob" /></span>
                  <span>{{ t('jobs.follow') }}</span>
                  <span class="kit-btn small stub">
                    <n-icon :component="CopyOutline" :size="13" />
                    {{ t('jobs.copy') }}
                  </span>
                </div>
                <div class="bj-log-body">
                  <span
                    >{{ t('jobs.logStub') }} · {{ t('common.stub.soon', { n: historyTask }) }}</span
                  >
                </div>
              </div>
            </template>
            {{ t('common.stub.soon', { n: historyTask }) }}
          </n-tooltip>
        </template>
      </div>
    </div>
  </LargeWindow>
</template>

<style scoped>
.bj-panes {
  display: grid;
  grid-template-columns: 380px minmax(0, 1fr);
  gap: 18px;
  min-height: 0;
  height: 100%;
  padding: 6px 16px 0 0;
}
.bj-panes.mobile {
  display: flex;
  flex-direction: column;
  padding-right: 0;
}
.bj-summary {
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.bj-refresh {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.bj-refresh:hover {
  background: var(--t-hover);
}
.bj-refresh.spinning :deep(.n-icon) {
  animation: bj-spin 0.8s linear infinite;
}
@keyframes bj-spin {
  to {
    transform: rotate(360deg);
  }
}
.bj-list-col {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 0;
}
.bj-panes.mobile .bj-list-col {
  flex: 1;
}
.bj-search {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  padding: 0 8px 0 12px;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  cursor: text;
}
.bj-search:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.bj-search input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
}
.bj-search input::placeholder {
  color: var(--t-placeholder);
}
.bj-search-ic {
  color: var(--t-text3);
}
.bj-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 2px;
}
.bj-row {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 52px;
  padding: 6px 10px;
  border: 0;
  border-radius: 12px;
  background: transparent;
  cursor: pointer;
  text-align: left;
  color: inherit;
  font: inherit;
}
.bj-row:hover {
  background: var(--t-hover);
}
.bj-row.active {
  background: var(--t-active);
}
.bj-row:focus-visible {
  outline: 0;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.bj-tile {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 9px;
  background: color-mix(in srgb, var(--tone) 18%, transparent);
  color: var(--tone);
}
.bj-row-main {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.bj-row-name {
  font-size: 13.5px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bj-row-sub {
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bj-pill {
  flex: none;
  display: inline-block;
  padding: 3px 9px;
  border-radius: 8px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 12px;
  font-weight: 600;
}
.bj-pill::first-letter,
.bj-status::first-letter {
  text-transform: uppercase;
}
.bj-pill.err {
  background: color-mix(in srgb, var(--t-error) 16%, transparent);
  color: var(--t-error);
}
.bj-done {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.bj-done :deep(.n-icon) {
  color: #2fa86b;
}
.bj-live {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text2);
  font-variant-numeric: tabular-nums;
}
.bj-alive {
  flex: none;
  display: inline-flex;
  padding: 0 4px;
}
.bj-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-image: var(--t-accent-grad);
  animation: bj-pulse 1.4s ease-in-out infinite;
}
.bj-dot.calm {
  background: #2fa86b;
  animation: none;
  opacity: 0.8;
}
@keyframes bj-pulse {
  50% {
    opacity: 0.45;
    transform: scale(0.75);
  }
}
@media (prefers-reduced-motion: reduce) {
  .bj-dot {
    animation: none;
  }
}
.bj-detail {
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 2px 2px 16px;
}
.bj-back {
  align-self: flex-start;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 32px;
  padding: 0 10px 0 6px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.bj-detail-head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  min-height: 34px;
}
.bj-detail-name {
  font-weight: 700;
  font-size: 18px;
  min-width: 0;
}
.bj-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 9px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  font-size: 12px;
  font-weight: 650;
}
.bj-status.failed {
  background: color-mix(in srgb, var(--t-error) 16%, transparent);
  color: var(--t-error);
}
.bj-status.failed .bj-dot,
.bj-status.done .bj-dot,
.bj-status.pending .bj-dot {
  animation: none;
  background: currentColor;
}
.bj-status.done,
.bj-status.pending {
  background: var(--t-pill);
  color: var(--t-text2);
}
.bj-grow {
  flex: 1;
}
.bj-red {
  color: var(--t-error);
}
.bj-kindline {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.bj-kind {
  padding: 2px 8px;
  border-radius: 7px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 11.5px;
}
.bj-journal {
  background: color-mix(in srgb, var(--t-primary) 14%, transparent);
  color: var(--t-primary);
}
/* The facts — tiles on --t-sub, like the mockup's «Запустила · Начато …». */
.bj-facts {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 4px 18px;
  padding: 12px 14px;
  border-radius: 14px;
  background: var(--t-sub);
}
.bj-fact {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.bj-fl {
  font-size: 12px;
  color: var(--t-text3);
}
.bj-fv {
  font-size: 13.5px;
  font-weight: 600;
  word-break: break-word;
}
.bj-error {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 14px;
  background: color-mix(in srgb, var(--t-error) 10%, transparent);
  color: var(--t-error);
}
.bj-err {
  font-size: 13px;
  color: var(--t-text1);
  word-break: break-word;
}
.bj-logstub {
  display: flex;
  flex-direction: column;
  gap: 10px;
  opacity: 0.6;
  cursor: default;
}
.bj-log-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  color: var(--t-text2);
}
.bj-log-t {
  font-weight: 650;
  color: var(--t-text1);
}
.bj-log-sub {
  color: var(--t-text3);
}
.bj-switch {
  display: inline-flex;
  align-items: center;
  width: 28px;
  height: 16px;
  padding: 2px;
  box-sizing: border-box;
  border-radius: 8px;
  background: var(--t-pill);
}
.bj-knob {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--t-text3);
}
.bj-log-body {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 96px;
  padding: 12px 14px;
  border-radius: 14px;
  background: var(--t-sub);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  color: var(--t-text3);
}
.bj-log-ic {
  color: var(--t-text3);
}
.bj-stub {
  opacity: 0.7;
  cursor: default;
}
</style>
