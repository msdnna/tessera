<script setup>
// The Gantt / timeline controls in the board's sub-toolbar (v2, #2959) — what
// used to be the chart's own second toolbar row. Driven by the controller the
// open chart view exposes (useChartView → `ctl`): the scale segments, «Сегодня T»,
// the key hints and «Настроить вид» (the task column, the counters, the hint).
//
// `hints` is decided by the sub-toolbar's fit: the text hints are the first thing
// to give their room up.
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NPopover, NSwitch, NDropdown } from 'naive-ui'
import { LocateOutline, OptionsOutline, ChevronDownOutline } from '@vicons/ionicons5'
import Kbd from '../Kbd.vue'
import ShortcutKeys from '../ShortcutKeys.vue'

const props = defineProps({
  ctl: { type: Object, default: null },
  hints: { type: Boolean, default: true },
  compact: { type: Boolean, default: false },
})

const { t } = useI18n()
const SCALES = ['days', 'weeks', 'months']
const showSettings = ref(false)
const counters = computed(() => props.ctl?.counters || [])
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
    <n-popover v-model:show="showSettings" trigger="click" placement="bottom-end">
      <template #trigger>
        <button
          type="button"
          class="cc-btn cc-icon"
          :title="t('board.chart.settings')"
          data-testid="chart-settings"
        >
          <n-icon :component="OptionsOutline" :size="16" />
          <span v-if="counters.some((c) => c.overdue)" class="cc-dot" />
        </button>
      </template>
      <div class="cc-pop">
        <div class="cc-row">
          <span>{{ t('board.chart.showTasks') }}</span>
          <n-switch size="small" :value="!ctl.leftCollapsed" @update:value="ctl.toggleLeft()" />
        </div>
        <div v-if="counters.length" class="cc-counters">
          <span
            v-for="c in counters"
            :key="c.key"
            class="cc-counter"
            :class="{ overdue: c.overdue }"
            >{{ c.text }}</span
          >
        </div>
        <!-- the link hint describes a hover-only knob: no use on a touch screen -->
        <div v-if="ctl.hint && !compact" class="cc-note">{{ ctl.hint }}</div>
      </div>
    </n-popover>
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
.cc-icon {
  width: 34px;
  padding: 0;
  justify-content: center;
}
.cc-dot {
  position: absolute;
  top: 7px;
  right: 7px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #e0533d;
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
.cc-pop {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 240px;
  max-width: 300px;
}
.cc-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
}
.cc-counters {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.cc-counter {
  height: 22px;
  padding: 0 9px;
  border-radius: 11px;
  display: inline-flex;
  align-items: center;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 12px;
}
.cc-counter.overdue {
  background: color-mix(in srgb, #e0533d 16%, var(--t-pop));
  color: #e0533d;
}
.cc-note {
  font-size: 12px;
  line-height: 1.45;
  color: var(--t-text3);
}
</style>
