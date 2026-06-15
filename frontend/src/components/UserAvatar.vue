<script setup>
import { computed, ref, watch } from 'vue'
import { initials } from '@/utils/initials'

// A circular user avatar: shows the uploaded image (Tessera user by id, or an
// explicit src for GitLab users), falling back to gradient initials on miss/error.
// Inherits the caller's class (e.g. `.avatar`) for size / ring / gradient bg.
const props = defineProps({
  userId: { type: String, default: '' },
  name: { type: String, default: '' },
  src: { type: String, default: '' },
})

const failed = ref(false)
const url = computed(() => props.src || (props.userId ? `/api/users/${props.userId}/avatar` : ''))
watch(url, () => (failed.value = false))
</script>

<template>
  <span class="ua">
    <img v-if="url && !failed" :src="url" class="ua-img" alt="" @error="failed = true" />
    <template v-else>{{ initials(name) }}</template>
  </span>
</template>

<style scoped>
.ua {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}
.ua-img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
}
</style>
