<script setup>
// «Отложить» (#2997, mockup SnoozeMenu): a small menu under a reminder. Every
// option is counted from now; a digit picks one without the arrows, D opens a
// line for a moment in words («чт 15:30», «через 3 дня»). Esc closes.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  CafeOutline,
  CalendarClearOutline,
  CalendarNumberOutline,
  FlagOutline,
  HourglassOutline,
  SunnyOutline,
} from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import { useAnchoredPanel } from '@/composables/useAnchoredPanel'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { useResponsive } from '@/composables/useResponsive'
import { useFormat } from '@/composables/useFormat'
import { snoozeOptions } from '@/utils/inbox'
import { parseCapture } from '@/utils/quickParse'
import { cap } from '@/utils/keyCaps'
import { vLayer } from '@/utils/layerZ'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  // The record's task, for «К сроку задачи».
  task: { type: Object, default: null },
  placement: { type: String, default: 'bottom-end' },
  testid: { type: String, default: 'snooze-menu' },
})
const emit = defineEmits(['update:show', 'pick'])
const { t } = useI18n()
const { formatTime, formatDue } = useFormat()

const panel = ref(null)
const showRef = computed(() => props.show)
function close() {
  emit('update:show', false)
}
// A phone (#2976 — package G «Входящие»): the variants in a sheet from the
// bottom, rows 48px; the system Back closes it.
const { isMobile: sheet } = useResponsive()
useOverlayBack(
  computed(() => props.show && sheet.value),
  close,
)
const { style, place } = useAnchoredPanel({
  show: showRef,
  anchor: () => props.anchor(),
  panel,
  placement: props.placement,
  offset: { x: 0, y: 4 },
  onOutside: close,
})

const ICONS = {
  hour: HourglassOutline,
  evening: SunnyOutline,
  morning: CafeOutline,
  monday: CalendarNumberOutline,
  due: FlagOutline,
}

const now = ref(new Date())
const options = computed(() => snoozeOptions(now.value, props.task))
const active = ref(0)
const picking = ref(false)
const custom = ref('')
const customEl = ref(null)
const customAt = computed(() => {
  const p = parseCapture(custom.value, { now: now.value })
  return p.remindAt
})

watch(
  () => props.show,
  async (on) => {
    if (!on) return
    now.value = new Date()
    active.value = 0
    picking.value = false
    custom.value = ''
    await nextTick()
    panel.value?.focus()
  },
)

function label(o) {
  // Today's and tomorrow's options name the clock; the rest the day too.
  const d = o.at
  const sameDay = d.toDateString() === now.value.toDateString()
  return sameDay ? formatTime(d) : formatDue(d.toISOString(), { now: now.value })
}

function pick(o) {
  emit('pick', o.at)
  close()
}
async function startCustom() {
  picking.value = true
  active.value = options.value.length
  await nextTick()
  place()
  customEl.value?.focus()
}
function applyCustom() {
  if (!customAt.value) return
  emit('pick', new Date(customAt.value))
  close()
}

function onKey(e) {
  if (e.isComposing) return
  const inField = e.target === customEl.value
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    if (picking.value && inField) {
      picking.value = false
      panel.value?.focus()
    } else close()
    return
  }
  if (inField) {
    if (e.key === 'Enter') {
      e.preventDefault()
      applyCustom()
    }
    e.stopPropagation()
    return
  }
  e.stopPropagation()
  const n = options.value.length
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    active.value = (active.value + 1) % (n + 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    active.value = (active.value + n) % (n + 1)
  } else if (e.key === 'Enter') {
    e.preventDefault()
    if (active.value === n) startCustom()
    else pick(options.value[active.value])
  } else if (/^[1-9]$/.test(e.key) && Number(e.key) <= n) {
    e.preventDefault()
    pick(options.value[Number(e.key) - 1])
  } else if (e.code === 'KeyD') {
    e.preventDefault()
    startCustom()
  }
}
</script>

<template>
  <Teleport to="body">
    <div v-if="show && sheet" v-layer class="sz-scrim" aria-hidden="true" />
    <div
      v-if="show"
      ref="panel"
      v-layer
      class="sz kit-menu"
      :class="{ sheet }"
      :style="sheet ? null : style"
      role="menu"
      tabindex="-1"
      :data-testid="testid"
      @keydown="onKey"
    >
      <span v-if="sheet" class="sz-handle" aria-hidden="true" />
      <div class="sz-head">
        <span class="sz-head-t">{{ t('inbox.snooze.title') }}</span>
        <span class="sz-head-now">{{ t('inbox.snooze.now', { time: formatTime(now) }) }}</span>
      </div>
      <button
        v-for="(o, i) in options"
        :key="o.key"
        type="button"
        class="sz-row"
        :class="{ on: active === i }"
        role="menuitem"
        :data-key="o.key"
        @mouseenter="active = i"
        @click="pick(o)"
      >
        <n-icon :component="ICONS[o.key]" :size="16" class="sz-ic" />
        <span class="sz-l">{{ t(`inbox.snooze.${o.key}`) }}</span>
        <span class="sz-at">{{ label(o) }}</span>
        <Kbd>{{ cap(i + 1) }}</Kbd>
      </button>
      <div class="sz-gap" />
      <button
        type="button"
        class="sz-row"
        :class="{ on: active === options.length, picking }"
        role="menuitem"
        data-key="pick"
        @mouseenter="active = options.length"
        @click="startCustom"
      >
        <n-icon :component="CalendarClearOutline" :size="16" class="sz-ic" />
        <span class="sz-l">{{ t('inbox.snooze.pick') }}</span>
        <Kbd>{{ cap('d') }}</Kbd>
      </button>
      <div v-if="picking" class="sz-pick">
        <input
          ref="customEl"
          v-model="custom"
          class="sz-input"
          :class="{ bad: custom.trim() && !customAt }"
          :placeholder="t('inbox.snooze.pickPlaceholder')"
          data-testid="snooze-custom"
        />
        <span class="sz-pick-at">{{
          customAt ? formatDue(customAt, { now }) : custom.trim() ? t('inbox.snooze.pickBad') : ''
        }}</span>
        <div class="sz-pick-hint">{{ t('inbox.snooze.pickHint') }}</div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.sz {
  position: fixed;
  z-index: 2060;
  width: 290px;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 6px;
  border-radius: 14px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
  outline: none;
}
.sz-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px 6px 10px;
  font-size: 11.5px;
  color: var(--t-text3);
}
.sz-head-t {
  flex: 1;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}
.sz-row {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 34px;
  padding: 0 6px 0 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
}
.sz-row.on {
  background: color-mix(in srgb, var(--t-primary) 22%, transparent);
}
.sz-row.on:first-of-type .sz-l {
  font-weight: 600;
}
.sz-ic {
  color: var(--t-text2);
}
.sz-row.on .sz-ic {
  color: var(--t-primary);
}
.sz-l {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sz-at {
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.sz-gap {
  height: 6px;
}
.sz-pick {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 6px 8px;
  align-items: center;
  padding: 6px 6px 4px;
}
.sz-input {
  height: 32px;
  padding: 0 10px;
  border: 0;
  outline: 0;
  border-radius: 9px;
  background: var(--t-input-bg);
  box-shadow: 0 0 0 2px var(--t-primary);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
}
.sz-input.bad {
  box-shadow: 0 0 0 2px #e0533d;
}
.sz-pick-at {
  font-size: 12.5px;
  color: var(--t-text2);
  white-space: nowrap;
}
.sz-pick-hint {
  grid-column: 1 / -1;
  font-size: 11.5px;
  line-height: 1.4;
  color: var(--t-text3);
}

/* ── a phone: the sheet (#2976) ── */
.sz-scrim {
  position: fixed;
  inset: 0;
  z-index: 2059;
  background: var(--t-scrim);
}
.sz.sheet {
  left: 0;
  right: 0;
  bottom: var(--kb-h, 0px);
  width: auto;
  padding: 0 10px calc(14px + var(--safe-bottom));
  border-radius: 18px 18px 0 0;
  box-shadow: var(--t-sh-drag);
}
.sz-handle {
  align-self: center;
  width: 36px;
  height: 5px;
  margin: 8px 0 6px;
  border-radius: 3px;
  background: var(--t-kbd);
}
.sz.sheet .sz-row {
  min-height: 48px;
  font-size: 15px;
}
.sz.sheet .sz-input {
  height: 44px;
  font-size: 16px;
}
</style>
