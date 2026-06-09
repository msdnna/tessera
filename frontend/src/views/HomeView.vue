<script setup>
import { ref, computed, reactive, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { NEmpty, NSpin } from 'naive-ui'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad } from '@/utils/gradient'
import TesseraSpinner from '@/components/TesseraSpinner.vue'

const router = useRouter()
const wsStore = useWorkspacesStore()
const auth = useAuthStore()

const loading = ref(false)
const summary = ref(null)
const allTasks = ref([])
const tagsMap = reactive({})
const membersMap = reactive({})
const filter = ref('me') // me | all | overdue | today | week | completed

const meId = computed(() => auth.user?.id)

const cards = computed(() => {
  const s = summary.value || {}
  return [
    { key: 'me', label: 'Мои задачи', value: s.assigned ?? 0, accent: 'var(--t-primary)' },
    { key: 'all', label: 'Все активные', value: s.active ?? 0, accent: 'var(--t-text2)' },
    { key: 'overdue', label: 'Просрочено', value: s.overdue ?? 0, accent: '#e0533d' },
    { key: 'today', label: 'Сегодня', value: s.due_today ?? 0, accent: '#e0a418' },
    { key: 'week', label: 'На неделе', value: s.due_week ?? 0, accent: '#2f80ed' },
    { key: 'completed', label: 'Выполнено', value: s.completed ?? 0, accent: '#18a058' },
  ]
})

function matchesDue(t, mode) {
  if (!t.due_date) return false
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const due = new Date(t.due_date)
  const day = new Date(due.getFullYear(), due.getMonth(), due.getDate())
  if (mode === 'overdue') return day < today && !t.completed_at
  if (mode === 'today') return day.getTime() === today.getTime() && !t.completed_at
  if (mode === 'week') return day >= today && day - today <= 7 * 86400000 && !t.completed_at
  return true
}

const visibleTasks = computed(() => {
  const arr = allTasks.value
  switch (filter.value) {
    case 'me':
      return arr.filter((t) => (t.assignee_ids || []).includes(meId.value) && !t.completed_at)
    case 'all':
      return arr.filter((t) => !t.completed_at)
    case 'completed':
      return arr.filter((t) => t.completed_at)
    default:
      return arr.filter((t) => matchesDue(t, filter.value))
  }
})

async function load() {
  const wsId = wsStore.currentId
  if (!wsId) return
  loading.value = true
  try {
    const [s, t, tg, mem] = await Promise.all([
      wsApi.summary(wsId),
      wsApi.tasks(wsId),
      wsApi.tags(wsId),
      wsApi.members(wsId),
    ])
    summary.value = s.data
    allTasks.value = t.data || []
    for (const k of Object.keys(tagsMap)) delete tagsMap[k]
    for (const x of tg.data || []) tagsMap[x.id] = x
    for (const k of Object.keys(membersMap)) delete membersMap[k]
    for (const m of mem.data || []) membersMap[m.user_id] = m
  } finally {
    loading.value = false
  }
}

function openTask(t) {
  router.push(`/board/${t.board_id}?task=${t.id}`)
}
function initials(name) {
  return (name || '?').trim().slice(0, 2).toUpperCase()
}
function dueLabel(d) {
  return new Date(d).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short' })
}
function isOverdue(t) {
  return t.due_date && !t.completed_at && new Date(t.due_date) < new Date()
}

onMounted(load)
watch(() => wsStore.currentId, load)
</script>

<template>
  <n-spin :show="loading" :rotate="false">
    <template #icon><TesseraSpinner /></template>
    <div class="home">
      <h2 class="greeting">Привет, {{ auth.user?.name || 'друг' }} 👋</h2>

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
          v-for="t in visibleTasks"
          :key="t.id"
          class="trow"
          :class="{ done: t.completed_at }"
          @click="openTask(t)"
        >
          <span
            class="pr-dot"
            :style="{
              background: PRIORITY_COLORS[t.priority] ? hueGrad(PRIORITY_COLORS[t.priority]) : 'transparent',
            }"
          />
          <span class="t-num">#{{ t.number }}</span>
          <span class="t-title">{{ t.title }}</span>
          <span
            v-for="tid in (t.tag_ids || []).slice(0, 3)"
            :key="tid"
            class="t-tag"
            :style="{
              background: (tagsMap[tid]?.color || '#888') + '22',
              color: tagsMap[tid]?.color || '#888',
            }"
          >
            {{ tagsMap[tid]?.name }}
          </span>
          <span class="t-loc">{{ t.project_name }} / {{ t.board_name }}</span>
          <span class="t-col" :style="{ '--c': t.column_color || 'var(--t-text3)' }">
            {{ t.column_name }}
          </span>
          <span v-if="t.due_date" class="t-due" :class="{ overdue: isOverdue(t) }">
            {{ dueLabel(t.due_date) }}
          </span>
          <span class="t-avas">
            <span
              v-for="uid in (t.assignee_ids || []).slice(0, 3)"
              :key="uid"
              class="t-ava"
              :title="membersMap[uid]?.name"
            >
              {{ initials(membersMap[uid]?.name) }}
            </span>
          </span>
        </div>

        <n-empty
          v-if="!visibleTasks.length && !loading"
          description="Здесь пока пусто"
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
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 16px;
  border-radius: 12px;
  border: 1px solid var(--t-border);
  background: var(--t-surface);
  cursor: pointer;
  text-align: left;
  border-left: 3px solid var(--accent);
  transition: background 0.12s;
}
.card:hover {
  background: var(--t-hover);
}
.card.active {
  background: color-mix(in srgb, var(--accent) 12%, var(--t-surface));
  border-color: var(--accent);
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
  color: var(--c);
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
  color: #e0533d;
  font-weight: 600;
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
