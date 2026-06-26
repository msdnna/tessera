<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { accountFlows } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import AuthLayout from '@/components/AuthLayout.vue'

const route = useRoute()
const router = useRouter()
const ws = useWorkspacesStore()
const state = ref('pending') // pending | ok | fail

onMounted(async () => {
  const token = route.query.token
  if (!token) {
    state.value = 'fail'
    return
  }
  try {
    const res = await accountFlows.acceptInvitation(token)
    const wsId = res.data?.id
    if (wsId) {
      await ws.loadWorkspaces()
      await ws.selectWorkspace(wsId)
    }
    state.value = 'ok'
    setTimeout(() => router.push('/'), 1200)
  } catch {
    state.value = 'fail'
  }
})
</script>

<template>
  <auth-layout title="Приглашение">
    <p class="note">
      <template v-if="state === 'pending'">Принимаем приглашение…</template>
      <template v-else-if="state === 'ok'">Готово! Вы присоединились к пространству.</template>
      <template v-else
        >Приглашение недействительно, истекло или предназначено другому адресу.</template
      >
    </p>
    <div class="auth-foot"><router-link to="/">На главную</router-link></div>
  </auth-layout>
</template>

<style scoped>
.note {
  color: #fff;
  font-size: 14px;
  line-height: 1.5;
}
</style>
