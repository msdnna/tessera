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
//   { key, label, icon, checked: true|false }                    // check mark
//   { key, label, dot: '#f0a020' }                 // a colour dot for an icon
//   { key, label, icon, disabled: true, disabledTitle }          // no rights
//   { key, label, icon, stub: 3005 }                             // not built yet
//   { key, ..., busy: true } / { key, ..., failed: { title, sub } }
//   { heading: 'Мои', aside: '2' }                               // section title
//   { key, label, glyph: 'layout-kanban', active, note, noteWarn, locked,
//     caps: ['1'] }                                // a list entry (#2987)
//   { key, label, icon, children: [...items], subWidth }  // a submenu (#3111)
// ]
// Emits select(key, value) — value is the colour for a swatch row, the new
// state for a switch row. A row of a submenu emits its own key.
//
// Keys: ↑/↓ move, ↵ activates, Esc closes, and every row answers to its own
// shortcut while the menu is open (the same key works on the row without it) —
// a row whose `caps` is one plain character (a digit) answers to that key too.
// The danger item arms on the first press — red, «Точно …?» with the cost —
// and fires on the second; it disarms after 4 s, on ↑/↓ or when left.
//
// A submenu (one level) opens to the right of the menu — to the left at the
// screen edge — on hover after a moment, on a click, or on → / ↵ with the
// cursor on its first row; ← and Esc close it and leave the cursor on its row.
// It lives inside the menu's element, so the pointer may travel onto it and a
// click there is not "outside". On a phone it is the sheet's second screen
// with «‹ Назад».
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AlertCircleOutline,
  CheckmarkOutline,
  ChevronBackOutline,
  ChevronForwardOutline,
  LockClosedOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import TesseraIcon from '@/components/TesseraIcon.vue'
import { useAnchoredPanel } from '@/composables/useAnchoredPanel'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { useResponsive } from '@/composables/useResponsive'
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
  // Drawn in place (design-system previews), not floating at an anchor.
  inline: { type: Boolean, default: false },
})
const emit = defineEmits(['update:show', 'select'])
const { t } = useI18n()

const panel = ref(null)
const showRef = computed(() => props.show && !props.inline)
function close() {
  emit('update:show', false)
}
// A phone (#2976 — package G «Ярусы на мобильном»): the menu is a sheet from
// the bottom over a scrim, rows 48px, the two-step item just as on a desktop;
// the system Back closes it.
const { isMobile } = useResponsive()
const sheet = computed(() => isMobile.value && !props.inline)
useOverlayBack(
  computed(() => props.show && sheet.value),
  close,
)
const { style } = useAnchoredPanel({
  show: showRef,
  anchor: () => props.anchor(),
  panel,
  placement: props.placement,
  offset: props.offset,
  onOutside: close,
})

const isRow = (i) => !i.gap && !i.heading
const rows = computed(() => props.items.filter(isRow))
const live = (i) => !i.disabled && !i.stub && !i.busy
const active = ref(-1)
const armed = ref('')
let disarmTimer = null

// The open submenu: the row that opened it, and the cursor inside it.
const sub = ref(null)
const subActive = ref(-1)
const subPanel = ref(null)
const subStyle = ref({ left: '-9999px', top: '0px' })
const subRows = computed(() => (sub.value?.children || []).filter(isRow))
let hoverTimer = null
const HOVER_OPEN_MS = 150
const HOVER_CLOSE_MS = 250

// What is drawn: the menu and, beside it, its open submenu — or, on a phone,
// the submenu alone as the sheet's second screen.
const levels = computed(() => {
  const inner = sub.value && { depth: 1, items: sub.value.children || [], rows: subRows.value }
  if (inner && sheet.value) return [inner]
  const out = [{ depth: 0, items: props.items, rows: rows.value }]
  if (inner) out.push(inner)
  return out
})
const cursorOf = (depth) => (depth ? subActive.value : active.value)

function disarm() {
  armed.value = ''
  clearTimeout(disarmTimer)
}
function armItem(key) {
  armed.value = key
  clearTimeout(disarmTimer)
  disarmTimer = setTimeout(disarm, 4000)
}
onBeforeUnmount(() => {
  clearTimeout(disarmTimer)
  clearTimeout(hoverTimer)
})

watch(
  () => props.show,
  async (on) => {
    disarm()
    clearTimeout(hoverTimer)
    sub.value = null
    subActive.value = -1
    if (!on) return
    active.value = rows.value.findIndex(live)
    const pre = props.arm && rows.value.find((i) => i.key === props.arm && i.confirm && live(i))
    if (pre) {
      armItem(pre.key)
      active.value = rows.value.indexOf(pre)
    }
    await nextTick()
    if (!props.inline) panel.value?.focus({ preventScroll: true })
  },
  // A menu mounted already open (inline, the design-system card) arms too.
  { immediate: true },
)

// The submenu stands beside the menu, its first row level with the row that
// opened it; with no room on the right it opens to the left.
function placeSub() {
  const el = subPanel.value
  const root = panel.value
  if (!el || !root || !sub.value || sheet.value || props.inline) return
  const row = [...root.querySelectorAll('[data-am-sub]')].find(
    (n) => n.getAttribute('data-am-sub') === sub.value.key,
  )
  if (!row) return
  const p = root.getBoundingClientRect()
  const r = row.getBoundingClientRect()
  const w = el.offsetWidth
  const h = el.offsetHeight
  const margin = 8
  let left = p.right + 2
  if (left + w > window.innerWidth - margin) left = p.left - w - 2
  left = Math.max(margin, left)
  const top = Math.min(
    Math.max(margin, r.top - 6),
    Math.max(margin, window.innerHeight - h - margin),
  )
  subStyle.value = { left: `${Math.round(left)}px`, top: `${Math.round(top)}px` }
}
watch(style, () => nextTick(placeSub))

function openSub(item, cursorOnFirst) {
  clearTimeout(hoverTimer)
  disarm()
  if (sub.value !== item) {
    sub.value = item
    subStyle.value = { left: '-9999px', top: '0px' }
  }
  active.value = rows.value.indexOf(item)
  subActive.value = cursorOnFirst ? subRows.value.findIndex(live) : -1
  nextTick(placeSub)
}
function closeSub() {
  clearTimeout(hoverTimer)
  if (!sub.value) return
  disarm()
  active.value = rows.value.indexOf(sub.value)
  sub.value = null
  subActive.value = -1
}

function activate(item, value) {
  if (!live(item)) return
  if (item.children) {
    openSub(item, true)
    return
  }
  if (item.confirm && armed.value !== item.key) {
    armItem(item.key)
    if (subRows.value.includes(item)) subActive.value = subRows.value.indexOf(item)
    else active.value = rows.value.indexOf(item)
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
  const inSub = !!sub.value
  const list = inSub ? subRows.value : rows.value
  if (!list.length) return
  let i = inSub ? subActive.value : active.value
  for (let n = 0; n < list.length; n++) {
    i = (i + step + list.length) % list.length
    if (live(list[i])) break
  }
  if (inSub) subActive.value = i
  else active.value = i
}

function onKeydown(e) {
  if (e.isComposing) return
  const list = sub.value ? subRows.value : rows.value
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    if (armed.value) disarm()
    else if (sub.value) closeSub()
    else close()
    return
  }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    move(e.key === 'ArrowDown' ? 1 : -1)
    return
  }
  if (e.key === 'ArrowLeft' && sub.value) {
    e.preventDefault()
    closeSub()
    return
  }
  if (e.key === 'ArrowRight' && !sub.value) {
    const item = rows.value[active.value]
    if (item?.children && live(item)) {
      e.preventDefault()
      openSub(item, true)
    }
    return
  }
  if (e.key === 'Enter') {
    e.preventDefault()
    const item = list[cursorOf(sub.value ? 1 : 0)]
    if (item) activate(item)
    return
  }
  // A row captioned by one plain key (a digit) answers to it.
  if (!hasMod(e) && e.key.length === 1) {
    const hit = list.find((i) => i.caps?.length === 1 && i.caps[0] === e.key)
    if (hit) {
      e.preventDefault()
      e.stopPropagation()
      activate(hit)
      return
    }
  }
  // A row's own shortcut.
  for (const item of list) {
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

function hasMod(e) {
  return e.metaKey || e.ctrlKey || e.altKey
}

// The pointer on a row makes it the cursor. A row with a submenu opens it after
// a moment; any other row of the menu closes the open one after a longer
// moment — the pointer may only be passing on its way into the submenu.
function onHover(depth, item) {
  if (!live(item)) return
  clearTimeout(hoverTimer)
  if (depth) {
    subActive.value = subRows.value.indexOf(item)
    return
  }
  active.value = rows.value.indexOf(item)
  if (sheet.value) return
  if (item.children) {
    if (sub.value !== item) hoverTimer = setTimeout(() => openSub(item, false), HOVER_OPEN_MS)
  } else if (sub.value) {
    hoverTimer = setTimeout(closeSub, HOVER_CLOSE_MS)
  }
}
function holdSub() {
  clearTimeout(hoverTimer)
}

function onLeave(item) {
  if (armed.value === item.key) disarm()
}

function stubTitle(item) {
  return t('common.stub.soon', { n: item.stub })
}
</script>

<template>
  <Teleport to="body" :disabled="inline">
    <div v-if="show && sheet" class="am-scrim" aria-hidden="true" />
    <div
      v-if="show"
      ref="panel"
      class="am kit-menu"
      :class="{ inline, sheet }"
      role="menu"
      tabindex="-1"
      :style="inline ? { width: `${width}px` } : sheet ? null : { ...style, width: `${width}px` }"
      :data-testid="testid"
      @keydown="onKeydown"
      @focusout="(e) => !panel?.contains(e.relatedTarget) && disarm()"
    >
      <span v-if="sheet" class="am-handle" aria-hidden="true" />
      <template v-if="sheet && sub">
        <div
          class="am-row am-back"
          role="menuitem"
          :data-testid="`${testid}-back`"
          @click="closeSub"
        >
          <n-icon :component="ChevronBackOutline" :size="18" class="am-icon" />
          <span class="am-label">{{ t('common.menu.back') }}</span>
        </div>
        <div class="am-head">
          <span class="am-head-text">{{ sub.label }}</span>
        </div>
      </template>
      <div v-else-if="header" class="am-head">
        <span class="am-head-text">{{ header }}</span>
        <span v-if="headerAside" class="am-head-aside">{{ headerAside }}</span>
      </div>
      <div
        v-for="lvl in levels"
        :key="lvl.depth"
        :ref="(el) => lvl.depth && !sheet && (subPanel = el)"
        :class="lvl.depth && !sheet ? 'am am-sub' : 'am-level'"
        :role="lvl.depth && !sheet ? 'menu' : undefined"
        :style="
          lvl.depth && !sheet ? { ...subStyle, width: `${sub.subWidth || 240}px` } : undefined
        "
        :data-testid="lvl.depth && !sheet ? `${testid}-sub` : undefined"
        @mouseenter="lvl.depth && holdSub()"
      >
        <template v-for="(item, idx) in lvl.items" :key="item.key || `gap-${idx}`">
          <div v-if="item.gap" class="am-gap" />
          <div v-else-if="item.heading" class="am-head am-section">
            <span class="am-head-text">{{ item.heading }}</span>
            <span v-if="item.aside" class="am-head-aside">{{ item.aside }}</span>
          </div>

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

          <div
            v-else-if="item.busy"
            class="am-row danger-soft busy"
            role="menuitem"
            aria-busy="true"
          >
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
              on: lvl.rows.indexOf(item) === cursorOf(lvl.depth),
              open: !!item.children && sub === item,
              current: item.active,
              danger: item.danger,
              disabled: item.disabled || item.stub,
              swatch: item.swatch !== undefined,
            }"
            :aria-disabled="item.disabled || !!item.stub || undefined"
            :aria-haspopup="item.children ? 'menu' : undefined"
            :aria-expanded="item.children ? sub === item : undefined"
            :aria-checked="item.checked !== undefined ? !!item.checked : undefined"
            :title="item.stub ? stubTitle(item) : item.disabled ? item.disabledTitle : undefined"
            :data-testid="`${testid}-${item.key}`"
            :data-am-sub="item.children ? item.key : undefined"
            @mouseenter="onHover(lvl.depth, item)"
            @mouseleave="onLeave(item)"
            @click="item.swatch === undefined && activate(item)"
          >
            <TesseraIcon v-if="item.glyph" :name="item.glyph" :size="15" class="am-icon am-glyph" />
            <span
              v-else-if="item.dot"
              class="am-dot-icon"
              :style="{ background: hueGrad(item.dot) }"
              aria-hidden="true"
            />
            <n-icon v-else-if="item.icon" :component="item.icon" :size="16" class="am-icon" />
            <span class="am-label">{{ item.label }}</span>
            <span v-if="item.note" class="am-note" :class="{ warn: item.noteWarn }">{{
              item.note
            }}</span>
            <n-icon v-if="item.locked" :component="LockClosedOutline" :size="13" class="am-lock" />
            <n-icon v-if="item.checked" :component="CheckmarkOutline" :size="15" class="am-check" />

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
            <span
              v-else-if="item.toggle !== undefined"
              class="am-switch"
              :class="{ on: item.toggle }"
            >
              <span class="am-knob" />
            </span>
            <n-icon
              v-else-if="item.disabled"
              :component="LockClosedOutline"
              :size="14"
              class="am-lock"
            />
            <span v-else-if="item.stub" class="am-soon">{{
              t('common.stub.badge', { n: item.stub })
            }}</span>
            <ShortcutKeys v-else-if="item.keys" :id="item.keys" />
            <span v-else-if="item.caps" class="am-caps">
              <Kbd v-for="(c, i) in item.caps" :key="i">{{ c }}</Kbd>
            </span>
            <n-icon
              v-if="item.children"
              :component="ChevronForwardOutline"
              :size="14"
              class="am-chev"
            />
          </div>
        </template>
      </div>
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
.am.inline {
  position: relative;
  z-index: auto;
}
.am-level {
  display: contents;
}
/* The submenu (#3111): the same panel, fixed beside the menu. */
.am-sub {
  z-index: 2051;
}
.am-row.open {
  background: var(--t-hover);
}
.am-chev {
  flex: none;
  margin-left: -2px;
  color: var(--t-text3);
}
.am-check {
  flex: none;
  color: var(--t-primary);
}
.am-dot-icon {
  flex: none;
  width: 9px;
  height: 9px;
  margin: 0 3.5px;
  border-radius: 50%;
}
.am-back {
  color: var(--t-text2);
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
.am-section {
  padding-top: 8px;
}
.am-row.current {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 26%, var(--t-pop))
  );
  font-weight: 600;
}
.am-row.current .am-glyph {
  color: var(--t-primary);
}
.am-glyph {
  color: var(--t-text3);
}
.am-note {
  flex: none;
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.am-note.warn {
  color: #f0a020;
  font-weight: 600;
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

/* ── a phone: the sheet (#2976) ── */
.am-scrim {
  position: fixed;
  inset: 0;
  z-index: 2049;
  background: var(--t-scrim);
}
.am.sheet {
  left: 0;
  right: 0;
  bottom: 0;
  max-height: calc(100% - var(--safe-top) - 24px);
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 0 10px calc(14px + var(--safe-bottom));
  border-radius: 18px 18px 0 0;
  box-shadow: var(--t-sh-drag);
  animation: am-up 0.2s ease;
}
.am-handle {
  flex: none;
  align-self: center;
  width: 36px;
  height: 5px;
  margin: 8px 0 6px;
  border-radius: 3px;
  background: var(--t-kbd);
}
.am.sheet .am-row {
  min-height: 48px;
  gap: 14px;
  padding: 0 10px 0 12px;
  border-radius: 10px;
  font-size: 15.5px;
}
.am.sheet .am-row.on:not(.current) {
  background: transparent;
}
.am.sheet .am-row.current {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 26%, var(--t-pop))
  );
}
.am.sheet .am-head {
  padding: 10px 12px 6px;
  font-size: 12px;
}
@keyframes am-up {
  from {
    transform: translateY(24px);
    opacity: 0.6;
  }
}
@media (prefers-reduced-motion: reduce) {
  .am.sheet {
    animation: none;
  }
}
</style>
