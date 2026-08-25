<script setup>
import { useI18n } from 'vue-i18n'
import { NIcon, NTooltip } from 'naive-ui'
import { TimerOutline } from '@vicons/ionicons5'
import { hueGrad } from '@/utils/gradient'

const { t } = useI18n()

// Swimlane header row: the lane's colour dot, name, task count and summed estimate
// on the left, an empty band across the axis on the right. Identical in the
// Timeline and Gantt views.
defineProps({
  lane: { type: Object, required: true },
  leftW: { type: Number, required: true },
  axisW: { type: Number, required: true },
  tier: { type: String, required: true },
  collapsed: { type: Boolean, default: false },
  // Short and full renderings of the lane's summed estimate ('' hides the chip).
  effort: { type: String, default: '' },
  effortFull: { type: String, default: '' },
})
</script>

<template>
  <div class="tl-lanehead chart-part" :class="[tier, { collapsed }]">
    <div class="tl-left lane" :style="{ width: `${leftW}px` }">
      <span
        class="lane-dot"
        :style="{ background: lane.color ? hueGrad(lane.color) : 'var(--t-accent-grad)' }"
      />
      <span class="lane-name">{{ lane.label }}</span>
      <span class="lane-count">{{ lane.tasks.length }}</span>
      <n-tooltip v-if="effort">
        <template #trigger>
          <span class="lane-effort"
            ><n-icon :component="TimerOutline" :size="12" /> {{ effort }}</span
          >
        </template>
        {{ t('board.chart.effortTotal', { value: effortFull }) }}
      </n-tooltip>
    </div>
    <div class="tl-track laneband" :style="{ width: `${axisW}px` }" />
  </div>
</template>

<style scoped>
@import './chart-parts.css';

.tl-lanehead {
  display: flex;
}
.tl-left.lane {
  position: sticky;
  left: 0;
  z-index: 5;
  align-items: center;
  gap: 7px;
  background: var(--t-surface-alt, var(--t-hover));
  border-bottom: 1px solid var(--t-border);
  padding: 5px 12px;
}
/* the base .tl-left:hover is imported above this rule, so spell the lane's own
   hover out — otherwise the lane band would stop lighting up under the pointer */
.tl-left.lane:hover {
  background: var(--t-hover);
}
.lane-dot {
  width: 9px;
  height: 9px;
  border-radius: 3px;
  flex: 0 0 auto;
}
.lane-name {
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.lane-count {
  font-size: 11px;
  color: var(--t-text3);
  background: var(--t-surface);
  border-radius: 20px;
  padding: 0 7px;
  margin-left: auto;
}
.lane-effort {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  color: var(--t-text2);
  white-space: nowrap;
}
/* the lane's band across the axis: a flat header strip, not a task track (kept
   after the imported .tl-track so it wins the day tier on source order — the
   higher-specificity weeks/hours rules still paint their gridlines through it,
   exactly as before the extraction) */
.laneband {
  position: relative;
  background: var(--t-surface-alt, var(--t-hover));
  border-bottom: 1px solid var(--t-border);
  flex: 0 0 auto;
}
</style>
