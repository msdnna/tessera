<script setup>
import { ref } from 'vue'
import { NForm, NFormItem, NInput, NButton } from 'naive-ui'
import { useRouter, useRoute, RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import AuthLayout from '@/components/AuthLayout.vue'

const email = ref('')
const password = ref('')
const loading = ref(false)
const formError = ref('') // server / submit error, shown as a banner
const errors = ref({}) // per-field validation messages
const authStore = useAuthStore()
const router = useRouter()
const route = useRoute()

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

function validate() {
  const e = {}
  if (!email.value.trim()) e.email = 'Укажите email'
  else if (!EMAIL_RE.test(email.value.trim())) e.email = 'Введите корректный email'
  if (!password.value) e.password = 'Укажите пароль'
  errors.value = e
  return Object.keys(e).length === 0
}

async function submit() {
  formError.value = ''
  if (!validate()) return
  loading.value = true
  try {
    await authStore.login(email.value.trim(), password.value)
    router.push(typeof route.query.next === 'string' ? route.query.next : '/')
  } catch (e) {
    formError.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <auth-layout subtitle="Войдите в аккаунт">
    <div v-if="formError" class="auth-error">{{ formError }}</div>
    <n-form @submit.prevent="submit">
      <n-form-item
        label="Email"
        :validation-status="errors.email ? 'error' : undefined"
        :feedback="errors.email"
      >
        <n-input
          v-model:value="email"
          placeholder="you@example.com"
          @input="errors.email = ''"
          @keyup.enter="submit"
        />
      </n-form-item>
      <n-form-item
        label="Пароль"
        :validation-status="errors.password ? 'error' : undefined"
        :feedback="errors.password"
      >
        <n-input
          v-model:value="password"
          type="password"
          show-password-on="click"
          @input="errors.password = ''"
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
