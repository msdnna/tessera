<script setup>
// «Вид доски» (v2, #2986 — mockups CustomPanel / PackageD): every setting of the
// board in one DockPanel docked right of the board under its sub-toolbar. The
// board gives up the width (columns, list, chart, matrix narrow), the top bar and
// the sidebar stay. On a phone it is a bottom sheet.
//
// An accordion of sections; a closed section keeps a summary of its settings
// («Средние · 8 полей»), so the board's state reads without opening anything.
// One section is open at a time; ⇧-click opens several. ↑/↓ walk the section
// headers, Space/↵ opens one. Sections that mean nothing in the current view are
// not drawn: cards and columns belong to the kanban (cards — to the matrix too),
// the scale to the timeline and the Gantt, the thresholds to the matrix.
//
// Everything here edits the board's own state (KanbanBoard binds it); the saved
// views are the first section, drawn by the board through the `views` slot.
import { computed, h, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NDropdown, NIcon, NInput } from 'naive-ui'
import {
  AddOutline,
  AlbumsOutline,
  BookmarksOutline,
  CalendarClearOutline,
  CalendarOutline,
  ChevronDownOutline,
  ChevronUpOutline,
  ColorPaletteOutline,
  DocumentTextOutline,
  FlagOutline,
  FunnelOutline,
  GitMergeOutline,
  GridOutline,
  KeypadOutline,
  PeopleOutline,
  PricetagOutline,
  TimeOutline,
  TrailSignOutline,
} from '@vicons/ionicons5'
import DockPanel from './kit/DockPanel.vue'
import ShortcutKeys from './ShortcutKeys.vue'
import IconColorPicker from './IconColorPicker.vue'
import { useResponsive } from '@/composables/useResponsive'
import { useFacetAddMenu } from '@/composables/useFacetAddMenu'
import { useFormat } from '@/composables/useFormat'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { priorityLabel } from '@/utils/priority'
import { softFill } from '@/utils/gradient'
import { stubTask } from '@/utils/featureStubs'
import { countActiveFilters } from '@/utils/facetKeys'

const show = defineModel('show', { type: Boolean, default: false })
const boardName = defineModel('boardName', { type: String, default: '' })
const cardSize = defineModel('cardSize', { type: String, default: 'medium' })
const stackFields = defineModel('stackFields', { type: Boolean, default: false })
const showEmpty = defineModel('showEmpty', { type: Boolean, default: true })
const autoCollapseEmpty = defineModel('autoCollapseEmpty', { type: Boolean, default: false })
const subtasksExpanded = defineModel('subtasksExpanded', { type: Boolean, default: false })

const props = defineProps({
  // The open view: board | list | timeline | gantt | calendar | matrix.
  layout: { type: String, default: 'board' },
  // Archive view (?archived=1): look, don't touch. Everything else this panel holds
  // is view state (card size, pills, auto-collapse, facets) and stays live there —
  // only the board's own branding is a write to shared data.
  readonly: { type: Boolean, default: false },
  // Per-field pill visibility (mutated via set-field so props stay read-only here).
  fieldVis: { type: Object, default: () => ({}) },
  // Reused composer facets (group/sort/filter) — rendered as chips + the same add
  // menu, so this panel doesn't reimplement the composer.
  facetChips: { type: Array, default: () => [] },
  addOptions: { type: Array, default: () => [] },
  filters: { type: Object, default: () => ({}) },
  // Label of the grouping the group chip steps to when clicked (the chip cycles —
  // see useBoardFacets). Shown as its tooltip, the same one the composer carries.
  groupNextLabel: { type: String, default: '' },
  // The grouping caption («Статус», «Теги»…) for the closed section's summary.
  groupLabel: { type: String, default: '' },
  // Board branding (icon/colour/mode), mirrors projects/groups via IconColorPicker.
  boardIcon: { type: String, default: '' },
  boardColor: { type: String, default: '' },
  boardIconMode: { type: String, default: 'badge' },
  // The open Gantt / timeline's controller (useChartView → ctl): the scale.
  chartCtl: { type: Object, default: null },
  // Closed «Представления»: «Мой спринт · изменено».
  viewsSummary: { type: String, default: '' },
  // Sections open when the panel is first drawn.
  defaultOpen: { type: Array, default: () => ['views'] },
  // Footer hint for the view digits (only when there are views to pick).
  digitsHint: { type: Boolean, default: false },
})
const emit = defineEmits(['set-field', 'add-facet', 'remove-chip', 'chip-click', 'update-board'])

const { t } = useI18n()
const { isMobile } = useResponsive()
const { formatDate } = useFormat()

// ── sections ──
const isChart = computed(() => props.layout === 'timeline' || props.layout === 'gantt')
const SECTION_ICONS = {
  views: BookmarksOutline,
  cards: AlbumsOutline,
  scale: CalendarOutline,
  facets: FunnelOutline,
  columns: GridOutline,
  board: ColorPaletteOutline,
}
const sections = computed(() => {
  const out = ['views']
  if (props.layout === 'board' || props.layout === 'matrix') out.push('cards')
  if (isChart.value) out.push('scale')
  out.push('facets')
  if (props.layout === 'board') out.push('columns')
  if (!props.readonly) out.push('board')
  return out
})
const open = ref(new Set(props.defaultOpen))
function toggle(key, e) {
  const next = new Set(open.value)
  if (e?.shiftKey) {
    if (next.has(key)) next.delete(key)
    else next.add(key)
  } else if (next.has(key) && next.size === 1) {
    next.clear()
  } else {
    next.clear()
    next.add(key)
  }
  open.value = next
}
const isOpen = (k) => open.value.has(k)
// The board asks for a section (⌘⇧S opens «Представления» with its field).
function reveal(key) {
  if (!open.value.has(key)) open.value = new Set([...open.value, key])
}
defineExpose({ reveal })

// ↑/↓ between section headers; the panel takes focus on open so the keys work.
const body = ref(null)
function heads() {
  return [...(body.value?.querySelectorAll('.cp-head') || [])]
}
function onKeydown(e) {
  if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return
  const list = heads()
  const i = list.indexOf(document.activeElement)
  if (i < 0) return
  e.preventDefault()
  const j = Math.min(list.length - 1, Math.max(0, i + (e.key === 'ArrowDown' ? 1 : -1)))
  list[j]?.focus()
}
watch(show, async (on) => {
  if (!on || isMobile.value) return
  await nextTick()
  // Not when the focus is already somewhere inside (a field asked for by ⌘⇧S).
  if (body.value?.contains(document.activeElement)) return
  heads()[0]?.focus({ preventScroll: true })
})

// ── summaries of closed sections ──
const FIELD_KEYS = [
  'priority',
  'due',
  'assignee',
  'tags',
  'estimate',
  'milestone',
  'description',
  'number',
  'gitlab',
]
const fieldOn = (k) => props.fieldVis?.[k] !== false
const fieldsOn = computed(() => FIELD_KEYS.filter(fieldOn).length)
const filtersOn = computed(() => countActiveFilters({ ...props.filters, q: '' }))
const summary = computed(() => ({
  views: props.viewsSummary,
  cards: t('board.customize.cardsSummary', fieldsOn.value, {
    named: { size: t(`board.customize.sizePlural.${cardSize.value}`), n: fieldsOn.value },
  }),
  scale: props.chartCtl ? t(`board.chart.scale.${props.chartCtl.scale}`) : '',
  facets: [
    props.groupLabel,
    filtersOn.value
      ? t('board.customize.filtersCount', filtersOn.value, { named: { n: filtersOn.value } })
      : '',
  ]
    .filter(Boolean)
    .join(' · '),
  columns: [
    autoCollapseEmpty.value ? t('board.customize.emptyCollapsed') : '',
    subtasksExpanded.value ? t('board.customize.subtasksOpen') : '',
  ]
    .filter(Boolean)
    .join(' · '),
  board: boardName.value,
}))

// ── cards ──
const SIZES = ['compact', 'medium', 'large']
// The pill each field adds to a card — what turning it on will show.
const PREVIEW_ICONS = {
  priority: FlagOutline,
  due: CalendarClearOutline,
  assignee: PeopleOutline,
  tags: PricetagOutline,
  estimate: TimeOutline,
  milestone: TrailSignOutline,
  description: DocumentTextOutline,
  number: KeypadOutline,
  gitlab: GitMergeOutline,
}
const HIGH = PRIORITY_COLORS[3]
const fields = computed(() =>
  FIELD_KEYS.map((key) => ({
    key,
    label: t(`board.customize.field.${key}`),
    icon: PREVIEW_ICONS[key],
    text:
      key === 'priority'
        ? priorityLabel(3)
        : key === 'due'
          ? formatDate(new Date(), { day: '2-digit', month: 'short' })
          : key === 'description'
            ? ''
            : t(`board.customize.preview.${key}`),
  })),
)
const highPill = { background: softFill(HIGH), color: HIGH }

// ── scale (timeline / Gantt) ──
const SCALES = ['days', 'weeks', 'months']
const CHART_STUBS = ['weekends', 'today', 'progress', 'links', 'undated']
const scaleStub = stubTask('chart.scale')

// ── facets: the composer's «+» menu behind the panel's own «+» ──
// On a phone naive's side submenus open past the edge of the screen, which is why
// the composer drills into one sub-list at a time — and this panel is the settings
// surface a phone can always reach. Own instance, own open/level state: sharing
// the composer's would pop both dropdowns at once.
const {
  show: addShow,
  menuOptions: addMenuOptions,
  renderLabel: renderAddLabel,
  onSelect: onAddSelect,
  onShow: onAddShow,
} = useFacetAddMenu({
  options: computed(() => props.addOptions),
  isMobile,
  onPick: (k) => emit('add-facet', k),
})

// ── board name: a draft, not the live model ──
// The same shape the sidebar's inline board rename has (ProjectRow: a buffer
// seeded on open, a commit guarded by `n === b.name`). Binding the model itself
// and emitting `update-board` on every blur AND every Enter is loose in three
// ways, all of them writes to data the whole workspace shares: Enter then a click
// away fired the same PUT twice; a no-op focus+blur PUT a full replace assembled
// from this client's copy (overwriting a colleague's icon change); and Esc closes
// the panel WITHOUT blurring the input, leaving the abandoned text in the model.
// The draft answers all three.
const boardInitials = computed(() => (boardName.value || '?').trim().slice(0, 2).toUpperCase())
const nameDraft = ref(boardName.value)
watch(boardName, (v) => (nameDraft.value = v))
watch(show, () => (nameDraft.value = boardName.value))
function commitName() {
  const n = nameDraft.value.trim()
  if (!n) {
    nameDraft.value = boardName.value
    return
  }
  if (n === boardName.value) return
  boardName.value = n
  emit('update-board', { name: n })
}

const hints = computed(() =>
  isMobile.value
    ? []
    : [
        { caps: ['↑↓'], text: t('board.customize.hintSection') },
        { caps: ['␣'], text: t('board.customize.hintOpen') },
        ...(props.digitsHint
          ? [{ spacer: true }, { caps: ['1–9'], text: t('board.customize.hintView') }]
          : []),
      ],
)
const subtitle = computed(() => t(`board.layout.${props.layout}`))
// The switch knob, drawn like ActionMenu's — Naive's NSwitch has no gradient track.
const sw = (on) =>
  h('span', { class: ['cp-switch', { on }], 'aria-hidden': 'true' }, [
    h('span', { class: 'cp-knob' }),
  ])
</script>

<template>
  <DockPanel
    v-model:show="show"
    :title="t('board.customize.title')"
    :subtitle="subtitle"
    toggle-key="view.panel"
    :hints="hints"
    testid="board-customize"
  >
    <div ref="body" class="cp" @keydown="onKeydown">
      <template v-for="key in sections" :key="key">
        <template v-if="key !== 'views' || $slots.views">
          <button
            type="button"
            class="cp-head"
            :class="{ open: isOpen(key) }"
            :aria-expanded="isOpen(key)"
            :data-testid="`customize-sec-${key}`"
            @click="(e) => toggle(key, e)"
          >
            <n-icon :component="SECTION_ICONS[key]" :size="16" class="cp-head-ic" />
            <span class="cp-head-title">{{ t(`board.customize.sec.${key}`) }}</span>
            <span class="cp-head-sum">{{ isOpen(key) ? '' : summary[key] }}</span>
            <n-icon
              :component="isOpen(key) ? ChevronUpOutline : ChevronDownOutline"
              :size="14"
              class="cp-head-chev"
            />
          </button>

          <div v-if="isOpen(key)" class="cp-body" :class="`cp-${key}`">
            <!-- Представления: the board draws it (it owns the saved views). -->
            <slot v-if="key === 'views'" name="views" />

            <template v-else-if="key === 'cards'">
              <div class="cp-lbl">
                <span>{{ t('board.customize.cardSize') }}</span>
                <ShortcutKeys v-if="!isMobile" id="view.cardSize" class="cp-lbl-key" />
              </div>
              <div class="cp-sizes">
                <button
                  v-for="s in SIZES"
                  :key="s"
                  type="button"
                  class="cp-size"
                  :class="{ on: cardSize === s }"
                  :data-testid="`card-size-${s}`"
                  @click="cardSize = s"
                >
                  <span class="cp-mini" :class="s">
                    <span class="m-title" />
                    <span v-if="s !== 'compact'" class="m-sub" />
                    <span v-if="s !== 'compact'" class="m-pills">
                      <span v-for="n in s === 'large' ? 3 : 2" :key="n" class="m-pill" />
                    </span>
                  </span>
                  <span class="cp-size-lbl">{{ t(`board.customize.size.${s}`) }}</span>
                </button>
              </div>
              <div class="cp-lbl cp-lbl-row">
                <span>{{ t('board.customize.fields') }}</span>
                <span class="cp-lbl-aside">{{
                  t('board.customize.fieldsCount', { on: fieldsOn, all: FIELD_KEYS.length })
                }}</span>
              </div>
              <div class="cp-fields">
                <button
                  v-for="f in fields"
                  :key="f.key"
                  type="button"
                  role="switch"
                  class="cp-row cp-field"
                  :class="{ off: !fieldOn(f.key) }"
                  :aria-checked="fieldOn(f.key)"
                  :data-testid="`field-${f.key}`"
                  @click="emit('set-field', f.key, !fieldOn(f.key))"
                >
                  <span class="cp-row-lbl">{{ f.label }}</span>
                  <span
                    class="cp-pill"
                    :style="f.key === 'priority' ? highPill : undefined"
                    aria-hidden="true"
                  >
                    <n-icon :component="f.icon" :size="12" />
                    <span v-if="f.text">{{ f.text }}</span>
                  </span>
                  <component :is="sw(fieldOn(f.key))" />
                </button>
              </div>
              <button
                type="button"
                role="switch"
                class="cp-row cp-well"
                :aria-checked="showEmpty"
                data-testid="cards-show-empty"
                @click="showEmpty = !showEmpty"
              >
                <span class="cp-row-lbl">{{ t('board.customize.showEmpty') }}</span>
                <span class="cp-row-note">{{ t('board.customize.showEmptyNote') }}</span>
                <component :is="sw(showEmpty)" />
              </button>
              <button
                type="button"
                role="switch"
                class="cp-row cp-well"
                :aria-checked="stackFields"
                data-testid="cards-stack"
                @click="stackFields = !stackFields"
              >
                <span class="cp-row-lbl">{{ t('board.customize.stack') }}</span>
                <span class="cp-row-note">{{ t('board.customize.stackNote') }}</span>
                <component :is="sw(stackFields)" />
              </button>
              <button
                v-if="layout === 'matrix'"
                type="button"
                role="switch"
                class="cp-row cp-well"
                :aria-checked="subtasksExpanded"
                @click="subtasksExpanded = !subtasksExpanded"
              >
                <span class="cp-row-lbl">{{ t('board.customize.subtasks') }}</span>
                <component :is="sw(subtasksExpanded)" />
              </button>
            </template>

            <template v-else-if="key === 'scale'">
              <div class="cp-seg" role="tablist">
                <button
                  v-for="s in SCALES"
                  :key="s"
                  type="button"
                  class="cp-seg-btn"
                  :class="{ on: chartCtl?.scale === s }"
                  :disabled="!chartCtl"
                  @click="chartCtl?.setScale(s)"
                >
                  {{ t(`board.chart.scale.${s}`) }}
                </button>
                <button
                  type="button"
                  class="cp-seg-btn stub"
                  aria-disabled="true"
                  :title="t('common.stub.soon', { n: scaleStub })"
                >
                  {{ t('board.chart.scale.quarters') }}
                </button>
              </div>
              <button
                v-if="chartCtl"
                type="button"
                role="switch"
                class="cp-row"
                :aria-checked="!chartCtl.leftCollapsed"
                @click="chartCtl.toggleLeft()"
              >
                <span class="cp-row-lbl">{{ t('board.chart.showTasks') }}</span>
                <component :is="sw(!chartCtl.leftCollapsed)" />
              </button>
              <div
                v-for="k in CHART_STUBS"
                :key="k"
                class="cp-row stub"
                aria-disabled="true"
                :title="t('common.stub.soon', { n: scaleStub })"
              >
                <span class="cp-row-lbl">{{ t(`board.customize.chart.${k}`) }}</span>
                <span class="cp-soon">{{ t('common.stub.badge') }}</span>
                <component :is="sw(false)" />
              </div>
              <div v-if="chartCtl?.counters?.length" class="cp-counters">
                <span
                  v-for="c in chartCtl.counters"
                  :key="c.key"
                  class="cp-counter"
                  :class="{ overdue: c.overdue }"
                  >{{ c.text }}</span
                >
              </div>
              <div v-if="chartCtl?.hint && !isMobile" class="cp-note">{{ chartCtl.hint }}</div>
              <div class="cp-note">{{ t('board.customize.chartNote') }}</div>
            </template>

            <template v-else-if="key === 'facets'">
              <div class="cp-chips">
                <!-- The group chip does the same thing here as in the composer — one
                     click steps the grouping on — so it gets the same tooltip. -->
                <span
                  v-for="(c, ci) in facetChips"
                  :key="ci"
                  class="cp-chip"
                  :class="{ group: c.kind === 'group' }"
                  :title="
                    c.kind === 'group' && groupNextLabel
                      ? t('board.composer.groupNext', { next: groupNextLabel })
                      : undefined
                  "
                  @click="emit('chip-click', c)"
                >
                  {{ c.label }}
                  <button
                    v-if="c.kind !== 'group'"
                    type="button"
                    class="cp-chip-x"
                    :title="t('board.composer.remove')"
                    @click.stop="emit('remove-chip', c)"
                  >
                    ×
                  </button>
                </span>
                <n-dropdown
                  :show="addShow"
                  trigger="click"
                  placement="bottom-start"
                  scrollable
                  :options="addMenuOptions"
                  :render-label="renderAddLabel"
                  @update:show="onAddShow"
                  @select="onAddSelect"
                >
                  <button type="button" class="cp-chip-add" :title="t('board.composer.add')">
                    <n-icon :component="AddOutline" :size="14" />
                  </button>
                </n-dropdown>
              </div>
            </template>

            <template v-else-if="key === 'columns'">
              <button
                type="button"
                role="switch"
                class="cp-row"
                :aria-checked="autoCollapseEmpty"
                data-testid="columns-auto-collapse"
                @click="autoCollapseEmpty = !autoCollapseEmpty"
              >
                <span class="cp-row-lbl">{{ t('board.customize.autoCollapse') }}</span>
                <component :is="sw(autoCollapseEmpty)" />
              </button>
              <button
                type="button"
                role="switch"
                class="cp-row"
                :aria-checked="subtasksExpanded"
                @click="subtasksExpanded = !subtasksExpanded"
              >
                <span class="cp-row-lbl">{{ t('board.customize.subtasks') }}</span>
                <component :is="sw(subtasksExpanded)" />
              </button>
            </template>

            <!-- Board: name, then icon + colour. Not drawn in the archive: the
                 rest of this panel is the reader's own view state, but the
                 name/icon/colour are the LIVE board's — `update-board` PUTs them
                 for the whole workspace. -->
            <template v-else-if="key === 'board'">
              <n-input
                v-model:value="nameDraft"
                size="small"
                :placeholder="t('board.customize.boardName')"
                data-testid="board-name-input"
                @blur="commitName"
                @keyup.enter="commitName"
              />
              <div class="board-icp">
                <IconColorPicker
                  :icon="boardIcon"
                  :color="boardColor"
                  :mode="boardIconMode"
                  :initials="boardInitials"
                  fallback-kanban
                  allow-upload
                  @update:icon="(v) => emit('update-board', { icon: v })"
                  @update:color="(v) => emit('update-board', { color: v })"
                  @update:mode="(v) => emit('update-board', { icon_mode: v })"
                />
              </div>
            </template>
          </div>
        </template>
      </template>
      <div v-if="isChart" class="cp-note cp-foot-note">
        {{ t('board.customize.chartHidden') }}
      </div>
    </div>
  </DockPanel>
</template>

<style scoped>
.cp {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.cp-head {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 40px;
  padding: 0 10px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  text-align: left;
  cursor: pointer;
  flex: none;
}
.cp-head:hover {
  background: var(--t-hover);
}
.cp-head:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: -2px;
}
.cp-head-ic {
  color: var(--t-text2);
}
.cp-head.open .cp-head-ic {
  color: var(--t-primary);
}
.cp-head-title {
  font-size: 13.5px;
  font-weight: 600;
  white-space: nowrap;
}
.cp-head-sum {
  flex: 1;
  min-width: 0;
  text-align: right;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cp-head-chev {
  color: var(--t-text3);
}
.cp-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 2px 8px 10px;
}
.cp-body.cp-views {
  padding: 0 4px 8px;
  gap: 2px;
}
.cp-lbl {
  display: flex;
  gap: 4px;
  padding: 6px 2px 2px;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
}
.cp-lbl-row {
  padding-top: 10px;
}
.cp-lbl-aside {
  margin-left: auto;
  font-weight: 500;
}
.cp-lbl-key {
  margin-left: 4px;
}
/* card size tiles with a mini card */
.cp-sizes {
  display: flex;
  gap: 6px;
}
.cp-size {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 8px 6px;
  border: 0;
  border-radius: 10px;
  background: var(--t-input-bg);
  color: var(--t-text2);
  font: inherit;
  cursor: pointer;
}
.cp-size.on {
  background: var(--t-card);
  box-shadow:
    var(--t-sh-card),
    0 0 0 2px var(--t-primary);
  color: var(--t-text1);
}
.cp-size:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.cp-mini {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 6px;
  box-sizing: border-box;
  border-radius: 6px;
  background: var(--t-pop);
}
.cp-size.on .cp-mini {
  background: var(--t-input-bg);
}
.m-title {
  width: 80%;
  height: 4px;
  border-radius: 2px;
  background: var(--t-text2);
}
.m-sub {
  width: 40%;
  height: 3px;
  border-radius: 2px;
  background: var(--t-text3);
}
.m-pills {
  display: flex;
  gap: 2px;
  margin-top: 2px;
}
.m-pill {
  width: 12px;
  height: 5px;
  border-radius: 3px;
  background: var(--t-pill);
}
.cp-size-lbl {
  font-size: 12px;
}
.cp-size.on .cp-size-lbl {
  font-weight: 600;
}
/* rows with a switch */
.cp-fields {
  display: flex;
  flex-direction: column;
  margin: 0 -2px 4px;
}
.cp-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 34px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.cp-row:hover {
  background: var(--t-hover);
}
.cp-row:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: -2px;
}
.cp-row.cp-well {
  padding: 0 8px;
  background: var(--t-input-bg);
}
.cp-row.stub {
  cursor: not-allowed;
  color: var(--t-text3);
}
.cp-row.stub:hover {
  background: transparent;
}
.cp-row-lbl {
  flex: 1 0 auto;
  font-size: 13px;
  white-space: nowrap;
}
.cp-field.off .cp-row-lbl {
  color: var(--t-text2);
}
.cp-row-note {
  min-width: 0;
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cp-soon {
  font-size: 11.5px;
  color: var(--t-text3);
}
.cp-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 20px;
  padding: 0 7px 0 5px;
  border-radius: var(--t-r-pill);
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 11px;
  white-space: nowrap;
}
.cp-field.off .cp-pill {
  opacity: 0.4;
}
:deep(.cp-switch) {
  flex: 0 0 30px;
  display: inline-flex;
  align-items: center;
  width: 30px;
  height: 18px;
  padding: 0 2px;
  box-sizing: border-box;
  border-radius: 9px;
  background: var(--t-pill);
  box-shadow: inset 0 1px 2px rgba(0, 0, 0, 0.2);
}
:deep(.cp-knob) {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--t-text3);
  transition: transform 0.15s ease;
}
:deep(.cp-switch.on) {
  background: var(--t-accent-grad);
  box-shadow: none;
}
:deep(.cp-switch.on .cp-knob) {
  transform: translateX(12px);
  background: #fff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
}
/* scale */
.cp-seg {
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: 9px;
  background: var(--t-input-bg);
}
.cp-seg-btn {
  flex: 1;
  height: 28px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
  white-space: nowrap;
}
.cp-seg-btn.on {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.cp-seg-btn.stub {
  color: var(--t-placeholder);
  cursor: not-allowed;
}
.cp-counters {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 6px 2px 0;
}
.cp-counter {
  height: 22px;
  padding: 0 9px;
  border-radius: 11px;
  display: inline-flex;
  align-items: center;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 12px;
  white-space: nowrap;
}
.cp-counter.overdue {
  background: color-mix(in srgb, #e0533d 16%, var(--t-pop));
  color: #e0533d;
}
.cp-note {
  padding: 4px 2px 0;
  font-size: 12px;
  line-height: 1.45;
  color: var(--t-text3);
}
.cp-foot-note {
  margin: 8px 10px 0;
  padding: 0;
}
/* facets */
.cp-chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding: 2px 0;
}
.cp-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 24px;
  padding: 0 4px 0 9px;
  border-radius: var(--t-r-pill);
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 14%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 24%, var(--t-pop))
  );
  color: var(--t-text1);
  font-size: 12.5px;
  cursor: pointer;
  white-space: nowrap;
}
.cp-chip.group {
  padding-right: 9px;
  background: var(--t-input-bg);
}
.cp-chip-x {
  border: 0;
  padding: 0 4px;
  background: none;
  color: var(--t-text3);
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
}
.cp-chip-add {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: 0;
  border-radius: 12px;
  background: var(--t-pill);
  color: var(--t-text2);
  cursor: pointer;
}
.cp-chip-add:hover {
  color: var(--t-primary);
}
/* Board icon/colour picker: left-align its rows (the shared component centres by
   default, which reads oddly in this left-aligned settings panel). */
.board-icp {
  padding-top: 6px;
}
.board-icp :deep(.icons),
.board-icp :deep(.swatches),
.board-icp :deep(.mode-toggle) {
  justify-content: flex-start;
  align-self: flex-start;
}
</style>
