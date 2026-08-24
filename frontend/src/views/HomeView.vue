<script setup>
import { ref, computed, reactive, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NSpin } from 'naive-ui'
import { CheckmarkDoneOutline } from '@vicons/ionicons5'
import { workspaces as wsApi } from '@/api'
import EmptyState from '@/components/EmptyState.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad } from '@/utils/gradient'
import { taskColumnName } from '@/utils/defaultNames'
import { useDateLocale } from '@/composables/useDateLocale'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import TagPill from '@/components/TagPill.vue'

const { t } = useI18n()
const router = useRouter()
const wsStore = useWorkspacesStore()
const auth = useAuthStore()

const loading = ref(false)
const summary = ref(null)
const allTasks = ref([])
const tagsMap = reactive({})
// Canonical tag prefix → friendly scope name, workspace-wide (Home spans projects,
// so it can't reuse a single project's prefix map).
const prefixNames = reactive({})
const membersMap = reactive({})
const filter = ref('me') // me | all | overdue | today | week | completed

const meId = computed(() => auth.user?.id)

// Counts and colours are the fixed part; the label is resolved inside the
// computed so it follows a language switch (#2799).
const CARDS = [
  { key: 'me', field: 'assigned', accent: 'var(--t-primary)' },
  { key: 'all', field: 'active', accent: 'var(--t-text2)' },
  { key: 'overdue', field: 'overdue', accent: '#e0533d' },
  { key: 'today', field: 'due_today', accent: '#e0a418' },
  { key: 'week', field: 'due_week', accent: '#2f80ed' },
  { key: 'completed', field: 'completed', accent: '#18a058' },
]

const cards = computed(() => {
  const s = summary.value || {}
  return CARDS.map((c) => ({
    key: c.key,
    label: t(`shell.home.card.${c.key}`),
    value: s[c.field] ?? 0,
    accent: c.accent,
  }))
})

function matchesDue(task, mode) {
  if (!task.due_date) return false
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const due = new Date(task.due_date)
  const day = new Date(due.getFullYear(), due.getMonth(), due.getDate())
  if (mode === 'overdue') return day < today && !task.completed_at
  if (mode === 'today') return day.getTime() === today.getTime() && !task.completed_at
  if (mode === 'week') return day >= today && day - today <= 7 * 86400000 && !task.completed_at
  return true
}

const visibleTasks = computed(() => {
  const arr = allTasks.value
  switch (filter.value) {
    case 'me':
      return arr.filter(
        (task) => (task.assignee_ids || []).includes(meId.value) && !task.completed_at,
      )
    case 'all':
      return arr.filter((task) => !task.completed_at)
    case 'completed':
      return arr.filter((task) => task.completed_at)
    default:
      return arr.filter((task) => matchesDue(task, filter.value))
  }
})

async function load() {
  const wsId = wsStore.currentId
  if (!wsId) return
  loading.value = true
  try {
    // Destructured as `tasks`, not `t` — the short name is the translation
    // function in this file now.
    const [s, tasks, tg, pfx, mem] = await Promise.all([
      wsApi.summary(wsId),
      wsApi.tasks(wsId),
      wsApi.tags(wsId),
      wsApi.tagPrefixes(wsId),
      wsApi.members(wsId),
    ])
    summary.value = s.data
    allTasks.value = tasks.data || []
    for (const k of Object.keys(tagsMap)) delete tagsMap[k]
    for (const x of tg.data || []) tagsMap[x.id] = x
    for (const k of Object.keys(prefixNames)) delete prefixNames[k]
    for (const p of pfx.data || []) prefixNames[p.prefix] = p.label
    for (const k of Object.keys(membersMap)) delete membersMap[k]
    for (const m of mem.data || []) membersMap[m.user_id] = m
  } finally {
    loading.value = false
  }
}

function openTask(task) {
  router.push(`/board/${task.board_id}?task=${task.id}`)
}
const { formatDue: dueLabel } = useDateLocale()
function isOverdue(task) {
  return task.due_date && !task.completed_at && new Date(task.due_date) < new Date()
}

onMounted(load)
watch(() => wsStore.currentId, load)
</script>

<template>
  <n-spin :show="loading" :rotate="false">
    <template #icon><TesseraSpinner /></template>
    <div class="home">
      <h2 class="greeting">
        {{ $t('shell.home.greeting', { name: auth.user?.name || $t('shell.home.friend') }) }}
      </h2>

      <div class="cards">
        <button
          v-for="c in cards"
          :key="c.key"
          class="card"
          :class="{ active: filter === c.key }"
          :style="{ '--accent': c.accent }"
          @click="filter = c.key"
        >
          <span class="card-val">{{ c.value }}</span>
          <span class="card-label">{{ c.label }}</span>
        </button>
      </div>

      <div class="list">
        <div
          v-for="task in visibleTasks"
          :key="task.id"
          class="trow"
          :class="{ done: task.completed_at }"
          @click="openTask(task)"
        >
          <span
            class="pr-dot"
            :style="{
              background: PRIORITY_COLORS[task.priority]
                ? hueGrad(PRIORITY_COLORS[task.priority])
                : 'transparent',
            }"
          />
          <span class="t-num">#{{ task.number }}</span>
          <span class="t-title">{{ task.title }}</span>
          <TagPill
            v-for="tid in (task.tag_ids || []).filter((id) => tagsMap[id]).slice(0, 3)"
            :key="tid"
            class="t-tag"
            :tag="tagsMap[tid]"
            :prefix-names="prefixNames"
            variant="ghost"
          />
          <span class="t-loc">{{ task.project_name }} / {{ task.board_name }}</span>
          <span class="t-col" :style="{ '--c': task.column_color || 'var(--t-text3)' }">
            {{ taskColumnName(task) }}
          </span>
          <span v-if="task.due_date" class="t-due" :class="{ overdue: isOverdue(task) }">
            {{ dueLabel(task.due_date) }}
          </span>
          <span class="t-avas">
            <UserAvatar
              v-for="uid in (task.assignee_ids || []).slice(0, 3)"
              :key="uid"
              class="t-ava"
              :user-id="uid"
              :name="membersMap[uid]?.name"
              :title="membersMap[uid]?.name"
            />
          </span>
        </div>

        <empty-state
          v-if="!visibleTasks.length && !loading"
          :icon="CheckmarkDoneOutline"
          :text="$t('shell.home.empty')"
          style="margin-top: 40px"
        />
      </div>
    </div>
  </n-spin>
</template>

<style scoped>
.home {
  max-width: 1100px;
  margin: 0 auto;
  padding: 8px 4px 40px;
}
.greeting {
  font-size: 22px;
  font-weight: 700;
  color: var(--t-text1);
  margin: 4px 0 18px;
}
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  margin-bottom: 24px;
}
.card {
  --card-fill: var(--t-surface);
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 16px;
  border-radius: 12px;
  border: 1px solid var(--t-border);
  /* 3px gradient left border (of the card's accent) that wraps the rounded
     corners: transparent left border reveals the gradient on the border-box;
     the padding-box layer is the (hover/active-aware) fill. */
  border-left: 3px solid transparent;
  background:
    linear-gradient(var(--card-fill), var(--card-fill)) padding-box,
    linear-gradient(
        to top,
        color-mix(in srgb, var(--accent) 86%, #000),
        var(--accent) 50%,
        color-mix(in srgb, var(--accent) 86%, #fff)
      )
      border-box;
  cursor: pointer;
  text-align: left;
}
.card:hover {
  --card-fill: var(--t-hover);
}
.card.active {
  --card-fill: color-mix(in srgb, var(--accent) 12%, var(--t-surface));
  border-color: var(--accent);
  border-left-color: transparent;
}
.card-val {
  font-size: 26px;
  font-weight: 700;
  color: var(--t-text1);
  line-height: 1;
}
.card-label {
  font-size: 13px;
  color: var(--t-text3);
}
.list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.trow {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
}
.trow:hover {
  background: var(--t-surface-alt);
}
.trow.done .t-title {
  text-decoration: line-through;
  opacity: 0.6;
}
.pr-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
}
.t-num {
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
  flex: none;
  font-size: 12px;
}
.t-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--t-text1);
}
.t-tag {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 10px;
  flex: none;
}
.t-loc {
  font-size: 12px;
  color: var(--t-text3);
  flex: none;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.t-col {
  font-size: 11px;
  /* gradient text of the column hue (border stays a soft solid of the hue) */
  background-image: linear-gradient(
    to top right,
    color-mix(in srgb, var(--c) 86%, #000),
    var(--c) 50%,
    color-mix(in srgb, var(--c) 86%, #fff)
  );
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  color: transparent;
  border: 1px solid color-mix(in srgb, var(--c) 50%, transparent);
  border-radius: 10px;
  padding: 1px 8px;
  flex: none;
}
.t-due {
  font-size: 12px;
  color: var(--t-text3);
  flex: none;
}
.t-due.overdue {
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
.t-avas {
  display: inline-flex;
  flex: none;
}
.t-ava {
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
  margin-left: -8px;
  box-shadow: 0 0 0 2px var(--t-bg);
}
.t-ava:first-child {
  margin-left: 0;
}
@media (max-width: 768px) {
  .t-loc,
  .t-tag {
    display: none;
  }
}
</style>
