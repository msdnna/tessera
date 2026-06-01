<script setup>
import { useRoute } from 'vue-router'
import KanbanBoard from '@/components/KanbanBoard.vue'

const route = useRoute()
</script>

<template>
  <KanbanBoard :key="route.params.id" :board-id="route.params.id" />
</template>
