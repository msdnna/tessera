<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { NButton } from 'naive-ui'
import { accountFlows } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useWorkspacesStore } from '@/stores/workspaces'
import { loginRouteForInvite, registerRouteForInvite } from '@/utils/inviteRoute'
import AuthLayout from '@/components/AuthLayout.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const ws = useWorkspacesStore()
const state = ref('pending') // pending | ok | fail | wrongAccount | error

// The server's own words, not the localized sentence: `message` is written in
// whatever language the UI is in and is rewritten outright the day this sentinel
// joins the errors catalogue (#2932).
const WRONG_ACCOUNT = 'this invitation is for a different email'

// `?outcome=` — the register form arriving with a verdict instead of a request.
// There the token was spent inside the register call, so re-accepting it here
// would report "already used" about an invitation that had just been applied,
// or replay a mismatch the server has already answered.
const OUTCOME_STATE = {
  email_mismatch: 'wrongAccount',
  invalid: 'fail',
  failed: 'error',
}

onMounted(async () => {
  const token = route.query.token
  const outcome = route.query.outcome
  if (outcome) {
    // An unknown verdict is still a failure to join, and saying so beats a
    // blank page: this reads a query parameter, and a future server value
    // arriving at an older build must not leave the invitee with nothing.
    state.value = OUTCOME_STATE[outcome] || 'fail'
    return
  }
  if (!token) {
    state.value = 'fail'
    return
  }
  // No account yet: accepting needs a session, so hand the token to the register
  // form. It presents it on sign-up — the only thing that joins an invitee to
  // the workspace now, since the invited email alone grants nothing.
  if (!auth.isAuthenticated) {
    router.replace(registerRouteForInvite(token))
    return
  }
  try {
    const res = await accountFlows.acceptInvitation(token)
    const wsId = res.data?.id
    if (wsId) {
      await ws.loadWorkspaces()
      await ws.selectWorkspace(wsId)
    }
    state.value = 'ok'
    setTimeout(() => router.push('/'), 1200)
  } catch (e) {
    // One of these refusals the invitee can fix themselves, and it is the common
    // one: the invitation went to a work address while the browser holds a
    // personal session. Told apart, it gets a cure (sign out, come back here);
    // lumped in with a spent or expired token, it got a dead end — this page
    // offers no way back to an invitation it just failed to accept, and the link
    // lives in an inbox.
    state.value = e?.raw === WRONG_ACCOUNT ? 'wrongAccount' : 'fail'
  }
})

// Sign out and return to *this* invitation: `?next` replays it once the right
// account is signed in, so the token survives the round trip.
async function switchAccount() {
  await auth.logout()
  router.replace(loginRouteForInvite(route.query.token))
}
</script>

<template>
  <auth-layout :title="$t('common.auth.invite.title')">
    <p class="note">{{ $t(`common.auth.invite.${state}`) }}</p>
    <n-button
      v-if="state === 'wrongAccount'"
      block
      type="primary"
      data-testid="invite-switch-account"
      @click="switchAccount"
    >
      {{ $t('common.auth.invite.switchAccount') }}
    </n-button>
    <div class="auth-foot">
      <router-link to="/">{{ $t('common.auth.toHome') }}</router-link>
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
