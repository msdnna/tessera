<script setup>
// The milestone's own row at the end of its group (v2, #2959): a diamond and the
// title with the due date in the task column, and on the axis a diamond on the
// due date labelled «Релиз 0.150 · 14 окт.».
import { computed } from 'vue'
import { useFormat } from '@/composables/useFormat'
import { hueGrad } from '@/utils/gradient'

const props = defineProps({
  lane: { type: Object, required: true },
  leftW: { type: Number, required: true },
  axisW: { type: Number, required: true },
  tier: { type: String, required: true },
  collapsed: { type: Boolean, default: false },
  // x of the due day's centre on the axis.
  x: { type: Number, default: null },
})

const { formatDate } = useFormat()
const date = computed(() =>
  props.lane.milestone?.due_date
    ? formatDate(props.lane.milestone.due_date, { day: 'numeric', month: 'short' })
    : '',
)
const grad = computed(() => hueGrad(props.lane.color || '#7c5cff'))
</script>

<template>
  <div class="tl-msrow chart-part" :class="[tier, { collapsed }]">
    <div class="tl-left ms-left" :style="{ width: `${leftW}px` }">
      <span class="ms-slot"><span class="ms-dia" :style="{ background: grad }" /></span>
      <span class="ms-title">{{ lane.milestone?.title }}</span>
      <span class="ms-date">{{ date }}</span>
    </div>
    <div class="tl-track" :style="{ width: `${axisW}px` }">
      <template v-if="x != null">
        <span class="ms-mark" :style="{ left: `${x - 7}px`, background: grad }" />
        <span class="ms-label" :style="{ left: `${x + 14}px` }"
          >{{ lane.milestone?.title }} · {{ date }}</span
        >
      </template>
    </div>
  </div>
</template>

<style scoped>
@import './chart-parts.css';

.tl-msrow {
  display: flex;
}
.ms-left {
  padding-left: 30px;
  cursor: default;
}
.ms-slot {
  flex: 0 0 14px;
  display: inline-flex;
  justify-content: center;
}
.ms-dia {
  width: 9px;
  height: 9px;
  border-radius: 2px;
  transform: rotate(45deg);
}
.ms-title {
  flex: 1;
  min-width: 0;
  font-size: 13.5px;
  font-weight: 500;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ms-date {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
}
.ms-mark {
  position: absolute;
  top: 11px;
  width: 14px;
  height: 14px;
  border-radius: 3px;
  transform: rotate(45deg);
  box-shadow: 0 0 0 3px var(--t-bg);
  z-index: 2;
}
.ms-label {
  position: absolute;
  top: 0;
  line-height: 36px;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text2);
  white-space: nowrap;
  pointer-events: none;
}
</style>
