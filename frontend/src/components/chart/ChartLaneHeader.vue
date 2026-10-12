<script setup>
import { useI18n } from 'vue-i18n'
import { NIcon, NTooltip } from 'naive-ui'
import { ChevronDownOutline, TimerOutline } from '@vicons/ionicons5'
import { hueGrad } from '@/utils/gradient'

const { t } = useI18n()

// Group header row (v2, #2959): a chevron that folds the group, its colour
// square, name and «X из N» on the left; across the axis, the group's summary bar
// — from its earliest start to its latest end, filled by the share of done tasks.
// Identical in the Timeline and Gantt views.
defineProps({
  lane: { type: Object, required: true },
  leftW: { type: Number, required: true },
  axisW: { type: Number, required: true },
  tier: { type: String, required: true },
  collapsed: { type: Boolean, default: false },
  // The group is folded to this header.
  folded: { type: Boolean, default: false },
  // { left, width, pct } of the summary bar, or null when nothing has dates.
  summary: { type: Object, default: null },
  // Short and full renderings of the lane's summed estimate ('' hides the chip).
  effort: { type: String, default: '' },
  effortFull: { type: String, default: '' },
})
defineEmits(['toggle'])
</script>

<template>
  <div class="tl-lanehead chart-part" :class="[tier, { collapsed }]">
    <div
      class="tl-left lane"
      :style="{ width: `${leftW}px` }"
      :title="t('board.chart.lanes')"
      @click="$emit('toggle', lane.key)"
    >
      <n-icon :component="ChevronDownOutline" :size="13" class="lane-chev" :class="{ folded }" />
      <span
        class="lane-dot"
        :style="{ background: lane.color ? hueGrad(lane.color) : 'var(--t-accent-grad)' }"
      />
      <!-- `title` is not decoration: the left column is a fixed width and the
           caption below is clipped to one line, while a tag or status lane is
           named by whatever the reader called the tag or the column (#2932). -->
      <span class="lane-name" :title="lane.label">{{ lane.label }}</span>
      <span class="lane-count">{{
        t('task.subtasks.progress', { done: lane.done || 0, total: lane.tasks.length })
      }}</span>
      <n-tooltip v-if="effort">
        <template #trigger>
          <span class="lane-effort"
            ><n-icon :component="TimerOutline" :size="12" /> {{ effort }}</span
          >
        </template>
        {{ t('board.chart.effortTotal', { value: effortFull }) }}
      </n-tooltip>
    </div>
    <div class="tl-track laneband" :style="{ width: `${axisW}px` }">
      <div
        v-if="summary"
        class="lane-sum"
        :style="{
          left: `${summary.left}px`,
          width: `${summary.width}px`,
          '--lc': lane.color || 'var(--t-primary)',
        }"
      >
        <span
          class="lane-sum-fill"
          :style="{ width: `${summary.pct * 100}%`, background: hueGrad(lane.color || '#7c5cff') }"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
@import './chart-parts.css';

.tl-lanehead {
  display: flex;
  /* the gap above a group is padding, not margin: the row layout measures the
     header's offsetHeight, which a margin would slip past */
  padding-top: 6px;
}
.tl-left.lane {
  z-index: 5;
  gap: 8px;
  height: 36px;
  padding-left: 10px;
  border-radius: 9px 0 0 9px;
}
.lane-chev {
  flex: none;
  color: var(--t-text3);
  transition: transform 0.15s ease;
}
.lane-chev.folded {
  transform: rotate(-90deg);
}
.lane-dot {
  width: 10px;
  height: 10px;
  border-radius: 3px;
  flex: 0 0 auto;
}
.lane-name {
  font-size: 13.5px;
  font-weight: 650;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.lane-count {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.lane-effort {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  margin-left: auto;
  font-size: 11px;
  color: var(--t-text3);
  white-space: nowrap;
}
.laneband {
  height: 36px;
}
/* summary bar: the group's span, its done share filled */
.lane-sum {
  position: absolute;
  top: 14px;
  height: 8px;
  border-radius: 4px;
  overflow: hidden;
  background: color-mix(in srgb, var(--lc) 26%, transparent);
}
.lane-sum-fill {
  display: block;
  height: 100%;
  border-radius: 4px;
}
</style>
