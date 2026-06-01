<script setup>
import { ref } from 'vue'
import { NCard, NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui'
import { useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const email = ref('')
const password = ref('')
const loading = ref(false)
const authStore = useAuthStore()
const router = useRouter()
const message = useMessage()

async function submit() {
  loading.value = true
  try {
    await authStore.login(email.value, password.value)
    router.push('/')
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="center">
    <n-card title="Вход в Tessera" style="max-width: 380px">
      <n-form @submit.prevent="submit">
        <n-form-item label="Email">
          <n-input v-model:value="email" placeholder="you@example.com" />
        </n-form-item>
        <n-form-item label="Пароль">
          <n-input
            v-model:value="password"
            type="password"
            show-password-on="click"
            @keyup.enter="submit"
          />
        </n-form-item>
        <n-button type="primary" block :loading="loading" @click="submit">Войти</n-button>
      </n-form>
      <template #footer>
        Нет аккаунта? <router-link to="/register">Регистрация</router-link>
      </template>
    </n-card>
  </div>
</template>

<style scoped>
.center {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
