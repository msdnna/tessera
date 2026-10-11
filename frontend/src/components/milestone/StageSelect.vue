<script setup>
import { vLayer } from '@/utils/layerZ'
// The stage selector of the board's sub-toolbar (#2994, mockup StageMenu): «Этап:
// Спринт 42 ▬ 58% ⌄» next to the grouping, in place of the old removable chip.
// The menu lists the active stages with their progress, the planned ones, «Бэклог»
// (tasks without a stage) and «Все задачи»; closed stages under «Показать
// закрытые». ⇧E opens it with the search focused; ↑↓ ↵ pick, B — «Бэклог»,
// A — «Все задачи» (while the search is empty).
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { cap } from '@/utils/keyCaps'
import { NIcon } from 'naive-ui'
import {
  TrailSignOutline,
  ChevronDownOutline,
  ChevronForwardOutline,
  SearchOutline,
  CheckmarkOutline,
  FileTrayStackedOutline,
  AppsOutline,
  SettingsOutline,
} from '@vicons/ionicons5'
import { useAnchoredPanel } from '@/composables/useAnchoredPanel'
import { useFormat } from '@/composables/useFormat'
import {
  BACKLOG_SCOPE,
  matchesScope,
  milestoneRangeShort,
  sortStages,
  stageProgress,
  stageState,
  daysLeft,
} from '@/utils/milestones'
import ShortcutKeys from '../ShortcutKeys.vue'
import Kbd from '../Kbd.vue'
import { tourEvent } from '@/utils/tourEvents'

const props = defineProps({
  stages: { type: Array, default: () => [] },
  // The ?milestone= value: '' (all tasks), 'backlog', a slug or an id.
  scope: { type: String, default: '' },
  // No caption and no progress — the sub-toolbar is short of room.
  compact: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['pick', 'manage'])
const { t } = useI18n()
const { formatters } = useFormat()

const current = computed(() =>
  props.scope && props.scope !== BACKLOG_SCOPE
    ? props.stages.find((m) => matchesScope(m, props.scope)) || null
    : null,
)
const label = computed(() => {
  if (!props.scope) return t('board.stage.all')
  if (props.scope === BACKLOG_SCOPE) return t('board.scope.backlog')
  return current.value?.title || t('board.scope.milestoneFallback')
})
const pct = computed(() => (current.value ? stageProgress(current.value).pct : 0))

const open = ref(false)
const btn = ref(null)
const panel = ref(null)
const searchEl = ref(null)
const q = ref('')
const showClosed = ref(false)
const cursor = ref(0)
const { style } = useAnchoredPanel({
  show: open,
  anchor: () => btn.value,
  panel,
  placement: 'bottom-end',
  offset: { x: 0, y: 6 },
  onOutside: () => (open.value = false),
})
function toggle() {
  if (props.disabled) return
  open.value = !open.value
}
watch(open, async (on) => {
  if (!on) return
  q.value = ''
  cursor.value = 0
  await nextTick()
  searchEl.value?.focus()
})
defineExpose({ open: () => !props.disabled && (open.value = true) })

const sorted = computed(() => sortStages(props.stages))
const match = (m) => !q.value.trim() || m.title.toLowerCase().includes(q.value.trim().toLowerCase())
const active = computed(() => sorted.value.filter((m) => stageState(m) === 'active' && match(m)))
const planned = computed(() => sorted.value.filter((m) => stageState(m) === 'planned' && match(m)))
const closed = computed(() => sorted.value.filter((m) => m.state === 'closed'))
const closedShown = computed(() =>
  showClosed.value || q.value.trim() ? closed.value.filter(match) : [],
)
// The rows ↑↓ walk, in order.
const rows = computed(() => [
  ...active.value.map((m) => ({ key: m.id, stage: m })),
  ...planned.value.map((m) => ({ key: m.id, stage: m })),
  { key: BACKLOG_SCOPE },
  { key: '' },
  ...closedShown.value.map((m) => ({ key: m.id, stage: m })),
])
const isCursor = (key) => rows.value[cursor.value]?.key === key

function pick(row) {
  open.value = false
  tourEvent('stage.selected')
  if (!row.stage) emit('pick', row.key)
  else emit('pick', row.stage)
}
function isOn(row) {
  if (!row.stage) return row.key === props.scope || (!row.key && !props.scope)
  return matchesScope(row.stage, props.scope)
}
function onKey(e) {
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    const n = rows.value.length
    cursor.value = (cursor.value + (e.key === 'ArrowDown' ? 1 : n - 1)) % n
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const r = rows.value[cursor.value]
    if (r) pick(r)
  } else if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    open.value = false
    btn.value?.focus()
  } else if (!q.value && !e.metaKey && !e.ctrlKey && !e.altKey) {
    if (e.code === 'KeyB') {
      e.preventDefault()
      pick({ key: BACKLOG_SCOPE })
    } else if (e.code === 'KeyA') {
      e.preventDefault()
      pick({ key: '' })
    }
  }
}
watch(q, () => (cursor.value = 0))

const sub = (m) => {
  const r = milestoneRangeShort(m, formatters.value)
  const parts = [r]
  const d = daysLeft(m)
  if (stageState(m) === 'active' && d != null && d >= 0) parts.push(t('board.stage.left', d))
  if (m.gl_global_id) parts.push(t('board.stage.gitlab'))
  return parts.filter(Boolean).join(' · ')
}
</script>

<template>
  <button
    ref="btn"
    type="button"
    class="ss"
    :class="{ on: !!scope, open }"
    :disabled="disabled"
    data-testid="stage-select"
    :title="t('board.stage.title')"
    @click="toggle"
  >
    <n-icon :component="TrailSignOutline" :size="14" class="ss-ic" />
    <span v-if="!compact" class="ss-lbl">{{ t('board.stage.caption') }}</span>
    <span class="ss-val">{{ label }}</span>
    <template v-if="current && !compact">
      <span class="ss-bar"><span class="ss-fill" :style="{ width: `${pct}%` }" /></span>
      <span class="ss-pct">{{ pct }}%</span>
    </template>
    <n-icon :component="ChevronDownOutline" :size="12" class="ss-ic" />
  </button>

  <Teleport to="body">
    <div
      v-if="open"
      ref="panel"
      v-layer
      class="ss-menu"
      :style="style"
      role="listbox"
      data-testid="stage-menu"
      @keydown="onKey"
    >
      <label class="ss-search">
        <n-icon :component="SearchOutline" :size="14" />
        <input
          ref="searchEl"
          v-model="q"
          :placeholder="t('board.stage.search')"
          spellcheck="false"
          data-testid="stage-search"
        />
        <ShortcutKeys id="board.stage" />
      </label>

      <template v-if="active.length">
        <div class="ss-h">
          <span>{{ t('board.stage.active') }}</span
          ><span>{{ active.length }}</span>
        </div>
        <button
          v-for="m in active"
          :key="m.id"
          type="button"
          class="ss-row"
          :class="{ on: isOn({ key: m.id, stage: m }), cur: isCursor(m.id) }"
          data-testid="stage-option"
          @click="pick({ key: m.id, stage: m })"
          @mousemove="cursor = rows.findIndex((r) => r.key === m.id)"
        >
          <n-icon :component="TrailSignOutline" :size="16" class="ss-row-ic" />
          <span class="ss-row-t">
            <span class="ss-row-n">{{ m.title }}</span>
            <span class="ss-row-s">{{ sub(m) }}</span>
          </span>
          <span class="ss-row-bar"
            ><span class="ss-fill" :style="{ width: `${stageProgress(m).pct}%` }"
          /></span>
          <span class="ss-row-c">{{ stageProgress(m).done }}/{{ stageProgress(m).total }}</span>
          <n-icon
            v-if="isOn({ key: m.id, stage: m })"
            :component="CheckmarkOutline"
            :size="15"
            class="ss-check"
          />
        </button>
      </template>

      <template v-if="planned.length">
        <div class="ss-h">
          <span>{{ t('board.stage.planned') }}</span
          ><span>{{ planned.length }}</span>
        </div>
        <button
          v-for="m in planned"
          :key="m.id"
          type="button"
          class="ss-row"
          :class="{ on: isOn({ key: m.id, stage: m }), cur: isCursor(m.id) }"
          data-testid="stage-option"
          @click="pick({ key: m.id, stage: m })"
          @mousemove="cursor = rows.findIndex((r) => r.key === m.id)"
        >
          <n-icon :component="TrailSignOutline" :size="16" class="ss-row-ic" />
          <span class="ss-row-t">
            <span class="ss-row-n">{{ m.title }}</span>
            <span class="ss-row-s">{{ sub(m) }}</span>
          </span>
          <span class="ss-row-bar"
            ><span class="ss-fill" :style="{ width: `${stageProgress(m).pct}%` }"
          /></span>
          <span class="ss-row-c">{{ stageProgress(m).done }}/{{ stageProgress(m).total }}</span>
        </button>
      </template>

      <button
        type="button"
        class="ss-row"
        :class="{ on: scope === 'backlog', cur: isCursor('backlog') }"
        data-testid="stage-option-backlog"
        @click="pick({ key: 'backlog' })"
        @mousemove="cursor = rows.findIndex((r) => r.key === 'backlog')"
      >
        <n-icon :component="FileTrayStackedOutline" :size="16" class="ss-row-ic" />
        <span class="ss-row-t">
          <span class="ss-row-n">{{ t('board.scope.backlog') }}</span>
          <span class="ss-row-s">{{ t('board.stage.backlogSub') }}</span>
        </span>
        <Kbd>{{ cap('b') }}</Kbd>
      </button>
      <button
        type="button"
        class="ss-row"
        :class="{ on: !scope, cur: isCursor('') }"
        data-testid="stage-option-all"
        @click="pick({ key: '' })"
        @mousemove="cursor = rows.findIndex((r) => r.key === '')"
      >
        <n-icon :component="AppsOutline" :size="16" class="ss-row-ic" />
        <span class="ss-row-t">
          <span class="ss-row-n">{{ t('board.stage.all') }}</span>
          <span class="ss-row-s">{{ t('board.stage.allSub') }}</span>
        </span>
        <Kbd>{{ cap('a') }}</Kbd>
      </button>

      <template v-if="closed.length">
        <button
          v-if="!q.trim()"
          type="button"
          class="ss-more"
          data-testid="stage-show-closed"
          @click="showClosed = !showClosed"
        >
          <n-icon :component="showClosed ? ChevronDownOutline : ChevronForwardOutline" :size="12" />
          {{ t('board.stage.showClosed') }}
          <span class="ss-grow" />
          <span class="ss-n">{{ closed.length }}</span>
        </button>
        <button
          v-for="m in closedShown"
          :key="m.id"
          type="button"
          class="ss-row closed"
          :class="{ on: isOn({ key: m.id, stage: m }), cur: isCursor(m.id) }"
          @click="pick({ key: m.id, stage: m })"
        >
          <n-icon :component="TrailSignOutline" :size="16" class="ss-row-ic" />
          <span class="ss-row-t">
            <span class="ss-row-n">{{ m.title }}</span>
            <span class="ss-row-s">{{ sub(m) }}</span>
          </span>
          <span class="ss-row-c">{{ stageProgress(m).done }}/{{ stageProgress(m).total }}</span>
        </button>
      </template>

      <button
        type="button"
        class="ss-more"
        data-testid="stage-manage"
        @click="((open = false), emit('manage'))"
      >
        <n-icon :component="SettingsOutline" :size="14" />
        {{ t('board.stage.manage') }}
      </button>
    </div>
  </Teleport>
</template>

<style scoped>
.ss {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  padding: 0 10px;
  border: none;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
}
.ss:hover:not(:disabled),
.ss.open {
  background: var(--t-hover);
}
.ss:disabled {
  opacity: 0.55;
  cursor: default;
}
.ss:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
/* A stage picked: the soft accent of an active control. */
.ss.on {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-ctl)),
    color-mix(in srgb, var(--t-primary) 26%, var(--t-ctl))
  );
}
.ss-ic {
  color: var(--t-text3);
}
.ss.on .ss-ic:first-child {
  color: var(--t-primary);
}
.ss-lbl {
  color: var(--t-text3);
}
.ss-val {
  font-weight: 600;
}
.ss-bar,
.ss-row-bar {
  position: relative;
  width: 28px;
  height: 4px;
  border-radius: 2px;
  background: color-mix(in srgb, var(--t-text3) 30%, transparent);
  overflow: hidden;
}
.ss-row-bar {
  width: 48px;
}
.ss-fill {
  position: absolute;
  inset: 0 auto 0 0;
  background: var(--t-accent-grad);
  border-radius: 2px;
}
.ss-pct {
  font-size: 12px;
  color: var(--t-text2);
  font-variant-numeric: tabular-nums;
}
.ss-menu {
  position: fixed;
  z-index: 2050;
  width: 340px;
  max-height: min(560px, 80vh);
  overflow-y: auto;
  padding: 8px;
  box-sizing: border-box;
  border-radius: 14px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
}
.ss-search {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  margin-bottom: 6px;
  padding: 0 8px 0 10px;
  border-radius: 9px;
  background: var(--t-input-bg);
  color: var(--t-text3);
}
.ss-search input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
}
.ss-h {
  display: flex;
  justify-content: space-between;
  padding: 10px 10px 4px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.ss-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  min-height: 44px;
  padding: 0 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.ss-row.cur {
  background: var(--t-hover);
}
.ss-row.on {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
}
.ss-row.closed {
  opacity: 0.66;
}
.ss-row-ic {
  color: var(--t-text3);
  flex: none;
}
.ss-row.on .ss-row-ic {
  color: var(--t-primary);
}
.ss-row-t {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.ss-row-n {
  font-size: 13.5px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ss-row-s {
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ss-row-c {
  font-size: 12.5px;
  color: var(--t-text2);
  font-variant-numeric: tabular-nums;
}
.ss-check {
  color: var(--t-primary);
}
.ss-more {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  height: 36px;
  padding: 0 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.ss-more:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.ss-grow {
  flex: 1;
}
.ss-n {
  color: var(--t-text3);
}
</style>
