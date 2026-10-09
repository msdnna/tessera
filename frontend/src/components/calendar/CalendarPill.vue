<script setup>
// One task in the «Календарь» view (v2, #2973 / #3019; mockups CalendarView,
// CatalogD «День календаря»): a 22px pill in the soft tone of its status — the
// column's glyph and colour, as the column header draws them — with a red dot
// when the due has passed. A done task is paler and struck through. In the week
// mode a timed due shows its time in front of the title.
import { computed } from 'vue'
import TesseraIcon from '../TesseraIcon.vue'

const props = defineProps({
  task: { type: Object, required: true },
  // { icon, color, done } — utils/columnStatus taskStatus.
  status: { type: Object, required: true },
  overdue: { type: Boolean, default: false },
  time: { type: String, default: '' },
  // The keyboard's pick (ring), and the original left behind while dragged.
  picked: { type: Boolean, default: false },
  ghost: { type: Boolean, default: false },
  // The copy that follows the pointer.
  flying: { type: Boolean, default: false },
})

const NEUTRAL = '#9aa0aa'
const hue = computed(() => props.status.color || NEUTRAL)
const style = computed(() => {
  const [a, b] = props.status.done ? [8, 14] : [14, 24]
  return {
    '--pc': hue.value,
    background: `linear-gradient(to top right, color-mix(in srgb, ${hue.value} ${a}%, var(--t-card)), color-mix(in srgb, ${hue.value} ${b}%, var(--t-card)))`,
  }
})
</script>

<template>
  <div
    class="cp"
    :class="{ done: status.done, picked, ghost, flying }"
    :style="style"
    :title="task.title"
    :data-task-id="task.id"
    data-testid="calendar-pill"
  >
    <TesseraIcon
      :name="status.icon"
      :variant="status.done ? 'filled' : 'outline'"
      class="cp-ic"
      :style="{ color: hue }"
    />
    <span v-if="time" class="cp-time">{{ time }}</span>
    <span class="cp-title">{{ task.title }}</span>
    <span v-if="overdue" class="cp-over" />
  </div>
</template>

<style scoped>
.cp {
  position: relative;
  flex: none;
  display: flex;
  align-items: center;
  gap: 5px;
  height: 22px;
  padding: 0 7px 0 5px;
  box-sizing: border-box;
  border-radius: 6px;
  box-shadow: var(--t-sh-card);
  font-size: 12px;
  color: var(--t-text1);
  white-space: nowrap;
  overflow: hidden;
  cursor: pointer;
  user-select: none;
}
.cp:hover {
  filter: brightness(1.06);
}
.cp-ic {
  flex: none;
  width: 11px;
  height: 11px;
}
.cp-time {
  flex: none;
  font-size: 11px;
  font-weight: 600;
  color: var(--t-text2);
  font-variant-numeric: tabular-nums;
}
.cp-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cp.done {
  color: var(--t-text3);
}
.cp.done .cp-title {
  text-decoration: line-through;
}
.cp-over {
  flex: 0 0 6px;
  width: 6px;
  height: 6px;
  margin-left: auto;
  border-radius: 3px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 86%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 86%, #fff)
  );
}
.cp.picked {
  box-shadow:
    0 0 0 2px var(--t-primary),
    var(--t-sh-card);
}
.cp.ghost {
  opacity: 0.4;
}
.cp.flying {
  box-shadow: var(--t-sh-drag);
  transform: rotate(-2deg);
  cursor: grabbing;
}
</style>
