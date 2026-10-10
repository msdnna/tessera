<script setup>
// One record of «Входящие» (#2997, mockup InboxItem): the type icon, the title,
// the first line of the text, the task chip, the time on the right. Under the
// keyboard cursor it lists its keys; opened (↵) it edits in place — title and
// Markdown text, saved on the way out.
//
// Icon by state: note — neutral; upcoming — soft accent; fired — a solid
// orange gradient (attention, not an error: red in v2 is an overdue task);
// done — soft green; snoozed — neutral with a moon.
//
// Tags and mentions (#3040) ride in the chip row after the task: a project tag
// in its colour (TagPill), a free label neutral, a mentioned person as «@Имя»
// in the accent. A tag pill narrows the feed to that tag. Opened, the record
// edits its tags as a «#тег #тег» line; the mentions are the ones its text
// names (the same chips comments draw) plus the ones the capture line named.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AlarmOutline,
  CheckmarkOutline,
  DocumentTextOutline,
  EllipsisHorizontal,
  LockClosedOutline,
  MoonOutline,
} from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import TesseraIcon from '@/components/TesseraIcon.vue'
import TagPill from '@/components/TagPill.vue'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import { taskStatus } from '@/components/palette/rows'
import { useInboxStore } from '@/stores/inbox'
import { useInboxFormat } from '@/composables/useInboxFormat'
import { recordTitle } from '@/composables/useInboxActions'
import {
  firstLine,
  parseTagLine,
  recordPeople,
  recordState,
  recordTags,
  tagKey,
  tagLine,
} from '@/utils/inbox'
import { mentionIdsIn } from '@/utils/mentions'
import { hueGrad } from '@/utils/gradient'
import { CAPS, cap } from '@/utils/keyCaps'
import { isMac } from '@/utils/keys'

const props = defineProps({
  rec: { type: Object, required: true },
  now: { type: Date, default: () => new Date() },
  selected: { type: Boolean, default: false },
  // The key hints under a selected record — keyboard only (#2969).
  kbd: { type: Boolean, default: false },
  expanded: { type: Boolean, default: false },
  touch: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'select', 'menu', 'collapse', 'task', 'tag'])
const { t } = useI18n()
const inbox = useInboxStore()
const { whenShort, hint, noteWhen, formatTime } = useInboxFormat()

const ORANGE = '#f0a020'
const state = computed(() => recordState(props.rec, props.now))
const ICON = {
  note: DocumentTextOutline,
  upcoming: AlarmOutline,
  fired: AlarmOutline,
  done: CheckmarkOutline,
  snoozed: MoonOutline,
}
const iconStyle = computed(() => (state.value === 'fired' ? { background: hueGrad(ORANGE) } : null))
const title = computed(() => recordTitle(props.rec) || t('inbox.item.untitled'))
const line = computed(() => firstLine(props.rec.body))
const time = computed(() =>
  props.rec.kind === 'note'
    ? noteWhen(props.rec, props.now)
    : whenShort(props.rec.remind_at, props.now),
)
const sub = computed(() => (props.rec.kind === 'note' ? '' : hint(props.rec, props.now)))
const status = computed(() => (props.rec.task ? taskStatus(props.rec.task) : null))
const isPrivate = computed(() => props.rec.kind === 'note' && props.rec.visibility === 'private')
const tags = computed(() => recordTags(props.rec, inbox.tagsOf))
const people = computed(() => recordPeople(props.rec, inbox.members))
const AT = '@'
// A free label (no project tag of that name) — the mockup's neutral tag hue.
const FREE_TAG = '#9aa0aa'
const firstName = (m) => (m.name || m.email || '').split(/\s+/)[0]
const hasChips = computed(
  () =>
    !!props.rec.task ||
    (state.value === 'snoozed' && !!props.rec.snoozed_from) ||
    tags.value.length > 0 ||
    people.value.length > 0,
)
const shiftD = isMac ? '⇧D' : 'Shift D'

// ── in-place editing ──
const draftTitle = ref('')
const draftBody = ref('')
const draftTags = ref('')
const editEl = ref(null)
const tagsEl = ref(null)
// Only a record that was actually opened writes back when it closes.
let editing = false
watch(
  () => props.expanded,
  async (on) => {
    if (on) {
      editing = true
      draftTitle.value = recordTitle(props.rec)
      draftBody.value = props.rec.body || ''
      draftTags.value = tagLine(props.rec.tags || [])
      await nextTick()
      if (inbox.editFocus === 'tags') tagsEl.value?.focus()
      else editEl.value?.querySelector('textarea')?.focus()
      inbox.editFocus = ''
    } else if (editing) {
      editing = false
      save()
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  if (editing) save()
})
// The mentions after an edit: the people the new text names, plus the ones
// the record named outside its text (the capture line's «@человек»).
function mentionsAfterEdit(r) {
  const items = inbox.mentionItems
  const before = new Set(mentionIdsIn(r.body || '', items))
  const kept = (r.mentions || []).filter((id) => !before.has(id))
  return [...new Set([...kept, ...mentionIdsIn(draftBody.value, items)])]
}
const sameList = (a, b) => a.length === b.length && a.every((x, i) => x === b[i])
async function save() {
  const r = props.rec
  const heading = draftTitle.value.trim() || recordTitle(r)
  const nextTags = parseTagLine(draftTags.value)
  const tagsChanged = !sameList(nextTags.map(tagKey), (r.tags || []).map(tagKey))
  const bodyChanged = draftBody.value !== (r.body || '')
  if (heading === recordTitle(r) && !bodyChanged && !tagsChanged) return
  const extra = {}
  if (tagsChanged) extra.tags = nextTags
  if (bodyChanged) extra.mentions = mentionsAfterEdit(r)
  try {
    if (r.kind === 'note')
      await inbox.updateNote(r, { title: heading, body: draftBody.value, ...extra })
    else await inbox.updateReminder(r, { message: heading, body: draftBody.value, ...extra })
  } catch {
    /* the panel's next load shows the server's text again */
  }
}
function onEditKey(e) {
  if (e.key === 'Escape' || (e.key === 'Enter' && (e.metaKey || e.ctrlKey))) {
    e.preventDefault()
    e.stopPropagation()
    emit('collapse')
  }
}
</script>

<template>
  <div
    class="ii"
    :class="[state, { sel: selected, kbd: kbd && selected, exp: expanded, touch }]"
    :data-key="`${rec.kind}:${rec.id}`"
    :data-state="state"
    data-testid="inbox-item"
    @click="expanded ? null : emit('open')"
    @pointerenter="emit('select')"
  >
    <span class="ii-ic" :style="iconStyle">
      <n-icon :component="ICON[state]" :size="15" />
    </span>
    <div class="ii-main">
      <div class="ii-row1">
        <input
          v-if="expanded"
          v-model="draftTitle"
          class="ii-title-in"
          :aria-label="t('inbox.capture.titleLabel')"
          @keydown="onEditKey"
          @click.stop
        />
        <span v-else class="ii-title">{{ title }}</span>
        <n-icon
          v-if="isPrivate"
          :component="LockClosedOutline"
          :size="11"
          class="ii-lock"
          :title="t('inbox.item.private')"
        />
      </div>
      <div v-if="line && !expanded" class="ii-sub">{{ line }}</div>
      <div v-if="hasChips && !expanded" class="ii-chips" data-testid="inbox-item-chips">
        <button
          v-if="rec.task"
          type="button"
          class="ii-chip task"
          tabindex="-1"
          :title="rec.task.title"
          @click.stop="emit('task')"
        >
          <TesseraIcon
            v-if="status"
            :name="status.name"
            :size="12"
            :style="{ color: status.color }"
          />
          <span class="ii-num">#{{ rec.task.number }}</span>
          <span class="ii-task-t">{{ rec.task.title }}</span>
        </button>
        <span
          v-if="state === 'snoozed' && rec.snoozed_from"
          class="ii-chip since"
          :title="t('inbox.item.snoozedTip', { time: formatTime(rec.snoozed_from) })"
          >{{ t('inbox.item.since', { time: formatTime(rec.snoozed_from) }) }}</span
        >
        <TagPill
          v-for="tg in tags"
          :key="`t-${tg.name}`"
          :tag="tg.tag || { name: tg.name, color: FREE_TAG }"
          class="ii-chip tag"
          :class="{ free: !tg.tag }"
          role="button"
          data-testid="inbox-item-tag"
          :title="t('inbox.item.tagFilter', { tag: tg.name })"
          @click.stop="emit('tag', tg.name)"
        />
        <span
          v-for="m in people"
          :key="`m-${m.user_id}`"
          class="ii-chip who"
          data-testid="inbox-item-mention"
          :title="m.name || m.email"
          ><span class="accent-grad-text">{{ AT }}{{ firstName(m) }}</span></span
        >
      </div>
      <div v-if="kbd && selected && !expanded" class="ii-keys" data-testid="inbox-item-keys">
        <span class="ii-key"
          ><Kbd>{{ CAPS.enter }}</Kbd
          >{{ t('inbox.action.open') }}</span
        >
        <span v-if="rec.kind === 'reminder'" class="ii-key">
          <Kbd>{{ cap('e') }}</Kbd
          >{{ t(rec.done ? 'inbox.action.undone' : 'inbox.action.done') }}
        </span>
        <span v-if="rec.kind === 'reminder'" class="ii-key">
          <Kbd>{{ cap('h') }}</Kbd
          >{{ t('inbox.action.snooze') }}
        </span>
        <span class="ii-key"
          ><Kbd>{{ cap('t') }}</Kbd
          >{{ t('inbox.action.toTask') }}</span
        >
        <span v-if="rec.kind === 'note'" class="ii-key">
          <Kbd>{{ shiftD }}</Kbd
          >{{ t('inbox.action.toDoc') }}
        </span>
        <span class="ii-key"
          ><Kbd>{{ cap('l') }}</Kbd
          >{{ t('inbox.action.link') }}</span
        >
        <span class="ii-key"
          ><Kbd>{{ CAPS.backspace }}</Kbd></span
        >
      </div>
      <div v-if="expanded" ref="editEl" class="ii-edit" @click.stop @keydown="onEditKey">
        <input
          ref="tagsEl"
          v-model="draftTags"
          class="ii-tags-in"
          :placeholder="t('inbox.item.tagsPlaceholder')"
          :aria-label="t('inbox.item.tagsLabel')"
          data-testid="inbox-item-tags-input"
          autocomplete="off"
          spellcheck="false"
        />
        <MarkdownEditor
          v-model="draftBody"
          variant="boxed"
          :min-rows="3"
          :mention-items="inbox.mentionItems"
          :placeholder="t('inbox.capture.bodyPlaceholder')"
        />
        <div class="ii-edit-foot">
          <span class="ii-key"
            ><Kbd>{{ CAPS.escape }}</Kbd
            >{{ t('inbox.item.collapse') }}</span
          >
        </div>
      </div>
    </div>
    <div v-if="!expanded" class="ii-side">
      <span class="ii-time" :class="state">{{ time }}</span>
      <span v-if="sub" class="ii-hint">{{ sub }}</span>
    </div>
    <button
      type="button"
      class="ii-more"
      tabindex="-1"
      :aria-label="t('inbox.action.menu')"
      data-testid="inbox-item-menu"
      @click.stop="emit('menu', $event.currentTarget)"
    >
      <n-icon :component="EllipsisHorizontal" :size="16" />
    </button>
  </div>
</template>

<style scoped>
.ii {
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 9px 10px;
  border-radius: 12px;
  cursor: pointer;
  --t-surface: var(--t-sidebar);
}
.ii:hover {
  background: var(--t-hover);
}
.ii.sel.kbd,
.ii.exp {
  background: color-mix(in srgb, var(--t-primary) 16%, var(--t-sidebar));
}
.ii.exp {
  cursor: default;
}
.ii-ic {
  flex: none;
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: var(--t-pill);
  color: var(--t-text2);
}
.ii.upcoming .ii-ic {
  background: color-mix(in srgb, var(--t-primary) 18%, var(--t-sidebar));
  color: var(--t-primary);
}
.ii.fired .ii-ic {
  color: #fff;
}
.ii.done .ii-ic {
  background: color-mix(in srgb, #18a058 18%, var(--t-sidebar));
  color: #18a058;
}
.ii-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ii-row1 {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 28px;
}
.ii-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13.5px;
  font-weight: 500;
  color: var(--t-text1);
}
.ii.done .ii-title {
  color: var(--t-text3);
  text-decoration: line-through;
}
.ii-lock {
  flex: none;
  color: var(--t-text3);
}
.ii-title-in {
  flex: 1;
  min-width: 0;
  height: 28px;
  padding: 0 8px;
  border: 0;
  outline: 0;
  border-radius: 8px;
  background: var(--t-input-bg);
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  font-weight: 500;
}
.ii-title-in:focus {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.ii-sub {
  margin-top: -4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12.5px;
  color: var(--t-text3);
}
.ii-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
}
.ii-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 22px;
  min-width: 0;
  padding: 0 8px;
  box-sizing: border-box;
  border: 0;
  border-radius: 999px;
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 12px;
  white-space: nowrap;
}
.ii-chip.task {
  flex: 0 1 auto;
  cursor: pointer;
}
.ii-chip.task:hover {
  color: var(--t-text1);
}
.ii-chip.since {
  flex: none;
  color: var(--t-text3);
}
.ii-chip.tag {
  flex: none;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
}
.ii-chip.who {
  flex: none;
  font-weight: 600;
}
.ii-num {
  color: var(--t-text3);
}
.ii-task-t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ii-side {
  flex: none;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  padding-top: 6px;
  max-width: 40%;
}
.ii-time {
  font-size: 12.5px;
  font-weight: 600;
  white-space: nowrap;
  color: var(--t-text3);
}
.ii-time.upcoming {
  background: var(--t-accent-grad);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.ii-time.fired {
  color: #f0a020;
  font-weight: 650;
}
.ii-time.done,
.ii-time.snoozed,
.ii-time.note {
  font-weight: 500;
}
.ii-hint {
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.ii-keys,
.ii-edit-foot {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  padding-top: 4px;
  font-size: 11.5px;
  color: var(--t-text3);
}
.ii-key {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.ii-edit {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.ii-tags-in {
  height: 28px;
  padding: 0 8px;
  border: 0;
  outline: 0;
  border-radius: 8px;
  background: var(--t-input-bg);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
}
.ii-tags-in:focus {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.ii-tags-in::placeholder {
  color: var(--t-placeholder);
}
.ii-more {
  position: absolute;
  top: 8px;
  right: 6px;
  width: 26px;
  height: 26px;
  display: none;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 7px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-card);
  color: var(--t-text2);
  cursor: pointer;
}
.ii:hover .ii-more {
  display: inline-flex;
}
.ii.touch .ii-more {
  position: static;
  display: inline-flex;
  flex: none;
  background: transparent;
  box-shadow: none;
}
.ii.exp .ii-more {
  display: none;
}
</style>
