<script setup>
// «Напоминания и заметки» of the task modal (#2998, mockup TaskModalB): the
// reader's own reminders on this task and the notes tied to it, between the
// subtasks and the activity. «Напомнить ⇧R» lays out the quick moments right
// here (1 in an hour · 2 tomorrow morning · 3 by the task's due · D a date in
// words); «Заметка ⇧M» captures a note in place with the task filled in (the
// modal keeps the focus, so the FAB's popover can't take it).
//
// The rows come from GET /tasks/:id/reminders and /tasks/:id/notes and are
// re-read whenever the shell's «Входящие» copy of this task's records changes
// — the socket and every action here go through that store.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  AlarmOutline,
  CafeOutline,
  CalendarClearOutline,
  CheckmarkOutline,
  DocumentTextOutline,
  FlagOutline,
  HourglassOutline,
  LockClosedOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { notes as notesApi, reminders as remApi } from '@/api'
import Kbd from '../Kbd.vue'
import ShortcutKeys from '../ShortcutKeys.vue'
import MarkdownEditor from '../MarkdownEditor.vue'
import CapturePop from '../inbox/CapturePop.vue'
import SnoozeMenu from '../inbox/SnoozeMenu.vue'
import { useInboxStore } from '@/stores/inbox'
import { useInboxActions } from '@/composables/useInboxActions'
import { useInboxFormat } from '@/composables/useInboxFormat'
import { useFormat } from '@/composables/useFormat'
import { useResponsive } from '@/composables/useResponsive'
import { firstLine, quickRemindOptions, recordState } from '@/utils/inbox'
import { parseCapture } from '@/utils/quickParse'
import { cap } from '@/utils/keyCaps'
import { humanizeError } from '@/utils/errors'

const props = defineProps({
  task: { type: Object, default: null },
  readonly: { type: Boolean, default: false },
})
const { t } = useI18n()
const message = useMessage()
const inbox = useInboxStore()
const act = useInboxActions()
const { whenShort, noteWhen } = useInboxFormat()
const { formatTime, formatDue } = useFormat()
const { isMobile } = useResponsive()

const reminders = ref([])
const notes = ref([])
// «Напомнить» lays out the quick moments (declared before the task watcher
// below resets it).
const open = ref(false)
let seq = 0

async function load() {
  const id = props.task?.id
  if (!id) return
  const mine = ++seq
  try {
    const [r, n] = await Promise.all([remApi.forTask(id), notesApi.forTask(id)])
    if (mine !== seq) return
    reminders.value = (r.data || []).filter((x) => !x.done).map((x) => ({ ...x, kind: 'reminder' }))
    notes.value = (n.data || []).map((x) => ({ ...x, kind: 'note' }))
  } catch {
    /* the section stays as it was; the modal itself reports load errors */
  }
}
watch(
  () => props.task?.id,
  () => {
    reminders.value = []
    notes.value = []
    open.value = false
    load()
  },
  { immediate: true },
)
// The shell's copy of this task's records — any change there re-reads ours.
const storeSig = computed(() => {
  const id = props.task?.id
  if (!id) return ''
  return [...inbox.reminders, ...inbox.notes]
    .filter((r) => r.task_id === id)
    .map(
      (r) =>
        `${r.id}:${r.remind_at || ''}:${r.done ? 1 : 0}:${r.updated_at || ''}:${r.title || r.message}`,
    )
    .join('|')
})
let sigTimer = 0
watch(storeSig, () => {
  clearTimeout(sigTimer)
  sigTimer = setTimeout(load, 120)
})

const rows = computed(() => [...reminders.value, ...notes.value])
const allPrivate = computed(() =>
  rows.value.every((r) => r.kind === 'reminder' || r.visibility === 'private'),
)

// ── «Напомнить»: the quick moments ──
const picking = ref(false)
const custom = ref('')
const now = ref(new Date())
const opts = computed(() => quickRemindOptions(now.value, props.task))
const OPT_ICON = { hour: HourglassOutline, morning: CafeOutline, due: FlagOutline }
const optsEl = ref(null)
const customEl = ref(null)
const customAt = computed(() => parseCapture(custom.value, { now: now.value }).remindAt)

async function startRemind() {
  if (props.readonly || !props.task) return
  now.value = new Date()
  open.value = true
  picking.value = false
  await nextTick()
  optsEl.value?.focus()
}
function startNote() {
  if (props.readonly || !props.task) return
  inbox.startCapture({ kind: 'note', text: `/${props.task.number} `, inline: true })
}
defineExpose({ startRemind, startNote })

async function remindAt(at) {
  open.value = false
  try {
    await inbox.createReminder({
      message: props.task.title,
      remind_at: new Date(at).toISOString(),
      task_id: props.task.id,
    })
    message.success(t('inbox.task.reminded', { when: formatDue(new Date(at).toISOString()) }))
    load()
  } catch (e) {
    message.error(humanizeError(e))
  }
}
async function startCustom() {
  picking.value = true
  custom.value = ''
  await nextTick()
  customEl.value?.focus()
}
function onOptsKey(e) {
  if (e.isComposing) return
  if (e.target === customEl.value) {
    if (e.key === 'Enter' && customAt.value) {
      e.preventDefault()
      remindAt(customAt.value)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      picking.value = false
      optsEl.value?.focus()
    }
    return
  }
  const n = opts.value.length
  if (/^[1-9]$/.test(e.key) && Number(e.key) <= n) {
    e.preventDefault()
    e.stopPropagation()
    remindAt(opts.value[Number(e.key) - 1].at)
  } else if (e.code === 'KeyD') {
    e.preventDefault()
    e.stopPropagation()
    startCustom()
  } else if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    open.value = false
  }
}
const optLabel = (o) =>
  o.key === 'due' ? formatDue(o.at.toISOString(), { now: now.value }) : formatTime(o.at)

// ── a row ──
const snoozeFor = ref(null)
const rowEls = {}
const editing = ref('')
const draft = ref('')
function rowClick(r) {
  if (props.readonly) return
  if (r.kind === 'reminder') {
    snoozeFor.value = r
    return
  }
  if (editing.value === r.id) return saveNote(r)
  editing.value = r.id
  draft.value = r.body || ''
}
async function saveNote(r) {
  editing.value = ''
  if (draft.value === (r.body || '')) return
  try {
    await inbox.updateNote(r, { body: draft.value })
    load()
  } catch (e) {
    message.error(humanizeError(e))
  }
}
function onEditKey(e, r) {
  if (e.key === 'Escape' || (e.key === 'Enter' && (e.metaKey || e.ctrlKey))) {
    e.preventDefault()
    e.stopPropagation()
    saveNote(r)
  }
}
const isFired = (r) => recordState(r) === 'fired'
// Every action goes through the shell's store; the re-read covers a task of
// another workspace, whose records that store doesn't hold.
async function done(r) {
  await act.complete(r)
  load()
}
async function del(r) {
  await act.remove(r)
  load()
}
async function snooze(r, at) {
  await act.snooze(r, at)
  load()
}
</script>

<template>
  <section v-if="task" class="tis" data-testid="task-inbox">
    <div class="tis-head">
      <span class="tis-title">{{ t('inbox.task.title') }}</span>
      <span v-if="rows.length" class="tis-count">{{ rows.length }}</span>
      <span v-if="rows.length && allPrivate" class="tis-priv">· {{ t('inbox.task.privacy') }}</span>
      <span class="grow" />
      <template v-if="!readonly">
        <button type="button" class="tis-btn" data-testid="task-inbox-note" @click="startNote">
          <n-icon :component="DocumentTextOutline" :size="15" />
          {{ t('inbox.task.note') }}
          <ShortcutKeys v-if="!isMobile" id="task.note" />
        </button>
        <button
          type="button"
          class="tis-btn accent"
          :class="{ on: open }"
          data-testid="task-inbox-remind"
          @click="open ? (open = false) : startRemind()"
        >
          <n-icon :component="AlarmOutline" :size="15" />
          {{ t('inbox.task.remind') }}
          <ShortcutKeys v-if="!isMobile" id="task.remind" />
        </button>
      </template>
    </div>

    <CapturePop inline />

    <div v-if="rows.length || open" class="tis-tray">
      <div
        v-if="open"
        ref="optsEl"
        class="tis-opts"
        tabindex="-1"
        data-testid="task-inbox-opts"
        @keydown="onOptsKey"
      >
        <button
          v-for="(o, i) in opts"
          :key="o.key"
          type="button"
          class="tis-opt"
          :data-key="o.key"
          @click="remindAt(o.at)"
        >
          <n-icon :component="OPT_ICON[o.key]" :size="14" />
          <span class="tis-opt-l">{{ t(`inbox.task.${o.key}`) }}</span>
          <span class="tis-opt-at">{{ optLabel(o) }}</span>
          <Kbd v-if="!isMobile">{{ cap(i + 1) }}</Kbd>
        </button>
        <button v-if="!picking" type="button" class="tis-opt ghost" @click="startCustom">
          <n-icon :component="CalendarClearOutline" :size="14" />
          <span class="tis-opt-l">{{ t('inbox.task.date') }}</span>
          <Kbd v-if="!isMobile">{{ cap('d') }}</Kbd>
        </button>
        <span v-else class="tis-custom">
          <input
            ref="customEl"
            v-model="custom"
            class="tis-input"
            :class="{ bad: custom.trim() && !customAt }"
            :placeholder="t('inbox.task.datePlaceholder')"
          />
          <span class="tis-opt-at">{{ customAt ? formatDue(customAt) : '' }}</span>
        </span>
      </div>
      <div
        v-for="r in rows"
        :key="`${r.kind}:${r.id}`"
        :ref="(el) => (rowEls[r.id] = el)"
        class="tis-row"
        :class="[r.kind, { fired: r.kind === 'reminder' && isFired(r), open: editing === r.id }]"
        data-testid="task-inbox-row"
        @click="rowClick(r)"
      >
        <span class="tis-ic">
          <n-icon :component="r.kind === 'note' ? DocumentTextOutline : AlarmOutline" :size="14" />
        </span>
        <span class="tis-t">
          <span class="tis-name">{{ r.kind === 'note' ? r.title : r.message }}</span>
          <span v-if="r.kind === 'note' && firstLine(r.body) && editing !== r.id" class="tis-sub">
            · {{ firstLine(r.body) }}
          </span>
        </span>
        <span class="tis-when" :class="{ accent: r.kind === 'reminder' && !isFired(r) }">{{
          r.kind === 'note' ? noteWhen(r) : whenShort(r.remind_at)
        }}</span>
        <n-icon
          v-if="r.kind === 'reminder' || r.visibility === 'private'"
          :component="LockClosedOutline"
          :size="12"
          class="tis-lock"
          :title="t('inbox.item.private')"
        />
        <span v-if="!readonly" class="tis-acts">
          <button
            v-if="r.kind === 'reminder'"
            type="button"
            class="tis-act"
            :title="t('inbox.menu.done')"
            @click.stop="done(r)"
          >
            <n-icon :component="CheckmarkOutline" :size="14" />
          </button>
          <button
            type="button"
            class="tis-act"
            :title="t('inbox.action.delete')"
            @click.stop="del(r)"
          >
            <n-icon :component="TrashOutline" :size="14" />
          </button>
        </span>
        <div v-if="editing === r.id" class="tis-edit" @click.stop @keydown="(e) => onEditKey(e, r)">
          <MarkdownEditor v-model="draft" variant="boxed" :min-rows="3" />
        </div>
      </div>
    </div>
    <SnoozeMenu
      :show="!!snoozeFor"
      :anchor="() => (snoozeFor ? rowEls[snoozeFor.id] : null)"
      :task="task"
      testid="task-inbox-snooze"
      @update:show="(v) => !v && (snoozeFor = null)"
      @pick="(at) => snooze(snoozeFor, at)"
    />
  </section>
</template>

<style scoped>
.tis {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.tis-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 28px;
}
.tis-title {
  font-size: 14.5px;
  font-weight: 650;
  color: var(--t-text1);
}
.tis-count,
.tis-priv {
  font-size: 13px;
  color: var(--t-text3);
  white-space: nowrap;
}
.grow {
  flex: 1;
}
.tis-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 6px 0 8px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
  white-space: nowrap;
}
.tis-btn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.tis-btn.accent {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-text1);
  font-weight: 600;
}
.tis-btn.accent :deep(.n-icon) {
  color: var(--t-primary);
}
.tis-tray {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px;
  border-radius: 12px;
  background: color-mix(in srgb, var(--t-sub) 65%, var(--t-pop));
}
.tis-opts {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 6px;
  outline: none;
}
.tis-opt {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 30px;
  padding: 0 5px 0 10px;
  border: 0;
  border-radius: 9px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.tis-opt:hover {
  background: var(--t-hover);
}
.tis-opt.ghost {
  background: transparent;
  box-shadow: none;
  color: var(--t-text2);
}
.tis-opt-at {
  font-size: 12.5px;
  color: var(--t-text3);
}
.tis-custom {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.tis-input {
  height: 30px;
  width: 190px;
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
.tis-input.bad {
  box-shadow: 0 0 0 2px #e0533d;
}
.tis-row {
  position: relative;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  min-height: 38px;
  padding: 0 10px 0 8px;
  border-radius: 9px;
  cursor: pointer;
}
.tis-row:hover {
  background: var(--t-hover);
}
.tis-ic {
  flex: none;
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 7px;
  background: var(--t-pill);
  color: var(--t-text2);
}
.tis-row.reminder .tis-ic {
  background: color-mix(in srgb, var(--t-primary) 18%, transparent);
  color: var(--t-primary);
}
.tis-row.fired .tis-ic {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 86%, #000),
    #f0a020 50%,
    color-mix(in srgb, #f0a020 86%, #fff)
  );
  color: #fff;
}
.tis-t {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13.5px;
}
.tis-name {
  color: var(--t-text1);
}
.tis-sub {
  color: var(--t-text3);
  font-size: 12.5px;
}
.tis-when {
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.tis-when.accent {
  background: var(--t-accent-grad);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  font-weight: 650;
}
.tis-row.fired .tis-when {
  color: #f0a020;
  font-weight: 650;
}
.tis-lock {
  color: var(--t-text3);
}
.tis-acts {
  display: none;
  gap: 2px;
}
.tis-row:hover .tis-acts {
  display: inline-flex;
}
.tis-act {
  width: 24px;
  height: 24px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.tis-act:hover {
  background: var(--t-pop);
  color: var(--t-text1);
}
.tis-edit {
  flex: 1 0 100%;
  padding: 0 0 8px 32px;
  cursor: default;
}
</style>
