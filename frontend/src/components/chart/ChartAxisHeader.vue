<script setup>
// Sticky chart header (v2, #2959): «ЗАДАЧА N … ЛЮДИ» over the task column, and
// two bands over the axis — what the second band counts, grouped by the first:
//   days   — week ranges («28 сент. – 4 окт.») over day numbers;
//   hours  — months over day numbers, hour ticks under them;
//   weeks  — months over week ranges («29–4»);
//   months — years over months.
// Every band comes precomputed from useChartTimeline, so this is pure layout —
// shared verbatim by the Timeline and Gantt views.
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

defineProps({
  leftW: { type: Number, required: true },
  dayW: { type: Number, required: true },
  // 'hours' | 'days' | 'weeks' | 'months'
  tier: { type: String, required: true },
  collapsed: { type: Boolean, default: false },
  // How many tasks the chart shows — next to «Задача».
  count: { type: Number, default: 0 },
  monthBands: { type: Array, default: () => [] },
  weekBands: { type: Array, default: () => [] },
  weekRangeBands: { type: Array, default: () => [] },
  yearBands: { type: Array, default: () => [] },
  days: { type: Array, default: () => [] },
  hourTicks: { type: Array, default: () => [] },
})
</script>

<template>
  <div class="tl-head chart-part" :class="{ collapsed }">
    <div class="tl-corner" :style="{ width: `${leftW}px` }">
      <span class="tc-cap">{{ t('board.chart.taskColumn') }}</span>
      <span class="tc-count">{{ count }}</span>
      <span v-if="leftW >= 220" class="tc-cap tc-people">{{ t('board.chart.people') }}</span>
    </div>
    <div class="tl-axis">
      <!-- first band -->
      <div class="tl-band tl-band1">
        <template v-if="tier === 'days'">
          <div
            v-for="w in weekRangeBands"
            :key="w.key"
            class="tl-b1"
            :style="{ width: `${w.span * dayW}px` }"
          >
            {{ w.label }}
          </div>
        </template>
        <template v-else-if="tier === 'months'">
          <div
            v-for="y in yearBands"
            :key="y.key"
            class="tl-b1"
            :style="{ width: `${y.span * dayW}px` }"
          >
            {{ y.label }}
          </div>
        </template>
        <template v-else>
          <div
            v-for="(m, i) in monthBands"
            :key="i"
            class="tl-b1 tl-month"
            :style="{ width: `${m.span * dayW}px` }"
          >
            {{ m.label }}
          </div>
        </template>
      </div>
      <!-- second band -->
      <div v-if="tier === 'weeks'" class="tl-band tl-band2">
        <div
          v-for="w in weekBands"
          :key="w.key"
          class="tl-b2 tl-weekh"
          :style="{ width: `${w.span * dayW}px` }"
        >
          {{ w.label }}
        </div>
      </div>
      <div v-else-if="tier === 'months'" class="tl-band tl-band2">
        <div
          v-for="(m, i) in monthBands"
          :key="i"
          class="tl-b2 tl-weekh tl-month"
          :style="{ width: `${m.span * dayW}px` }"
        >
          {{ m.label }}
        </div>
      </div>
      <template v-else>
        <div class="tl-band tl-band2 tl-daysrow">
          <div
            v-for="(d, i) in days"
            :key="i"
            class="tl-dayh"
            :class="{ weekend: d.weekend, today: d.isToday }"
            :style="{ width: `${dayW}px` }"
            :title="d.dow"
          >
            <span class="dh-num">{{ d.day }}</span>
          </div>
        </div>
        <div v-if="tier === 'hours'" class="tl-hoursrow">
          <span
            v-for="h in hourTicks"
            :key="h.key"
            class="tl-hourtick"
            :style="{ left: `${h.left}px` }"
            >{{ h.label }}</span
          >
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.tl-head {
  display: flex;
  position: sticky;
  top: 0;
  z-index: 6;
}
/* The task column's head: on the canvas, no box — the caption, the count and
   «Люди» pinned right. */
.tl-corner {
  box-sizing: border-box;
  position: sticky;
  left: 0;
  z-index: 7;
  flex: 0 0 auto;
  display: flex;
  align-items: flex-end;
  gap: 6px;
  padding: 0 12px 9px 10px;
  background: var(--t-bg);
  overflow: hidden;
}
.tc-cap {
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
  white-space: nowrap;
}
.tc-count {
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.tc-people {
  margin-left: auto;
}
/* collapsed: zero the padding so the column reaches a TRUE 0 width */
.tl-head.collapsed .tl-corner {
  padding-left: 0;
  padding-right: 0;
}
.tl-axis {
  flex: 0 0 auto;
  /* the track backdrop (--t-col) starts here; the header shares it */
  background: var(--t-col);
  border-top-left-radius: 14px;
}
.tl-band {
  display: flex;
  height: 26px;
}
.tl-b1 {
  box-sizing: border-box;
  flex: 0 0 auto;
  padding: 7px 8px 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tl-month {
  text-transform: capitalize;
}
.tl-b2 {
  box-sizing: border-box;
  flex: 0 0 auto;
  padding: 4px 8px 0;
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
}
.tl-dayh {
  /* border-box so a cell's total width == the inline `dayW`, keeping the day
     headers aligned with the bar / backdrop / today-line geometry. */
  box-sizing: border-box;
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11.5px;
  line-height: 22px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.tl-dayh.weekend {
  color: var(--t-placeholder);
}
.dh-num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  height: 22px;
}
.tl-dayh.today .dh-num {
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: #fff;
  font-weight: 700;
}
/* hours tier: thin row of hour labels positioned at their fraction of the day */
.tl-hoursrow {
  position: relative;
  height: 14px;
}
.tl-hourtick {
  position: absolute;
  top: 0;
  transform: translateX(-50%);
  font-size: 9px;
  line-height: 14px;
  color: var(--t-text3);
  pointer-events: none;
}
</style>
