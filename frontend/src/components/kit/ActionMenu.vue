<script setup>
// The v2 row menu (#2969, mockup MenuA): rows with their key on the right, a
// colour row, a switch row and a two-step danger item.
//
// items: [
//   { key, label, icon, keys: '<catalog id>' | caps: ['⌘', '⌫'] },
//   { gap: true },
//   { key, label, icon, danger: true, confirm: { title, sub } }  // two-step
//   { key, label, icon, swatch: '#f0a020' }                      // colour row
//   { key, label, icon, toggle: true|false }                     // switch row
//   { key, label, icon, disabled: true, disabledTitle }          // no rights
//   { key, label, icon, stub: 3005 }                             // not built yet
//   { key, ..., busy: true } / { key, ..., failed: { title, sub } }
// ]
// Emits select(key, value) — value is the colour for a swatch row, the new
// state for a switch row.
//
// Keys: ↑/↓ move, ↵ activates, Esc closes, and every row answers to its own
// shortcut while the menu is open (the same key works on the row without it).
// The danger item arms on the first press — red, «Точно …?» with the cost —
// and fires on the second; it disarms after 4 s, on ↑/↓ or when left.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { AlertCircleOutline, LockClosedOutline, TrashOutline } from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import { useAnchoredPanel } from '@/composables/useAnchoredPanel'
import { byId } from '@/shortcuts/catalog'
import { matchChord, parseKeys } from '@/shortcuts/keymap'
import { hueGrad } from '@/utils/gradient'
import { PICKER_COLORS } from '@/utils/pickerColors'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'right-start' },
  offset: { type: Object, default: () => ({ x: 4, y: 0 }) },
  width: { type: Number, default: 300 },
  header: { type: String, default: '' },
  headerAside: { type: String, default: '' },
  items: { type: Array, default: () => [] },
  testid: { type: String, default: 'action-menu' },
  // Open with this two-step item already armed (⌘⌫ on a row: the first press
  // is the one that opened the menu).
  arm: { type: String, default: '' },
})
const emit = defineEmits(['update:show', 'select'])
const { t } = useI18n()

const panel = ref(null)
const showRef = computed(() => props.show)
function close() {
  emit('update:show', false)
}
const { style } = useAnchoredPanel({
  show: showRef,
  anchor: () => props.anchor(),
  panel,
  placement: props.placement,
  offset: props.offset,
  onOutside: close,
})

const rows = computed(() => props.items.filter((i) => !i.gap))
const live = (i) => !i.disabled && !i.stub && !i.busy
const active = ref(-1)
const armed = ref('')
let disarmTimer = null

function disarm() {
  armed.value = ''
  clearTimeout(disarmTimer)
}
function armItem(key) {
  armed.value = key
  clearTimeout(disarmTimer)
  disarmTimer = setTimeout(disarm, 4000)
}
onBeforeUnmount(() => clearTimeout(disarmTimer))

watch(
  () => props.show,
  async (on) => {
    disarm()
    if (!on) return
    active.value = rows.value.findIndex(live)
    const pre = props.arm && rows.value.find((i) => i.key === props.arm && i.confirm && live(i))
    if (pre) {
      armItem(pre.key)
      active.value = rows.value.indexOf(pre)
    }
    await nextTick()
    panel.value?.focus({ preventScroll: true })
  },
)

function activate(item, value) {
  if (!live(item)) return
  if (item.confirm && armed.value !== item.key) {
    armItem(item.key)
    active.value = rows.value.indexOf(item)
    return
  }
  disarm()
  if (item.swatch !== undefined) {
    emit('select', item.key, value)
    return
  }
  if (item.toggle !== undefined) {
    emit('select', item.key, !item.toggle)
    return
  }
  emit('select', item.key)
  // A two-step item keeps the menu open for its busy/failed states; the owner
  // closes it. Everything else closes on the spot.
  if (!item.confirm) close()
}

function move(step) {
  disarm()
  const list = rows.value
  if (!list.length) return
  let i = active.value
  for (let n = 0; n < list.length; n++) {
    i = (i + step + list.length) % list.length
    if (live(list[i])) break
  }
  active.value = i
}

function onKeydown(e) {
  if (e.isComposing) return
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    if (armed.value) disarm()
    else close()
    return
  }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    move(e.key === 'ArrowDown' ? 1 : -1)
    return
  }
  if (e.key === 'Enter') {
    e.preventDefault()
    const item = rows.value[active.value]
    if (item) activate(item)
    return
  }
  // A row's own shortcut.
  for (const item of rows.value) {
    const entry = item.keys && byId.get(item.keys)
    if (!entry?.keys) continue
    const chords = parseKeys(entry.keys)
    if (chords.length === 1 && matchChord(chords[0], e)) {
      e.preventDefault()
      e.stopPropagation()
      activate(item)
      return
    }
  }
}

function onLeave(item) {
  if (armed.value === item.key) disarm()
}

function stubTitle(item) {
  return t('common.stub.soon', { n: item.stub })
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="show"
      ref="panel"
      class="am kit-menu"
      role="menu"
      tabindex="-1"
      :style="{ ...style, width: `${width}px` }"
      :data-testid="testid"
      @keydown="onKeydown"
      @focusout="(e) => !panel?.contains(e.relatedTarget) && disarm()"
    >
      <div v-if="header" class="am-head">
        <span class="am-head-text">{{ header }}</span>
        <span v-if="headerAside" class="am-head-aside">{{ headerAside }}</span>
      </div>
      <template v-for="(item, idx) in items" :key="item.key || `gap-${idx}`">
        <div v-if="item.gap" class="am-gap" />

        <!-- two-step danger, armed -->
        <div
          v-else-if="item.confirm && armed === item.key && !item.busy && !item.failed"
          class="am-row armed"
          role="menuitem"
          :data-testid="`${testid}-${item.key}-armed`"
          @click="activate(item)"
          @mouseleave="onLeave(item)"
        >
          <n-icon :component="item.icon || TrashOutline" :size="16" />
          <span class="am-armed-text">
            <span class="am-armed-title">{{ item.confirm.title }}</span>
            <span v-if="item.confirm.sub" class="am-armed-sub">{{ item.confirm.sub }}</span>
          </span>
          <Kbd class="am-armed-cap">{{ CAPS.enter }}</Kbd>
        </div>

        <div v-else-if="item.busy" class="am-row danger-soft busy" role="menuitem" aria-busy="true">
          <TesseraSpinner :size="14" />
          <span class="am-label">{{ item.busyLabel || t('common.menu.deleting') }}</span>
        </div>

        <div
          v-else-if="item.failed"
          class="am-row danger-soft failed"
          role="menuitem"
          @click="activate(item)"
        >
          <n-icon :component="AlertCircleOutline" :size="16" class="am-red" />
          <span class="am-armed-text">
            <span class="am-failed-title">{{ item.failed.title }}</span>
            <span class="am-failed-sub">{{ item.failed.sub }}</span>
          </span>
          <span class="am-retry">{{ t('common.menu.retry') }}</span>
          <Kbd>{{ CAPS.enter }}</Kbd>
        </div>

        <div
          v-else
          class="am-row"
          role="menuitem"
          :class="{
            on: rows.indexOf(item) === active,
            danger: item.danger,
            disabled: item.disabled || item.stub,
            swatch: item.swatch !== undefined,
          }"
          :aria-disabled="item.disabled || !!item.stub || undefined"
          :title="item.stub ? stubTitle(item) : item.disabled ? item.disabledTitle : undefined"
          :data-testid="`${testid}-${item.key}`"
          @mouseenter="live(item) && (active = rows.indexOf(item))"
          @mouseleave="onLeave(item)"
          @click="item.swatch === undefined && activate(item)"
        >
          <n-icon v-if="item.icon" :component="item.icon" :size="16" class="am-icon" />
          <span class="am-label">{{ item.label }}</span>

          <span v-if="item.swatch !== undefined" class="am-dots">
            <button
              v-for="c in PICKER_COLORS"
              :key="c.hex"
              type="button"
              tabindex="-1"
              class="am-dot"
              :class="{ on: c.hex === item.swatch }"
              :style="{ background: hueGrad(c.hex), '--sw': c.hex }"
              :title="t(`common.colors.${c.key}`)"
              @click.stop="activate(item, c.hex)"
            />
          </span>
          <span v-else-if="item.toggle !== undefined" class="am-switch" :class="{ on: item.toggle }">
            <span class="am-knob" />
          </span>
          <n-icon v-else-if="item.disabled" :component="LockClosedOutline" :size="14" class="am-lock" />
          <span v-else-if="item.stub" class="am-soon">{{ t('common.stub.badge', { n: item.stub }) }}</span>
          <ShortcutKeys v-else-if="item.keys" :id="item.keys" />
          <span v-else-if="item.caps" class="am-caps">
            <Kbd v-for="(c, i) in item.caps" :key="i">{{ c }}</Kbd>
          </span>
        </div>
      </template>
    </div>
  </Teleport>
</template>

<style scoped>
.am {
  position: fixed;
  z-index: 2050;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px;
  box-sizing: border-box;
  border-radius: var(--t-r-card);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
  outline: 0;
  --t-surface: var(--t-pop);
}
.am-head {
  display: flex;
  gap: 8px;
  padding: 6px 10px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.am-head-text {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.am-head-aside {
  font-weight: 500;
  letter-spacing: 0;
  text-transform: none;
}
.am-gap {
  height: 6px;
}
.am-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 32px;
  padding: 0 6px 0 10px;
  box-sizing: border-box;
  border-radius: 7px;
  font-size: 13.5px;
  cursor: pointer;
  user-select: none;
}
.am-row.on {
  background: var(--t-hover);
}
.am-row.swatch {
  min-height: 36px;
  cursor: default;
}
.am-icon {
  color: var(--t-text2);
}
.am-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.am-row.danger .am-icon {
  color: #e0533d;
}
.am-row.danger.on {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 14%, var(--t-pop)),
    color-mix(in srgb, #e0533d 24%, var(--t-pop))
  );
}
.am-row.disabled {
  color: var(--t-placeholder);
  cursor: not-allowed;
}
.am-row.disabled .am-icon {
  color: var(--t-placeholder);
}
.am-lock {
  color: var(--t-text3);
}
.am-soon {
  font-size: 11.5px;
  color: var(--t-text3);
}
.am-caps {
  display: inline-flex;
  gap: 3px;
}
.am-row.armed {
  min-height: 46px;
  padding: 6px 6px 6px 10px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 86%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 86%, #fff)
  );
  color: #fff;
  box-shadow: 0 4px 14px rgba(224, 83, 61, 0.32);
}
.am-armed-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.am-armed-title {
  font-size: 13.5px;
  font-weight: 650;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.am-armed-sub {
  font-size: 11.5px;
  line-height: 15px;
  color: rgba(255, 255, 255, 0.86);
}
.am-armed-cap {
  background: rgba(255, 255, 255, 0.24);
  color: #fff;
  box-shadow: none;
}
.am-row.danger-soft {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 14%, var(--t-pop)),
    color-mix(in srgb, #e0533d 24%, var(--t-pop))
  );
}
.am-row.busy {
  padding: 0 10px;
  color: var(--t-text2);
  cursor: progress;
}
.am-row.failed {
  min-height: 46px;
  padding: 6px 6px 6px 10px;
}
.am-red {
  color: #e0533d;
}
.am-failed-title {
  font-size: 13.5px;
  font-weight: 600;
}
.am-failed-sub {
  font-size: 11.5px;
  color: var(--t-text2);
}
.am-retry {
  font-size: 12px;
  color: var(--t-text2);
}
.am-dots {
  display: inline-flex;
  gap: 7px;
  padding-right: 4px;
}
.am-dot {
  width: 14px;
  height: 14px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  cursor: pointer;
  transition: transform 0.12s ease;
}
.am-dot:hover {
  transform: scale(1.18);
}
.am-dot.on {
  box-shadow:
    0 0 0 2px var(--t-pop),
    0 0 0 4px var(--sw);
}
.am-switch {
  display: inline-flex;
  align-items: center;
  width: 30px;
  height: 18px;
  padding: 0 2px;
  box-sizing: border-box;
  border-radius: 9px;
  background: var(--t-pill);
  box-shadow: inset 0 1px 2px rgba(0, 0, 0, 0.2);
}
.am-knob {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--t-text3);
  transition: transform 0.15s ease;
}
.am-switch.on {
  background: var(--t-accent-grad);
}
.am-switch.on .am-knob {
  transform: translateX(12px);
  background: #fff;
}
</style>
