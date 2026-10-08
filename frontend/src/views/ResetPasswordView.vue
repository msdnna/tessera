<script setup>
import { ref, computed } from 'vue'
import { NForm, NFormItem, NInput, NButton, NIcon, useMessage } from 'naive-ui'
import { LinkOutline, CheckmarkCircleOutline, ArrowBackOutline } from '@vicons/ionicons5'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { accountFlows } from '@/api'
import AuthLayout from '@/components/AuthLayout.vue'
import PasswordMeter from '@/components/PasswordMeter.vue'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import { passwordError } from '@/utils/password'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const token = route.query.token || ''
const pw = ref('')
const confirm = ref('')
const loading = ref(false)
const done = ref(false)
// Said, not merely enforced: the button being dead with no reason next to it is
// how the too-long half of the rule would still be invisible. Empty while the
// field is untouched — a form does not scold before it is filled in.
const pwError = computed(() => (pw.value ? passwordError(pw.value) : ''))
const valid = computed(() => !pwError.value && pw.value === confirm.value)
// Said under the second field once it holds something, rather than only being a
// dead button: the mockup's «Пароли не совпадают».
const mismatch = computed(() => !!confirm.value && pw.value !== confirm.value)

async function submit() {
  if (!valid.value || !token) return
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
          : $t('common.auth.reset.title')
    "
    :subtitle="token && !done ? $t('common.auth.reset.subtitle') : ''"
    :icon="!token ? LinkOutline : done ? CheckmarkCircleOutline : null"
    :tone="!token ? '#e0533d' : '#18a058'"
  >
    <p v-if="!token" class="note">{{ $t('common.auth.reset.noToken') }}</p>
    <p v-else-if="done" class="note">{{ $t('common.auth.reset.done') }}</p>
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
