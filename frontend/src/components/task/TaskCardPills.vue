<script setup>
// The property row of a board card: priority · due · estimate · milestone ·
// description · tags · people, plus the pickers each of them opens.
//
// Extracted from TaskCard (#2665). Everything here is board *context* — it comes
// from the board store, not from props — so the card passes only the task it
// renders. The row decides for itself whether it has anything to show: when no
// field survives the size preset and the customize toggles it renders nothing.
import { tourEvent } from '@/utils/tourEvents'
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NPopover, NTooltip, NInput, useMessage } from 'naive-ui'
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
import { hueGrad, softFill, readableHue } from '@/utils/gradient'
import { buildTagGroups, matchTagByName } from '@/utils/tagGroups'
import { milestoneRange } from '@/utils/milestones'
import { useFormat } from '@/composables/useFormat'
import {
  formatEstimate,
  formatEstimateFull,
  estimateTooltip,
  sumEstimates,
} from '@/utils/estimation'
import { cardFieldVisible } from '@/utils/cardFields'
import { glAssigneesOf, glPickerMembers } from '@/utils/assignees'
import { isTaskOverdue } from '@/utils/taskFilter'
import { uiCollator } from '@/i18n'
import { taskBasePatch } from '@/utils/taskPatch'
import PopMenu from '../PopMenu.vue'
import UserAvatar from '../UserAvatar.vue'
import DueEditor from '../DueEditor.vue'
import RichContent from '../RichContent.vue'
import TagPill from '../TagPill.vue'
import TagPicker from './TagPicker.vue'
import ReminderPill from './ReminderPill.vue'
import { useThemeStore } from '@/stores/theme'
import { useBoardViewStore } from '@/stores/boardView'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useConflictsStore } from '@/stores/conflicts'
import { useInboxStore } from '@/stores/inbox'
import { useProjectWindowStore } from '@/stores/projectWindow'
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
// The reader's next open reminder on this task (#2998) — read from the shell's
// «Входящие» copy (one map for the whole board), shown after the due pill.
// A done task carries no bell.
const inbox = useInboxStore()
const reminder = computed(() =>
  props.task.completed_at ? null : inbox.byTask.get(props.task.id)?.[0] || null,
)
const { formatDue, formatters } = useFormat()
const { t } = useI18n()
const message = useMessage()
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
// Whether the row has any content at all (drives whether it renders). Every term
// mirrors the matching pill's own `v-if` below — including its `stackFields` leg:
// in stack mode a visible field draws its row even unset, and that row is the
// field's ONLY trigger on the card. Without that leg here a card with nothing set
// at all (and «показывать пустые» off) lost the whole block — priority, due, tags
// and assignees became unreachable from the board on exactly the cards that need
// them, while a neighbour differing by a single value showed all six rows.
// Description is the mirror image: its pill is suppressed in stack mode, so its
// term carries the same `!stackFields`.
const hasAnyPill = computed(
  () =>
    hasConflict.value ||
    (show('priority') && (showEmpty.value || stackFields.value || props.task.priority)) ||
    (show('due') && (showEmpty.value || stackFields.value || due.value)) ||
    (show('due') && !!reminder.value) ||
    (show('estimate') && (stackFields.value || estText.value)) ||
    (show('milestone') && (stackFields.value || taskMilestone.value)) ||
    (show('description') && !stackFields.value && hasDescription.value) ||
    (show('tags') && (showEmpty.value || stackFields.value || taskTags.value.length)) ||
    (show('assignee') &&
      (showEmpty.value ||
        stackFields.value ||
        author.value ||
        assignees.value.length ||
        glAssignees.value.length)),
)

// The pickable tags — what a typed name is matched against, so the answer covers
// exactly the tags the reader could have clicked instead (GitLab meta-prefixed
// tags are left out on purpose: attaching one desyncs from the field it drives).
const tagPickerFlat = computed(() =>
  buildTagGroups(tags.value, tagPrefixNames, metaTagPrefixes.value).flatMap((g) => g.tags),
)
const creatingTag = ref(false)
// The v2 tag picker (#3113), anchored at whichever tag pill this card draws.
const tagOpen = ref(false)
const tagBtn = ref(null)
// «Управлять тегами…»: the project window on «Теги», filtered by what was typed.
const projectWindow = useProjectWindowStore()
function manageTags(typed = '') {
  if (!projectId.value) return
  tagOpen.value = false
  projectWindow.open(projectId.value, 'tags', { filter: (typed || '').trim() })
}

const taskTags = computed(() => (props.task.tag_ids || []).map((id) => tagsMap[id]).filter(Boolean))
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
// their names/logins (no avatar), so the shared resolver looks the face up in the
// workspace GitLab-members list — same source the picker/filter/list rows use.
const glAssignees = computed(() => glAssigneesOf(props.task, gitlabMembers.value))
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

// v2 avatar cascade (#2954): assignees, then GitLab assignees, then the author —
// at most three avatars on the card, the rest folded into «+N». Hovering the
// cascade lists everyone with their role. An author who is also an assignee
// shows once, as the assignee.
const PEOPLE_SHOWN = 3
const people = computed(() => {
  const out = [
    ...assignees.value.map((u) => ({
      key: `u${u.user_id}`,
      userId: u.user_id,
      name: u.name,
      role: 'assignee',
    })),
    ...glAssignees.value.map((g, i) => ({
      key: `g${i}`,
      src: g.avatar_url,
      name: g.name || g,
      role: 'gitlab',
      ext: true,
    })),
  ]
  const a = author.value
  if (a && !authorIsAssignee.value)
    out.push({ key: 'author', userId: a.id, src: a.avatar, name: a.name, role: 'author' })
  return out
})
const shownPeople = computed(() => people.value.slice(0, PEOPLE_SHOWN))
const extraPeople = computed(() => Math.max(0, people.value.length - PEOPLE_SHOWN))

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
  const cmp = uiCollator()
  ;[...members.value]
    .sort((a, b) => cmp.compare(a.name || '', b.name || ''))
    .forEach((m) => add(m.user_id))
  return out.slice(0, Math.max(10, assigned.length))
})

const due = computed(() => formatDue(props.task.due_date))
// Long form (capitalised, full weekday) for the stacked row, where the terse
// lowercase pill form would clash with capitalised siblings like the priority.
const dueLong = computed(() => formatDue(props.task.due_date, { long: true }))
// Overdue: the due date's calendar day is already past. Shared with the «Срок»
// facet — a card painted red has to be one the «Просрочено» filter still lists
// (it also owns the "a closed task is never late" half, which used to live here).
const overdue = computed(() => isTaskOverdue(props.task, new Date(), formatters.value))
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

// Every write below goes through here: a refusal is reported and the board
// re-read either way. The pills draw straight off `props.task`, so a rejected
// promise on its own is indistinguishable from a click that missed — no toast,
// no change, and the board still holding whatever it held (including a change a
// racing second client DID make). Same contract the right-click menu over this
// card already follows (composables/useTaskMenu.js).
//
// @returns {boolean} whether the write went through
async function run(fn) {
  try {
    await fn()
  } catch (e) {
    message.error(e.message)
    emit('changed')
    return false
  }
  emit('changed')
  // «Поля на карточке» of the «Начало работы» checklist (#3000).
  tourEvent('task.fields')
  return true
}
// Inline edits send the full-replace body minus the description — see
// utils/taskPatch.js for why omitting it matters.
const apply = (patch) =>
  run(() => tasksApi.update(props.task.id, { ...taskBasePatch(props.task), ...patch }))
const setPriority = (p) => apply({ priority: p })

// Per-task due-notification override sentinels (-1 / 'inherit' = user default).
const dueEnabledSel = computed(() => {
  const v = props.task.due_notify_enabled
  return v == null ? 'inherit' : v ? 'on' : 'off'
})
const dueLeadSel = computed(() => props.task.due_lead_minutes ?? -1)
const dueRepeatSel = computed(() => props.task.due_repeat_minutes ?? -1)
function saveDueNotify(patch) {
  const lead = patch.lead ?? dueLeadSel.value
  const repeat = patch.repeat ?? dueRepeatSel.value
  const enabled = patch.enabled ?? dueEnabledSel.value
  return run(() =>
    tasksApi.dueNotify(props.task.id, {
      lead_minutes: lead === -1 ? null : lead,
      repeat_minutes: repeat === -1 ? null : repeat,
      enabled: enabled === 'inherit' ? null : enabled === 'on',
    }),
  )
}

const toggleTag = (id) =>
  run(() =>
    hasTag(id) ? tasksApi.removeTag(props.task.id, id) : tasksApi.addTag(props.task.id, id),
  )
// Resolves true once the tag is on the task — the picker clears its query then.
async function createTag(name) {
  const n = (name || '').trim()
  // The guard is what makes a second Enter harmless: the field is cleared only once
  // the tag is on the task, so an impatient repeat used to send a second create —
  // and the second one came back «имя занято» about the tag the first had just made.
  if (!n || creatingTag.value) return false
  // A name the project already carries is a request for THAT tag, not for a twin:
  // the server refuses the duplicate (409) and the task would stay untagged.
  const existing = matchTagByName(tagPickerFlat.value, n)
  if (existing) {
    if (hasTag(existing.id)) message.info(t('task.tags.alreadyOn'))
    else await run(() => tasksApi.addTag(props.task.id, existing.id))
    return true
  }
  const palette = ['#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']
  // The name is cleared only once both calls are through: a create that landed
  // but failed to attach leaves the tag on the project, and the row it is about
  // to appear in is the one to retry from.
  creatingTag.value = true
  let ok
  try {
    ok = await run(async () => {
      const res = await projectsApi.createTag(projectId.value, {
        name: n,
        color: palette[Math.floor(Math.random() * palette.length)],
      })
      await tasksApi.addTag(props.task.id, res.data.id)
    })
  } finally {
    creatingTag.value = false
  }
  return !!ok
}
async function toggleAssignee(uid) {
  const adding = !isAssigned(uid)
  const ok = await run(() =>
    adding ? tasksApi.addAssignee(props.task.id, uid) : tasksApi.removeAssignee(props.task.id, uid),
  )
  // The MRU is «people you actually assign», so a refused pick must not enter it
  // — it would push a name nobody was assigned to the top of every other picker.
  if (ok && adding) {
    // Bump to the front of the MRU list (cap the stored history).
    const next = [uid, ...recentAssignees.value.filter((x) => x !== uid)].slice(0, 30)
    recentAssignees.value = next
    localStorage.setItem(RECENT_ASSIGNEES_KEY, JSON.stringify(next))
  }
}
// GitLab half of the picker: the project roster plus whoever this task already
// carries that the roster has since dropped — otherwise a person who left the
// GitLab project stays assigned with no row to un-assign them from (see
// utils/assignees). Shares the search box with the Tessera half above.
const glPickerList = computed(() =>
  glPickerMembers(gitlabMembers.value, glAssignees.value, assigneeQuery.value),
)
// GitLab-member assignees: board tasks carry their logins in gitlab_assignee_logins.
const glAssigneeLogins = computed(() => props.task.gitlab_assignee_logins || [])
function isGlAssigned(username) {
  return glAssigneeLogins.value.includes(username)
}
const toggleGlAssignee = (m) =>
  run(() =>
    isGlAssigned(m.gl_username)
      ? tasksApi.removeGitlabAssignee(props.task.id, m.gl_username)
      : tasksApi.pinGitlabAssignee(props.task.id, {
          gl_username: m.gl_username,
          gl_name: m.gl_name,
          gl_avatar_url: m.gl_avatar_url,
        }),
  )
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
          class="pill prio-pill"
          :class="{ set: task.priority }"
          :style="task.priority ? { '--pc': PRIORITY_COLORS[task.priority] } : null"
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
      <PopMenu>
        <div
          v-for="o in priorityOptions"
          :key="o.value"
          class="menu-item"
          @click="setPriority(o.value)"
        >
          <span class="dot" :style="{ background: hueGrad(PRIORITY_COLORS[o.value]) }" />
          {{ o.label }}
        </div>
      </PopMenu>
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

    <!-- reminder bell (#2998): right after the due date -->
    <ReminderPill
      v-if="show('due') && reminder"
      :reminder="reminder"
      :task="task"
      :readonly="readonly"
    />

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

    <!-- tags: stacked when >1; hover previews the full list, a click opens the
         v2 tag picker (#3113) -->
    <n-popover
      v-if="show('tags') && (showEmpty || taskTags.length || stackFields)"
      trigger="hover"
      :disabled="taskTags.length < 2 || tagOpen"
      placement="top-start"
    >
      <template #trigger>
        <!-- Three mutually exclusive shapes, one anchor: whichever renders is
             the tag pill the guide points at (#2759). -->
        <button
          v-if="!stackFields && !taskTags.length"
          ref="tagBtn"
          class="pill"
          data-tour="card-tags"
          @click.stop="!readonly && (tagOpen = !tagOpen)"
        >
          <n-icon :component="PricetagOutline" :size="13" />
        </button>
        <button
          v-else-if="!stackFields"
          ref="tagBtn"
          class="pill tag-pill"
          data-tour="card-tags"
          @click.stop="!readonly && (tagOpen = !tagOpen)"
        >
          <!-- v2 (#2954): the first tag in its own outline pill, the rest
               counted as plain «+N» text beside it. -->
          <TagPill class="tname" :tag="taskTags[0]" :prefix-names="tagPrefixNames" />
          <span v-if="taskTags.length > 1" class="more">+{{ taskTags.length - 1 }}</span>
        </button>
        <!-- stacked: leading tag icon + outlined-oval chips that fit on the
             row, rest → +N (same behaviour as the task modal). -->
        <button
          v-else
          ref="tagBtn"
          class="pill"
          :title="t('task.field.tags')"
          data-tour="card-tags"
          @click.stop="!readonly && (tagOpen = !tagOpen)"
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
    <TagPicker
      v-if="!readonly && show('tags')"
      :show="tagOpen"
      :anchor="() => tagBtn"
      :tags="tags"
      :selected="task.tag_ids || []"
      :prefix-names="tagPrefixNames"
      :locked-prefixes="metaTagPrefixes"
      :creating="creatingTag"
      :create="createTag"
      :can-manage="!!projectId"
      key-id="card.tags"
      testid="card-tag-picker"
      @update:show="tagOpen = $event"
      @toggle="toggleTag"
      @manage="manageTags"
    />

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
          <n-popover
            trigger="hover"
            placement="bottom-end"
            :delay="400"
            :show-arrow="false"
            raw
            class="people-pop"
          >
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
                  v-for="p in shownPeople"
                  :key="p.key"
                  class="avatar"
                  :class="{ 'author-ava': p.role === 'author', 'ext-ava': p.ext }"
                  :user-id="p.userId"
                  :src="p.src"
                  :name="p.name"
                />
                <span v-if="extraPeople" class="avatar more-ava">+{{ extraPeople }}</span>
                <n-icon
                  v-if="!stackFields && !people.length"
                  :component="PersonAddOutline"
                  :size="14"
                  class="no-people"
                />
                <span v-else-if="stackFields && !people.length" class="pill-text sf-empty">—</span>
              </button>
            </template>
            <div class="people-list">
              <div v-for="p in people" :key="p.key" class="people-row">
                <UserAvatar class="avatar sm" :user-id="p.userId" :src="p.src" :name="p.name" />
                <span class="people-name">{{ p.name }}</span>
                <span class="people-role">{{ t(`task.people.role.${p.role}`) }}</span>
              </div>
              <div v-if="!people.length" class="people-row people-none">
                {{ t('task.people.none') }}
              </div>
            </div>
          </n-popover>
        </template>
        <PopMenu class="assignee-menu">
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
            <template v-if="glPickerList.length">
              <div class="assignee-sep">GitLab</div>
              <div
                v-for="m in glPickerList"
                :key="m.gl_username"
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
            <div v-if="!pickerMembers.length && !glPickerList.length" class="assignee-empty">
              {{ t('task.people.noMatches') }}
            </div>
          </div>
        </PopMenu>
      </n-popover>
    </div>
  </div>
</template>

<style scoped>
.pills {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
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
  height: auto;
  min-height: 26px;
  padding: 3px 6px;
  background: transparent;
  border-radius: 7px;
  overflow: hidden;
  opacity: 1;
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
  box-shadow: 0 0 0 2px var(--t-hover);
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
/* v2 pills (#2954): capsules with no outline. An empty field is a bare muted
   icon; a set one sits on the neutral pill level; priority, overdue and
   conflict carry a soft same-hue gradient of their own colour. */
.pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  box-sizing: border-box;
  height: 22px;
  min-width: 22px;
  padding: 0 4px;
  border: none;
  border-radius: var(--t-r-pill);
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  font-size: 12px;
  font-weight: 500;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
}
.pill:not(.set) {
  opacity: 0.7;
}
.pill:not(.set):hover {
  opacity: 1;
}
.pill.set {
  padding: 0 7px;
  background: var(--t-pill);
  color: var(--t-text2);
}
/* Soft same-hue fill: base colour mixed over the card at 11% → 19%. */
.prio-pill.set,
.pill.overdue,
.conf-pill {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--pc) 11%, var(--card-fill, var(--t-card))),
    color-mix(in srgb, var(--pc) 19%, var(--card-fill, var(--t-card)))
  );
  color: var(--pc);
}
.prio-pill.set {
  padding: 0 5px;
}
.pill.overdue {
  --pc: #e0533d;
  font-weight: 600;
}
.conf-pill {
  --pc: #f0a020;
}
.est-pill,
.ms-pill {
  max-width: 140px;
}
.ms-pill .pill-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ms-pill.closed {
  opacity: 0.6;
}
/* repeat glyph on a recurring task's due pill — inherits the pill's text
   colour (purple accent clashed inside the neutral pill, worse on dark) */
.pill-recur {
  color: inherit;
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
/* description indicator: icon-only pill; hover opens the rendered-markdown card. */
.pill.desc-pill {
  padding: 0 5px;
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
.pill.tag-pill {
  flex: 0 1 auto;
  min-width: 0;
  max-width: 100%;
  gap: 4px;
  padding: 0;
  background: none;
  opacity: 1;
}
/* The first tag's own box: TagPill leaves padding/radius/size to the call site
   (a two-tone scoped tag overrides the padding with its segments). It may
   shrink so a long scoped tag truncates instead of overflowing the card. */
.tname {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: var(--t-r-pill);
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.more {
  padding: 0 2px;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
}
.preview {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  max-width: 220px;
}
/* v2 cascade: 22px avatars overlapping by 6px, each ringed in the card colour so
   the overlap reads as a stack. */
.avatar {
  width: 22px;
  height: 22px;
  flex: none;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 10px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-left: -6px;
  box-shadow: 0 0 0 2px var(--card-fill, var(--t-card));
}
.avatar:first-child {
  margin-left: 0;
}
.more-ava {
  width: auto;
  min-width: 22px;
  padding: 0 5px;
  box-sizing: border-box;
  border-radius: 11px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 11px;
}
.avatar.sm {
  width: 20px;
  height: 20px;
  margin-left: 0;
  box-shadow: none;
}
.assignee-pill {
  padding: 0 2px 0 6px;
  background: none;
  opacity: 1;
}
.assignee-pill.pill:not(.set) {
  opacity: 1;
}
.no-people {
  color: var(--t-text3);
  opacity: 0.75;
}
/* Hover list of everyone on the card, rendered into body (raw popover). */
.people-list {
  min-width: 220px;
  padding: 6px;
  border-radius: var(--t-r-tip);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.people-row {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 30px;
  padding: 0 6px;
  border-radius: 7px;
}
.people-name {
  flex: 1;
  font-size: 13px;
  color: var(--t-text1);
  white-space: nowrap;
}
.people-role {
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.people-none {
  font-size: 12.5px;
  color: var(--t-text3);
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
.menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 6px;
  border-radius: 7px;
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
