<script setup>
// A list row with swipe actions on the touch (v2, #2976 — package G «Свайп»).
//
// Past 25% of the width the row stays open on its actions; past 60% the side's
// first action runs on release, with a short vibration at the threshold. Each
// action is 76px wide, on a gradient of its own hue. Only tasks and «Входящие»
// records have it — the board's cards scroll sideways with their columns.
//
//   <SwipeRow :right="[{ key: 'done', label, icon, tone: '#18a058' }]"
//             :left="[{ key: 'snooze', label, icon, tone: '#f0a020' }]"
//             @action="(key) => …">…the row…</SwipeRow>
//
// `left` actions sit under the row's left edge and are revealed by a swipe to
// the right; `right` ones — by a swipe to the left (the first is the outermost,
// the one a long swipe runs). An action with `stub: N` is drawn inactive and
// answers with «Скоро · #N». A mouse never swipes: the row is a plain row on a
// desktop.
import { computed, ref, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import { hueGrad } from '@/utils/gradient'

const props = defineProps({
  left: { type: Array, default: () => [] },
  right: { type: Array, default: () => [] },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['action'])
const { t } = useI18n()
const message = useMessage()

const W = 76
const x = ref(0)
const moving = ref(false)
const root = ref(null)
let start = null
let horizontal = null
let buzzed = false

const leftW = computed(() => props.left.length * W)
const rightW = computed(() => props.right.length * W)

function width() {
  return root.value?.offsetWidth || 360
}
function onDown(e) {
  if (props.disabled || e.pointerType !== 'touch') return
  start = { x: e.clientX - x.value, y: e.clientY, at: e.clientX }
  horizontal = null
  buzzed = false
}
function onMove(e) {
  if (!start) return
  const dx = e.clientX - start.at
  const dy = e.clientY - start.y
  if (horizontal === null) {
    if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return
    horizontal = Math.abs(dx) > Math.abs(dy)
    if (!horizontal) {
      start = null
      return
    }
    root.value?.setPointerCapture?.(e.pointerId)
  }
  moving.value = true
  let nx = e.clientX - start.x
  if (nx > 0 && !props.left.length) nx = 0
  if (nx < 0 && !props.right.length) nx = 0
  x.value = nx
  const far = Math.abs(nx) > width() * 0.6
  if (far && !buzzed) {
    buzzed = true
    navigator.vibrate?.(10)
  } else if (!far) buzzed = false
}
function onUp() {
  if (!start) return
  start = null
  moving.value = false
  if (!horizontal) return
  const w = width()
  const v = x.value
  if (Math.abs(v) > w * 0.6) {
    const a = v > 0 ? props.left[0] : props.right[0]
    x.value = 0
    if (a) run(a)
    return
  }
  if (Math.abs(v) > w * 0.25) x.value = v > 0 ? leftW.value : -rightW.value
  else x.value = 0
}
function close() {
  x.value = 0
}
function run(a) {
  close()
  if (a.stub) {
    message.info(t('common.stub.soon', { n: a.stub }))
    return
  }
  emit('action', a.key)
}
// A tap on the open row closes it instead of opening the row.
function onClickCapture(e) {
  if (x.value !== 0) {
    e.stopPropagation()
    e.preventDefault()
    close()
  }
}
onBeforeUnmount(() => (start = null))
defineExpose({ close })

const bg = (a) => ({ background: hueGrad(a.tone) })
</script>

<template>
  <div
    ref="root"
    class="sw"
    :class="{ moving, open: x !== 0 }"
    @pointerdown="onDown"
    @pointermove="onMove"
    @pointerup="onUp"
    @pointercancel="onUp"
  >
    <div v-if="left.length && x > 0" class="sw-acts left">
      <button
        v-for="a in left"
        :key="a.key"
        type="button"
        class="sw-act"
        :class="{ stub: a.stub }"
        :style="bg(a)"
        :data-testid="`swipe-${a.key}`"
        @click="run(a)"
      >
        <n-icon :component="a.icon" :size="20" />
        <span>{{ a.label }}</span>
      </button>
    </div>
    <div v-if="right.length && x < 0" class="sw-acts right">
      <button
        v-for="a in [...right].reverse()"
        :key="a.key"
        type="button"
        class="sw-act"
        :class="{ stub: a.stub }"
        :style="bg(a)"
        :data-testid="`swipe-${a.key}`"
        @click="run(a)"
      >
        <n-icon :component="a.icon" :size="20" />
        <span>{{ a.label }}</span>
      </button>
    </div>
    <div
      class="sw-body"
      :style="x ? { transform: `translateX(${x}px)` } : null"
      @click.capture="onClickCapture"
    >
      <slot />
    </div>
  </div>
</template>

<style scoped>
.sw {
  position: relative;
  overflow: hidden;
  border-radius: var(--sw-r, 14px);
  touch-action: pan-y;
}
.sw-body {
  position: relative;
  z-index: 1;
  transition: transform 0.2s ease;
}
.sw.moving .sw-body {
  transition: none;
}
.sw-acts {
  position: absolute;
  top: 0;
  bottom: 0;
  display: flex;
}
.sw-acts.left {
  left: 0;
}
.sw-acts.right {
  right: 0;
}
.sw-act {
  width: 76px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 0 4px;
  border: 0;
  color: #fff;
  font: inherit;
  font-size: 12px;
  font-weight: 650;
  cursor: pointer;
}
.sw-act.stub {
  opacity: 0.55;
}
@media (prefers-reduced-motion: reduce) {
  .sw-body {
    transition: none;
  }
}
</style>
