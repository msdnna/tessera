<script setup>
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { boards as boardsApi, projects as projectsApi } from '@/api'
import KanbanBoard from '@/components/KanbanBoard.vue'

const route = useRoute()
const router = useRouter()

// Boards live at /project/<projectSlug>/board/<boardSlug>. This view resolves the
// slug pair (or a legacy /board/<id> UUID/slug) to the board's UUID — which
// KanbanBoard uses for all its calls — and canonicalizes the URL to the nested,
// human-readable form whatever the entry point.
const boardId = ref(null)

async function resolve() {
  const { projectSlug, boardSlug, id } = route.params
  try {
    let board
    if (projectSlug && boardSlug) {
      board = (await boardsApi.resolve(projectSlug, boardSlug)).data
    } else if (id) {
      board = (await boardsApi.get(id)).data // legacy: UUID or bare slug
    } else {
      boardId.value = null
      return
    }
    boardId.value = board.id
    const pslug = projectSlug || (await projectsApi.get(board.project_id)).data.slug
    const target = `/project/${pslug}/board/${board.slug}`
    if (route.path !== target) router.replace({ path: target, query: route.query })
  } catch {
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
