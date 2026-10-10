<script setup>
// «Этапы» of the «Проект» window (#2989, mockup ProjectWin «Этапы»): the
// project's stages with dates, state, progress and their GitLab milestone. A
// click (↵) opens the stage page; a new stage is an empty row — name, Tab, dates,
// ↵. Closed stages sit in their own folded group. A stage that comes from
// GitLab (#3010) is renamed, re-dated and closed here (⇧X on its row) when its
// binding writes to GitLab — the change goes to the milestone at once; without
// writing it stays read-only. A refused write shows as a red mark in the GitLab
// column, with «Повторить запись в GitLab» in the row's menu.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CAPS, cap } from '@/utils/keyCaps'
import { NIcon, NDatePicker } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  AddOutline,
  TrailSignOutline,
  GitBranchOutline,
  ChevronDownOutline,
  ChevronForwardOutline,
  EllipsisHorizontalOutline,
  OpenOutline,
  CreateOutline,
  CalendarClearOutline,
  CheckmarkDoneOutline,
  RefreshOutline,
  CloudUploadOutline,
  TrashOutline,
  AlertCircleOutline,
  GitMergeOutline,
} from '@vicons/ionicons5'
import { projects as projApi, milestones as msApi, gitlab as glApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useFormat } from '@/composables/useFormat'
import { humanizeError } from '@/utils/errors'
import { useStageSync } from '@/composables/useStageSync'
import {
  milestoneRangeShort,
  toDateOnlyISO,
  stageState,
  daysLeft,
  stageProgress,
  sortStages,
  stagePath,
  stageEditable,
  stageUnsynced,
  stageConflict,
  unsyncedKey,
} from '@/utils/milestones'
import InlineEdit from '../kit/InlineEdit.vue'
import ActionMenu from '../kit/ActionMenu.vue'
import Kbd from '../Kbd.vue'
import StageDatesPopover from './StageDatesPopover.vue'

const props = defineProps({
  project: { type: Object, required: true },
})
const emit = defineEmits(['count', 'changed', 'open-stage'])
const { t } = useI18n()
const message = useMessage()
const ws = useWorkspacesStore()
const { formatters, datePattern, firstDayOfWeek } = useFormat()

const list = ref([])
const loading = ref(true)
const glLinkable = ref(false)
async function load() {
  const pid = props.project.id
  try {
    const { data } = await projApi.milestones(pid)
    if (pid !== props.project.id) return
    list.value = data || []
    emit('count', list.value.length)
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    loading.value = false
  }
  glLinkable.value = false
  if (ws.currentId) {
    try {
      const { data } = await glApi.listIntegrations(ws.currentId)
      glLinkable.value = (data.integrations || []).some(
        (b) => b.enabled === true && b.writeback?.push_milestone === true && b.project_id === pid,
      )
    } catch {
      glLinkable.value = false
    }
  }
}
onMounted(load)
watch(() => props.project.id, load)
watch(
  () => ws.milestoneRevFor(props.project.id),
  () => load(),
)
async function changed() {
  emit('changed')
  await load()
}

const sorted = computed(() => sortStages(list.value))
const open = computed(() => sorted.value.filter((m) => m.state !== 'closed'))
const closed = computed(() => sorted.value.filter((m) => m.state === 'closed'))
const showClosed = ref(false)
const linked = (m) => !!m.gl_global_id
const sync = useStageSync()
// A GitLab stage whose binding does not write stays read-only (#3010).
const lockOf = (m) =>
  stageEditable(m) ? {} : { disabled: true, disabledTitle: t('milestones.gitlab.readOnly') }
function syncMarkTitle(m) {
  if (stageConflict(m)) return t('milestones.gitlab.conflict')
  if (stageUnsynced(m)) return `${t(unsyncedKey(m))}: ${m.gl_push_error}`
  return ''
}

const range = (m) => milestoneRangeShort(m, formatters.value)
function stateLabel(m) {
  return t(`milestones.state.${stageState(m)}`)
}
function leftLabel(m) {
  if (stageState(m) !== 'active') return ''
  const d = daysLeft(m)
  if (d == null) return ''
  if (d < 0) return t('milestones.stage.overdueDays', -d)
  return t('milestones.stage.daysLeft', d)
}

// The window closes and navigates (ProjectWindow — it owns the history entry
// Back closes it with).
function openStage(m) {
  emit('open-stage', stagePath(props.project.slug, m))
}

// ── new stage: name, Tab, dates, ↵ ──
const adding = ref(false)
const newTitle = ref('')
const newRange = ref(null)
const newBusy = ref(false)
const newErr = ref('')
const newTitleEl = ref(null)
const newDatesEl = ref(null)
function startAdd() {
  adding.value = true
  newTitle.value = ''
  newRange.value = null
  newErr.value = ''
  nextTick(() => newTitleEl.value?.focus())
}
async function createStage() {
  const title = newTitle.value.trim()
  if (!title || newBusy.value) return
  newBusy.value = true
  newErr.value = ''
  try {
    const body = { title }
    if (newRange.value) {
      body.start_date = toDateOnlyISO(newRange.value[0])
      body.due_date = toDateOnlyISO(newRange.value[1])
    }
    await projApi.createMilestone(props.project.id, body)
    newTitle.value = ''
    newRange.value = null
    await changed()
    nextTick(() => newTitleEl.value?.focus())
  } catch (e) {
    newErr.value = humanizeError(e)
  } finally {
    newBusy.value = false
  }
}
function onNewTitleKey(e) {
  if (e.key === 'Tab' && !e.shiftKey) {
    e.preventDefault()
    newDatesEl.value?.focus?.()
  } else if (e.key === 'Enter') {
    e.preventDefault()
    createStage()
  } else if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    adding.value = false
  }
}

// ── rename in place (native stages) ──
const renamingId = ref(null)
const renameValue = ref('')
const renameBusy = ref(false)
function startRename(m) {
  if (!stageEditable(m)) return
  renamingId.value = m.id
  renameValue.value = m.title
}
async function commitRename(m, value, { via }) {
  const n = (value || '').trim()
  if (!n || n === m.title) {
    if (!n && via === 'enter') return
    renamingId.value = null
    return
  }
  renameBusy.value = true
  try {
    const { data } = await msApi.update(m.id, {
      title: n,
      description: m.description || '',
      start_date: m.start_date || null,
      due_date: m.due_date || null,
      state: m.state,
    })
    sync.report(data)
    renamingId.value = null
    await changed()
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    renameBusy.value = false
  }
}

// ── menu ──
const menuFor = ref(null)
const menuAnchor = ref(null)
const menuShow = computed({
  get: () => !!menuFor.value,
  set: (v) => {
    if (!v) menuFor.value = null
  },
})
const menuStage = computed(() => list.value.find((m) => m.id === menuFor.value) || null)
const busy = ref(false)
const failed = ref('')
const menuItems = computed(() => {
  const m = menuStage.value
  if (!m) return []
  const gl = linked(m)
  const items = [
    { key: 'open', label: t('milestones.stage.openPage'), icon: OpenOutline, caps: ['↵'] },
    {
      key: 'rename',
      label: t('milestones.stage.rename'),
      icon: CreateOutline,
      caps: ['F2'],
      ...lockOf(m),
    },
    {
      key: 'dates',
      label: t('milestones.stage.datesMenu'),
      icon: CalendarClearOutline,
      ...lockOf(m),
    },
    {
      key: 'state',
      label: m.state === 'closed' ? t('milestones.stage.reopen') : t('milestones.stage.close'),
      icon: m.state === 'closed' ? RefreshOutline : CheckmarkDoneOutline,
      keys: 'stage.close',
      ...lockOf(m),
    },
  ]
  if (stageConflict(m)) {
    items.push({ key: 'resolve', label: t('milestones.gitlab.resolveMenu'), icon: GitMergeOutline })
  } else if (stageUnsynced(m)) {
    items.push({ key: 'retry', label: t('milestones.gitlab.retryMenu'), icon: RefreshOutline })
  }
  if (!gl && glLinkable.value) {
    items.push({ key: 'push', label: t('milestones.manager.push'), icon: CloudUploadOutline })
  }
  if (gl && m.gl_url) {
    items.push({ key: 'gitlab', label: t('milestones.manager.openInGitlab'), icon: OpenOutline })
  }
  items.push({ gap: true })
  items.push({
    key: 'delete',
    label: t('milestones.stage.delete'),
    icon: TrashOutline,
    danger: true,
    confirm: {
      title: t('milestones.stage.deleteArmed', { name: m.title }),
      sub: gl ? t('milestones.manager.deleteLinked') : t('milestones.stage.deleteSub'),
    },
    busy: busy.value,
    failed: failed.value ? { title: t('milestones.stage.deleteFailed'), sub: failed.value } : null,
  })
  return items
})
function openMenu(m, el) {
  failed.value = ''
  menuAnchor.value = el
  menuFor.value = m.id
}
const datesFor = ref(null)
const datesAnchor = ref(null)
const datesShow = computed({
  get: () => !!datesFor.value,
  set: (v) => {
    if (!v) datesFor.value = null
  },
})
async function onMenu(key) {
  const m = menuStage.value
  if (!m) return
  if (key === 'open') openStage(m)
  else if (key === 'rename') startRename(m)
  else if (key === 'dates') {
    datesAnchor.value = rowEls[m.id]
    datesFor.value = m
  } else if (key === 'state') await toggleState(m)
  else if (key === 'push') await pushToGitlab(m)
  else if (key === 'retry') {
    menuFor.value = null
    if (await sync.retry(m)) await changed()
  } else if (key === 'resolve') {
    menuFor.value = null
    sync.resolve(m)
  } else if (key === 'gitlab') window.open(m.gl_url, '_blank', 'noopener')
  else if (key === 'delete') await remove(m)
}
async function toggleState(m) {
  if (!stageEditable(m)) return
  try {
    const { data } = await msApi.update(m.id, {
      title: m.title,
      description: m.description || '',
      start_date: m.start_date || null,
      due_date: m.due_date || null,
      state: m.state === 'closed' ? 'active' : 'closed',
    })
    sync.report(data)
    await changed()
  } catch (e) {
    message.error(humanizeError(e))
  }
}
async function pushToGitlab(m) {
  try {
    await msApi.pushToGitlab(m.id)
    await changed()
    message.success(t('milestones.manager.pushed'))
  } catch (e) {
    message.error(humanizeError(e))
  }
}
async function remove(m) {
  busy.value = true
  failed.value = ''
  try {
    await msApi.remove(m.id)
    menuFor.value = null
    await changed()
  } catch (e) {
    failed.value = humanizeError(e)
  } finally {
    busy.value = false
  }
}

// ── keys: ↑↓ walk rows, ↵ opens, F2 renames, N — new stage ──
const rowEls = {}
function rowKey(e, m, i, rows) {
  if (renamingId.value) return
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    const next = rows[i + (e.key === 'ArrowDown' ? 1 : -1)]
    if (next) rowEls[next.id]?.focus()
  } else if (e.key === 'Enter') {
    e.preventDefault()
    openStage(m)
  } else if (e.key === 'F2') {
    e.preventDefault()
    startRename(m)
  } else if (e.shiftKey && e.code === 'KeyX' && !e.metaKey && !e.ctrlKey && !e.altKey) {
    // ⇧X — close the stage (or reopen a closed one), as on its page.
    e.preventDefault()
    toggleState(m)
  } else if (e.code === 'Period' || (e.shiftKey && e.key === 'F10')) {
    e.preventDefault()
    openMenu(m, rowEls[m.id])
  }
}
function onPaneKey(e) {
  if (e.target?.tagName === 'INPUT' || renamingId.value) return
  if (e.code === 'KeyN' && !e.metaKey && !e.ctrlKey && !e.altKey) {
    e.preventDefault()
    startAdd()
  }
}
const allRows = computed(() => [...open.value, ...(showClosed.value ? closed.value : [])])

const rootEl = ref(null)
function focusFirst() {
  nextTick(() => {
    const first = allRows.value[0]
    // No stages yet: the pane itself, so N still starts the first one.
    if (first) rowEls[first.id]?.focus()
    else rootEl.value?.focus()
  })
}
function escape() {
  if (adding.value) {
    adding.value = false
    return true
  }
  return false
}
defineExpose({ focusFirst, escape })
</script>

<template>
  <div ref="rootEl" class="st" tabindex="-1" data-testid="project-stages" @keydown="onPaneKey">
    <div class="st-title">
      <h2>{{ t('project.window.tab.stages') }}</h2>
      <span class="st-note">
        {{ t('milestones.stage.summaryOpen', open.length) }} ·
        {{ t('milestones.stage.summaryClosed', closed.length) }}
      </span>
      <span class="st-grow" />
      <button type="button" class="st-add" data-testid="stage-add" @click="startAdd">
        <n-icon :component="AddOutline" :size="15" />
        {{ t('milestones.stage.add') }}
        <Kbd>{{ cap('n') }}</Kbd>
      </button>
    </div>

    <div class="st-head">
      <span>{{ t('milestones.stage.colStage') }}</span>
      <span>{{ t('milestones.stage.colDates') }}</span>
      <span>{{ t('milestones.stage.colState') }}</span>
      <span>{{ t('milestones.stage.colProgress') }}</span>
      <span>GitLab</span>
      <span />
    </div>

    <div class="st-body">
      <div v-if="!loading && !list.length && !adding" class="st-empty">
        {{ t('milestones.stage.empty') }}
      </div>

      <template v-for="(m, i) in open" :key="m.id">
        <div
          :ref="(el) => (rowEls[m.id] = el)"
          class="st-row"
          :class="{ ctx: menuFor === m.id }"
          tabindex="-1"
          data-testid="stage-row"
          :data-stage="m.title"
          @click="renamingId !== m.id && openStage(m)"
          @keydown="rowKey($event, m, i, allRows)"
        >
          <div class="st-name">
            <n-icon :component="TrailSignOutline" :size="16" class="st-ic" />
            <InlineEdit
              v-if="renamingId === m.id"
              v-model="renameValue"
              :busy="renameBusy"
              @commit="(v, how) => commitRename(m, v, how)"
              @cancel="renamingId = null"
            />
            <template v-else>
              <span class="st-t">{{ m.title }}</span>
              <span v-if="leftLabel(m)" class="st-left">{{ leftLabel(m) }}</span>
            </template>
          </div>
          <span class="st-dates">{{ range(m) || '—' }}</span>
          <span class="st-state" :class="stageState(m)">
            <span v-if="stageState(m) === 'active'" class="st-dot" />{{ stateLabel(m) }}
          </span>
          <span class="st-prog">
            <span class="st-bar">
              <span class="st-fill" :style="{ width: `${stageProgress(m).pct}%` }" />
            </span>
            <span class="st-n">{{ stageProgress(m).done }} / {{ stageProgress(m).total }}</span>
          </span>
          <span class="st-gl">
            <template v-if="linked(m)">
              <n-icon :component="GitBranchOutline" :size="13" />%{{ m.gl_iid ?? '' }}
              <n-icon
                v-if="syncMarkTitle(m)"
                :component="AlertCircleOutline"
                :size="13"
                class="st-gl-err"
                :title="syncMarkTitle(m)"
                data-testid="stage-row-unsynced"
              />
            </template>
          </span>
          <button
            type="button"
            class="st-more"
            tabindex="-1"
            data-testid="stage-menu"
            @click.stop="openMenu(m, $event.currentTarget)"
          >
            <n-icon :component="EllipsisHorizontalOutline" :size="15" />
          </button>
        </div>
      </template>

      <div v-if="adding" class="st-row st-new" data-testid="stage-new-row">
        <div class="st-name">
          <n-icon :component="TrailSignOutline" :size="16" class="st-ic" />
          <input
            ref="newTitleEl"
            v-model="newTitle"
            class="st-input"
            :placeholder="t('milestones.stage.namePlaceholder')"
            :disabled="newBusy"
            data-testid="stage-new-title"
            @keydown="onNewTitleKey"
          />
        </div>
        <div class="st-new-dates">
          <n-date-picker
            ref="newDatesEl"
            v-model:value="newRange"
            type="daterange"
            size="small"
            clearable
            :format="datePattern"
            :first-day-of-week="firstDayOfWeek"
            @keydown.enter.prevent="createStage"
          />
        </div>
        <span class="st-new-hint">
          <Kbd>{{ CAPS.tab }}</Kbd
          >{{ t('milestones.stage.hintDates') }} <Kbd>{{ CAPS.enter }}</Kbd
          >{{ t('milestones.stage.hintCreate') }} <Kbd>{{ CAPS.escape }}</Kbd>
        </span>
      </div>
      <div v-if="newErr" class="st-err">{{ newErr }}</div>

      <template v-if="closed.length">
        <button
          type="button"
          class="st-group"
          data-testid="stage-closed-toggle"
          @click="showClosed = !showClosed"
        >
          <n-icon :component="showClosed ? ChevronDownOutline : ChevronForwardOutline" :size="13" />
          {{ t('milestones.stage.closedGroup') }}
          <span class="st-gcount">{{ closed.length }}</span>
        </button>
        <template v-if="showClosed">
          <div
            v-for="(m, i) in closed"
            :key="m.id"
            :ref="(el) => (rowEls[m.id] = el)"
            class="st-row closed"
            :class="{ ctx: menuFor === m.id }"
            tabindex="-1"
            data-testid="stage-row"
            :data-stage="m.title"
            @click="openStage(m)"
            @keydown="rowKey($event, m, open.length + i, allRows)"
          >
            <div class="st-name">
              <n-icon :component="TrailSignOutline" :size="16" class="st-ic" />
              <span class="st-t">{{ m.title }}</span>
            </div>
            <span class="st-dates">{{ range(m) || '—' }}</span>
            <span class="st-state closed">{{ stateLabel(m) }}</span>
            <span class="st-prog">
              <span class="st-bar">
                <span
                  class="st-fill"
                  :class="{ full: stageProgress(m).pct === 100 }"
                  :style="{ width: `${stageProgress(m).pct}%` }"
                />
              </span>
              <span class="st-n">{{ stageProgress(m).done }} / {{ stageProgress(m).total }}</span>
            </span>
            <span class="st-gl">
              <template v-if="linked(m)">
                <n-icon :component="GitBranchOutline" :size="13" />%{{ m.gl_iid ?? '' }}
                <n-icon
                  v-if="syncMarkTitle(m)"
                  :component="AlertCircleOutline"
                  :size="13"
                  class="st-gl-err"
                  :title="syncMarkTitle(m)"
                  data-testid="stage-row-unsynced"
                />
              </template>
            </span>
            <button
              type="button"
              class="st-more"
              tabindex="-1"
              @click.stop="openMenu(m, $event.currentTarget)"
            >
              <n-icon :component="EllipsisHorizontalOutline" :size="15" />
            </button>
          </div>
        </template>
      </template>
    </div>

    <ActionMenu
      v-model:show="menuShow"
      :anchor="() => menuAnchor"
      placement="bottom-end"
      :width="270"
      :header="menuStage?.title || ''"
      :items="menuItems"
      testid="stage-actions"
      @select="onMenu"
    />
    <StageDatesPopover
      v-model:show="datesShow"
      :anchor="() => datesAnchor"
      :stage="datesFor"
      @saved="changed"
    />
  </div>
</template>

<style scoped>
.st {
  outline: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  padding: 6px 4px 0 0;
}
.st-title {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
}
.st-title h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
}
.st-note {
  font-size: 13px;
  color: var(--t-text3);
}
.st-grow {
  flex: 1;
}
.st-add {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 6px 0 12px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: var(--t-accent-grad);
  color: #fff;
  font: inherit;
  font-size: 13.5px;
  font-weight: 600;
  cursor: pointer;
}
.st-add :deep(.kbd) {
  background: rgba(255, 255, 255, 0.22);
  color: #fff;
  box-shadow: none;
}
.st-add:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.st-head,
.st-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 140px 124px 140px 72px 32px;
  align-items: center;
  gap: 8px;
}
.st-head {
  padding: 0 12px 8px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.st-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 4px 16px 0;
}
.st-empty {
  padding: 32px 0;
  text-align: center;
  font-size: 13px;
  color: var(--t-text3);
}
.st-row {
  min-height: 48px;
  padding: 0 4px 0 12px;
  border-radius: 10px;
  outline: 0;
  cursor: pointer;
  font-size: 13px;
}
.st-row:hover,
.st-row.ctx {
  background: var(--t-hover);
}
.st-row:focus-visible {
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.st-row.closed {
  opacity: 0.66;
}
.st-new {
  cursor: default;
  grid-template-columns: minmax(0, 1fr) 260px auto;
}
.st-new:hover {
  background: transparent;
}
.st-name {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.st-ic {
  flex: none;
  color: var(--t-text3);
}
.st-t {
  font-size: 14px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.st-left {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
}
.st-dates {
  color: var(--t-text2);
  white-space: nowrap;
}
.st-state {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  width: fit-content;
  height: 22px;
  padding: 0 8px;
  border-radius: 7px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}
.st-state.active {
  background: color-mix(in srgb, #18a058 18%, transparent);
  color: color-mix(in srgb, #18a058 80%, var(--t-text1));
}
.st-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: linear-gradient(to top right, #128045, #18a058, #3cc47c);
}
.st-prog {
  display: flex;
  align-items: center;
  gap: 10px;
}
.st-bar {
  position: relative;
  width: 70px;
  height: 4px;
  border-radius: 2px;
  background: var(--t-pill);
  overflow: hidden;
}
.st-fill {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: 2px;
  background: var(--t-accent-grad);
}
.st-fill.full {
  background: linear-gradient(to right, #128045, #18a058, #3cc47c);
}
.st-n {
  font-variant-numeric: tabular-nums;
  color: var(--t-text2);
  white-space: nowrap;
}
.st-gl-err {
  margin-left: 4px;
  color: var(--t-error);
}
.st-gl {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--t-text3);
  font-size: 12.5px;
}
.st-more {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  opacity: 0;
}
.st-row:hover .st-more,
.st-row.ctx .st-more,
.st-row:focus-visible .st-more {
  opacity: 1;
}
.st-input {
  flex: 1;
  min-width: 0;
  height: 32px;
  padding: 0 10px;
  border: 0;
  outline: 0;
  border-radius: 9px;
  background: var(--t-input-bg);
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  box-shadow: 0 0 0 2px var(--t-primary);
}
.st-new-hint {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.st-err {
  padding: 4px 12px;
  font-size: 12.5px;
  color: var(--t-error);
}
.st-group {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 38px;
  margin-top: 10px;
  padding: 0 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  font-weight: 650;
  cursor: pointer;
}
.st-group:hover {
  background: var(--t-hover);
}
.st-gcount {
  font-weight: 500;
  color: var(--t-text3);
}
@media (max-width: 768px) {
  .st-head {
    display: none;
  }
  .st-row {
    grid-template-columns: minmax(0, 1fr) auto 32px;
  }
  .st-dates,
  .st-gl,
  .st-prog {
    display: none;
  }
  .st-new {
    grid-template-columns: minmax(0, 1fr);
    padding: 8px 12px;
  }
  .st-new-hint {
    display: none;
  }
}
</style>
