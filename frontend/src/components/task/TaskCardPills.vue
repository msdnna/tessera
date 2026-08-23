<script setup>
// The property row of a board card: priority · due · estimate · milestone ·
// description · tags · people, plus the pickers each of them opens.
//
// Extracted from TaskCard (#2665). Everything here is board *context* — it comes
// from the board store, not from props — so the card passes only the task it
// renders. The row decides for itself whether it has anything to show: when no
// field survives the size preset and the customize toggles it renders nothing.
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NPopover, NTooltip, NInput } from 'naive-ui'
import {
  FlagOutline,
  CalendarClearOutline,
  PersonAddOutline,
  PricetagOutline,
  CheckmarkOutline,
  RepeatOutline,
  TimerOutline,
  RibbonOutline,
  WarningOutline,
  ReorderThreeOutline,
} from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { tasks as tasksApi, projects as projectsApi } from '@/api'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { priorityLabel, priorityOptions as buildPriorityOptions } from '@/utils/priority'
import { hueGrad, tagPillBg, softFill, readableHue, onColor } from '@/utils/gradient'
import { buildTagGroups, tagParts } from '@/utils/tagGroups'
import { milestoneRange } from '@/utils/milestones'
import { useFormat } from '@/composables/useFormat'
import {
  formatEstimate,
  formatEstimateFull,
  estimateTooltip,
  sumEstimates,
} from '@/utils/estimation'
import { cardFieldVisible } from '@/utils/cardFields'
import { taskBasePatch } from '@/utils/taskPatch'
import UserAvatar from '../UserAvatar.vue'
import DueEditor from '../DueEditor.vue'
import RichContent from '../RichContent.vue'
import TagPill from '../TagPill.vue'
import { useThemeStore } from '@/stores/theme'
import { useBoardViewStore } from '@/stores/boardView'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useConflictsStore } from '@/stores/conflicts'
import { useTagFit } from '@/composables/useTagFit'

const props = defineProps({
  task: { type: Object, required: true },
  // Children of this task — only for the Σ estimate rollup shown when the task
  // itself has no estimate.
  subtasks: { type: Array, default: () => [] },
  // Archive view: pills become display-only so a click falls through to the card.
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'changed'])

const theme = useThemeStore()
const wsStore = useWorkspacesStore()
const conflictsStore = useConflictsStore()
const hasConflict = computed(() => conflictsStore.has(props.task.id))
const { formatDue, formatters } = useFormat()
const { t } = useI18n()
// Tag/label colour clamped for legibility on the active theme (used for text).
const tagText = (c) => readableHue(c, theme.isDark)

const bv = useBoardViewStore()
const {
  columns,
  metaTagPrefixes,
  tagsList: tags,
  membersList: members,
  gitlabMembersList: gitlabMembers,
  showEmpty,
  stackFields,
  cardSize,
} = storeToRefs(bv)
const tagsMap = bv.tagsMap
const membersMap = bv.membersMap
const milestonesMap = bv.milestonesMap
const tagPrefixNames = bv.prefixNames
const fieldVis = bv.fieldVis
const projectId = computed(() => bv.projectId)

// Size preset × customize toggle — the single predicate, shared with the card
// (which asks it about the meta row's number / GitLab chip).
const show = (k) => cardFieldVisible(cardSize.value, fieldVis, k)
// Whether the row has any content at all (drives whether it renders).
const hasAnyPill = computed(
  () =>
    hasConflict.value ||
    (show('priority') && (showEmpty.value || props.task.priority)) ||
    (show('due') && (showEmpty.value || due.value)) ||
    (show('estimate') && estText.value) ||
    (show('milestone') && taskMilestone.value) ||
    (show('description') && hasDescription.value) ||
    (show('tags') && (showEmpty.value || taskTags.value.length)) ||
    (show('assignee') &&
      (showEmpty.value || author.value || assignees.value.length || glAssignees.value.length)),
)

// Picker tags grouped by prefix (friendly name); a single prefix-less bucket
// renders flat without a header.
const tagPickerGroups = computed(() =>
  buildTagGroups(tags.value, tagPrefixNames, metaTagPrefixes.value),
)
const tagPickerHeaders = computed(() => tagPickerGroups.value.length > 1)
const newTagName = ref('')

const taskTags = computed(() => (props.task.tag_ids || []).map((id) => tagsMap[id]).filter(Boolean))
// The single-tag pill hands its box over to TagPill when the tag is scoped, so
// the GitLab-EE two-tone pill isn't boxed-in by the button's own soft fill.
const firstTagScoped = computed(() => {
  const t = taskTags.value[0]
  return t ? tagParts(t.name, tagPrefixNames).hasScope : false
})
// Stacked tags row: fit as many chips as the row width allows, rest → +N.
const stagValEl = ref(null)
const stagMeasureEl = ref(null)
const { visibleCount: visibleTagCount } = useTagFit(stagValEl, stagMeasureEl, taskTags, { pad: 4 })
const taskMilestone = computed(() =>
  props.task.milestone_id ? milestonesMap[props.task.milestone_id] || null : null,
)
const assignees = computed(() =>
  (props.task.assignee_ids || []).map((id) => membersMap[id]).filter(Boolean),
)
// External GitLab assignees (no Tessera account). The board query carries only
// their names/logins (no avatar), so resolve the avatar from the workspace
// GitLab-members list by username — same source the picker/filter already use.
const glAssignees = computed(() => {
  const logins = props.task.gitlab_assignee_logins || []
  const names = props.task.gitlab_assignees || []
  if (!logins.length) return names.map((n) => ({ name: n }))
  return logins.map((login) => {
    const m = gitlabMembers.value.find((x) => x.gl_username === login)
    return { login, name: m?.gl_name || login, avatar_url: m?.gl_avatar_url || null }
  })
})
// Author (read-only): GitLab issue author for synced cards, else the Tessera
// creator resolved from created_by.
const author = computed(() => {
  const t = props.task
  if (t.gitlab_author)
    return {
      name: t.gitlab_author_name || t.gitlab_author,
      login: t.gitlab_author,
      avatar: t.gitlab_author_avatar_url,
      gl: true,
    }
  if (t.created_by) {
    const m = membersMap[t.created_by]
    if (m) return { name: m.name, id: t.created_by }
  }
  return null
})
// When the author is also an assignee, don't render a separate (muted) author
// avatar — the person already shows once as the accent assignee. The tooltip
// still lists both roles.
const authorIsAssignee = computed(() => {
  const a = author.value
  if (!a) return false
  if (a.id) return (props.task.assignee_ids || []).includes(a.id)
  // GitLab author (no Tessera id): match by login/name against GitLab assignees.
  const key = (a.login || a.name || '').toLowerCase()
  return glAssignees.value.some((g) => {
    const gn = (g.name || g || '').toString().toLowerCase()
    return gn === key || (g.login || '').toLowerCase() === key
  })
})

// Combined assignee names (Tessera + GitLab) for the merged people tooltip.
const assigneeNames = computed(() =>
  [
    ...assignees.value.map((u) => u.name),
    ...glAssignees.value.map((g) => `${g.name || g} (GitLab)`),
  ].join(', '),
)

// Assignee picker: a search box + a list capped at 10, ordered assigned →
// recently-picked → alphabetical. "Recent" is a small cross-board localStorage
// MRU of user ids the user has assigned, so the people you actually use surface
// first; falls back to alphabetical when there's no history.
const RECENT_ASSIGNEES_KEY = 'tessera_recent_assignees'
function readRecentAssignees() {
  try {
    const v = JSON.parse(localStorage.getItem(RECENT_ASSIGNEES_KEY) || '[]')
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}
const assigneeQuery = ref('')
const recentAssignees = ref(readRecentAssignees())
const pickerMembers = computed(() => {
  const q = assigneeQuery.value.trim().toLowerCase()
  if (q) return members.value.filter((m) => (m.name || '').toLowerCase().includes(q))
  // No query: assigned first, then MRU, then alphabetical — deduped, capped at 10
  // (but never hiding a currently-assigned member so they can be removed).
  const byId = new Map(members.value.map((m) => [m.user_id, m]))
  const seen = new Set()
  const out = []
  const add = (id) => {
    if (seen.has(id) || !byId.has(id)) return
    seen.add(id)
    out.push(byId.get(id))
  }
  const assigned = props.task.assignee_ids || []
  assigned.forEach(add)
  recentAssignees.value.forEach(add)
  ;[...members.value]
    .sort((a, b) => (a.name || '').localeCompare(b.name || ''))
    .forEach((m) => add(m.user_id))
  return out.slice(0, Math.max(10, assigned.length))
})

const due = computed(() => formatDue(props.task.due_date))
// Long form (capitalised, full weekday) for the stacked row, where the terse
// lowercase pill form would clash with capitalised siblings like the priority.
const dueLong = computed(() => formatDue(props.task.due_date, { long: true }))
const done = computed(() => !!props.task.completed_at)
// Overdue: due date in the past on a not-yet-done task.
const overdue = computed(
  () => !!props.task.due_date && !done.value && Date.parse(props.task.due_date) < Date.now(),
)
const dueTs = computed(() => (props.task.due_date ? Date.parse(props.task.due_date) : null))
const startTs = computed(() => (props.task.start_date ? Date.parse(props.task.start_date) : null))
// Estimate chip: the task's own estimate, or — if unset — the rollup sum of its
// subtasks (so a parent shows "Σ …"). Unit resolved from the project config.
const estCfg = computed(() => wsStore.estimationFor(projectId.value))
const ownEstimate = computed(() => props.task?.estimate ?? null)
const rollupEstimate = computed(() => sumEstimates(props.subtasks))
const estIsRollup = computed(() => ownEstimate.value == null && rollupEstimate.value != null)
const estText = computed(() => {
  const v = ownEstimate.value ?? rollupEstimate.value
  return v != null ? formatEstimate(v, estCfg.value) : ''
})
// Hover tooltip: full spelled-out estimate + projected window (own estimate only).
const estTooltip = computed(() => {
  const v = ownEstimate.value ?? rollupEstimate.value
  if (v == null) return ''
  const body = estIsRollup.value
    ? formatEstimateFull(v, estCfg.value)
    : estimateTooltip(props.task?.start_date, v, estCfg.value, formatters.value)
  return estIsRollup.value
    ? t('task.estimate.rollupTooltip', { value: body })
    : t('task.estimate.tooltip', { value: body })
})
// Computed, not a plain array: the labels have to re-resolve on a language switch.
const priorityOptions = computed(() => buildPriorityOptions())
// Stacked-cards effect: offset colored shadows behind the top tag pill.
// Each deeper layer peeks 5px further right and is a little shorter (larger
// negative spread) so it reads as a stack behind the top pill.
const stackLayers = computed(() => Math.min(taskTags.value.length - 1, 2))
const stackShadow = computed(() => {
  if (taskTags.value.length < 2) return ''
  // Colours are mixed to an *opaque* soft tint of the tag's hue so the layers
  // don't show through one another (the old translucent `55` alpha did).
  return taskTags.value
    .slice(1, 3)
    .map(
      (tag, i) =>
        `${(i + 1) * 5}px 0 0 ${-(i + 1)}px color-mix(in srgb, ${tag.color || '#888'} 45%, var(--t-surface))`,
    )
    .join(', ')
})
// Shared flag gradient defs live in App.vue (one per priority level), so a board
// with 100s of cards references 4 defs instead of inlining an <svg> per card.
const flagGradId = computed(() => (props.task.priority ? `t-prio-grad-${props.task.priority}` : ''))

function isAssigned(uid) {
  return (props.task.assignee_ids || []).includes(uid)
}
function hasTag(id) {
  return (props.task.tag_ids || []).includes(id)
}

// Board cards ship without the description text (stripped from the list payload
// to keep boards small); the list carries a has_description flag instead. The
// pill shows on that flag and lazily fetches the text on hover. The OR also
// honours a description a modal edit may have merged back onto the task object.
const hasDescription = computed(
  () => !!props.task.has_description || !!(props.task.description && props.task.description.trim()),
)
// Hover preview text, kept in local state (not on the prop). null = not loaded;
// seeded from the task when a description happens to be present, otherwise fetched
// once on first hover. Reset when the card is reused for a different task.
const descText = ref(props.task.description ?? null)
const descLoading = ref(false)
watch(
  () => props.task.id,
  () => {
    descText.value = props.task.description ?? null
  },
)
async function loadDescriptionPreview() {
  if (descText.value !== null || descLoading.value) return
  descLoading.value = true
  try {
    const { data } = await tasksApi.description(props.task.id)
    descText.value = data?.description || ''
  } catch {
    descText.value = ''
  } finally {
    descLoading.value = false
  }
}

// Inline edits send the full-replace body minus the description — see
// utils/taskPatch.js for why omitting it matters.
async function apply(patch) {
  await tasksApi.update(props.task.id, { ...taskBasePatch(props.task), ...patch })
  emit('changed')
}
const setPriority = (p) => apply({ priority: p })

// Per-task due-notification override sentinels (-1 / 'inherit' = user default).
const dueEnabledSel = computed(() => {
  const v = props.task.due_notify_enabled
  return v == null ? 'inherit' : v ? 'on' : 'off'
})
const dueLeadSel = computed(() => props.task.due_lead_minutes ?? -1)
const dueRepeatSel = computed(() => props.task.due_repeat_minutes ?? -1)
async function saveDueNotify(patch) {
  const lead = patch.lead ?? dueLeadSel.value
  const repeat = patch.repeat ?? dueRepeatSel.value
  const enabled = patch.enabled ?? dueEnabledSel.value
  try {
    await tasksApi.dueNotify(props.task.id, {
      lead_minutes: lead === -1 ? null : lead,
      repeat_minutes: repeat === -1 ? null : repeat,
      enabled: enabled === 'inherit' ? null : enabled === 'on',
    })
    emit('changed')
  } catch (e) {
    void e
  }
}

async function toggleTag(id) {
  if (hasTag(id)) await tasksApi.removeTag(props.task.id, id)
  else await tasksApi.addTag(props.task.id, id)
  emit('changed')
}
async function createTag() {
  const n = newTagName.value.trim()
  if (!n) return
  const palette = ['#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']
  const res = await projectsApi.createTag(projectId.value, {
    name: n,
    color: palette[Math.floor(Math.random() * palette.length)],
  })
  await tasksApi.addTag(props.task.id, res.data.id)
  newTagName.value = ''
  emit('changed')
}
async function toggleAssignee(uid) {
  const adding = !isAssigned(uid)
  if (adding) await tasksApi.addAssignee(props.task.id, uid)
  else await tasksApi.removeAssignee(props.task.id, uid)
  if (adding) {
    // Bump to the front of the MRU list (cap the stored history).
    const next = [uid, ...recentAssignees.value.filter((x) => x !== uid)].slice(0, 30)
    recentAssignees.value = next
    localStorage.setItem(RECENT_ASSIGNEES_KEY, JSON.stringify(next))
  }
  emit('changed')
}
// GitLab-member assignees: board tasks carry their logins in gitlab_assignee_logins.
const glAssigneeLogins = computed(() => props.task.gitlab_assignee_logins || [])
function isGlAssigned(username) {
  return glAssigneeLogins.value.includes(username)
}
async function toggleGlAssignee(m) {
  if (isGlAssigned(m.gl_username)) await tasksApi.removeGitlabAssignee(props.task.id, m.gl_username)
  else
    await tasksApi.pinGitlabAssignee(props.task.id, {
      gl_username: m.gl_username,
      gl_name: m.gl_name,
      gl_avatar_url: m.gl_avatar_url,
    })
  emit('changed')
}
</script>

<template>
  <div v-if="hasAnyPill" class="pills" :class="{ stacked: stackFields, ro: readonly }">
    <!-- unresolved GitLab write-back conflict on this task -->
    <n-tooltip v-if="hasConflict">
      <template #trigger>
        <button class="pill set conf-pill" @click.stop="conflictsStore.openResolver(task.id)">
          <n-icon :component="WarningOutline" :size="13" />
          <span class="pill-text">{{ t('task.pill.conflict') }}</span>
        </button>
      </template>
      {{ t('task.pill.conflictHint') }}
    </n-tooltip>

    <!-- Fields render as horizontal pills, or (stack mode) as full-width
         "icon + value" rows — same triggers/pickers, so every field stays
         clickable; hover highlights the row (see .pills.stacked CSS). -->
    <!-- priority -->
    <n-popover
      v-if="show('priority') && (showEmpty || task.priority || stackFields)"
      trigger="click"
      placement="bottom-start"
    >
      <template #trigger>
        <button
          class="pill"
          :class="{ set: task.priority }"
          :title="stackFields ? t('task.field.priority') : ''"
          data-tour="card-priority"
          @click.stop
        >
          <n-icon
            :component="FlagOutline"
            :size="13"
            :style="
              task.priority
                ? {
                    color: PRIORITY_COLORS[task.priority],
                    '--icon-grad': `url(#${flagGradId})`,
                  }
                : {}
            "
          />
          <span v-if="stackFields" class="pill-text" :class="{ 'sf-empty': !task.priority }">{{
            task.priority ? priorityLabel(task.priority) : '—'
          }}</span>
        </button>
      </template>
      <div class="menu">
        <div
          v-for="o in priorityOptions"
          :key="o.value"
          class="menu-item"
          @click="setPriority(o.value)"
        >
          <span class="dot" :style="{ background: hueGrad(PRIORITY_COLORS[o.value]) }" />
          {{ o.label }}
        </div>
      </div>
    </n-popover>

    <!-- due date: opens the calendar directly -->
    <n-popover
      v-if="show('due') && (showEmpty || due || stackFields)"
      trigger="click"
      placement="bottom-start"
    >
      <template #trigger>
        <button
          class="pill"
          :class="{ set: due, overdue }"
          :title="stackFields ? t('task.field.due') : ''"
          data-tour="card-due"
          @click.stop
        >
          <n-icon :component="CalendarClearOutline" :size="13" />
          <span v-if="due" class="pill-text">{{ stackFields ? dueLong : due }}</span>
          <span v-else-if="stackFields" class="pill-text sf-empty">—</span>
          <n-icon
            v-if="task.recurrence"
            :component="RepeatOutline"
            :size="11"
            class="pill-recur"
            :title="t('task.field.recurring')"
          />
        </button>
      </template>
      <DueEditor
        :due="dueTs"
        :start="startTs"
        :recurrence="task.recurrence"
        :notify="{ enabled: dueEnabledSel, lead: dueLeadSel, repeat: dueRepeatSel }"
        :columns="columns"
        @apply="apply"
        @notify="saveDueNotify"
      />
    </n-popover>

    <!-- estimate: display-only chip (own value, or Σ subtask rollup) -->
    <n-tooltip v-if="show('estimate') && (estText || stackFields)">
      <template #trigger>
        <div
          class="pill"
          :class="{ set: estText, 'est-pill': estText }"
          :title="stackFields ? t('task.field.estimate') : ''"
        >
          <n-icon :component="TimerOutline" :size="13" />
          <span v-if="estText" class="pill-text">{{ estIsRollup ? 'Σ ' : '' }}{{ estText }}</span>
          <span v-else-if="stackFields" class="pill-text sf-empty">—</span>
        </div>
      </template>
      {{ estText ? estTooltip : t('task.field.estimate') }}
    </n-tooltip>

    <!-- milestone («Этап»): display-only chip; editing lives in the task modal -->
    <n-tooltip v-if="show('milestone') && (taskMilestone || stackFields)">
      <template #trigger>
        <div
          class="pill"
          :class="{
            set: taskMilestone,
            'ms-pill': taskMilestone,
            closed: taskMilestone && taskMilestone.state === 'closed',
          }"
          :title="stackFields ? t('task.field.milestone') : ''"
        >
          <n-icon :component="RibbonOutline" :size="13" />
          <span v-if="taskMilestone" class="pill-text">{{ taskMilestone.title }}</span>
          <span v-else-if="stackFields" class="pill-text sf-empty">—</span>
        </div>
      </template>
      <template v-if="taskMilestone">
        {{
          taskMilestone.state === 'closed'
            ? t('task.milestone.tooltipClosed', { title: taskMilestone.title })
            : t('task.milestone.tooltip', { title: taskMilestone.title })
        }}
        <template v-if="milestoneRange(taskMilestone, formatters)">
          · {{ milestoneRange(taskMilestone, formatters) }}</template
        >
      </template>
      <template v-else>{{ t('task.field.milestone') }}</template>
    </n-tooltip>

    <!-- description: shown when the task has one (and not in stack mode); the
         text is fetched lazily on hover (board cards don't carry it), rendered
         as markdown in the popover; click opens the task. -->
    <n-popover
      v-if="show('description') && !stackFields && hasDescription"
      trigger="hover"
      placement="top-start"
      :style="{ padding: '0' }"
      @update:show="(v) => v && loadDescriptionPreview()"
    >
      <template #trigger>
        <div
          class="pill set desc-pill"
          :title="t('task.pill.hasDescription')"
          @click.stop="emit('open', task.id)"
        >
          <n-icon :component="ReorderThreeOutline" :size="14" />
        </div>
      </template>
      <div class="desc-pop">
        <div v-if="descLoading || descText === null" class="desc-loading">
          {{ t('common.state.loading') }}
        </div>
        <RichContent v-else :source="descText || ''" :members="members" />
      </div>
    </n-popover>

    <!-- tags: stacked when >1; hover previews full list, click opens picker -->
    <n-popover
      v-if="show('tags') && (showEmpty || taskTags.length || stackFields)"
      trigger="click"
      placement="bottom-start"
    >
      <template #trigger>
        <n-popover trigger="hover" :disabled="taskTags.length < 2" placement="top-start">
          <template #trigger>
            <!-- Three mutually exclusive shapes, one anchor: whichever renders is
                 the tag pill the guide points at (#2759). -->
            <button
              v-if="!stackFields && !taskTags.length"
              class="pill"
              data-tour="card-tags"
              @click.stop
            >
              <n-icon :component="PricetagOutline" :size="13" />
            </button>
            <button
              v-else-if="!stackFields"
              class="pill tag-pill"
              data-tour="card-tags"
              :style="
                firstTagScoped
                  ? { border: 'none', background: 'none', padding: 0 }
                  : {
                      border: '1px solid transparent',
                      background: tagPillBg(taskTags[0].color),
                      boxShadow: stackShadow,
                      marginRight: stackLayers ? stackLayers * 4 + 'px' : undefined,
                    }
              "
              @click.stop
            >
              <!-- Scoped pill: the cascade shadow sits on the pill itself and the
                   +N moves right past it, so the stack peeks BEHIND the tag (not
                   shoved out past the +N). Unscoped keeps +N inside the soft box. -->
              <TagPill
                class="tname"
                :tag="taskTags[0]"
                :prefix-names="tagPrefixNames"
                variant="grad-text"
                :style="firstTagScoped ? { boxShadow: stackShadow } : null"
              />
              <span
                v-if="taskTags.length > 1"
                class="more"
                :style="{
                  color: tagText(taskTags[0].color),
                  marginLeft: firstTagScoped && stackLayers ? stackLayers * 5 + 'px' : undefined,
                }"
                >+{{ taskTags.length - 1 }}</span
              >
            </button>
            <!-- stacked: leading tag icon + outlined-oval chips that fit on the
                 row, rest → +N (same behaviour as the task modal). -->
            <button
              v-else
              class="pill"
              :title="t('task.field.tags')"
              data-tour="card-tags"
              @click.stop
            >
              <n-icon
                :component="PricetagOutline"
                :size="13"
                :style="taskTags.length ? { color: tagText(taskTags[0].color) } : {}"
              />
              <span v-if="taskTags.length" ref="stagValEl" class="stag-val">
                <TagPill
                  v-for="tag in taskTags.slice(0, visibleTagCount)"
                  :key="tag.id"
                  class="mchip"
                  :tag="tag"
                  :prefix-names="tagPrefixNames"
                  variant="outline"
                />
                <span
                  v-if="visibleTagCount < taskTags.length"
                  class="mchip chip-more"
                  :style="{
                    color: tagText(taskTags[0].color),
                    background: softFill(taskTags[0].color),
                  }"
                  >+{{ taskTags.length - visibleTagCount }}</span
                >
                <!-- measurement copies must be the same component with the same
                     props, or the scope segment wouldn't be measured (useTagFit). -->
                <span ref="stagMeasureEl" class="stag-measure" aria-hidden="true">
                  <TagPill
                    v-for="tag in taskTags"
                    :key="`m${tag.id}`"
                    class="mchip"
                    :tag="tag"
                    :prefix-names="tagPrefixNames"
                    variant="outline"
                  />
                </span>
              </span>
              <span v-else class="pill-text sf-empty">—</span>
            </button>
          </template>
          <div class="preview">
            <TagPill
              v-for="tag in taskTags"
              :key="tag.id"
              class="chip"
              :tag="tag"
              :prefix-names="tagPrefixNames"
              variant="ghost"
            />
          </div>
        </n-popover>
      </template>
      <div class="menu tagmenu">
        <div class="chip-groups">
          <div v-for="g in tagPickerGroups" :key="g.key" class="chip-group">
            <div v-if="tagPickerHeaders" class="chip-grp-head">{{ g.label }}</div>
            <div class="chip-grid">
              <button
                v-for="tag in g.tags"
                :key="tag.id"
                class="tagchip"
                :class="{ on: hasTag(tag.id) }"
                :style="
                  hasTag(tag.id)
                    ? {
                        background: hueGrad(tag.color),
                        color: onColor(tag.color),
                        borderColor: 'transparent',
                      }
                    : {
                        background: softFill(tag.color),
                        color: tagText(tag.color),
                        borderColor: (tag.color || '#888') + '66',
                      }
                "
                @click="toggleTag(tag.id)"
              >
                <TagPill
                  :tag="tag"
                  :prefix-names="tagPrefixNames"
                  variant="inherit"
                  :scope-mode="tagPickerHeaders ? 'hide' : 'auto'"
                />
              </button>
            </div>
          </div>
        </div>
        <n-input
          v-model:value="newTagName"
          size="tiny"
          :placeholder="t('task.tags.newPlaceholder')"
          @keyup.enter="createTag"
          @click.stop
        />
      </div>
    </n-popover>

    <!-- author + assignees merged into one overlapping avatar group: the
         author (muted) leads, then the assignee(s). Hover shows the full
         breakdown; click opens the assignee picker (search + recent). The
         group right-aligns (margin-left:auto) and stays right-aligned even
         when it wraps to its own line. -->
    <div
      v-if="
        show('assignee') &&
        (showEmpty || author || assignees.length || glAssignees.length || stackFields)
      "
      class="people"
    >
      <n-popover
        trigger="click"
        placement="bottom-end"
        @update:show="(v) => !v && (assigneeQuery = '')"
      >
        <template #trigger>
          <n-tooltip placement="top">
            <template #trigger>
              <button
                class="pill assignee-pill"
                :title="stackFields ? t('task.field.assignee') : ''"
                data-tour="card-assignees"
                @click.stop
              >
                <n-icon
                  v-if="stackFields"
                  :component="PersonAddOutline"
                  :size="13"
                  class="sf-people-ic"
                />
                <UserAvatar
                  v-if="author && !authorIsAssignee"
                  class="avatar author-ava"
                  :user-id="author.id"
                  :src="author.avatar"
                  :name="author.name"
                />
                <UserAvatar
                  v-for="u in assignees"
                  :key="u.user_id"
                  class="avatar"
                  :user-id="u.user_id"
                  :name="u.name"
                />
                <UserAvatar
                  v-for="(g, i) in glAssignees"
                  :key="`g${i}`"
                  class="avatar ext-ava"
                  :src="g.avatar_url"
                  :name="g.name || g"
                />
                <n-icon
                  v-if="!stackFields && !author && !assignees.length && !glAssignees.length"
                  :component="PersonAddOutline"
                  :size="13"
                />
                <span
                  v-else-if="stackFields && !author && !assignees.length && !glAssignees.length"
                  class="pill-text sf-empty"
                  >—</span
                >
              </button>
            </template>
            <div class="people-tip">
              <div v-if="author">
                {{
                  t('task.people.author', {
                    name: author.gl ? `@${author.login} (GitLab)` : author.name,
                  })
                }}
              </div>
              <div v-if="assigneeNames">
                {{ t('task.people.assignee', { names: assigneeNames }) }}
              </div>
              <div v-if="!author && !assigneeNames">{{ t('task.people.none') }}</div>
            </div>
          </n-tooltip>
        </template>
        <div class="menu assignee-menu">
          <n-input
            v-model:value="assigneeQuery"
            size="tiny"
            :placeholder="t('common.action.search')"
            clearable
            @click.stop
          />
          <div class="assignee-list">
            <div
              v-for="m in pickerMembers"
              :key="m.user_id"
              class="menu-item assignee-item"
              @click="toggleAssignee(m.user_id)"
            >
              <UserAvatar class="avatar sm" :user-id="m.user_id" :name="m.name" />
              <span class="aname">{{ m.name }}</span>
              <n-icon v-if="isAssigned(m.user_id)" :component="CheckmarkOutline" class="chk" />
            </div>
            <template v-if="gitlabMembers.length">
              <div class="assignee-sep">GitLab</div>
              <div
                v-for="m in gitlabMembers"
                :key="m.gl_user_id"
                class="menu-item assignee-item"
                @click="toggleGlAssignee(m)"
              >
                <UserAvatar
                  class="avatar sm"
                  :src="m.gl_avatar_url"
                  :name="m.gl_name || m.gl_username"
                />
                <span class="aname">{{ m.gl_name || m.gl_username }}</span>
                <n-icon
                  v-if="isGlAssigned(m.gl_username)"
                  :component="CheckmarkOutline"
                  class="chk"
                />
              </div>
            </template>
            <div v-if="!pickerMembers.length && !gitlabMembers.length" class="assignee-empty">
              {{ t('task.people.noMatches') }}
            </div>
          </div>
        </div>
      </n-popover>
    </div>
  </div>
</template>

<style scoped>
.pills {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
}
/* Archive (read-only) view: the pills become display-only so a click passes
   through to open the read-only modal. Lives here, not on the card: a scoped
   `.card.tc-readonly .pill` selector in the parent can't reach into this
   component's markup. */
.pills.ro .pill,
.pills.ro .assignee-pill {
  pointer-events: none;
}
/* Stacked mode (customize view): the same pill triggers, laid out as full-width
   "icon + value" rows. Field icons stay flush-left so values align on one axis;
   rows highlight on hover and remain clickable (open the same pickers). */
.pills.stacked {
  flex-direction: column;
  align-items: stretch;
  gap: 1px;
}
.pills.stacked .people {
  margin-left: 0;
  width: 100%;
}
/* box-sizing:border-box is the key fix: <button> pills default to border-box but
   the display-only <div> pills (estimate/milestone) are content-box, so width:100%
   + padding made them overflow the card and mismatch height. Force border-box so
   every row is the same height and its hover spans exactly the card width. */
.pills.stacked .pill,
.pills.stacked .assignee-pill {
  box-sizing: border-box;
  width: 100%;
  max-width: 100%;
  justify-content: flex-start;
  gap: 8px;
  min-height: 26px;
  padding: 3px 6px;
  border: 1px solid transparent;
  background: transparent;
  border-radius: 6px;
  overflow: hidden;
}
.pills.stacked .pill:hover,
.pills.stacked .assignee-pill:hover {
  background: var(--t-hover);
}
.pills.stacked .pill .n-icon {
  flex: none;
}
.pills.stacked .pill-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pills.stacked .sf-empty {
  color: var(--t-text3);
}
/* Assignee row: keep the avatar overlap cascade (no flex gap between avatars),
   give the field icon its own spacing, and paint the avatar ring in the row's
   current background so it doesn't flash white on the grey hover. */
.pills.stacked .assignee-pill {
  gap: 0;
}
.pills.stacked .sf-people-ic {
  margin-right: 8px;
  flex: none;
}
.pills.stacked .sf-people-ic + .avatar {
  margin-left: 0;
}
.pills.stacked .assignee-pill:hover .avatar {
  border-color: var(--t-hover);
}
/* Stacked tags row: chips that fit, rest → +N (mirrors the task modal). */
.stag-val {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  position: relative;
}
/* Outlined-oval tag chip: gradient border-box + soft fill (via inline background),
   like the modal. The 1px transparent border is on every chip incl. the invisible
   measurement copies so widths match exactly. */
.mchip {
  font-size: 11px;
  padding: 1px 8px;
  border: 1px solid transparent;
  border-radius: 10px;
  flex: none;
  white-space: nowrap;
}
.mchip.chip-more {
  color: var(--t-text3);
  background: var(--t-surface-alt);
}
/* Invisible natural-width measurement row (never sliced). */
.stag-measure {
  position: absolute;
  left: 0;
  top: 0;
  display: inline-flex;
  gap: 6px;
  visibility: hidden;
  pointer-events: none;
  white-space: nowrap;
}
.pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 22px;
  padding: 2px 6px;
  border-radius: 6px;
  border: 1px dashed var(--t-border);
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
/* The estimate pill is a <div>, not a <button>: form controls get UA
   `line-height: normal`, but a bare div inherits the card's taller line-height,
   making it ~5px higher than the sibling pills. Pin its box model so it matches. */
.est-pill {
  box-sizing: border-box;
  height: 22px;
  line-height: 1;
}
/* milestone chip: same neutral look as the other set pills (no fill) */
.ms-pill {
  box-sizing: border-box;
  height: 22px;
  line-height: 1;
  max-width: 140px;
  border-style: solid;
  color: var(--t-text2);
}
.ms-pill .pill-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ms-pill.closed {
  opacity: 0.6;
}
/* conflict warning pill: orange, draws the eye to an unresolved write-back conflict */
.conf-pill {
  box-sizing: border-box;
  height: 22px;
  line-height: 1;
  border-style: solid;
  border-color: color-mix(in srgb, #e0922f 55%, transparent);
  background: color-mix(in srgb, #e0922f 14%, transparent);
  color: #b96a08;
}
.pill.set {
  border-style: solid;
  color: var(--t-text2);
}
/* repeat glyph on a recurring task's due pill — inherits the pill's text
   colour (purple accent clashed inside the neutral pill, worse on dark) */
.pill-recur {
  color: inherit;
}
/* overdue due-date pill: soft red tint (like a warning tag) */
.pill.overdue {
  color: #e0533d;
  border-color: #e0533d;
  border-style: solid;
  background: color-mix(in srgb, #e0533d 12%, transparent);
}
/* Flag (priority) icon carries the active priority's gradient (set --icon-grad
   inline on the flag only; the date pill is also .pill.set but has no
   --icon-grad, so it falls back to currentColor). */
.pill.set :deep(svg [stroke='currentColor']) {
  stroke: var(--icon-grad, currentColor);
}
.pill.set :deep(svg [fill='currentColor']) {
  fill: var(--icon-grad, currentColor);
}
.pill-text {
  font-size: 11px;
}
/* description indicator: icon-only pill; hover opens the rendered-markdown card.
   It's a <div>, so pin the box model like .est-pill to match the sibling pills. */
.desc-pill {
  box-sizing: border-box;
  height: 22px;
  line-height: 1;
  padding: 2px 5px;
}
.desc-pop {
  max-width: 340px;
  max-height: 320px;
  overflow: auto;
  padding: 10px 12px;
  font-size: 13px;
}
.desc-loading {
  padding: 8px 12px;
  font-size: 12px;
  color: var(--t-text3);
}
.chip {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 10px;
}
.tag-pill {
  border-style: solid;
  gap: 5px;
}
.tname {
  font-size: 11px;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.more {
  font-size: 10px;
  opacity: 0.8;
}
.preview {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  max-width: 220px;
}
.avatar {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 10px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  /* cascade: overlap the previous avatar with a ring in the card colour */
  margin-left: -9px;
  border: 2px solid var(--t-surface);
}
.avatar:first-child {
  margin-left: 0;
}
.avatar.sm {
  margin-left: 0;
  border: none;
}
.assignee-pill {
  border: none;
  padding: 2px;
}
/* author + assignee group — pushed to the right edge of the row; margin-left
   auto keeps it right-aligned even when it wraps onto its own line. */
.people {
  display: inline-flex;
  align-items: center;
  margin-left: auto;
}
/* the author avatar: read-only, visually muted vs the accent assignees, and it
   leads the stack (so it sits flush-left like any first avatar) */
.author-ava {
  background: var(--t-text3);
  opacity: 0.85;
}
/* Multiline people tooltip (Автор / Исполнитель breakdown). */
.people-tip {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
  line-height: 1.35;
}
/* Assignee picker: a pinned search box above a scrollable, capped list. */
.assignee-menu {
  gap: 6px;
}
.assignee-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 240px;
  overflow-y: auto;
}
.assignee-empty {
  padding: 8px 6px;
  text-align: center;
  font-size: 12px;
  color: var(--t-text3);
}
.assignee-sep {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--t-text3);
  padding: 6px 6px 2px;
}
/* external GitLab assignee (no Tessera account): neutral, slightly muted */
.ext-ava {
  background: var(--t-text3);
  opacity: 0.9;
}
.menu {
  min-width: 180px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tagmenu {
  width: 300px;
  max-width: 80vw;
  max-height: 260px;
  overflow-y: auto;
}
.chip-grp-head {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.6;
  margin: 6px 0 4px;
}
.chip-group:first-child .chip-grp-head {
  margin-top: 0;
}
.chip-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-bottom: 8px;
}
.tagchip {
  font-size: 12px;
  padding: 2px 9px;
  border-radius: 10px;
  border: 1px solid transparent;
  background: transparent;
  cursor: pointer;
}
.tagchip.on {
  font-weight: 600;
}
.menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 6px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}
.menu-item:hover {
  background: var(--t-hover);
}
.assignee-item .aname {
  flex: 1;
}
.assignee-item .chk {
  color: var(--t-primary);
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
</style>
