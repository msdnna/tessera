<script setup>
// «Приглашение» (v2, #3059 — AuthSheet): the invitation is shown before it is
// accepted — who invites, into what, how big it is, with which role — and can be
// declined. The same page serves the workspace's shared link (#3062): `/join/<token>`
// redirects here, and the server tells the two kinds apart.
//
// The #2932 behaviour is kept: an invitee with no account is sent to register
// with the token (now from the «Принять» button rather than at once), the wrong
// account gets a way to switch, and `?outcome=` reports what registration
// already did with the token instead of trying it a second time.
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { NButton, NIcon } from 'naive-ui'
import { accountFlows } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useFormat } from '@/composables/useFormat'
import { loginRouteForInvite, registerRouteForInvite } from '@/utils/inviteRoute'
import { workspaceName } from '@/utils/defaultNames'
import { hueGrad } from '@/utils/gradient'
import { initialsOfSpace } from '@/utils/authFlow'
import {
  PeopleOutline,
  CheckmarkCircleOutline,
  CheckmarkCircle,
  AlertCircleOutline,
  SwapHorizontalOutline,
  CloseCircleOutline,
  LinkOutline,
} from '@vicons/ionicons5'
import AuthLayout from '@/components/AuthLayout.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'

const { t } = useI18n()
const { formatDate } = useFormat()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const ws = useWorkspacesStore()
// loading | preview | accepting | ok | fail | wrongAccount | error | declined
const state = ref('loading')
const preview = ref(null)
const busy = ref(false)
const token = typeof route.query.token === 'string' ? route.query.token : ''

const STATE_ICON = {
  loading: PeopleOutline,
  accepting: PeopleOutline,
  ok: CheckmarkCircleOutline,
  fail: AlertCircleOutline,
  wrongAccount: SwapHorizontalOutline,
  error: AlertCircleOutline,
  declined: CloseCircleOutline,
}
const STATE_TONE = {
  loading: '',
  accepting: '',
  ok: '#18a058',
  fail: '#e0533d',
  wrongAccount: '#f0a020',
  error: '#e0533d',
  declined: '#e0533d',
}

// The server's own words, not the localized sentence (#2932).
const WRONG_ACCOUNT = 'this invitation is for a different email'

// `?outcome=` — the register form arriving with a verdict (see inviteRoute.js).
const OUTCOME_STATE = {
  email_mismatch: 'wrongAccount',
  invalid: 'fail',
  failed: 'error',
}

const isLink = computed(() => preview.value?.kind === 'link')
const invitedEmail = computed(() => preview.value?.email || '')
const myEmail = computed(() => auth.user?.email || '')
// The address that will join: the invited one, or — for a shared link — the
// signed-in account.
const shownEmail = computed(() => invitedEmail.value || (auth.isAuthenticated ? myEmail.value : ''))
const mismatch = computed(
  () =>
    auth.isAuthenticated &&
    !!invitedEmail.value &&
    !!myEmail.value &&
    invitedEmail.value.toLowerCase() !== myEmail.value.toLowerCase(),
)
const spaceName = computed(() => workspaceName(preview.value?.workspace))
const tileStyle = computed(() => {
  const c = preview.value?.workspace?.color
  return { background: c ? hueGrad(c) : 'var(--t-accent-grad)' }
})
const roleLabel = computed(() =>
  preview.value?.role === 'admin'
    ? t('common.auth.invite.roleAdmin')
    : t('common.auth.invite.roleMember'),
)

onMounted(async () => {
  const outcome = route.query.outcome
  if (outcome) {
    state.value = OUTCOME_STATE[outcome] || 'fail'
    return
  }
  if (!token) {
    state.value = 'fail'
    return
  }
  try {
    const { data } = await accountFlows.lookupInvitation(token)
    if (data?.state !== 'pending') {
      state.value = data?.state === 'declined' ? 'declined' : 'fail'
      return
    }
    preview.value = data
    state.value = 'preview'
  } catch (e) {
    state.value = e.status === 404 || e.status === 400 ? 'fail' : 'error'
  }
})

async function accept() {
  if (busy.value || state.value !== 'preview') return
  // No account yet: accepting needs a session, so the register form presents
  // the token on sign-up — the only thing that joins an invitee now.
  if (!auth.isAuthenticated) {
    router.push(registerRouteForInvite(token))
    return
  }
  if (mismatch.value) return
  busy.value = true
  state.value = 'accepting'
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
    state.value = e?.raw === WRONG_ACCOUNT ? 'wrongAccount' : 'fail'
  } finally {
    busy.value = false
  }
}

async function decline() {
  if (busy.value) return
  // A shared link names nobody: there is nothing to refuse, the page just goes.
  if (isLink.value) {
    router.push(auth.isAuthenticated ? '/' : '/login')
    return
  }
  busy.value = true
  try {
    await accountFlows.declineInvitation(token)
    state.value = 'declined'
  } catch {
    state.value = 'fail'
  } finally {
    busy.value = false
  }
}

// Sign out and return to *this* invitation (`?next` replays it after sign-in).
async function switchAccount() {
  await auth.logout()
  router.replace(loginRouteForInvite(token))
}

// ↵ accepts, like the mockup's key cap says; a field never has focus here.
function onKey(e) {
  if (e.key !== 'Enter' || e.isComposing || e.defaultPrevented) return
  if (e.target?.closest?.('button, a, input, textarea')) return
  if (state.value === 'preview' && !mismatch.value) accept()
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

// One card for every state (AuthLayout's root must stay a single element across
// them, see App.vue): the preview draws its own head, the rest are a badge, a
// title and a line.
const layoutTitle = computed(() => {
  if (state.value === 'preview') return ''
  if (state.value === 'loading' || state.value === 'accepting') return t('common.auth.invite.title')
  return t(`common.auth.invite.${state.value}Title`)
})
const stateNote = computed(() => {
  if (state.value === 'loading') return t('common.auth.invite.loading')
  if (state.value === 'accepting') return t('common.auth.invite.pending')
  return t(`common.auth.invite.${state.value}`)
})
</script>

<template>
  <auth-layout
    :title="layoutTitle"
    :icon="state === 'preview' ? null : STATE_ICON[state]"
    :tone="STATE_TONE[state] || ''"
  >
    <div v-if="state === 'preview'" class="iv" data-testid="invite-preview">
      <div class="iv-head">
        <span class="iv-tile" :style="tileStyle">{{ initialsOfSpace(spaceName) }}</span>
        <div class="iv-who">
          <span class="iv-by">
            <template v-if="preview.inviter?.name">
              <UserAvatar :name="preview.inviter.name" class="iv-av" />
              {{ $t('common.auth.invite.invites', { name: preview.inviter.name }) }}
            </template>
            <template v-else>{{ $t('common.auth.invite.invitesAnon') }}</template>
          </span>
          <span class="iv-space" data-testid="invite-space">{{ spaceName }}</span>
        </div>
      </div>
      <div class="iv-pills">
        <span class="iv-pill">{{
          $t('common.auth.invite.projects', { n: preview.projects }, preview.projects)
        }}</span>
        <span class="iv-pill">{{
          $t('common.auth.invite.members', { n: preview.members }, preview.members)
        }}</span>
        <span class="iv-pill" data-testid="invite-role">{{
          $t('common.auth.invite.role', { role: roleLabel })
        }}</span>
      </div>

      <div v-if="shownEmail" class="iv-field">
        <span class="iv-label">{{ $t('common.auth.invite.email') }}</span>
        <div class="iv-email" data-testid="invite-email">
          <span class="iv-email-v">{{ shownEmail }}</span>
          <n-icon
            v-if="auth.isAuthenticated && !mismatch"
            :component="CheckmarkCircle"
            :size="16"
            color="#18a058"
          />
          <n-icon v-else-if="mismatch" :component="AlertCircleOutline" :size="16" color="#f0a020" />
        </div>
        <span v-if="mismatch" class="iv-warn" data-testid="invite-mismatch">{{
          $t('common.auth.invite.emailMismatch', { me: myEmail })
        }}</span>
      </div>
      <p v-else-if="isLink" class="iv-note">
        <n-icon :component="LinkOutline" :size="14" />
        {{ $t('common.auth.invite.linkKind') }} ·
        {{ $t('common.auth.invite.until', { date: formatDate(preview.expires_at) }) }}
      </p>

      <n-button
        v-if="mismatch"
        block
        size="large"
        type="primary"
        class="auth-submit"
        data-testid="invite-switch-account"
        @click="switchAccount"
      >
        {{ $t('common.auth.invite.switchAccount') }}
      </n-button>
      <n-button
        v-else
        block
        size="large"
        type="primary"
        class="auth-submit"
        :loading="busy"
        data-testid="invite-accept"
        @click="accept"
      >
        {{ $t('common.auth.invite.accept') }}<Kbd>{{ CAPS.enter }}</Kbd>
      </n-button>
      <button type="button" class="iv-decline" data-testid="invite-decline" @click="decline">
        {{ $t('common.auth.invite.decline') }}
      </button>
      <div v-if="!auth.isAuthenticated" class="auth-foot iv-foot">
        {{ $t('common.auth.invite.haveAccount') }}
        <router-link :to="loginRouteForInvite(token)" data-testid="invite-login">{{
          $t('common.auth.invite.login')
        }}</router-link>
      </div>
    </div>

    <template v-else>
      <p class="note" data-testid="invite-state">{{ stateNote }}</p>
      <n-button
        v-if="state === 'wrongAccount'"
        block
        size="large"
        type="primary"
        class="auth-submit auth-gap"
        data-testid="invite-switch-account"
        @click="switchAccount"
      >
        {{ $t('common.auth.invite.switchAccount') }}
      </n-button>
      <div class="auth-foot">
        <router-link to="/">{{ $t('common.auth.toHome') }}</router-link>
      </div>
    </template>
  </auth-layout>
</template>

<style scoped>
.iv {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.iv-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.iv-tile {
  flex: none;
  width: 52px;
  height: 52px;
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 17px;
  font-weight: 700;
}
.iv-who {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.iv-by {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--t-text2);
}
.iv-av {
  flex: none;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 9px;
  font-weight: 700;
}
.iv-space {
  font-size: 20px;
  font-weight: 700;
  line-height: 1.25;
  color: var(--t-text1);
  overflow-wrap: anywhere;
}
.iv-pills {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.iv-pill {
  height: 26px;
  display: inline-flex;
  align-items: center;
  padding: 0 10px;
  border-radius: 999px;
  background: var(--t-ctl);
  font-size: 12.5px;
  color: var(--t-text2);
}
.iv-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.iv-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
}
.iv-email {
  height: 40px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  border-radius: 10px;
  background: var(--t-input-bg);
  font-size: 14px;
  color: var(--t-text1);
}
.iv-email-v {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.iv-warn {
  font-size: 12.5px;
  color: color-mix(in srgb, #f0a020 80%, var(--t-text1));
}
.iv-note {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  font-size: 13px;
  color: var(--t-text2);
}
.iv-decline {
  align-self: center;
  padding: 0;
  border: 0;
  background: none;
  font: inherit;
  font-size: 13px;
  color: var(--t-text3);
  cursor: pointer;
}
.iv-decline:hover {
  color: var(--t-text1);
}
.iv-foot {
  margin-top: 0;
}
</style>
