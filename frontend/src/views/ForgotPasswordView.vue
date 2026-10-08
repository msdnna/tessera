<script setup>
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NForm, NFormItem, NInput, NButton, NIcon } from 'naive-ui'
import { KeyOutline, MailUnreadOutline, ArrowBackOutline } from '@vicons/ionicons5'
import { RouterLink } from 'vue-router'
import { accountFlows } from '@/api'
import AuthLayout from '@/components/AuthLayout.vue'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'

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
  <auth-layout
    :title="sent ? $t('common.auth.forgot.sentTitle') : $t('common.auth.forgot.title')"
    :subtitle="sent ? '' : $t('common.auth.forgot.subtitle')"
    :icon="sent ? MailUnreadOutline : KeyOutline"
    :tone="sent ? '#2f80ed' : '#f0a020'"
  >
    <n-form v-if="!sent" @submit.prevent="submit">
      <n-form-item
        :label="$t('common.auth.login.email')"
        :validation-status="emailError ? 'error' : undefined"
        :feedback="emailError"
      >
        <n-input
          v-model:value="email"
          :placeholder="$t('common.auth.login.emailPlaceholder')"
          :input-props="{ autocomplete: 'email' }"
          data-testid="forgot-email"
          @input="emailError = ''"
          @keyup.enter="submit"
        />
      </n-form-item>
      <n-button
        type="primary"
        size="large"
        block
        class="auth-submit"
        data-testid="forgot-submit"
        :loading="loading"
        @click="submit"
      >
        {{ $t('common.auth.forgot.submit') }}<Kbd>{{ CAPS.enter }}</Kbd>
      </n-button>
    </n-form>
    <p v-else class="note" data-testid="forgot-sent">{{ $t('common.auth.forgot.sent') }}</p>
    <div class="auth-foot auth-back">
      <router-link to="/login">
        <n-icon :component="ArrowBackOutline" :size="13" />{{ $t('common.auth.forgot.back') }}
      </router-link>
    </div>
  </auth-layout>
</template>
