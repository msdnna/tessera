<script setup>
// The start screen, «Мои задачи» (v2, #2960): the aurora over the top, a
// greeting with today's counts, the quick entry, my open tasks by due date, and
// a side column with «Входящие» (today's reminders, latest notes) and the week. The header above goes
// translucent here (AppLayout) so the aurora runs under it.
import { ref, computed, onMounted, watch, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NSpin, NButton, useMessage } from 'naive-ui'
import { CheckmarkDoneOutline, CloudOfflineOutline } from '@vicons/ionicons5'
import { workspaces as wsApi, tasks as tasksApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { usePaletteStore } from '@/stores/palette'
import { useInboxStore } from '@/stores/inbox'
import { useFormat } from '@/composables/useFormat'
import { useShortcuts } from '@/composables/useShortcuts'
import { capitalizeFirst } from '@/utils/format'
import { humanizeError } from '@/utils/errors'
import {
  SECTION_KEYS,
  mineOf,
  buildSections,
  reviewCount,
  weekBars,
  dueDay,
} from '@/utils/homeSections'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import EmptyState from '@/components/EmptyState.vue'
import HomeAurora from '@/components/home/HomeAurora.vue'
import HomeQuickEntry from '@/components/home/HomeQuickEntry.vue'
import HomeTaskList from '@/components/home/HomeTaskList.vue'
import HomeSideCards from '@/components/home/HomeSideCards.vue'

const { t } = useI18n()
const message = useMessage()
const router = useRouter()
const wsStore = useWorkspacesStore()
const auth = useAuthStore()
const palette = usePaletteStore()
// «Входящие» (#2998): the shell keeps the feed; the side block and the header
// pill read today's reminders from it.
const inbox = useInboxStore()
const { formatDate, formatTime, firstDayOfWeek, formatters } = useFormat()

const loading = ref(false)
// Non-empty = the last load was refused (#2932). Kept apart from the toast: the
// toast is gone in seconds, while the list's place has to say why it is empty.
const loadError = ref('')
const tasks = ref([])
const tags = ref([])
const members = ref([])

const meId = computed(() => auth.user?.id)
const now = ref(new Date())

// Only the newest request may write to the page. A workspace switch abandons the
// one before it, and that one is still on its way: without the comparison below
// the space the reader left decides the screen by finishing last — its tasks
// under the new space's name, and clicking a row routes to a board outside the
// current scope. Nothing refetches them, because the switch has already happened.
let loadSeq = 0
// The workspace the rows on screen were read for, so a failure can tell «this
// list is stale» from «this list is fine, only its refresh went missing».
let loadedWs = ''

// Разобрать экран: показывать нечего, и ни одна плитка больше ни за что не отвечает.
function clearPage() {
  tasks.value = []
  tags.value = []
  members.value = []
  loadedWs = ''
}

async function load() {
  const wsId = wsStore.currentId
  const mine = ++loadSeq
  if (!wsId) return
  loading.value = true
  loadError.value = ''
  try {
    const [tk, tg, mem] = await Promise.allSettled([
      wsApi.tasks(wsId, { include_subtasks: 1 }),
      wsApi.tags(wsId),
      wsApi.members(wsId),
    ])
    // A workspace switch abandoned this request: the space the reader left must
    // not decide the screen by finishing last (#2932).
    if (mine !== loadSeq) return
    const ok = (r) => (r.status === 'fulfilled' ? r.value.data || [] : [])
    if (tk.status === 'rejected') {
      // A refused task list is not «all clear»: say why in the list's place, and
      // keep this space's rows only if they are this space's (#2932).
      // The api layer hands over an Error whose message is already humanized.
      loadError.value = tk.reason?.message || humanizeError('')
      message.error(loadError.value)
      if (loadedWs !== wsId) clearPage()
      return
    }
    tasks.value = ok(tk)
    tags.value = ok(tg)
    members.value = ok(mem)
    now.value = new Date()
    loadedWs = wsId
  } finally {
    // Only the request that lit the spinner puts it out.
    if (mine === loadSeq) loading.value = false
  }
}

// ── greeting ──
const firstName = computed(
  () => (auth.user?.name || '').trim().split(/\s+/)[0] || t('shell.home.friend'),
)
const greeting = computed(() => {
  const h = now.value.getHours()
  const part =
    h >= 5 && h < 12 ? 'morning' : h >= 12 && h < 17 ? 'day' : h >= 17 ? 'evening' : 'night'
  return t(`shell.home.greet.${part}`, { name: firstName.value })
})
const dateLine = computed(() =>
  capitalizeFirst(formatDate(now.value, { weekday: 'long', day: 'numeric', month: 'long' })),
)

// ── my tasks, the sections and the counters ──
// useFormat speaks Naive's week (0 = Monday); the sections take getDay()'s (1 = Monday).
const fdow = computed(() => ((firstDayOfWeek.value ?? 0) + 1) % 7)
const mine = computed(() => mineOf(tasks.value, meId.value))
const sectionMap = computed(() =>
  buildSections(mine.value, {
    now: now.value,
    firstDayOfWeek: fdow.value,
    fmt: formatters.value,
  }),
)
const sections = computed(() =>
  SECTION_KEYS.map((key) => ({ key, tasks: sectionMap.value[key] })).filter((s) => s.tasks.length),
)
// Later and undated start folded: the screen is about today.
const folded = reactive(new Set(['later', 'nodate']))
function toggleSection(key) {
  if (folded.has(key)) folded.delete(key)
  else folded.add(key)
}
const todayReminders = computed(() => {
  const today = new Date(
    now.value.getFullYear(),
    now.value.getMonth(),
    now.value.getDate(),
  ).getTime()
  return inbox.reminders
    .filter((r) => !r.done && dueDay(r.remind_at) === today)
    .sort((a, b) => a.remind_at.localeCompare(b.remind_at))
})
const nextReminder = computed(() =>
  todayReminders.value.find((r) => Date.parse(r.remind_at) > now.value.getTime()),
)
const pills = computed(() => {
  const out = []
  const today = sectionMap.value.today.length
  const over = sectionMap.value.overdue.length
  const review = reviewCount(tasks.value, meId.value)
  if (today) out.push({ key: 'today', text: t('shell.home.pill.today', { n: today }) })
  if (over)
    out.push({ key: 'overdue', tone: '#e0533d', text: t('shell.home.pill.overdue', { n: over }) })
  if (review)
    out.push({
      key: 'review',
      tone: '#f0a020',
      text: t('shell.home.pill.review', review, { named: { n: review } }),
    })
  if (nextReminder.value)
    out.push({
      key: 'reminder',
      text: `${formatTime(nextReminder.value.remind_at)} ${nextReminder.value.message || ''}`.trim(),
      reminder: true,
    })
  return out
})
const week = computed(() =>
  weekBars(tasks.value, meId.value, { now: now.value, firstDayOfWeek: fdow.value }),
)

// ── the selected row and its keys ──
const selectedId = ref(null)
const visibleIds = computed(() =>
  sections.value.filter((s) => !folded.has(s.key)).flatMap((s) => s.tasks.map((x) => x.id)),
)
watch(visibleIds, (ids) => {
  if (!ids.length) selectedId.value = null
  else if (!ids.includes(selectedId.value)) selectedId.value = ids[0]
})
function move(dir) {
  const ids = visibleIds.value
  if (!ids.length) return
  const i = ids.indexOf(selectedId.value)
  selectedId.value = ids[Math.max(0, Math.min(ids.length - 1, (i < 0 ? -1 : i) + dir))]
  document
    .querySelector(`.hl-row[data-task-id="${selectedId.value}"]`)
    ?.scrollIntoView({ block: 'nearest' })
}
const selectedTask = () => mine.value.find((x) => x.id === selectedId.value) || null

function openTask(task) {
  router.push(`/board/${task.board_id}?task=${task.number ?? task.id}`)
}
async function complete(task) {
  try {
    await tasksApi.update(task.id, { completed: true })
    tasks.value = tasks.value.map((x) =>
      x.id === task.id ? { ...x, completed_at: new Date().toISOString() } : x,
    )
  } catch (e) {
    message.error(humanizeError(e))
  }
}
useShortcuts(
  {
    'card.up': () => move(-1),
    'card.down': () => move(1),
    'card.open': () => selectedTask() && openTask(selectedTask()),
    'card.complete': () => selectedTask() && complete(selectedTask()),
    'card.due': () => selectedTask() && palette.openTask(selectedTask().id, 'due'),
  },
  { when: () => !!visibleIds.value.length },
)

onMounted(load)
watch(() => wsStore.currentId, load)
</script>

<template>
  <div class="home" data-testid="home">
    <HomeAurora />
    <n-spin :show="loading && !tasks.length" :rotate="false" class="home-spin">
      <template #icon><TesseraSpinner /></template>
      <div class="home-inner">
        <header class="home-greet">
          <div class="hg-text">
            <div class="hg-date">{{ dateLine }}</div>
            <h1 class="hg-hello" data-testid="home-greeting">{{ greeting }}</h1>
          </div>
          <div class="hg-pills">
            <span
              v-for="p in pills"
              :key="p.key"
              class="hg-pill"
              :class="{ tone: !!p.tone, rem: p.reminder }"
              :style="p.tone ? { '--tone': p.tone } : {}"
              :data-pill="p.key"
              >{{ p.text }}</span
            >
          </div>
        </header>

        <HomeQuickEntry :members="members" :tags="tags" @created="load" />

        <div class="home-grid">
          <div class="home-main">
            <!-- A refusal is not «empty»: the list's place explains it and offers
                 a re-read (#2932). -->
            <EmptyState v-if="loadError && !loading" :icon="CloudOfflineOutline" :text="loadError">
              <n-button size="small" secondary @click="load">
                {{ t('common.action.retry') }}
              </n-button>
            </EmptyState>
            <HomeTaskList
              v-else-if="sections.length"
              :sections="sections"
              :folded="folded"
              :selected-id="selectedId"
              @select="selectedId = $event"
              @open="openTask"
              @complete="complete"
              @toggle="toggleSection"
            />
            <EmptyState
              v-else-if="!loading"
              :icon="CheckmarkDoneOutline"
              :text="t('shell.home.allClear')"
            />
          </div>
          <aside class="home-side">
            <HomeSideCards :reminders="todayReminders" :week="week" />
          </aside>
        </div>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.home {
  position: relative;
  min-height: 100%;
}
/* the aurora reaches up under the translucent header and out to the edges of
   the content's padding */
.home :deep(.h-aurora) {
  top: calc(-1 * (52px + var(--page-pad, 16px)));
  left: calc(-1 * var(--page-pad, 16px));
  right: calc(-1 * var(--page-pad, 16px));
}
.home-spin {
  position: relative;
  z-index: 1;
}
.home-inner {
  display: flex;
  flex-direction: column;
  gap: 18px;
  max-width: 1180px;
  margin: 0 auto;
  padding: 16px 24px 24px;
}
.home-greet {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}
.hg-date {
  font-size: 13.5px;
  color: var(--t-text2);
}
.hg-hello {
  margin: 4px 0 0;
  font-size: 32px;
  line-height: 1.15;
  font-weight: 700;
  letter-spacing: -0.015em;
  color: var(--t-text1);
}
.hg-pills {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.hg-pill {
  display: inline-flex;
  align-items: center;
  height: 28px;
  padding: 0 12px;
  border-radius: 14px;
  background: color-mix(in srgb, var(--t-pill) 80%, transparent);
  color: var(--t-text1);
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
}
.hg-pill.tone {
  background: color-mix(in srgb, var(--tone) 18%, transparent);
  color: var(--tone);
  font-weight: 600;
}
.hg-pill.rem {
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.home-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 340px;
  gap: 24px;
  align-items: start;
}
@media (max-width: 1100px) {
  .home-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
@media (max-width: 640px) {
  .home-inner {
    padding: 8px 4px 24px;
  }
  .hg-hello {
    font-size: 26px;
  }
}
</style>
