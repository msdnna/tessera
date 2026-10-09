<script setup>
// Burn-up / burn-down of a stage (#2993, mockup StagePage): drawn by hand in SVG
// — the project has no chart library and needs none for two lines. Burn-up: the
// scope line (text2), done (primary 2.5px over a primary 32% → 0 fill) and the
// forecast dashed from today to the end at the pace of the last five working
// days. Burn-down: what is left, and the ideal line dashed. Weekends sit on a
// faint band, today is the 2px gradient mark the Gantt uses.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  dayMs,
  daysRange,
  isWeekend,
  forecast,
  niceAxis,
  burndown,
  pick,
} from '@/utils/milestoneMetrics'

const props = defineProps({
  series: { type: Array, default: () => [] },
  start: { type: String, default: '' },
  end: { type: String, default: '' },
  today: { type: String, default: '' },
  mode: { type: String, default: 'tasks' }, // tasks | estimate
  kind: { type: String, default: 'up' }, // up | down
  height: { type: Number, default: 220 },
  compact: { type: Boolean, default: false },
})

const box = ref(null)
// Small until measured: a wide first guess would stretch a narrow card.
const width = ref(280)
let ro = null
onMounted(() => {
  if (typeof ResizeObserver === 'undefined' || !box.value) return
  ro = new ResizeObserver(([e]) => {
    width.value = Math.max(200, Math.round(e.contentRect.width))
  })
  ro.observe(box.value)
})
onBeforeUnmount(() => ro?.disconnect())

const PAD = computed(() =>
  props.compact ? { l: 22, r: 10, t: 10, b: 20 } : { l: 32, r: 14, t: 12, b: 26 },
)
// The axis runs from the start to the end (or the last day drawn, if later).
const days = computed(() => {
  const last = props.series.length ? props.series[props.series.length - 1].date : props.start
  let to = props.end || props.today || last
  if (dayMs(last) > dayMs(to)) to = last
  return daysRange(props.start, to)
})
const n = computed(() => Math.max(1, days.value.length - 1))
const innerW = computed(() => width.value - PAD.value.l - PAD.value.r)
const innerH = computed(() => props.height - PAD.value.t - PAD.value.b)
const x = (i) => PAD.value.l + (innerW.value * i) / n.value
const indexOf = (key) => days.value.indexOf(key)

const fc = computed(() =>
  props.kind === 'up' ? forecast(props.series, props.end || '', props.mode) : null,
)
const values = computed(() => {
  if (props.kind === 'down') return burndown(props.series, props.mode).map((p) => p.left)
  return props.series.map((p) => pick(p, props.mode, 'scope'))
})
const axis = computed(() => niceAxis(Math.max(1, ...values.value, fc.value?.value || 0)))
const y = (v) => PAD.value.t + innerH.value - (innerH.value * v) / axis.value.top

function path(points) {
  return points.map(([px, py], i) => `${i ? 'L' : 'M'}${px.toFixed(1)},${py.toFixed(1)}`).join(' ')
}
const scopePath = computed(() =>
  path(props.series.map((p) => [x(indexOf(p.date)), y(pick(p, props.mode, 'scope'))])),
)
const donePts = computed(() =>
  props.series.map((p) => [x(indexOf(p.date)), y(pick(p, props.mode, 'done'))]),
)
const donePath = computed(() => path(donePts.value))
const doneArea = computed(() => {
  const pts = donePts.value
  if (!pts.length) return ''
  const base = y(0)
  return `${path(pts)} L${pts[pts.length - 1][0].toFixed(1)},${base} L${pts[0][0].toFixed(1)},${base} Z`
})
const lastPt = computed(() =>
  donePts.value.length ? donePts.value[donePts.value.length - 1] : null,
)
// The forecast and the scope carried on to the end, dashed.
const forecastPath = computed(() => {
  if (!fc.value || !lastPt.value || !props.end) return ''
  const ei = indexOf(props.end)
  if (ei < 0) return ''
  return path([lastPt.value, [x(ei), y(fc.value.value)]])
})
const forecastEnd = computed(() => {
  const ei = indexOf(props.end)
  if (!fc.value || ei < 0) return null
  return [x(ei), y(fc.value.value)]
})
const scopeAhead = computed(() => {
  if (props.kind !== 'up' || !props.series.length || !props.end) return ''
  const last = props.series[props.series.length - 1]
  const ei = indexOf(props.end)
  const li = indexOf(last.date)
  if (ei <= li) return ''
  const v = y(pick(last, props.mode, 'scope'))
  return path([
    [x(li), v],
    [x(ei), v],
  ])
})
// Burn-down: left per day and the ideal line.
const leftPath = computed(() =>
  props.kind === 'down'
    ? path(burndown(props.series, props.mode).map((p) => [x(indexOf(p.date)), y(p.left)]))
    : '',
)
const idealPath = computed(() => {
  if (props.kind !== 'down' || !props.series.length) return ''
  const first = props.series[0]
  const top = Math.max(0, pick(first, props.mode, 'scope') - pick(first, props.mode, 'done'))
  return path([
    [x(0), y(top)],
    [x(n.value), y(0)],
  ])
})

const weekends = computed(() => {
  const out = []
  const step = innerW.value / n.value
  days.value.forEach((d, i) => {
    if (isWeekend(dayMs(d))) out.push({ x: x(i) - step / 2, w: step })
  })
  return out
})
const todayX = computed(() => {
  const i = indexOf(props.today)
  return i >= 0 ? x(i) : null
})
// Every day labelled when there is room, else every few.
const labelEvery = computed(() => Math.max(1, Math.ceil(days.value.length / (width.value / 34))))
const xLabels = computed(() =>
  days.value
    .map((d, i) => ({ d, i }))
    .filter(({ i }) => i % labelEvery.value === 0 || i === days.value.length - 1)
    .map(({ d, i }) => ({
      x: x(i),
      text: String(Number(d.slice(8, 10))),
      today: d === props.today,
    })),
)

defineExpose({ forecast: fc })
</script>

<template>
  <div ref="box" class="bc" data-testid="burn-chart">
    <svg :width="width" :height="height" :viewBox="`0 0 ${width} ${height}`" role="img">
      <defs>
        <linearGradient id="bc-fill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" style="stop-color: var(--t-primary); stop-opacity: 0.32" />
          <stop offset="1" style="stop-color: var(--t-primary); stop-opacity: 0" />
        </linearGradient>
      </defs>
      <rect
        v-for="(w, i) in weekends"
        :key="`w${i}`"
        class="bc-weekend"
        :x="w.x"
        :y="PAD.t"
        :width="w.w"
        :height="innerH"
      />
      <g v-if="!compact">
        <!-- No grid lines (v2: no separators) — the values sit at the edge. -->
        <text
          v-for="tk in axis.ticks"
          :key="`t${tk}`"
          class="bc-ylab"
          :x="PAD.l - 8"
          :y="y(tk) + 4"
          text-anchor="end"
        >
          {{ tk }}
        </text>
      </g>
      <template v-if="kind === 'up'">
        <path :d="scopePath" class="bc-scope" />
        <path v-if="scopeAhead" :d="scopeAhead" class="bc-scope dashed" />
        <path :d="doneArea" fill="url(#bc-fill)" />
        <path :d="donePath" class="bc-done" />
        <path v-if="forecastPath" :d="forecastPath" class="bc-forecast" />
        <circle
          v-if="forecastEnd"
          class="bc-fend"
          :cx="forecastEnd[0]"
          :cy="forecastEnd[1]"
          r="4"
        />
      </template>
      <template v-else>
        <path :d="idealPath" class="bc-scope dashed" />
        <path :d="leftPath" class="bc-done" />
      </template>
      <circle
        v-if="lastPt && kind === 'up'"
        class="bc-dot"
        :cx="lastPt[0]"
        :cy="lastPt[1]"
        r="3.5"
      />
      <rect
        v-if="todayX != null"
        class="bc-today"
        :x="todayX - 1"
        :y="PAD.t - 6"
        width="2"
        :height="innerH + 6"
        rx="1"
      />
      <text
        v-for="l in xLabels"
        :key="`x${l.x}`"
        class="bc-xlab"
        :class="{ today: l.today }"
        :x="l.x"
        :y="height - 6"
        text-anchor="middle"
      >
        {{ l.text }}
      </text>
    </svg>
  </div>
</template>

<style scoped>
.bc {
  width: 100%;
  min-width: 0;
  overflow: hidden;
}
svg {
  display: block;
  overflow: visible;
}
.bc-weekend {
  fill: var(--t-pill);
  opacity: 0.45;
}
.bc-ylab,
.bc-xlab {
  fill: var(--t-text3);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}
.bc-xlab.today {
  fill: var(--t-text1);
  font-weight: 700;
}
.bc-scope {
  fill: none;
  stroke: var(--t-text2);
  stroke-width: 1.5;
}
.bc-scope.dashed {
  stroke-dasharray: 3 4;
}
.bc-done {
  fill: none;
  stroke: var(--t-primary);
  stroke-width: 2.5;
  stroke-linejoin: round;
  stroke-linecap: round;
}
.bc-forecast {
  fill: none;
  stroke: var(--t-primary);
  stroke-width: 2;
  stroke-dasharray: 6 5;
}
.bc-fend {
  fill: var(--t-col, var(--t-pop));
  stroke: var(--t-primary);
  stroke-width: 2;
}
.bc-dot {
  fill: var(--t-primary);
}
.bc-today {
  fill: url(#t-accent-grad-svg);
}
</style>
