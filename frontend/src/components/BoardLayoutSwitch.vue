<script setup>
import { computed, h, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NDropdown, NIcon } from 'naive-ui'
import { ChevronDownOutline } from '@vicons/ionicons5'
import { useBoardViewStore } from '@/stores/boardView'
import TesseraIcon from './TesseraIcon.vue'

// v2 segmented view switch (#2954). It is the stretchy part of the board's
// sub-toolbar: the board measures what the other pieces leave (`available`, px)
// and the switch shows as many views as fit there, the rest behind a chevron.
// The active view is always a visible segment.
const props = defineProps({
  available: { type: Number, default: Infinity },
})
// The board re-fits its row when the segment widths become known or change.
const emit = defineEmits(['measured'])

const store = useBoardViewStore()
const { t } = useI18n()
// Display order on the bar. Labels are resolved per render so the switcher
// follows a language change.
const LAYOUTS = [
  { value: 'board', icon: 'layout-kanban' },
  { value: 'list', icon: 'layout-list' },
  { value: 'calendar', icon: 'layout-calendar' },
  { value: 'timeline', icon: 'layout-timeline' },
  { value: 'gantt', icon: 'layout-gantt' },
  { value: 'matrix', icon: 'layout-matrix' },
]
// Which views claim the room first when not all fit (the design's trio leads).
const PRIORITY = ['board', 'list', 'timeline', 'calendar', 'gantt', 'matrix']
const label = (v) => t(`board.layout.${v}`)

// Natural segment widths, read from an invisible row of all segments.
const measureEl = ref(null)
const widths = reactive({})
const chevronW = ref(28)
function measure() {
  const row = measureEl.value
  if (!row) return
  for (const el of row.querySelectorAll('[data-seg]')) widths[el.dataset.seg] = el.offsetWidth
  const chev = row.querySelector('[data-chevron]')
  if (chev) chevronW.value = chev.offsetWidth
  emit('measured')
}
let ro = null
onMounted(() => {
  measure()
  // Labels change width with the language and the font loading in.
  ro = new ResizeObserver(measure)
  if (measureEl.value) ro.observe(measureEl.value)
})
onBeforeUnmount(() => ro?.disconnect())
watch(
  () => t('board.layout.board'),
  () => nextTick(measure),
)

// Container chrome: 3px padding each side, 2px between segments.
const PAD = 6
const GAP = 2
const shownKeys = computed(() => {
  const all = LAYOUTS.map((o) => o.value)
  // Not measured yet → assume a wide segment, never zero (zero would show all).
  const w = (k) => widths[k] || 120
  const total = (keys, chevron) =>
    PAD +
    keys.reduce((n, k) => n + w(k), 0) +
    GAP * (keys.length - 1 + (chevron ? 1 : 0)) +
    (chevron ? chevronW.value : 0)
  if (total(all, false) <= props.available) return all
  const keep = [store.layout]
  for (const k of PRIORITY) {
    if (keep.includes(k)) continue
    if (total([...keep, k], true) <= props.available) keep.push(k)
  }
  return all.filter((k) => keep.includes(k))
})
const shown = computed(() => LAYOUTS.filter((o) => shownKeys.value.includes(o.value)))
const overflow = computed(() =>
  LAYOUTS.filter((o) => !shownKeys.value.includes(o.value)).map((o) => ({
    key: o.value,
    label: label(o.value),
    icon: () => h(TesseraIcon, { name: o.icon }),
  })),
)
</script>

<template>
  <div class="layout-switch" data-tour="board-layout">
    <button
      v-for="o in shown"
      :key="o.value"
      type="button"
      class="seg"
      :class="{ active: store.layout === o.value }"
      :data-layout="o.value"
      @click="store.layout = o.value"
    >
      <TesseraIcon :name="o.icon" class="seg-ic" />
      {{ label(o.value) }}
    </button>
    <n-dropdown
      v-if="overflow.length"
      trigger="click"
      placement="bottom-start"
      :options="overflow"
      @select="(k) => (store.layout = k)"
    >
      <button type="button" class="seg seg-more" :title="t('board.layout.more')">
        <n-icon :component="ChevronDownOutline" :size="13" />
      </button>
    </n-dropdown>

    <!-- Invisible natural-width row the fit is computed from. -->
    <div ref="measureEl" class="seg-measure" aria-hidden="true">
      <span v-for="o in LAYOUTS" :key="o.value" class="seg" :data-seg="o.value">
        <TesseraIcon :name="o.icon" class="seg-ic" />
        {{ label(o.value) }}
      </span>
      <span class="seg seg-more" data-chevron>
        <n-icon :component="ChevronDownOutline" :size="13" />
      </span>
    </div>
  </div>
</template>

<style scoped>
.layout-switch {
  position: relative;
  display: flex;
  align-items: center;
  gap: 2px;
  flex: none;
  padding: 3px;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
}
.seg {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px;
  border: none;
  border-radius: calc(var(--t-r-ctl) - 3px);
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
}
.seg:hover {
  background: var(--t-hover);
}
.seg-ic {
  font-size: 15px;
  color: var(--t-text3);
}
/* The active view lifts to the card level with the card shadow; its glyph takes
   the accent. */
.seg.active {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.seg.active .seg-ic {
  color: var(--t-primary);
}
.seg-more {
  width: 28px;
  padding: 0;
  justify-content: center;
  color: var(--t-text3);
}
/* Measured with the active weight so a segment never grows once selected. */
/* Zero-size and clipped so the row never adds scrollable overflow to the page;
   its segments keep their natural width (no shrinking) for the measurement. */
.seg-measure {
  position: absolute;
  left: 0;
  top: 0;
  width: 0;
  height: 0;
  overflow: hidden;
  display: flex;
  gap: 2px;
  visibility: hidden;
  pointer-events: none;
}
.seg-measure .seg {
  flex: none;
  font-weight: 600;
}
</style>
