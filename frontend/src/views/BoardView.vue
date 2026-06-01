<script setup>
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import KanbanBoard from '@/components/KanbanBoard.vue'

const route = useRoute()
const openTaskId = ref(null)

// Task detail modal lands in Phase 4b; for now record the requested task id.
function onOpen(id) {
  openTaskId.value = id
}
</script>

<template>
  <KanbanBoard :key="route.params.id" :board-id="route.params.id" @open="onOpen" />
</template>
