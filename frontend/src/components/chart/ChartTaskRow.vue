<script setup>
import { useI18n } from 'vue-i18n'
import { NIcon, NTooltip } from 'naive-ui'
import { TimerOutline, CheckmarkOutline, PersonAddOutline } from '@vicons/ionicons5'
import TesseraIcon from '../TesseraIcon.vue'
import UserAvatar from '../UserAvatar.vue'
import ShortcutKeys from '../ShortcutKeys.vue'

const { t } = useI18n()

// One task row (v2, #2959): the sticky cell — status glyph, #number, title, the
// people — plus the axis track carrying the estimate ghost, the bar and the
// scheduled subtasks as thin sub-bars. Geometry and meta arrive precomputed in
// `row` (useChartBars.renderRows + useChartView.viewRows).
//
// The bar takes its column's colour as a soft same-hue gradient, a 3px line
// inside it shows the subtask progress, and an overdue bar turns red with a
// «просрочено на N дн.» pill. A click selects the bar (handles + the date
// tooltip with ⇧←/⇧→), a double click or ↵ opens the task.
//
// The two views differ only in what they hang off a bar: the Gantt adds the
// drag-to-link knob and the data-task-id drop targets (`linkable`), the Timeline
// listens for hover to raise its preview card.
const props = defineProps({
  row: { type: Object, required: true },
  leftW: { type: Number, required: true },
  axisW: { type: Number, required: true },
  tier: { type: String, required: true },
  collapsed: { type: Boolean, default: false },
  // Bars glide on zoom; suppressed mid-drag, where a transition would lag.
  animate: { type: Boolean, default: false },
  // Gantt: render the link knob and mark rows/sub-bars as dependency drop targets.
  linkable: { type: Boolean, default: false },
  // id of the bar a link is currently being dragged from, if any.
  linkFromId: { type: String, default: null },
  selected: { type: Boolean, default: false },
  // { range, days } of the selected bar, for its tooltip.
  selLabel: { type: Object, default: null },
})
defineEmits(['open', 'select', 'bar-down', 'link-down', 'menu', 'bar-enter', 'bar-leave'])

const OVERDUE = '#e0533d'
const tone = () => (props.row.overdue ? OVERDUE : props.row.status?.color || 'var(--t-primary)')
// Does the title fit inside the bar? A width estimate is enough to decide — the
// label clips with an ellipsis anyway if it is a few pixels off.
const fitsInside = () =>
  props.row.geom.width >=
  props.row.task.title.length * 6.6 + 22 + (props.row.task.completed_at ? 16 : 0)
</script>

<template>
  <div
    class="tl-row chart-part"
    :class="[tier, { collapsed, animate, selected, slim: leftW < 220 }]"
    :data-task-id="linkable ? row.task.id : null"
  >
    <div
      class="tl-left"
      :style="{ width: `${leftW}px`, height: `${row.height}px` }"
      :title="row.task.title"
      @click="$emit('open', row.task.id)"
    >
      <TesseraIcon
        :name="row.status?.icon || 'status-todo'"
        :size="14"
        class="row-st"
        :style="{ color: row.status?.color || 'var(--t-text3)' }"
      />
      <span class="row-num">{{ row.task.number != null ? `#${row.task.number}` : '' }}</span>
      <span class="row-title" :class="{ done: row.task.completed_at }">{{ row.task.title }}</span>
      <span v-if="row.people?.length" class="row-people">
        <UserAvatar
          v-for="m in row.people"
          :key="m.user_id"
          class="row-ava"
          :user-id="m.user_id"
          :name="m.name"
          :title="m.name"
        />
      </span>
      <n-icon v-else :component="PersonAddOutline" :size="13" class="row-noava" />
    </div>
    <div class="tl-track" :style="{ width: `${axisW}px`, height: `${row.height}px` }">
      <div
        v-if="row.ghost"
        class="ghost"
        :style="{
          left: `${row.ghost.left}px`,
          width: `${row.ghost.width}px`,
          '--ghost-c': tone(),
        }"
      >
        <n-tooltip>
          <template #trigger>
            <span class="ghost-est"
              ><n-icon :component="TimerOutline" :size="11" /> {{ row.ghostEst }}</span
            >
          </template>
          {{ row.ghostTitle }}
        </n-tooltip>
      </div>
      <div
        class="bar"
        :class="{
          done: row.task.completed_at,
          point: !(row.geom.hasStart && row.geom.hasDue),
          linksrc: linkFromId === row.task.id,
          sel: selected,
        }"
        :style="{
          left: `${row.geom.left}px`,
          width: `${row.geom.width}px`,
          '--c': tone(),
        }"
        @pointerdown="$emit('bar-down', $event, row.task, 'move')"
        @click="$emit('select', row.task.id)"
        @dblclick="$emit('open', row.task.id)"
        @mouseenter="$emit('bar-enter', $event, row.task)"
        @mouseleave="$emit('bar-leave')"
        @contextmenu.prevent.stop="$emit('menu', $event, row.task)"
      >
        <span class="handle l" @pointerdown.stop="$emit('bar-down', $event, row.task, 'start')" />
        <template v-if="fitsInside()">
          <n-icon
            v-if="row.task.completed_at"
            :component="CheckmarkOutline"
            :size="12"
            class="bar-check"
          />
          <span class="bar-title">{{ row.task.title }}</span>
        </template>
        <span v-if="row.progress != null" class="bar-prog">
          <span :style="{ width: `${row.progress * 100}%` }" />
        </span>
        <span class="handle r" @pointerdown.stop="$emit('bar-down', $event, row.task, 'due')" />
        <span
          v-if="linkable"
          class="link-knob"
          :title="t('board.chart.linkCreate')"
          @pointerdown="$emit('link-down', $event, row.task)"
          @click.stop
        />
      </div>
      <!-- what does not fit inside goes after the bar: the title, the overdue
           pill, and — on the selected bar — its dates and keys -->
      <div class="bar-after" :style="{ left: `${row.geom.left + row.geom.width + 8}px` }">
        <span v-if="selected && selLabel" class="bar-tip">
          <b>{{ selLabel.range }}</b>
          <span class="tip-days">{{ selLabel.days }}</span>
          <ShortcutKeys id="chart.shiftLeft" soft />
          <ShortcutKeys id="chart.shiftRight" soft />
        </span>
        <template v-else>
          <span v-if="!fitsInside()" class="after-title">{{ row.task.title }}</span>
          <span v-if="row.overdue" class="overdue-pill">{{ row.overdueText }}</span>
        </template>
      </div>
      <!-- subtask sub-bars stacked under the parent bar; draggable to reschedule
           (move / resize an edge) just like the parent, and — in the Gantt — link
           sources of their own, so subtasks can carry blocking dependencies too. -->
      <div
        v-for="s in row.subs"
        :key="s.task.id"
        class="tl-subbar"
        :class="{
          done: s.task.completed_at,
          point: !(s.geom.hasStart && s.geom.hasDue),
          linksrc: linkFromId === s.task.id,
        }"
        :data-task-id="linkable ? s.task.id : null"
        :style="{
          left: `${s.geom.left}px`,
          width: `${s.geom.width}px`,
          top: `${s.top}px`,
          '--c': tone(),
        }"
        @pointerdown="$emit('bar-down', $event, s.task, 'move')"
        @click="$emit('open', s.task.id)"
        @mouseenter="$emit('bar-enter', $event, s.task)"
        @mouseleave="$emit('bar-leave')"
        @contextmenu.prevent.stop="$emit('menu', $event, s.task)"
      >
        <span class="handle l" @pointerdown.stop="$emit('bar-down', $event, s.task, 'start')" />
        <span class="tl-subbar-title">{{ s.task.title }}</span>
        <span class="handle r" @pointerdown.stop="$emit('bar-down', $event, s.task, 'due')" />
        <span
          v-if="linkable"
          class="link-knob"
          :title="t('board.chart.linkCreate')"
          @pointerdown="$emit('link-down', $event, s.task)"
          @click.stop
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
@import './chart-parts.css';

.tl-row {
  display: flex;
}
/* top-align the cell so it stays beside the parent bar when the row grows to fit
   subtask sub-bars (the height comes from an inline style) */
.tl-row .tl-left {
  align-items: flex-start;
  padding: 0 12px 0 30px;
  line-height: 36px;
}
.chart-part.collapsed .tl-left {
  padding: 0;
}
/* the slim (phone) column: status and title only */
.chart-part.slim .tl-left {
  padding-left: 12px;
}
.chart-part.slim .row-num,
.chart-part.slim .row-people,
.chart-part.slim .row-noava {
  display: none;
}
.row-st {
  flex: none;
  align-self: center;
  margin-top: 0;
  height: 36px;
}
.row-num {
  flex: 0 0 40px;
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.row-title {
  flex: 1;
  min-width: 0;
  font-size: 13.5px;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.row-title.done {
  color: var(--t-text3);
}
.row-people {
  flex: none;
  display: inline-flex;
  align-items: center;
  height: 36px;
}
.row-ava {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  margin-left: -5px;
  box-shadow: 0 0 0 2px var(--t-bg);
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 8.5px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
}
.row-ava:first-child {
  margin-left: 0;
}
.row-noava {
  flex: none;
  align-self: center;
  color: var(--t-text3);
  opacity: 0.7;
}

/* glide bars on zoom; never while dragging/zooming (would lag) */
.chart-part.animate .bar,
.chart-part.animate .ghost,
.chart-part.animate .bar-after {
  transition:
    left 0.18s ease,
    width 0.18s ease;
}

/* ghost estimate envelope: dashed bar behind the real bar, sized to the estimate */
.ghost {
  position: absolute;
  top: 3px;
  height: 30px;
  box-sizing: border-box;
  border: 2px dashed color-mix(in srgb, var(--ghost-c, var(--t-primary)) 55%, transparent);
  background: color-mix(in srgb, var(--ghost-c, var(--t-primary)) 8%, transparent);
  border-radius: 9px;
  z-index: 1;
  pointer-events: none;
}
.ghost-est {
  position: absolute;
  right: 6px;
  top: 50%;
  transform: translateY(-50%);
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 10px;
  font-weight: 600;
  white-space: nowrap;
  color: color-mix(in srgb, var(--ghost-c, var(--t-primary)) 80%, var(--t-text1));
  /* the envelope is pointer-events:none (so the bar stays draggable); re-enable
     events on the label itself so its estimate tooltip can trigger */
  pointer-events: auto;
}

/* the bar: the column's colour as a soft same-hue gradient over the canvas */
.bar {
  position: absolute;
  top: 6px;
  height: 24px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 0 8px 0 10px;
  border-radius: 7px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--c) 20%, var(--t-bg)),
    color-mix(in srgb, var(--c) 30%, var(--t-bg))
  );
  cursor: grab;
  /* visible (not hidden) so the right-edge link knob isn't clipped; the title
     clamps itself via flex:1 + min-width:0 below. */
  overflow: visible;
  z-index: 2;
  user-select: none;
  transition: box-shadow 0.12s ease;
}
.bar:hover {
  box-shadow: var(--t-sh-hover);
}
.bar:active {
  cursor: grabbing;
}
.bar.linksrc {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.bar.sel {
  box-shadow:
    0 0 0 2px var(--t-primary),
    var(--t-sh-hover);
}
.bar-check {
  flex: none;
  color: #18a058;
}
.bar-title {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  font-weight: 500;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  pointer-events: none;
}
/* subtask progress: a 3px line along the bottom of the bar */
.bar-prog {
  position: absolute;
  left: 3px;
  right: 3px;
  bottom: 3px;
  height: 3px;
  border-radius: 2px;
  background: color-mix(in srgb, var(--c) 18%, transparent);
  overflow: hidden;
  pointer-events: none;
}
.bar-prog > span {
  display: block;
  height: 100%;
  border-radius: 2px;
  background: linear-gradient(
    to right,
    color-mix(in srgb, var(--c) 86%, #000),
    var(--c) 50%,
    color-mix(in srgb, var(--c) 86%, #fff)
  );
}
/* resize zones on the edges; the selected bar shows them as grips */
.handle {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 8px;
  cursor: ew-resize;
}
.handle.l {
  left: 0;
}
.handle.r {
  right: 0;
}
.bar.sel .handle::after {
  content: '';
  position: absolute;
  top: 6px;
  width: 3px;
  height: 12px;
  border-radius: 2px;
  background: var(--t-text1);
  opacity: 0.6;
}
.bar.sel .handle.l::after {
  left: 3px;
}
.bar.sel .handle.r::after {
  right: 3px;
}

/* after the bar: the title that did not fit, the overdue pill, the tooltip */
.bar-after {
  position: absolute;
  top: 0;
  height: 36px;
  display: flex;
  align-items: center;
  gap: 8px;
  z-index: 3;
  pointer-events: none;
  white-space: nowrap;
}
.after-title {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 12px;
  color: var(--t-text2);
}
.overdue-pill {
  height: 20px;
  padding: 0 8px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  background: color-mix(in srgb, #e0533d 18%, var(--t-bg));
  color: #e0533d;
  font-size: 11.5px;
  font-weight: 600;
}
.bar-tip {
  height: 26px;
  padding: 0 6px 0 10px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  font-size: 12px;
  color: var(--t-text1);
}
.bar-tip b {
  font-weight: 600;
}
.tip-days {
  color: var(--t-text3);
}

/* subtask sub-bar: a thinner bar below the parent in the same tone. Named
   tl-subbar to avoid the board composer's .subbar. */
.tl-subbar {
  position: absolute;
  height: 14px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  padding: 0 6px;
  border-radius: 5px;
  background: color-mix(in srgb, var(--c) 18%, var(--t-bg));
  cursor: grab;
  overflow: visible;
  z-index: 2;
  user-select: none;
}
.tl-subbar:active {
  cursor: grabbing;
}
.tl-subbar.done {
  opacity: 0.55;
}
.tl-subbar-title {
  flex: 1;
  min-width: 0;
  font-size: 10px;
  color: var(--t-text2);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  pointer-events: none;
}

/* drag-to-link knob on the bar's right edge (reveal on hover) */
.link-knob {
  position: absolute;
  right: -7px;
  top: 50%;
  transform: translateY(-50%);
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--t-pop);
  box-shadow: 0 0 0 2px var(--t-primary);
  cursor: crosshair;
  opacity: 0;
  transition: opacity 0.12s ease;
  z-index: 3;
}
.bar:hover .link-knob,
.bar.linksrc .link-knob,
.tl-subbar:hover .link-knob,
.tl-subbar.linksrc .link-knob {
  opacity: 1;
}
</style>
