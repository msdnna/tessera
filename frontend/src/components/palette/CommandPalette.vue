<script setup>
// The ⌘K palette (v2, #2957): search across the workspace, commands, and —
// after Tab on a task — that task's actions. One instance in AppLayout, driven
// by the palette store. A hand-rolled overlay rather than NModal: the spec
// wants it pinned 72px from the top over the v2 scrim, which NModal's own
// centring and mask fight.
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NSpin } from 'naive-ui'
import { SearchOutline, CloseOutline, Checkmark, ChevronForwardOutline } from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import TesseraIcon from '@/components/TesseraIcon.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { usePaletteStore } from '@/stores/palette'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { useResponsive } from '@/composables/useResponsive'
import { pushBlocker } from '@/shortcuts/dispatcher'
import { hasModKey } from '@/utils/keys'
import { highlight } from '@/utils/fuzzy'
import { usePaletteResults } from './usePaletteResults'
import { usePaletteTask } from './usePaletteTask'
import { usePaletteForm } from './usePaletteForm'

const { t } = useI18n()
const palette = usePaletteStore()
// A phone (#2976 — package G «Поиск»): the palette is the whole screen, with
// «Отмена» next to the field instead of Esc.
const { isMobile } = useResponsive()
const results = usePaletteResults()
const taskScope = usePaletteTask()
const form = usePaletteForm()

const inputEl = ref(null)
const listEl = ref(null)
const active = ref(0)

// Three modes: results, a task's actions (scope.taskId) and a palette form
// (scope.form, #2982).
const scoped = computed(() => !!palette.scope?.taskId)
const formMode = computed(() => !!palette.scope?.form)
const field = computed(() => (scoped.value ? palette.scope?.field || null : null))

const groups = computed(() => {
  if (formMode.value) return form.groupsFor(palette.query)
  return scoped.value ? taskScope.groupsFor(palette.query) : results.groups.value
})
const flat = computed(() => groups.value.flatMap((g) => g.rows))
const loading = computed(() => {
  if (formMode.value) return false
  return scoped.value ? taskScope.loading.value : results.loading.value && !flat.value.length
})

// Section pills (results mode). A pill with a prefix writes it into the query
// so what the list shows is always spelled out in the field.
const SECTIONS = [
  { id: 'all' },
  { id: 'tasks', prefix: '#' },
  { id: 'boards' },
  { id: 'people', prefix: '@' },
  { id: 'commands', prefix: '>' },
]
const currentSection = computed(() => results.parsed.value.section)
function pickSection(s) {
  const bare = palette.query.replace(/^[#@>]\s*/, '')
  palette.section = s.prefix || s.id === 'all' ? 'all' : s.id
  palette.query = s.prefix ? `${s.prefix}${bare}` : bare
  inputEl.value?.focus()
}

const placeholder = computed(() => {
  if (formMode.value) return t(`shell.paletteForm.placeholder.${form.field.value?.key || 'name'}`)
  if (!scoped.value) return t('shell.palette.placeholder')
  if (!field.value) return t('shell.palette.placeholderTask')
  return t(`shell.palette.placeholderField.${field.value}`)
})

// Keep the highlighted row where the user put it while a toggle re-renders the
// list (assignees, tags); start over when the query or the scope changes.
watch(
  () => [palette.query, palette.scope?.taskId, field.value, palette.section],
  () => (active.value = 0),
)
// Opening a single-value field (status, priority) puts the cursor on the value
// the task has now, so ↑/↓ move relative to it.
watch(
  () => [field.value, taskScope.task.value?.id],
  async () => {
    if (!['status', 'priority'].includes(field.value)) return
    await nextTick()
    const i = flat.value.findIndex((r) => r.checked)
    if (i >= 0) active.value = i
  },
)
watch(flat, (rows) => {
  if (active.value >= rows.length) active.value = Math.max(0, rows.length - 1)
})
// In a form the highlighted row is the current field (option lists start at
// the top, like any list).
watch(
  () => [formMode.value, form.step.value, form.field.value?.type],
  async () => {
    if (!formMode.value) return
    await nextTick()
    active.value =
      form.field.value?.type === 'pick'
        ? Math.max(
            0,
            flat.value.findIndex((r) => r.checked),
          )
        : form.step.value
  },
)

// ── open / close ──
let releaseBlocker = null
let returnFocus = null
watch(
  () => palette.open,
  async (open) => {
    if (open) {
      returnFocus = document.activeElement
      releaseBlocker = pushBlocker()
      await nextTick()
      inputEl.value?.focus()
      inputEl.value?.select()
    } else {
      releaseBlocker?.()
      releaseBlocker = null
      if (returnFocus?.isConnected) returnFocus.focus?.()
      returnFocus = null
    }
  },
)
onBeforeUnmount(() => releaseBlocker?.())
const openRef = computed(() => palette.open)
useOverlayBack(openRef, () => palette.hide())

// ── navigation ──
function move(delta) {
  const n = flat.value.length
  if (!n) return
  active.value = (active.value + delta + n) % n
  nextTick(() => {
    listEl.value?.querySelector('.cp-row.active')?.scrollIntoView({ block: 'nearest' })
  })
}

async function runRow(row, e) {
  if (!row || row.disabled) return
  const next = (await row.run({ newTab: !!e && hasModKey(e) })) || 'close'
  if (next === 'close') palette.hide()
  else if (next === 'back') back()
  inputEl.value?.focus()
}

// Tab: narrow to the highlighted task (results), open the highlighted field
// (task actions). The query typed before narrowing comes back on the way out.
let savedQuery = ''
function forward() {
  if (formMode.value) return form.next()
  const row = flat.value[active.value]
  if (!row) return
  if (!scoped.value && row.task) {
    savedQuery = palette.query
    palette.scope = { taskId: row.task.id, field: null, direct: false }
    palette.query = ''
  } else if (scoped.value && !field.value && row.field) {
    palette.scope = { ...palette.scope, field: row.field }
    palette.query = ''
  }
}

function back() {
  if (formMode.value) return form.prev()
  if (!scoped.value) return
  if (field.value) {
    palette.scope = { ...palette.scope, field: null, direct: false }
    palette.query = ''
  } else {
    palette.scope = null
    palette.query = savedQuery
    savedQuery = ''
  }
}

async function submitForm(open) {
  const next = await form.submit(open)
  if (next === 'close') palette.hide()
  inputEl.value?.focus()
}

// Keys of the form mode: Tab / ⇧Tab and ↑/↓ walk the fields (↑/↓ walk the
// options on a pick field), ↵ creates from any field, ⌘↵ creates and opens;
// on «Иконка и цвет» ←/→ is the colour and ↑/↓ the icon.
function onFormKey(e) {
  const mod = hasModKey(e)
  const f = form.field.value
  if (f?.type === 'look' && ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) {
    e.preventDefault()
    if (e.key === 'ArrowLeft' || e.key === 'ArrowRight')
      form.shiftColor(e.key === 'ArrowRight' ? 1 : -1)
    else form.shiftIcon(e.key === 'ArrowDown' ? 1 : -1)
    return true
  }
  switch (e.key) {
    case 'Tab':
      e.preventDefault()
      if (e.shiftKey) form.prev()
      else form.next()
      return true
    case 'ArrowDown':
    case 'ArrowUp':
      if (f?.type === 'pick') return false
      e.preventDefault()
      if (e.key === 'ArrowDown') form.next()
      else form.prev()
      return true
    case 'Enter':
      e.preventDefault()
      if (f?.type === 'pick' && !mod) runRow(flat.value[active.value], e)
      else submitForm(mod)
      return true
    case 'Backspace':
      if (!palette.query && form.step.value === 0) {
        e.preventDefault()
        form.exit()
        return true
      }
      return false
  }
  return false
}

function onKeydown(e) {
  if (e.isComposing) return
  if (formMode.value && onFormKey(e)) return
  const mod = hasModKey(e)
  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault()
      move(1)
      return
    case 'ArrowUp':
      e.preventDefault()
      move(-1)
      return
    case 'Enter':
      e.preventDefault()
      runRow(flat.value[active.value], e)
      return
    case 'Tab':
      e.preventDefault()
      if (e.shiftKey) back()
      else forward()
      return
    case 'Escape':
      e.preventDefault()
      e.stopPropagation()
      palette.hide()
      return
    case 'Backspace':
      if (scoped.value && !palette.query) {
        e.preventDefault()
        back()
      }
      return
  }
  if (scoped.value && taskScope.task.value && mod && e.shiftKey && e.code === 'KeyC') {
    e.preventDefault()
    taskScope.copyLink().then((next) => next === 'close' && palette.hide())
  }
}

// ── presentation ──
const chipStatus = computed(() => taskScope.status.value)

function rowIndex(row) {
  return flat.value.indexOf(row)
}

const foot = computed(() => {
  if (formMode.value) {
    return [
      { caps: ['↵'], label: t('shell.paletteForm.foot.create') },
      { caps: ['Tab'], label: t('shell.paletteForm.foot.next') },
      { id: 'palette.back', label: t('shell.paletteForm.foot.back') },
      { id: 'palette.newTab', label: t('shell.paletteForm.foot.createOpen') },
    ]
  }
  if (!scoped.value) {
    return [
      { caps: ['↑', '↓'], label: t('shell.palette.foot.select') },
      { caps: ['↵'], label: t('shell.palette.foot.open') },
      { caps: ['Tab'], label: t('shell.palette.foot.actions') },
      { id: 'palette.newTab', label: t('shell.palette.foot.newTab') },
    ]
  }
  return [
    { caps: ['↑', '↓'], label: t('shell.palette.foot.select') },
    { caps: ['↵'], label: t('shell.palette.foot.apply') },
    ...(field.value ? [] : [{ caps: ['Tab'], label: t('shell.palette.foot.options') }]),
    {
      id: 'palette.back',
      label: t(field.value ? 'shell.palette.foot.toActions' : 'shell.palette.foot.toResults'),
    },
  ]
})
const formTitle = computed(() => (form.kind.value ? t(`shell.keys.form.${form.kind.value}`) : ''))
const footRight = computed(() => {
  if (formMode.value) return `> ${formTitle.value}`
  if (!scoped.value) return t('shell.palette.foot.prefixes')
  if (field.value) return t(`shell.palette.task.${field.value}`)
  return t('shell.palette.foot.taskActions', { n: flat.value.length })
})

// A row the keyboard can do more with: Tab narrows to a task (results) or
// opens a field (task actions).
const tabLabel = (row) => {
  if (formMode.value)
    return row.fieldHint && form.field.value?.type !== 'pick'
      ? t('shell.paletteForm.nextField')
      : null
  if (!scoped.value && row.task) return t('shell.palette.hint.actions')
  if (scoped.value && !field.value && row.field) return t('shell.palette.hint.choose')
  return null
}
const enterLabel = (row) => {
  if (formMode.value) return null
  if (!scoped.value)
    return row.task ? t('shell.palette.hint.open') : row.form ? t('shell.paletteForm.fill') : null
  return field.value ? t('shell.palette.hint.apply') : null
}
</script>

<template>
  <Teleport to="body">
    <transition name="cp">
      <div v-if="palette.open" class="cp-scrim" @mousedown.self="palette.hide()">
        <div class="cp" role="dialog" aria-modal="true" data-testid="command-palette">
          <div class="cp-input">
            <n-icon
              :component="formMode ? ChevronForwardOutline : SearchOutline"
              :size="19"
              class="cp-search-ico"
            />
            <template v-if="formMode">
              <span class="cp-chip cp-chip-cmd" data-testid="palette-form-chip">
                <span class="cp-chip-gt">&gt;</span>
                <span class="cp-chip-title">{{ formTitle }}</span>
                <button
                  type="button"
                  class="cp-chip-x"
                  :aria-label="t('shell.paletteForm.foot.back')"
                  @mousedown.prevent
                  @click="form.exit()"
                >
                  <n-icon :component="CloseOutline" :size="13" />
                </button>
              </span>
              <span v-if="form.field.value" class="cp-chip cp-chip-field">{{
                t(`shell.paletteForm.fields.${form.field.value.key}`)
              }}</span>
            </template>
            <span v-if="scoped && taskScope.task.value" class="cp-chip">
              <TesseraIcon
                v-if="chipStatus"
                :name="chipStatus.name"
                :size="13"
                :style="{ color: chipStatus.color }"
              />
              <span class="cp-chip-num">#{{ taskScope.task.value.number }}</span>
              <span class="cp-chip-title">{{ taskScope.task.value.title }}</span>
              <button
                type="button"
                class="cp-chip-x"
                :aria-label="t('shell.palette.foot.toResults')"
                @mousedown.prevent
                @click="palette.scope = null"
              >
                <n-icon :component="CloseOutline" :size="13" />
              </button>
            </span>
            <input
              ref="inputEl"
              v-model="palette.query"
              class="cp-field"
              data-testid="palette-input"
              :placeholder="placeholder"
              spellcheck="false"
              autocomplete="off"
              @keydown="onKeydown"
            />
            <span v-if="scoped || formMode" class="cp-back">
              <ShortcutKeys id="palette.back" joined />
              {{ t('shell.palette.back') }}
            </span>
            <ShortcutKeys id="palette.close" />
            <button
              v-if="isMobile"
              type="button"
              class="cp-cancel"
              data-testid="palette-cancel"
              @mousedown.prevent
              @click="palette.hide()"
            >
              {{ t('common.action.cancel') }}
            </button>
          </div>

          <div v-if="formMode && form.error.value" class="cp-error" role="alert">
            {{ form.error.value }}
          </div>
          <div v-if="!scoped && !formMode" class="cp-tabs">
            <button
              v-for="s in SECTIONS"
              :key="s.id"
              type="button"
              class="cp-tab"
              :class="{ on: currentSection === s.id }"
              :data-section="s.id"
              @mousedown.prevent
              @click="pickSection(s)"
            >
              {{ t(`shell.palette.sections.${s.id}`) }}
              <span v-if="s.prefix" class="cp-tab-prefix">{{ s.prefix }}</span>
            </button>
          </div>

          <div ref="listEl" class="cp-list">
            <div v-if="loading" class="cp-empty"><n-spin size="small" /></div>
            <div v-else-if="!flat.length" class="cp-empty">
              {{
                palette.query.trim() ? t('shell.palette.nothing') : t('shell.palette.typeToSearch')
              }}
            </div>
            <template v-for="g in groups" v-else :key="g.id">
              <div class="cp-group">
                <span>{{ g.title }}</span>
                <span v-if="g.count" class="cp-group-n">{{ g.count }}</span>
              </div>
              <div
                v-for="row in g.rows"
                :key="row.key"
                class="cp-row"
                :class="{
                  active: rowIndex(row) === active,
                  disabled: row.disabled,
                  muted: row.muted,
                }"
                :data-row="row.key"
                @mousemove="active = rowIndex(row)"
                @mousedown.prevent
                @click="runRow(row, $event)"
              >
                <span class="cp-lead">
                  <TesseraIcon
                    v-if="row.status"
                    :name="row.status.name"
                    :variant="row.status.name === 'status-done' ? 'filled' : 'outline'"
                    :size="16"
                    :style="{ color: row.status.color }"
                  />
                  <UserAvatar
                    v-else-if="row.avatar"
                    :user-id="row.avatar"
                    :name="row.title"
                    class="cp-avatar"
                  />
                  <span v-else-if="row.dot" class="cp-dot" :style="{ background: row.dot }" />
                  <n-icon v-else-if="row.icon" :component="row.icon" :size="16" />
                </span>
                <span class="cp-text">
                  <span class="cp-title">
                    <template v-for="(seg, i) in highlight(row.title, row.hl)" :key="i">
                      <mark v-if="seg.hit" class="cp-hit">{{ seg.text }}</mark>
                      <template v-else>{{ seg.text }}</template>
                    </template>
                  </span>
                  <span v-if="row.hint || row.sub" class="cp-sub">{{ row.hint || row.sub }}</span>
                </span>
                <span v-if="row.value" class="cp-value">{{ row.value }}</span>
                <n-icon v-if="row.checked" :component="Checkmark" :size="16" class="cp-check" />
                <span
                  v-if="rowIndex(row) === active && (tabLabel(row) || enterLabel(row))"
                  class="cp-hints"
                >
                  <template v-if="tabLabel(row)">
                    <ShortcutKeys id="palette.actions" /><span class="cp-hint-l">{{
                      tabLabel(row)
                    }}</span>
                  </template>
                  <template v-if="enterLabel(row)">
                    <ShortcutKeys id="palette.open" /><span>{{ enterLabel(row) }}</span>
                  </template>
                </span>
                <ShortcutKeys v-else-if="row.keys" :id="row.keys" />
                <span v-else-if="row.keysCaps" class="cp-caps">
                  <Kbd v-for="c in row.keysCaps" :key="c">{{ c }}</Kbd>
                </span>
              </div>
            </template>
          </div>

          <div class="cp-foot">
            <span v-for="(f, i) in foot" :key="i" class="cp-foot-item">
              <ShortcutKeys v-if="f.id" :id="f.id" />
              <template v-else>
                <Kbd v-for="c in f.caps" :key="c">{{ c }}</Kbd>
              </template>
              {{ f.label }}
            </span>
            <span class="cp-foot-sp" />
            <span class="cp-foot-right">{{ footRight }}</span>
          </div>
        </div>
      </div>
    </transition>
  </Teleport>
</template>

<style scoped>
.cp-scrim {
  position: fixed;
  inset: 0;
  z-index: 3000;
  display: flex;
  justify-content: center;
  align-items: flex-start;
  padding: 72px 16px 16px;
  background: var(--t-scrim);
}
.cp {
  width: 680px;
  max-width: 100%;
  max-height: calc(100vh - 104px);
  display: flex;
  flex-direction: column;
  border-radius: var(--t-r-pop);
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  overflow: hidden;
  color: var(--t-text1);
}

/* ── input row ── */
.cp-input {
  flex: none;
  display: flex;
  align-items: center;
  gap: 12px;
  height: 58px;
  padding: 0 16px 0 18px;
}
.cp-search-ico {
  color: var(--t-text3);
  flex: none;
}
.cp-field {
  flex: 1;
  min-width: 0;
  height: 100%;
  border: none;
  outline: none;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 16px;
  caret-color: var(--t-primary);
}
.cp-field::placeholder {
  color: var(--t-text3);
}
.cp-chip {
  flex: 0 1 auto;
  max-width: 40%;
  height: 28px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 4px 0 8px;
  border-radius: 8px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 26%, var(--t-pop))
  );
  font-size: 13px;
  white-space: nowrap;
}
.cp-chip-num {
  color: var(--t-text3);
}
.cp-chip-gt {
  color: var(--t-text3);
  font-weight: 700;
}
.cp-chip-cmd .cp-chip-title {
  font-weight: 600;
}
.cp-chip-field {
  padding: 0 9px;
  background: var(--t-pill);
  color: var(--t-text2);
}
.cp-error {
  flex: none;
  padding: 8px 18px;
  font-size: 12.5px;
  color: #e0533d;
}
.cp-chip-title {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cp-chip-x {
  display: flex;
  padding: 3px;
  border: none;
  border-radius: 5px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.cp-chip-x:hover {
  color: var(--t-text1);
}
.cp-back {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}

/* ── section pills ── */
.cp-tabs {
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 14px 8px;
  flex-wrap: wrap;
}
.cp-tab {
  height: 26px;
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 0 11px;
  border: none;
  border-radius: 13px;
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.cp-tab:hover {
  color: var(--t-text1);
}
.cp-tab.on {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 26%, var(--t-pop))
  );
  color: var(--t-text1);
  font-weight: 600;
}
.cp-tab-prefix {
  color: var(--t-text3);
  font-weight: 600;
}

/* ── list ── */
.cp-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 8px 8px;
  scrollbar-width: thin;
}
.cp-empty {
  display: flex;
  justify-content: center;
  padding: 28px 0 24px;
  font-size: 13px;
  color: var(--t-text3);
}
.cp-group {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 10px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.cp-group-n {
  font-weight: 500;
  letter-spacing: 0;
  text-transform: none;
}
.cp-row {
  position: relative;
  isolation: isolate;
  display: flex;
  align-items: center;
  gap: 12px;
  height: 48px;
  padding: 0 10px;
  border-radius: 10px;
  cursor: pointer;
}
.cp-row.active::before {
  content: '';
  position: absolute;
  inset: 0;
  z-index: -1;
  border-radius: inherit;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 18%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 27%, var(--t-pop))
  );
}
.cp-row.disabled {
  cursor: default;
}
.cp-row.disabled .cp-title,
.cp-row.muted .cp-title {
  color: var(--t-text3);
}
.cp-lead {
  flex: 0 0 32px;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--t-pill);
  color: var(--t-text2);
}
.cp-avatar {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  font-size: 10px;
  font-weight: 600;
  background: var(--t-kbd);
}
.cp-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
}
.cp-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.cp-title,
.cp-sub {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cp-title {
  font-size: 14px;
  line-height: 19px;
}
.cp-sub {
  font-size: 12px;
  line-height: 16px;
  color: var(--t-text3);
}
.cp-hit {
  background: color-mix(in srgb, var(--t-primary) 32%, transparent);
  color: inherit;
  border-radius: 3px;
  padding: 0 1px;
  font-weight: 600;
}
.cp-value {
  font-size: 12.5px;
  color: var(--t-text2);
  white-space: nowrap;
}
.cp-check {
  color: var(--t-primary);
  flex: none;
}
.cp-hints,
.cp-caps {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text2);
  white-space: nowrap;
}
.cp-caps {
  gap: 3px;
}
.cp-hint-l {
  margin-right: 8px;
}

/* ── footer ── */
.cp-foot {
  flex: none;
  display: flex;
  align-items: center;
  gap: 16px;
  height: 40px;
  padding: 0 16px;
  background: color-mix(in srgb, var(--t-pop) 55%, var(--t-bg));
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
}
.cp-foot-item {
  display: flex;
  align-items: center;
  gap: 6px;
}
.cp-foot-sp {
  flex: 1;
}
.cp-foot-right {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.cp-enter-active,
.cp-leave-active {
  transition: opacity 0.14s ease;
}
.cp-enter-active .cp,
.cp-leave-active .cp {
  transition: transform 0.14s ease;
}
.cp-enter-from,
.cp-leave-to {
  opacity: 0;
}
.cp-enter-from .cp,
.cp-leave-to .cp {
  transform: translateY(-6px) scale(0.99);
}

@media (max-width: 768px) {
  /* The whole screen, over the keyboard's top edge (#2976). */
  .cp-scrim {
    padding: 0;
    background: var(--t-bg);
  }
  .cp {
    width: 100%;
    height: calc(var(--app-vh, 100vh) - var(--kb-h, 0px));
    max-height: none;
    padding-top: calc(10px + var(--safe-top));
    box-sizing: border-box;
    border-radius: 0;
    background: var(--t-bg);
    box-shadow: none;
  }
  .cp-input {
    height: 48px;
    margin: 0 10px 10px 12px;
    padding: 0 6px 0 14px;
    gap: 10px;
    border-radius: 14px;
    background: var(--t-sub);
    box-shadow: inset 0 0 0 2px var(--t-primary);
  }
  .cp-cancel {
    flex: none;
    height: 44px;
    margin-right: -88px;
    padding: 0 4px 0 18px;
    border: 0;
    background: transparent;
    color: var(--t-primary);
    font: inherit;
    font-size: 15px;
    font-weight: 650;
    cursor: pointer;
  }
  .cp-input:has(.cp-cancel) {
    margin-right: 92px;
  }
  /* The areas scroll sideways in one row of 34px chips. */
  .cp-tabs {
    flex-wrap: nowrap;
    overflow-x: auto;
    padding: 0 12px 10px;
    scrollbar-width: none;
  }
  .cp-tabs::-webkit-scrollbar {
    display: none;
  }
  .cp-tab {
    flex: none;
    height: 34px;
    padding: 0 14px;
    border-radius: 17px;
    font-size: 13.5px;
  }
  .cp-row {
    min-height: 56px;
  }
  .cp-foot {
    display: none;
  }
  .cp-back,
  .cp-hints,
  .cp-caps,
  .cp :deep(.sk) {
    display: none;
  }
}
</style>
