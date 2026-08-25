<script setup>
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NForm, NFormItem, NInput, NButton } from 'naive-ui'
import { RouterLink } from 'vue-router'
import { accountFlows } from '@/api'
import AuthLayout from '@/components/AuthLayout.vue'

const { t } = useI18n()

const email = ref('')
const loading = ref(false)
const sent = ref(false)
const emailError = ref('')

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

async function submit() {
  const v = email.value.trim()
  if (!v) {
    emailError.value = t('common.auth.validation.emailRequired')
    return
  }
  if (!EMAIL_RE.test(v)) {
    emailError.value = t('common.auth.validation.emailInvalid')
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
  <auth-layout :title="$t('common.auth.forgot.title')">
    <template v-if="!sent">
      <n-form @submit.prevent="submit">
        <n-form-item
          :label="$t('common.auth.login.email')"
          :validation-status="emailError ? 'error' : undefined"
          :feedback="emailError"
        >
          <n-input
            v-model:value="email"
            :placeholder="$t('common.auth.login.emailPlaceholder')"
            @input="emailError = ''"
            @keyup.enter="submit"
          />
        </n-form-item>
        <n-button type="primary" block :loading="loading" @click="submit">{{
          $t('common.auth.forgot.submit')
        }}</n-button>
      </n-form>
    </template>
    <p v-else class="note">{{ $t('common.auth.forgot.sent') }}</p>
    <div class="auth-foot">
      <router-link to="/login">{{ $t('common.auth.forgot.back') }}</router-link>
    </div>
  </auth-layout>
</template>

<style scoped>
.note {
  color: #fff;
  font-size: 14px;
  line-height: 1.5;
}
</style>
