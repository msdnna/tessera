<script setup>
import { ref, watch, computed } from 'vue'
import { NInput, NIcon, NSpin, NText } from 'naive-ui'
import {
  SearchOutline,
  CheckmarkCircle,
  EllipseOutline,
  DocumentTextOutline,
  DocumentsOutline,
  HelpCircleOutline,
} from '@vicons/ionicons5'
import EmptyState from '@/components/EmptyState.vue'
import { useRouter } from 'vue-router'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useHelpStore } from '@/stores/help'

const router = useRouter()
const ws = useWorkspacesStore()
const help = useHelpStore()

const q = ref('')
const open = ref(false)
const loading = ref(false)
const results = ref({ tasks: [], notes: [] })
const inputRef = ref(null)

let debounce = null
let seq = 0

watch(q, (val) => {
  clearTimeout(debounce)
  const term = val.trim()
  if (!term) {
    results.value = { tasks: [], notes: [], documents: [] }
    loading.value = false
    return
  }
  loading.value = true
  open.value = true
  debounce = setTimeout(() => runSearch(term), 220)
})

async function runSearch(term) {
  const wsId = ws.currentId
  if (!wsId) return
  const mine = ++seq
  try {
    const res = await wsApi.search(wsId, term)
    if (mine !== seq) return // a newer query landed first
    results.value = {
      tasks: res.data.tasks || [],
      notes: res.data.notes || [],
      documents: res.data.documents || [],
    }
  } catch {
    results.value = { tasks: [], notes: [], documents: [] }
  } finally {
    if (mine === seq) loading.value = false
  }
}

// Help articles are searched client-side off the bundled index (#2794), so they
// answer instantly and without the server round-trip the other groups wait for.
const helpHits = computed(() => help.find(q.value, 3))

const hasResults = () =>
  results.value.tasks.length || results.value.notes.length || results.value.documents.length

function close() {
  // delay so a result click registers before blur hides the panel
  setTimeout(() => (open.value = false), 150)
}

function gotoTask(t) {
  open.value = false
  q.value = ''
  // Prefer the readable nested path; fall back to the legacy /board/<id> route.
  const path =
    t.project_slug && t.board_slug
      ? `/project/${t.project_slug}/board/${t.board_slug}`
      : `/board/${t.board_id}`
  router.push(`${path}?task=${t.number ?? t.id}`)
}

function gotoNote(n) {
  open.value = false
  q.value = ''
  router.push(`/notes?note=${n.slug || n.id}`)
}

function gotoDocument(d) {
  open.value = false
  q.value = ''
  router.push(`/documents/${d.slug || d.id}`)
}

// A help hit opens the drawer, not the help page: the reader asked a question
// from wherever they were, and the answer should not cost them that screen.
function gotoHelp(h) {
  open.value = false
  q.value = ''
  help.openDrawer(h.slug)
}
</script>

<template>
  <div class="search" data-tour="ws-search">
    <n-input
      ref="inputRef"
      v-model:value="q"
      placeholder="Поиск задач, заметок и документов…"
      clearable
      round
      @focus="q.trim() && (open = true)"
      @blur="close"
    >
      <template #prefix>
        <n-icon :component="SearchOutline" />
      </template>
    </n-input>

    <div v-if="open && q.trim()" class="panel">
      <!-- Help sits above the server-backed groups and outside the loading
           branch: it is resolved from the bundled index, so it is already there
           while the request is still in flight (#2794). -->
      <div v-if="helpHits.length" class="grp">
        <div class="grp-h">Справка</div>
        <button
          v-for="h in helpHits"
          :key="h.slug"
          class="row"
          :data-help="h.slug"
          @mousedown.prevent="gotoHelp(h)"
        >
          <n-icon class="ico" :component="HelpCircleOutline" />
          <span class="ttl">{{ h.title }}</span>
          <span class="cat">{{ h.category }}</span>
        </button>
      </div>
      <div v-if="loading" class="loading">
        <n-spin size="small" />
      </div>
      <template v-else>
        <template v-if="hasResults()">
          <div v-if="results.tasks.length" class="grp">
            <div class="grp-h">Задачи</div>
            <button
              v-for="t in results.tasks"
              :key="t.id"
              class="row"
              @mousedown.prevent="gotoTask(t)"
            >
              <n-icon
                class="ico"
                :component="t.completed_at ? CheckmarkCircle : EllipseOutline"
                :class="{ done: t.completed_at }"
              />
              <span class="num">#{{ t.number }}</span>
              <span class="ttl">{{ t.title }}</span>
            </button>
          </div>
          <div v-if="results.notes.length" class="grp">
            <div class="grp-h">Заметки</div>
            <button
              v-for="n in results.notes"
              :key="n.id"
              class="row"
              @mousedown.prevent="gotoNote(n)"
            >
              <n-icon class="ico" :component="DocumentTextOutline" />
              <span class="ttl">{{ n.title || 'Без названия' }}</span>
            </button>
          </div>
          <div v-if="results.documents.length" class="grp">
            <div class="grp-h">Документы</div>
            <button
              v-for="d in results.documents"
              :key="d.id"
              class="row"
              @mousedown.prevent="gotoDocument(d)"
            >
              <n-icon class="ico" :component="DocumentsOutline" />
              <span class="ttl">{{ d.title || 'Без названия' }}</span>
            </button>
          </div>
        </template>
        <div v-else-if="!helpHits.length" class="empty">
          <empty-state :icon="SearchOutline" text="Ничего не найдено" size="small" />
        </div>
      </template>
      <div class="hint">
        <n-text depth="3">
          Поиск по названию и описанию задач, заголовку и тексту заметок, заголовку документов и по
          справке
        </n-text>
      </div>
    </div>
  </div>
</template>

<style scoped>
.search {
  position: relative;
  width: 100%;
}
.panel {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 10px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.18);
  padding: 6px;
  max-height: 60vh;
  overflow-y: auto;
  /* Above kanban cards: a card with subtasks is positioned at z-index 50 and
     would otherwise overlap this dropdown (a plain card stays in the auto layer
     and renders behind it). */
  z-index: 60;
}
.loading {
  display: flex;
  justify-content: center;
  padding: 16px;
}
.grp + .grp {
  margin-top: 4px;
}
.grp-h {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--t-text3);
  padding: 6px 8px 2px;
}
.row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  border: none;
  background: none;
  text-align: left;
  padding: 7px 8px;
  border-radius: 7px;
  cursor: pointer;
  color: var(--t-text1);
  font-size: 13px;
}
.row:hover {
  background: var(--t-hover);
}
.ico {
  flex: none;
  color: var(--t-text3);
}
.ico.done {
  color: var(--t-primary);
}
.num {
  flex: none;
  color: var(--t-text3);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.ttl {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* Category of a help article, pushed to the right edge of the row. */
.cat {
  flex: none;
  margin-left: auto;
  padding-left: 8px;
  color: var(--t-text3);
  font-size: 11px;
}
.empty {
  padding: 12px;
}
.hint {
  border-top: 1px solid var(--t-border);
  margin-top: 4px;
  padding: 6px 8px 2px;
  font-size: 11px;
}
</style>
