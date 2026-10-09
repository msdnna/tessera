<script setup>
// The task's row menu (#3112) drawn by kit/ActionMenu: the state is the
// caller's `useTaskMenu()` — this only binds it. The title is «#N название».
import ActionMenu from '@/components/kit/ActionMenu.vue'

defineProps({
  menu: { type: Object, required: true },
  testid: { type: String, default: 'task-menu' },
})
</script>

<template>
  <ActionMenu
    :show="menu.show.value"
    :anchor="menu.anchor"
    :placement="menu.placement.value"
    :offset="menu.placement.value === 'point' ? { x: 2, y: 2 } : { x: 0, y: 4 }"
    :width="280"
    :header="menu.header.value"
    :items="menu.options.value"
    :testid="testid"
    @update:show="menu.setShow"
    @select="menu.select"
  />
</template>
