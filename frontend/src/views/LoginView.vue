<script setup>
import { ref } from 'vue'
import { NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui'
import { useRouter, useRoute, RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import AuthLayout from '@/components/AuthLayout.vue'

const email = ref('')
const password = ref('')
const loading = ref(false)
const authStore = useAuthStore()
const router = useRouter()
const route = useRoute()
const message = useMessage()

async function submit() {
  loading.value = true
  try {
    await authStore.login(email.value, password.value)
    router.push(typeof route.query.next === 'string' ? route.query.next : '/')
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <auth-layout subtitle="Войдите в аккаунт">
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
    <div class="auth-foot">
      <router-link to="/forgot-password">Забыли пароль?</router-link>
    </div>
    <div class="auth-foot">Нет аккаунта? <router-link to="/register">Регистрация</router-link></div>
  </auth-layout>
</template>
