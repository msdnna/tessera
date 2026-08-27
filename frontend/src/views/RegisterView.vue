<script setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NForm, NFormItem, NInput, NButton, NIcon } from 'naive-ui'
import { LogoGitlab } from '@vicons/ionicons5'
import { useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { auth } from '@/api'
import AuthLayout from '@/components/AuthLayout.vue'

const { t } = useI18n()

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
  // Resolved on submit, not in a module-level table: the messages have to speak
  // the language the form is being filled in right now (#2799).
  if (!name.value.trim()) e.name = t('common.auth.validation.nameRequired')
  if (!email.value.trim()) e.email = t('common.auth.validation.emailRequired')
  else if (!EMAIL_RE.test(email.value.trim())) e.email = t('common.auth.validation.emailInvalid')
  if (!password.value) e.password = t('common.auth.validation.passwordRequired')
  else if (password.value.length < 8) e.password = t('common.auth.validation.passwordTooShort')
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
  <auth-layout :title="$t('common.auth.register.title')">
    <div v-if="formError" class="auth-error">{{ formError }}</div>
    <n-form @submit.prevent="submit">
      <n-form-item
        :label="$t('common.auth.register.name')"
        :validation-status="errors.name ? 'error' : undefined"
        :feedback="errors.name"
      >
        <n-input
          v-model:value="name"
          :placeholder="$t('common.auth.register.namePlaceholder')"
          data-testid="register-name"
          @input="errors.name = ''"
        />
      </n-form-item>
      <n-form-item
        :label="$t('common.auth.login.email')"
        :validation-status="errors.email ? 'error' : undefined"
        :feedback="errors.email"
      >
        <n-input
          v-model:value="email"
          :placeholder="$t('common.auth.login.emailPlaceholder')"
          data-testid="register-email"
          @input="errors.email = ''"
        />
      </n-form-item>
      <n-form-item
        :label="$t('common.auth.login.password')"
        :validation-status="errors.password ? 'error' : undefined"
        :feedback="errors.password"
      >
        <n-input
          v-model:value="password"
          type="password"
          show-password-on="click"
          :placeholder="$t('common.auth.register.passwordPlaceholder')"
          data-testid="register-password"
          @input="errors.password = ''"
          @keyup.enter="submit"
        />
      </n-form-item>
      <n-button
        type="primary"
        block
        data-testid="register-submit"
        :loading="loading"
        @click="submit"
      >
        {{ $t('common.auth.register.submit') }}
      </n-button>
    </n-form>
    <template v-if="gitlabEnabled">
      <div class="auth-or">
        <span>{{ $t('common.auth.login.or') }}</span>
      </div>
      <n-button block @click="registerWithGitlab">
        <template #icon><n-icon :component="LogoGitlab" /></template>
        {{ $t('common.auth.register.gitlab') }}
      </n-button>
    </template>
    <div class="auth-foot">
      {{ $t('common.auth.register.haveAccount') }}
      <router-link to="/login">{{ $t('common.auth.register.login') }}</router-link>
    </div>
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
