<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { NIcon, NButton, NTooltip, useMessage } from 'naive-ui'
import {
  RibbonOutline,
  LogoGitlab,
  SettingsOutline,
  CheckmarkCircleOutline,
  ArrowForwardOutline,
} from '@vicons/ionicons5'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { milestoneKey, milestoneRange } from '@/utils/milestones'
import { useFormat } from '@/composables/useFormat'
import { formatEstimate } from '@/utils/estimation'
import EmptyState from '@/components/EmptyState.vue'
import LoaderOverlay from '@/components/LoaderOverlay.vue'
import MilestoneManager from '@/components/MilestoneManager.vue'

const router = useRouter()
const store = useWorkspacesStore()
const { formatters } = useFormat()
const message = useMessage()

const list = ref([])
const loading = ref(false)
const stateFilter = ref('active') // 'active' | 'all'

// Per-project «Управление этапами» modal (reuses the existing manager).
const mgr = ref({ show: false, projectId: null, projectName: '' })

async function load() {
  const ws = store.currentId
  if (!ws) {
    list.value = []
    return
  }
  loading.value = true
  try {
    const { data } = await wsApi.milestones(ws)
    list.value = data || []
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

// Apply the active/all filter, then group by project preserving server order
// (project_name, then milestone position).
const groups = computed(() => {
  const out = []
  const byId = new Map()
  for (const m of list.value) {
    if (stateFilter.value === 'active' && m.state === 'closed') continue
    let g = byId.get(m.project_id)
    if (!g) {
      g = {
        projectId: m.project_id,
        projectName: m.project_name,
        projectSlug: m.project_slug,
        boardSlug: m.board_slug,
        milestones: [],
      }
      byId.set(m.project_id, g)
      out.push(g)
    }
    g.milestones.push(m)
  }
  return out
})

const total = computed(() => groups.value.reduce((n, g) => n + g.milestones.length, 0))

function isLinked(m) {
  return !!m.gl_global_id
}
function pct(m) {
  if (!m.task_count) return 0
  return Math.round((m.done_count / m.task_count) * 100)
}
function estimateLabel(m) {
  if (!(m.estimate_sum > 0)) return ''
  return formatEstimate(m.estimate_sum, store.estimationFor(m.project_id))
}

function openBoard(g, m) {
  if (!g.boardSlug || !g.projectSlug) return
  router.push({
    path: `/project/${g.projectSlug}/board/${g.boardSlug}`,
    query: { milestone: milestoneKey(m) },
  })
}

function manage(g) {
  mgr.value = { show: true, projectId: g.projectId, projectName: g.projectName }
}

watch(() => store.currentId, load)
onMounted(load)
</script>

<template>
  <div class="ms-screen">
    <header class="ms-head">
      <h1 class="ms-title">
        <n-icon :component="RibbonOutline" class="grad-icon" :size="22" /> Этапы
      </h1>
      <div class="ms-seg">
        <n-button
          size="small"
          :type="stateFilter === 'active' ? 'primary' : 'default'"
          :tertiary="stateFilter !== 'active'"
          @click="stateFilter = 'active'"
        >
          Активные
        </n-button>
        <n-button
          size="small"
          :type="stateFilter === 'all' ? 'primary' : 'default'"
          :tertiary="stateFilter !== 'all'"
          @click="stateFilter = 'all'"
        >
          Все
        </n-button>
      </div>
    </header>

    <loader-overlay :show="loading" contained />

    <empty-state
      v-if="!loading && !total"
      :icon="RibbonOutline"
      :text="
        stateFilter === 'active'
          ? 'Активных этапов нет — создайте их в проекте или переключитесь на «Все»'
          : 'Этапов пока нет — создайте первый из контекстного меню проекта'
      "
    />

    <div v-else-if="total" class="ms-groups">
      <section v-for="g in groups" :key="g.projectId" class="ms-group">
        <div class="ms-group-head">
          <span class="ms-proj">{{ g.projectName }}</span>
          <span class="ms-count">{{ g.milestones.length }}</span>
          <span class="ms-spacer" />
          <n-tooltip>
            <template #trigger>
              <n-button text size="small" class="ms-manage" @click="manage(g)">
                <n-icon :component="SettingsOutline" />
              </n-button>
            </template>
            Управление этапами проекта
          </n-tooltip>
        </div>

        <div
          v-for="m in g.milestones"
          :key="m.id"
          class="ms-row"
          :class="{ closed: m.state === 'closed', clickable: !!g.boardSlug }"
          @click="openBoard(g, m)"
        >
          <div class="ms-main">
            <div class="ms-line1">
              <span class="ms-name">{{ m.title }}</span>
              <n-tooltip v-if="isLinked(m)">
                <template #trigger>
                  <a
                    class="ms-gl"
                    :href="m.gl_url"
                    target="_blank"
                    rel="noopener noreferrer"
                    @click.stop
                  >
                    <n-icon :component="LogoGitlab" />
                  </a>
                </template>
                Синхронизируется с GitLab
              </n-tooltip>
              <span v-if="milestoneRange(m, formatters)" class="ms-range">{{
                milestoneRange(m, formatters)
              }}</span>
            </div>
            <div class="ms-line2">
              <template v-if="m.task_count">
                <div class="ms-bar">
                  <div class="ms-bar-fill" :style="{ width: pct(m) + '%' }" />
                </div>
                <span class="ms-stat">
                  <n-icon :component="CheckmarkCircleOutline" :size="13" />
                  {{ m.done_count }}/{{ m.task_count }}
                </span>
              </template>
              <span v-else class="ms-stat muted">нет задач</span>
              <span v-if="estimateLabel(m)" class="ms-est">Σ {{ estimateLabel(m) }}</span>
            </div>
          </div>
          <n-icon v-if="g.boardSlug" :component="ArrowForwardOutline" class="ms-go" :size="16" />
        </div>
      </section>
    </div>

    <milestone-manager
      v-model:show="mgr.show"
      :project-id="mgr.projectId"
      :project-name="mgr.projectName"
      :ws-id="store.currentId"
      @changed="load"
    />
  </div>
</template>

<style scoped>
.ms-screen {
  max-width: 920px;
  margin: 0 auto;
  position: relative;
}
.ms-head {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 18px;
}
.ms-title {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-size: 20px;
  font-weight: 700;
  margin: 0;
  color: var(--t-text1);
}
.ms-seg {
  display: flex;
  gap: 6px;
  margin-left: auto;
}
.ms-groups {
  display: flex;
  flex-direction: column;
  gap: 22px;
}
.ms-group-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--t-border);
}
.ms-proj {
  font-weight: 600;
  font-size: 14px;
  color: var(--t-text1);
}
.ms-count {
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-hover);
  border-radius: 10px;
  padding: 1px 8px;
}
.ms-spacer {
  flex: 1;
}
.ms-manage {
  color: var(--t-text3);
}
.ms-manage:hover {
  color: var(--t-primary);
}
.ms-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 8px;
  margin-bottom: 8px;
  transition:
    border-color 0.15s,
    transform 0.1s;
}
.ms-row.clickable {
  cursor: pointer;
}
.ms-row.clickable:hover {
  border-color: color-mix(in srgb, var(--t-primary) 55%, var(--t-border));
}
.ms-row.closed .ms-name {
  opacity: 0.6;
  text-decoration: line-through;
}
.ms-main {
  flex: 1;
  min-width: 0;
}
.ms-line1 {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.ms-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--t-text1);
}
.ms-gl {
  display: inline-flex;
  color: #fc6d26;
  font-size: 15px;
}
.ms-range {
  font-size: 12px;
  color: var(--t-text3);
}
.ms-line2 {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 7px;
}
.ms-bar {
  flex: 0 1 180px;
  height: 6px;
  border-radius: 3px;
  background: var(--t-hover);
  overflow: hidden;
}
.ms-bar-fill {
  height: 100%;
  border-radius: 3px;
  background: var(--t-accent-grad);
  transition: width 0.2s;
}
.ms-stat {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--t-text2);
}
.ms-stat.muted {
  color: var(--t-text3);
}
.ms-est {
  font-size: 12px;
  color: var(--t-text3);
}
.ms-go {
  color: var(--t-text3);
  flex: none;
}
.ms-row.clickable:hover .ms-go {
  color: var(--t-primary);
}
</style>
