<script setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NForm, NFormItem, NInput, NButton, NIcon } from 'naive-ui'
import { LogoGitlab, AlertCircleOutline } from '@vicons/ionicons5'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { auth, accountFlows } from '@/api'
import AuthLayout from '@/components/AuthLayout.vue'
import PasswordMeter from '@/components/PasswordMeter.vue'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import { passwordError } from '@/utils/password'
import { loginRouteForInvite, inviteOutcomeRoute } from '@/utils/inviteRoute'
import { stashPendingInvite } from '@/utils/pendingInvite'

const { t } = useI18n()

const email = ref('')
const name = ref('')
const password = ref('')
const loading = ref(false)
const formError = ref('')
const errors = ref({})
const authStore = useAuthStore()
const router = useRouter()
// ?invite=<token> — set by the invite page when the invitee has no account yet.
// Carried into register so the backend can apply that one invitation.
const inviteToken = useRoute().query.invite || ''

const gitlabEnabled = ref(false)
// Whether this server sends letters: only then does sign-up lead on to the
// «Проверьте почту» code screen (#3057) — without mail no code can arrive.
const mailEnabled = ref(false)
onMounted(async () => {
  try {
    const { data } = await auth.providers()
    gitlabEnabled.value = data?.gitlab === true
    mailEnabled.value = data?.mail === true
  } catch {
    gitlabEnabled.value = false
  }
})
// An addressed invitation fills the e-mail in (#3059): the invited address is
// the one thing the invitee cannot check on their own, and registering another
// one is exactly the mismatch the accept page then has to explain (#2932).
onMounted(async () => {
  if (!inviteToken) return
  try {
    const { data } = await accountFlows.lookupInvitation(inviteToken)
    if (data?.kind === 'email' && data?.email && !email.value) email.value = data.email
  } catch {
    /* an unknown or spent token: the form stays as it is */
  }
})
function registerWithGitlab() {
  // The token cannot ride in the URL across the provider's redirect, so it waits
  // in the tab: the callback picks it up and finishes the acceptance. Without
  // this the SSO invitee signs up into a workspace they were never joined to.
  stashPendingInvite(inviteToken)
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
  // Both bounds, from the shared rule: the ceiling is bcrypt's and is counted in
  // bytes, so a long Russian passphrase used to reach the server and come back
  // as "internal error" (#2932).
  else if (passwordError(password.value)) e.password = passwordError(password.value)
  errors.value = e
  return Object.keys(e).length === 0
}

async function submit() {
  formError.value = ''
  if (!validate()) return
  loading.value = true
  try {
    const outcome = await authStore.register(
      email.value.trim(),
      name.value.trim(),
      password.value,
      inviteToken,
    )
    // The account is created either way, so a token that granted nothing has to
    // be said out loud here — the invited address is exactly what the invitee
    // cannot check (the link is as often pasted into a chat as mailed), and on
    // the board there is no sign of a workspace they never joined. The accept
    // page owns those words, and for a mismatch also the way out of it.
    if (inviteToken && outcome && outcome !== 'applied') {
      router.replace(inviteOutcomeRoute(inviteToken, outcome))
      return
    }
    router.push(mailEnabled.value ? '/verify-email' : '/')
  } catch (e) {
    formError.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <auth-layout
    :title="$t('common.auth.register.title')"
    :subtitle="$t('common.auth.register.subtitle')"
  >
    <div v-if="formError" class="auth-error" role="alert">
      <n-icon :component="AlertCircleOutline" :size="16" />
      <span>{{ formError }}</span>
    </div>
    <n-form @submit.prevent="submit">
      <n-form-item
        :label="$t('common.auth.register.name')"
        :validation-status="errors.name ? 'error' : undefined"
        :feedback="errors.name"
      >
        <n-input
          v-model:value="name"
          :placeholder="$t('common.auth.register.namePlaceholder')"
          :input-props="{ autocomplete: 'name' }"
          data-testid="register-name"
          @input="errors.name = ''"
          @keyup.enter="submit"
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
          :input-props="{ autocomplete: 'email' }"
          data-testid="register-email"
          @input="errors.email = ''"
          @keyup.enter="submit"
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
          :input-props="{ autocomplete: 'new-password' }"
          data-testid="register-password"
          @input="errors.password = ''"
          @keyup.enter="submit"
        />
      </n-form-item>
      <PasswordMeter v-if="!errors.password" :value="password" />
      <n-button
        type="primary"
        size="large"
        block
        class="auth-submit"
        data-testid="register-submit"
        :loading="loading"
        @click="submit"
      >
        {{ $t('common.auth.register.submit') }}<Kbd>{{ CAPS.enter }}</Kbd>
      </n-button>
    </n-form>
    <template v-if="gitlabEnabled">
      <div class="auth-or">{{ $t('common.auth.login.or') }}</div>
      <n-button block size="large" class="auth-alt" @click="registerWithGitlab">
        <template #icon><n-icon :component="LogoGitlab" /></template>
        {{ $t('common.auth.register.gitlab') }}
      </n-button>
    </template>
    <div class="auth-foot">
      {{ $t('common.auth.register.haveAccount') }}
      <!-- An invitee who already has an account lands here anyway (the accept
           page sends every signed-out visitor to register) and takes this link.
           It carries the token on, or the invitation is left unaccepted with its
           only link back in the mailbox. -->
      <router-link :to="loginRouteForInvite(inviteToken)">{{
        $t('common.auth.register.login')
      }}</router-link>
    </div>
  </auth-layout>
</template>
