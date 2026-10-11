<script setup>
// The «Входящие» panel (#2997, mockups PackageB frame 2 and DockPanel): the
// user's reminders and notes of the workspace, docked at the right edge of the
// content under the top bar (kit/DockPanel, variant «sidebar» — part of the
// layout, not a floating layer). The content gives up the width; ≤1024px the
// panel lies over it; on a phone it is a full-height sheet.
//
// Keys inside: ↑↓ move · 1–4 filters · ↵ open (a note opens in place, a
// reminder tied to a task opens the task) · E complete · H snooze · T to a task
// · ⇧D to a document · L tie to a task · ⌫ delete (undo toast) · Esc close.
// A record's tag pill narrows the feed to that tag (#3040); the chip beside the
// segments shows it and takes it off.
// G I (the shell's) opens it with the focus in the list, and closes it again.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AddOutline,
  AlarmOutline,
  ArrowRedoOutline,
  CloseOutline,
  CheckmarkDoneOutline,
  CheckmarkOutline,
  CloudOfflineOutline,
  CreateOutline,
  DocumentOutline,
  DocumentTextOutline,
  EyeOutline,
  LinkOutline,
  MoonOutline,
  PricetagOutline,
  TrashOutline,
  UnlinkOutline,
} from '@vicons/ionicons5'
import DockPanel from '@/components/kit/DockPanel.vue'
import SwipeRow from '@/components/kit/SwipeRow.vue'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import InboxItem from './InboxItem.vue'
import SnoozeMenu from './SnoozeMenu.vue'
import LinkTaskPop from './LinkTaskPop.vue'
import { useInboxStore, MIN_W, MAX_W } from '@/stores/inbox'
import { useBoardViewStore } from '@/stores/boardView'
import { useAuthStore } from '@/stores/auth'
import { useResponsive } from '@/composables/useResponsive'
import { useInboxActions } from '@/composables/useInboxActions'
import { doneTodayCount, FILTERS, groupInbox, tagKey } from '@/utils/inbox'
import { CAPS, cap } from '@/utils/keyCaps'
import { isMac } from '@/utils/keys'

const { t } = useI18n()
const inbox = useInboxStore()
const board = useBoardViewStore()
const auth = useAuthStore()
const { isMobile } = useResponsive()
const act = useInboxActions()

const show = computed({
  get: () => inbox.panelOpen,
  set: (v) => (v ? inbox.open() : inbox.close()),
})

const boardId = computed(() => (board.active ? board.boardId : null))
const groups = computed(() =>
  groupInbox(
    { reminders: inbox.reminders, notes: inbox.notes },
    { now: inbox.now, filter: inbox.filter, boardId: boardId.value, tag: inbox.tagFilter },
  ),
)
// The tag the feed is narrowed to, spelled as the first record wearing it does.
const tagName = computed(() => {
  const key = inbox.tagFilter
  if (!key) return ''
  for (const r of [...inbox.reminders, ...inbox.notes])
    for (const n of r.tags || []) if (tagKey(n) === key) return n
  return key
})
function filterByTag(name) {
  const key = tagKey(name)
  inbox.tagFilter = inbox.tagFilter === key ? '' : key
}
const flat = computed(() => groups.value.flatMap((g) => g.items))
const keyOf = (r) => `${r.kind}:${r.id}`
const current = computed(() => flat.value.find((r) => keyOf(r) === inbox.selected) || null)
const doneToday = computed(() => doneTodayCount(inbox.reminders, inbox.now))
const empty = computed(() => inbox.loaded && !flat.value.length)

// Key hints show only after the keyboard moved the cursor, never after the
// pointer or on a touch screen (#2969). They go as soon as the pointer moves,
// not on the press: dropping the selected record's hint row under a pressed
// button shifted the records below it, and the click (a tag pill, the task
// chip) landed on the list instead.
const kbd = ref(false)
const listEl = ref(null)

function itemEl(key) {
  return listEl.value?.querySelector(`[data-key="${CSS.escape(key)}"]`) || null
}
function selectKey(key, viaKbd = false) {
  inbox.selected = key
  if (viaKbd) {
    kbd.value = true
    nextTick(() => itemEl(key)?.scrollIntoView({ block: 'nearest' }))
  }
}
function move(delta) {
  const list = flat.value
  if (!list.length) return
  const i = list.findIndex((r) => keyOf(r) === inbox.selected)
  const next =
    i < 0 ? (delta > 0 ? 0 : list.length - 1) : Math.max(0, Math.min(list.length - 1, i + delta))
  selectKey(keyOf(list[next]), true)
}

// G I and «Открыть» in the capture toast ask for the focus in the list.
watch(
  () => inbox.focusSeq,
  async () => {
    await nextTick()
    await nextTick()
    listEl.value?.focus({ preventScroll: true })
    if (!current.value && flat.value.length) selectKey(keyOf(flat.value[0]), true)
    else if (current.value) selectKey(inbox.selected, true)
  },
)

function open(rec) {
  selectKey(keyOf(rec))
  if (rec.kind === 'reminder' && rec.task && inbox.expanded !== keyOf(rec)) {
    act.openTask(rec)
    return
  }
  inbox.expanded = inbox.expanded === keyOf(rec) ? '' : keyOf(rec)
}
// «Теги» in the menu: the record opens in place with the focus in its tag line.
function editTags(rec) {
  selectKey(keyOf(rec))
  inbox.editFocus = 'tags'
  if (inbox.expanded === keyOf(rec)) inbox.expanded = ''
  nextTick(() => (inbox.expanded = keyOf(rec)))
}
function collapse() {
  inbox.expanded = ''
  nextTick(() => listEl.value?.focus({ preventScroll: true }))
}

// ── menus at a record ──
const snoozeFor = ref(null)
const linkFor = ref(null)
const menuFor = ref(null)
let menuAnchor = null
const anchorOf = (rec) => () => itemEl(keyOf(rec))

function openSnooze(rec) {
  if (rec.kind !== 'reminder') return
  selectKey(keyOf(rec))
  snoozeFor.value = rec
}
function openLink(rec) {
  selectKey(keyOf(rec))
  linkFor.value = rec
}
function openMenu(rec, el) {
  selectKey(keyOf(rec))
  menuAnchor = el || itemEl(keyOf(rec))
  menuFor.value = rec
}

async function removeSelected(rec) {
  const i = flat.value.findIndex((r) => r.id === rec.id)
  await act.remove(rec)
  const next = flat.value[Math.min(i, flat.value.length - 1)]
  if (next) selectKey(keyOf(next), kbd.value)
}

const isAuthor = (rec) => rec.kind === 'note' && rec.author_id === auth.user?.id
const shiftD = isMac ? '⇧D' : 'Shift D'

const menuItems = computed(() => {
  const r = menuFor.value
  if (!r) return []
  const items = [
    { key: 'open', label: t('inbox.menu.open'), icon: CreateOutline, caps: [CAPS.enter] },
  ]
  if (r.kind === 'reminder') {
    items.push(
      {
        key: 'done',
        label: t(r.done ? 'inbox.menu.undone' : 'inbox.menu.done'),
        icon: CheckmarkDoneOutline,
        caps: [cap('e')],
      },
      { key: 'snooze', label: t('inbox.menu.snooze'), icon: MoonOutline, caps: [cap('h')] },
    )
  }
  items.push({ gap: true })
  items.push({
    key: 'toTask',
    label: t('inbox.menu.toTask'),
    icon: ArrowRedoOutline,
    caps: [cap('t')],
  })
  if (r.kind === 'note')
    items.push({
      key: 'toDoc',
      label: t('inbox.menu.toDoc'),
      icon: DocumentOutline,
      caps: [shiftD],
    })
  items.push({
    key: 'link',
    label: t(r.task_id ? 'inbox.menu.relink' : 'inbox.menu.link'),
    icon: LinkOutline,
    caps: [cap('l')],
  })
  if (r.task_id) items.push({ key: 'unlink', label: t('inbox.action.unlink'), icon: UnlinkOutline })
  if (isAuthor(r))
    items.push({
      key: 'visibility',
      label: t('inbox.action.visibility'),
      icon: EyeOutline,
      toggle: r.visibility === 'workspace',
    })
  items.push({ key: 'tags', label: t('inbox.action.tags'), icon: PricetagOutline })
  items.push({ gap: true })
  items.push({
    key: 'delete',
    label: t('inbox.action.delete'),
    icon: TrashOutline,
    danger: true,
    caps: [CAPS.backspace],
  })
  return items
})

function run(action, rec) {
  if (!rec) return
  switch (action) {
    case 'open':
      return open(rec)
    case 'done':
      return act.complete(rec)
    case 'snooze':
      return openSnooze(rec)
    case 'toTask':
      return act.toTask(rec)
    case 'toDoc':
      return rec.kind === 'note' && act.toDoc(rec)
    case 'link':
      return openLink(rec)
    case 'unlink':
      return act.link(rec, null)
    case 'visibility':
      return act.toggleVisibility(rec)
    case 'tags':
      return editTags(rec)
    case 'delete':
      return removeSelected(rec)
  }
}
function onMenu(key) {
  const rec = menuFor.value
  menuFor.value = null
  run(key, rec)
}

function onListKey(e) {
  if (e.isComposing || e.target !== listEl.value) return
  const mod = e.metaKey || e.ctrlKey || e.altKey
  if (mod) return
  const r = current.value
  let handled = true
  if (e.key === 'ArrowDown') move(1)
  else if (e.key === 'ArrowUp') move(-1)
  else if (/^[1-4]$/.test(e.key)) inbox.filter = FILTERS[Number(e.key) - 1]
  else if (!r) handled = false
  else if (e.key === 'Enter') open(r)
  else if (e.code === 'KeyE' && !e.shiftKey) run('done', r)
  else if (e.code === 'KeyH' && !e.shiftKey) run('snooze', r)
  else if (e.code === 'KeyT' && !e.shiftKey) run('toTask', r)
  else if (e.code === 'KeyD' && e.shiftKey) run('toDoc', r)
  else if (e.code === 'KeyL' && !e.shiftKey) run('link', r)
  else if (e.key === 'Backspace' || e.key === 'Delete') run('delete', r)
  else if (e.key === '.' || (e.key === 'F10' && e.shiftKey)) openMenu(r)
  else handled = false
  if (handled) {
    e.preventDefault()
    e.stopPropagation()
  }
}

// ── width: dragged by the left edge, 320–520, remembered ──
function startResize(e) {
  e.preventDefault()
  const x0 = e.clientX
  const w0 = inbox.width
  const onMove = (ev) => inbox.setWidth(w0 + (x0 - ev.clientX))
  const onUp = () => {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
  }
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}

const hints = computed(() => [
  { caps: [CAPS.arrows], text: t('inbox.panel.hintSelect') },
  { caps: ['1–4'], text: t('inbox.panel.hintFilter') },
  { spacer: true },
  { id: 'nav.goInbox', text: t('inbox.panel.hintHide') },
])
const filterLabel = (f) => t(`inbox.panel.filter.${f}`)
// A phone (#2976 — package G «Входящие»): a reminder swipes right to be done,
// left to be put off (the variants in a sheet).
function swipeFor(rec) {
  return {
    left: rec.done
      ? []
      : [
          {
            key: 'done',
            label: t('shell.mobile.swipeComplete'),
            icon: CheckmarkOutline,
            tone: '#18a058',
          },
        ],
    right: [
      { key: 'snooze', label: t('shell.mobile.swipeSnooze'), icon: AlarmOutline, tone: '#f0a020' },
    ],
  }
}
// The count next to the title; on a phone, under the large title, it says
// what the count is made of (#2976 — package G «Входящие»).
const subtitle = computed(() => {
  if (!isMobile.value) return inbox.total ? String(inbox.total) : ''
  const parts = []
  if (inbox.fired) parts.push(t('inbox.panel.firedN', { n: inbox.fired }))
  parts.push(t('inbox.panel.notesN', inbox.notes.length, { named: { n: inbox.notes.length } }))
  return parts.join(' · ')
})
</script>

<template>
  <div
    class="inbox-dock"
    :class="{ open: show && !isMobile }"
    :style="show && !isMobile ? { '--inbox-w': `${inbox.width}px` } : null"
    @pointermove.capture="kbd = false"
    @pointerdown.capture="kbd = false"
  >
    <span
      v-if="show && !isMobile"
      class="inbox-grip"
      role="separator"
      aria-orientation="vertical"
      :aria-label="t('inbox.panel.resize')"
      :aria-valuemin="MIN_W"
      :aria-valuemax="MAX_W"
      :aria-valuenow="inbox.width"
      @pointerdown="startResize"
    />
    <DockPanel
      v-model:show="show"
      variant="sidebar"
      :title="t('inbox.title')"
      :subtitle="subtitle"
      :width="inbox.width"
      :hints="hints"
      full
      :page="isMobile"
      testid="inbox-panel"
    >
      <template #actions>
        <button type="button" class="ib-add" data-testid="inbox-add" @click="inbox.startCapture()">
          <n-icon :component="AddOutline" :size="15" />
          {{ t('inbox.panel.add') }}
          <ShortcutKeys v-if="!isMobile" id="create.note" bare />
        </button>
      </template>

      <div class="ib-seg" role="tablist" data-testid="inbox-filters">
        <button
          v-for="f in FILTERS"
          :key="f"
          type="button"
          class="ib-seg-btn"
          :class="{ on: inbox.filter === f }"
          role="tab"
          :aria-selected="inbox.filter === f"
          :data-filter="f"
          :title="f === 'board' && !boardId ? t('inbox.panel.boardNone') : ''"
          tabindex="-1"
          @click="inbox.filter = f"
        >
          {{ filterLabel(f) }}
        </button>
      </div>
      <div v-if="inbox.tagFilter" class="ib-tagf" data-testid="inbox-tag-filter">
        <span class="ib-tagf-k">{{ t('inbox.panel.byTag') }}</span>
        <button
          type="button"
          class="ib-tagf-chip"
          :title="t('inbox.panel.tagClear')"
          @click="inbox.tagFilter = ''"
        >
          #{{ tagName }}
          <n-icon :component="CloseOutline" :size="12" />
        </button>
      </div>

      <!-- Captured without a connection (#3042): waiting on the device. -->
      <div v-if="inbox.pending" class="ib-pending" data-testid="inbox-pending">
        <span>{{ t('inbox.pending', { n: inbox.pending }, inbox.pending) }}</span>
        <button type="button" class="ib-pending-b" @click="inbox.flushQueue()">
          {{ t('inbox.pendingSend') }}
        </button>
      </div>
      <div ref="listEl" class="ib-list" tabindex="0" data-testid="inbox-list" @keydown="onListKey">
        <template v-if="!inbox.loaded && inbox.loading">
          <div v-for="i in 5" :key="i" class="ib-skel">
            <span class="ib-skel-ic" />
            <span class="ib-skel-lines"><span /><span /></span>
          </div>
        </template>
        <div v-else-if="inbox.error && !inbox.loaded" class="ib-state" data-testid="inbox-error">
          <span class="ib-state-ic bad"
            ><n-icon :component="CloudOfflineOutline" :size="24"
          /></span>
          <div class="ib-state-t">{{ t('inbox.panel.errorTitle') }}</div>
          <div class="ib-state-s">{{ t('inbox.panel.errorText') }}</div>
          <button type="button" class="ib-retry" @click="inbox.load()">
            {{ t('common.action.retry') }}
          </button>
        </div>
        <div v-else-if="empty" class="ib-state" data-testid="inbox-empty">
          <span class="ib-state-ic"><n-icon :component="CheckmarkDoneOutline" :size="24" /></span>
          <div class="ib-state-t">
            {{
              inbox.filter === 'all' && !inbox.tagFilter
                ? t('inbox.panel.emptyTitle')
                : t('inbox.panel.emptyFiltered')
            }}
          </div>
          <div class="ib-state-s">
            {{
              inbox.filter === 'board' && !boardId
                ? t('inbox.panel.boardNone')
                : t('inbox.panel.emptyText')
            }}
          </div>
          <div class="ib-state-btns">
            <button type="button" class="ib-ghost" @click="inbox.startCapture({ kind: 'note' })">
              <n-icon :component="DocumentTextOutline" :size="15" />
              {{ t('inbox.kind.note') }}
              <ShortcutKeys v-if="!isMobile" id="create.note" bare />
            </button>
            <button
              type="button"
              class="ib-ghost"
              @click="inbox.startCapture({ kind: 'reminder' })"
            >
              <n-icon :component="AlarmOutline" :size="15" />
              {{ t('inbox.kind.reminder') }}
              <ShortcutKeys v-if="!isMobile" id="create.reminder" bare />
            </button>
          </div>
          <div v-if="doneToday" class="ib-state-done">
            {{ t('inbox.panel.doneToday', { n: doneToday }) }}
          </div>
        </div>
        <template v-else>
          <section v-for="g in groups" :key="g.key" class="ib-group" :data-group="g.key">
            <div class="ib-group-h">
              <span class="ib-group-t">{{ t(`inbox.panel.group.${g.key}`) }}</span>
              <span class="ib-group-n">{{ g.items.length }}</span>
              <span class="ib-fill" />
              <button
                v-if="g.key === 'fired'"
                type="button"
                class="ib-group-act"
                data-testid="inbox-done-all"
                @click="act.completeAll(g.items)"
              >
                {{ t('inbox.panel.doneAll') }}
              </button>
            </div>
            <component
              :is="isMobile && r.kind === 'reminder' ? SwipeRow : 'div'"
              v-for="r in g.items"
              :key="keyOf(r)"
              class="ib-cell"
              v-bind="isMobile && r.kind === 'reminder' ? swipeFor(r) : {}"
              @action="(a) => run(a, r)"
            >
              <InboxItem
                :rec="r"
                :now="inbox.now"
                :selected="inbox.selected === keyOf(r)"
                :kbd="kbd && !isMobile"
                :expanded="inbox.expanded === keyOf(r)"
                :touch="isMobile"
                @select="!kbd && selectKey(keyOf(r))"
                @open="open(r)"
                @task="act.openTask(r)"
                @tag="filterByTag"
                @menu="(el) => openMenu(r, el)"
                @collapse="collapse"
                @contextmenu.prevent="openMenu(r)"
              />
            </component>
          </section>
        </template>
      </div>
    </DockPanel>

    <SnoozeMenu
      :show="!!snoozeFor"
      :anchor="snoozeFor ? anchorOf(snoozeFor) : () => null"
      :task="snoozeFor?.task || null"
      @update:show="(v) => !v && (snoozeFor = null)"
      @pick="(at) => act.snooze(snoozeFor, at)"
    />
    <LinkTaskPop
      :show="!!linkFor"
      :anchor="linkFor ? anchorOf(linkFor) : () => null"
      @update:show="(v) => !v && (linkFor = null)"
      @pick="(task) => act.link(linkFor, task)"
    />
    <ActionMenu
      :show="!!menuFor"
      :anchor="() => menuAnchor"
      placement="bottom-end"
      :items="menuItems"
      :width="290"
      testid="inbox-item-actions"
      @update:show="(v) => !v && (menuFor = null)"
      @select="onMenu"
    />
  </div>
</template>

<style scoped>
.inbox-dock {
  position: relative;
  display: flex;
  flex: none;
}
.inbox-dock.open {
  padding: 0 12px 0 0;
}
.inbox-grip {
  position: absolute;
  top: 18px;
  bottom: 18px;
  left: -5px;
  z-index: 2;
  width: 8px;
  cursor: col-resize;
}
.inbox-grip::after {
  content: '';
  position: absolute;
  top: 50%;
  left: 3px;
  width: 3px;
  height: 36px;
  margin-top: -18px;
  border-radius: 2px;
  background: var(--t-text3);
  opacity: 0;
  transition: opacity 0.15s ease;
}
.inbox-grip:hover::after {
  opacity: 0.5;
}
.ib-add {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 5px 0 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.ib-add:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.ib-seg {
  position: sticky;
  top: 0;
  z-index: 1;
  display: grid;
  grid-template-columns: repeat(4, auto);
  gap: 2px;
  margin: 0 4px 8px;
  padding: 3px;
  border-radius: 10px;
  background: var(--t-sub);
}
.ib-seg-btn {
  height: 28px;
  padding: 0 8px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.ib-seg-btn.on {
  background: var(--t-pop);
  color: var(--t-text1);
  font-weight: 650;
  box-shadow: var(--t-sh-card);
}
.ib-tagf {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: -2px 8px 8px;
  font-size: 12px;
  color: var(--t-text3);
}
.ib-tagf-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 22px;
  max-width: 220px;
  padding: 0 6px 0 9px;
  border: 0;
  border-radius: 999px;
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-text1);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
}
.ib-tagf-chip:hover {
  background: color-mix(in srgb, var(--t-primary) 24%, transparent);
}
.ib-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  outline: none;
  min-height: 200px;
}
.ib-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-bottom: 8px;
}
.ib-group-h {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 10px;
  font-size: 11.5px;
  color: var(--t-text3);
}
.ib-group-t {
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}
.ib-fill {
  flex: 1;
}
.ib-group-act {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 11.5px;
  cursor: pointer;
}
.ib-group-act:hover {
  color: var(--t-text1);
}
.ib-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 72px 20px 24px;
  text-align: center;
}
.ib-state-ic {
  width: 56px;
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 8px;
  border-radius: 16px;
  background: color-mix(in srgb, var(--t-primary) 20%, var(--t-sidebar));
  color: var(--t-primary);
}
.ib-state-ic.bad {
  background: color-mix(in srgb, #e0533d 18%, var(--t-sidebar));
  color: #e0533d;
}
.ib-state-t {
  font-size: 15px;
  font-weight: 650;
}
.ib-state-s {
  max-width: 280px;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--t-text3);
}
.ib-state-btns {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
  margin-top: 12px;
}
.ib-ghost {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 32px;
  padding: 0 5px 0 10px;
  border: 0;
  border-radius: 10px;
  background: var(--t-pill);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.ib-ghost:hover {
  background: var(--t-hover);
}
.ib-state-done {
  margin-top: 18px;
  font-size: 12px;
  color: var(--t-text3);
}
.ib-retry {
  margin-top: 10px;
  height: 30px;
  padding: 0 14px;
  border: 0;
  border-radius: 8px;
  background: var(--t-accent-grad);
  color: #fff;
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.ib-skel {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px;
}
.ib-skel-ic {
  width: 28px;
  height: 28px;
  border-radius: 9px;
  background: var(--t-pill);
}
.ib-skel-lines {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.ib-skel-lines span {
  height: 9px;
  border-radius: 5px;
  background: var(--t-pill);
}
.ib-skel-lines span + span {
  width: 60%;
}
.ib-pending {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 12px 6px;
  padding: 6px 10px;
  border-radius: 10px;
  background: color-mix(in srgb, #f0a020 14%, var(--t-pop));
  font-size: 12.5px;
  color: var(--t-text2);
}
.ib-pending span {
  flex: 1;
}
.ib-pending-b {
  border: 0;
  background: transparent;
  color: var(--t-primary);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
</style>
