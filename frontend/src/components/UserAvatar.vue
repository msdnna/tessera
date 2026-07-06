<script setup>
import { computed, ref, watch } from 'vue'
import { initials } from '@/utils/initials'
import { apiBaseURL } from '@/utils/serverBase'
import { useApiImage } from '@/composables/useApiImage'

// A circular user avatar: shows the uploaded image (Tessera user by id, or an
// explicit src for GitLab users), falling back to gradient initials on miss/error.
// Inherits the caller's class (e.g. `.avatar`) for size / ring / gradient bg.
const props = defineProps({
  userId: { type: String, default: '' },
  name: { type: String, default: '' },
  src: { type: String, default: '' },
})

const failed = ref(false)
// The source URL: an explicit src (e.g. GitLab avatar proxy '/api/gitlab/avatar?…')
// or the Tessera user avatar endpoint. useApiImage resolves it directly on web,
// or via an axios-fetched blob: URL on desktop (the webview can't load remote <img>).
const rawUrl = computed(() =>
  props.src ? props.src : props.userId ? `${apiBaseURL()}/users/${props.userId}/avatar` : '',
)
const url = useApiImage(rawUrl)
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
