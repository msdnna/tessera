<script setup>
// «Фоновые задания» — a large window (v2, #2975, mockup JobsWin): the instance's
// background work as the jobs registry reports it (internal/jobs, GET
// /admin/jobs). The navigation filters by status and by job family; the middle
// column is the list (52px rows), the right one the picked job: progress with an
// ETA, the attempt and the place in the queue, the steps and the live log
// (#3067). Finished runs come from the 30-day history. Only the job types that
// really exist are drawn — the tick-loop workers and their on-demand passes, the
// GitLab syncs, the board exports and imports.
//
// Live: every change arrives over the socket (`job.updated`, stores/jobs); a slow
// poll is the fallback for a missed event.
//
// Opened from the footer «⋯» and by G J (administrators only).
import { ref, computed, watch, onBeforeUnmount, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
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
  CloseCircleOutline,
  EllipseOutline,
  SearchOutline,
  HourglassOutline,
  LogoGitlab,
  CopyOutline,
  RefreshOutline,
  SwapVerticalOutline,
  NotificationsOutline,
  ConstructOutline,
  DownloadOutline,
} from '@vicons/ionicons5'
import EmptyState from '@/components/EmptyState.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import LargeWindow from './kit/LargeWindow.vue'
import ShortcutKeys from './ShortcutKeys.vue'
import Kbd from './Kbd.vue'
import { admin as adminApi } from '@/api'
import { runDuration, elapsedSince } from '@/utils/duration'
import {
  jobName as jobLabel,
  jobOpText,
  jobStepLabel,
  jobTypeLabel,
  JOB_TYPES,
} from '@/utils/jobLabels'
import { jobUid, statusGroup, progressPct, upsertJob, logText } from '@/utils/jobs'
import { copyText } from '@/utils/clipboard'
import { cap } from '@/utils/keyCaps'
import { useJobsStore } from '@/stores/jobs'
import { downloadJobFile } from '@/composables/useJobFile'
import { useFormat } from '@/composables/useFormat'
import { useResponsive } from '@/composables/useResponsive'
import { useShortcuts } from '@/composables/useShortcuts'
import './kit/kit.css'

const props = defineProps({ show: { type: Boolean, default: false } })
const emit = defineEmits(['update:show'])
const message = useMessage()
const { t, te } = useI18n()
const { formatTime, formatDateTime } = useFormat()
const jobsStore = useJobsStore()

const jobs = ref([])
const selectedUid = ref(null)
const loading = ref(false)
const refreshing = ref(false)
const section = ref('all')
const query = ref('')
const winRef = ref(null)
const searchEl = ref(null)
const logEl = ref(null)
const follow = ref(true)
// A 1s ticker drives the live "processing"/next-run/ETA clocks; the poll is the
// socket's fallback.
const now = ref(Date.now())
let pollTimer = null
let tickTimer = null
const POLL_MS = 15000

// Phone layout (#2893): list and detail are two screens, not two stacked panes.
const { isMobile } = useResponsive()

const isWorker = (j) => j.kind === 'worker'
const STATUS_FILTERS = ['running', 'pending', 'failed', 'done']
const STATUSES = [...STATUS_FILTERS, 'cancelled']
const countBy = (pred) => jobs.value.filter(pred).length
const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  return jobs.value.filter((j) => {
    if (STATUS_FILTERS.includes(section.value) && statusGroup(j) !== section.value) return false
    if (JOB_TYPES.includes(section.value) && j.type !== section.value) return false
    if (!q) return true
    return `${jobName(j)} ${opText(j)} ${j.actor_name || ''}`.toLowerCase().includes(q)
  })
})
const selected = computed(() => jobs.value.find((j) => jobUid(j) === selectedUid.value) || null)
const mobileDetail = computed(() => isMobile.value && !!selected.value)

async function load() {
  loading.value = true
  try {
    const { data } = await adminApi.jobs()
    jobs.value = data || []
    // Pre-selecting the first job is a convenience for the side-by-side layout;
    // on a phone it would open the detail screen unasked — and, because load()
    // runs on a poll, it would also drag the user back into it a beat after
    // they tapped "back".
    if (!selected.value && shown.value.length && !isMobile.value)
      selectedUid.value = jobUid(shown.value[0])
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
  pollTimer = setInterval(load, POLL_MS)
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

// The socket's updates go over the loaded list: the same run takes the new
// state (a finishing run keeps its row), a new one goes on top.
watch(
  () => jobsStore.tick,
  () => {
    if (!props.show) return
    let list = jobs.value
    for (const { job } of Object.values(jobsStore.runs)) list = upsertJob(list, job)
    jobs.value = list
  },
)

// A filter that hides the picked job moves the pick to the first one shown.
watch(section, () => {
  if (isMobile.value) return
  if (!shown.value.some((j) => jobUid(j) === selectedUid.value))
    selectedUid.value = shown.value[0] ? jobUid(shown.value[0]) : null
})

// The picked job's log: its backlog from the API once, then the socket's lines.
const logLines = computed(
  () => (selectedUid.value && jobsStore.get(selectedUid.value)?.lines) || [],
)
async function loadLog(j) {
  if (!j || isWorker(j)) return
  try {
    const { data } = await adminApi.job(j.run_id ? `jobrun:${j.run_id}` : j.key)
    jobsStore.addLines(jobUid(j), data?.log || [])
  } catch {
    /* the log is a nicety — the row stands without it */
  }
}
watch(selectedUid, () => {
  follow.value = true
  loadLog(selected.value)
})
// Follow: keep the newest line in view.
watch(
  () => logLines.value.length,
  () => {
    if (!follow.value) return
    nextTick(() => {
      const el = logEl.value
      if (el) el.scrollTop = el.scrollHeight
    })
  },
)
function onLogScroll() {
  const el = logEl.value
  if (!el) return
  // Scrolling up to read stops following; back at the bottom resumes it.
  follow.value = el.scrollHeight - el.scrollTop - el.clientHeight < 8
}

async function runJob(key) {
  try {
    await adminApi.runJob(key)
    message.info(t('jobs.started'))
    await load()
  } catch (e) {
    message.error(e.message)
  }
}
async function cancelJob(j) {
  try {
    await adminApi.cancelJob(j.key)
    message.info(t('jobs.cancelling'))
    await load()
  } catch (e) {
    message.error(e.message)
  }
}
async function download(j) {
  try {
    await downloadJobFile(j)
  } catch (e) {
    message.error(e.message)
  }
}
async function copyLog() {
  const text = logText(logLines.value, fmtTime)
  if (text && (await copyText(text))) message.success(t('jobs.copied'))
}

// Names and current operations arrive as catalog keys with the server's Russian
// wording as fallback — rendered in utils/jobLabels.
const jobName = (j) => jobLabel(j, { t, te })
const opText = (j) => jobOpText(j, { t, te })
const stepText = (s) => jobStepLabel(s, { t, te })
const typeText = (type) => jobTypeLabel(type, { t, te })
// A status the backend adds later shows raw rather than as a missing translation.
const statusLabel = (s) => (STATUSES.includes(s) ? t(`jobs.status.${s}`) : s)
const KINDS = ['worker', 'sync', 'run', 'export', 'import']
const kindLabel = (j) => (KINDS.includes(j.kind) ? t(`jobs.kind.${j.kind}`) : j.kind)
const MODES = ['incremental', 'full']
const modeLabel = (j) => (MODES.includes(j.mode) ? t(`jobs.mode.${j.mode}`) : j.mode || '')
const TRIGGERS = ['manual', 'auto', 'webhook']
const triggerLabel = (j) =>
  TRIGGERS.includes(j.trigger) ? t(`jobs.trigger.${j.trigger}`) : j.trigger || ''
// "run now" is only for the tick-loop workers; a per-integration sync is started
// from the GitLab window.
const canRun = (j) => isWorker(j)
const canCancel = (j) => j.cancelable && (j.status === 'running' || j.status === 'pending')
const canDownload = (j) => j.kind === 'export' && j.status === 'done'

// A running job's live elapsed; a finished one's total duration.
function processingText(j) {
  if (!j) return ''
  if (j.status === 'running' && j.started_at) return elapsedSince(j.started_at, now.value)
  return runDuration(j.started_at, j.finished_at)
}
// When a worker (ticker) next fires: last_tick + interval.
function nextRunText(j) {
  if (!isWorker(j) || !j.last_tick_at || !j.interval_sec) return ''
  return inText(new Date(j.last_tick_at).getTime() + j.interval_sec * 1000)
}
function inText(at) {
  const delta = Math.round((at - now.value) / 1000)
  if (delta <= 0) return t('jobs.next.soon')
  return delta >= 60
    ? t('jobs.next.inMinutes', { m: Math.floor(delta / 60), s: delta % 60 })
    : t('jobs.next.inSeconds', { s: delta })
}
const retryText = (j) =>
  j.retry_at ? t('jobs.retryIn', { when: inText(new Date(j.retry_at).getTime()) }) : ''
function etaText(j) {
  if (j.status !== 'running' || !j.eta_sec) return ''
  const s = j.eta_sec
  return t('jobs.eta', {
    when: s >= 60 ? t('jobs.etaMin', { m: Math.round(s / 60) }) : t('jobs.etaSec', { s }),
  })
}
function progressText(j) {
  const pct = progressPct(j)
  if (pct == null) return ''
  return t('jobs.progress', { done: j.progress_done || 0, total: j.progress_total, pct })
}
function fmtTime(iso) {
  return iso ? formatTime(iso, { second: '2-digit' }) : '—'
}
const attemptText = (j) =>
  j.max_attempts > 1 ? t('jobs.attemptOf', { n: j.attempt || 1, max: j.max_attempts }) : ''

// The row's tile: by family — GitLab orange, import/export teal, notifications
// blue, the rest the accent.
const TILES = {
  gitlab: [LogoGitlab, '#fc6d26'],
  import_export: [SwapVerticalOutline, '#0eb0a9'],
  notify: [NotificationsOutline, '#2f80ed'],
  maintenance: [ConstructOutline, 'var(--t-primary)'],
}
const tileOf = (j) => (TILES[j.type] || [ServerOutline])[0]
const toneOf = (j) => (TILES[j.type] || [null, 'var(--t-primary)'])[1]
function rowSub(j) {
  if (isWorker(j)) {
    const next = nextRunText(j)
    return [opText(j), next && t('jobs.nextRun', { when: next })].filter(Boolean).join(' · ')
  }
  const meta = j.meta || {}
  const parts = [j.actor_name]
  if (j.kind === 'sync') parts.push(modeLabel(j), triggerLabel(j))
  else if (j.kind === 'run') parts.push(triggerLabel(j))
  else if (meta.rows != null) parts.push(t('jobs.rows', { n: meta.rows }))
  return parts.filter(Boolean).join(' · ') || opText(j)
}
function stepIcon(s) {
  if (s.status === 'done') return CheckmarkCircleOutline
  if (s.status === 'failed') return CloseCircleOutline
  return EllipseOutline
}
function stepValue(s) {
  if (s.status === 'done' && s.started_at && s.finished_at && !s.done)
    return runDuration(s.started_at, s.finished_at)
  if (s.total) return `${s.done || 0} / ${s.total}`
  if (s.done) return String(s.done)
  return s.status === 'pending' ? '—' : ''
}

const navItems = computed(() => [
  { key: 'all', icon: LayersOutline, label: t('jobs.nav.all'), count: jobs.value.length || null },
  {
    key: 'running',
    icon: PlayCircleOutline,
    label: t('jobs.nav.running'),
    count: countBy((j) => j.status === 'running' && !isWorker(j)) || null,
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
    count: countBy((j) => statusGroup(j) === 'done') || null,
  },
  { heading: t('jobs.nav.type') },
  ...JOB_TYPES.map((type) => ({
    key: type,
    icon: TILES[type][0],
    label: typeText(type),
    count: countBy((j) => j.type === type) || null,
  })),
])
const runningCount = computed(() => countBy((j) => j.status === 'running' && !isWorker(j)))
const pendingCount = computed(() => countBy((j) => j.status === 'pending'))

// ── keys ──
const listEl = ref(null)
function pick(j) {
  selectedUid.value = jobUid(j)
}
function onListKey(e) {
  if (e.isComposing || e.target?.tagName === 'INPUT') return
  if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
  e.preventDefault()
  const list = shown.value
  if (!list.length) return
  const i = list.findIndex((j) => jobUid(j) === selectedUid.value)
  const next = list[Math.max(0, Math.min(list.length - 1, i + (e.key === 'ArrowDown' ? 1 : -1)))]
  selectedUid.value = jobUid(next)
  nextTick(() =>
    listEl.value
      ?.querySelector(`[data-key="${CSS.escape(jobUid(next))}"]`)
      ?.focus({ preventScroll: false }),
  )
}
function paneEscape() {
  if (query.value) {
    query.value = ''
    return true
  }
  if (isMobile.value && selected.value) {
    selectedUid.value = null
    return true
  }
  return false
}
const topmost = () => !!winRef.value?.topmost()
useShortcuts(
  {
    'win.filter': () => searchEl.value?.focus(),
    'jobs.stop': () => selected.value && canCancel(selected.value) && cancelJob(selected.value),
    'jobs.run': () => selected.value && canRun(selected.value) && runJob(selected.value.key),
    'jobs.follow': () => (follow.value = !follow.value),
  },
  { when: topmost },
)
// ⌘C copies the log — only while nothing is selected: copying text out of the
// window keeps working as everywhere.
useShortcuts(
  { 'jobs.copyLog': copyLog },
  {
    when: () => topmost() && logLines.value.length > 0 && !String(window.getSelection?.() || ''),
  },
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
      <div class="lw-foot-row" data-testid="jobs-history-note">{{ t('jobs.history') }}</div>
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
            :key="jobUid(j)"
            type="button"
            class="bj-row"
            :class="{ active: jobUid(j) === selectedUid }"
            :data-key="jobUid(j)"
            :data-status="j.status"
            data-testid="jobs-row"
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
              j.queue_pos ? t('jobs.queuePos', { n: j.queue_pos }) : statusLabel(j.status)
            }}</span>
            <span v-else-if="j.status === 'cancelled'" class="bj-pill">{{
              statusLabel(j.status)
            }}</span>
            <span v-else-if="j.status === 'done'" class="bj-done">
              <n-icon :component="CheckmarkCircleOutline" :size="14" />
              {{ fmtTime(j.finished_at) }}
            </span>
            <span v-else-if="!isWorker(j)" class="bj-live">
              <span class="bj-dot" />
              {{ progressPct(j) != null ? `${progressPct(j)}%` : processingText(j) }}
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
          <button v-if="isMobile" type="button" class="bj-back" @click="selectedUid = null">
            <n-icon :component="ChevronBackOutline" :size="15" />
            {{ t('jobs.backToList') }}
          </button>
          <div class="bj-detail-head">
            <span class="bj-detail-name">{{ jobName(selected) }}</span>
            <span class="bj-status" :class="selected.status" data-testid="jobs-status">
              <span class="bj-dot" />
              {{ statusLabel(selected.status) }}
            </span>
            <span class="bj-grow" />
            <button
              v-if="canDownload(selected)"
              type="button"
              class="kit-btn"
              data-testid="jobs-download"
              @click="download(selected)"
            >
              <n-icon :component="DownloadOutline" :size="15" />
              {{ t('jobs.download') }}
            </button>
            <button
              v-if="canCancel(selected)"
              type="button"
              class="kit-btn"
              data-testid="jobs-stop"
              @click="cancelJob(selected)"
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
            <span v-if="selected.type" class="bj-kind bj-journal">{{
              typeText(selected.type)
            }}</span>
            <span v-if="opText(selected)" class="bj-op">{{ opText(selected) }}</span>
            <span v-if="retryText(selected)" class="bj-op">{{ retryText(selected) }}</span>
          </div>

          <!-- Progress: «248 из 401 · 62%» and the ETA. -->
          <div v-if="progressPct(selected) != null" class="bj-progress" data-testid="jobs-progress">
            <span class="bj-bar"
              ><span class="bj-bar-fill" :style="{ width: `${progressPct(selected)}%` }"
            /></span>
            <div class="bj-progress-line">
              <span class="bj-grow">{{ progressText(selected) }}</span>
              <span class="bj-eta">{{ etaText(selected) }}</span>
            </div>
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
              <div v-if="selected.actor_name" class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.startedBy') }}</span>
                <span class="bj-fv bj-who">
                  <UserAvatar
                    class="bj-ava"
                    :user-id="selected.actor_id"
                    :name="selected.actor_name"
                  />
                  {{ selected.actor_name }}
                </span>
              </div>
              <div v-else-if="triggerLabel(selected)" class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.trigger') }}</span>
                <span class="bj-fv">{{ triggerLabel(selected) }}</span>
              </div>
              <div v-if="modeLabel(selected)" class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.mode') }}</span>
                <span class="bj-fv">{{ modeLabel(selected) }}</span>
              </div>
              <div class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.startedAt') }}</span>
                <span class="bj-fv">{{
                  selected.started_at ? formatDateTime(selected.started_at) : '—'
                }}</span>
              </div>
              <div class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.duration') }}</span>
                <span class="bj-fv">{{ processingText(selected) || '—' }}</span>
              </div>
              <div v-if="attemptText(selected)" class="bj-fact" data-testid="jobs-attempt">
                <span class="bj-fl">{{ t('jobs.fact.attempt') }}</span>
                <span class="bj-fv">{{ attemptText(selected) }}</span>
              </div>
              <div v-if="selected.queue_pos" class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.queue') }}</span>
                <span class="bj-fv">{{ t('jobs.queuePos', { n: selected.queue_pos }) }}</span>
              </div>
              <div v-if="selected.kind === 'sync'" class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.createdUpdated') }}</span>
                <span class="bj-fv">+{{ selected.created }} / ~{{ selected.updated }}</span>
              </div>
              <div v-if="selected.meta?.file_name" class="bj-fact">
                <span class="bj-fl">{{ t('jobs.fact.file') }}</span>
                <span class="bj-fv">{{ selected.meta.file_name }}</span>
              </div>
            </template>
          </div>

          <div v-if="selected.error" class="bj-error">
            <n-icon :component="AlertCircleOutline" :size="16" />
            <span class="bj-err">{{ selected.error }}</span>
          </div>

          <!-- Steps: done ✓ with its time or count, the running one pulsing, the
               rest «—». -->
          <div v-if="selected.steps?.length" class="bj-steps" data-testid="jobs-steps">
            <div
              v-for="s in selected.steps"
              :key="s.key"
              class="bj-step"
              :class="s.status"
              :data-step="s.key"
            >
              <span v-if="s.status === 'running'" class="bj-step-run"><span class="bj-dot" /></span>
              <n-icon v-else :component="stepIcon(s)" :size="16" class="bj-step-ic" />
              <span class="bj-step-name">{{ stepText(s) }}</span>
              <span class="bj-step-val">{{ stepValue(s) }}</span>
            </div>
          </div>

          <!-- The live log (#3092). -->
          <div v-if="!isWorker(selected)" class="bj-log" data-testid="jobs-log">
            <div class="bj-log-head">
              <span class="bj-log-t">{{ t('jobs.log') }}</span>
              <span class="bj-log-sub">{{
                t('jobs.logMeta', {
                  lines: selected.log_lines || 0,
                  errors: selected.log_errors || 0,
                })
              }}</span>
              <span class="bj-grow" />
              <button
                type="button"
                class="bj-follow"
                :class="{ on: follow }"
                role="switch"
                :aria-checked="follow"
                data-testid="jobs-follow"
                @click="follow = !follow"
              >
                <span class="bj-switch"><span class="bj-knob" /></span>
                {{ t('jobs.follow') }}
                <Kbd v-if="!isMobile">{{ cap('f') }}</Kbd>
              </button>
              <button
                type="button"
                class="kit-btn small"
                :disabled="!logLines.length"
                data-testid="jobs-copy"
                @click="copyLog"
              >
                <n-icon :component="CopyOutline" :size="13" />
                {{ t('jobs.copy') }}
                <ShortcutKeys v-if="!isMobile" id="jobs.copyLog" joined />
              </button>
            </div>
            <div ref="logEl" class="bj-log-body" data-testid="jobs-log-body" @scroll="onLogScroll">
              <div v-if="!logLines.length" class="bj-log-empty">{{ t('jobs.logEmpty') }}</div>
              <div
                v-for="l in logLines"
                :key="l.seq"
                class="bj-line"
                :class="l.level"
                data-testid="jobs-log-line"
              >
                <span class="bj-line-t">{{ fmtTime(l.at) }}</span>
                <span class="bj-line-l">{{
                  l.level === 'error' ? 'ERR' : l.level.toUpperCase()
                }}</span>
                <span class="bj-line-x">{{ l.text }}</span>
              </div>
            </div>
            <div
              v-if="selected.persisted && selected.log_lines > logLines.length"
              class="bj-log-note"
            >
              {{ t('jobs.logTail', { n: logLines.length }) }}
            </div>
          </div>
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
/* Progress (#3091): an 8px bar on the accent gradient, the count and the ETA. */
.bj-progress {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.bj-bar {
  display: flex;
  height: 8px;
  border-radius: 4px;
  background: var(--t-pill);
  overflow: hidden;
}
.bj-bar-fill {
  border-radius: 4px;
  background-image: var(--t-accent-grad);
  transition: width 0.3s ease;
}
.bj-progress-line {
  display: flex;
  font-size: 12.5px;
  color: var(--t-text2);
  font-variant-numeric: tabular-nums;
}
.bj-eta {
  color: var(--t-text3);
}
.bj-who {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.bj-ava {
  flex: none;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 8px;
  font-weight: 600;
}
/* Steps (#3091). */
.bj-steps {
  display: flex;
  flex-direction: column;
}
.bj-step {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 26px;
  font-size: 13px;
  color: var(--t-text1);
}
.bj-step.pending {
  color: var(--t-text3);
}
.bj-step-ic {
  flex: none;
  color: var(--t-text3);
}
.bj-step.done .bj-step-ic {
  color: #18a058;
}
.bj-step.failed .bj-step-ic {
  color: var(--t-error);
}
.bj-step-run {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
}
.bj-step-name {
  flex: 1;
  min-width: 0;
}
.bj-step.running .bj-step-name {
  font-weight: 600;
}
.bj-step-val {
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
/* The live log (#3092). */
.bj-log {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 0;
}
.bj-log-head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 12.5px;
  color: var(--t-text2);
}
.bj-log-t {
  font-size: 13.5px;
  font-weight: 650;
  color: var(--t-text1);
}
.bj-log-sub {
  color: var(--t-text3);
}
.bj-follow {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.bj-switch {
  position: relative;
  display: inline-flex;
  align-items: center;
  width: 30px;
  height: 18px;
  padding: 2px;
  box-sizing: border-box;
  border-radius: 9px;
  background: var(--t-pill);
  transition: background 0.15s;
}
.bj-follow.on .bj-switch {
  background-image: var(--t-accent-grad);
}
.bj-knob {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
  transition: transform 0.15s;
}
.bj-follow.on .bj-knob {
  transform: translateX(12px);
}
.bj-log-body {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-height: 96px;
  max-height: 320px;
  overflow-y: auto;
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--t-sub);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
}
.bj-log-empty {
  color: var(--t-text3);
}
.bj-line {
  display: flex;
  gap: 10px;
  white-space: nowrap;
}
.bj-line-t {
  flex: none;
  color: var(--t-text3);
}
.bj-line-l {
  flex: 0 0 36px;
  color: var(--t-text3);
  font-weight: 600;
}
.bj-line-x {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--t-text2);
}
.bj-line.warn .bj-line-l {
  color: #f0a020;
}
.bj-line.error .bj-line-l,
.bj-line.error .bj-line-x {
  color: var(--t-error);
}
.bj-log-note {
  font-size: 12px;
  color: var(--t-text3);
}
.bj-status.cancelled {
  background: var(--t-pill);
  color: var(--t-text2);
}
.bj-status.cancelled .bj-dot {
  animation: none;
  background: currentColor;
}
.bj-op {
  white-space: nowrap;
}
</style>
