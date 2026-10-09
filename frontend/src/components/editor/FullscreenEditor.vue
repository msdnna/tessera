<script setup>
// The fullscreen editor (#3118, PackageJ «10 · Полноэкранный редактор»): a large
// window over the task modal for a long description or comment. Built on
// kit/LargeWindow at 1240×788: the task and «Описание» in the header with the
// Написать · Рядом · Просмотр segment (⌘1 / ⌘2 / ⌘3) in the middle, the toolbar
// over the panes, and a footer «Сохранено · 12:40 · Esc свернуть · Готово ⌘↵».
// «Рядом» is the default (a phone starts on «Написать» — two panes side by side
// don't fit there).
//
// The text stays in the caller's v-model: the window edits through it rather
// than keeping a copy, so folding it (Esc, ✕) loses nothing, and while it is up
// the typing is handed to the caller's persist every couple of seconds — the
// description autosaves as it does on blur. ⌘↵ / «Готово» is the field's own
// primary action: the description saves and leaves, the composer sends.
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { ContractOutline, CloudDoneOutline } from '@vicons/ionicons5'
import LargeWindow from '../kit/LargeWindow.vue'
import MarkdownEditor from '../MarkdownEditor.vue'
import TesseraIcon from '../TesseraIcon.vue'
import Kbd from '../Kbd.vue'
import ShortcutKeys from '../ShortcutKeys.vue'
import { CAPS } from '@/utils/keyCaps'
import { chordsOf } from '@/shortcuts/labels'
import { modeAction } from '@/utils/edKeys'
import { useResponsive } from '@/composables/useResponsive'

const props = defineProps({
  modelValue: { type: String, default: '' },
  // null → «Описание».
  title: { type: String, default: null },
  // The task the text belongs to: { number, icon, color } — the header's crumb.
  task: { type: Object, default: null },
  placeholder: { type: String, default: null },
  mentionItems: { type: Array, default: () => [] },
  taskPeople: { type: Array, default: () => [] },
  commandItems: { type: Array, default: () => [] },
  attachTaskId: { type: String, default: null },
  // The primary button's word (null → «Готово»; the composer says «Отправить»).
  doneLabel: { type: String, default: null },
})
const emit = defineEmits(['update:modelValue', 'persist', 'close', 'done', 'attachments-changed'])

const { t, locale } = useI18n()
const { isMobile } = useResponsive()
const show = ref(true)
const editor = ref(null)
const win = ref(null)
const mode = ref(typeof window !== 'undefined' && window.innerWidth <= 768 ? 'write' : 'split')
const MODES = ['write', 'split', 'preview']
const headTitle = computed(() => props.title ?? t('documents.editor.fullscreen.title'))
const crumbs = computed(() => (props.task?.number ? [`#${props.task.number}`] : []))
// «⌘1» on a Mac, «Ctrl+1» elsewhere — quiet caps inside the segment.
const capOf = (m) =>
  chordsOf(`editor.${m}`)
    .map((c) => c.join(c[0] === 'Ctrl' ? '+' : ''))
    .join(' ')

function setMode(m) {
  if (!MODES.includes(m)) return
  mode.value = m
  if (m !== 'preview') setTimeout(() => editor.value?.focus(), 0)
}

// ── autosave while the window is up ──
const SAVE_DELAY = 1500
const savedAt = ref(null)
let saveTimer = 0
let dirty = false
watch(
  () => props.modelValue,
  () => {
    dirty = true
    clearTimeout(saveTimer)
    saveTimer = setTimeout(flush, SAVE_DELAY)
  },
)
function flush() {
  clearTimeout(saveTimer)
  if (!dirty) return
  dirty = false
  emit('persist')
  savedAt.value = new Date()
}
const savedText = computed(() => {
  if (!savedAt.value) return ''
  const time = new Intl.DateTimeFormat(locale.value, {
    hour: '2-digit',
    minute: '2-digit',
  }).format(savedAt.value)
  return t('documents.editor.fullscreen.saved', { time })
})

// Esc, ✕ and Back fold the window; what was typed is already in the caller's
// text, and the last of it is handed to persist on the way out.
function fold() {
  flush()
  show.value = false
  emit('close')
}
function done() {
  clearTimeout(saveTimer)
  dirty = false
  show.value = false
  emit('done')
}

// ⌘1 / ⌘2 / ⌘3 — from the field too, so a key of the window rather than of the
// global dispatcher (which leaves a text field's keys alone).
function onKey(e) {
  if (!show.value || !win.value?.topmost?.()) return
  const m = modeAction(e)
  if (!m) return
  e.preventDefault()
  e.stopPropagation()
  setMode(m)
}
onMounted(() => {
  window.addEventListener('keydown', onKey, true)
  // The window is mounted already open, so LargeWindow's `opened` never fires:
  // the field takes the focus once the modal's own focus handling has run.
  setTimeout(onOpened, 60)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey, true)
  clearTimeout(saveTimer)
})

function onOpened() {
  editor.value?.focus()
}
</script>

<template>
  <LargeWindow
    ref="win"
    :show="show"
    wide
    no-nav
    :crumbs="crumbs"
    :title="headTitle"
    :close-icon="ContractOutline"
    :close-label="t('documents.editor.fullscreen.fold')"
    testid="md-fullscreen"
    @update:show="!$event && fold()"
    @opened="onOpened"
  >
    <template v-if="task?.icon" #badge>
      <span class="fe-st" :style="task.color ? { color: task.color } : null">
        <TesseraIcon :name="task.icon" :size="15" />
      </span>
    </template>
    <template #center>
      <div class="fe-seg" role="tablist" data-testid="fs-modes">
        <button
          v-for="m in MODES"
          :key="m"
          type="button"
          role="tab"
          class="fe-seg-b"
          :class="{ on: mode === m }"
          :aria-selected="mode === m"
          :data-testid="`fs-mode-${m}`"
          @mousedown.prevent
          @click="setMode(m)"
        >
          {{ t(`documents.editor.fullscreen.${m}`) }}
          <span v-if="!isMobile" class="fe-cap">{{ capOf(m) }}</span>
        </button>
      </div>
    </template>

    <div class="fe-body">
      <MarkdownEditor
        ref="editor"
        variant="full"
        :full-mode="mode"
        :model-value="modelValue"
        :placeholder="placeholder"
        :mention-items="mentionItems"
        :task-people="taskPeople"
        :command-items="commandItems"
        :attach-task-id="attachTaskId"
        :expandable="false"
        @update:model-value="emit('update:modelValue', $event)"
        @attachments-changed="emit('attachments-changed')"
        @persist="emit('persist')"
        @full-mode="setMode"
        @cancel="fold"
        @submit="done"
      />
    </div>

    <template #foot>
      <template v-if="savedText">
        <n-icon :component="CloudDoneOutline" :size="15" class="fe-ok" />
        <span data-testid="fs-saved">{{ savedText }}</span>
      </template>
      <span v-if="mode === 'split' && !isMobile" class="fe-sync">{{
        t('documents.editor.fullscreen.syncScroll')
      }}</span>
      <span class="fe-grow" />
      <span v-if="!isMobile" class="fe-esc"
        ><Kbd>{{ CAPS.escape }}</Kbd> {{ t('documents.editor.fullscreen.foldHint') }}</span
      >
      <button
        type="button"
        class="fe-done"
        data-testid="fs-done"
        @mousedown.prevent
        @click="editor && done()"
      >
        {{ doneLabel ?? t('documents.editor.fullscreen.done') }}
        <ShortcutKeys v-if="!isMobile" id="editor.send" />
      </button>
    </template>
  </LargeWindow>
</template>

<style scoped>
.fe-st {
  display: flex;
  color: var(--t-text2);
}
.fe-seg {
  flex: none;
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: 9px;
  background: var(--t-ctl);
}
.fe-seg-b {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 12px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
}
.fe-seg-b.on {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.fe-seg-b:not(.on):hover {
  color: var(--t-text1);
}
.fe-cap {
  font-size: 11px;
  font-weight: 500;
  color: var(--t-text3);
}
.fe-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 0 4px;
}
.fe-body > * {
  flex: 1;
  min-height: 0;
}
.fe-ok {
  color: #18a058;
}
.fe-sync {
  margin-left: 16px;
}
.fe-grow {
  flex: 1;
}
.fe-esc {
  display: flex;
  align-items: center;
  gap: 5px;
}
.fe-done {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 32px;
  margin-left: 8px;
  padding: 0 6px 0 12px;
  border: 0;
  border-radius: 9px;
  background: var(--t-accent-grad);
  color: #fff;
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: box-shadow 0.12s ease;
}
.fe-done:hover {
  box-shadow: 0 4px 14px color-mix(in srgb, var(--t-primary) 40%, transparent);
}
</style>
