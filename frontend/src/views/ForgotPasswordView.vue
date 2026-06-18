<script setup>
import { ref } from 'vue'
import { NForm, NFormItem, NInput, NButton } from 'naive-ui'
import { RouterLink } from 'vue-router'
import { accountFlows } from '@/api'
import AuthLayout from '@/components/AuthLayout.vue'

const email = ref('')
const loading = ref(false)
const sent = ref(false)
const emailError = ref('')

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

async function submit() {
  const v = email.value.trim()
  if (!v) {
    emailError.value = 'Укажите email'
    return
  }
  if (!EMAIL_RE.test(v)) {
    emailError.value = 'Введите корректный email'
    return
  }
  emailError.value = ''
  loading.value = true
  try {
    await accountFlows.forgotPassword(email.value.trim())
    sent.value = true
  } catch {
    // Always treated as success (no account enumeration).
    sent.value = true
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <auth-layout subtitle="Восстановление пароля">
    <template v-if="!sent">
      <n-form @submit.prevent="submit">
        <n-form-item
          label="Email"
          :validation-status="emailError ? 'error' : undefined"
          :feedback="emailError"
        >
          <n-input
            v-model:value="email"
            placeholder="you@example.com"
            @input="emailError = ''"
            @keyup.enter="submit"
          />
        </n-form-item>
        <n-button type="primary" block :loading="loading" @click="submit"
          >Отправить ссылку</n-button
        >
      </n-form>
    </template>
    <p v-else class="note">
      Если аккаунт с таким адресом существует, мы отправили на него ссылку для сброса пароля.
    </p>
    <div class="auth-foot"><router-link to="/login">Вернуться ко входу</router-link></div>
  </auth-layout>
</template>

<style scoped>
.note {
  color: #fff;
  font-size: 14px;
  line-height: 1.5;
}
</style>
