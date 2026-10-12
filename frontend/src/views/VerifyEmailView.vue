<script setup>
// «Подтверждение email» (v2, #3035 — AuthSheet).
//
// Two doors share the address:
//   * `?token=` — the link from the letter, as before: checked once, a verdict.
//   * no token, signed in — the code screen (#3057): six cells, «Отправить ещё
//     раз через 0:42», «Изменить email» (#3058). `?change=1` opens straight on
//     the new-address form (Settings → Профиль), `?next=` is where «Продолжить»
//     and «Позже» lead.
//
// The server owns every number on this screen — where the live code went, its
// attempts, when the next send is allowed — and answers each call with the
// whole state (GET /auth/verification), so nothing here is guessed.
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { NButton, NForm, NFormItem, NInput, NIcon } from 'naive-ui'
import {
  MailOutline,
  MailOpenOutline,
  MailUnreadOutline,
  AlertCircleOutline,
  CheckmarkCircleOutline,
  ArrowBackOutline,
} from '@vicons/ionicons5'
import { accountFlows } from '@/api'
import { useAuthStore } from '@/stores/auth'
import AuthLayout from '@/components/AuthLayout.vue'
import CodeCells from '@/components/auth/CodeCells.vue'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import { safeNext, formatCountdown } from '@/utils/authFlow'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const token = typeof route.query.token === 'string' ? route.query.token : ''
const next = safeNext(route.query.next)

// ── the link ──
const linkState = ref('pending') // pending | ok | fail
const LINK_ICON = { pending: MailOutline, ok: MailOpenOutline, fail: AlertCircleOutline }
const LINK_TONE = { pending: '#2f80ed', ok: '#18a058', fail: '#e0533d' }

// ── the code ──
// view: loading | code | change | done | signedOut
const view = ref(token ? 'link' : 'loading')
const state = ref(null)
const code = ref('')
const codeError = ref('')
const busy = ref(false)
const doneKind = ref('verified') // verified | changed | already
const cells = ref(null)

// The resend countdown, ticking locally from the server's `resend_in`.
const resendIn = ref(0)
let timer = null
function startCountdown(sec) {
  resendIn.value = Math.max(0, Math.ceil(Number(sec) || 0))
  clearInterval(timer)
  if (resendIn.value > 0) {
    timer = setInterval(() => {
      resendIn.value = Math.max(0, resendIn.value - 1)
      if (!resendIn.value) clearInterval(timer)
    }, 1000)
  }
}
onBeforeUnmount(() => clearInterval(timer))

function applyState(s) {
  state.value = s
  startCountdown(s?.resend_in)
}

const target = computed(() => state.value?.target || auth.user?.email || '')
const changing = computed(() => !!state.value?.change)
// The password is asked when the server would ask it: an account with one whose
// address is already proven (see StartEmailChange).
const needsPassword = computed(() => !!state.value?.has_password && !!state.value?.verified)

async function sendCode() {
  busy.value = true
  codeError.value = ''
  try {
    const res = await accountFlows.resendVerification()
    if (res.data) applyState(res.data)
    code.value = ''
    cells.value?.focus()
  } catch (e) {
    if (e.status === 429 && e.data?.retry_after) startCountdown(e.data.retry_after)
    else codeError.value = e.message
  } finally {
    busy.value = false
  }
}

async function submitCode(value = code.value) {
  if (busy.value || value.length !== 6) return
  busy.value = true
  codeError.value = ''
  try {
    const res = await accountFlows.verifyCode(value)
    const wasChange = changing.value
    if (res.data?.user) auth.setUser({ ...auth.user, ...res.data.user })
    doneKind.value = wasChange ? 'changed' : 'verified'
    view.value = 'done'
  } catch (e) {
    code.value = ''
    if (e.raw === 'invalid code') {
      codeError.value = t('common.auth.verify.wrongCode', { n: e.data?.attempts_left ?? 0 })
    } else if (e.raw === 'too many attempts') {
      codeError.value = t('common.auth.verify.tooManyAttempts')
    } else if (e.raw === 'code expired') {
      codeError.value = t('common.auth.verify.expired')
    } else {
      codeError.value = e.message
    }
    if (state.value && e.data && 'attempts_left' in e.data) {
      state.value = { ...state.value, attempts_left: e.data.attempts_left }
    }
    cells.value?.focus()
  } finally {
    busy.value = false
  }
}

// ── «Изменить email» ──
const newEmail = ref('')
const password = ref('')
const changeError = ref('')
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

function openChange() {
  changeError.value = ''
  newEmail.value = ''
  password.value = ''
  view.value = 'change'
}
function backFromChange() {
  // Opened straight on the form (from Settings) with nothing to confirm: back
  // is where it came from.
  if (route.query.change && state.value?.verified && !changing.value) {
    router.push(next)
    return
  }
  view.value = 'code'
}

async function submitChange() {
  const v = newEmail.value.trim()
  if (!EMAIL_RE.test(v)) {
    changeError.value = t('common.auth.validation.emailInvalid')
    return
  }
  busy.value = true
  changeError.value = ''
  try {
    const res = await accountFlows.startEmailChange(v, password.value)
    applyState(res.data)
    code.value = ''
    codeError.value = ''
    view.value = 'code'
  } catch (e) {
    if (e.status === 429 && e.data?.retry_after) {
      startCountdown(e.data.retry_after)
      changeError.value = t('common.auth.verify.resendIn', {
        time: formatCountdown(e.data.retry_after),
      })
    } else {
      changeError.value = e.message
    }
  } finally {
    busy.value = false
  }
}

async function cancelChange() {
  busy.value = true
  try {
    const res = await accountFlows.cancelEmailChange()
    applyState(res.data)
    code.value = ''
    codeError.value = ''
    if (res.data?.verified) {
      router.push(next)
      return
    }
    // Back to the account's own address: it needs a code of its own.
    if (!res.data?.expires_at && !res.data?.resend_in) await sendCode()
  } catch (e) {
    codeError.value = e.message
  } finally {
    busy.value = false
  }
}

function proceed() {
  router.push(next)
}

// ↵ on the «done» card — it has no field to take the key.
function onKey(e) {
  if (e.key === 'Enter' && view.value === 'done' && !e.isComposing) proceed()
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

onMounted(async () => {
  if (token) {
    try {
      await accountFlows.verifyEmail(token)
      linkState.value = 'ok'
      if (auth.isAuthenticated && auth.user) auth.setUser({ ...auth.user, email_verified: true })
    } catch {
      linkState.value = 'fail'
    }
    return
  }
  if (!auth.isAuthenticated) {
    view.value = 'signedOut'
    return
  }
  try {
    const res = await accountFlows.verification()
    applyState(res.data)
  } catch (e) {
    codeError.value = e.message
    view.value = 'code'
    return
  }
  const s = state.value
  if (route.query.change && !s.change) {
    openChange()
    return
  }
  if (s.verified && !s.change) {
    doneKind.value = 'already'
    view.value = 'done'
    return
  }
  view.value = 'code'
  // Nothing live to type in (the sign-up code expired, or never went out):
  // send one now, unless the pause says it has only just been sent.
  if (!s.expires_at && !s.resend_in) await sendCode()
})

const DONE_TITLE = {
  verified: 'common.auth.verify.okTitle',
  changed: 'common.auth.verify.changedTitle',
  already: 'common.auth.verify.okTitle',
}
const title = computed(() => {
  switch (view.value) {
    case 'link':
      return t(`common.auth.verify.${linkState.value}Title`)
    case 'change':
      return t('common.auth.verify.changeTitle')
    case 'done':
      return t(DONE_TITLE[doneKind.value])
    case 'signedOut':
      return t('common.auth.verify.title')
    default:
      return t('common.auth.verify.codeTitle')
  }
})
const subtitle = computed(() => {
  if (view.value === 'code' && target.value) {
    return t('common.auth.verify.codeSent', { email: target.value })
  }
  if (view.value === 'change') return t('common.auth.verify.changeSub')
  return ''
})
const icon = computed(() => {
  if (view.value === 'link') return LINK_ICON[linkState.value]
  if (view.value === 'done') return CheckmarkCircleOutline
  return MailUnreadOutline
})
const tone = computed(() => {
  if (view.value === 'link') return LINK_TONE[linkState.value]
  if (view.value === 'done') return '#18a058'
  return '#2f80ed'
})
const doneNote = computed(() => {
  const email = auth.user?.email || target.value
  if (doneKind.value === 'changed') return t('common.auth.verify.changed', { email })
  if (doneKind.value === 'already') return t('common.auth.verify.already', { email })
  return t('common.auth.verify.okCode', { email })
})
</script>

<template>
  <auth-layout :title="title" :subtitle="subtitle" :icon="icon" :tone="tone">
    <!-- The link from the letter. -->
    <template v-if="view === 'link'">
      <p class="note" data-testid="verify-state">{{ $t(`common.auth.verify.${linkState}`) }}</p>
      <div class="auth-foot">
        <router-link to="/">{{ $t('common.auth.toHome') }}</router-link>
      </div>
    </template>

    <template v-else-if="view === 'signedOut'">
      <p class="note" data-testid="verify-signed-out">{{ $t('common.auth.verify.signedOut') }}</p>
      <div class="auth-foot">
        <router-link :to="{ path: '/login', query: { next: route.fullPath } }">{{
          $t('common.auth.verify.signIn')
        }}</router-link>
      </div>
    </template>

    <template v-else-if="view === 'code' || view === 'loading'">
      <CodeCells
        ref="cells"
        v-model="code"
        :error="!!codeError"
        :disabled="busy || view === 'loading'"
        :label="$t('common.auth.verify.codeLabel')"
        @complete="submitCode"
      />
      <p v-if="codeError" class="vc-err" role="alert" data-testid="code-error">
        <n-icon :component="AlertCircleOutline" :size="14" />{{ codeError }}
      </p>
      <div class="vc-row">
        <span v-if="resendIn > 0" class="vc-wait" data-testid="code-resend-wait">{{
          $t('common.auth.verify.resendIn', { time: formatCountdown(resendIn) })
        }}</span>
        <button
          v-else
          type="button"
          class="vc-link"
          :disabled="busy"
          data-testid="code-resend"
          @click="sendCode"
        >
          {{ $t('common.auth.verify.resend') }}
        </button>
        <button
          v-if="changing"
          type="button"
          class="vc-link vc-quiet"
          :disabled="busy"
          data-testid="code-cancel-change"
          @click="cancelChange"
        >
          {{ $t('common.auth.verify.cancelChange') }}
        </button>
        <button
          v-else
          type="button"
          class="vc-link vc-quiet"
          data-testid="code-change-email"
          @click="openChange"
        >
          {{ $t('common.auth.verify.changeEmail') }}
        </button>
      </div>
      <p v-if="state && !state.mail" class="vc-hint" data-testid="code-no-mail">
        {{ $t('common.auth.verify.noMail') }}
      </p>
      <div class="auth-foot">
        <router-link :to="next" data-testid="code-later">{{
          $t('common.auth.verify.later')
        }}</router-link>
      </div>
    </template>

    <template v-else-if="view === 'change'">
      <n-form @submit.prevent="submitChange">
        <n-form-item
          :label="$t('common.auth.verify.newEmail')"
          :validation-status="changeError ? 'error' : undefined"
          :feedback="changeError"
        >
          <n-input
            v-model:value="newEmail"
            :placeholder="$t('common.auth.login.emailPlaceholder')"
            :input-props="{ autocomplete: 'email', type: 'email' }"
            data-testid="change-email"
            @input="changeError = ''"
            @keyup.enter="submitChange"
          />
        </n-form-item>
        <n-form-item v-if="needsPassword" :label="$t('common.auth.verify.currentPassword')">
          <n-input
            v-model:value="password"
            type="password"
            show-password-on="click"
            :input-props="{ autocomplete: 'current-password' }"
            data-testid="change-password"
            @keyup.enter="submitChange"
          />
        </n-form-item>
        <n-button
          type="primary"
          size="large"
          block
          class="auth-submit"
          data-testid="change-submit"
          :loading="busy"
          :disabled="!newEmail.trim() || (needsPassword && !password)"
          @click="submitChange"
        >
          {{ $t('common.auth.verify.sendCode') }}<Kbd>{{ CAPS.enter }}</Kbd>
        </n-button>
      </n-form>
      <div class="auth-foot auth-back">
        <a href="#" data-testid="change-back" @click.prevent="backFromChange">
          <n-icon :component="ArrowBackOutline" :size="13" />{{ $t('common.auth.verify.back') }}
        </a>
      </div>
    </template>

    <template v-else-if="view === 'done'">
      <p class="note" data-testid="verify-done">{{ doneNote }}</p>
      <n-button
        type="primary"
        size="large"
        block
        class="auth-submit auth-gap"
        data-testid="verify-continue"
        @click="proceed"
      >
        {{ $t('common.auth.verify.continue') }}<Kbd>{{ CAPS.enter }}</Kbd>
      </n-button>
    </template>
  </auth-layout>
</template>

<style scoped>
.vc-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 16px;
  font-size: 13px;
}
.vc-wait {
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.vc-link {
  padding: 0;
  border: 0;
  background: none;
  font: inherit;
  color: var(--t-primary);
  font-weight: 600;
  cursor: pointer;
}
.vc-link.vc-quiet {
  color: var(--t-text2);
  font-weight: 500;
}
.vc-link:hover:not(:disabled) {
  text-decoration: underline;
}
.vc-link:disabled {
  opacity: 0.6;
  cursor: default;
}
.vc-err {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 10px 0 0;
  font-size: 13px;
  color: color-mix(in srgb, #e0533d 82%, var(--t-text1));
}
.vc-hint {
  margin: 14px 0 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--t-text3);
}
</style>
