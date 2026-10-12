<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useBoardViewStore } from '@/stores/boardView'
import { LAYOUTS, LAYOUT_ICONS } from '@/utils/boardViews'
import TesseraIcon from './TesseraIcon.vue'

// v2 segmented view switch (#2954, #2970). Six equal views — Канбан · Список ·
// Таймлайн · Гант · Календарь · Матрица (V, then 1–6) — and no «more» menu: when
// the sub-toolbar is short, the segments drop their captions (the active one
// keeps its own), so every view stays one click away. The board measures what
// the other pieces leave (`available`, px).
const props = defineProps({
  available: { type: Number, default: Infinity },
})
// The board re-fits its row when the segment widths become known or change.
const emit = defineEmits(['measured'])

const store = useBoardViewStore()
const { t } = useI18n()
const label = (v) => t(`board.layout.${v}`)

// Natural segment widths, read from an invisible row of all segments.
const measureEl = ref(null)
const widths = reactive({})
function measure() {
  const row = measureEl.value
  if (!row) return
  for (const el of row.querySelectorAll('[data-seg]')) widths[el.dataset.seg] = el.offsetWidth
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
const full = computed(() => {
  // Not measured yet → assume they fit (the board re-fits once measured).
  const w = (k) => widths[k] || 0
  const total = PAD + LAYOUTS.reduce((n, k) => n + w(k), 0) + GAP * (LAYOUTS.length - 1)
  return total <= props.available
})
const segments = computed(() =>
  LAYOUTS.map((value) => ({
    value,
    icon: LAYOUT_ICONS[value],
    caption: full.value || store.layout === value,
  })),
)
</script>

<template>
  <div class="layout-switch" :class="{ compact: !full }" data-tour="board-layout">
    <button
      v-for="(o, i) in segments"
      :key="o.value"
      type="button"
      class="seg"
      :class="{ active: store.layout === o.value, icon: !o.caption }"
      :data-layout="o.value"
      :title="o.caption ? undefined : `${label(o.value)} · V ${i + 1}`"
      :aria-label="label(o.value)"
      @click="store.layout = o.value"
    >
      <TesseraIcon :name="o.icon" class="seg-ic" />
      <template v-if="o.caption">{{ label(o.value) }}</template>
    </button>

    <!-- Invisible natural-width row the fit is computed from. -->
    <div ref="measureEl" class="seg-measure" aria-hidden="true">
      <span v-for="v in LAYOUTS" :key="v" class="seg" :data-seg="v">
        <TesseraIcon :name="LAYOUT_ICONS[v]" class="seg-ic" />
        {{ label(v) }}
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
.seg.icon {
  width: 30px;
  padding: 0;
  justify-content: center;
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
