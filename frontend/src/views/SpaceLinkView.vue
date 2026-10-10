<script setup>
// /space/<workspace id> — the workspace's own link (#3077, «Ссылка на
// пространство» in the members window). A member is taken into the space; anyone
// else gets «Нет доступа» with «Запросить доступ».
//
// Asks the server rather than the sidebar's list: the list may still be loading
// on a fresh tab, and the server is the one that knows. GET /workspaces/:id
// answers 403 alike for a foreign and an unknown id, so this screen can't be
// used to learn which ids exist; the request itself answers 404 for an unknown
// one, which is no more than the requester typed.
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import NoAccessPane from '@/components/NoAccessPane.vue'

const route = useRoute()
const router = useRouter()
const wsStore = useWorkspacesStore()
const target = ref(null)

let seq = 0
async function resolve() {
  const id = String(route.params.id || '')
  const mine = ++seq
  target.value = null
  try {
    await wsApi.get(id)
    if (mine !== seq) return
    await wsStore.resync()
    await wsStore.ensureWorkspace(id)
    router.replace('/')
  } catch (e) {
    if (mine !== seq) return
    if (e?.status === 403) target.value = { workspace_id: id }
    else router.replace('/not-found')
  }
}
watch(() => route.params.id, resolve, { immediate: true })
</script>

<template>
  <div class="space-link">
    <NoAccessPane v-if="target" :target="target" />
  </div>
</template>

<style scoped>
.space-link {
  height: 100%;
}
</style>
