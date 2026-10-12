<script setup>
// Quick entry on the start screen (v2, #2960): one line that reads itself as you
// type — a date, a time, «!приоритет», «#тег», «@человек» (utils/quickParse) —
// with the chips and the target board previewed under it. Enter creates the task
// in the target board's «К работе»; Tab moves to the board picker.
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useTaskCreatedToast } from '@/composables/useTaskCreatedToast'
import { NIcon, NPopover } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  AddCircleOutline,
  CalendarClearOutline,
  FlagOutline,
  ChevronDownOutline,
} from '@vicons/ionicons5'
import { boards as boardsApi, projects as projApi, tasks as tasksApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useFormat } from '@/composables/useFormat'
import { parseQuick } from '@/utils/quickParse'
import { sGet } from '@/utils/serverStorage'
import { sortedColumns } from '@/utils/status'
import { todoColumn } from '@/utils/columnStatus'
import { columnName } from '@/utils/defaultNames'
import { normalizeTitle } from '@/utils/title'
import { humanizeError } from '@/utils/errors'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { priorityLabel } from '@/utils/priority'
import { hueGrad } from '@/utils/gradient'
import TagPill from '../TagPill.vue'
import UserAvatar from '../UserAvatar.vue'
import ShortcutKeys from '../ShortcutKeys.vue'

const props = defineProps({
  members: { type: Array, default: () => [] },
  // Every tag of the workspace; a #tag resolves inside the target board's project.
  tags: { type: Array, default: () => [] },
})
const emit = defineEmits(['created'])

const { t } = useI18n()
const message = useMessage()
const taskCreated = useTaskCreatedToast()
const ws = useWorkspacesStore()
const { formatDue } = useFormat()

const text = ref('')
const inputEl = ref(null)
const busy = ref(false)

// ── the target board: the one last open, else the first ──
const boards = ref([]) // [{ id, name, project }]
const boardId = ref(null)
const todoName = ref('')
const pickOpen = ref(false)
async function loadBoards() {
  const projects = ws.projects || []
  const lists = await Promise.allSettled(projects.map((p) => projApi.boards(p.id)))
  boards.value = projects.flatMap((p, i) =>
    (lists[i].status === 'fulfilled' ? lists[i].value.data || [] : []).map((b) => ({
      id: b.id,
      name: b.name,
      project: p,
    })),
  )
  const last = sGet('tessera_last_board_id')
  boardId.value = boards.value.some((b) => b.id === last) ? last : boards.value[0]?.id || null
  loadColumn()
}
const target = computed(() => boards.value.find((b) => b.id === boardId.value) || null)
let todoCol = null
async function loadColumn() {
  todoCol = null
  todoName.value = ''
  if (!boardId.value) return
  try {
    const cols = (await boardsApi.columns(boardId.value)).data || []
    todoCol = todoColumn(sortedColumns(cols))
    todoName.value = columnName(todoCol) || ''
  } catch {
    /* the preview just goes without the column name */
  }
}
function pickBoard(id) {
  boardId.value = id
  pickOpen.value = false
  loadColumn()
  nextTick(() => inputEl.value?.focus())
}

const projectTags = computed(() =>
  props.tags.filter((tg) => !target.value || tg.project_id === target.value.project.id),
)
const parsed = computed(() =>
  parseQuick(text.value, { people: props.members, tags: projectTags.value }),
)
const typing = computed(() => !!text.value.trim())

function onTab(e) {
  if (!typing.value) return
  e.preventDefault()
  pickOpen.value = true
}

async function submit() {
  const p = parsed.value
  const title = normalizeTitle(p.title)
  if (!typing.value || busy.value) return
  if (!target.value) {
    message.warning(t('shell.home.quick.noBoard'))
    return
  }
  busy.value = true
  try {
    if (!todoCol) await loadColumn()
    if (!todoCol) throw new Error(t('board.column.needStatus'))
    const res = await boardsApi.createTask(target.value.id, {
      column_id: todoCol.id,
      title: title || t('shell.home.quick.untitled'),
      priority: p.priority ?? 0,
      due_date: p.due,
    })
    const task = res.data
    // Tags and people go on after: a missing #tag is created in the board's project.
    const palette = ['#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']
    for (const tg of p.tags) {
      let id = tg.tag?.id
      if (!id) {
        const made = await projApi.createTag(target.value.project.id, {
          name: tg.name,
          color: palette[Math.floor(Math.random() * palette.length)],
        })
        id = made.data.id
      }
      await tasksApi.addTag(task.id, id)
    }
    for (const person of p.people) await tasksApi.addAssignee(task.id, person.user_id)
    // The board is not on screen: the toast says where the task went (#3117).
    taskCreated(task, { project: target.value.project, board: target.value, column: todoCol })
    text.value = ''
    emit('created', task)
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    busy.value = false
  }
}

// The projects may still be loading when the screen mounts: list the boards
// once they are there, and again when the workspace changes.
watch(
  () => (ws.projects || []).map((p) => p.id).join(','),
  (ids) => ids && loadBoards(),
  { immediate: true },
)
defineExpose({ focus: () => inputEl.value?.focus() })
</script>

<template>
  <div class="hq" :class="{ typing }">
    <div class="hq-box">
      <n-icon :component="AddCircleOutline" :size="22" class="hq-ico" />
      <input
        ref="inputEl"
        v-model="text"
        class="hq-input"
        :placeholder="t('shell.home.quick.placeholder')"
        :disabled="busy"
        data-testid="home-quick-input"
        @keydown.enter.prevent="submit"
        @keydown.tab="onTab"
        @keydown.esc="text = ''"
      />
      <span v-if="typing" class="hq-hint kbd-hint">
        <ShortcutKeys id="card.open" /> {{ t('shell.home.quick.hintTyping') }} ·
        <ShortcutKeys id="palette.actions" /> {{ t('shell.home.quick.hintBoard') }}
      </span>
      <span v-else class="hq-hint kbd-hint">
        {{ t('shell.home.quick.hintIdle') }} <ShortcutKeys id="create.task" />
      </span>
    </div>

    <div v-if="typing" class="hq-preview" data-testid="home-quick-preview">
      <i18n-t keypath="shell.home.quick.taskTitle" tag="span" class="hq-what" scope="global">
        <template #title>
          <b>{{ parsed.title || t('shell.home.quick.untitled') }}</b>
        </template>
      </i18n-t>
      <span v-if="parsed.due" class="hq-chip hq-date">
        <n-icon :component="CalendarClearOutline" :size="13" />
        {{ formatDue(parsed.due) }}
      </span>
      <span
        v-if="parsed.priority"
        class="hq-chip"
        :style="{
          color: PRIORITY_COLORS[parsed.priority],
          background: `color-mix(in srgb, ${PRIORITY_COLORS[parsed.priority]} 16%, transparent)`,
        }"
      >
        <n-icon :component="FlagOutline" :size="13" />
        {{ priorityLabel(parsed.priority) }}
      </span>
      <TagPill
        v-for="tg in parsed.tags"
        :key="tg.name"
        :tag="tg.tag || { name: tg.name, color: '#9aa0aa' }"
        variant="outline"
      />
      <span v-for="m in parsed.people" :key="m.user_id" class="hq-chip hq-person">
        <UserAvatar class="hq-ava" :user-id="m.user_id" :name="m.name" />
        {{ m.name }}
      </span>
      <span class="hq-in">{{ t('shell.home.quick.in') }}</span>
      <n-popover v-model:show="pickOpen" trigger="click" placement="bottom-start">
        <template #trigger>
          <button type="button" class="hq-target" data-testid="home-quick-board">
            <span
              v-if="target"
              class="hq-picon"
              :style="{ background: hueGrad(target.project.color || '#7c5cff') }"
            />
            <span v-if="target">{{ target.project.name }} · {{ target.name }}</span>
            <span v-if="todoName" class="hq-col">· {{ todoName }}</span>
            <n-icon :component="ChevronDownOutline" :size="12" />
          </button>
        </template>
        <div class="hq-boards">
          <button
            v-for="b in boards"
            :key="b.id"
            type="button"
            class="hq-board"
            :class="{ on: b.id === boardId }"
            @click="pickBoard(b.id)"
          >
            <span class="hq-picon" :style="{ background: hueGrad(b.project.color || '#7c5cff') }" />
            <span class="hq-bproj">{{ b.project.name }}</span>
            <span>{{ b.name }}</span>
          </button>
        </div>
      </n-popover>
    </div>
  </div>
</template>

<style scoped>
.hq {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.hq-box {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 56px;
  padding: 0 16px;
  border-radius: 16px;
  background: color-mix(in srgb, var(--t-pop) 84%, transparent);
  backdrop-filter: blur(16px);
  box-shadow: var(--t-sh-pop);
  transition: box-shadow 0.15s ease;
}
.hq-box:focus-within {
  box-shadow:
    0 0 0 2px var(--t-primary),
    var(--t-sh-pop);
}
.hq-ico {
  flex: none;
  /* flat accent: a bbox-relative SVG gradient drops the «+» (straight lines have
     no height to spread it over) */
  color: var(--t-primary);
}
@media (max-width: 640px) {
  .hq-hint {
    display: none;
  }
}
.hq-input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 15.5px;
}
.hq-input::placeholder {
  color: var(--t-placeholder);
}
.hq-hint {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.hq-preview {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 8px;
  padding: 0 6px;
  font-size: 13px;
  color: var(--t-text2);
}
.hq-what b {
  color: var(--t-text1);
  font-weight: 600;
}
.hq-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 24px;
  padding: 0 9px;
  border-radius: 12px;
  font-size: 12.5px;
  font-weight: 500;
}
.hq-date {
  color: #0eb0a9;
  background: color-mix(in srgb, #0eb0a9 16%, transparent);
}
.hq-person {
  padding-left: 3px;
  color: var(--t-text1);
  background: var(--t-pill);
}
.hq-ava {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 8px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.hq-in {
  color: var(--t-text3);
}
.hq-target {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 8px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.hq-target:hover,
.hq-target:focus-visible {
  background: var(--t-hover);
}
.hq-col {
  color: var(--t-text3);
}
.hq-picon {
  flex: none;
  width: 14px;
  height: 14px;
  border-radius: 4px;
}
.hq-boards {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 240px;
  max-height: 300px;
  overflow-y: auto;
}
.hq-board {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.hq-board:hover,
.hq-board.on {
  background: var(--t-hover);
}
.hq-bproj {
  color: var(--t-text3);
}

/* A phone (#2976 — package G): a 44px field on the card level, no key hints. */
@media (max-width: 768px) {
  .hq-box {
    height: 48px;
    padding: 0 14px;
    gap: 10px;
    border-radius: 12px;
    background: var(--t-card);
    backdrop-filter: none;
    box-shadow: none;
  }
  .hq-box:focus-within {
    box-shadow: 0 0 0 2px var(--t-primary);
  }
  .hq-hint {
    display: none;
  }
  .hq-input {
    font-size: 16px;
  }
}
</style>
