<script setup>
import { NIcon, NTooltip } from 'naive-ui'
import { TimerOutline } from '@vicons/ionicons5'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad } from '@/utils/gradient'

// One task row: the sticky title cell plus the axis track carrying the estimate
// ghost, the task's bar and its scheduled subtasks as thin sub-bars. Geometry
// arrives precomputed in `row` (see useChartBars.renderRows).
//
// The two views differ only in what they hang off a bar: the Gantt adds the
// drag-to-link knob and the data-task-id drop targets (`linkable`), the Timeline
// listens for hover to raise its preview card. Everything else is identical.
defineProps({
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
})
defineEmits(['open', 'bar-down', 'link-down', 'menu', 'bar-enter', 'bar-leave'])
</script>

<template>
  <div
    class="tl-row chart-part"
    :class="[tier, { collapsed, animate }]"
    :data-task-id="linkable ? row.task.id : null"
  >
    <div
      class="tl-left"
      :style="{ width: `${leftW}px`, height: `${row.height}px` }"
      :title="row.task.title"
      @click="$emit('open', row.task.id)"
    >
      <span
        class="row-bar"
        :style="{ background: hueGrad(PRIORITY_COLORS[row.task.priority || 0]) }"
      />
      <span class="row-title" :class="{ done: row.task.completed_at }">{{ row.task.title }}</span>
    </div>
    <div class="tl-track" :style="{ width: `${axisW}px`, height: `${row.height}px` }">
      <div
        v-if="row.ghost"
        class="ghost"
        :style="{
          left: `${row.ghost.left}px`,
          width: `${row.ghost.width}px`,
          '--ghost-c': PRIORITY_COLORS[row.task.priority || 0],
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
        }"
        :style="{
          left: `${row.geom.left}px`,
          width: `${row.geom.width}px`,
          '--bar-grad': hueGrad(PRIORITY_COLORS[row.task.priority || 0]),
        }"
        @pointerdown="$emit('bar-down', $event, row.task, 'move')"
        @click="$emit('open', row.task.id)"
        @mouseenter="$emit('bar-enter', $event, row.task)"
        @mouseleave="$emit('bar-leave')"
        @contextmenu.prevent.stop="$emit('menu', $event, row.task)"
      >
        <span class="handle l" @pointerdown.stop="$emit('bar-down', $event, row.task, 'start')" />
        <span class="bar-title">{{ row.task.title }}</span>
        <span class="handle r" @pointerdown.stop="$emit('bar-down', $event, row.task, 'due')" />
        <span
          v-if="linkable"
          class="link-knob"
          title="Создать зависимость"
          @pointerdown="$emit('link-down', $event, row.task)"
          @click.stop
        />
      </div>
      <!-- subtask sub-bars stacked under the parent bar; draggable to reschedule
           (move / resize an edge) just like the parent, and — in the Gantt — link
           sources of their own, so subtasks can carry blocking dependencies too.
           A surface border separates same-priority children from the parent and
           from each other. -->
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
          '--bar-grad': hueGrad(PRIORITY_COLORS[s.task.priority || 0]),
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
          title="Создать зависимость"
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
/* top-align the title so it stays beside the parent bar when the row grows to fit
   subtask sub-bars (the height comes from an inline style) */
.tl-row .tl-left {
  align-items: flex-start;
  padding-top: 9px;
}
.row-bar {
  width: 3px;
  height: 18px;
  border-radius: 2px;
  flex: 0 0 auto;
}
.row-title {
  font-size: 13px;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.row-title.done {
  text-decoration: line-through;
  color: var(--t-text3);
}

/* glide bars on zoom; never while dragging/zooming (would lag) */
.chart-part.animate .bar,
.chart-part.animate .ghost {
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
  border: 2px dashed color-mix(in srgb, var(--ghost-c, var(--t-primary)) 65%, transparent);
  background: color-mix(in srgb, var(--ghost-c, var(--t-primary)) 12%, transparent);
  border-radius: 7px;
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

.bar {
  position: absolute;
  top: 6px;
  height: 24px;
  background: var(--bar-grad);
  border-radius: 6px;
  display: flex;
  align-items: center;
  padding: 0 8px;
  /* border-box so the rendered width equals the date-span px exactly — without it
     the 8px side padding pushed the visible right edge 16px past `geom.width`, and
     the Gantt's dependency arrows (anchored at geom.left+geom.width) started
     inside the bar. */
  box-sizing: border-box;
  cursor: grab;
  /* visible (not hidden) so the right-edge link knob isn't clipped; the title
     clamps itself via flex:1 + min-width:0 below. */
  overflow: visible;
  z-index: 2;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.18);
  user-select: none;
}
.bar:active {
  cursor: grabbing;
}
.bar.done {
  opacity: 0.55;
}
.bar.linksrc {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.bar-title {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: #fff;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  pointer-events: none;
}
.bar.done .bar-title {
  text-decoration: line-through;
}
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

/* subtask sub-bar: a thinner bar below the parent, priority-coloured. The
   surface-coloured border guarantees separation even when a child shares the
   parent's priority colour. Named tl-subbar to avoid the board composer's .subbar. */
.tl-subbar {
  position: absolute;
  height: 14px;
  background: var(--bar-grad);
  border: 1px solid var(--t-surface);
  border-radius: 5px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  padding: 0 6px;
  cursor: grab;
  overflow: visible;
  z-index: 2;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.16);
  user-select: none;
}
.tl-subbar:active {
  cursor: grabbing;
}
.tl-subbar.done {
  opacity: 0.5;
}
.tl-subbar-title {
  flex: 1;
  min-width: 0;
  font-size: 10px;
  color: #fff;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  pointer-events: none;
}
.tl-subbar.done .tl-subbar-title {
  text-decoration: line-through;
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
  background: var(--t-surface);
  border: 2px solid var(--t-primary);
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
