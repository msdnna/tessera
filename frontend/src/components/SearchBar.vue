<script setup>
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NInput, NIcon, NSpin } from 'naive-ui'
import {
  SearchOutline,
  CheckmarkCircle,
  EllipseOutline,
  DocumentTextOutline,
  DocumentsOutline,
  HelpCircleOutline,
  AlertCircleOutline,
} from '@vicons/ionicons5'
import EmptyState from '@/components/EmptyState.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import { usePaletteStore } from '@/stores/palette'
import { useRouter } from 'vue-router'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useInboxStore } from '@/stores/inbox'
import { useHelpStore } from '@/stores/help'
import { useBoardViewStore } from '@/stores/boardView'

const { t } = useI18n()
const router = useRouter()
const inbox = useInboxStore()
const ws = useWorkspacesStore()
const help = useHelpStore()
const board = useBoardViewStore()

const q = ref('')
const open = ref(false)
const loading = ref(false)
const results = ref({ tasks: [], notes: [], documents: [] })
// Which kinds the server had to cut short. Sent per kind because a term can fill
// the task page while matching two notes, and a blanket "there is more" would
// then point at the wrong list.
const more = ref({ tasks: false, notes: false, documents: false })
// Why the last search produced nothing, when the reason was not "nothing
// matched". Held separately from `results` so the panel can tell the two apart:
// see the template, where an empty result set with an error is a failure notice
// and without one is "нашлось ничего".
const error = ref('')
const inputRef = ref(null)

const EMPTY = () => ({ tasks: [], notes: [], documents: [] })
const NO_MORE = () => ({ tasks: false, notes: false, documents: false })

const DEBOUNCE_MS = 220

let debounce = null
let seq = 0

// On a board (v2, #2954) this field is also the board's title filter: typing
// narrows the cards right away, and the workspace-wide results only drop down
// when nothing on the board matches. Off a board it is the global search alone.
const onBoard = computed(() => board.active)
const boardHasMatches = computed(() => onBoard.value && board.shownTasks > 0)
const showPanel = computed(() => open.value && !!q.value.trim() && !boardHasMatches.value)
// The board can change the filter itself (a saved view brings its own), and
// moving to another board starts clean.
watch(
  () => board.searchQ,
  (v) => {
    if (onBoard.value && v !== q.value) q.value = v
  },
)
watch(
  () => board.boardId,
  () => {
    q.value = ''
  },
)

watch(q, (val) => {
  if (onBoard.value) board.searchQ = val
  clearTimeout(debounce)
  const term = val.trim()
  if (!term) {
    results.value = EMPTY()
    more.value = NO_MORE()
    error.value = ''
    loading.value = false
    return
  }
  loading.value = true
  open.value = true
  debounce = setTimeout(() => runSearch(term), DEBOUNCE_MS)
})

// The search box outlives a workspace switch: it sits in the topbar and the shell
// swaps `ws.currentId` without remounting it, so hits fetched for the workspace
// the reader has left had nothing to invalidate them. Clicking the switcher blurs
// the input and closes the panel, but the typed term stays in the box — and
// focusing it again re-opens the panel over those hits (`@focus` only needs a
// non-empty term) without asking anything of the server, since the watcher above
// fires on keystrokes alone. The rows then point into the other workspace: opening
// one navigates to a board this workspace does not contain, and the board resolves
// `?task=<number>` against the *current* workspace, fails, and leaves the reader
// on a stranger's board with no explanation.
//
// Android has never had this — SearchViewModel.bind() drops the whole state when
// the workspace id changes — so the divergence was web's alone (#2932).
watch(
  () => ws.currentId,
  () => {
    // Invalidate the request in flight before anything else: it was asked of the
    // previous workspace, and without this its answer lands afterwards and
    // repopulates the panel we are clearing.
    seq++
    clearTimeout(debounce)
    results.value = EMPTY()
    more.value = NO_MORE()
    error.value = ''
    const term = q.value.trim()
    if (!term) {
      // Nothing typed: the bumped `seq` above also means the in-flight request's
      // `finally` will not clear the spinner, so clear it here.
      loading.value = false
      return
    }
    // Re-ask, rather than clearing the box: the term is still on screen, and an
    // empty panel under it would read as "nothing matched" — a statement about
    // the new workspace that nobody has checked.
    loading.value = true
    debounce = setTimeout(() => runSearch(term), DEBOUNCE_MS)
  },
)

async function runSearch(term) {
  // The sequence number is taken before the first early return, not after: a
  // request already in flight has to be invalidated even by a call that goes on
  // to do nothing, or its answer lands later and repopulates the panel.
  const mine = ++seq
  const wsId = ws.currentId
  // Typed before the workspace list arrived — the search box is in the topbar
  // and is mountable well before the shell knows where to look. Nothing to ask,
  // but the spinner the watcher just turned on is ours to turn off; leaving it
  // spinning is a panel that never resolves, since nothing retries until the
  // next keystroke.
  if (!wsId) {
    loading.value = false
    return
  }
  try {
    const res = await wsApi.search(wsId, term)
    if (mine !== seq) return // a newer query landed first
    results.value = {
      tasks: res.data.tasks || [],
      notes: res.data.notes || [],
      documents: res.data.documents || [],
    }
    more.value = { ...NO_MORE(), ...(res.data.has_more || {}) }
    error.value = ''
  } catch (e) {
    if (mine !== seq) return
    results.value = EMPTY()
    more.value = NO_MORE()
    // The message is already in the reader's language: the api interceptor runs
    // every failure through humanizeError before rejecting. The fallback covers
    // a throw from somewhere else that carries no message at all.
    error.value = e?.message || t('shell.search.failed')
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
  // Notes live in the «Входящие» panel now (#2999): open it on this one.
  inbox.reveal('note', n.id)
}

function gotoDocument(d) {
  open.value = false
  q.value = ''
  router.push(`/documents/${d.slug || d.id}`)
}

// The ⌘K palette (#2957) is the search everywhere; this field stays the board's
// title filter. Off a board there is nothing to filter, so the field is just the
// palette's doorway: focusing it opens the palette.
const palette = usePaletteStore()
function openPalette() {
  const term = q.value.trim()
  open.value = false
  if (onBoard.value) q.value = ''
  palette.show(term)
}
function onFocus() {
  if (!onBoard.value) {
    inputRef.value?.blur()
    openPalette()
    return
  }
  if (q.value.trim()) open.value = true
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
      :placeholder="t(onBoard ? 'shell.search.placeholderBoard' : 'shell.search.placeholder')"
      :input-props="{ 'data-testid': 'header-search' }"
      clearable
      @focus="onFocus"
      @blur="close"
      @keydown.esc="q = ''"
    >
      <template #prefix>
        <n-icon :component="SearchOutline" />
      </template>
      <template #suffix>
        <ShortcutKeys v-if="!q" id="nav.palette" joined soft class="search-kbd" />
      </template>
    </n-input>

    <div v-if="showPanel" class="panel">
      <!-- On a board the cards are filtered first; this panel only opens when
           nothing on the board matched, so say where the results come from. -->
      <div v-if="onBoard" class="board-none">{{ t('shell.search.boardNone') }}</div>
      <!-- Help sits above the server-backed groups and outside the loading
           branch: it is resolved from the bundled index, so it is already there
           while the request is still in flight (#2794). -->
      <div v-if="helpHits.length" class="grp">
        <div class="grp-h">{{ t('shell.search.help') }}</div>
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
        <!-- A failed request is not an empty result, and it goes first: before
             this branch existed the panel drew "Ничего не найдено" over a 500,
             an expired session and a dropped connection alike, so the reader
             concluded the thing they were looking for did not exist (#2932). -->
        <div v-if="error" class="failed">
          <empty-state :icon="AlertCircleOutline" :text="error" size="small" />
        </div>
        <template v-else-if="hasResults()">
          <div v-if="results.tasks.length" class="grp">
            <div class="grp-h">{{ t('shell.search.tasks') }}</div>
            <!-- `task`, not `t`: the loop variable would shadow the translation
                 function for the whole template. -->
            <button
              v-for="task in results.tasks"
              :key="task.id"
              class="row"
              @mousedown.prevent="gotoTask(task)"
            >
              <n-icon
                class="ico"
                :component="task.completed_at ? CheckmarkCircle : EllipseOutline"
                :class="{ done: task.completed_at }"
              />
              <span class="num">#{{ task.number }}</span>
              <span class="ttl">{{ task.title }}</span>
            </button>
            <!-- The count comes from the rendered list rather than from a
                 constant: when the server says it truncated, what it handed back
                 *is* the page size, and a hard-coded 25 here would quietly start
                 lying the day the backend's changes. -->
            <div v-if="more.tasks" class="more">
              {{ t('shell.search.more', { n: results.tasks.length }) }}
            </div>
          </div>
          <div v-if="results.notes.length" class="grp">
            <div class="grp-h">{{ t('shell.search.notes') }}</div>
            <button
              v-for="n in results.notes"
              :key="n.id"
              class="row"
              @mousedown.prevent="gotoNote(n)"
            >
              <n-icon class="ico" :component="DocumentTextOutline" />
              <span class="ttl">{{ n.title || t('shell.search.untitled') }}</span>
            </button>
            <div v-if="more.notes" class="more">
              {{ t('shell.search.more', { n: results.notes.length }) }}
            </div>
          </div>
          <div v-if="results.documents.length" class="grp">
            <div class="grp-h">{{ t('shell.search.documents') }}</div>
            <button
              v-for="d in results.documents"
              :key="d.id"
              class="row"
              @mousedown.prevent="gotoDocument(d)"
            >
              <n-icon class="ico" :component="DocumentsOutline" />
              <span class="ttl">{{ d.title || t('shell.search.untitled') }}</span>
            </button>
            <div v-if="more.documents" class="more">
              {{ t('shell.search.more', { n: results.documents.length }) }}
            </div>
          </div>
        </template>
        <div v-else-if="!helpHits.length" class="empty">
          <empty-state
            :icon="SearchOutline"
            :title="t('shell.search.nothing')"
            :text="t('shell.search.nothingHint')"
            size="small"
          />
        </div>
      </template>
      <!-- Everything else (boards, people, commands) lives in the palette. -->
      <button
        class="row everywhere"
        data-testid="search-everywhere"
        @mousedown.prevent="openPalette"
      >
        <n-icon class="ico" :component="SearchOutline" />
        <span class="ttl">{{ t('shell.search.everywhere') }}</span>
        <ShortcutKeys id="nav.palette" />
      </button>
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
  background: var(--t-pop);
  border-radius: var(--t-r-tip);
  box-shadow: var(--t-sh-pop);
  padding: 6px;
  max-height: 60vh;
  overflow-y: auto;
  /* Above kanban cards: a card with subtasks is positioned at z-index 50 and
     would otherwise overlap this dropdown (a plain card stays in the auto layer
     and renders behind it). */
  z-index: 60;
}
.board-none {
  padding: 6px 8px 4px;
  font-size: 12px;
  color: var(--t-text3);
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
.empty,
.failed {
  padding: 12px;
}
/* The truncation note sits under its own group, indented to the rows' text so it
   reads as a footer of that list rather than as another result. */
.more {
  padding: 4px 8px 2px 32px;
  color: var(--t-text3);
  font-size: 11px;
}
/* v2: no divider over the hint — the gap and the dimmer text set it apart. */
.hint,
.row.everywhere {
  margin-top: 4px;
  color: var(--t-text2);
}
.row.everywhere .ttl {
  flex: 1;
}
/* The search field sits right on the canvas: a filled 34px well (v2, #2953). */
.search :deep(.n-input) {
  --n-height: 34px !important;
}
.search-kbd {
  margin-right: -2px;
}
/* No keyboard to press it on: drop the hint on phones/touch screens. */
@media (max-width: 768px), (hover: none) {
  .search-kbd {
    display: none;
  }
}
</style>
