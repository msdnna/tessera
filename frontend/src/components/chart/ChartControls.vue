<script setup>
// The Gantt / timeline controls in the board's sub-toolbar (v2, #2959) — what
// used to be the chart's own second toolbar row. Driven by the controller the
// open chart view exposes (useChartView → `ctl`): the scale segments, «Сегодня T»,
// the key hints. Its own «Настроить вид» (the task column, the counters, the link
// hint) moved into the board's view panel, section «Шкала» (#2986): one set of
// sliders in the sub-toolbar for every view.
//
// `hints` is decided by the sub-toolbar's fit: the text hints are the first thing
// to give their room up.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NDropdown } from 'naive-ui'
import { LocateOutline, ChevronDownOutline } from '@vicons/ionicons5'
import Kbd from '../Kbd.vue'
import ShortcutKeys from '../ShortcutKeys.vue'

defineProps({
  ctl: { type: Object, default: null },
  hints: { type: Boolean, default: true },
  compact: { type: Boolean, default: false },
})

const { t } = useI18n()
const SCALES = ['days', 'weeks', 'months', 'quarters']
// A phone has no room for three segments: the scale becomes a menu.
const scaleOptions = computed(() =>
  SCALES.map((s) => ({ key: s, label: t(`board.chart.scale.${s}`) })),
)
</script>

<template>
  <div v-if="ctl" class="cc" data-testid="chart-controls">
    <n-dropdown v-if="compact" trigger="click" :options="scaleOptions" @select="ctl.setScale">
      <button type="button" class="cc-btn" data-testid="chart-scale-menu">
        {{ t(`board.chart.scale.${ctl.scale}`) }}
        <n-icon :component="ChevronDownOutline" :size="12" />
      </button>
    </n-dropdown>
    <div v-else class="cc-seg" role="tablist">
      <button
        v-for="s in SCALES"
        :key="s"
        type="button"
        class="cc-seg-btn"
        :class="{ on: ctl.scale === s }"
        :data-testid="`chart-scale-${s}`"
        @click="ctl.setScale(s)"
      >
        {{ t(`board.chart.scale.${s}`) }}
      </button>
    </div>
    <button type="button" class="cc-btn" data-testid="chart-today" @click="ctl.today()">
      <n-icon :component="LocateOutline" :size="14" />
      <span v-if="!compact">{{ t('board.chart.today') }}</span>
      <ShortcutKeys v-if="!compact" id="chart.today" />
    </button>
    <span v-if="hints && !compact" class="cc-hints">
      <span class="cc-hint"
        ><ShortcutKeys id="chart.scroll" /> {{ t('board.chart.hintScroll') }}</span
      >
      <span class="cc-hint">
        <button
          type="button"
          class="cc-kbd-btn"
          :disabled="!ctl.canZoomIn"
          :title="t('board.chart.zoomIn')"
          @click="ctl.zoomIn()"
        >
          <Kbd>+</Kbd>
        </button>
        <button
          type="button"
          class="cc-kbd-btn"
          :disabled="!ctl.canZoomOut"
          :title="t('board.chart.zoomOut')"
          @click="ctl.zoomOut()"
        >
          <Kbd>−</Kbd>
        </button>
        {{ t('board.chart.hintZoom') }}
      </span>
    </span>
  </div>
</template>

<style scoped>
.cc {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
  min-width: 0;
}
.cc-seg {
  display: inline-flex;
  gap: 2px;
  height: 34px;
  padding: 3px;
  box-sizing: border-box;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
}
.cc-seg-btn {
  height: 28px;
  padding: 0 12px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
  white-space: nowrap;
}
.cc-seg-btn:hover {
  color: var(--t-text1);
}
.cc-seg-btn.on {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.cc-btn {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 34px;
  padding: 0 7px 0 10px;
  border: none;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
}
.cc-btn:hover {
  color: var(--t-text1);
  background: var(--t-hover);
}
.cc-hints {
  display: inline-flex;
  align-items: center;
  gap: 14px;
  margin: 0 4px;
  white-space: nowrap;
}
.cc-hint {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--t-text3);
}
.cc-kbd-btn {
  display: inline-flex;
  padding: 0;
  border: none;
  background: none;
  cursor: pointer;
}
.cc-kbd-btn:disabled {
  opacity: 0.4;
  cursor: default;
}
</style>
