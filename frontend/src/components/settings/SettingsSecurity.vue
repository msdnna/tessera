<script setup>
// Settings → «Безопасность» (v2, #3106). The password form, or — for an account
// provisioned through GitLab without a password hash — the «Прислать ссылку»
// card instead of a form that could only be refused (#2932, `has_password`).
// Two-factor sign-in is drawn where the mockup has it, as a stub (#3081).
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NIcon, NInput, NTooltip } from 'naive-ui'
import { CheckmarkCircle, KeyOutline, PhonePortraitOutline } from '@vicons/ionicons5'
import { accountFlows, users } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { isPasswordValid, passwordError } from '@/utils/password'
import { stubTask } from '@/utils/featureStubs'
import './settings.css'

const { t } = useI18n()
const auth = useAuthStore()

const pw = reactive({ current: '', next: '', confirm: '' })
const saving = ref(false)
const saved = ref(false)
const error = ref('')
const valid = computed(() => !!pw.current && isPasswordValid(pw.next) && pw.next === pw.confirm)
// The reason, live — the ceiling is bcrypt's and counted in bytes, so the case
// this catches is an ordinary Russian passphrase (#2932).
const fieldError = computed(() => (pw.next ? passwordError(pw.next) : ''))
// Hidden on `has_password === false` only: a profile cached by a build that
// predates the flag must not lock a password-holding account out of its form.
const hasPassword = computed(() => auth.user?.has_password !== false)
const linkSending = ref(false)
const linkSent = ref(false)

async function sendLink() {
  error.value = ''
  linkSending.value = true
  try {
    await accountFlows.forgotPassword(auth.user?.email)
    linkSent.value = true
  } catch (e) {
    error.value = e.message
  } finally {
    linkSending.value = false
  }
}

async function change() {
  error.value = ''
  if (!valid.value) {
    error.value = passwordError(pw.next) || t('settings.security.invalid')
    return
  }
  saving.value = true
  try {
    // The change logs out every session the old password was behind, this one
    // included, and hands back a replacement — storing it keeps this tab in.
    const res = await users.changePassword({ current_password: pw.current, new_password: pw.next })
    auth.setAuth(res.data)
    pw.current = pw.next = pw.confirm = ''
    saved.value = true
    setTimeout(() => (saved.value = false), 2000)
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
  }
}

const twoFactorTask = stubTask('settings.twoFactor')
</script>

<template>
  <div class="ss">
    <section class="st-card" data-testid="settings-password">
      <div class="st-card-head">
        <n-icon :component="KeyOutline" :size="16" class="ss-ic" />
        <span class="st-card-title">{{ t('settings.security.password') }}</span>
      </div>

      <template v-if="!hasPassword">
        <p class="st-hint">{{ t('settings.security.noPassword', { email: auth.user?.email }) }}</p>
        <div class="st-actions">
          <span v-if="error" class="st-err">{{ error }}</span>
          <span v-if="linkSent" class="st-ok">
            <n-icon :component="CheckmarkCircle" />
            {{ t('settings.security.linkSent', { email: auth.user?.email }) }}
          </span>
          <n-button v-else type="primary" :loading="linkSending" @click="sendLink">
            {{ t('settings.security.setLink') }}
          </n-button>
        </div>
      </template>

      <template v-else>
        <div class="st-grid3">
          <label class="st-field">
            <span class="st-label">{{ t('settings.security.current') }}</span>
            <n-input
              v-model:value="pw.current"
              type="password"
              show-password-on="click"
              :input-props="{ autocomplete: 'current-password' }"
            />
          </label>
          <label class="st-field">
            <span class="st-label">{{ t('settings.security.next') }}</span>
            <n-input
              v-model:value="pw.next"
              type="password"
              show-password-on="click"
              :input-props="{ autocomplete: 'new-password' }"
            />
          </label>
          <label class="st-field">
            <span class="st-label">{{ t('settings.security.confirm') }}</span>
            <n-input
              v-model:value="pw.confirm"
              type="password"
              show-password-on="click"
              :input-props="{ autocomplete: 'new-password' }"
            />
          </label>
        </div>
        <div class="st-actions">
          <span v-if="error || fieldError" class="st-err">{{ error || fieldError }}</span>
          <transition name="st-fade">
            <span v-if="saved" class="st-ok">
              <n-icon :component="CheckmarkCircle" /> {{ t('settings.security.changed') }}
            </span>
          </transition>
          <n-button type="primary" :disabled="!valid" :loading="saving" @click="change">
            {{ t('settings.security.change') }}
          </n-button>
        </div>
      </template>
    </section>

    <n-tooltip :delay="300">
      <template #trigger>
        <section class="st-card ss-row st-stub" data-testid="settings-2fa">
          <span class="ss-badge"><n-icon :component="PhonePortraitOutline" :size="17" /></span>
          <span class="ss-row-main">
            <span class="ss-row-title">{{ t('settings.security.twoFactor') }}</span>
            <span class="st-hint">{{ t('settings.security.twoFactorHint') }}</span>
          </span>
          <span class="st-soon">{{ t('common.stub.badge') }}</span>
        </section>
      </template>
      {{ t('common.stub.soon', { n: twoFactorTask }) }}
    </n-tooltip>
  </div>
</template>

<style scoped>
.ss {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.ss-ic {
  color: var(--t-text3);
}
.ss-row {
  flex-direction: row;
  align-items: center;
  gap: 14px;
}
.ss-badge {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  color: var(--t-text2);
}
.ss-row-main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}
.ss-row-title {
  font-size: 14px;
  font-weight: 600;
}
.st-fade-enter-active,
.st-fade-leave-active {
  transition: opacity 0.3s ease;
}
.st-fade-enter-from,
.st-fade-leave-to {
  opacity: 0;
}
</style>
