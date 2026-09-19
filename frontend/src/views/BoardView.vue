<script setup>
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { boards as boardsApi, projects as projectsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import KanbanBoard from '@/components/KanbanBoard.vue'

const route = useRoute()
const router = useRouter()
const wsStore = useWorkspacesStore()

// Boards live at /project/<projectSlug>/board/<boardSlug>. This view resolves the
// slug pair (or a legacy /board/<id> UUID/slug) to the board's UUID — which
// KanbanBoard uses for all its calls — and canonicalizes the URL to the nested,
// human-readable form whatever the entry point.
const boardId = ref(null)

// Which resolution the screen is waiting for (#2932). Clicking a second board
// while the first is still resolving leaves two requests in flight, and they
// come back in network order — so every step after an await asks whether it is
// still the answer being waited for. Nothing here is a harmless stale render:
// KanbanBoard is keyed on `boardId` (a remount), `ensureWorkspace` moves the
// whole app's scope, and the canonicalization REWRITES THE URL — a late answer
// for the board the reader left would take them back to it, address bar and all,
// with nothing on screen to say why. Same comparison as HomeView / NotesView /
// TaskModal.
let resolveSeq = 0

async function resolve() {
  const { projectSlug, boardSlug, id } = route.params
  const mine = ++resolveSeq
  try {
    let board
    if (projectSlug && boardSlug) {
      board = (await boardsApi.resolve(projectSlug, boardSlug)).data
    } else if (id) {
      // Legacy: UUID or bare slug. A bare slug is ambiguous (unique per project
      // only), so hand the backend the workspace we are currently in — otherwise
      // the same link can land on a same-named board of another workspace.
      board = (await boardsApi.get(id, wsStore.currentId)).data
    } else {
      boardId.value = null
      return
    }
    if (mine !== resolveSeq) return
    // Before boardId: assigning it mounts KanbanBoard, which reads currentId
    // straight away for its realtime scope filter and for the workspace members
    // it loads. Switching after would still render the board with the wrong
    // workspace's context (#2721).
    await wsStore.ensureWorkspace(board.workspace_id)
    if (mine !== resolveSeq) return
    boardId.value = board.id
    const pslug = projectSlug || (await projectsApi.get(board.project_id)).data.slug
    if (mine !== resolveSeq) return
    const target = `/project/${pslug}/board/${board.slug}`
    if (route.path !== target) router.replace({ path: target, query: route.query })
  } catch {
    // A board that failed to resolve blanks the view — but only if it is still
    // the board being asked for. An abandoned 404 must not take down the one
    // that answered in the meantime.
    if (mine !== resolveSeq) return
    boardId.value = null
  }
}

watch(() => route.params, resolve, { immediate: true, deep: true })
</script>

<template>
  <!-- Single element root: KanbanBoard is a multi-root component and the `v-if`
       collapses to a comment while resolving, so without this wrapper the view is
       a non-element root of the page <transition> (a stream of Vue warnings). The
       wrapper gives the transition one stable element to track. -->
  <div class="board-view">
    <KanbanBoard v-if="boardId" :key="boardId" :board-id="boardId" />
  </div>
</template>

<style scoped>
.board-view {
  height: 100%;
}
</style>
