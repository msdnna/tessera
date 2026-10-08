<script setup>
// One row of the «Список» view (v2, #2973 / #3020; mockups ListView, MobileD).
//
// Desktop: a 40px grid row — the done circle, the number, the title, then the
// field columns (priority, due, people, estimate, tags) at the widths the
// reader dragged them to. Phone: a 56px row — circle, priority bar, the title
// over «#number · due», the first assignee; the field columns are gone.
//
// A component of its own so a change of the selection re-renders only the rows
// it touches, not the whole list (the list mounts only the rows in view).
// Everything the row shows comes from the board through `ctx` (one object for
// all rows — maps, formatters, the estimate unit).
import { computed, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { EllipseOutline, CheckmarkCircleOutline, PersonAddOutline } from '@vicons/ionicons5'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { priorityLabel } from '@/utils/priority'
import { hueGradVert } from '@/utils/gradient'
import { formatEstimate } from '@/utils/estimation'
import { glAssigneesOf } from '@/utils/assignees'
import { tagsThatFit } from '@/utils/listView'
import UserAvatar from '../UserAvatar.vue'
import TagPill from '../TagPill.vue'
import InlineEdit from '../kit/InlineEdit.vue'
import ShortcutKeys from '../ShortcutKeys.vue'

const props = defineProps({
  task: { type: Object, required: true },
  mobile: { type: Boolean, default: false },
  // The grid columns of the field part («112px 92px …»), shared by every row.
  cols: { type: String, default: '' },
  tagsWidth: { type: Number, default: 150 },
  selected: { type: Boolean, default: false },
  cursor: { type: Boolean, default: false },
  // The key hint over the cursor row (keyboard in use, desktop only); `below`
  // when the row sits at the top of the viewport and the hint would be cut.
  hint: { type: Boolean, default: false },
  hintBelow: { type: Boolean, default: false },
  editing: { type: Boolean, default: false },
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'pick', 'toggle', 'rename', 'commit', 'cancel', 'menu', 'people'])

const { t } = useI18n()
const ctx = inject('listCtx')

const done = computed(() => !!props.task.completed_at)
const prio = computed(() => props.task.priority || 0)
const prioColor = computed(() => PRIORITY_COLORS[prio.value])
const overdue = computed(() => ctx.isOverdue(props.task))
const due = computed(() => (props.task.due_date ? ctx.fmtDue(props.task.due_date) : ''))
const people = computed(() => [
  ...(props.task.assignee_ids || []).map((uid) => ({
    key: uid,
    userId: uid,
    name: ctx.membersMap[uid]?.name || '',
  })),
  ...glAssigneesOf(props.task, ctx.gitlabMembers()).map((g, i) => ({
    key: `g${i}`,
    src: g.avatar_url || '',
    name: g.name,
    title: `${g.name} (GitLab)`,
    ext: true,
  })),
])
const estimate = computed(() =>
  props.task.estimate != null ? formatEstimate(props.task.estimate, ctx.estCfg()) : '',
)
const tags = computed(() => (props.task.tag_ids || []).map((id) => ctx.tagsMap[id]).filter(Boolean))
const shownTags = computed(() =>
  tags.value.slice(0, tagsThatFit(props.tagsWidth, tags.value.length)),
)
const draft = defineModel('draft', { type: String, default: '' })

function onClick(e) {
  if (e.shiftKey || e.metaKey || e.ctrlKey) {
    emit('pick', e)
    return
  }
  emit('pick', e)
  emit('open')
}
</script>

<template>
  <div
    class="lr"
    :class="{ mobile, done, selected, cursor, editing }"
    :style="mobile ? null : { gridTemplateColumns: `28px 52px minmax(0, 1fr) ${cols}` }"
    :data-task-id="task.id"
    data-testid="list-row"
    @click="onClick"
    @dblclick.stop="!readonly && !mobile && $emit('rename')"
    @contextmenu.prevent.stop="$emit('menu', $event)"
  >
    <button
      type="button"
      class="lr-done"
      :disabled="readonly"
      :aria-pressed="done"
      :title="done ? t('board.list.reopen') : t('board.list.complete')"
      data-testid="list-row-done"
      @click.stop="$emit('toggle')"
    >
      <n-icon
        :component="done ? CheckmarkCircleOutline : EllipseOutline"
        :size="mobile ? 19 : 17"
      />
    </button>

    <template v-if="mobile">
      <span class="lr-bar" :style="{ background: hueGradVert(prioColor) }" />
      <span class="lr-main">
        <span class="lr-title">{{ task.title }}</span>
        <span class="lr-sub">
          <span v-if="task.number" class="lr-num">#{{ task.number }}</span>
          <span v-if="due" class="lr-due" :class="{ overdue }">{{ due }}</span>
        </span>
      </span>
      <span class="lr-people">
        <UserAvatar
          v-if="people[0]"
          class="lv-ava m"
          :class="{ ext: people[0].ext }"
          :user-id="people[0].userId || ''"
          :src="people[0].src || ''"
          :name="people[0].name"
        />
      </span>
    </template>

    <template v-else>
      <span class="lr-num">{{ task.number ? `#${task.number}` : '' }}</span>
      <InlineEdit
        v-if="editing"
        v-model="draft"
        class="lr-edit"
        variant="row"
        :show-hint="false"
        :placeholder="t('board.column.taskPlaceholder')"
        data-testid="list-row-rename"
        @commit="(v) => $emit('commit', v)"
        @cancel="$emit('cancel')"
        @click.stop
      />
      <span v-else class="lr-title" :title="task.title">{{ task.title }}</span>

      <span class="lr-prio">
        <template v-if="prio">
          <span class="lr-pbar" :style="{ background: hueGradVert(prioColor) }" />
          {{ priorityLabel(prio) }}
        </template>
        <span v-else class="lr-none">—</span>
      </span>
      <span class="lr-due" :class="{ overdue, none: !due }">{{ due || '—' }}</span>
      <span class="lr-people">
        <template v-if="people.length">
          <UserAvatar
            v-for="p in people.slice(0, 3)"
            :key="p.key"
            class="lv-ava"
            :class="{ ext: p.ext }"
            :user-id="p.userId || ''"
            :src="p.src || ''"
            :name="p.name"
            :title="p.title || p.name"
          />
          <span v-if="people.length > 3" class="lr-more">+{{ people.length - 3 }}</span>
        </template>
        <button
          v-else-if="!readonly"
          type="button"
          class="lr-addp"
          :title="t('board.list.assign')"
          @click.stop="$emit('people')"
        >
          <n-icon :component="PersonAddOutline" :size="14" />
        </button>
      </span>
      <span class="lr-est" :class="{ none: !estimate }">{{ estimate || '—' }}</span>
      <span class="lr-tags">
        <TagPill
          v-for="tag in shownTags"
          :key="tag.id"
          class="lr-tag"
          :tag="tag"
          :prefix-names="ctx.prefixNames"
        />
        <span v-if="tags.length > shownTags.length" class="lr-more"
          >+{{ tags.length - shownTags.length }}</span
        >
        <span v-if="!tags.length" class="lr-none">—</span>
      </span>
    </template>

    <span v-if="hint" class="lr-hint" :class="{ below: hintBelow }" data-testid="list-row-hint">
      <ShortcutKeys id="card.open" /> {{ t('board.list.hintOpen') }}
      <template v-if="!readonly">
        <ShortcutKeys id="list.rename" /> {{ t('board.list.hintRename') }}
        <ShortcutKeys id="card.complete" /> {{ t('board.list.hintComplete') }}
        <ShortcutKeys id="card.priorityPick" /> {{ t('board.list.hintPriority') }}
        <ShortcutKeys id="list.reorder" /> {{ t('board.list.hintOrder') }}
      </template>
    </span>
  </div>
</template>

<style scoped>
.lr {
  position: relative;
  isolation: isolate;
  display: grid;
  align-items: center;
  gap: 10px;
  height: 40px;
  padding: 0 10px 0 8px;
  box-sizing: border-box;
  border-radius: 9px;
  cursor: pointer;
  color: var(--t-text1);
  font-size: 14px;
}
.lr:hover {
  background: var(--t-hover);
}
/* Selected: the soft accent of a selected card; the cursor row adds the ring. */
.lr.selected {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 10%, var(--t-bg)),
    color-mix(in srgb, var(--t-primary) 18%, var(--t-bg))
  );
}
.lr.cursor {
  box-shadow: 0 0 0 2px var(--t-primary);
  z-index: 1;
}
.lr.editing {
  cursor: default;
}
.lr-done {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.lr-done:hover:not(:disabled) {
  color: var(--t-primary);
}
.lr-done:disabled {
  cursor: default;
}
.lr.done .lr-done {
  color: #18a058;
}
.lr-num {
  font-size: 12.5px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.lr-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.lr.done .lr-title {
  color: var(--t-text3);
  text-decoration: line-through;
}
.lr-edit {
  min-width: 0;
  margin-left: -8px;
}
.lr-prio {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  color: var(--t-text2);
  white-space: nowrap;
  overflow: hidden;
}
.lr-pbar {
  flex: 0 0 3px;
  width: 3px;
  height: 14px;
  border-radius: 2px;
}
.lr-due,
.lr-est {
  font-size: 12.5px;
  color: var(--t-text2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.lr-est {
  text-align: right;
}
.lr-due.none,
.lr-est.none,
.lr-none {
  color: var(--t-placeholder);
  font-size: 12.5px;
}
.lr-due.overdue {
  font-weight: 600;
  background-image: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 86%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 86%, #fff)
  );
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  color: transparent;
}
.lr-people {
  display: flex;
  align-items: center;
  padding-left: 6px;
  min-width: 0;
}
.lv-ava {
  flex: none;
  width: 20px;
  height: 20px;
  margin-left: -6px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 8.5px;
  font-weight: 700;
  box-shadow: 0 0 0 2px var(--t-bg);
}
/* External (GitLab) assignee: neutral fill, not the accent gradient — the same
   distinction the card's avatar row makes. */
.lv-ava.ext {
  background: var(--t-text3);
}
.lr-addp {
  display: flex;
  padding: 2px;
  margin-left: -6px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text3);
  opacity: 0.7;
  cursor: pointer;
}
.lr-addp:hover {
  opacity: 1;
  color: var(--t-primary);
}
.lr-more {
  margin-left: 4px;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
}
.lr-tags {
  display: flex;
  align-items: center;
  gap: 4px;
  height: 22px;
  min-width: 0;
  overflow: hidden;
}
/* TagPill leaves the box to the call site: a 20px capsule, as on the card. */
.lr-tag {
  flex: 0 1 auto;
  min-width: 0;
  max-width: 100%;
  height: 20px;
  box-sizing: border-box;
  padding: 0 8px;
  border-radius: var(--t-r-pill);
  font-size: 12px;
  line-height: 18px;
}

/* The key hint over the cursor row (mockup ListView). */
.lr-hint {
  position: absolute;
  right: 10px;
  top: -32px;
  z-index: 3;
  display: flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 8px;
  border-radius: 8px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  font-size: 11.5px;
  color: var(--t-text2);
  white-space: nowrap;
  pointer-events: none;
}
.lr-hint.below {
  top: auto;
  bottom: -32px;
}

/* ── phone: 56px, the title over «#number · due», no field columns ── */
.lr.mobile {
  display: flex;
  gap: 10px;
  height: 56px;
  padding: 0 10px 0 6px;
  border-radius: 10px;
}
.lr.mobile:hover {
  background: transparent;
}
.lr.mobile .lr-done {
  flex: none;
}
.lr-bar {
  flex: 0 0 3px;
  width: 3px;
  height: 22px;
  border-radius: 2px;
}
.lr-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.lr.mobile .lr-title {
  font-size: 15px;
}
.lr-sub {
  display: flex;
  gap: 8px;
  font-size: 12.5px;
}
.lr.mobile .lr-people {
  flex: none;
  padding: 0;
}
.lv-ava.m {
  width: 26px;
  height: 26px;
  margin: 0;
  font-size: 10px;
  box-shadow: none;
}
</style>
