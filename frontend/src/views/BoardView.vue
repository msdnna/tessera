<script setup>
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { boards as boardsApi } from '@/api'
import KanbanBoard from '@/components/KanbanBoard.vue'

const route = useRoute()
const router = useRouter()

// The route param may be a human-readable slug or a (legacy) UUID. Resolve it to
// the board's UUID — which KanbanBoard uses for all its calls — and canonicalize
// the URL to the slug so it always reads cleanly, whatever the entry point.
const boardId = ref(null)

async function resolve(param) {
  if (!param) {
    boardId.value = null
    return
  }
  try {
    const { data } = await boardsApi.get(param)
    boardId.value = data.id
    if (data.slug && param !== data.slug) {
      router.replace({ path: `/board/${data.slug}`, query: route.query })
    }
  } catch {
    boardId.value = null
  }
}

watch(() => route.params.id, resolve, { immediate: true })
</script>

<template>
  <KanbanBoard v-if="boardId" :key="boardId" :board-id="boardId" />
</template>
