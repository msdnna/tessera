<script setup>
// The capture popover of «Входящие» (#2996, mockup CapturePop): one line read
// as you type — a date or a time makes it a reminder, anything else a note;
// «/123» or «/слово» ties it to a task. It opens at the FAB (tier 1 of the
// interaction model: no scrim, focus kept inside) from the FAB menu, «N, M»,
// «N, R» and the palette; on a phone it is a bottom sheet.
//
// Keys: ↵ save · ⇧↵ open the body (Markdown) · ⌘↵ save from the body · Tab
// switch the type (a manual choice sticks: «вручную ×») · ↑ in the empty field
// brings the last record back · Esc close.
//
// «#тег» and «@человек» (#3040) become chips and are saved with the record: a
// tag the named task's project has takes its colour (the record wears the
// project's tag), any other is a free label; a handle that names one member
// is a mention, one that names nobody stays in the title. The people the body
// names (its @-chips) are mentioned too.
import { computed, nextTick, onBeforeUnmount, ref, watch, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  AlarmOutline,
  AlertCircleOutline,
  CloseOutline,
  CreateOutline,
  DocumentTextOutline,
  LockClosedOutline,
} from '@vicons/ionicons5'
import { workspaces as wsApi } from '@/api'
import Kbd from '@/components/Kbd.vue'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import TesseraIcon from '@/components/TesseraIcon.vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import TagPill from '@/components/TagPill.vue'
import BottomSheet from '@/components/kit/BottomSheet.vue'
import { taskStatus } from '@/components/palette/rows'
import { useInboxStore } from '@/stores/inbox'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useResponsive } from '@/composables/useResponsive'
import { useInboxFormat } from '@/composables/useInboxFormat'
import { pushBlocker } from '@/shortcuts/dispatcher'
import { parseCapture } from '@/utils/quickParse'
import { CAPS } from '@/utils/keyCaps'
import { humanizeError } from '@/utils/errors'
import { mentionIdsIn } from '@/utils/mentions'

const { t } = useI18n()
const message = useMessage()
const inbox = useInboxStore()
const ws = useWorkspacesStore()
const { isMobile } = useResponsive()
const { whenLong } = useInboxFormat()

// The FAB's popover and the copy inside the task modal (#2998: the modal traps
// the focus, so its «Заметка ⇧M» captures in place) share the store's capture;
// each shows only the captures meant for it.
const props = defineProps({
  inline: { type: Boolean, default: false },
})
const show = computed(() => !!inbox.capture && !!inbox.capture.inline === props.inline)
const text = ref('')
const body = ref('')
const bodyOpen = ref(false)
// null — the type follows the text; 'note' | 'reminder' — chosen by hand.
const manual = ref(null)
const saving = ref(false)
const failed = ref('')
const inputEl = ref(null)
const bodyWrap = ref(null)
const now = ref(new Date())

let release = null
watch(
  () => inbox.capture?.seq,
  async () => {
    const c = inbox.capture
    if (!c || !show.value) {
      release?.()
      release = null
      return
    }
    release ??= pushBlocker()
    text.value = c.text || ''
    body.value = ''
    bodyOpen.value = false
    manual.value = c.kind || null
    failed.value = ''
    now.value = new Date()
    await nextTick()
    await nextTick()
    const el = inputEl.value
    el?.focus()
    el?.setSelectionRange?.(text.value.length, text.value.length)
  },
)
onBeforeUnmount(() => release?.())

const taskHit = ref(null)
const taskMissing = ref('')
const taskBusy = ref(false)
// The members a handle may name; the named task's project tags colour a tag.
const people = computed(() => inbox.members.map((m) => ({ ...m, id: m.user_id })))
const projectTags = computed(
  () => (taskHit.value?.project_id && inbox.tagsOf.get(taskHit.value.project_id)) || [],
)
const parsed = computed(() =>
  parseCapture(text.value, { now: now.value, people: people.value, tags: projectTags.value }),
)
const kind = computed(() => manual.value || parsed.value.kind)
// The next whole hour — a hand-picked reminder without a time in the text.
function nextHour() {
  const d = new Date(now.value.getTime() + 3600000)
  d.setMinutes(0, 0, 0)
  return d.toISOString()
}
const remindAt = computed(() =>
  kind.value === 'reminder' ? parsed.value.remindAt || nextHour() : null,
)
// A date typed into a note stays visible, struck through: it is not applied.
const dateStruck = computed(() => kind.value === 'note' && !!parsed.value.remindAt)
const title = computed(() => parsed.value.title)

// ── the task named by «/…» ──
let taskSeq = 0
let taskTimer = 0
watch(
  () => JSON.stringify(parsed.value.taskRef),
  () => {
    const ref_ = parsed.value.taskRef
    clearTimeout(taskTimer)
    taskHit.value = null
    taskMissing.value = ''
    if (!ref_ || !ws.currentId) {
      taskBusy.value = false
      return
    }
    taskBusy.value = true
    const mine = ++taskSeq
    taskTimer = setTimeout(async () => {
      const q = ref_.number != null ? String(ref_.number) : ref_.word
      try {
        const { data } = await wsApi.search(ws.currentId, q)
        if (mine !== taskSeq) return
        const list = data?.tasks || []
        const hit =
          ref_.number != null ? list.find((x) => x.number === ref_.number) : list[0] || null
        taskHit.value = hit || null
        taskMissing.value = hit ? '' : ref_.number != null ? `/${ref_.number}` : `/${ref_.word}`
      } catch {
        if (mine === taskSeq) taskMissing.value = `/${q}`
      } finally {
        if (mine === taskSeq) taskBusy.value = false
      }
    }, 220)
  },
)

const canSave = computed(
  () =>
    !saving.value && !taskBusy.value && !taskMissing.value && !!(title.value || body.value.trim()),
)

function close() {
  inbox.endCapture()
}

function toggleKind() {
  manual.value = kind.value === 'note' ? 'reminder' : 'note'
}
function setKind(k) {
  manual.value = k
}

async function openBody() {
  bodyOpen.value = true
  await nextTick()
  bodyWrap.value?.querySelector('textarea')?.focus()
}

async function save() {
  if (!canSave.value) return
  saving.value = true
  failed.value = ''
  const line = text.value
  const heading = title.value || body.value.trim().split('\n')[0].slice(0, 120)
  try {
    const taskId = taskHit.value?.id || undefined
    const tags = parsed.value.tags.map((tg) => tg.tag?.name || tg.name)
    const mentions = [
      ...new Set([
        ...parsed.value.people.map((p) => p.user_id),
        ...mentionIdsIn(body.value, inbox.mentionItems),
      ]),
    ]
    let res
    if (kind.value === 'reminder') {
      res = await inbox.createReminder({
        message: heading,
        body: body.value,
        remind_at: remindAt.value,
        task_id: taskId,
        tags,
        mentions,
      })
    } else {
      res = await inbox.createNote({
        title: heading,
        body: body.value,
        task_id: taskId,
        visibility: 'private',
        tags,
        mentions,
      })
    }
    inbox.lastCaptured = line
    close()
    // No connection (#3042): kept on the device, sent when it is back.
    if (res?.queued) {
      message.info(t('inbox.capture.queued'))
      return
    }
    message.success(() =>
      h('span', { class: 'cap-toast' }, [
        t('inbox.capture.saved'),
        ' ',
        h(
          'button',
          {
            type: 'button',
            class: 'cap-toast-open',
            onClick: () => inbox.open({ focus: true }),
          },
          t('inbox.capture.openPanel'),
        ),
      ]),
    )
  } catch (e) {
    failed.value = humanizeError(e)
  } finally {
    saving.value = false
  }
}

function onLineKey(e) {
  if (e.isComposing) return
  if (e.key === 'Enter' && e.shiftKey) {
    e.preventDefault()
    openBody()
  } else if (e.key === 'Enter') {
    e.preventDefault()
    save()
  } else if (e.key === 'Tab' && !e.shiftKey) {
    e.preventDefault()
    toggleKind()
  } else if (e.key === 'ArrowUp' && !text.value && inbox.lastCaptured) {
    e.preventDefault()
    text.value = inbox.lastCaptured
  }
}

function onPanelKey(e) {
  if (e.isComposing) return
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    close()
  } else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
    e.preventDefault()
    save()
  }
}

// A click outside closes an empty capture; one with text keeps it open — the
// line is not thrown away by a stray click on the board.
const panel = ref(null)
function onDocDown(e) {
  if (panel.value?.contains(e.target)) return
  const el = e.target instanceof Element ? e.target : null
  if (el?.closest('[data-testid="quick-fab-button"], .n-popover, .v-binder-follower-container'))
    return
  if (!text.value.trim() && !body.value.trim()) close()
}
watch(
  () => show.value && !isMobile.value && !props.inline,
  (on) => {
    if (on) document.addEventListener('pointerdown', onDocDown, true)
    else document.removeEventListener('pointerdown', onDocDown, true)
  },
)
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocDown, true))

const status = computed(() => (taskHit.value ? taskStatus(taskHit.value) : null))
const AT = '@'
// A free label (no project tag of that name) — the mockup's neutral tag hue.
const FREE_TAG = '#9aa0aa'
const sheetShow = computed({
  get: () => show.value && isMobile.value,
  set: (v) => {
    if (!v) close()
  },
})
</script>

<template>
  <template v-if="show">
    <component
      :is="isMobile && !inline ? BottomSheet : 'div'"
      v-bind="isMobile && !inline ? { show: sheetShow, testid: 'capture' } : {}"
      @update:show="(v) => (v ? null : close())"
    >
      <div
        ref="panel"
        class="cap"
        :class="{ sheet: isMobile && !inline, inline }"
        role="dialog"
        :aria-label="t('inbox.capture.title')"
        :data-testid="isMobile && !inline ? 'capture-body' : 'capture'"
        @keydown="onPanelKey"
      >
        <div class="cap-top">
          <div class="cap-seg" role="tablist">
            <button
              type="button"
              class="cap-seg-btn"
              :class="{ on: kind === 'note' }"
              role="tab"
              :aria-selected="kind === 'note'"
              data-kind="note"
              tabindex="-1"
              @click="setKind('note')"
            >
              <n-icon :component="DocumentTextOutline" :size="14" />
              {{ t('inbox.kind.note') }}
            </button>
            <button
              type="button"
              class="cap-seg-btn"
              :class="{ on: kind === 'reminder' }"
              role="tab"
              :aria-selected="kind === 'reminder'"
              data-kind="reminder"
              tabindex="-1"
              @click="setKind('reminder')"
            >
              <n-icon :component="AlarmOutline" :size="14" class="cap-alarm" />
              {{ t('inbox.kind.reminder') }}
            </button>
          </div>
          <button
            v-if="manual"
            type="button"
            class="cap-mode manual"
            :title="t('inbox.capture.byTextHint')"
            @click="manual = null"
          >
            {{ t('inbox.capture.manual') }}
            <n-icon :component="CloseOutline" :size="12" />
          </button>
          <span v-else class="cap-mode">{{ t('inbox.capture.byText') }}</span>
          <span class="cap-fill" />
          <span v-if="!isMobile" class="cap-dest">
            <n-icon :component="LockClosedOutline" :size="11" />
            {{ t('inbox.capture.dest') }}
          </span>
        </div>

        <input
          ref="inputEl"
          v-model="text"
          class="cap-line"
          :class="{ error: !!taskMissing }"
          :placeholder="t('inbox.capture.placeholder')"
          :disabled="saving"
          data-testid="capture-input"
          autocomplete="off"
          spellcheck="false"
          @keydown="onLineKey"
        />

        <div v-if="!text.trim()" class="cap-help">
          <p>{{ t('inbox.capture.helpReminder') }}</p>
          <p>{{ t('inbox.capture.helpNote') }}</p>
        </div>
        <template v-else>
          <div class="cap-chips" data-testid="capture-chips">
            <span
              v-if="kind === 'reminder' || dateStruck"
              class="cap-chip when"
              :class="{ struck: dateStruck }"
              data-testid="capture-when"
            >
              <n-icon :component="AlarmOutline" :size="14" />
              <span>{{ whenLong(dateStruck ? parsed.remindAt : remindAt, now) }}</span>
            </span>
            <TagPill
              v-for="tg in parsed.tags"
              :key="`t-${tg.name}`"
              :tag="tg.tag || { name: tg.name, color: FREE_TAG }"
              class="cap-chip tag"
              data-testid="capture-tag"
              :title="tg.tag ? '' : t('inbox.capture.freeTag')"
            />
            <span
              v-for="p in parsed.people"
              :key="`m-${p.user_id}`"
              class="cap-chip who"
              data-testid="capture-mention"
              :title="p.email"
              ><span class="accent-grad-text">{{ AT }}{{ p.name || p.email }}</span></span
            >
          </div>
          <div v-if="parsed.taskRef" class="cap-chips">
            <span v-if="taskBusy" class="cap-chip task busy">
              <TesseraSpinner :size="11" />
            </span>
            <span v-else-if="taskMissing" class="cap-chip task missing" data-testid="capture-task">
              <n-icon :component="AlertCircleOutline" :size="14" />
              {{ t('inbox.capture.noTask', { ref: taskMissing }) }}
            </span>
            <span v-else-if="taskHit" class="cap-chip task" data-testid="capture-task">
              <TesseraIcon
                v-if="status"
                :name="status.name"
                :size="13"
                :style="{ color: status.color }"
              />
              <span class="cap-num">#{{ taskHit.number }}</span>
              <span class="cap-task-t">{{ taskHit.title }}</span>
            </span>
          </div>
          <div class="cap-title">
            <span class="cap-title-k">{{ t('inbox.capture.titleLabel') }}</span>
            <span class="cap-title-v" data-testid="capture-title">{{
              title || t('inbox.capture.noTitle')
            }}</span>
          </div>
        </template>

        <div v-if="bodyOpen" ref="bodyWrap" class="cap-body">
          <MarkdownEditor
            v-model="body"
            variant="boxed"
            :min-rows="4"
            :mention-items="inbox.mentionItems"
            :placeholder="t('inbox.capture.bodyPlaceholder')"
          />
        </div>
        <button v-else-if="isMobile && !inline" type="button" class="cap-addbody" @click="openBody">
          <n-icon :component="CreateOutline" :size="17" />
          {{ t('inbox.capture.addBody') }}
        </button>

        <div v-if="failed" class="cap-failed" data-testid="capture-failed">
          <n-icon :component="AlertCircleOutline" :size="14" />
          <span class="cap-failed-t">{{ failed }}</span>
        </div>

        <div v-if="!isMobile || inline" class="cap-foot">
          <span v-if="bodyOpen" class="cap-hint">
            <Kbd>{{ CAPS.modEnter }}</Kbd
            >{{ t('inbox.capture.fromBody') }}
          </span>
          <span v-else class="cap-hint">
            <Kbd>{{ CAPS.shiftEnter }}</Kbd
            >{{ t('inbox.capture.bodyHint') }}
          </span>
          <span class="cap-hint"
            ><Kbd>{{ CAPS.tab }}</Kbd
            >{{ t('inbox.capture.kindHint') }}</span
          >
          <span class="cap-fill" />
          <button type="button" class="cap-cancel" @click="close">
            {{ t('common.form.cancel') }}
            <Kbd>{{ CAPS.escape }}</Kbd>
          </button>
          <button
            type="button"
            class="cap-save"
            :class="{ disabled: !canSave && !saving, busy: saving }"
            :disabled="!canSave"
            data-testid="capture-save"
            @click="save"
          >
            <template v-if="saving">
              <TesseraSpinner :size="12" variant="white" />
              {{ t('inbox.capture.saving') }}
            </template>
            <template v-else>
              {{ t('common.action.save') }}
              <Kbd class="cap-cap">{{ CAPS.enter }}</Kbd>
            </template>
          </button>
        </div>
      </div>
      <template v-if="isMobile && !inline" #actions>
        <button type="button" class="bs-btn" @click="close">{{ t('common.form.cancel') }}</button>
        <button
          type="button"
          class="bs-btn primary"
          :disabled="!canSave"
          data-testid="capture-save"
          @click="save"
        >
          {{ saving ? t('inbox.capture.saving') : t('common.action.save') }}
        </button>
      </template>
    </component>
  </template>
</template>

<style scoped>
.cap {
  width: 460px;
  max-width: calc(100vw - 24px);
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 12px 0;
  border-radius: var(--t-r-pop);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
  --t-surface: var(--t-pop);
  overflow: hidden;
}
.cap.inline {
  width: auto;
  max-width: none;
  background: var(--t-sub);
  box-shadow: none;
  --t-surface: var(--t-sub);
}
.cap.inline .cap-foot {
  background: color-mix(in srgb, var(--t-sub) 70%, var(--t-bg));
}
.cap.sheet {
  width: auto;
  max-width: none;
  padding: 0;
  background: transparent;
  box-shadow: none;
  overflow: visible;
}
.cap-top {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.cap-seg {
  display: inline-flex;
  gap: 2px;
  padding: 3px;
  border-radius: 10px;
  background: var(--t-sub);
}
.cap-seg-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.cap-seg-btn.on {
  background: var(--t-pop);
  color: var(--t-text1);
  font-weight: 650;
  box-shadow: var(--t-sh-card);
}
.cap-seg-btn.on .cap-alarm {
  color: var(--t-primary);
}
.cap-mode {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.cap-mode.manual {
  height: 22px;
  padding: 0 6px 0 8px;
  border: 0;
  border-radius: 999px;
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.cap-fill {
  flex: 1;
}
.cap-dest {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.cap-line {
  height: 46px;
  padding: 0 14px;
  box-sizing: border-box;
  border: 0;
  outline: 0;
  border-radius: 12px;
  background: var(--t-input-bg);
  box-shadow: 0 0 0 2px var(--t-primary);
  color: var(--t-text1);
  font: inherit;
  font-size: 15px;
}
.cap-line.error {
  box-shadow: 0 0 0 2px #e0533d;
}
.cap-line::placeholder {
  color: var(--t-placeholder);
}
.cap-help {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 0 2px;
  font-size: 12.5px;
  line-height: 1.45;
  color: var(--t-text3);
}
.cap-help p {
  margin: 0;
}
.cap-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
}
.cap-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  max-width: 100%;
  padding: 0 10px;
  box-sizing: border-box;
  border-radius: 999px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 12.5px;
  line-height: 1;
  white-space: nowrap;
}
.cap-chip.when {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-text1);
  font-weight: 650;
}
.cap-chip.when :deep(.n-icon) {
  color: var(--t-primary);
}
.cap-chip.when.struck {
  background: var(--t-pill);
  color: var(--t-text3);
  font-weight: 500;
  text-decoration: line-through;
}
.cap-chip.when.struck :deep(.n-icon) {
  color: var(--t-text3);
}
.cap-chip.who {
  font-weight: 600;
}
.cap-chip.task {
  min-width: 0;
}
.cap-chip.task.missing {
  background: color-mix(in srgb, #e0533d 16%, transparent);
  color: #e0533d;
  font-weight: 600;
}
.cap-num {
  color: var(--t-text3);
}
.cap-task-t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--t-text1);
}
.cap-title {
  display: flex;
  gap: 8px;
  align-items: baseline;
  min-width: 0;
  padding: 0 2px;
  font-size: 12.5px;
}
.cap-title-k {
  color: var(--t-text3);
}
.cap-title-v {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cap-body :deep(textarea) {
  font-size: 13.5px;
}
.cap-addbody {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 46px;
  padding: 0 14px;
  border: 0;
  border-radius: 12px;
  background: var(--t-input-bg);
  color: var(--t-text2);
  font: inherit;
  font-size: 15px;
  text-align: left;
}
.cap-failed {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  color: #e0533d;
}
.cap-failed-t {
  flex: 1;
  min-width: 0;
}
.cap-foot {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 48px;
  margin: 2px -12px 0;
  padding: 0 10px 0 14px;
  background: color-mix(in srgb, var(--t-pop) 55%, var(--t-bg));
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.cap-hint {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.cap-cancel,
.cap-save {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 30px;
  border: 0;
  border-radius: 8px;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
  flex-shrink: 0;
}
.cap-cancel {
  padding: 0 5px 0 10px;
  background: transparent;
  color: var(--t-text2);
}
.cap-cancel:hover {
  background: var(--t-hover);
}
.cap-save {
  padding: 0 5px 0 12px;
  background: var(--t-accent-grad);
  color: #fff;
  font-weight: 600;
}
.cap-save.disabled {
  background: var(--t-pill);
  color: var(--t-placeholder);
  cursor: not-allowed;
}
.cap-save.busy {
  padding: 0 12px 0 10px;
  opacity: 0.8;
  cursor: progress;
}
.cap-cap {
  background: rgba(255, 255, 255, 0.22);
  color: #fff;
  box-shadow: none;
}
.cap-save.disabled .cap-cap {
  background: var(--t-kbd);
  color: var(--t-placeholder);
}
.cap-cancel:focus-visible,
.cap-save:focus-visible,
.cap-seg-btn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.bs-btn {
  flex: 1;
  height: 46px;
  border: 0;
  border-radius: 12px;
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 15px;
  font-weight: 600;
}
.bs-btn.primary {
  flex: 2;
  background: var(--t-accent-grad);
  color: #fff;
}
.bs-btn.primary:disabled {
  background: var(--t-pill);
  color: var(--t-placeholder);
}
@media (max-width: 768px) {
  .cap-line {
    font-size: 16px;
  }
}
</style>

<style>
.cap-toast-open {
  margin-left: 6px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--t-primary);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
</style>
