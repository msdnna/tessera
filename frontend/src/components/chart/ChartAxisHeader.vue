<script setup>
// Sticky chart header: the month band over a day / week band, plus hour ticks in
// the hours tier. Every band comes precomputed from useChartTimeline, so this is
// pure layout — shared verbatim by the Timeline and Gantt views.
defineProps({
  leftW: { type: Number, required: true },
  dayW: { type: Number, required: true },
  // 'hours' | 'days' | 'weeks' — picks which band row renders under the months.
  tier: { type: String, required: true },
  collapsed: { type: Boolean, default: false },
  monthBands: { type: Array, default: () => [] },
  weekBands: { type: Array, default: () => [] },
  days: { type: Array, default: () => [] },
  hourTicks: { type: Array, default: () => [] },
})
</script>

<template>
  <div class="tl-head chart-part" :class="{ collapsed }">
    <div class="tl-corner" :style="{ width: `${leftW}px` }">Задача</div>
    <div class="tl-axis">
      <div class="tl-months">
        <div
          v-for="(m, i) in monthBands"
          :key="i"
          class="tl-month"
          :style="{ width: `${m.span * dayW}px` }"
        >
          {{ m.label }}
        </div>
      </div>
      <div v-if="tier === 'weeks'" class="tl-weeksrow">
        <div
          v-for="w in weekBands"
          :key="w.key"
          class="tl-weekh"
          :style="{ width: `${w.span * dayW}px` }"
        >
          {{ w.label }}
        </div>
      </div>
      <template v-else>
        <div class="tl-daysrow">
          <div
            v-for="(d, i) in days"
            :key="i"
            class="tl-dayh"
            :class="{ weekend: d.weekend, today: d.isToday }"
            :style="{ width: `${dayW}px` }"
          >
            <span class="dh-num">{{ d.day }}</span>
            <span class="dh-wd">{{ d.dow }}</span>
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
.tl-corner {
  box-sizing: border-box;
  position: sticky;
  left: 0;
  z-index: 7;
  flex: 0 0 auto;
  background: var(--t-surface-alt, var(--t-hover));
  border-right: 1px solid var(--t-border);
  border-bottom: 1px solid var(--t-border);
  display: flex;
  align-items: flex-end;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
  /* clip the caption as the column collapses (the toggle is instant) */
  overflow: hidden;
}
/* collapsed: zero the padding + right border so the column reaches a TRUE 0 width */
.tl-head.collapsed .tl-corner {
  padding-left: 0;
  padding-right: 0;
  border-right-width: 0;
}
.tl-axis {
  flex: 0 0 auto;
}
.tl-months {
  display: flex;
  height: 22px;
  border-bottom: 1px solid var(--t-border);
}
.tl-month {
  box-sizing: border-box;
  flex: 0 0 auto;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text2);
  padding: 3px 8px;
  white-space: nowrap;
  overflow: hidden;
  border-right: 1px solid var(--t-border);
  background: var(--t-surface-alt, var(--t-hover));
  text-transform: capitalize;
}
.tl-daysrow {
  display: flex;
  background: var(--t-surface);
  border-bottom: 1px solid var(--t-border);
}
.tl-dayh {
  /* border-box so a cell's total width == the inline `dayW`, keeping the day
     headers aligned with the bar/gridline/today-line geometry (which use dayW). */
  box-sizing: border-box;
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 3px 0;
  font-size: 11px;
  color: var(--t-text3);
  border-right: 1px solid color-mix(in srgb, var(--t-border) 55%, transparent);
}
.tl-dayh.weekend {
  background: var(--t-hover);
}
.tl-dayh.today .dh-num {
  background: var(--t-accent-grad);
  color: #fff;
  border-radius: 50%;
  width: 18px;
  height: 18px;
  line-height: 18px;
  text-align: center;
}
.dh-num {
  font-size: 12px;
  color: var(--t-text1);
}
.dh-wd {
  font-size: 9px;
}

/* weeks tier: one cell per week, labelled by its first date */
.tl-weeksrow {
  display: flex;
  background: var(--t-surface);
  border-bottom: 1px solid var(--t-border);
}
.tl-weekh {
  box-sizing: border-box;
  flex: 0 0 auto;
  padding: 4px 6px;
  font-size: 11px;
  color: var(--t-text2);
  white-space: nowrap;
  overflow: hidden;
  border-right: 1px solid color-mix(in srgb, var(--t-border) 55%, transparent);
}
/* hours tier: thin row of hour labels positioned at their fraction of the day */
.tl-hoursrow {
  position: relative;
  height: 14px;
  background: var(--t-surface);
  border-bottom: 1px solid var(--t-border);
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
