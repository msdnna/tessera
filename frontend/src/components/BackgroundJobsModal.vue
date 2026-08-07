<script setup>
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { NModal, NButton, NIcon, NTooltip, useMessage } from 'naive-ui'
import { PlayOutline, StopOutline, ServerOutline, SyncOutline } from '@vicons/ionicons5'
import EmptyState from '@/components/EmptyState.vue'
import { admin as adminApi } from '@/api'
import { runDuration, elapsedSince } from '@/utils/duration'

const props = defineProps({ show: { type: Boolean, default: false } })
const emit = defineEmits(['update:show'])
const message = useMessage()

const jobs = ref([])
const selectedKey = ref(null)
const loading = ref(false)
const refreshing = ref(false)
// A 1s ticker drives the live "processing"/next-run clocks; polling reloads the
// list every 3s while the modal is open.
const now = ref(Date.now())
let pollTimer = null
let tickTimer = null

const selected = computed(() => jobs.value.find((j) => j.key === selectedKey.value) || null)

async function load() {
  loading.value = true
  try {
    const { data } = await adminApi.jobs()
    jobs.value = data || []
    if (!selected.value && jobs.value.length) selectedKey.value = jobs.value[0].key
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

async function runJob(key) {
  try {
    await adminApi.runJob(key)
    message.info('Запущено')
    await load()
  } catch (e) {
    message.error(e.message)
  }
}
async function cancelJob(key) {
  try {
    await adminApi.cancelJob(key)
    message.info('Отменяется…')
    await load()
  } catch (e) {
    message.error(e.response?.data?.error || e.message)
  }
}

const STATUS = {
  running: { label: 'выполняется', color: '#3b9c5a' },
  pending: { label: 'в очереди', color: '#e0922f' },
  done: { label: 'готово', color: 'var(--t-text3)' },
  failed: { label: 'ошибка', color: '#d9534f' },
}
const statusMeta = (s) => STATUS[s] || { label: s, color: 'var(--t-text3)' }
const isWorker = (j) => j.kind === 'worker'
// "worker" stays in English (no good Russian equivalent); a discrete run is a
// «синхронизация».
const kindLabel = (j) => (isWorker(j) ? 'worker' : 'синхронизация')
const MODE = { incremental: 'инкрементальная', full: 'полная' }
const modeLabel = (j) => MODE[j.mode] || j.mode || ''
const TRIGGER = { manual: 'вручную', auto: 'по расписанию' }
const triggerLabel = (j) => TRIGGER[j.trigger] || j.trigger || ''
// "run now" is only for the tick-loop workers; a per-integration sync is started
// from the GitLab modal, so on-demand run here targets workers only.
const canRun = (j) => isWorker(j)
const canCancel = (j) => j.cancelable && j.status === 'running'

// A running job's live elapsed; a finished one's total duration.
function processingText(j) {
  if (!j) return ''
  if (j.status === 'running' && j.started_at) return elapsedSince(j.started_at, now.value)
  return runDuration(j.started_at, j.finished_at)
}
// When a worker (ticker) next fires: last_tick + interval. A ticker spawns the
// actual work, so knowing when it next runs is useful even while it's idle.
function nextRunText(j) {
  if (!isWorker(j) || !j.last_tick_at || !j.interval_sec) return ''
  const next = new Date(j.last_tick_at).getTime() + j.interval_sec * 1000
  const delta = Math.round((next - now.value) / 1000)
  if (delta <= 0) return 'вот-вот'
  return delta >= 60 ? `через ${Math.floor(delta / 60)} мин ${delta % 60} с` : `через ${delta} с`
}
function fmtTime(iso) {
  return iso
    ? new Date(iso).toLocaleTimeString('ru-RU', {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      })
    : '—'
}
const kindIcon = (j) => (isWorker(j) ? ServerOutline : SyncOutline)
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    class="bj-modal"
    :bordered="false"
    :style="{ width: '860px', maxWidth: '94vw' }"
    :auto-focus="false"
    @update:show="emit('update:show', $event)"
  >
    <template #header>
      <div class="bj-head">
        <n-icon :component="ServerOutline" class="grad-icon" />
        <span>Фоновые задания</span>
        <n-tooltip>
          <template #trigger>
            <n-button
              quaternary
              circle
              size="small"
              class="bj-refresh"
              :class="{ spinning: refreshing }"
              @click="refresh"
            >
              <template #icon><n-icon :component="SyncOutline" /></template>
            </n-button>
          </template>
          Обновить
        </n-tooltip>
      </div>
    </template>

    <div class="bj-panes">
      <!-- Left: job list -->
      <div class="bj-list t-hoverscroll">
        <EmptyState v-if="!jobs.length" :icon="ServerOutline" text="Нет фоновых заданий" />
        <button
          v-for="j in jobs"
          :key="j.key"
          class="bj-row"
          :class="{ active: j.key === selectedKey }"
          @click="selectedKey = j.key"
        >
          <n-icon :component="kindIcon(j)" class="bj-row-icon" />
          <div class="bj-row-main">
            <div class="bj-row-name">{{ j.name }}</div>
            <div class="bj-row-sub">
              <span class="bj-dot" :style="{ background: statusMeta(j.status).color }" />
              {{ statusMeta(j.status).label }}
              <span v-if="isWorker(j) && nextRunText(j)" class="bj-op"
                >· след. запуск {{ nextRunText(j) }}</span
              >
              <span v-else-if="j.current_op" class="bj-op">· {{ j.current_op }}</span>
            </div>
          </div>
          <span v-if="j.status === 'running'" class="bj-elapsed">{{ processingText(j) }}</span>
          <span v-else-if="j.finished_at" class="bj-elapsed">{{ fmtTime(j.finished_at) }}</span>
        </button>
      </div>

      <!-- Right: details -->
      <div class="bj-detail">
        <EmptyState v-if="!selected" :icon="ServerOutline" text="Выберите задание" />
        <template v-else>
          <div class="bj-detail-head">
            <span class="bj-dot" :style="{ background: statusMeta(selected.status).color }" />
            <span class="bj-detail-name">{{ selected.name }}</span>
            <span class="bj-kind">{{ kindLabel(selected) }}</span>
            <span v-if="selected.persisted" class="bj-kind bj-journal">журнал</span>
          </div>

          <dl class="bj-facts">
            <dt>Статус</dt>
            <dd>{{ statusMeta(selected.status).label }}</dd>
            <template v-if="isWorker(selected)">
              <template v-if="selected.current_op">
                <dt>Назначение</dt>
                <dd>{{ selected.current_op }}</dd>
              </template>
              <dt>Последняя активность</dt>
              <dd>{{ fmtTime(selected.last_tick_at) }}</dd>
              <template v-if="nextRunText(selected)">
                <dt>Следующий запуск</dt>
                <dd>{{ nextRunText(selected) }}</dd>
              </template>
            </template>
            <template v-else>
              <template v-if="modeLabel(selected)">
                <dt>Режим</dt>
                <dd>{{ modeLabel(selected) }}</dd>
              </template>
              <template v-if="triggerLabel(selected)">
                <dt>Запуск</dt>
                <dd>{{ triggerLabel(selected) }}</dd>
              </template>
              <dt>Начато</dt>
              <dd>{{ fmtTime(selected.started_at) }}</dd>
              <dt>Завершено</dt>
              <dd>{{ selected.finished_at ? fmtTime(selected.finished_at) : '—' }}</dd>
              <dt>Длительность</dt>
              <dd>{{ processingText(selected) || '—' }}</dd>
              <dt>Создано / обновлено</dt>
              <dd>+{{ selected.created }} / ~{{ selected.updated }}</dd>
            </template>
            <template v-if="selected.error">
              <dt>Ошибка</dt>
              <dd class="bj-err">{{ selected.error }}</dd>
            </template>
          </dl>

          <div class="bj-actions">
            <n-button v-if="canRun(selected)" size="small" secondary @click="runJob(selected.key)">
              <template #icon><n-icon :component="PlayOutline" /></template>
              Запустить сейчас
            </n-button>
            <n-button
              v-if="canCancel(selected)"
              size="small"
              type="error"
              secondary
              @click="cancelJob(selected.key)"
            >
              <template #icon><n-icon :component="StopOutline" /></template>
              Остановить
            </n-button>
          </div>
        </template>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.bj-head {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}
.bj-panes {
  display: flex;
  gap: 14px;
  min-height: 340px;
}
.bj-list {
  flex: 0 0 320px;
  max-height: 62vh;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.bj-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  border-radius: 10px;
  border: 1px solid transparent;
  background: transparent;
  cursor: pointer;
  text-align: left;
  color: inherit;
  font: inherit;
}
.bj-row:hover {
  background: var(--t-fill1);
}
.bj-row.active {
  border-color: color-mix(in srgb, var(--t-primary) 40%, transparent);
  background: color-mix(in srgb, var(--t-primary) 8%, transparent);
}
.bj-row-icon {
  flex: 0 0 auto;
  color: var(--t-text3);
  font-size: 18px;
}
.bj-row-main {
  flex: 1 1 auto;
  min-width: 0;
}
.bj-row-name {
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bj-row-sub {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bj-op {
  overflow: hidden;
  text-overflow: ellipsis;
}
.bj-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: 0 0 auto;
}
.bj-elapsed {
  flex: 0 0 auto;
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.bj-refresh {
  color: var(--t-text3);
}
.bj-refresh.spinning :deep(.n-icon) {
  animation: bj-spin 0.8s linear infinite;
}
@keyframes bj-spin {
  to {
    transform: rotate(360deg);
  }
}
.bj-detail {
  flex: 1 1 auto;
  border-left: 1px solid var(--t-border);
  padding-left: 14px;
  min-width: 0;
}
.bj-detail-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.bj-detail-name {
  font-weight: 600;
  font-size: 15px;
}
.bj-kind {
  font-size: 11px;
  padding: 1px 7px;
  border-radius: 8px;
  background: var(--t-fill1);
  color: var(--t-text3);
}
.bj-journal {
  background: color-mix(in srgb, var(--t-primary) 12%, transparent);
  color: var(--t-primary);
}
.bj-facts {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 6px 14px;
  margin: 0 0 16px;
  font-size: 13px;
}
.bj-facts dt {
  color: var(--t-text3);
}
.bj-facts dd {
  margin: 0;
  word-break: break-word;
}
.bj-err {
  color: #d9534f;
}
.bj-actions {
  display: flex;
  gap: 8px;
}
</style>
