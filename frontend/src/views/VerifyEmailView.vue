<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { accountFlows } from '@/api'
import AuthLayout from '@/components/AuthLayout.vue'

const route = useRoute()
const state = ref('pending') // pending | ok | fail

onMounted(async () => {
  const token = route.query.token
  if (!token) {
    state.value = 'fail'
    return
  }
  try {
    await accountFlows.verifyEmail(token)
    state.value = 'ok'
  } catch {
    state.value = 'fail'
  }
})
</script>

<template>
  <auth-layout subtitle="Подтверждение почты">
    <p class="note">
      <template v-if="state === 'pending'">Подтверждаем…</template>
      <template v-else-if="state === 'ok'">Почта подтверждена. Спасибо!</template>
      <template v-else>Ссылка недействительна или устарела.</template>
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
