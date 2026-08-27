<script setup>
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { ChevronBackOutline, ChevronForwardOutline } from '@vicons/ionicons5'

// Toolbar above the chart: jump-to-today, zoom in/out, collapse the task column,
// and a row of counter chips. Shared by the Timeline and Gantt views — the Gantt
// adds a hint and a "N связей" counter, both passed in.
defineProps({
  zoomIdx: { type: Number, required: true },
  zoomCount: { type: Number, required: true },
  leftCollapsed: { type: Boolean, default: false },
  hint: { type: String, default: '' },
  // [{ key, text, overdue? }] — rendered right-aligned as muted chips.
  counters: { type: Array, default: () => [] },
})
defineEmits(['today', 'zoom-in', 'zoom-out', 'toggle-left'])

const { t } = useI18n()
</script>

<template>
  <div class="tl-toolbar">
    <button class="tl-today-btn" type="button" @click="$emit('today')">
      {{ t('board.chart.today') }}
    </button>
    <div class="tl-zoom">
      <button
        class="tl-zoom-btn"
        type="button"
        :disabled="zoomIdx === 0"
        :title="t('board.chart.zoomOut')"
        @click="$emit('zoom-out')"
      >
        −
      </button>
      <button
        class="tl-zoom-btn"
        type="button"
        :disabled="zoomIdx === zoomCount - 1"
        :title="t('board.chart.zoomIn')"
        @click="$emit('zoom-in')"
      >
        +
      </button>
      <button
        class="tl-zoom-btn"
        type="button"
        :title="t(leftCollapsed ? 'board.chart.showTasks' : 'board.chart.collapseTasks')"
        @click="$emit('toggle-left')"
      >
        <n-icon
          :component="leftCollapsed ? ChevronForwardOutline : ChevronBackOutline"
          :size="15"
        />
      </button>
    </div>
    <span v-if="hint" class="tl-hint">{{ hint }}</span>
    <div class="tl-counters">
      <span v-for="c in counters" :key="c.key" class="tl-counter" :class="{ overdue: c.overdue }">{{
        c.text
      }}</span>
    </div>
  </div>
</template>

<style scoped>
.tl-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}
.tl-today-btn {
  border: 1px solid var(--t-border);
  background: var(--t-surface);
  color: var(--t-text2);
  border-radius: 7px;
  padding: 5px 12px;
  font-size: 13px;
  cursor: pointer;
}
.tl-today-btn:hover {
  background: var(--t-hover);
}
.tl-zoom {
  display: flex;
  gap: 4px;
}
.tl-zoom-btn {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  border: 1px solid var(--t-border);
  background: var(--t-surface);
  color: var(--t-text2);
  border-radius: 7px;
  font-size: 17px;
  line-height: 1;
  cursor: pointer;
}
.tl-zoom-btn:hover:not(:disabled) {
  background: var(--t-hover);
}
.tl-zoom-btn:disabled {
  opacity: 0.4;
  cursor: default;
}
.tl-hint {
  font-size: 12px;
  color: var(--t-text3);
}
.tl-counters {
  display: flex;
  gap: 8px;
  margin-left: auto;
}
.tl-counter {
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-hover);
  border-radius: 20px;
  padding: 3px 10px;
}
.tl-counter.overdue {
  color: #e0533d;
  background: color-mix(in srgb, #e0533d 12%, transparent);
}
</style>
