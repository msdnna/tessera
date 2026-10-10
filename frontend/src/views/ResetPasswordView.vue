<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { NForm, NFormItem, NInput, NButton, NIcon } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  LinkOutline,
  CheckmarkCircleOutline,
  ArrowBackOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { accountFlows } from '@/api'
import AuthLayout from '@/components/AuthLayout.vue'
import PasswordMeter from '@/components/PasswordMeter.vue'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import { passwordError } from '@/utils/password'
import { minutesLeft } from '@/utils/authFlow'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const token = route.query.token || ''
const pw = ref('')
const confirm = ref('')
const loading = ref(false)
const done = ref(false)

// The link's own deadline (#3060): «Ссылка действует ещё 54 минуты», read from
// the server without spending the link, and a dead link said before the form
// is filled in rather than after. `checked` is null while asking; a check that
// could not reach the server leaves the form as it was — the reset itself still
// gives the verdict.
const checked = ref(token ? null : false)
const expiresAt = ref(null)
const now = ref(Date.now())
let tick = null
const left = computed(() => (expiresAt.value ? minutesLeft(expiresAt.value, now.value) : 0))
const expired = computed(() => checked.value === false || (!!expiresAt.value && left.value <= 0))
onMounted(async () => {
  if (!token) return
  try {
    const { data } = await accountFlows.checkResetToken(token)
    expiresAt.value = data?.expires_at || null
    checked.value = true
    tick = setInterval(() => (now.value = Date.now()), 15000)
  } catch (e) {
    checked.value = e.status === 400 ? false : true
  }
})
onBeforeUnmount(() => clearInterval(tick))
// Said, not merely enforced: the button being dead with no reason next to it is
// how the too-long half of the rule would still be invisible. Empty while the
// field is untouched — a form does not scold before it is filled in.
const pwError = computed(() => (pw.value ? passwordError(pw.value) : ''))
const valid = computed(() => !pwError.value && pw.value === confirm.value)
// Said under the second field once it holds something, rather than only being a
// dead button: the mockup's «Пароли не совпадают».
const mismatch = computed(() => !!confirm.value && pw.value !== confirm.value)

async function submit() {
  if (!valid.value || !token || expired.value) return
  loading.value = true
  try {
    await accountFlows.resetPassword(token, pw.value)
    done.value = true
    setTimeout(() => router.push('/login'), 1500)
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <auth-layout
    :title="
      !token
        ? $t('common.auth.reset.badTitle')
        : done
          ? $t('common.auth.reset.doneTitle')
          : expired
            ? $t('common.auth.reset.expiredTitle')
            : $t('common.auth.reset.title')
    "
    :subtitle="
      !token || done || expired
        ? ''
        : expiresAt
          ? $t('common.auth.reset.validFor', { n: left }, left)
          : $t('common.auth.reset.subtitle')
    "
    :icon="!token ? LinkOutline : done ? CheckmarkCircleOutline : expired ? TimeOutline : null"
    :tone="!token || expired ? '#e0533d' : '#18a058'"
  >
    <p v-if="!token" class="note">{{ $t('common.auth.reset.noToken') }}</p>
    <p v-else-if="done" class="note">{{ $t('common.auth.reset.done') }}</p>
    <template v-else-if="expired">
      <p class="note" data-testid="reset-expired">{{ $t('common.auth.reset.expired') }}</p>
      <n-button
        type="primary"
        size="large"
        block
        class="auth-submit auth-gap"
        data-testid="reset-request-new"
        @click="router.push('/forgot-password')"
      >
        {{ $t('common.auth.reset.requestNew') }}
      </n-button>
    </template>
    <n-form v-else @submit.prevent="submit">
      <n-form-item
        :label="$t('common.auth.reset.password')"
        :validation-status="pwError ? 'error' : undefined"
        :feedback="pwError"
      >
        <n-input
          v-model:value="pw"
          type="password"
          show-password-on="click"
          placeholder=""
          :input-props="{ autocomplete: 'new-password' }"
          data-testid="reset-password"
          @keyup.enter="submit"
        />
      </n-form-item>
      <PasswordMeter v-if="!pwError" :value="pw" />
      <n-form-item
        :label="$t('common.auth.reset.confirm')"
        :validation-status="mismatch ? 'error' : undefined"
        :feedback="mismatch ? $t('common.auth.reset.mismatch') : undefined"
      >
        <n-input
          v-model:value="confirm"
          type="password"
          show-password-on="click"
          placeholder=""
          :input-props="{ autocomplete: 'new-password' }"
          data-testid="reset-confirm"
          @keyup.enter="submit"
        />
      </n-form-item>
      <n-button
        type="primary"
        size="large"
        block
        class="auth-submit"
        data-testid="reset-submit"
        :disabled="!valid"
        :loading="loading"
        @click="submit"
      >
        {{ $t('common.auth.reset.submit') }}<Kbd>{{ CAPS.enter }}</Kbd>
      </n-button>
    </n-form>
    <div class="auth-foot auth-back">
      <router-link to="/login">
        <n-icon :component="ArrowBackOutline" :size="13" />{{ $t('common.auth.reset.toLogin') }}
      </router-link>
    </div>
  </auth-layout>
</template>
