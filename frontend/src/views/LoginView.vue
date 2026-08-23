<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NForm, NFormItem, NInput, NButton, NIcon } from 'naive-ui'
import { LogoGitlab } from '@vicons/ionicons5'
import { useRouter, useRoute, RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { auth } from '@/api'
import AuthLayout from '@/components/AuthLayout.vue'
import { isTauri } from '@/utils/serverBase'
import {
  startDesktopGitlabLogin,
  OAUTH_DONE_EVENT,
  OAUTH_WAIT_MS,
} from '@/composables/useDesktopOAuth'

const { t, te } = useI18n()

const email = ref('')
const password = ref('')
const loading = ref(false)
const formError = ref('') // server / submit error, shown as a banner
const errors = ref({}) // per-field validation messages
const authStore = useAuthStore()
const router = useRouter()
const route = useRoute()

const gitlabEnabled = ref(false)

// Resolved per call, not as a module-level table: a table built at import time
// would freeze on the language of the first render (#2799).
function oauthError(reason) {
  const key = `common.auth.oauthError.${reason}`
  return reason && te(key) ? t(key) : t('common.auth.oauthError.generic')
}

onMounted(async () => {
  // Show a banner if the OAuth callback bounced back with an error.
  const oe = route.query.oauth_error
  if (typeof oe === 'string' && oe) {
    formError.value = oauthError(oe)
  }
  try {
    const { data } = await auth.providers()
    gitlabEnabled.value = data?.gitlab === true
  } catch {
    gitlabEnabled.value = false
  }
})

// Desktop waits for the browser round-trip; on web the page simply navigates away.
const glWaiting = ref(false)
let glTimer = null

function stopWaiting() {
  glWaiting.value = false
  clearTimeout(glTimer)
  glTimer = null
}

// The handoff itself is handled in App.vue (the OS may deliver it to a fresh process);
// here we only stop the spinner and surface an error if one came back.
function onOAuthDone(e) {
  stopWaiting()
  const reason = e.detail?.error
  if (reason) formError.value = oauthError(reason)
}
onMounted(() => window.addEventListener(OAUTH_DONE_EVENT, onOAuthDone))
onUnmounted(() => {
  window.removeEventListener(OAUTH_DONE_EVENT, onOAuthDone)
  clearTimeout(glTimer)
})

async function loginWithGitlab() {
  formError.value = ''
  // Web: a top-level navigation, which is what the SameSite=Lax state cookie needs.
  if (!isTauri()) {
    window.location.href = auth.gitlabAuthorizeUrl()
    return
  }
  // Desktop: the login must NOT happen inside our own webview (that was the #2696 bug,
  // and RFC 8252 forbids it anyway) — hand it to the system browser.
  glWaiting.value = true
  if (!(await startDesktopGitlabLogin(auth.gitlabAuthorizeUrl('desktop')))) {
    stopWaiting()
    formError.value = t('common.auth.oauthError.browserOpen')
    return
  }
  glTimer = setTimeout(() => {
    stopWaiting()
    formError.value = t('common.auth.oauthError.noReturn')
  }, OAUTH_WAIT_MS)
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

function validate() {
  const e = {}
  if (!email.value.trim()) e.email = t('common.auth.validation.emailRequired')
  else if (!EMAIL_RE.test(email.value.trim())) e.email = t('common.auth.validation.emailInvalid')
  if (!password.value) e.password = t('common.auth.validation.passwordRequired')
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
  <auth-layout :title="t('common.auth.login.title')">
    <div v-if="formError" class="auth-error">{{ formError }}</div>
    <n-form @submit.prevent="submit">
      <n-form-item
        :label="t('common.auth.login.email')"
        :validation-status="errors.email ? 'error' : undefined"
        :feedback="errors.email"
      >
        <n-input
          v-model:value="email"
          :placeholder="t('common.auth.login.emailPlaceholder')"
          data-testid="login-email"
          @input="errors.email = ''"
          @keyup.enter="submit"
        />
      </n-form-item>
      <n-form-item
        :label="t('common.auth.login.password')"
        :validation-status="errors.password ? 'error' : undefined"
        :feedback="errors.password"
      >
        <n-input
          v-model:value="password"
          type="password"
          show-password-on="click"
          data-testid="login-password"
          @input="errors.password = ''"
          @keyup.enter="submit"
        />
      </n-form-item>
      <n-button type="primary" block data-testid="login-submit" :loading="loading" @click="submit">
        {{ t('common.auth.login.submit') }}
      </n-button>
    </n-form>
    <template v-if="gitlabEnabled">
      <div class="auth-or"><span>{{ t('common.auth.login.or') }}</span></div>
      <n-button block class="gl-oauth-btn" :loading="glWaiting" @click="loginWithGitlab">
        <template #icon><n-icon :component="LogoGitlab" /></template>
        {{ glWaiting ? t('common.auth.login.gitlabWaiting') : t('common.auth.login.gitlab') }}
      </n-button>
      <div v-if="glWaiting" class="auth-foot">
        <a href="#" @click.prevent="stopWaiting">{{ t('common.action.cancel') }}</a>
      </div>
    </template>
    <div class="auth-foot">
      <router-link to="/forgot-password">{{ t('common.auth.login.forgot') }}</router-link>
    </div>
    <div class="auth-foot">
      {{ t('common.auth.login.noAccount') }}
      <router-link to="/register">{{ t('common.auth.login.register') }}</router-link>
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
.gl-oauth-btn {
  --n-border-hover: var(--t-accent, #7c5cff);
}
</style>
