<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { accountFlows } from '@/api'
import { MailOutline, MailOpenOutline, AlertCircleOutline } from '@vicons/ionicons5'
import AuthLayout from '@/components/AuthLayout.vue'

const route = useRoute()
const state = ref('pending') // pending | ok | fail

// The badge says the state before the words do: a letter in blue while the
// link is checked, an opened one in green, a warning in red.
const STATE_ICON = { pending: MailOutline, ok: MailOpenOutline, fail: AlertCircleOutline }
const STATE_TONE = { pending: '#2f80ed', ok: '#18a058', fail: '#e0533d' }

onMounted(async () => {
  const token = route.query.token
  if (!token) {
    state.value = 'fail'
    return
  }
  try {
    await accountFlows.verifyEmail(token)
    state.value = 'ok'
  } catch {
    state.value = 'fail'
  }
})
</script>

<template>
  <auth-layout
    :title="$t(`common.auth.verify.${state}Title`)"
    :icon="STATE_ICON[state]"
    :tone="STATE_TONE[state]"
  >
    <p class="note" data-testid="verify-state">{{ $t(`common.auth.verify.${state}`) }}</p>
    <div class="auth-foot">
      <router-link to="/">{{ $t('common.auth.toHome') }}</router-link>
    </div>
  </auth-layout>
</template>
