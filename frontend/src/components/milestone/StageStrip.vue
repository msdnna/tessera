<script setup>
// The stage strip over the board (#2994, mockup StageStrip): shown while the
// board is filtered to one stage. Folded (the default) it is one 40px row —
// name, dates, progress «18 из 31», the first line of the goals, days left;
// unfolded, the goals in full (up to 240px, then a scroll) and three small
// figures. ⇧H or the chevron folds it; the state is kept per board and user.
// «⋯» — close the stage (⇧X), open it in GitLab, hide the strip on this board.
//
// A GitLab stage (#3010) is closed here too when its binding writes to GitLab;
// the write goes out at once. If GitLab refused it, the goal line gives way to
// «Описание не синхронизировано с GitLab · ↵ повторить» (mockup StageStrip,
// goalErr); a conflict says so the same way and opens the conflicts window.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  TrailSignOutline,
  ChevronDownOutline,
  ChevronUpOutline,
  ChevronForwardOutline,
  EllipsisHorizontalOutline,
  GitBranchOutline,
  CheckmarkDoneOutline,
  EyeOffOutline,
  OpenOutline,
  RefreshOutline,
  AlertCircleOutline,
  GitMergeOutline,
} from '@vicons/ionicons5'
import { milestones as msApi } from '@/api'
import { useFormat } from '@/composables/useFormat'
import { humanizeError } from '@/utils/errors'
import { formatHours } from '@/utils/estimation'
import { useStageSync } from '@/composables/useStageSync'
import {
  milestoneRangeShort,
  stageProgress,
  stageState,
  daysLeft,
  stagePath,
  stageEditable,
  stageUnsynced,
  stageConflict,
  unsyncedKey,
} from '@/utils/milestones'
import { percent } from '@/utils/milestoneMetrics'
import RichContent from '../RichContent.vue'
import ShortcutKeys from '../ShortcutKeys.vue'
import ActionMenu from '../kit/ActionMenu.vue'
import Kbd from '../Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import { tourEvent } from '@/utils/tourEvents'

const props = defineProps({
  stage: { type: Object, required: true },
  projectSlug: { type: String, default: '' },
  open: { type: Boolean, default: false },
  estimation: { type: Object, default: null },
  mobile: { type: Boolean, default: false },
  // Bumped by the board when its tasks change, so the figures follow.
  revision: { type: Number, default: 0 },
})
const emit = defineEmits(['update:open', 'hide', 'changed'])
const { t } = useI18n()
const router = useRouter()
const message = useMessage()
const { formatters, formatDateTime } = useFormat()

const progress = computed(() => stageProgress(props.stage))
const range = computed(() => milestoneRangeShort(props.stage, formatters.value))
const left = computed(() => (stageState(props.stage) === 'active' ? daysLeft(props.stage) : null))
const leftLabel = computed(() => {
  if (left.value == null) return ''
  if (left.value < 0) return t('board.strip.overdue', -left.value)
  return t('board.strip.left', left.value)
})
const linked = computed(() => !!props.stage.gl_global_id)
const editable = computed(() => stageEditable(props.stage))
const unsynced = computed(() => stageUnsynced(props.stage))
const conflict = computed(() => stageConflict(props.stage))
const sync = useStageSync()
const retrying = ref(false)
async function retry() {
  if (retrying.value) return
  retrying.value = true
  try {
    if (await sync.retry(props.stage)) emit('changed')
  } finally {
    retrying.value = false
  }
}
// The red line's one action: a conflict is settled in the window, a refused
// write is sent again.
function onSyncMark() {
  if (conflict.value) sync.resolve(props.stage)
  else retry()
}
// The first line of the goals with the Markdown taken off — the folded hint.
const firstLine = computed(() => {
  const lines = String(props.stage.description || '')
    .split('\n')
    .map((l) =>
      l
        .replace(/^\s*(#+|[-*+]|\d+[.)])\s*/, '')
        .replace(/[*_`>[\]]/g, '')
        .trim(),
    )
    .filter(Boolean)
  return lines.slice(0, 3).join(' · ')
})

// The unfolded figures need the metrics read.
const metrics = ref(null)
async function loadMetrics() {
  if (!props.open || props.mobile) return
  try {
    metrics.value = (await msApi.metrics(props.stage.id)).data
  } catch {
    metrics.value = null
  }
}
watch(() => [props.open, props.stage.id, props.revision], loadMetrics, { immediate: true })
const fmtEst = (v) => formatHours(v || 0, props.estimation)
const estPct = computed(() =>
  metrics.value ? percent(metrics.value.totals.done_estimate, metrics.value.totals.estimate) : 0,
)
const synced = computed(() =>
  metrics.value?.milestone?.gl_synced_at
    ? formatDateTime(metrics.value.milestone.gl_synced_at, {
        dateStyle: 'short',
        timeStyle: 'short',
      })
    : '',
)

// On a phone the strip does not unfold — its chevron leads to the stage page.
function toggle() {
  tourEvent('stage.strip')
  if (props.mobile) openPage()
  else emit('update:open', !props.open)
}
function openPage(edit = false) {
  router.push({
    path: stagePath(props.projectSlug, props.stage),
    query: edit ? { edit: '1' } : undefined,
  })
}

// ── «⋯» ──
const menuShow = ref(false)
const menuBtn = ref(null)
const closed = computed(() => props.stage.state === 'closed')
const menuItems = computed(() => [
  { key: 'page', label: t('board.strip.page'), icon: OpenOutline, keys: 'nav.goStage' },
  {
    key: 'close',
    label: closed.value ? t('milestones.stage.reopen') : t('milestones.page.close'),
    icon: closed.value ? RefreshOutline : CheckmarkDoneOutline,
    ...(!editable.value
      ? { disabled: true, disabledTitle: t('milestones.gitlab.readOnly') }
      : closed.value
        ? {}
        : {
            keys: 'stage.close',
            confirm: { title: t('board.strip.closeArmed', { name: props.stage.title }), sub: '' },
          }),
  },
  ...(conflict.value
    ? [{ key: 'resolve', label: t('milestones.gitlab.resolveMenu'), icon: GitMergeOutline }]
    : unsynced.value
      ? [{ key: 'retry', label: t('milestones.gitlab.retryMenu'), icon: RefreshOutline }]
      : []),
  ...(linked.value && props.stage.gl_url
    ? [{ key: 'gitlab', label: t('milestones.manager.openInGitlab'), icon: GitBranchOutline }]
    : []),
  { gap: true },
  { key: 'hide', label: t('board.strip.hide'), icon: EyeOffOutline },
])
async function onMenu(key) {
  if (key === 'page') openPage()
  else if (key === 'gitlab') window.open(props.stage.gl_url, '_blank', 'noopener')
  else if (key === 'hide') emit('hide')
  else if (key === 'resolve') {
    menuShow.value = false
    sync.resolve(props.stage)
  } else if (key === 'retry') {
    menuShow.value = false
    retry()
  } else if (key === 'close') {
    const m = props.stage
    try {
      const { data } = await msApi.update(m.id, {
        title: m.title,
        description: m.description || '',
        start_date: m.start_date || null,
        due_date: m.due_date || null,
        state: closed.value ? 'active' : 'closed',
      })
      menuShow.value = false
      sync.report(data)
      emit('changed')
    } catch (e) {
      message.error(humanizeError(e))
    }
  }
}
</script>

<template>
  <section
    class="sstrip"
    :class="{ open: open && !mobile, mobile }"
    data-testid="stage-strip"
    :data-open="open ? '1' : '0'"
  >
    <div class="sstrip-row">
      <n-icon :component="TrailSignOutline" :size="16" class="sstrip-ic" />
      <button type="button" class="sstrip-name" @click="openPage()">{{ stage.title }}</button>
      <span v-if="range && !mobile" class="sstrip-dates">{{ range }}</span>
      <span class="sstrip-bar"
        ><span class="sstrip-fill" :style="{ width: `${progress.pct}%` }"
      /></span>
      <span class="sstrip-count">{{
        t('board.strip.progress', { done: progress.done, total: progress.total })
      }}</span>
      <span v-if="!mobile && (unsynced || conflict)" class="sstrip-goal sstrip-err">
        <n-icon :component="AlertCircleOutline" :size="14" class="sstrip-err-ic" />
        <span class="sstrip-err-t">{{
          conflict ? t('milestones.gitlab.conflict') : t(unsyncedKey(stage))
        }}</span>
        <button
          type="button"
          class="sstrip-retry"
          :disabled="retrying"
          :title="conflict ? '' : stage.gl_push_error"
          data-testid="stage-strip-retry"
          @click="onSyncMark"
        >
          <Kbd>{{ CAPS.enter }}</Kbd
          >{{ conflict ? t('milestones.gitlab.resolve') : t('milestones.gitlab.retry') }}
        </button>
      </span>
      <span v-else-if="!mobile && !open" class="sstrip-goal">
        <template v-if="firstLine">{{ firstLine }}</template>
        <button v-else type="button" class="sstrip-add" @click="openPage(true)">
          {{ t('board.strip.noGoals') }}
        </button>
      </span>
      <span v-else class="sstrip-grow" />
      <button
        v-if="mobile && (unsynced || conflict)"
        type="button"
        class="sstrip-btn sstrip-err-ic"
        :title="conflict ? t('milestones.gitlab.conflict') : t(unsyncedKey(stage))"
        data-testid="stage-strip-retry"
        @click="onSyncMark"
      >
        <n-icon :component="AlertCircleOutline" :size="15" />
      </button>
      <span v-if="leftLabel" class="sstrip-left" :class="{ late: left < 0 }">{{ leftLabel }}</span>
      <ShortcutKeys v-if="!mobile" id="board.stageStrip" class="sstrip-kbd" />
      <button
        type="button"
        class="sstrip-btn"
        :title="open ? t('board.strip.fold') : t('board.strip.unfold')"
        data-testid="stage-strip-toggle"
        @click="toggle"
      >
        <n-icon
          :component="mobile ? ChevronForwardOutline : open ? ChevronUpOutline : ChevronDownOutline"
          :size="14"
        />
      </button>
      <button
        v-if="!mobile"
        ref="menuBtn"
        type="button"
        class="sstrip-btn"
        data-testid="stage-strip-menu"
        @click="menuShow = true"
      >
        <n-icon :component="EllipsisHorizontalOutline" :size="15" />
      </button>
    </div>

    <div v-if="open && !mobile" class="sstrip-body">
      <div class="sstrip-goals">
        <div class="sstrip-h">{{ t('board.strip.goals') }}</div>
        <div v-if="stage.description" class="sstrip-md">
          <RichContent :source="stage.description" />
        </div>
        <button v-else type="button" class="sstrip-add" @click="openPage(true)">
          {{ t('board.strip.noGoals') }}
        </button>
      </div>
      <div class="sstrip-side">
        <div class="sstrip-tiles">
          <div class="sstrip-tile">
            <div class="sstrip-tl">{{ t('board.strip.byEstimate') }}</div>
            <div class="sstrip-tv">{{ estPct }}%</div>
            <span class="sstrip-bar wide"
              ><span class="sstrip-fill" :style="{ width: `${estPct}%` }"
            /></span>
            <div class="sstrip-ts">
              {{
                metrics
                  ? t('board.strip.estOf', {
                      done: fmtEst(metrics.totals.done_estimate),
                      total: fmtEst(metrics.totals.estimate),
                    })
                  : ''
              }}
            </div>
          </div>
          <div class="sstrip-tile">
            <div class="sstrip-tl">{{ t('board.strip.afterStart') }}</div>
            <div class="sstrip-tv">
              +{{ metrics?.scope?.added ?? 0 }}
              <span class="sstrip-minus">−{{ metrics?.scope?.removed ?? 0 }}</span>
            </div>
            <div class="sstrip-ts">
              {{
                t('board.strip.scope', {
                  from: metrics?.scope?.at_start ?? 0,
                  to: metrics?.scope?.now ?? 0,
                })
              }}
            </div>
          </div>
          <div class="sstrip-tile">
            <div class="sstrip-tl">{{ t('board.strip.overdueTitle') }}</div>
            <div class="sstrip-tv" :class="{ red: (metrics?.signals?.overdue || 0) > 0 }">
              {{ metrics?.signals?.overdue ?? 0 }}
            </div>
            <div class="sstrip-ts">
              {{ t('board.strip.ofOpen', metrics?.totals?.open ?? 0) }}
            </div>
          </div>
        </div>
        <div class="sstrip-foot">
          <span v-if="linked" class="sstrip-gl">
            <n-icon :component="GitBranchOutline" :size="12" />
            %{{ stage.gl_iid ?? '' }}<template v-if="synced"> · {{ synced }}</template>
          </span>
          <span class="sstrip-grow" />
          <button type="button" class="sstrip-link" @click="openPage()">
            {{ t('board.strip.pageArrow') }}
          </button>
          <ShortcutKeys id="nav.goStage" />
        </div>
      </div>
    </div>

    <ActionMenu
      v-model:show="menuShow"
      :anchor="() => menuBtn"
      placement="bottom-end"
      :width="270"
      :header="stage.title"
      :items="menuItems"
      testid="stage-strip-actions"
      @select="onMenu"
    />
  </section>
</template>

<style scoped>
/* Margin 0 16 10 under the sub-toolbar, radius 12, --t-col — one step above the
   canvas, the level of the columns' own backing (mockup StageStrip). */
.sstrip {
  flex: none;
  margin: 0 16px 10px;
  border-radius: 12px;
  background: var(--t-col);
  color: var(--t-text1);
}
.sstrip.mobile {
  margin: 0 12px 8px;
}
.sstrip-row {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 40px;
  padding: 0 6px 0 12px;
  min-width: 0;
  font-size: 13px;
}
.sstrip-ic {
  flex: none;
  color: var(--t-primary);
}
.sstrip-name {
  flex: none;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  font-weight: 650;
  cursor: pointer;
}
.sstrip-name:hover {
  text-decoration: underline;
}
.sstrip-dates {
  flex: none;
  color: var(--t-text3);
}
.sstrip-bar {
  position: relative;
  flex: none;
  width: 120px;
  height: 4px;
  border-radius: 2px;
  background: var(--t-pill);
  overflow: hidden;
}
.mobile .sstrip-bar {
  width: 56px;
}
.sstrip-bar.wide {
  display: block;
  width: 100%;
  margin: 6px 0;
}
.sstrip-fill {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: 2px;
  background: var(--t-accent-grad);
}
.sstrip-count {
  flex: none;
  color: var(--t-text2);
  font-variant-numeric: tabular-nums;
}
.sstrip-goal {
  flex: 1;
  min-width: 0;
  color: var(--t-text2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sstrip-grow {
  flex: 1;
}
/* «Описание не синхронизировано с GitLab · ↵ повторить» in place of the goal
   line (mockup StageStrip, goalErr): text1, the icon red, the action text2. */
.sstrip-err {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--t-text1);
}
.sstrip-err-t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sstrip-err-ic {
  flex: none;
  color: var(--t-error);
}
.sstrip-retry {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 0 4px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  cursor: pointer;
}
.sstrip-retry:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.sstrip-retry:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.sstrip-left {
  flex: none;
  height: 22px;
  padding: 0 8px;
  display: inline-flex;
  align-items: center;
  border-radius: 7px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 12px;
}
.sstrip-left.late {
  color: var(--t-error);
}
.sstrip-kbd {
  flex: none;
}
.sstrip-btn {
  flex: none;
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
}
.sstrip-btn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.sstrip-btn:focus-visible,
.sstrip-name:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.sstrip-add {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--t-primary);
  font: inherit;
  cursor: pointer;
}
.sstrip-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 340px;
  gap: 18px;
  padding: 2px 12px 12px 38px;
}
.sstrip-h {
  margin-bottom: 6px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.sstrip-goals {
  max-height: 240px;
  overflow-y: auto;
  min-width: 0;
}
.sstrip-md {
  font-size: 13.5px;
  line-height: 1.6;
}
.sstrip-md :deep(h1),
.sstrip-md :deep(h2),
.sstrip-md :deep(h3) {
  margin: 0 0 4px;
  font-size: 13.5px;
  font-weight: 650;
}
.sstrip-md :deep(p),
.sstrip-md :deep(ol),
.sstrip-md :deep(ul) {
  margin: 0 0 4px;
}
.sstrip-tiles {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}
.sstrip-tile {
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  min-width: 0;
}
.sstrip-tl {
  font-size: 11.5px;
  color: var(--t-text3);
}
.sstrip-tv {
  margin-top: 6px;
  font-size: 17px;
  font-weight: 700;
  white-space: nowrap;
}
.sstrip-tv.red {
  color: var(--t-error);
}
.sstrip-minus {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text3);
}
.sstrip-ts {
  margin-top: 4px;
  font-size: 11.5px;
  line-height: 1.35;
  color: var(--t-text3);
}
.sstrip-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  font-size: 12.5px;
}
.sstrip-gl {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--t-text3);
}
.sstrip-link {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--t-primary);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
</style>
