<script setup>
// One picker for every property of a task (v2, #3118 — mockups PropPicker,
// PackageJ «Свойства задачи»): status, priority, assignees, estimate, stage,
// parent, the relation form. Built on the TagPicker frame: a header or a search
// field with the field's key, sections with a count, rows of 34px (44 with a
// second line), a key footer. The same rows render in three shells:
//
//   pop      a popover at its anchor (kit/AnchorLayer — so it also works inside
//            the task modal), a bottom sheet on a phone (search 44, rows 48);
//   palette  drawn in place inside the ⌘K palette: rows 48, no field of its own
//            (the palette's input is the field), no footer.
//
// The picker only draws and steers. Its rows are handed in ready-made (see
// components/task/props/propItems.js), each with its own `run`; a row marked
// `close` shuts the picker once it ran (single choice), the others keep it open
// (several people, a toggle).
//
// Keys, with the focus in the picker: ↑↓ move, ↵ runs the row under the cursor,
// Space toggles a multi row once the arrows picked one (until then it types),
// digits run the row carrying that digit when the focus is in the list (no
// field, or after the arrows — in a field a digit is input), the field's own
// key (`cycleKey`) asks the host for the next field, Esc closes the picker and
// only the picker. `onKey` sees every key first, for the field's own keys
// (], E, ⇧⌫, ⌘↵, ⌥1–4…).
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AlertCircleOutline,
  CheckmarkOutline,
  ChevronForwardOutline,
  LockClosedOutline,
  SearchOutline,
} from '@vicons/ionicons5'
import AnchorLayer from './AnchorLayer.vue'
import Kbd from '../Kbd.vue'
import ShortcutKeys from '../ShortcutKeys.vue'
import TesseraIcon from '../TesseraIcon.vue'
import UserAvatar from '../UserAvatar.vue'
import { keyOf } from '@/shortcuts/keymap'
import { hasModKey } from '@/utils/keys'
import { hueGrad } from '@/utils/gradient'
import { CAPS } from '@/utils/keyCaps'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'below-or-side' },
  offset: { type: Object, default: () => ({ x: 0, y: 6 }) },
  // 'pop' (a popover, a sheet on a phone) or 'palette' (in place, ⌘K metrics).
  shell: { type: String, default: 'pop' },
  // Drawn in place instead of at an anchor (the palette, design previews).
  inline: { type: Boolean, default: false },
  width: { type: Number, default: 300 },
  // The field's name: the header line («Статус  S») when `header`, the title
  // of the sheet on a phone always.
  title: { type: String, default: '' },
  header: { type: Boolean, default: true },
  // The catalog key of the field, shown in the header or the search field.
  keyId: { type: String, default: '' },
  search: { type: Boolean, default: false },
  placeholder: { type: String, default: '' },
  query: { type: String, default: '' },
  // The rows: { kind: 'section'|'gap'|'note'|'row', … } — see propItems.js.
  items: { type: Array, default: () => [] },
  // The id of the row the cursor starts on (the current value).
  cursor: { type: String, default: '' },
  footer: { type: Array, default: () => [] },
  // «Только просмотр …»: a lock banner over a dimmed list nobody can pick from.
  readonly: { type: String, default: '' },
  loading: { type: Boolean, default: false },
  // { title, sub } — a failed load, with «Повторить ↵».
  error: { type: Object, default: null },
  // { icon, title, sub } — nothing to show.
  empty: { type: Object, default: null },
  // The field's own key: pressed again in the list, it asks for the next field.
  cycleKey: { type: String, default: '' },
  // A selector inside the picker to focus on open (a field of the top slot);
  // otherwise the search field, or the picker itself.
  focus: { type: String, default: '' },
  onKey: { type: Function, default: null },
  testid: { type: String, default: 'prop-picker' },
  searchTestid: { type: String, default: '' },
})
const emit = defineEmits(['update:show', 'update:query', 'pick', 'cycle', 'retry'])
const { t } = useI18n()

const InPlace = (_, { slots }) => slots.default?.()
const layerProps = computed(() =>
  props.inline
    ? {}
    : {
        show: props.show,
        anchor: props.anchor,
        placement: props.placement,
        offset: props.offset,
        testid: props.testid,
        'onUpdate:show': (v) => !v && close(),
      },
)
const palette = computed(() => props.shell === 'palette')

const q = ref(props.query)
watch(
  () => props.query,
  (v) => (q.value = v),
)
watch(q, (v) => {
  if (v !== props.query) emit('update:query', v)
  navigated.value = false
})

const root = ref(null)
const input = ref(null)
const listEl = ref(null)
const active = ref(0)
// The arrows moved the cursor: Space and digits act on the list from now on,
// and the cursor row wears the keyboard ring.
const navigated = ref(false)

const rows = computed(() => (props.readonly ? [] : props.items.filter((i) => i.kind === 'row')))
const pickable = (r) => r && !r.disabled
function indexOf(id) {
  return rows.value.findIndex((r) => r.id === id)
}
function placeCursor() {
  const i = props.cursor ? indexOf(props.cursor) : -1
  const first = rows.value.findIndex(pickable)
  active.value = i >= 0 ? i : Math.max(0, first)
}
// A new list (a query typed, a toggle re-rendered it) keeps the cursor on the
// same row when it is still there.
watch(rows, (now, before) => {
  const id = before?.[active.value]?.id
  const i = id ? now.findIndex((r) => r.id === id) : -1
  if (i >= 0) active.value = i
  else if (q.value) active.value = Math.max(0, now.findIndex(pickable))
  else placeCursor()
})

async function focusIn() {
  await nextTick()
  await nextTick()
  if (palette.value) return
  const own = props.focus ? root.value?.querySelector(props.focus) : null
  if (own) own.focus({ preventScroll: true })
  else if (props.search && input.value) input.value.focus({ preventScroll: true })
  else root.value?.focus({ preventScroll: true })
}
watch(
  () => props.show,
  (on) => {
    if (!on) return
    q.value = props.query
    navigated.value = false
    placeCursor()
    focusIn()
  },
  { immediate: true },
)

function close() {
  emit('update:show', false)
}

function move(step) {
  const n = rows.value.length
  if (!n) return
  let i = active.value
  for (let k = 0; k < n; k++) {
    i = (i + step + n) % n
    if (pickable(rows.value[i])) break
  }
  active.value = i
  navigated.value = true
  nextTick(() => listEl.value?.querySelector('.pp-row.on')?.scrollIntoView?.({ block: 'nearest' }))
}

function pick(row) {
  if (!pickable(row) || props.readonly) return
  emit('pick', row)
  const out = row.run?.()
  if (row.close) close()
  return out
}

const api = {
  get query() {
    return q.value
  },
  set query(v) {
    q.value = v
  },
  get navigated() {
    return navigated.value
  },
  get row() {
    return rows.value[active.value] || null
  },
  close,
  move,
  pick,
  focus: focusIn,
}

// Keys of the palette shell come from the palette's own input (it forwards
// them); keys of the popover from the picker itself.
function handleKey(e) {
  if (e.isComposing) return false
  if (props.onKey && props.onKey(e, api)) return true
  if (e.key === 'Escape') {
    if (palette.value) return false
    e.preventDefault()
    e.stopPropagation()
    close()
    return true
  }
  if (props.readonly) return false
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    move(e.key === 'ArrowDown' ? 1 : -1)
    return true
  }
  if (e.key === 'Enter' && !hasModKey(e) && !e.altKey) {
    e.preventDefault()
    pick(rows.value[active.value])
    return true
  }
  const plain = !hasModKey(e) && !e.altKey && !e.ctrlKey && !e.metaKey
  const typing = props.search || palette.value
  if (e.key === ' ' && plain && (navigated.value || !typing)) {
    const row = rows.value[active.value]
    if (row?.check === 'multi') {
      e.preventDefault()
      pick(row)
      return true
    }
  }
  const k = keyOf(e)
  if (k && /^\d$/.test(k) && plain && !e.shiftKey) {
    const row = rows.value.find((r) => r.digit === k)
    if (row && (!typing || navigated.value || !q.value)) {
      e.preventDefault()
      pick(row)
      return true
    }
  }
  if (
    props.cycleKey &&
    k === props.cycleKey &&
    plain &&
    !e.shiftKey &&
    (!typing || navigated.value)
  ) {
    e.preventDefault()
    emit('cycle')
    return true
  }
  return false
}
function onKeydown(e) {
  if (palette.value) return
  handleKey(e)
}

// Section counts and the label of the field.
const showHeader = computed(() => !palette.value && props.header && !!props.title)
const noRows = computed(() => !props.loading && !props.error && !rows.value.length)
const rowIndex = (row) => rows.value.indexOf(row)
const isOn = (row) => rowIndex(row) === active.value && pickable(row)

function onRowEnter(row) {
  if (!pickable(row)) return
  active.value = rowIndex(row)
  navigated.value = false
}
function progressStyle(p) {
  const pct = p.total ? Math.round((p.done / p.total) * 100) : 0
  return { width: `${pct}%` }
}

defineExpose({ handleKey, move, pick, api, root })
</script>

<template>
  <component :is="inline ? InPlace : AnchorLayer" v-bind="layerProps">
    <div
      ref="root"
      class="pp"
      :class="[`pp-${shell}`, { 'pp-ro': !!readonly }]"
      :style="palette ? null : { width: `${width}px` }"
      role="dialog"
      :aria-label="title || placeholder"
      tabindex="-1"
      :data-testid="`${testid}-card`"
      @keydown="onKeydown"
    >
      <!-- A phone's sheet: the field's name and «Готово» -->
      <div v-if="!palette" class="pp-shead">
        <span class="pp-shead-t">{{ title || placeholder }}</span>
        <button
          type="button"
          class="pp-shead-done"
          :data-testid="`${testid}-done`"
          @mousedown.prevent
          @click="close"
        >
          {{ t('props.sheetDone') }}
        </button>
      </div>

      <div v-if="readonly" class="pp-robar" :data-testid="`${testid}-readonly`">
        <n-icon :component="LockClosedOutline" :size="14" class="pp-dim" />
        <span>{{ readonly }}</span>
      </div>

      <div v-if="showHeader" class="pp-hdr">
        <span class="pp-hdr-t">{{ title }}</span>
        <ShortcutKeys v-if="keyId" :id="keyId" />
      </div>

      <slot name="top" />

      <label v-if="search && !palette" class="pp-field">
        <n-icon :component="SearchOutline" :size="14" class="pp-dim" />
        <input
          ref="input"
          v-model="q"
          class="pp-input"
          :placeholder="placeholder"
          :aria-label="placeholder"
          :disabled="!!readonly"
          :data-testid="searchTestid || `${testid}-search`"
          spellcheck="false"
          autocomplete="off"
        />
        <ShortcutKeys v-if="keyId" :id="keyId" class="pp-fkey" />
      </label>

      <div ref="listEl" class="pp-list" :class="{ dim: !!readonly }">
        <template v-if="loading">
          <div v-for="w in [46, 62, 38]" :key="w" class="pp-sk">
            <span class="pp-sk-a" />
            <span class="pp-sk-l" :style="{ width: `${w}%` }" />
            <span class="pp-grow" />
            <span class="pp-sk-m" />
          </div>
        </template>
        <div v-else-if="error" class="pp-err" :data-testid="`${testid}-error`">
          <span class="pp-err-i"><n-icon :component="AlertCircleOutline" :size="15" /></span>
          <span class="pp-err-tx">
            <span class="pp-err-t">{{ error.title }}</span>
            <span v-if="error.sub" class="pp-err-s">{{ error.sub }}</span>
          </span>
          <button type="button" class="pp-err-b" @mousedown.prevent @click="emit('retry')">
            {{ t('props.retry') }}<Kbd>{{ CAPS.enter }}</Kbd>
          </button>
        </div>
        <template v-else>
          <div v-if="noRows && empty" class="pp-empty" :data-testid="`${testid}-empty`">
            <span v-if="empty.icon" class="pp-empty-i"
              ><n-icon :component="empty.icon" :size="18"
            /></span>
            <span class="pp-empty-t">{{ empty.title }}</span>
            <span v-if="empty.sub" class="pp-empty-s">{{ empty.sub }}</span>
          </div>
          <template v-for="(it, i) in items" :key="it.id || `${it.kind}${i}`">
            <div v-if="it.kind === 'section'" class="pp-sec">
              <span class="pp-grow">{{ it.title }}</span>
              <span v-if="it.count != null && it.count !== ''" class="pp-sec-n">{{
                it.count
              }}</span>
            </div>
            <div v-else-if="it.kind === 'gap'" class="pp-gap" />
            <div v-else-if="it.kind === 'note'" class="pp-note">{{ it.text }}</div>
            <slot v-else-if="it.kind === 'slot'" :name="it.name" :item="it" />
            <div
              v-else-if="it.kind === 'row'"
              class="pp-row"
              :class="{
                on: !readonly && isOn(it),
                kf: !readonly && isOn(it) && navigated,
                two: !!it.two,
                off: it.disabled,
                muted: it.muted,
              }"
              :role="it.check === 'multi' ? 'menuitemcheckbox' : 'menuitem'"
              :aria-checked="it.check ? String(!!it.on) : undefined"
              :aria-disabled="it.disabled ? 'true' : undefined"
              :title="it.tip || undefined"
              :data-testid="it.testid || `${testid}-row`"
              :data-row="it.id"
              @mouseenter="onRowEnter(it)"
              @mousedown.prevent
              @click="pick(it)"
            >
              <span v-if="it.check === 'multi'" class="pp-box" :class="{ on: it.on }">
                <n-icon v-if="it.on" :component="CheckmarkOutline" :size="12" />
              </span>
              <TesseraIcon
                v-if="it.status"
                :name="it.status.name"
                :size="15"
                class="pp-lead"
                :style="{ color: it.status.color }"
              />
              <span v-else-if="it.dot" class="pp-dot" :style="{ background: hueGrad(it.dot) }" />
              <UserAvatar
                v-else-if="it.avatar"
                class="pp-ava"
                :class="{ ext: it.avatar.ext }"
                :user-id="it.avatar.userId || ''"
                :src="it.avatar.src || ''"
                :name="it.avatar.name || it.label"
              />
              <n-icon v-else-if="it.icon" :component="it.icon" :size="16" class="pp-ico" />
              <span v-if="it.mono" class="pp-mono">{{ it.mono }}</span>
              <span class="pp-text" :class="{ two: !!it.two }">
                <span class="pp-label">{{ it.label }}</span>
                <span v-if="it.sub" class="pp-sub">{{ it.sub }}</span>
              </span>
              <span v-if="it.badge" class="pp-badge">{{ it.badge }}</span>
              <n-icon v-if="it.lock" :component="LockClosedOutline" :size="13" class="pp-dim" />
              <span v-if="it.progress" class="pp-prog">
                <span class="pp-bar"><span :style="progressStyle(it.progress)" /></span>
                {{ it.progress.done }}/{{ it.progress.total }}
              </span>
              <span v-if="it.meta" class="pp-meta">{{ it.meta }}</span>
              <span v-if="it.digit || it.caps" class="pp-caps">
                <Kbd v-for="c in it.caps || [it.digit]" :key="c">{{ c }}</Kbd>
              </span>
              <span v-if="it.check === 'single' && it.on" class="pp-ck">
                <n-icon :component="CheckmarkOutline" :size="12" />
              </span>
              <n-icon v-if="it.chev" :component="ChevronForwardOutline" :size="14" class="pp-dim" />
            </div>
          </template>
        </template>
      </div>

      <div v-if="footer.length && !palette" class="pp-foot">
        <template v-for="(f, i) in footer" :key="i">
          <span v-if="f === '|'" class="pp-grow" />
          <span v-else class="pp-fk">
            <Kbd v-for="c in f.caps" :key="c">{{ c }}</Kbd>
            <template v-if="f.label">{{ f.label }}</template>
          </span>
        </template>
      </div>
    </div>
  </component>
</template>

<style scoped>
.pp {
  box-sizing: border-box;
  max-width: calc(100vw - 16px);
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px;
  border-radius: var(--t-r-card);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
  outline: none;
  --t-surface: var(--t-pop);
}
.pp-shead {
  display: none;
}
.pp-robar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 2px 2px 4px;
  padding: 8px 10px;
  border-radius: 9px;
  background: var(--t-sub);
  font-size: 12.5px;
  line-height: 1.4;
  color: var(--t-text2);
}
.pp-dim {
  flex: none;
  color: var(--t-text3);
}
.pp-hdr {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 34px;
  padding: 0 6px 0 10px;
  font-size: 14px;
  font-weight: 650;
}
.pp-hdr-t {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pp-field {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  margin: 2px 2px 4px;
  padding: 0 6px 0 10px;
  border-radius: 9px;
  background: var(--t-input-bg);
  box-shadow: 0 0 0 2px var(--t-primary);
  cursor: text;
}
.pp-input {
  flex: 1;
  min-width: 0;
  height: 100%;
  padding: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
}
.pp-input::placeholder {
  color: var(--t-placeholder);
}
.pp-fkey {
  flex: none;
}
.pp-list {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: min(420px, 60vh);
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
}
.pp-list.dim {
  opacity: 0.45;
  pointer-events: none;
}
.pp-sec {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.pp-sec-n {
  font-weight: 500;
  letter-spacing: 0;
  text-transform: none;
}
.pp-gap {
  flex: none;
  height: 6px;
}
.pp-note {
  padding: 4px 10px 6px;
  font-size: 12px;
  line-height: 1.45;
  color: var(--t-text3);
  text-wrap: pretty;
}
.pp-row {
  flex: none;
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 34px;
  padding: 0 8px 0 10px;
  box-sizing: border-box;
  border-radius: 7px;
  font-size: 13.5px;
  cursor: pointer;
  user-select: none;
}
.pp-row.two {
  height: 44px;
}
.pp-row.on {
  background: var(--t-hover);
}
.pp-row.kf {
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.pp-row.off {
  cursor: default;
}
.pp-row.off .pp-label,
.pp-row.muted .pp-label {
  color: var(--t-text3);
}
.pp-box {
  flex: 0 0 16px;
  width: 16px;
  height: 16px;
  border-radius: 5px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--t-pill);
  color: #fff;
}
.pp-box.on {
  background: var(--t-accent-grad);
}
.pp-lead {
  flex: none;
}
.pp-dot {
  flex: 0 0 10px;
  width: 10px;
  height: 10px;
  margin: 0 3px;
  border-radius: 50%;
}
.pp-ava {
  flex: none;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 9.5px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.pp-ava.ext {
  background: var(--t-text3);
}
.pp-ico {
  flex: none;
  color: var(--t-text2);
}
.pp-mono {
  flex: none;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.pp-text {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 8px;
  overflow: hidden;
  white-space: nowrap;
}
.pp-text.two {
  flex-direction: column;
  align-items: stretch;
  gap: 1px;
}
.pp-label,
.pp-sub {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.pp-label {
  flex: 0 1 auto;
}
.pp-sub {
  flex: 0 1 auto;
  font-size: 12px;
  color: var(--t-text3);
}
.pp-badge {
  flex: none;
  height: 18px;
  display: flex;
  align-items: center;
  padding: 0 7px;
  border-radius: 999px;
  background: var(--t-pill);
  font-size: 11px;
  color: var(--t-text2);
  white-space: nowrap;
}
.pp-prog {
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.pp-bar {
  width: 36px;
  height: 4px;
  border-radius: 2px;
  display: flex;
  overflow: hidden;
  background: color-mix(in srgb, var(--t-text3) 30%, transparent);
}
.pp-bar > span {
  background: var(--t-accent-grad);
}
.pp-meta {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.pp-caps {
  flex: none;
  display: flex;
  gap: 3px;
}
.pp-ck {
  flex: 0 0 18px;
  width: 18px;
  height: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: var(--t-accent-grad);
  color: #fff;
}
.pp-grow {
  flex: 1;
}
.pp-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 16px 12px 12px;
  text-align: center;
}
.pp-empty-i {
  width: 36px;
  height: 36px;
  margin-bottom: 4px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--t-pill);
  color: var(--t-text3);
}
.pp-empty-t {
  font-size: 13.5px;
  font-weight: 600;
}
.pp-empty-s {
  font-size: 12.5px;
  color: var(--t-text3);
  text-wrap: pretty;
}
.pp-sk {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 34px;
  padding: 0 10px;
}
.pp-sk-a {
  width: 22px;
  height: 22px;
  border-radius: 11px;
  background: var(--t-pill);
}
.pp-sk-l {
  height: 10px;
  border-radius: 5px;
  background: var(--t-pill);
}
.pp-sk-m {
  width: 28px;
  height: 10px;
  border-radius: 5px;
  background: var(--t-pill);
  opacity: 0.6;
}
.pp-err {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 4px 2px;
  padding: 10px;
  border-radius: 10px;
  background: var(--t-sub);
}
.pp-err-i {
  flex: 0 0 28px;
  width: 28px;
  height: 28px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 16%, var(--t-sub)),
    color-mix(in srgb, #e0533d 28%, var(--t-sub))
  );
  color: #e0533d;
}
.pp-err-tx {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.pp-err-t {
  font-size: 13px;
  font-weight: 600;
}
.pp-err-s {
  font-size: 12px;
  color: var(--t-text3);
}
.pp-err-b {
  flex: none;
  height: 28px;
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 0 5px 0 10px;
  border: 0;
  border-radius: 8px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-sub)),
    color-mix(in srgb, var(--t-primary) 28%, var(--t-sub))
  );
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
.pp-foot {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 34px;
  margin: 4px -6px -6px;
  padding: 0 12px;
  border-radius: 0 0 var(--t-r-card) var(--t-r-card);
  background: color-mix(in srgb, var(--t-pop) 55%, var(--t-bg));
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.pp-fk {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.pp-fk :deep(.kbd) + :deep(.kbd) {
  margin-left: -2px;
}

/* ── the palette shell: ⌘K metrics, no card of its own ── */
.pp-palette {
  width: auto;
  max-width: none;
  padding: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}
.pp-palette .pp-list {
  max-height: none;
  overflow: visible;
}
.pp-palette .pp-row {
  height: 48px;
  padding: 0 10px;
  border-radius: 10px;
  font-size: 14px;
}
.pp-palette .pp-row.two {
  height: 56px;
}

/* ── a phone: the sheet is the card — its own header with «Готово», a 44px
   field, 48px rows, no key hints ── */
.kit-layer:not(.al) .pp {
  width: auto !important;
  max-width: none;
  padding: 0;
  background: transparent;
  box-shadow: none;
}
.kit-layer:not(.al) .pp-shead {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 48px;
  margin: -8px -8px 2px 2px;
}
.pp-shead-t {
  flex: 1;
  font-size: 17px;
  font-weight: 650;
}
.pp-shead-done {
  height: 44px;
  padding: 0 12px;
  border: 0;
  background: transparent;
  color: var(--t-primary);
  font: inherit;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
}
.kit-layer:not(.al) .pp-hdr,
.kit-layer:not(.al) .pp-foot,
.kit-layer:not(.al) .pp-fkey {
  display: none;
}
.kit-layer:not(.al) .pp-field {
  height: 44px;
}
.kit-layer:not(.al) .pp-input {
  font-size: 16px;
}
.kit-layer:not(.al) .pp-list {
  max-height: none;
}
.kit-layer:not(.al) .pp-row {
  height: 48px;
  font-size: 15px;
}
.kit-layer:not(.al) .pp-row.two {
  height: 56px;
}
</style>
