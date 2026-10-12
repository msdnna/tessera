<script setup>
// «Команды пространства» — a large window (v2, #2975, mockup CmdsWin) over the
// workspace's own `/`-dictionary for the editor: entries that are only text
// (they never execute — the built-in list is what the backend runs).
//
// Rows are edited in place: Enter opens a row into a form, a second Enter saves
// it, Esc gives up. Each change goes out at once as the complete dictionary in
// one PUT (the endpoint is full-replace, like the tag prefixes); deleting is
// undone from its toast, ⌥↑/⌥↓ reorder (order in the list is order in the
// popup). Palette macros and task templates of the mockup are #3069.
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  FlashOutline,
  CodeSlashOutline,
  TerminalOutline,
  DocumentTextOutline,
  AddOutline,
  EllipsisHorizontal,
  CreateOutline,
  ArrowUpOutline,
  ArrowDownOutline,
  TrashOutline,
  InformationCircleOutline,
  LockClosedOutline,
} from '@vicons/ionicons5'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useUndoStore } from '@/stores/undo'
import { useShortcuts } from '@/composables/useShortcuts'
import { useResponsive } from '@/composables/useResponsive'
import { builtinCommandKeys, canonCommandKey, isValidCommandKey } from '@/utils/commands'
import { CAPS, cap } from '@/utils/keyCaps'
import { isMac } from '@/utils/keys'
import { useMacrosStore } from '@/stores/macros'
import MacrosPane from './commands/MacrosPane.vue'
import TemplatesPane from './commands/TemplatesPane.vue'
import { workspaceName } from '@/utils/defaultNames'
import LargeWindow from './kit/LargeWindow.vue'
import ActionMenu from './kit/ActionMenu.vue'
import Kbd from './Kbd.vue'
import './kit/kit.css'

const MAX_ROWS = 50

const props = defineProps({
  show: { type: Boolean, default: false },
  workspaceId: { type: String, default: '' },
})
const emit = defineEmits(['update:show'])

const { t } = useI18n()
const store = useWorkspacesStore()
const undo = useUndoStore()
const message = useMessage()
const section = ref('custom')
const winRef = ref(null)
const saving = ref(false)
// The dictionary as the server has it — every write replaces it whole.
const rows = ref([])

const canEdit = computed(() => store.commandsCanManage)
const workspace = computed(() => store.list.find((w) => w.id === props.workspaceId) || null)

const builtin = computed(() => store.commands.filter((c) => c.builtin))
// Aliases count too — the backend's IsBuiltin resolves them, so "/done" is as
// taken as "/close".
const builtinKeys = computed(() => builtinCommandKeys(builtin.value))

// Per-row validation, shown inline — the backend rejects the whole PUT on the
// first bad key, so catching it here keeps the user from losing the other rows.
function rowError(row, i, list = rows.value) {
  const key = canonCommandKey(row.key)
  if (!key) return t('shell.commands.error.empty')
  if (!isValidCommandKey(key)) return t('shell.commands.error.charset')
  if (builtinKeys.value.has(key)) return t('shell.commands.error.builtin')
  if (list.some((r, j) => j < i && canonCommandKey(r.key) === key))
    return t('shell.commands.error.duplicate')
  if ((row.description || '').length > 200) return t('shell.commands.error.tooLong')
  return ''
}
const errors = computed(() => rows.value.map((r, i) => rowError(r, i)))

// persist writes the whole dictionary. A list with a bad row never leaves —
// the server would refuse it all; the row says why.
async function persist(next) {
  if (next.some((r, i) => rowError(r, i, next))) return false
  saving.value = true
  try {
    const payload = next.map((r) => ({
      key: canonCommandKey(r.key),
      description: (r.description || '').trim(),
    }))
    const res = await wsApi.setCommands(props.workspaceId, payload)
    const saved = res?.data || payload
    store.setCustomCommands(saved)
    rows.value = saved.map((c) => ({ key: c.key, description: c.description || '' }))
    return true
  } catch (e) {
    message.error(e.message)
    return false
  } finally {
    saving.value = false
  }
}

// ── editing a row in place ──
// `editing` is the index of the open form (rows.length = the new row).
const editing = ref(-1)
const draft = ref({ key: '', description: '' })
const keyEl = ref(null)
// The rows plus one empty slot at the end — where the new row's form opens.
const slots = computed(() => [...rows.value, null])
function setKeyEl(el) {
  if (el) keyEl.value = el
}
const draftList = computed(() => {
  const list = rows.value.slice()
  if (editing.value < 0) return list
  list[editing.value] = { ...draft.value }
  return list
})
const draftError = computed(() =>
  editing.value < 0 ? '' : rowError(draft.value, editing.value, draftList.value),
)
// A fresh row doesn't shout «пустой ключ» before anything was typed.
const showDraftError = computed(
  () => !!draftError.value && !(editing.value === rows.value.length && !draft.value.key),
)
// Another (stored) row is broken: saving this one would send it too, and the
// server refuses the lot — say which one.
const blockingRow = computed(() =>
  editing.value < 0
    ? -1
    : draftList.value.findIndex((r, i) => i !== editing.value && rowError(r, i, draftList.value)),
)

function startEdit(i) {
  if (!canEdit.value || saving.value) return
  if (i > rows.value.length || (i === rows.value.length && rows.value.length >= MAX_ROWS)) return
  editing.value = i
  cursor.value = i
  const r = rows.value[i] || { key: '', description: '' }
  draft.value = { key: r.key, description: r.description }
  nextTick(() => keyEl.value?.focus())
}
function cancelEdit() {
  const i = editing.value
  editing.value = -1
  nextTick(() => focusRow(Math.min(i, rows.value.length - 1)))
}
async function saveEdit() {
  if (editing.value < 0 || draftError.value || blockingRow.value >= 0) return
  const i = editing.value
  if (await persist(draftList.value)) {
    editing.value = -1
    nextTick(() => focusRow(i))
  }
}
function onFormKey(e) {
  if (e.isComposing) return
  if (e.key === 'Enter') {
    e.preventDefault()
    saveEdit()
  } else if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    cancelEdit()
  }
}

// ── the list under the keyboard ──
const cursor = ref(-1)
const rowEls = {}
function focusRow(i) {
  if (i < 0 || i >= rows.value.length) return
  cursor.value = i
  rowEls[i]?.focus()
}
async function move(i, delta) {
  const j = i + delta
  if (!canEdit.value || j < 0 || j >= rows.value.length) return
  const next = rows.value.slice()
  ;[next[i], next[j]] = [next[j], next[i]]
  if (await persist(next)) nextTick(() => focusRow(j))
}
async function remove(i) {
  if (!canEdit.value) return
  const before = rows.value.slice()
  const gone = before[i]
  const next = before.filter((_, j) => j !== i)
  if (!(await persist(next))) return
  nextTick(() => focusRow(Math.min(i, rows.value.length - 1)))
  undo.push({
    text: { key: 'shell.commands.deleted', params: { key: gone.key } },
    doneText: { key: 'shell.commands.restored', params: { key: gone.key } },
    undo: async () => {
      if (!(await persist(before))) throw new Error('restore failed')
    },
  })
}
function onListKey(e) {
  if (e.isComposing || editing.value >= 0 || e.target?.tagName === 'INPUT') return
  const i = cursor.value
  const plain = !e.metaKey && !e.ctrlKey && !e.altKey
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    const step = e.key === 'ArrowDown' ? 1 : -1
    if (e.altKey && i >= 0) move(i, step)
    else if (plain) focusRow(Math.max(0, Math.min(rows.value.length - 1, i + step)))
  } else if (e.key === 'Enter' && plain && i >= 0) {
    e.preventDefault()
    startEdit(i)
  } else if ((e.key === 'Backspace' || e.key === 'Delete') && plain && i >= 0) {
    e.preventDefault()
    remove(i)
  }
}

// ── the row menu ──
const menuFor = ref(-1)
const menuAnchor = ref(null)
const menuShow = computed({
  get: () => menuFor.value >= 0,
  set: (v) => {
    if (!v) menuFor.value = -1
  },
})
const ALT = isMac ? '⌥' : 'Alt'
const menuItems = computed(() => [
  { key: 'edit', label: t('shell.commands.edit'), icon: CreateOutline, caps: [CAPS.enter] },
  {
    key: 'up',
    label: t('shell.commands.up'),
    icon: ArrowUpOutline,
    caps: [ALT, '↑'],
    disabled: menuFor.value <= 0,
  },
  {
    key: 'down',
    label: t('shell.commands.down'),
    icon: ArrowDownOutline,
    caps: [ALT, '↓'],
    disabled: menuFor.value >= rows.value.length - 1,
  },
  { gap: true },
  {
    key: 'delete',
    label: t('common.action.delete'),
    icon: TrashOutline,
    danger: true,
    caps: [CAPS.backspace],
  },
])
function openMenu(i, el) {
  cursor.value = i
  menuAnchor.value = el
  menuFor.value = i
}
function onMenu(key) {
  const i = menuFor.value
  menuFor.value = -1
  if (key === 'edit') startEdit(i)
  else if (key === 'up') move(i, -1)
  else if (key === 'down') move(i, 1)
  else if (key === 'delete') remove(i)
}

// Palette commands and task templates (#3069): their own panes.
const macros = useMacrosStore()
const macrosPane = ref(null)
const templatesPane = ref(null)
const navItems = computed(() => [
  {
    key: 'custom',
    icon: FlashOutline,
    label: t('shell.commands.navCustom'),
    count: rows.value.length || null,
  },
  {
    key: 'builtin',
    icon: CodeSlashOutline,
    label: t('shell.commands.navBuiltin'),
    count: builtin.value.length || null,
  },
  { heading: t('shell.commands.navPalette') },
  {
    key: 'macros',
    icon: TerminalOutline,
    label: t('shell.commands.navMacros'),
    count: macros.macros.length || null,
  },
  {
    key: 'templates',
    icon: DocumentTextOutline,
    label: t('shell.commands.navTemplates'),
    count: macros.templates.length || null,
  },
])

function paneEscape() {
  for (const pane of [macrosPane.value, templatesPane.value]) {
    if (pane?.editing != null) {
      pane.close()
      return true
    }
  }
  if (menuFor.value >= 0) {
    menuFor.value = -1
    return true
  }
  if (editing.value >= 0) {
    cancelEdit()
    return true
  }
  return false
}
const topmost = () => !!winRef.value?.topmost()
useShortcuts(
  {
    'win.new': () => {
      if (section.value === 'macros') return macrosPane.value?.open(null)
      if (section.value === 'templates') return templatesPane.value?.open(null)
      if (!canEdit.value || editing.value >= 0) return
      section.value = 'custom'
      startEdit(rows.value.length)
    },
  },
  { when: topmost },
)
// Seed from the store whenever the window opens.
watch(
  () => props.show,
  (s) => {
    if (!s) {
      editing.value = -1
      menuFor.value = -1
      return
    }
    rows.value = (store.customCommands || []).map((c) => ({
      key: c.key,
      description: c.description || '',
    }))
    editing.value = -1
    cursor.value = -1
  },
  { immediate: true },
)

// The list takes the keyboard on open — not on a phone, where focus would
// only draw a ring nobody asked for.
const { isMobile } = useResponsive()
function onOpened() {
  if (rows.value.length && !isMobile.value) focusRow(0)
}
</script>

<template>
  <LargeWindow
    ref="winRef"
    v-model:section="section"
    :show="show"
    :crumbs="[workspaceName(workspace || store.current)]"
    :title="t('shell.commands.windowTitle')"
    :icon="FlashOutline"
    :items="navItems"
    :escape="paneEscape"
    testid="commands-window"
    tab-testid="commands-tab"
    @update:show="emit('update:show', $event)"
    @opened="onOpened"
  >
    <template #aside>
      <span class="cw-aside">
        {{ t('shell.commands.aside') }}
        <Kbd>{{ cap('/') }}</Kbd>
      </span>
    </template>
    <template #nav-foot>
      <div class="lw-foot-row">{{ t('shell.commands.footShared') }}</div>
      <div class="lw-foot-row">{{ t('shell.commands.footSave') }}</div>
    </template>

    <div class="cw cmd-modal">
      <MacrosPane v-if="section === 'macros'" ref="macrosPane" />
      <TemplatesPane v-else-if="section === 'templates'" ref="templatesPane" />
      <template v-else-if="section === 'custom'">
        <div class="cw-title">
          <h2>{{ t('shell.commands.navCustom') }}</h2>
          <span class="cw-note">{{ t('shell.commands.summary', rows.length) }}</span>
          <span class="cw-grow" />
          <button
            v-if="canEdit"
            type="button"
            class="kit-btn primary"
            data-testid="cmd-new"
            :disabled="rows.length >= 50 || editing >= 0"
            @click="startEdit(rows.length)"
          >
            <n-icon :component="AddOutline" :size="15" />
            {{ t('shell.commands.new') }}
            <Kbd>{{ cap('n') }}</Kbd>
          </button>
        </div>

        <div class="cw-head">
          <span />
          <span>{{ t('shell.commands.colKey') }}</span>
          <span>{{ t('shell.commands.colDesc') }}</span>
          <span />
        </div>

        <div class="cw-body t-hoverscroll" tabindex="-1" @keydown="onListKey">
          <!-- The rows, then one more slot: the new row's form, or its door. -->
          <template v-for="(row, i) in slots" :key="i">
            <div
              v-if="editing === i"
              class="cw-form cmd-row"
              data-testid="cmd-form"
              @keydown="onFormKey"
            >
              <label class="cw-field">
                <span class="cw-fl">{{ t('shell.commands.colKey') }}</span>
                <span class="cw-key-in" :class="{ error: showDraftError }">
                  <span class="cmd-slash">/</span>
                  <input
                    :ref="setKeyEl"
                    v-model="draft.key"
                    maxlength="32"
                    spellcheck="false"
                    autocomplete="off"
                    :placeholder="t('shell.commands.keyPlaceholder')"
                    data-testid="cmd-key"
                  />
                </span>
              </label>
              <label class="cw-field cw-field-grow">
                <span class="cw-fl">{{ t('shell.commands.colDesc') }}</span>
                <input
                  v-model="draft.description"
                  class="kf-input"
                  maxlength="200"
                  :placeholder="t('shell.commands.descPlaceholder')"
                  data-testid="cmd-desc"
                />
              </label>
              <div class="cw-form-foot">
                <span v-if="showDraftError" class="cmd-err">{{ draftError }}</span>
                <span v-else-if="blockingRow >= 0" class="cmd-err">
                  {{ t('shell.commands.blocked', { key: rows[blockingRow].key }) }}
                </span>
                <span class="cw-grow" />
                <span class="cw-keys">
                  <Kbd>{{ CAPS.tab }}</Kbd> {{ t('shell.commands.keyField') }}
                  <Kbd>{{ CAPS.enter }}</Kbd> {{ t('shell.commands.keySave') }}
                  <Kbd>{{ CAPS.escape }}</Kbd> {{ t('shell.commands.keyCancel') }}
                </span>
                <button type="button" class="kit-btn small" @click="cancelEdit">
                  {{ t('common.action.cancel') }}
                </button>
                <button
                  type="button"
                  class="kit-btn small primary"
                  data-testid="cmd-save"
                  :disabled="saving || !!draftError || blockingRow >= 0"
                  @click="saveEdit"
                >
                  {{ t('common.action.save') }}
                </button>
              </div>
            </div>
            <div
              v-else-if="row"
              :ref="(el) => (rowEls[i] = el)"
              class="cw-row cmd-row"
              :class="{ ctx: menuFor === i, bad: !!errors[i] }"
              tabindex="-1"
              data-testid="cmd-row"
              @focus="cursor = i"
              @click="cursor = i"
              @dblclick="startEdit(i)"
            >
              <span class="cw-tile"><n-icon :component="FlashOutline" :size="15" /></span>
              <span class="cw-key">
                <code>/{{ row.key }}</code>
                <span v-if="errors[i]" class="cmd-err">{{ errors[i] }}</span>
              </span>
              <span class="cw-desc">{{ row.description }}</span>
              <button
                v-if="canEdit"
                type="button"
                class="cw-more"
                tabindex="-1"
                :title="t('common.action.more')"
                data-testid="cmd-menu"
                @click.stop="openMenu(i, $event.currentTarget)"
              >
                <n-icon :component="EllipsisHorizontal" :size="16" />
              </button>
              <n-icon
                v-else
                :component="LockClosedOutline"
                :size="13"
                class="cw-lock"
                :title="t('shell.commands.readOnly')"
              />
            </div>
            <button
              v-else-if="canEdit && rows.length < 50"
              type="button"
              class="cw-add"
              data-testid="cmd-add-row"
              @click="startEdit(rows.length)"
            >
              <n-icon :component="AddOutline" :size="15" />
              {{ t('shell.commands.addRow') }}
              <span class="cw-grow" />
              <Kbd soft>{{ cap('n') }}</Kbd>
            </button>
          </template>
          <p v-if="!rows.length && editing < 0" class="cw-empty">
            {{ t('shell.commands.empty') }}
          </p>
          <p v-if="!canEdit" class="cw-empty">{{ t('shell.commands.readOnly') }}</p>
        </div>

        <div class="cw-info">
          <n-icon :component="InformationCircleOutline" :size="16" />
          <span>{{ t('shell.commands.hint') }}</span>
        </div>
      </template>

      <template v-else>
        <div class="cw-title">
          <h2>{{ t('shell.commands.navBuiltin') }}</h2>
          <span class="cw-note">{{ t('shell.commands.builtinNote') }}</span>
        </div>
        <div class="cw-body cmd-builtin t-hoverscroll">
          <div v-for="c in builtin" :key="c.key" class="cw-row static" data-testid="cmd-builtin">
            <span class="cw-tile"><n-icon :component="CodeSlashOutline" :size="15" /></span>
            <span class="cw-key">
              <code class="cmd-chip">/{{ c.key }}</code>
              <span v-if="c.aliases.length" class="cw-alias">
                {{ c.aliases.map((a) => `/${a}`).join(' ') }}
              </span>
            </span>
            <span class="cw-desc">
              {{ c.description }}
              <code v-if="c.example" class="cw-example">{{ c.example }}</code>
            </span>
          </div>
        </div>
      </template>
    </div>

    <ActionMenu
      v-model:show="menuShow"
      :anchor="() => menuAnchor"
      placement="bottom-end"
      :offset="{ x: 0, y: 6 }"
      :items="menuItems"
      :width="260"
      testid="cmd-row-menu"
      @select="onMenu"
    />
  </LargeWindow>
</template>

<style scoped>
.cw {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  padding: 6px 16px 0 0;
}
.lw-mobile .cw {
  padding-right: 0;
}
.cw-aside {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  color: var(--t-text3);
  margin-right: 6px;
}
.lw-mobile .cw-aside {
  display: none;
}
.cw-title {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 10px;
  margin-bottom: 14px;
  min-height: 36px;
}
.cw-title h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
}
.cw-note {
  font-size: 13px;
  color: var(--t-text3);
}
.cw-grow {
  flex: 1;
}
.cw-head,
.cw-row {
  display: grid;
  grid-template-columns: 36px minmax(140px, 220px) minmax(0, 1fr) 32px;
  align-items: center;
  gap: 10px;
}
.cw-head {
  padding: 0 10px 8px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.cw-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  outline: 0;
  padding: 2px 2px 16px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.cw-row {
  min-height: 46px;
  padding: 4px 10px;
  border-radius: 12px;
  outline: 0;
  cursor: default;
}
.cw-row:not(.static):hover,
.cw-row.ctx {
  background: var(--t-hover);
}
.cw-row:focus-visible {
  background: var(--t-hover);
  box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--t-primary) 55%, transparent);
}
.cw-tile {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 9px;
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-primary);
}
.cw-tile :deep([stroke='currentColor']) {
  stroke: url(#t-accent-grad-svg);
}
.cw-tile :deep([fill='currentColor']) {
  fill: url(#t-accent-grad-svg);
}
.cw-row.static .cw-tile {
  background: var(--t-pill);
  color: var(--t-text3);
}
.cw-row.static .cw-tile :deep([stroke='currentColor']) {
  stroke: currentColor;
}
.cw-key {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.cw-key code,
.cmd-chip {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cw-alias {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11.5px;
  color: var(--t-text3);
}
.cw-desc {
  min-width: 0;
  font-size: 13px;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cw-example {
  margin-left: 8px;
  padding: 1px 6px;
  border-radius: 6px;
  background: var(--t-pill);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11.5px;
  color: var(--t-text3);
}
.cmd-err {
  font-size: 11.5px;
  color: var(--t-error);
}
.cw-more {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  opacity: 0;
}
.cw-row:hover .cw-more,
.cw-row.ctx .cw-more,
.cw-row:focus-visible .cw-more {
  opacity: 1;
}
.cw-more:hover {
  background: var(--t-ctl);
  color: var(--t-text1);
}
.cw-lock {
  color: var(--t-text3);
}
/* The row in edit: a block on --t-sub, radius 14, with a 2px ring. */
.cw-form {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 12px;
  padding: 12px 14px;
  margin: 4px 0;
  border-radius: 14px;
  background: var(--t-sub);
  box-shadow: 0 0 0 2px var(--t-primary);
}
.cw-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 220px;
}
.cw-field-grow {
  flex: 1;
  min-width: 200px;
  width: auto;
}
.cw-fl {
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text2);
}
.cw-key-in {
  display: flex;
  align-items: center;
  gap: 2px;
  height: 36px;
  padding: 0 10px;
  box-sizing: border-box;
  border-radius: 9px;
  background: var(--t-input-bg);
}
.cw-key-in:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.cw-key-in.error {
  box-shadow: 0 0 0 2px var(--t-error);
}
.cw-key-in input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13.5px;
}
.cmd-slash {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  color: var(--t-text3);
}
.cw-form-foot {
  flex-basis: 100%;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 12px;
  color: var(--t-text3);
}
.cw-keys {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.lw-mobile .cw-keys {
  display: none;
}
.cw-add {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 42px;
  padding: 0 12px;
  border: 0;
  border-radius: 12px;
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.cw-add:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.lw-mobile .cw-add :deep(.kbd),
.lw-mobile .cw-title :deep(.kbd) {
  display: none;
}
.cw-empty {
  margin: 14px 10px;
  font-size: 13px;
  color: var(--t-text3);
}
.cw-info {
  flex: none;
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin: 8px 0 4px;
  padding: 10px 14px;
  border-radius: 12px;
  background: var(--t-sub);
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--t-text2);
}
.cw-info :deep(.n-icon) {
  flex: none;
  margin-top: 1px;
  color: var(--t-text3);
}
.lw-mobile .cw-head {
  display: none;
}
.lw-mobile .cw-row {
  grid-template-columns: 30px minmax(0, 1fr) 28px;
}
.lw-mobile .cw-desc {
  grid-column: 2;
  white-space: normal;
}
.lw-mobile .cw-more {
  opacity: 1;
  grid-row: 1;
  grid-column: 3;
}
.lw-mobile .cw-field {
  width: 100%;
}
</style>
