<script setup>
import { vLayer } from '@/utils/layerZ'
// The due picker (v2, #3118 — mockups DuePicker, PackageJ «Срок»): one 560px
// popover for the start, the due, the repeat and the due notification, opened
// from the task modal's «Срок» row (D) and from a card's due pill. Replaces the
// old DueEditor.
//
//   ┌ [📅 когда? «завтра 18:00», «пт», «через 3 дня»   D] ┐  ← words, quickParse
//   │ → пт, 9 окт., 18:00 · через 6 дней         ↵ применить │
//   │ [ Начало | Срок ]                                Tab   │
//   │ presets 1–5, 0    │ Октябрь 2026   ‹ ›  Сегодня T       │
//   │                   │ the grid · Время [18:00] 09 12 18   │
//   │ ⟳ Повтор        Каждую неделю по пн и чт · следующая › │  ← doors
//   │ 🔔 Напоминание о сроке   По умолчанию — за час       › │
//   └ ⌫ очистить                               ↵ готово  Esc ┘
//
// Every change is saved at once (`apply` carries start + due + recurrence in one
// patch, `notify` goes to its own endpoint), so ↵ «готово» only closes.
//
// Keys: in the field — typing, ↵ applies what was typed, ↓ to the presets, Tab
// Начало ↔ Срок, ⌫ on an empty field clears. In the presets — ↑↓, 1–5 and 0, ↵,
// → into the grid. In the grid — arrows, PgUp/PgDn, T, ↵. Esc closes; inside a
// door's panel Esc goes back, ⇧⌫ drops the repeat.
//
// Where it opens: in the task modal left of the properties panel (`side`), on a
// card under the pill; a phone gets a full-height sheet whose doors (and the
// calendar) are its next pages.
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  ArrowForwardOutline,
  CalendarClearOutline,
  CalendarNumberOutline,
  CalendarOutline,
  ChevronBackOutline,
  ChevronForwardOutline,
  CloseCircleOutline,
  HelpCircleOutline,
  NotificationsOutline,
  RepeatOutline,
} from '@vicons/ionicons5'
import BottomSheet from '../kit/BottomSheet.vue'
import Kbd from '../Kbd.vue'
import ShortcutKeys from '../ShortcutKeys.vue'
import DueCalendar from './DueCalendar.vue'
import DueRepeatPane from './DueRepeatPane.vue'
import DueNotifyPane from './DueNotifyPane.vue'
import { useAnchoredPanel } from '@/composables/useAnchoredPanel'
import { useResponsive } from '@/composables/useResponsive'
import { useFormat } from '@/composables/useFormat'
import { useDueModel } from '@/composables/useDueModel'
import { useNotifyDefaults } from '@/composables/useNotifyDefaults'
import { parseClock, parseDue } from '@/utils/quickParse'
import {
  coordsOf,
  gridStep,
  notifySummary,
  PRESET_KEYS,
  presetDay,
  relDays,
  sameDay,
  wallLabel,
  wallNow,
} from '@/utils/dueModel'
import { CAPS } from '@/utils/keyCaps'
import { capsOfDisplay } from '@/shortcuts/keymap'

const SHIFT_BACKSPACE = capsOfDisplay(['shift', 'backspace'])

const props = defineProps({
  show: { type: Boolean, default: false },
  // The element the picker belongs to (the row's value, the pill): focus comes
  // back to it, a click on it is not "outside".
  anchor: { type: Function, default: () => null },
  // A selector of the panel the anchor sits in: the picker opens to its left
  // (the task modal's properties). Empty — under the anchor.
  side: { type: String, default: '' },
  due: { type: [Number, String], default: null },
  start: { type: [Number, String], default: null },
  recurrence: { type: Object, default: null },
  notify: { type: Object, default: () => ({ enabled: 'inherit', lead: -1, repeat: -1 }) },
  columns: { type: Array, default: () => [] },
  // The catalog key shown in the field (task.due / card.due).
  keyId: { type: String, default: '' },
  testid: { type: String, default: 'due-picker' },
})
const emit = defineEmits(['update:show', 'apply', 'notify'])
const { t } = useI18n()
const { formatters, formatTime, locale } = useFormat()
const fmt = () => formatters.value

const model = useDueModel(
  () => ({
    due: props.due,
    start: props.start,
    recurrence: props.recurrence,
    columns: props.columns,
  }),
  (patch) => emit('apply', patch),
)
const notifyDefaults = useNotifyDefaults()
// The notification as the reader last set it: every change goes out whole, so
// two quick clicks never merge with a copy of the task that is not back yet.
const notifyLocal = ref({ ...props.notify })
watch(
  () => props.notify,
  (v) => (notifyLocal.value = { ...v }),
  { deep: true },
)
function onNotify(patch) {
  notifyLocal.value = { ...notifyLocal.value, ...patch }
  emit('notify', { ...notifyLocal.value })
}

// ── shell: a popover, or a sheet on a phone ──
const { isMobile } = useResponsive()
const mounted = ref(false)
const initialPhone =
  typeof window !== 'undefined' && typeof window.matchMedia === 'function'
    ? window.matchMedia('(max-width: 768px)').matches
    : false
onMounted(() => (mounted.value = true))
const phone = computed(() => (mounted.value ? isMobile.value : initialPhone))

const modalBody = inject('n-modal-body', null)
const layerTo = computed(() => modalBody?.value || 'body')
const panel = ref(null)
const desktopShow = computed(() => props.show && !phone.value)
function close() {
  emit('update:show', false)
}
// Left of the side panel when there is room, else under the anchor; the modal
// card's content top is the preferred top (mockup: the popover hangs from it).
function point() {
  const a = props.anchor()
  const el = panel.value
  if (!a?.getBoundingClientRect || !el) return null
  const r = a.getBoundingClientRect()
  const w = el.offsetWidth
  const h = el.offsetHeight
  const side = props.side ? a.closest(props.side) : null
  if (side) {
    const s = side.getBoundingClientRect()
    const x = s.left - w + 4
    if (x >= 8) {
      const card = a.closest('.tm-card')
      const top = card ? card.getBoundingClientRect().top + 78 : r.top - 40
      // Keep the row's height inside the popover's span when the card is tall.
      const y = Math.min(Math.max(top, r.bottom - h + 8), r.top - 8)
      return { x, y: Math.max(8, y) }
    }
  }
  // Under the anchor; above it when there is no room below.
  const below = r.bottom + 4
  const y = below + h > window.innerHeight - 8 && r.top - h - 4 >= 8 ? r.top - h - 4 : below
  return { x: r.left, y }
}
const { style: panelStyle, place } = useAnchoredPanel({
  show: desktopShow,
  anchor: () => point() || props.anchor(),
  panel,
  placement: 'point',
  offset: { x: 0, y: 0 },
  onOutside: (e) => {
    if (props.anchor()?.contains?.(e.target)) return
    close()
  },
})
// The panes differ in height: re-place on every change of size.
let ro = null
watch(panel, (el) => {
  ro?.disconnect()
  ro = null
  if (!el || typeof ResizeObserver === 'undefined') return
  ro = new ResizeObserver(() => place())
  ro.observe(el)
})
onBeforeUnmount(() => ro?.disconnect())

const sheetShow = computed({
  get: () => props.show && phone.value,
  set: (v) => !v && close(),
})

// ── state of one opening ──
const pane = ref('main') // main | rep | not | cal (cal — the phone's page)
const text = ref('')
const zone = ref('input') // input | list | grid
const listIdx = ref(0)
const kf = ref(null) // the grid's keyboard day
const inputEl = ref(null)
const subEl = ref(null)

const today = computed(() => coordsOf(wallNow(fmt())))

// ── what was typed ──
const parsed = computed(() => parseDue(text.value, { now: wallNow(fmt()) }))
const parsedEp = computed(() => {
  const p = parsed.value
  if (!p.ok || p.clear) return null
  return {
    day: p.day ? coordsOf(p.day) : model.activeEp?.day || today.value,
    time: p.time || model.activeEp?.time || null,
  }
})
const preview = computed(() => {
  const p = parsed.value
  if (!p.empty) {
    if (!p.ok) return { kind: 'bad', text: t('task.due.notUnderstood', { text: p.bad }) }
    if (p.clear) return { kind: 'ok', label: t('task.due.clearPreview'), sub: '' }
    const ep = parsedEp.value
    return {
      kind: 'ok',
      label: wallLabel(ep, fmt()),
      sub: `· ${relDays(ep.day, today.value, locale.value)}`,
    }
  }
  if (zone.value === 'grid' && kf.value) {
    return {
      kind: 'ok',
      label: wallLabel({ day: kf.value, time: model.activeEp?.time || null }, fmt()),
      sub: `· ${t('task.due.gridKeys')}`,
    }
  }
  if (model.startEp && model.dueEp && model.span) {
    return {
      kind: 'hint',
      text: t('task.due.rangeHint', {
        start: wallLabel({ day: model.startEp.day, time: null }, fmt()),
        due: wallLabel({ day: model.dueEp.day, time: null }, fmt()),
        days: t('task.due.daysCount', { n: model.span.days + 1 }, model.span.days + 1),
      }),
    }
  }
  return { kind: 'hint', text: t('task.due.wordsHint') }
})
function applyText() {
  const p = parsed.value
  if (p.empty) return close()
  if (!p.ok) return
  model.applyParsed(p)
  text.value = ''
  close()
}

// ── segments ──
const segs = computed(() => [
  {
    key: 'start',
    label: t('task.due.start'),
    value: model.startEp ? wallLabel(model.startEp, fmt()) : t('task.due.startUnset'),
    set: !!model.startEp,
  },
  {
    key: 'due',
    label: t('task.due.due'),
    value: model.dueEp ? wallLabel(model.dueEp, fmt()) : t('task.due.dueUnset'),
    set: !!model.dueEp,
  },
])
function toggleTarget() {
  model.setTarget(model.target === 'due' ? 'start' : 'due')
  if (zone.value === 'grid') kf.value = model.activeEp?.day || today.value
}

// ── presets + doors (the list zone) ──
const presets = computed(() => [
  ...PRESET_KEYS.map((k, i) => {
    const d = presetDay(k, today.value)
    return {
      key: k,
      label: t(`task.due.preset.${k}`),
      long: t(`task.due.presetLong.${k}`),
      date: wallLabel({ day: d, time: null }, fmt()),
      cap: String(i + 1),
    }
  }),
  {
    key: 'none',
    label: t('task.due.preset.none'),
    long: t('task.due.preset.none'),
    date: '',
    cap: '0',
  },
])
const LIST_LEN = 8 // six presets + two doors
function activateList(i, viaKey) {
  if (i < presets.value.length) {
    model.applyPreset(presets.value[i].key)
    if (viaKey) close()
  } else openPane(i === presets.value.length ? 'rep' : 'not')
}

// ── grid ──
const gridSel = computed(() => model.activeEp?.day || null)
const gridEnd = computed(() => {
  const other = model.target === 'start' ? model.dueEp : model.startEp
  return other && !sameDay(other.day, gridSel.value) ? other.day : null
})
const gridView = computed({
  get: () => model.view || { year: today.value.year, month: today.value.month },
  set: (v) => (model.view = v),
})
function onGridPick(day) {
  model.pickDay(day)
  if (zone.value === 'grid') kf.value = day
}

// ── time ──
const TIMES = ['09:00', '12:00', '18:00']
const clockLabel = (hm) => {
  const [h, m] = hm.split(':').map(Number)
  return formatTime(new Date(Date.UTC(2026, 0, 1, h, m)), { timeZone: 'UTC' })
}
const timeText = computed(() => (model.activeEp?.time ? clockLabel(model.activeEp.time) : ''))
const timeDraft = ref('')
watch(timeText, (v) => (timeDraft.value = v))
function commitTime() {
  const v = timeDraft.value.trim()
  if (!v || v === timeText.value) {
    timeDraft.value = timeText.value
    return
  }
  const hm = parseClock(v)
  if (hm) model.setTime(hm)
  else timeDraft.value = timeText.value
}

// ── doors ──
const notifyText = computed(() => notifySummary(notifyLocal.value, notifyDefaults.value))
const repText = computed(() =>
  model.freq && model.nextLabel
    ? `${model.summary} · ${t('task.due.rep.next', { date: model.nextLabel })}`
    : model.summary,
)
async function openPane(p) {
  pane.value = p
  await nextTick()
  if (!phone.value) subEl.value?.focus({ preventScroll: true })
}
async function backToMain() {
  pane.value = 'main'
  zone.value = 'input'
  await nextTick()
  if (!phone.value) inputEl.value?.focus({ preventScroll: true })
}

// ── keys ──
const isTyping = (e) => e.key.length === 1 && !e.metaKey && !e.ctrlKey && !e.altKey
async function toZone(z) {
  zone.value = z
  await nextTick()
  if (z === 'input') inputEl.value?.focus({ preventScroll: true })
  else panel.value?.querySelector('.dp')?.focus({ preventScroll: true })
}
function onKeydown(e) {
  if (e.isComposing) return
  // Keys typed here never reach the global dispatcher (the modal's own E, S, J…).
  e.stopPropagation()
  if (pane.value !== 'main') return onSubKey(e)
  if (e.key === 'Escape') {
    e.preventDefault()
    close()
    return
  }
  if (e.key === 'Tab') {
    e.preventDefault()
    toggleTarget()
    return
  }
  const inField = e.target === inputEl.value
  const inTime = e.target?.classList?.contains('dp-time-in')
  if (inTime) {
    if (e.key === 'Enter') {
      e.preventDefault()
      commitTime()
    }
    return
  }
  if (zone.value === 'input' || inField) {
    if (e.key === 'Enter') {
      e.preventDefault()
      applyText()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      listIdx.value = 0
      toZone('list')
    } else if (e.key === 'Backspace' && !text.value) {
      e.preventDefault()
      model.clearActive()
    }
    return
  }
  if (e.key === 'Backspace') {
    e.preventDefault()
    model.clearActive()
    return
  }
  if (zone.value === 'list') {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const n = listIdx.value + (e.key === 'ArrowDown' ? 1 : -1)
      if (n < 0) return toZone('input')
      listIdx.value = Math.min(LIST_LEN - 1, n)
    } else if (e.key === 'ArrowRight') {
      e.preventDefault()
      kf.value = model.activeEp?.day || today.value
      model.navTo(kf.value)
      toZone('grid')
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      activateList(listIdx.value, true)
    } else if (/^[0-5]$/.test(e.key)) {
      e.preventDefault()
      activateList(e.key === '0' ? 5 : Number(e.key) - 1, true)
    } else if (isTyping(e)) toZone('input')
    return
  }
  // grid
  if (e.key === 'Enter') {
    e.preventDefault()
    if (kf.value) model.pickDay(kf.value)
    close()
    return
  }
  if (e.key === ' ') {
    e.preventDefault()
    if (kf.value) model.pickDay(kf.value)
    return
  }
  const next = gridStep(e, kf.value, today.value)
  if (next) {
    e.preventDefault()
    // ← off the grid's first column goes back to the presets.
    kf.value = next
    if (next.year !== model.view?.year || next.month !== model.view?.month) model.navTo(next)
    return
  }
  if (isTyping(e)) toZone('input')
}
function onSubKey(e) {
  if (e.key === 'Escape') {
    e.preventDefault()
    backToMain()
    return
  }
  if (pane.value === 'rep' && e.key === 'Backspace' && e.shiftKey) {
    e.preventDefault()
    model.removeRule()
    return
  }
  if (e.key === 'Enter' && !e.target?.closest?.('button, input, [role="switch"]')) {
    e.preventDefault()
    close()
  }
}

// Clicks that close: a preset on the phone (a sheet has no keyboard to keep).
function tapPreset(i) {
  activateList(i, phone.value)
}
// Every opening starts on the main panel, in the field, on «Срок».
watch(
  () => props.show,
  async (on) => {
    if (!on) return
    pane.value = 'main'
    text.value = ''
    zone.value = 'input'
    listIdx.value = 0
    kf.value = null
    model.setTarget('due')
    model.navTo(model.dueEp?.day)
    timeDraft.value = timeText.value
    if (phone.value) return
    await nextTick()
    await nextTick()
    inputEl.value?.focus({ preventScroll: true })
  },
  { immediate: true },
)
defineExpose({ model, pane, openPane })
</script>

<template>
  <!-- ── desktop: a popover ── -->
  <Teleport :to="layerTo">
    <div
      v-if="desktopShow"
      ref="panel"
      v-layer
      class="dp-layer kit-layer"
      :style="panelStyle"
      :data-testid="testid"
      @keydown="onKeydown"
    >
      <div class="dp" tabindex="-1" role="dialog" :aria-label="t('task.due.title')">
        <template v-if="pane === 'main'">
          <label class="dp-field" :class="{ bad: preview.kind === 'bad' }">
            <n-icon :component="CalendarOutline" :size="15" class="dp-ficon" />
            <input
              ref="inputEl"
              v-model="text"
              class="dp-input"
              :placeholder="t('task.due.placeholder')"
              :aria-label="t('task.due.placeholder')"
              data-testid="due-input"
              spellcheck="false"
              autocomplete="off"
              @focus="zone = 'input'"
            />
            <ShortcutKeys v-if="keyId" :id="keyId" class="dp-fkey" />
          </label>

          <div v-if="preview.kind === 'ok'" class="dp-prev" data-testid="due-preview">
            <n-icon :component="ArrowForwardOutline" :size="13" class="dp-prev-i" />
            <span class="dp-prev-l">{{ preview.label }}</span>
            <span class="dp-prev-s">{{ preview.sub }}</span>
            <span class="dp-grow" />
            <span class="dp-fk"
              ><Kbd>{{ CAPS.enter }}</Kbd
              >{{ t('task.due.apply') }}</span
            >
          </div>
          <div v-else-if="preview.kind === 'bad'" class="dp-bad" data-testid="due-bad">
            <n-icon :component="HelpCircleOutline" :size="15" class="dp-bad-i" />
            <span>{{ preview.text }}</span>
          </div>
          <div v-else class="dp-hint" data-testid="due-hint">{{ preview.text }}</div>

          <div class="dp-segs" role="tablist">
            <button
              v-for="s in segs"
              :key="s.key"
              type="button"
              tabindex="-1"
              role="tab"
              class="dp-seg"
              :class="{ on: model.target === s.key }"
              :aria-selected="model.target === s.key"
              :data-seg="s.key"
              @mousedown.prevent
              @click="model.setTarget(s.key)"
            >
              <span class="dp-seg-l">{{ s.label }}</span>
              <span class="dp-seg-v" :class="{ unset: !s.set }">{{ s.value }}</span>
            </button>
            <span class="dp-seg-k"
              ><Kbd>{{ CAPS.tab }}</Kbd></span
            >
          </div>

          <div class="dp-body">
            <div class="dp-presets" role="listbox">
              <div
                v-for="(p, i) in presets"
                :key="p.key"
                class="dp-pre"
                :class="{ on: zone === 'list' && listIdx === i }"
                role="option"
                :data-preset="p.key"
                @mousedown.prevent
                @mouseenter="zone === 'list' && (listIdx = i)"
                @click="tapPreset(i)"
              >
                <span class="dp-pre-t">{{ p.label }}</span>
                <span class="dp-pre-d">{{ p.date }}</span>
                <Kbd>{{ p.cap }}</Kbd>
              </div>
            </div>
            <div class="dp-calcol">
              <DueCalendar
                v-model:view="gridView"
                :week-start="model.weekStart"
                :sel="gridSel"
                :end="gridEnd"
                :span="model.span"
                :rep="model.target === 'due' ? model.occKeys : new Set()"
                :today="today"
                :focus="zone === 'grid' ? kf : null"
                @pick="onGridPick"
              />
              <div class="dp-times">
                <span class="dp-times-l">{{ t('task.due.time') }}</span>
                <input
                  v-model="timeDraft"
                  class="dp-time-in"
                  :placeholder="t('task.due.noTimeDash')"
                  :aria-label="t('task.due.time')"
                  data-testid="due-time"
                  @blur="commitTime"
                />
                <button
                  v-for="tm in TIMES"
                  :key="tm"
                  type="button"
                  tabindex="-1"
                  class="dp-chip"
                  :class="{ on: model.activeEp?.time === tm }"
                  :data-time="tm"
                  @mousedown.prevent
                  @click="model.setTime(tm)"
                >
                  {{ clockLabel(tm) }}
                </button>
                <button
                  type="button"
                  tabindex="-1"
                  class="dp-chip"
                  data-time="none"
                  @mousedown.prevent
                  @click="model.setTime(null)"
                >
                  {{ t('task.due.noTime') }}
                </button>
              </div>
            </div>
          </div>

          <div class="dp-doors">
            <button
              type="button"
              tabindex="-1"
              class="dp-door"
              :class="{ on: zone === 'list' && listIdx === 6 }"
              data-testid="due-door-rep"
              @mousedown.prevent
              @click="openPane('rep')"
            >
              <span class="dp-door-i"><n-icon :component="RepeatOutline" :size="14" /></span>
              <span class="dp-door-l">{{ t('task.due.rep.title') }}</span>
              <span class="dp-door-v">{{ repText }}</span>
              <n-icon :component="ChevronForwardOutline" :size="14" class="dp-door-c" />
            </button>
            <button
              type="button"
              tabindex="-1"
              class="dp-door"
              :class="{ on: zone === 'list' && listIdx === 7 }"
              data-testid="due-door-notify"
              @mousedown.prevent
              @click="openPane('not')"
            >
              <span class="dp-door-i"><n-icon :component="NotificationsOutline" :size="14" /></span>
              <span class="dp-door-l">{{ t('task.due.notify.door') }}</span>
              <span class="dp-door-v">{{ notifyText }}</span>
              <n-icon :component="ChevronForwardOutline" :size="14" class="dp-door-c" />
            </button>
          </div>

          <div class="dp-foot">
            <button
              type="button"
              tabindex="-1"
              class="dp-fk"
              data-testid="due-clear"
              @click="model.clearActive()"
            >
              <Kbd>{{ CAPS.backspace }}</Kbd
              >{{ t('task.due.clear') }}
            </button>
            <span class="dp-grow" />
            <button type="button" tabindex="-1" class="dp-fk" @click="close">
              <Kbd>{{ CAPS.enter }}</Kbd
              >{{ t('task.due.done') }}
            </button>
            <button type="button" tabindex="-1" class="dp-fk" @click="close">
              <Kbd>{{ CAPS.escape }}</Kbd>
            </button>
          </div>
        </template>

        <template v-else>
          <div ref="subEl" class="dp-sub" tabindex="-1">
            <div class="dp-subhead">
              <button type="button" class="dp-back" data-testid="due-back" @click="backToMain">
                <n-icon :component="ChevronBackOutline" :size="14" />{{ t('task.due.due') }}
              </button>
              <span class="dp-subt">{{
                pane === 'rep' ? t('task.due.rep.title') : t('task.due.notify.door')
              }}</span>
            </div>
            <DueRepeatPane v-if="pane === 'rep'" :model="model" :columns="columns" />
            <DueNotifyPane
              v-else
              :notify="notifyLocal"
              :defaults="notifyDefaults"
              @notify="onNotify"
            />
            <div class="dp-foot">
              <button type="button" tabindex="-1" class="dp-fk" @click="backToMain">
                <Kbd>{{ CAPS.escape }}</Kbd
                >{{ t('task.due.back') }}
              </button>
              <button
                v-if="pane === 'rep' && model.freq"
                type="button"
                tabindex="-1"
                class="dp-fk"
                data-testid="due-rep-remove"
                @click="model.removeRule()"
              >
                <Kbd v-for="c in SHIFT_BACKSPACE" :key="c">{{ c }}</Kbd
                >{{ t('task.due.rep.remove') }}
              </button>
              <span class="dp-grow" />
              <button type="button" tabindex="-1" class="dp-fk" @click="close">
                <Kbd>{{ CAPS.enter }}</Kbd
                >{{ t('task.due.done') }}
              </button>
            </div>
          </div>
        </template>
      </div>
    </div>
  </Teleport>

  <!-- ── phone: a full-height sheet, the doors are its pages ── -->
  <BottomSheet v-if="phone" v-model:show="sheetShow" full :to="layerTo" :testid="testid">
    <div class="dps kit-layer" @keydown.stop>
      <div class="dps-head" data-sheet-grab>
        <button v-if="pane !== 'main'" type="button" class="dps-back" @click="pane = 'main'">
          <n-icon :component="ChevronBackOutline" :size="18" />{{ t('task.due.due') }}
        </button>
        <span class="dps-title">{{
          pane === 'main'
            ? t('task.due.due')
            : pane === 'cal'
              ? t('task.due.calendar')
              : pane === 'rep'
                ? t('task.due.rep.title')
                : t('task.due.notify.door')
        }}</span>
        <button type="button" class="dps-done" data-testid="due-sheet-done" @click="close">
          {{ t('task.due.doneBtn') }}
        </button>
      </div>

      <template v-if="pane === 'main'">
        <label class="dp-field dps-field" :class="{ bad: preview.kind === 'bad' }">
          <n-icon :component="CalendarOutline" :size="15" class="dp-ficon" />
          <input
            ref="inputEl"
            v-model="text"
            class="dp-input"
            enterkeyhint="done"
            :placeholder="t('task.due.placeholderShort')"
            data-testid="due-input"
            spellcheck="false"
            autocomplete="off"
            @keydown.enter.prevent="applyText"
          />
        </label>
        <div v-if="preview.kind === 'ok'" class="dps-prev" data-testid="due-preview">
          <n-icon :component="ArrowForwardOutline" :size="13" class="dp-prev-i" />
          <span>{{ preview.label }} {{ preview.sub }}</span>
        </div>
        <div v-else-if="preview.kind === 'bad'" class="dp-bad" data-testid="due-bad">
          <n-icon :component="HelpCircleOutline" :size="15" class="dp-bad-i" />
          <span>{{ preview.text }}</span>
        </div>
        <div class="dps-sec">{{ t('task.due.quick') }}</div>
        <div
          v-for="(p, i) in presets"
          :key="p.key"
          class="dps-row"
          :data-preset="p.key"
          @click="tapPreset(i)"
        >
          <n-icon
            :component="p.key === 'none' ? CloseCircleOutline : CalendarClearOutline"
            :size="18"
            class="dps-ri"
          />
          <span class="dps-rt">{{ p.long }}</span>
          <span class="dps-rm">{{ p.date }}</span>
        </div>
        <div class="dps-gap" />
        <div class="dps-row" data-testid="due-page-cal" @click="pane = 'cal'">
          <n-icon :component="CalendarNumberOutline" :size="18" class="dps-ri" />
          <span class="dps-rt">{{ t('task.due.pickOnCalendar') }}</span>
          <span class="dps-rm">{{ model.dueEp ? wallLabel(model.dueEp, fmt()) : '' }}</span>
          <n-icon :component="ChevronForwardOutline" :size="16" class="dps-rc" />
        </div>
        <div class="dps-row" data-testid="due-door-rep" @click="pane = 'rep'">
          <n-icon :component="RepeatOutline" :size="18" class="dps-ri" />
          <span class="dps-rt">{{ t('task.due.rep.title') }}</span>
          <span class="dps-rm">{{ model.freq ? model.summary : t('task.due.rep.noneShort') }}</span>
          <n-icon :component="ChevronForwardOutline" :size="16" class="dps-rc" />
        </div>
        <div class="dps-row" data-testid="due-door-notify" @click="pane = 'not'">
          <n-icon :component="NotificationsOutline" :size="18" class="dps-ri" />
          <span class="dps-rt">{{ t('task.due.notify.door') }}</span>
          <span class="dps-rm">{{ notifyText }}</span>
          <n-icon :component="ChevronForwardOutline" :size="16" class="dps-rc" />
        </div>
      </template>

      <template v-else-if="pane === 'cal'">
        <div class="dp-segs dps-segs" role="tablist">
          <button
            v-for="s in segs"
            :key="s.key"
            type="button"
            role="tab"
            class="dp-seg"
            :class="{ on: model.target === s.key }"
            :data-seg="s.key"
            @click="model.setTarget(s.key)"
          >
            <span class="dp-seg-l">{{ s.label }}</span>
            <span class="dp-seg-v" :class="{ unset: !s.set }">{{ s.value }}</span>
          </button>
        </div>
        <DueCalendar
          v-model:view="gridView"
          :week-start="model.weekStart"
          :sel="gridSel"
          :end="gridEnd"
          :span="model.span"
          :rep="model.target === 'due' ? model.occKeys : new Set()"
          :today="today"
          :cell="44"
          @pick="onGridPick"
        />
        <div class="dp-times dps-times">
          <span class="dp-times-l">{{ t('task.due.time') }}</span>
          <input
            v-model="timeDraft"
            class="dp-time-in"
            inputmode="numeric"
            :placeholder="t('task.due.noTimeDash')"
            data-testid="due-time"
            @blur="commitTime"
            @keydown.enter.prevent="commitTime"
          />
          <button
            v-for="tm in TIMES"
            :key="tm"
            type="button"
            class="dp-chip"
            :class="{ on: model.activeEp?.time === tm }"
            :data-time="tm"
            @click="model.setTime(tm)"
          >
            {{ clockLabel(tm) }}
          </button>
          <button type="button" class="dp-chip" data-time="none" @click="model.setTime(null)">
            {{ t('task.due.noTime') }}
          </button>
        </div>
      </template>

      <DueRepeatPane v-else-if="pane === 'rep'" :model="model" :columns="columns" mobile />
      <DueNotifyPane
        v-else
        :notify="notifyLocal"
        :defaults="notifyDefaults"
        mobile
        @notify="onNotify"
      />
    </div>
  </BottomSheet>
</template>

<style scoped>
.dp-layer {
  position: fixed;
  z-index: 2050;
}
.dp {
  width: 560px;
  max-width: calc(100vw - 16px);
  max-height: calc(100vh - 16px);
  overflow-y: auto;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px;
  border-radius: var(--t-r-card, 14px);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
  outline: none;
  scrollbar-width: thin;
  --t-surface: var(--t-pop);
}
.dp-grow {
  flex: 1;
}
.dp-field {
  display: flex;
  align-items: center;
  gap: 9px;
  height: 40px;
  margin: 2px 2px 0;
  padding: 0 6px 0 11px;
  border-radius: 10px;
  background: var(--t-input-bg);
  box-shadow: 0 0 0 2px var(--t-primary);
  cursor: text;
}
.dp-field.bad {
  box-shadow: 0 0 0 2px #e0533d;
}
.dp-ficon {
  flex: none;
  color: var(--t-text3);
}
.dp-input {
  flex: 1;
  min-width: 0;
  height: 100%;
  padding: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 14px;
}
.dp-input::placeholder {
  color: var(--t-placeholder);
}
.dp-fkey {
  flex: none;
}
.dp-prev {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 26px;
  padding: 0 8px 0 12px;
  font-size: 13px;
  white-space: nowrap;
}
.dp-prev-i {
  flex: none;
  color: var(--t-primary);
}
.dp-prev-l {
  font-weight: 600;
}
.dp-prev-s {
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
}
.dp-bad {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin: 2px 2px 0;
  padding: 8px 10px;
  border-radius: 9px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 10%, var(--t-pop)),
    color-mix(in srgb, #f0a020 18%, var(--t-pop))
  );
  font-size: 12.5px;
  line-height: 1.45;
}
.dp-bad-i {
  flex: none;
  margin-top: 1px;
  color: #f0a020;
}
.dp-hint {
  display: flex;
  align-items: center;
  min-height: 26px;
  padding: 0 12px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.dp-segs {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 3px;
  border-radius: 10px;
  background: var(--t-ctl);
}
.dp-seg {
  flex: 1;
  min-width: 0;
  height: 40px;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: flex-start;
  gap: 1px;
  padding: 0 12px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.dp-seg.on {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
}
.dp-seg-l {
  font-size: 11.5px;
  color: var(--t-text3);
}
.dp-seg-v {
  max-width: 100%;
  font-size: 13px;
  color: var(--t-text2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dp-seg.on .dp-seg-v {
  color: var(--t-text1);
  font-weight: 600;
}
.dp-seg-k {
  display: flex;
  align-items: center;
  padding: 0 8px;
}
.dp-body {
  display: grid;
  grid-template-columns: minmax(206px, max-content) minmax(0, 1fr);
  gap: 12px;
  padding: 2px 2px 0;
}
.dp-presets {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding-top: 2px;
}
.dp-pre {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 6px 0 10px;
  border-radius: 7px;
  font-size: 13px;
  cursor: pointer;
  user-select: none;
}
.dp-pre:hover,
.dp-pre.on {
  background: var(--t-hover);
}
.dp-pre-t {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dp-pre-d {
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.dp-calcol {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.dp-times {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  padding: 6px 2px 2px 6px;
}
.dp-times-l {
  margin-right: 2px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.dp-time-in {
  width: 62px;
  height: 28px;
  box-sizing: border-box;
  padding: 0 9px;
  border: 0;
  outline: 0;
  border-radius: 8px;
  background: var(--t-input-bg);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}
.dp-time-in:focus {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.dp-time-in::placeholder {
  color: var(--t-text2);
}
.dp-chip {
  height: 28px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  border: 0;
  border-radius: 999px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.dp-chip:hover {
  background: var(--t-hover);
}
.dp-chip.on {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 28%, var(--t-pop))
  );
  font-weight: 600;
}
.dp-doors {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 2px;
}
.dp-door {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 42px;
  padding: 0 8px 0 10px;
  border: 0;
  border-radius: 10px;
  background: var(--t-sub);
  color: var(--t-text1);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.dp-door:hover,
.dp-door.on {
  background: var(--t-hover);
}
.dp-door-i {
  flex: 0 0 26px;
  width: 26px;
  height: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-sub)),
    color-mix(in srgb, var(--t-primary) 28%, var(--t-sub))
  );
  color: var(--t-primary);
}
.dp-door-l {
  font-size: 13px;
  color: var(--t-text3);
  white-space: nowrap;
}
.dp-door-v {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dp-door-c {
  flex: none;
  color: var(--t-text3);
}
.dp-foot {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 34px;
  margin: 4px -6px -6px;
  padding: 0 12px;
  border-radius: 0 0 var(--t-r-card, 14px) var(--t-r-card, 14px);
  background: color-mix(in srgb, var(--t-pop) 55%, var(--t-bg));
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.dp-fk {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.dp-prev .dp-fk {
  font-size: 11.5px;
  color: var(--t-text3);
  cursor: default;
}
.dp-sub {
  display: flex;
  flex-direction: column;
  gap: 6px;
  outline: none;
}
.dp-subhead {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  padding: 0 4px;
}
.dp-back {
  height: 28px;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 9px 0 5px;
  border: 0;
  border-radius: 8px;
  background: var(--t-ctl);
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.dp-back:hover {
  color: var(--t-text1);
}
.dp-subt {
  font-size: 14px;
  font-weight: 650;
}

/* ── the phone's sheet ── */
.dps {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0 -6px;
}
.dps-head {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 48px;
  padding: 0 0 0 4px;
}
.dps-back {
  height: 44px;
  display: flex;
  align-items: center;
  gap: 2px;
  margin-left: -8px;
  padding: 0 8px 0 0;
  border: 0;
  background: transparent;
  color: var(--t-primary);
  font: inherit;
  font-size: 15px;
  cursor: pointer;
}
.dps-title {
  flex: 1;
  font-size: 17px;
  font-weight: 650;
}
.dps-done {
  height: 44px;
  padding: 0 4px 0 12px;
  border: 0;
  background: transparent;
  color: var(--t-primary);
  font: inherit;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
}
.dps-field {
  height: 44px;
  margin: 2px 0 4px;
}
.dps-field .dp-input {
  font-size: 16px;
}
.dps-prev {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px 4px 4px;
  font-size: 13px;
  color: var(--t-text2);
}
.dps-sec {
  padding: 12px 4px 6px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.dps-row {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 48px;
  padding: 0 4px;
  border-radius: 10px;
  font-size: 15px;
  cursor: pointer;
}
.dps-row:active {
  background: var(--t-hover);
}
.dps-ri {
  flex: none;
  color: var(--t-text2);
}
.dps-rt {
  flex: 0 1 auto;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dps-rm {
  flex: 1;
  min-width: 0;
  text-align: right;
  font-size: 13px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dps-rc {
  flex: none;
  color: var(--t-text3);
}
.dps-gap {
  height: 10px;
}
.dps-segs {
  margin-bottom: 10px;
}
.dps-segs .dp-seg {
  height: 42px;
}
.dps-segs .dp-seg-v {
  font-size: 14px;
}
.dps-times {
  padding: 14px 0 0;
  gap: 8px;
}
.dps-times .dp-time-in {
  width: 72px;
  height: 36px;
  font-size: 15px;
}
.dps-times .dp-chip {
  height: 36px;
  padding: 0 14px;
  font-size: 14px;
}
</style>
