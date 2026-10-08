<script setup>
// «Быстрые действия» (v2, #2956): the floating + in the working area's corner.
// N opens it, the second key picks an item (T task into «К работе», R reminder,
// M a record), Esc closes; «N, then T» also works in one go while it's closed.
// Reminders and notes are captured by the «Входящие» popover right above the
// button (#2996); while it is open the button turns into its close cross.
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { NIcon } from 'naive-ui'
import {
  AddOutline,
  AlarmOutline,
  CloseOutline,
  DocumentTextOutline,
  SearchOutline,
} from '@vicons/ionicons5'
import ShortcutKeys from './ShortcutKeys.vue'
import CapturePop from './inbox/CapturePop.vue'
import { useQuickStore } from '@/stores/quick'
import { useInboxStore } from '@/stores/inbox'
import { useBoardViewStore } from '@/stores/boardView'
import { usePaletteStore } from '@/stores/palette'
import { useResponsive } from '@/composables/useResponsive'
import { useShortcuts } from '@/composables/useShortcuts'

const { t } = useI18n()
const route = useRoute()
const quick = useQuickStore()
const inbox = useInboxStore()
const boardView = useBoardViewStore()
const palette = usePaletteStore()
const { isMobile } = useResponsive()

const hint = ref(false)
const rootEl = ref(null)

// The button stays out of screens that own their bottom-right corner: a call
// room, an open document (its comment composer sends from there) and an open
// note. «N» still opens the menu there — it just has no button to sit above.
const buttonVisible = computed(() => {
  const p = route.path
  if (/^\/(conferences|documents)\/./.test(p)) return false
  // A phone (#2976 — package G): screens without anything to create keep the
  // corner free — the stage page, «Ещё», the notification feed.
  if (isMobile.value && (/\/stage\//.test(p) || p === '/more' || p === '/notifications'))
    return false
  return true
})
const capturing = computed(() => !!inbox.capture)
const visible = computed(() => buttonVisible.value || quick.fabOpen || capturing.value)

// «N, M» — a record typed by its text (a note unless it names a moment); «N, R»
// — a reminder, the type fixed.
function captureNote() {
  quick.fabOpen = false
  inbox.startCapture()
}
function captureReminder() {
  quick.fabOpen = false
  inbox.startCapture({ kind: 'reminder' })
}
function onFab() {
  if (capturing.value) return inbox.endCapture()
  quick.fabOpen = !quick.fabOpen
}

// On an open kanban the task goes into the board's own «К работе» quick-add,
// right where the user is looking; anywhere else the dialog asks for a board.
function createTask() {
  quick.fabOpen = false
  if (boardView.active && boardView.layout === 'board') boardView.requestQuickAdd()
  else quick.create('task')
}

const ITEMS = [
  { id: 'task', icon: AddOutline, keys: 'create.task', run: createTask },
  { id: 'reminder', icon: AlarmOutline, keys: 'create.reminder', run: captureReminder },
  { id: 'note', icon: DocumentTextOutline, keys: 'create.note', run: captureNote },
]
// The bare second key, shown in the menu (the header says «N, затем»).
const SECOND = { task: 'fab.task', reminder: 'fab.reminder', note: 'fab.note' }

function openPalette() {
  quick.fabOpen = false
  palette.show()
}

const isOpen = () => quick.fabOpen
useShortcuts({
  'board.quick': () => (quick.fabOpen = !quick.fabOpen),
  'create.task': createTask,
  'create.reminder': captureReminder,
  'create.note': captureNote,
})
useShortcuts(
  {
    'fab.task': createTask,
    'fab.reminder': captureReminder,
    'fab.note': captureNote,
    'fab.close': () => (quick.fabOpen = false),
  },
  { when: isOpen },
)

// A click anywhere else closes the menu.
function onDocDown(e) {
  if (!rootEl.value?.contains(e.target)) quick.fabOpen = false
}
watch(
  () => quick.fabOpen,
  (open) => {
    if (open) document.addEventListener('mousedown', onDocDown, true)
    else document.removeEventListener('mousedown', onDocDown, true)
  },
)
onBeforeUnmount(() => document.removeEventListener('mousedown', onDocDown, true))
</script>

<template>
  <div
    v-if="visible"
    ref="rootEl"
    class="fab-wrap"
    :class="{
      mobile: isMobile,
      docked: boardView.panelOpen && !isMobile,
      inboxed: inbox.panelOpen && !isMobile,
    }"
    :style="inbox.panelOpen && !isMobile ? { '--inbox-w': `${inbox.width + 12}px` } : null"
    data-testid="quick-fab"
  >
    <CapturePop />
    <transition name="fab-menu">
      <div v-if="quick.fabOpen" class="fab-menu" role="menu">
        <div class="fab-head">
          <span class="fab-head-t">{{ t('shell.quick.menuTitle') }}</span>
          <span class="fab-head-k">{{ t('shell.quick.menuThen') }}</span>
        </div>
        <button
          v-for="it in ITEMS"
          :key="it.id"
          type="button"
          class="fab-item"
          role="menuitem"
          :data-item="it.id"
          @click="it.run"
        >
          <n-icon :component="it.icon" :size="16" class="fab-item-ic" />
          <span class="fab-item-l">{{
            it.id === 'note' ? t('inbox.panel.add') : t(`shell.keys.create.${it.id}`)
          }}</span>
          <ShortcutKeys v-if="!isMobile" :id="SECOND[it.id]" />
        </button>
        <div class="fab-sp" />
        <button type="button" class="fab-item" role="menuitem" @click="openPalette">
          <n-icon :component="SearchOutline" :size="16" class="fab-item-ic" />
          <span class="fab-item-l">{{ t('shell.quick.allCommands') }}</span>
          <ShortcutKeys v-if="!isMobile" id="nav.palette" joined />
        </button>
      </div>
    </transition>
    <div class="fab-row">
      <transition name="fab-hint">
        <div v-if="hint && !quick.fabOpen && !capturing && !isMobile" class="fab-hint">
          {{ t('shell.keys.board.quick') }}
          <ShortcutKeys id="board.quick" />
        </div>
      </transition>
      <button
        v-if="buttonVisible || quick.fabOpen || capturing"
        type="button"
        class="fab"
        :class="{ open: quick.fabOpen || capturing }"
        data-testid="quick-fab-button"
        :aria-label="t('shell.keys.board.quick')"
        :aria-expanded="quick.fabOpen || capturing"
        @mouseenter="hint = true"
        @mouseleave="hint = false"
        @click="onFab"
      >
        <n-icon
          :component="quick.fabOpen || capturing ? CloseOutline : AddOutline"
          :size="quick.fabOpen || capturing ? 24 : 26"
        />
      </button>
    </div>
  </div>
</template>

<style scoped>
.fab-wrap {
  position: absolute;
  right: 20px;
  bottom: 20px;
  /* Above a card that lifts itself to 50 (a card with a subtask tray), so the
     menu and the «Входящие» capture never slide under one (#2996). */
  z-index: 60;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 10px;
  pointer-events: none;
}
.fab-wrap > * {
  pointer-events: auto;
}
/* The board's «Вид доски» panel owns the right edge while open (#2986). */
.fab-wrap.docked {
  right: 376px;
}
/* So does the «Входящие» panel at the layout's edge (#2997). */
.fab-wrap.inboxed {
  right: calc(20px + var(--inbox-w, 400px));
}
.fab-wrap.inboxed.docked {
  right: calc(376px + var(--inbox-w, 400px));
}
/* A phone: 16px off the edge and off the bottom navigation (#2976). */
.fab-wrap.mobile {
  right: 16px;
  bottom: calc(16px + var(--mnav-h, 0px));
}
.fab-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.fab {
  width: var(--fab, 52px);
  height: var(--fab, 52px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: 16px;
  background: var(--t-accent-grad);
  box-shadow:
    0 10px 24px color-mix(in srgb, var(--t-primary) 38%, transparent),
    var(--t-sh-pop);
  color: #fff;
  cursor: pointer;
  transition:
    transform 0.15s ease,
    background 0.15s ease;
}
.fab:hover {
  transform: translateY(-1px);
}
.fab.open {
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
}
.mobile .fab {
  --fab: 56px;
  border-radius: 18px;
}
.fab-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 30px;
  padding: 0 6px 0 10px;
  border-radius: 8px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  font-size: 12.5px;
  color: var(--t-text1);
  white-space: nowrap;
}
.fab-menu {
  width: 280px;
  padding: 6px;
  border-radius: 14px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.fab-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.fab-head-t {
  flex: 1;
}
.fab-head-k {
  text-transform: none;
  letter-spacing: 0;
  font-weight: 500;
}
.fab-item {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 38px;
  padding: 0 8px 0 10px;
  border: none;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
}
.fab-item:hover,
.fab-item:focus-visible {
  background: var(--t-hover);
  outline: none;
}
.fab-item-ic {
  color: var(--t-text2);
}
.fab-item-l {
  flex: 1;
}
.fab-sp {
  height: 6px;
}
.fab-menu-enter-active,
.fab-menu-leave-active,
.fab-hint-enter-active,
.fab-hint-leave-active {
  transition:
    opacity 0.14s ease,
    transform 0.14s ease;
}
.fab-menu-enter-from,
.fab-menu-leave-to {
  opacity: 0;
  transform: translateY(6px) scale(0.98);
  transform-origin: bottom right;
}
.fab-hint-enter-from,
.fab-hint-leave-to {
  opacity: 0;
  transform: translateX(4px);
}
</style>
