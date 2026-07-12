<script setup>
import { ref, onMounted } from 'vue'
import { NForm, NFormItem, NInput, NButton, NIcon } from 'naive-ui'
import { LogoGitlab } from '@vicons/ionicons5'
import { useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { auth } from '@/api'
import AuthLayout from '@/components/AuthLayout.vue'

const email = ref('')
const name = ref('')
const password = ref('')
const loading = ref(false)
const formError = ref('')
const errors = ref({})
const authStore = useAuthStore()
const router = useRouter()

const gitlabEnabled = ref(false)
onMounted(async () => {
  try {
    const { data } = await auth.providers()
    gitlabEnabled.value = data?.gitlab === true
  } catch {
    gitlabEnabled.value = false
  }
})
function registerWithGitlab() {
  window.location.href = auth.gitlabAuthorizeUrl()
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

function validate() {
  const e = {}
  if (!name.value.trim()) e.name = 'Укажите имя'
  if (!email.value.trim()) e.email = 'Укажите email'
  else if (!EMAIL_RE.test(email.value.trim())) e.email = 'Введите корректный email'
  if (!password.value) e.password = 'Укажите пароль'
  else if (password.value.length < 8) e.password = 'Минимум 8 символов'
  errors.value = e
  return Object.keys(e).length === 0
}

async function submit() {
  formError.value = ''
  if (!validate()) return
  loading.value = true
  try {
    await authStore.register(email.value.trim(), name.value.trim(), password.value)
    router.push('/')
  } catch (e) {
    formError.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <auth-layout title="Регистрация">
    <div v-if="formError" class="auth-error">{{ formError }}</div>
    <n-form @submit.prevent="submit">
      <n-form-item
        label="Имя"
        :validation-status="errors.name ? 'error' : undefined"
        :feedback="errors.name"
      >
        <n-input v-model:value="name" placeholder="Ваше имя" @input="errors.name = ''" />
      </n-form-item>
      <n-form-item
        label="Email"
        :validation-status="errors.email ? 'error' : undefined"
        :feedback="errors.email"
      >
        <n-input
          v-model:value="email"
          placeholder="you@example.com"
          @input="errors.email = ''"
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
          placeholder="минимум 8 символов"
          @input="errors.password = ''"
          @keyup.enter="submit"
        />
      </n-form-item>
      <n-button type="primary" block :loading="loading" @click="submit">Создать аккаунт</n-button>
    </n-form>
    <template v-if="gitlabEnabled">
      <div class="auth-or"><span>или</span></div>
      <n-button block @click="registerWithGitlab">
        <template #icon><n-icon :component="LogoGitlab" /></template>
        Продолжить с GitLab
      </n-button>
    </template>
    <div class="auth-foot">Уже есть аккаунт? <router-link to="/login">Вход</router-link></div>
  </auth-layout>
</template>

<style scoped>
.auth-or {
  display: flex;
  align-items: center;
  text-align: center;
  margin: 14px 0 12px;
  color: var(--t-text-3, #8a8a99);
  font-size: 12px;
}
.auth-or::before,
.auth-or::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--t-border, rgba(140, 140, 160, 0.25));
}
.auth-or span {
  padding: 0 10px;
}
</style>
