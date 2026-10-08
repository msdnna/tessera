<script setup>
// The stage page (#2993, mockup StagePage / MobileStage): a screen in the
// content area at /project/<project>/stage/<stage>. Left: the goals (the stage's
// Markdown description — what GitLab calls the milestone description), the
// burn-up and the stage's tasks by status. Right: progress by count and by
// estimate, the signals, the split by status, the scope change after the
// start, the people and the pace against the three previous stages. One read:
// GET /milestones/:id/metrics (#2992).
//
// Keys: E — edit the goals, B — the stage's board, ⇧X — close the stage (two
// steps), Esc — back. A GitLab stage is read-only until writing back to GitLab
// exists (#3010): the goals come from GitLab, editing and closing are stubs.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CAPS } from '@/utils/keyCaps'
import { useRoute, useRouter } from 'vue-router'
import { NIcon, NTooltip, useMessage } from 'naive-ui'
import {
  TrailSignOutline,
  CalendarClearOutline,
  GitBranchOutline,
  OpenOutline,
  CheckmarkDoneOutline,
  EllipsisHorizontalOutline,
  CreateOutline,
  ChevronDownOutline,
  ChevronForwardOutline,
  RefreshOutline,
  TrashOutline,
  PersonOutline,
  AlertCircleOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import { projects as projApi, milestones as msApi, workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useProjectWindowStore } from '@/stores/projectWindow'
import { useShortcuts } from '@/composables/useShortcuts'
import { useResponsive } from '@/composables/useResponsive'
import { useFormat } from '@/composables/useFormat'
import { humanizeError } from '@/utils/errors'
import { stubTask } from '@/utils/featureStubs'
import { formatHours } from '@/utils/estimation'
import { buildMentionItems } from '@/utils/mentions'
import {
  milestoneRangeShort,
  milestoneKey,
  stageState,
  daysLeft,
  todayUTC,
  stageDay,
} from '@/utils/milestones'
import { percent, paceAverage, forecast, dayMs } from '@/utils/milestoneMetrics'
import { columnStatusName, isReviewColumn } from '@/utils/columnStatus'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import RichContent from '@/components/RichContent.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import TesseraIcon from '@/components/TesseraIcon.vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import Kbd from '@/components/Kbd.vue'
import EmptyState from '@/components/EmptyState.vue'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import BurnChart from '@/components/milestone/BurnChart.vue'
import MetricRing from '@/components/milestone/MetricRing.vue'
import StageDatesPopover from '@/components/project/StageDatesPopover.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const message = useMessage()
const ws = useWorkspacesStore()
const projectWindow = useProjectWindowStore()
const { isMobile } = useResponsive()
const { formatters, formatDateTime } = useFormat()

const project = computed(() => ws.projects.find((p) => p.slug === route.params.projectSlug) || null)
const stage = ref(null)
const metrics = ref(null)
const loading = ref(true)
const notFound = ref(false)
const failed = ref('')
const members = ref([])

async function load({ quiet = false } = {}) {
  const p = project.value
  if (!p) return
  if (!quiet) loading.value = true
  failed.value = ''
  try {
    const { data } = await projApi.milestones(p.id)
    const key = String(route.params.stageSlug || '')
    const m = (data || []).find((x) => x.slug === key || x.id === key)
    if (!m) {
      notFound.value = true
      stage.value = null
      return
    }
    notFound.value = false
    stage.value = m
    const res = await msApi.metrics(m.id)
    metrics.value = res.data
    // «Добавить цели →» on the board's strip lands here in the editor.
    if (route.query.edit === '1') {
      const q = { ...route.query }
      delete q.edit
      router.replace({ query: q })
      if (!m.gl_global_id) nextTick(startEdit)
    }
  } catch (e) {
    failed.value = humanizeError(e)
  } finally {
    loading.value = false
  }
}
watch(
  () => [project.value?.id, route.params.stageSlug],
  ([pid]) => {
    if (pid) load()
  },
  { immediate: true },
)
watch(
  () => project.value && ws.milestoneRevFor(project.value.id),
  () => {
    if (stage.value && !editing.value) load({ quiet: true })
  },
)
onMounted(async () => {
  topbar.value = !!document.getElementById('tb-slot-left')
  if (!ws.currentId) return
  try {
    members.value = (await wsApi.members(ws.currentId)).data || []
  } catch {
    members.value = []
  }
})
const topbar = ref(false)

const head = computed(() => metrics.value?.milestone || stage.value || {})
const linked = computed(() => !!(stage.value?.gl_global_id || head.value.gl_url))
const gitlabStub = stubTask('stage.gitlabWrite')
const today = computed(() => todayUTC())
const state = computed(() => stageState(stage.value, today.value))
const left = computed(() => daysLeft(stage.value, today.value))
const range = computed(() => milestoneRangeShort(stage.value, formatters.value))
const leftLabel = computed(() => {
  if (state.value !== 'active' || left.value == null) return ''
  if (left.value < 0) return t('milestones.stage.overdueDays', -left.value)
  return t('milestones.stage.daysLeftLong', left.value)
})
const estCfg = computed(() => (project.value ? ws.estimationFor(project.value.id) : null))
const fmtEst = (v) => formatHours(v || 0, estCfg.value)

// ── figures ──
const totals = computed(() => metrics.value?.totals || { tasks: 0, done: 0, open: 0 })
const countPct = computed(() => percent(totals.value.done, totals.value.tasks))
const estPct = computed(() => percent(totals.value.done_estimate, totals.value.estimate))
const signals = computed(() => metrics.value?.signals || {})
const statuses = computed(() => metrics.value?.statuses || [])
const scope = computed(() => metrics.value?.scope || {})
const people = computed(() => metrics.value?.people || [])
const pace = computed(() => metrics.value?.pace || [])
const paceAvg = computed(() => paceAverage(pace.value))
const paceMax = computed(() => Math.max(1, ...pace.value.map((p) => Math.max(p.done, p.total))))
const series = computed(() => metrics.value?.series || [])
const chartMode = ref('tasks')
const chartKind = ref('up')
const fc = computed(() => forecast(series.value, metrics.value?.end || '', chartMode.value))
const started = computed(() => series.value.length > 0)
const lastPoint = computed(() => series.value[series.value.length - 1] || null)
const memberName = (uid) => members.value.find((m) => m.user_id === uid)?.name || ''

// ── task list by status, filterable by a signal ──
const signal = ref('')
function toggleSignal(key) {
  signal.value = signal.value === key ? '' : key
}
const openGroups = ref(new Set())
const groupedTasks = computed(() => {
  const tasks = (metrics.value?.tasks || []).filter((tk) => {
    if (!signal.value) return true
    if (tk.completed_at) return false
    if (signal.value === 'overdue') return tk.overdue
    if (signal.value === 'no_estimate') return tk.estimate == null
    if (signal.value === 'no_assignee') return !tk.assignees?.length
    return true
  })
  return (
    statuses.value
      .map((s, i) => ({
        ...s,
        index: i,
        key: (s.name || '').trim().toLowerCase(),
        tasks: tasks.filter(
          (tk) =>
            (tk.column_name || '').trim().toLowerCase() === (s.name || '').trim().toLowerCase(),
        ),
      }))
      .filter((g) => g.tasks.length)
      // Unfinished work first; what is done goes last, folded.
      .sort((a, b) => (a.done === a.count) - (b.done === b.count) || b.index - a.index)
  )
})
watch(
  () => metrics.value,
  () => {
    const open = new Set()
    for (const g of groupedTasks.value) if (g.done < g.count) open.add(g.key)
    openGroups.value = open
  },
)
function toggleGroup(key) {
  const next = new Set(openGroups.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  openGroups.value = next
}
function statusIcon(g) {
  return columnStatusName({
    isDone: g.count > 0 && g.done === g.count,
    first: g.index === 0,
    name: g.name,
    nameKey: isReviewColumn(g.name) ? 'review' : '',
  })
}
function openTask(tk) {
  router.push(`/board/${tk.board_id}?task=${tk.number ?? tk.id}`)
}
const dueLabel = (tk) =>
  tk.due_date ? formatters.value.formatDate(tk.due_date, { day: '2-digit', month: 'short' }) : ''

// ── actions ──
// The stage's board: the board most of its tasks live on, else the project's
// first; scoped to the stage.
async function openBoard() {
  const p = project.value
  if (!p || !stage.value) return
  const count = new Map()
  for (const tk of metrics.value?.tasks || [])
    count.set(tk.board_slug, (count.get(tk.board_slug) || 0) + 1)
  let slug = [...count.entries()].sort((a, b) => b[1] - a[1])[0]?.[0]
  if (!slug) {
    if (!ws.boardsByProject[p.id]) await ws.loadBoards(p.id)
    slug = ws.boardsByProject[p.id]?.[0]?.slug
  }
  if (!slug) {
    message.warning(t('project.milestone.noBoard'))
    return
  }
  router.push({
    path: `/project/${p.slug}/board/${slug}`,
    query: { milestone: milestoneKey(stage.value) },
  })
}

async function patchStage(patch) {
  const m = stage.value
  const { data } = await msApi.update(m.id, {
    title: m.title,
    description: m.description || '',
    start_date: m.start_date || null,
    due_date: m.due_date || null,
    state: m.state,
    ...patch,
  })
  return data
}

// Close: two steps — the button arms («Точно закрыть?»), the second press closes.
const closeArmed = ref(false)
let disarm = 0
async function onClose() {
  if (!stage.value || linked.value) return
  if (stage.value.state === 'closed') {
    await setState('active')
    return
  }
  if (!closeArmed.value) {
    closeArmed.value = true
    clearTimeout(disarm)
    disarm = setTimeout(() => (closeArmed.value = false), 4000)
    return
  }
  clearTimeout(disarm)
  closeArmed.value = false
  await setState('closed')
}
async function setState(next) {
  try {
    await patchStage({ state: next })
    await load({ quiet: true })
  } catch (e) {
    message.error(humanizeError(e))
  }
}

// ── goals: Markdown, edited in place ──
const editing = ref(false)
const draft = ref('')
const saving = ref(false)
const goalsEl = ref(null)
const mentionItems = computed(() => buildMentionItems(members.value, [], {}))
function startEdit() {
  if (!stage.value || linked.value) return
  draft.value = stage.value.description || ''
  editing.value = true
  nextTick(() => goalsEl.value?.querySelector('textarea')?.focus())
}
async function saveGoals() {
  if (saving.value) return
  saving.value = true
  try {
    await patchStage({ description: draft.value })
    editing.value = false
    await load({ quiet: true })
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    saving.value = false
  }
}
const cancelArmed = ref(false)
function cancelEdit() {
  if (draft.value !== (stage.value?.description || '') && !cancelArmed.value) {
    cancelArmed.value = true
    setTimeout(() => (cancelArmed.value = false), 3000)
    return
  }
  cancelArmed.value = false
  editing.value = false
}
function onGoalsKey(e) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
    e.preventDefault()
    saveGoals()
  } else if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    cancelEdit()
  }
}

// ── «⋯» ──
const menuShow = ref(false)
const menuBtn = ref(null)
const deleting = ref(false)
const menuItems = computed(() => {
  const gl = linked.value
  const items = [
    {
      key: 'dates',
      label: t('milestones.stage.datesMenu'),
      icon: CalendarClearOutline,
      ...(gl ? { stub: gitlabStub } : {}),
    },
    { key: 'window', label: t('milestones.stage.inWindow'), icon: OpenOutline },
  ]
  if (gl && head.value.gl_url) {
    items.push({
      key: 'gitlab',
      label: t('milestones.manager.openInGitlab'),
      icon: GitBranchOutline,
    })
  }
  items.push({ gap: true })
  items.push({
    key: 'delete',
    label: t('milestones.stage.delete'),
    icon: TrashOutline,
    danger: true,
    confirm: {
      title: t('milestones.stage.deleteArmed', { name: stage.value?.title || '' }),
      sub: gl ? t('milestones.manager.deleteLinked') : t('milestones.stage.deleteSub'),
    },
    busy: deleting.value,
  })
  return items
})
const datesShow = ref(false)
async function onMenu(key) {
  if (key === 'dates') datesShow.value = true
  else if (key === 'window') projectWindow.open(project.value?.id, 'stages')
  else if (key === 'gitlab') window.open(head.value.gl_url, '_blank', 'noopener')
  else if (key === 'delete') {
    deleting.value = true
    try {
      await msApi.remove(stage.value.id)
      menuShow.value = false
      router.push('/')
    } catch (e) {
      message.error(humanizeError(e))
    } finally {
      deleting.value = false
    }
  }
}

function back() {
  if (window.history.length > 1) router.back()
  else router.push('/')
}
const pageLive = () => !!stage.value && !editing.value
useShortcuts(
  {
    'stage.edit': () => startEdit(),
    'stage.board': () => openBoard(),
    'stage.close': () => onClose(),
    'stage.back': () => back(),
  },
  { when: pageLive },
)

const syncedLabel = computed(() =>
  head.value.gl_synced_at
    ? formatDateTime(head.value.gl_synced_at, { dateStyle: 'short', timeStyle: 'short' })
    : '',
)
const notStartedLabel = computed(() => {
  const s = stageDay(stage.value?.start_date)
  if (s == null) return ''
  return formatters.value.formatDate(new Date(s).toISOString(), { day: 'numeric', month: 'short' })
})
const forecastLabel = computed(() => {
  if (!fc.value || !metrics.value?.end) return ''
  const end = formatters.value.formatDate(new Date(dayMs(metrics.value.end)).toISOString(), {
    day: 'numeric',
    month: 'short',
  })
  const v = chartMode.value === 'estimate' ? fmtEst(fc.value.value) : Math.round(fc.value.value)
  return t('milestones.page.forecastBy', { value: v, date: end })
})
</script>

<template>
  <div class="sp" :class="{ mobile: isMobile }" data-testid="stage-page">
    <teleport v-if="project && !isMobile" to="#tb-slot-left" :disabled="!topbar">
      <div class="crumbs" data-testid="stage-crumbs">
        <span class="crumb">{{ project.name }}</span>
        <n-icon :component="ChevronForwardOutline" :size="12" class="crumb-sep" />
        <button type="button" class="crumb link" @click="projectWindow.open(project.id, 'stages')">
          {{ t('milestones.title') }}
        </button>
        <n-icon :component="ChevronForwardOutline" :size="12" class="crumb-sep" />
        <span class="crumb cur">{{ stage?.title || '' }}</span>
      </div>
    </teleport>

    <div v-if="loading && !stage" class="sp-loading"><TesseraSpinner /></div>
    <EmptyState
      v-else-if="notFound || !project"
      :icon="TrailSignOutline"
      :text="t('milestones.page.notFound')"
    />
    <EmptyState v-else-if="failed && !metrics" :icon="AlertCircleOutline" :text="failed" />

    <template v-else-if="stage">
      <!-- ── head ── -->
      <header v-if="isMobile" class="sp-mhead">
        <!-- «‹ Этапы» is the shell's header on a phone (#2976); the page names
             the stage under it, large, with its badge. -->
        <span class="sp-badge sp-mbadge"><n-icon :component="TrailSignOutline" :size="22" /></span>
        <div class="sp-mtitle">
          <div class="sp-mproj">{{ project.name }} · {{ t('milestones.page.stageWord') }}</div>
          <div class="sp-mname">{{ stage.title }}</div>
        </div>
        <button ref="menuBtn" type="button" class="sp-iconbtn" @click="menuShow = true">
          <n-icon :component="EllipsisHorizontalOutline" :size="18" />
        </button>
      </header>
      <header v-else class="sp-head">
        <span class="sp-badge"><n-icon :component="TrailSignOutline" :size="22" /></span>
        <div class="sp-head-main">
          <div class="sp-title-row">
            <h1 class="sp-title">{{ stage.title }}</h1>
            <span class="sp-state" :class="state" data-testid="stage-state">
              <span v-if="state === 'active'" class="sp-dot" />{{ t(`milestones.state.${state}`) }}
            </span>
          </div>
          <div class="sp-meta">
            <span v-if="range" class="sp-meta-i">
              <n-icon :component="CalendarClearOutline" :size="14" />{{ range }}
            </span>
            <span v-if="leftLabel" class="sp-meta-i strong">{{ leftLabel }}</span>
            <a
              v-if="head.gl_url"
              class="sp-meta-i link"
              :href="head.gl_url"
              target="_blank"
              rel="noopener noreferrer"
            >
              <n-icon :component="GitBranchOutline" :size="13" />
              {{ t('milestones.page.inGitlab', { iid: head.gl_iid ?? '' }) }}
              <n-icon :component="OpenOutline" :size="12" />
            </a>
            <span class="sp-meta-i">{{ project.name }}</span>
          </div>
        </div>
        <div class="sp-actions">
          <button type="button" class="sp-btn" data-testid="stage-open-board" @click="openBoard">
            <TesseraIcon name="layout-kanban" :size="15" />
            {{ t('milestones.page.openBoard') }}
            <ShortcutKeys id="stage.board" />
          </button>
          <n-tooltip :disabled="!linked">
            <template #trigger>
              <button
                type="button"
                class="sp-btn"
                :class="{ armed: closeArmed }"
                :disabled="linked"
                data-testid="stage-close"
                @click="onClose"
              >
                <n-icon
                  :component="stage.state === 'closed' ? RefreshOutline : CheckmarkDoneOutline"
                  :size="15"
                />
                {{
                  stage.state === 'closed'
                    ? t('milestones.stage.reopen')
                    : closeArmed
                      ? t('milestones.page.closeArmed')
                      : t('milestones.page.close')
                }}
                <ShortcutKeys v-if="stage.state !== 'closed'" id="stage.close" />
              </button>
            </template>
            {{ t('common.stub.soon', { n: gitlabStub }) }}
          </n-tooltip>
          <button
            ref="menuBtn"
            type="button"
            class="sp-iconbtn"
            data-testid="stage-more"
            @click="menuShow = true"
          >
            <n-icon :component="EllipsisHorizontalOutline" :size="18" />
          </button>
        </div>
      </header>

      <div v-if="isMobile" class="sp-mchips">
        <span class="sp-state" :class="state">
          <span v-if="state === 'active'" class="sp-dot" />{{ t(`milestones.state.${state}`) }}
        </span>
        <span v-if="range" class="sp-mrange">{{ range }}</span>
        <span v-if="leftLabel" class="sp-mleft">{{ leftLabel }}</span>
      </div>

      <div class="sp-grid">
        <!-- ── left column ── -->
        <div class="sp-col">
          <!-- goals -->
          <section
            ref="goalsEl"
            class="sp-card"
            :class="{ editing, 'm-order-4': isMobile }"
            data-testid="stage-goals"
            @keydown="editing && onGoalsKey($event)"
          >
            <div class="sp-card-h">
              <span class="sp-card-t">{{ t('milestones.page.goals') }}</span>
              <span v-if="editing" class="sp-card-s">{{ t('milestones.page.goalsMarkdown') }}</span>
              <span v-else-if="linked" class="sp-card-s">
                <n-icon :component="GitBranchOutline" :size="12" />
                {{ t('milestones.page.syncedFrom', { iid: head.gl_iid ?? '' }) }}
                <template v-if="syncedLabel">· {{ syncedLabel }}</template>
              </span>
              <span class="sp-grow" />
              <n-tooltip v-if="!editing" :disabled="!linked">
                <template #trigger>
                  <button
                    type="button"
                    class="sp-btn small"
                    :disabled="linked"
                    data-testid="stage-goals-edit"
                    @click="startEdit"
                  >
                    <n-icon :component="CreateOutline" :size="14" />
                    {{ t('milestones.page.edit') }}
                    <ShortcutKeys v-if="!isMobile" id="stage.edit" />
                  </button>
                </template>
                {{ t('common.stub.soon', { n: gitlabStub }) }}
              </n-tooltip>
            </div>
            <template v-if="editing">
              <MarkdownEditor
                v-model="draft"
                :mention-items="mentionItems"
                :min-rows="5"
                :expandable="false"
              />
              <div class="sp-edit-foot">
                <span class="sp-edit-hint"
                  ><Kbd>{{ '@' }}</Kbd
                  >{{ t('milestones.page.hintPerson') }}</span
                >
                <span class="sp-grow" />
                <span v-if="cancelArmed" class="sp-edit-warn">{{
                  t('milestones.page.discard')
                }}</span>
                <button type="button" class="sp-btn small ghost" @click="cancelEdit">
                  {{ t('common.action.cancel') }} <Kbd>{{ CAPS.escape }}</Kbd>
                </button>
                <button
                  type="button"
                  class="sp-btn small primary"
                  :disabled="saving"
                  data-testid="stage-goals-save"
                  @click="saveGoals"
                >
                  {{ t('common.action.save') }} <Kbd>{{ CAPS.modEnter }}</Kbd>
                </button>
              </div>
            </template>
            <!-- A wrapper, not a class on RichContent: it renders a fragment, and
                 the heading sizes below need this scope on an element. -->
            <div v-else-if="stage.description" class="sp-goals">
              <RichContent :source="stage.description" :members="members" />
            </div>
            <button v-else-if="!linked" type="button" class="sp-goals-empty" @click="startEdit">
              {{ t('milestones.page.goalsEmpty') }}
            </button>
            <div v-else class="sp-goals-empty static">
              {{ t('milestones.page.goalsEmptyGitlab') }}
            </div>
          </section>

          <!-- burn-up -->
          <section class="sp-card" :class="{ 'm-order-3': isMobile }" data-testid="stage-burn">
            <div class="sp-card-h">
              <span class="sp-card-t">{{
                chartKind === 'up' ? t('milestones.page.burnup') : t('milestones.page.burndown')
              }}</span>
              <span class="sp-card-s">{{
                chartMode === 'tasks'
                  ? t('milestones.page.byTasks')
                  : t('milestones.page.byEstimate')
              }}</span>
              <span class="sp-grow" />
              <template v-if="!isMobile">
                <div class="sp-seg">
                  <button
                    v-for="m in ['tasks', 'estimate']"
                    :key="m"
                    type="button"
                    :class="{ on: chartMode === m }"
                    @click="chartMode = m"
                  >
                    {{ t(`milestones.page.mode.${m}`) }}
                  </button>
                </div>
                <div class="sp-seg">
                  <button
                    v-for="k in ['up', 'down']"
                    :key="k"
                    type="button"
                    :class="{ on: chartKind === k }"
                    @click="chartKind = k"
                  >
                    {{ t(`milestones.page.kind.${k}`) }}
                  </button>
                </div>
              </template>
              <span v-else-if="fc" class="sp-card-s">{{ forecastLabel }}</span>
            </div>
            <div v-if="!started" class="sp-chart-empty">
              <n-icon :component="TimeOutline" :size="18" />
              {{ t('milestones.page.notStarted', { date: notStartedLabel }) }}
            </div>
            <template v-else>
              <BurnChart
                :series="series"
                :start="metrics.start"
                :end="metrics.end"
                :today="metrics.today"
                :mode="chartMode"
                :kind="chartKind"
                :height="isMobile ? 130 : 210"
                :compact="isMobile"
              />
              <div v-if="!isMobile" class="sp-legend">
                <span class="lg done"
                  >{{ t('milestones.page.legendDone') }} ·
                  {{
                    chartMode === 'estimate' ? fmtEst(lastPoint?.done_est) : lastPoint?.done
                  }}</span
                >
                <span class="lg scope"
                  >{{ t('milestones.page.legendScope') }} ·
                  {{
                    chartMode === 'estimate' ? fmtEst(lastPoint?.scope_est) : lastPoint?.scope
                  }}</span
                >
                <span v-if="fc && chartKind === 'up'" class="lg fc">{{ forecastLabel }}</span>
              </div>
            </template>
          </section>

          <!-- tasks -->
          <section v-if="!isMobile" class="sp-card" data-testid="stage-tasks">
            <div class="sp-card-h">
              <span class="sp-card-t">{{ t('milestones.page.tasks') }}</span>
              <span v-if="signal" class="sp-card-s"
                >· {{ t(`milestones.page.signal.${signal}`) }}</span
              >
              <span class="sp-grow" />
              <button type="button" class="sp-link" @click="openBoard">
                {{ t('milestones.page.openBoardArrow') }}
              </button>
            </div>
            <div v-if="!groupedTasks.length" class="sp-none">
              {{ t('milestones.page.noTasks') }}
            </div>
            <template v-for="g in groupedTasks" :key="g.key">
              <button type="button" class="sp-tgroup" @click="toggleGroup(g.key)">
                <n-icon
                  :component="openGroups.has(g.key) ? ChevronDownOutline : ChevronForwardOutline"
                  :size="12"
                />
                {{ g.name }} <span class="sp-tcount">{{ g.tasks.length }}</span>
              </button>
              <template v-if="openGroups.has(g.key)">
                <button
                  v-for="tk in g.tasks"
                  :key="tk.id"
                  type="button"
                  class="sp-task"
                  data-testid="stage-task"
                  @click="openTask(tk)"
                >
                  <TesseraIcon
                    :name="statusIcon(g)"
                    :size="14"
                    class="sp-tstat"
                    :style="{ color: g.color || 'var(--t-text3)' }"
                  />
                  <span class="sp-tnum">#{{ tk.number }}</span>
                  <span class="sp-ttitle">{{ tk.title }}</span>
                  <span v-if="tk.overdue" class="sp-tdue">{{ dueLabel(tk) }}</span>
                  <span class="sp-test">{{ tk.estimate != null ? fmtEst(tk.estimate) : '—' }}</span>
                  <span class="sp-tav">
                    <UserAvatar
                      v-for="uid in tk.assignees.slice(0, 2)"
                      :key="uid"
                      :user-id="uid"
                      :name="memberName(uid)"
                      class="sp-av"
                    />
                    <n-icon v-if="!tk.assignees.length" :component="PersonOutline" :size="14" />
                  </span>
                </button>
              </template>
            </template>
          </section>
        </div>

        <!-- ── right column ── -->
        <div class="sp-col side">
          <section class="sp-card" :class="{ 'm-order-1': isMobile }" data-testid="stage-progress">
            <div v-if="!isMobile" class="sp-card-h">
              <span class="sp-card-t">{{ t('milestones.page.progress') }}</span>
            </div>
            <div class="sp-rings">
              <div class="sp-ring">
                <MetricRing :pct="countPct" />
                <div class="sp-ring-txt">
                  <div class="sp-ring-l">{{ t('milestones.page.ringTasks') }}</div>
                  <div class="sp-ring-v">
                    {{ t('milestones.page.ofTotal', { done: totals.done, total: totals.tasks }) }}
                  </div>
                  <div class="sp-ring-s">{{ t('milestones.page.openCount', totals.open) }}</div>
                </div>
              </div>
              <div class="sp-ring">
                <MetricRing :pct="estPct" />
                <div class="sp-ring-txt">
                  <div class="sp-ring-l">{{ t('milestones.page.ringEstimate') }}</div>
                  <div class="sp-ring-v">
                    {{ fmtEst(totals.done_estimate) }} / {{ fmtEst(totals.estimate) }}
                  </div>
                  <div class="sp-ring-s">
                    {{
                      t('milestones.page.estLeft', {
                        value: fmtEst(Math.max(0, totals.estimate - totals.done_estimate)),
                      })
                    }}
                  </div>
                </div>
              </div>
            </div>
          </section>

          <div class="sp-signals" :class="{ 'm-order-2': isMobile }" data-testid="stage-signals">
            <button
              type="button"
              class="sp-signal red"
              :class="{ on: signal === 'overdue' }"
              @click="toggleSignal('overdue')"
            >
              <b>{{ signals.overdue || 0 }}</b> {{ t('milestones.page.signal.overdue') }}
            </button>
            <button
              type="button"
              class="sp-signal orange"
              :class="{ on: signal === 'no_estimate' }"
              @click="toggleSignal('no_estimate')"
            >
              <b>{{ signals.no_estimate || 0 }}</b> {{ t('milestones.page.signal.no_estimate') }}
            </button>
            <button
              type="button"
              class="sp-signal"
              :class="{ on: signal === 'no_assignee' }"
              @click="toggleSignal('no_assignee')"
            >
              <b>{{ signals.no_assignee || 0 }}</b> {{ t('milestones.page.signal.no_assignee') }}
            </button>
          </div>

          <section class="sp-card" :class="{ 'm-order-5': isMobile }" data-testid="stage-statuses">
            <div class="sp-card-h">
              <span class="sp-card-t">{{ t('milestones.page.byStatus') }}</span>
              <span class="sp-card-s">{{ t('milestones.page.tasksCount', totals.tasks) }}</span>
            </div>
            <div class="sp-stack">
              <span
                v-for="s in statuses"
                :key="s.name"
                class="sp-stack-seg"
                :style="{
                  flexGrow: s.count,
                  background: `linear-gradient(to top right, color-mix(in srgb, ${s.color || 'var(--t-text3)'} 82%, #000), ${s.color || 'var(--t-text3)'}, color-mix(in srgb, ${s.color || 'var(--t-text3)'} 82%, #fff))`,
                }"
                :title="`${s.name} · ${s.count}`"
              />
            </div>
            <div class="sp-stat-list">
              <span v-for="s in statuses" :key="s.name" class="sp-stat">
                <span class="sp-stat-dot" :style="{ background: s.color || 'var(--t-text3)' }" />
                <span class="sp-stat-n">{{ s.name }}</span>
                <b>{{ s.count }}</b>
              </span>
            </div>
          </section>

          <template v-if="!isMobile">
            <section class="sp-card" data-testid="stage-scope">
              <div class="sp-card-h">
                <span class="sp-card-t">{{ t('milestones.page.scope') }}</span>
                <span v-if="notStartedLabel" class="sp-card-s">{{
                  t('milestones.page.afterStart', { date: notStartedLabel })
                }}</span>
              </div>
              <div class="sp-tiles">
                <div class="sp-tile">
                  <div class="sp-tile-v">+{{ scope.added || 0 }}</div>
                  <div class="sp-tile-l">{{ t('milestones.page.added') }}</div>
                </div>
                <div class="sp-tile">
                  <div class="sp-tile-v">−{{ scope.removed || 0 }}</div>
                  <div class="sp-tile-l">{{ t('milestones.page.removed') }}</div>
                </div>
                <div class="sp-tile">
                  <div class="sp-tile-v">{{ scope.at_start || 0 }} → {{ scope.now || 0 }}</div>
                  <div class="sp-tile-l">{{ t('milestones.page.inStage') }}</div>
                </div>
              </div>
            </section>

            <section class="sp-card" data-testid="stage-people">
              <div class="sp-card-h">
                <span class="sp-card-t">{{ t('milestones.page.byPeople') }}</span>
              </div>
              <div v-if="!people.length" class="sp-none">{{ t('milestones.page.noTasks') }}</div>
              <div v-for="p in people" :key="p.user_id || 'none'" class="sp-person">
                <UserAvatar v-if="p.user_id" :user-id="p.user_id" :name="p.name" class="sp-av" />
                <span v-else class="sp-av none"
                  ><n-icon :component="PersonOutline" :size="13"
                /></span>
                <span class="sp-pname">{{
                  p.user_id ? p.name : t('milestones.page.unassigned')
                }}</span>
                <span class="sp-pbar">
                  <span class="sp-pfill" :style="{ width: `${percent(p.done, p.total)}%` }" />
                </span>
                <span class="sp-pn">{{ p.done }}/{{ p.total }}</span>
                <span class="sp-pest">{{ p.estimate ? fmtEst(p.estimate) : '' }}</span>
              </div>
            </section>

            <section class="sp-card" data-testid="stage-pace">
              <div class="sp-card-h">
                <span class="sp-card-t">{{ t('milestones.page.pace') }}</span>
                <span class="sp-card-s">{{ t('milestones.page.paceSub') }}</span>
              </div>
              <div class="sp-pace">
                <div v-for="p in pace" :key="p.id" class="sp-pace-col" :class="{ cur: p.current }">
                  <span class="sp-pace-v">
                    <template v-if="p.current && fc"
                      >{{ p.done }} → {{ Math.round(fc.value) }}</template
                    >
                    <template v-else>{{ p.done }}</template>
                  </span>
                  <span class="sp-pace-bar">
                    <span
                      v-if="p.current && fc"
                      class="sp-pace-ghost"
                      :style="{ height: `${(Math.round(fc.value) / paceMax) * 100}%` }"
                    />
                    <span
                      class="sp-pace-fill"
                      :style="{ height: `${(p.done / paceMax) * 100}%` }"
                    />
                  </span>
                  <span class="sp-pace-l">{{ p.title }}</span>
                </div>
              </div>
              <div v-if="paceAvg != null" class="sp-pace-note">
                {{ t('milestones.page.paceAvg', { n: pace.length - 1, avg: paceAvg }) }}
              </div>
            </section>
          </template>
        </div>
      </div>
    </template>

    <ActionMenu
      v-model:show="menuShow"
      :anchor="() => menuBtn"
      placement="bottom-end"
      :width="260"
      :header="stage?.title || ''"
      :items="menuItems"
      testid="stage-page-menu"
      @select="onMenu"
    />
    <StageDatesPopover
      v-model:show="datesShow"
      :anchor="() => menuBtn"
      placement="bottom-end"
      :stage="stage"
      @saved="load({ quiet: true })"
    />
  </div>
</template>

<style scoped>
.sp {
  height: 100%;
  overflow-y: auto;
  padding: 22px 32px 40px;
  box-sizing: border-box;
  color: var(--t-text1);
}
.sp.mobile {
  padding: 8px 12px 24px;
}
.sp-loading {
  display: flex;
  justify-content: center;
  padding: 80px 0;
}
.crumbs {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-size: 13.5px;
}
.crumb {
  color: var(--t-text2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 200px;
  min-width: 0;
}
/* The stage's own name keeps its room; the project gives way first. */
.crumb.link,
.crumb.cur {
  flex: none;
}
.crumb.link {
  border: 0;
  padding: 0;
  background: transparent;
  font: inherit;
  cursor: pointer;
}
.crumb.link:hover {
  color: var(--t-text1);
}
.crumb.cur {
  color: var(--t-text1);
  font-weight: 650;
  overflow: hidden;
  text-overflow: ellipsis;
}
.crumb-sep {
  color: var(--t-text3);
  flex: none;
}
.sp-head {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  margin-bottom: 22px;
}
.sp-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  flex: none;
  border-radius: 12px;
  background: color-mix(in srgb, var(--t-primary) 18%, var(--t-col));
  color: var(--t-primary);
}
.sp-head-main {
  flex: 1;
  min-width: 0;
}
.sp-title-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.sp-title {
  margin: 0;
  font-size: 26px;
  font-weight: 700;
  letter-spacing: -0.01em;
}
.sp-state {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 22px;
  padding: 0 9px;
  border-radius: 7px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 12px;
  font-weight: 650;
}
.sp-state.active {
  background: color-mix(in srgb, #18a058 18%, transparent);
  color: color-mix(in srgb, #18a058 80%, var(--t-text1));
}
.sp-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: linear-gradient(to top right, #128045, #18a058, #3cc47c);
}
.sp-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 14px;
  margin-top: 6px;
  font-size: 13.5px;
  color: var(--t-text2);
}
.sp-meta-i {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.sp-meta-i.strong {
  color: var(--t-text1);
  font-weight: 650;
}
.sp-meta-i.link {
  color: var(--t-text2);
  text-decoration: none;
}
.sp-meta-i.link:hover {
  color: var(--t-text1);
}
.sp-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sp-btn {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 34px;
  padding: 0 10px 0 12px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  white-space: nowrap;
  cursor: pointer;
}
.sp-btn:hover:not(:disabled) {
  background: var(--t-hover);
}
.sp-btn:disabled {
  opacity: 0.55;
  cursor: default;
}
.sp-btn:focus-visible,
.sp-iconbtn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.sp-btn.small {
  height: 28px;
  font-size: 13px;
}
.sp-btn.ghost {
  background: transparent;
}
.sp-btn.primary {
  background: var(--t-accent-grad);
  color: #fff;
  font-weight: 600;
}
.sp-btn.primary :deep(.kbd) {
  background: rgba(255, 255, 255, 0.22);
  color: #fff;
  box-shadow: none;
}
.sp-btn.armed {
  background: color-mix(in srgb, var(--t-primary) 22%, var(--t-ctl));
  color: var(--t-primary);
}
.sp-iconbtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.sp-iconbtn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.sp-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(300px, 352px);
  gap: 20px;
  align-items: start;
}
.sp-col {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}
/* Metric cards: --t-col, radius 16, padding 16, no shadow (mockup). */
.sp-card {
  padding: 16px;
  border-radius: 16px;
  background: var(--t-col);
  min-width: 0;
}
.sp-card.editing {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--t-primary) 55%, transparent);
}
.sp-card-h {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  min-height: 28px;
}
.sp-card-t {
  font-size: 14px;
  font-weight: 650;
  white-space: nowrap;
}
.sp-card-s {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.sp-grow {
  flex: 1;
}
.sp-goals {
  font-size: 14px;
  line-height: 1.6;
}
/* Goals are a short list: a heading in them stays at card scale. */
.sp-goals :deep(h1),
.sp-goals :deep(h2),
.sp-goals :deep(h3) {
  margin: 0 0 6px;
  font-size: 15px;
}
.sp-goals-empty {
  width: 100%;
  padding: 18px;
  border: 0;
  border-radius: 12px;
  background: var(--t-card);
  color: var(--t-text3);
  font: inherit;
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
}
.sp-goals-empty.static {
  cursor: default;
}
.sp-edit-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
}
.sp-edit-hint {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--t-text3);
}
.sp-edit-warn {
  font-size: 12.5px;
  color: var(--t-error);
}
.sp-seg {
  display: inline-flex;
  padding: 2px;
  border-radius: 9px;
  background: var(--t-card);
}
.sp-seg button {
  white-space: nowrap;
  height: 26px;
  padding: 0 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.sp-seg button.on {
  background: var(--t-pill);
  color: var(--t-text1);
  font-weight: 650;
}
.sp-chart-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 160px;
  border-radius: 12px;
  background: var(--t-card);
  color: var(--t-text3);
  font-size: 13px;
}
.sp-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 18px;
  margin-top: 10px;
  font-size: 12.5px;
  color: var(--t-text2);
}
.lg::before {
  content: '';
  display: inline-block;
  width: 14px;
  height: 0;
  margin-right: 6px;
  vertical-align: middle;
  border-top: 2.5px solid var(--t-primary);
}
.lg.scope::before {
  border-top: 2px solid var(--t-text2);
}
.lg.fc::before {
  border-top: 2px dashed var(--t-primary);
}
.sp-link {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.sp-link:hover {
  color: var(--t-text1);
}
.sp-none {
  padding: 8px 2px;
  font-size: 13px;
  color: var(--t-text3);
}
.sp-tgroup {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 4px;
  border: 0;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  font-weight: 650;
  cursor: pointer;
}
.sp-tcount {
  font-weight: 500;
  color: var(--t-text3);
}
.sp-task {
  display: grid;
  grid-template-columns: 18px 52px minmax(0, 1fr) auto 44px 50px;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 38px;
  padding: 0 8px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
}
.sp-task:hover {
  background: var(--t-hover);
}
.sp-task:focus-visible {
  outline: 0;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.sp-tnum {
  color: var(--t-text3);
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
}
.sp-ttitle {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sp-tdue {
  padding: 2px 7px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--t-error) 18%, transparent);
  color: var(--t-error);
  font-size: 11.5px;
  font-weight: 650;
}
.sp-test {
  text-align: right;
  color: var(--t-text3);
  font-size: 12.5px;
}
.sp-tav {
  display: flex;
  justify-content: flex-end;
  color: var(--t-text3);
}
.sp-av {
  width: 22px;
  height: 22px;
  font-size: 9px;
}
.sp-av.none {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--t-pill);
  color: var(--t-text3);
}
.sp-rings {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}
.sp-ring {
  display: flex;
  align-items: center;
  gap: 12px;
}
.sp-ring-l {
  font-size: 12.5px;
  color: var(--t-text3);
}
.sp-ring-v {
  margin: 4px 0;
  font-size: 14.5px;
  font-weight: 700;
  white-space: nowrap;
}
.sp-ring-s {
  font-size: 12px;
  color: var(--t-text3);
}
.sp-signals {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.sp-signal {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 30px;
  padding: 0 10px;
  border: 0;
  border-radius: 999px;
  background: var(--t-pill);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.sp-signal.red {
  background: color-mix(in srgb, #e0533d 18%, var(--t-col));
}
.sp-signal.red b {
  color: #e0533d;
}
.sp-signal.orange {
  background: color-mix(in srgb, #f0a020 18%, var(--t-col));
}
.sp-signal.orange b {
  color: #f0a020;
}
.sp-signal.on {
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.sp-stack {
  display: flex;
  gap: 3px;
  height: 12px;
}
.sp-stack-seg {
  flex-basis: 0;
  border-radius: 4px;
  min-width: 6px;
}
.sp-stat-list {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 18px;
  margin-top: 12px;
}
.sp-stat {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 13px;
  color: var(--t-text2);
  min-width: 0;
}
.sp-stat-n {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sp-stat b {
  color: var(--t-text1);
}
.sp-stat-dot {
  width: 8px;
  height: 8px;
  border-radius: 3px;
  flex: none;
}
.sp-tiles {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}
/* Nested tiles: --t-card with the card shadow (mockup). */
.sp-tile {
  padding: 12px;
  border-radius: 12px;
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
}
.sp-tile-v {
  font-size: 19px;
  font-weight: 700;
  white-space: nowrap;
}
.sp-tile-l {
  margin-top: 4px;
  font-size: 12px;
  color: var(--t-text3);
}
.sp-person {
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr) 64px 34px 40px;
  align-items: center;
  gap: 8px;
  min-height: 32px;
  font-size: 13.5px;
}
.sp-pname {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sp-pbar {
  height: 4px;
  border-radius: 2px;
  background: var(--t-pill);
  overflow: hidden;
}
.sp-pfill {
  display: block;
  height: 100%;
  background: var(--t-accent-grad);
}
.sp-pn,
.sp-pest {
  font-size: 12.5px;
  text-align: right;
  color: var(--t-text2);
  font-variant-numeric: tabular-nums;
}
.sp-pest {
  color: var(--t-text3);
}
.sp-pace {
  display: flex;
  justify-content: space-around;
  align-items: flex-end;
  gap: 12px;
  height: 130px;
}
.sp-pace-col {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  height: 100%;
  min-width: 0;
}
.sp-pace-v {
  font-size: 13px;
  font-weight: 650;
  white-space: nowrap;
}
.sp-pace-col.cur .sp-pace-v {
  color: var(--t-primary);
}
.sp-pace-bar {
  position: relative;
  flex: 1;
  width: 40px;
}
.sp-pace-fill,
.sp-pace-ghost {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  border-radius: 8px;
  background: var(--t-pill);
}
.sp-pace-ghost {
  background: color-mix(in srgb, var(--t-primary) 22%, transparent);
}
.sp-pace-col.cur .sp-pace-fill {
  background: var(--t-accent-grad-vert, var(--t-accent-grad));
}
.sp-pace-l {
  max-width: 76px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sp-pace-col.cur .sp-pace-l {
  color: var(--t-text1);
  font-weight: 650;
}
.sp-pace-note {
  margin-top: 10px;
  font-size: 12.5px;
  color: var(--t-text3);
}
/* ── phone (mockup MobileStage): one column — rings, signals, burn-up, goals,
   statuses. */
.sp-mhead {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.sp-mbadge {
  flex: none;
}
.sp-back {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--t-text1);
}
.sp-mtitle {
  flex: 1;
  text-align: center;
  min-width: 0;
}
.sp-mproj {
  font-size: 12px;
  color: var(--t-text3);
}
.sp-mname {
  font-size: 24px;
  font-weight: 700;
  letter-spacing: -0.01em;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sp-mchips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  font-size: 13px;
  color: var(--t-text2);
}
.sp-mleft {
  color: var(--t-text1);
  font-weight: 650;
}
.sp.mobile .sp-grid,
.sp.mobile .sp-col {
  display: contents;
}
.sp.mobile .sp-grid {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.sp.mobile .sp-rings {
  justify-content: space-around;
}
.m-order-1 {
  order: 1;
}
.m-order-2 {
  order: 2;
}
.m-order-3 {
  order: 3;
}
.m-order-4 {
  order: 4;
}
.m-order-5 {
  order: 5;
}
</style>
