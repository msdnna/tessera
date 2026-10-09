<script setup>
// «Участники пространства» — a large window (v2, #2975, mockup MembersWin):
// invite by email (addresses become chips, a bad one is marked), the link of
// the latest invitation, invitations waiting for an answer and the roster with
// a role menu. Every change lands at once; revoking an invitation is undone
// from its toast, removing a member is a two-step item of the role menu.
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NTooltip, useMessage } from 'naive-ui'
import {
  PeopleOutline,
  MailOutline,
  PeopleCircleOutline,
  ShieldCheckmarkOutline,
  ChevronDownOutline,
  CloseOutline,
  AlertCircleOutline,
  LinkOutline,
  CopyOutline,
  FunnelOutline,
  LockClosedOutline,
  PersonAddOutline,
  EllipsisHorizontal,
  RefreshOutline,
  TimeOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { workspaces as wsApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useUndoStore } from '@/stores/undo'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useFormat } from '@/composables/useFormat'
import { useShortcuts } from '@/composables/useShortcuts'
import { copyText } from '@/utils/clipboard'
import { CAPS, cap } from '@/utils/keyCaps'
import { stubTask } from '@/utils/featureStubs'
import { workspaceName } from '@/utils/defaultNames'
import {
  LOCK_VIEWER,
  LOCK_OWNER,
  LOCK_SELF,
  memberRowLock,
  addMemberRefusal,
} from '@/utils/members'
import LargeWindow from './kit/LargeWindow.vue'
import ActionMenu from './kit/ActionMenu.vue'
import UserAvatar from './UserAvatar.vue'
import ShortcutKeys from './ShortcutKeys.vue'
import Kbd from './Kbd.vue'
import './kit/kit.css'

/** Which sentence explains each refusal the add field (and a locked row) hits. */
const REFUSAL_KEY = {
  [LOCK_VIEWER]: 'cannotManage',
  [LOCK_OWNER]: 'cannotOwner',
  [LOCK_SELF]: 'cannotSelf',
}

const props = defineProps({
  show: { type: Boolean, default: false },
  wsId: { type: String, default: null },
})
const emit = defineEmits(['update:show'])

const { t } = useI18n()
const { formatDate } = useFormat()
const message = useMessage()
const auth = useAuthStore()
const wsStore = useWorkspacesStore()
const undo = useUndoStore()
const members = ref([])
const invitations = ref([])
const inviteError = ref('')
const role = ref('member')
const section = ref('members')
const winRef = ref(null)

// Whether the viewer may manage anyone here at all. AddMember, UpdateMemberRole,
// RemoveMember, ListInvitations and DeleteInvitation are all `requireManager`,
// while the window itself is open to every member (ListMembers is
// `requireMember`). Read off the row for *this* workspace rather than the
// store's `canManage`, which follows the globally-selected one — the id
// arrives here as a prop.
const workspace = computed(() => wsStore.list.find((w) => w.id === props.wsId) || null)
const canManage = computed(() => ['owner', 'admin'].includes(workspace.value?.my_role))

// Who is looking: the viewer's own row and the owner's are not offered the role
// menu — see utils/members.js. `ownerId` is left to the row's own `role` field.
const who = computed(() => ({
  selfId: auth.user?.id || '',
  selfEmail: auth.user?.email || '',
  canManage: canManage.value,
}))
const rowLock = (m) => memberRowLock(m, who.value)
const isSelf = (m) => !!who.value.selfId && m.user_id === who.value.selfId
const lockReason = (m) => {
  const lock = rowLock(m)
  return lock ? t(`shell.members.${REFUSAL_KEY[lock]}`) : ''
}
const roleName = (r) =>
  ['owner', 'admin', 'member'].includes(r) ? t(`shell.members.${r}`) : r || ''
// The roles an invitation or a member can be given here — computed, so a
// language switch reaches the open menu. `value` is the wire format.
const roleOptions = computed(() => [
  { label: t('shell.members.member'), value: 'member' },
  { label: t('shell.members.admin'), value: 'admin' },
])
const owner = computed(() => members.value.find((m) => m.role === 'owner') || null)

const lastLink = ref('') // link from the most recent invitation, for copying
// Which workspace the lists above currently describe — see load().
const loadedWsId = ref('')

// An invitation lives a week (backend invitationTTL), but the list endpoint only
// drops the accepted ones — an expired invitation stays in the response forever.
// Every path that redeems one refuses it past `expires_at`, so an unmarked row
// tells the admin "invited, hasn't accepted yet" about a link that answers
// "invalid or expired invitation"; there is no unique key on (workspace, email)
// either, so re-invites pile up and only the newest of the identical-looking
// rows works. The date is what tells them apart.
const invitationRows = computed(() =>
  invitations.value.map((inv) => {
    const until = inv.expires_at ? new Date(inv.expires_at) : null
    const expired = !!until && until.getTime() <= Date.now()
    return { ...inv, expired, until: until && !expired ? formatDate(until) : '' }
  }),
)
const pendingCount = computed(() => invitationRows.value.filter((i) => !i.expired).length)

// The two lists are fetched behind different gates — ListMembers is
// `requireMember`, ListInvitations is `requireManager` — so under `Promise.all`
// a plain member's 403 on the second threw away the roster the server had
// already returned with the first. They are independent, and the manager-only
// one is not asked for by someone it would refuse.
async function load() {
  if (!props.wsId) return
  // Everything held here belongs to one workspace, and this component outlives
  // all of them (it is mounted once, `wsId` follows the sidebar). Reopening it
  // elsewhere must not show the previous workspace's roster, its invitations or
  // — worst — its invite link, which carries the token of an invitation into
  // that other workspace (#2932). Only on an actual change of workspace:
  // `load()` is also the refresh after every write, and the link is written by
  // one of them; with SMTP off it is the only copy of that token anywhere.
  const wsId = props.wsId
  if (loadedWsId.value !== wsId) {
    loadedWsId.value = wsId
    members.value = []
    invitations.value = []
    lastLink.value = ''
    inviteError.value = ''
    chips.value = []
    draft.value = ''
    filter.value = ''
  }
  const [m, inv] = await Promise.allSettled([
    wsApi.members(wsId),
    canManage.value ? wsApi.invitations(wsId) : Promise.resolve({ data: [] }),
  ])
  // Switched away while these were in flight: the answers are about the
  // workspace that is no longer on screen.
  if (wsId !== props.wsId) return
  if (m.status === 'fulfilled') members.value = m.value.data || []
  else message.error(m.reason.message)
  if (inv.status === 'fulfilled') invitations.value = inv.value.data || []
  else message.error(inv.reason.message)
}

// ── the invite field: addresses become chips ──
// Enter, a comma or a space turns the typed address into a chip; ⌫ in the empty
// field takes the last chip back. Enter on an empty field (or the button) sends
// every good chip — the typed rest first.
const draft = ref('')
const chips = ref([]) // { email, bad }
const inviteEl = ref(null)
const busy = ref(false)
const EMAIL_RE = /^[^\s@,]+@[^\s@,]+\.[^\s@,]+$/
const goodChips = computed(() => chips.value.filter((c) => !c.bad))
const badChip = computed(() => chips.value.find((c) => c.bad) || null)

function commitDraft() {
  const parts = draft.value
    .split(/[\s,;]+/)
    .map((s) => s.trim())
    .filter(Boolean)
  draft.value = ''
  for (const email of parts) {
    if (chips.value.some((c) => c.email.toLowerCase() === email.toLowerCase())) continue
    chips.value = [...chips.value, { email, bad: !EMAIL_RE.test(email) }]
  }
}
function dropChip(i) {
  chips.value = chips.value.filter((_, j) => j !== i)
  inviteError.value = ''
}
function onInviteKey(e) {
  if (e.isComposing) return
  if (e.key === 'Enter') {
    e.preventDefault()
    if (draft.value.trim()) commitDraft()
    else submitInvite()
  } else if (e.key === ',' || e.key === ' ' || e.key === ';') {
    if (!draft.value.trim()) return
    e.preventDefault()
    commitDraft()
  } else if (e.key === 'Backspace' && !draft.value && chips.value.length) {
    e.preventDefault()
    chips.value = chips.value.slice(0, -1)
    inviteError.value = ''
  } else if (e.key === 'Escape' && (draft.value || chips.value.length)) {
    e.preventDefault()
    e.stopPropagation()
    draft.value = ''
    chips.value = []
    inviteError.value = ''
  }
}
function onPaste(e) {
  const text = e.clipboardData?.getData('text') || ''
  if (!/[\s,;]/.test(text.trim())) return
  e.preventDefault()
  draft.value += text
  commitDraft()
}

// One address: add an already-registered user instantly; otherwise fall back to
// an email invitation (the link is shared by hand, or by email when SMTP is on).
// Returns true when the address was taken.
async function inviteOne(e) {
  // CreateMembership is an upsert, so an address already on the list is a role
  // change, and the caller's own address with the role on its "member" default
  // was a self-demotion answered with a green "Участник добавлен". The server
  // refuses it now; this only spares the round trip and says which rule hit.
  const refused = addMemberRefusal(e, members.value, who.value)
  if (refused) {
    inviteError.value = t(`shell.members.${REFUSAL_KEY[refused]}`)
    return false
  }
  try {
    await wsApi.addMember(props.wsId, { email: e, role: role.value })
    lastLink.value = ''
    message.success(t('shell.members.added'))
    return true
  } catch (err) {
    // `raw`, not `message`: the branch is on the server's English sentinel.
    if (err.raw === 'no user with that email') {
      try {
        const res = await wsApi.createInvitation(props.wsId, { email: e, role: role.value })
        lastLink.value = res.data.link || ''
        message.success(t('shell.members.inviteCreated'))
        return true
      } catch (err2) {
        message.error(err2.message)
      }
    } else {
      message.error(err.message)
    }
    return false
  }
}

async function submitInvite() {
  if (busy.value) return
  commitDraft()
  if (badChip.value) {
    inviteError.value = t('shell.members.badEmail', { email: badChip.value.email })
    return
  }
  const list = goodChips.value.map((c) => c.email)
  if (!list.length) return
  inviteError.value = ''
  busy.value = true
  try {
    const left = []
    for (const email of list) {
      if (!(await inviteOne(email))) left.push(email)
    }
    chips.value = chips.value.filter((c) => left.includes(c.email))
    await load()
  } finally {
    busy.value = false
  }
}

// ── role menus: the invite's role, and a member's ──
const menuFor = ref(null) // 'invite' | member user_id
const menuAnchor = ref(null)
const menuShow = computed({
  get: () => !!menuFor.value,
  set: (v) => {
    if (!v) menuFor.value = null
  },
})
const menuMember = computed(() =>
  menuFor.value && menuFor.value !== 'invite'
    ? members.value.find((m) => m.user_id === menuFor.value) || null
    : null,
)
const rolesTask = stubTask('members.roles')
const menuItems = computed(() => {
  const cur = menuFor.value === 'invite' ? role.value : menuMember.value?.role
  const items = []
  if (menuFor.value !== 'invite') {
    items.push({
      key: 'owner',
      label: t('shell.members.owner'),
      note: t('shell.members.transfer'),
      icon: ShieldCheckmarkOutline,
      stub: rolesTask,
    })
  }
  items.push(
    {
      key: 'admin',
      label: roleOptions.value[1].label,
      note: t('shell.members.adminNote'),
      icon: ShieldCheckmarkOutline,
      active: cur === 'admin',
      caps: [cap(menuFor.value === 'invite' ? 1 : 2)],
    },
    {
      key: 'member',
      label: roleOptions.value[0].label,
      note: t('shell.members.memberNote'),
      icon: PeopleOutline,
      active: cur === 'member',
      caps: [cap(menuFor.value === 'invite' ? 2 : 3)],
    },
    {
      key: 'guest',
      label: t('shell.members.guest'),
      icon: PeopleCircleOutline,
      stub: rolesTask,
    },
  )
  if (menuMember.value) {
    items.push({ gap: true })
    items.push({
      key: 'remove',
      label: t('shell.members.remove'),
      icon: TrashOutline,
      danger: true,
      caps: [CAPS.backspace],
      confirm: {
        title: t('shell.members.removeConfirm'),
        sub: menuMember.value.name || menuMember.value.email,
      },
    })
  }
  return items
})
function openMenu(key, el) {
  menuAnchor.value = el
  menuFor.value = key
}
async function onMenu(key) {
  const target = menuFor.value
  if (target === 'invite') {
    if (key === 'admin' || key === 'member') role.value = key
    return
  }
  const m = menuMember.value
  if (!m) return
  if (key === 'remove') {
    menuFor.value = null
    await remove(m.user_id)
  } else if ((key === 'admin' || key === 'member') && key !== m.role) {
    await changeRole(m.user_id, key)
  }
}

async function remove(userId) {
  try {
    await wsApi.removeMember(props.wsId, userId)
    await load()
  } catch (e) {
    message.error(e.message)
  }
}

async function changeRole(userId, newRole) {
  try {
    await wsApi.updateMemberRole(props.wsId, userId, newRole)
    await load()
    message.success(t('shell.members.roleUpdated'))
  } catch (e) {
    message.error(e.message)
    await load() // back to the server's state
  }
}

// Revoking is undone from the toast: the undo invites the same address with the
// same role again — a fresh link, since the old token is gone for good.
async function revokeInvite(inv) {
  try {
    await wsApi.deleteInvitation(props.wsId, inv.id)
    await load()
  } catch (e) {
    message.error(e.message)
    return
  }
  const wsId = props.wsId
  undo.push({
    text: { key: 'shell.members.revoked', params: { email: inv.email } },
    doneText: { key: 'shell.members.revokeUndone', params: { email: inv.email } },
    undo: async () => {
      const res = await wsApi.createInvitation(wsId, { email: inv.email, role: inv.role })
      if (wsId === props.wsId) {
        lastLink.value = res.data?.link || ''
        await load()
      }
    },
  })
}

async function copyLink() {
  if (!lastLink.value) return
  if (await copyText(lastLink.value)) message.success(t('shell.members.linkCopied'))
  else message.info(lastLink.value)
}

// ── the roster filter ──
const filter = ref('')
const filterEl = ref(null)
const shownMembers = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q) return members.value
  return members.value.filter(
    (m) => (m.name || '').toLowerCase().includes(q) || (m.email || '').toLowerCase().includes(q),
  )
})

const invitesTask = stubTask('members.inviteLink')
const navItems = computed(() => [
  {
    key: 'members',
    icon: PeopleOutline,
    label: t('shell.members.navMembers'),
    count: members.value.length || null,
  },
  ...(canManage.value
    ? [
        {
          key: 'invites',
          icon: MailOutline,
          label: t('shell.members.navInvites'),
          count: invitations.value.length || null,
        },
      ]
    : []),
  {
    key: 'groups',
    icon: PeopleCircleOutline,
    label: t('shell.members.navGroups'),
    stub: rolesTask,
  },
  {
    key: 'roles',
    icon: ShieldCheckmarkOutline,
    label: t('shell.members.navRoles'),
    stub: rolesTask,
  },
])

function focusInvite() {
  if (section.value !== 'members') section.value = 'members'
  nextTick(() => inviteEl.value?.focus())
}
// Esc inside the window: an open menu closes itself; a typed filter is cleared
// first; then the window.
function paneEscape() {
  if (menuFor.value) {
    menuFor.value = null
    return true
  }
  if (filter.value) {
    filter.value = ''
    return true
  }
  return false
}
const topmost = () => !!winRef.value?.topmost()
useShortcuts(
  {
    'members.invite': () => canManage.value && focusInvite(),
    'win.filter': () => {
      section.value = 'members'
      nextTick(() => filterEl.value?.focus())
    },
  },
  { when: topmost },
)

// «Пригласить «…»» from the assignee picker (#3118) hands an address over: it
// lands in the invite field as a chip, ready to send.
const wsDialogs = useWsDialogsStore()
function takeDraft() {
  const email = wsDialogs.draftEmail
  if (!email || !props.show) return
  wsDialogs.draftEmail = ''
  draft.value = email
  commitDraft()
}
watch(
  () => [props.show, props.wsId],
  ([show]) => {
    if (show) load().then(takeDraft)
    else menuFor.value = null
  },
)
watch(canManage, (can) => {
  if (!can && section.value === 'invites') section.value = 'members'
})

function close() {
  emit('update:show', false)
}
</script>

<template>
  <LargeWindow
    ref="winRef"
    v-model:section="section"
    :show="show"
    :crumbs="[workspaceName(workspace)]"
    :title="t('shell.members.windowTitle')"
    :icon="PeopleOutline"
    :items="navItems"
    :escape="paneEscape"
    testid="members-window"
    tab-testid="members-tab"
    @update:show="(v) => !v && close()"
  >
    <template #nav-foot>
      <div v-if="owner" class="lw-foot-row mw-foot">
        <UserAvatar :user-id="owner.user_id" :name="owner.name" class="mw-foot-av" />
        <span class="mw-foot-t">{{ t('shell.members.ownerIs', { name: owner.name }) }}</span>
      </div>
      <div v-if="canManage" class="lw-foot-row">
        <ShortcutKeys id="members.invite" />
        <span>{{ t('shell.members.inviteKey') }}</span>
      </div>
    </template>

    <div class="mw" data-testid="members-pane">
      <div class="mw-title">
        <h2>
          {{
            section === 'invites' ? t('shell.members.navInvites') : t('shell.members.navMembers')
          }}
        </h2>
        <span class="mw-note">
          {{ t('shell.members.summary', members.length) }}
          <template v-if="canManage && pendingCount">
            · {{ t('shell.members.pendingSummary', pendingCount) }}
          </template>
        </span>
      </div>

      <div class="mw-scroll t-hoverscroll">
        <!-- Every door below this line is requireManager. Saying so once beats
             controls that answer 403, and beats silence — the roster is still
             worth the trip. -->
        <p v-if="!canManage" class="mw-readonly hint">{{ t('shell.members.readOnly') }}</p>

        <section v-else-if="section === 'members'" class="invite mw-invite">
          <div class="mw-inv-row">
            <label
              class="mw-chipfield"
              :class="{ error: !!inviteError }"
              data-testid="members-invite-field"
            >
              <span
                v-for="(c, i) in chips"
                :key="c.email"
                class="mw-chip"
                :class="{ bad: c.bad }"
                data-testid="invite-chip"
              >
                <n-icon v-if="c.bad" :component="AlertCircleOutline" :size="13" />
                {{ c.email }}
                <button
                  type="button"
                  class="mw-chip-x"
                  tabindex="-1"
                  :aria-label="t('common.action.delete')"
                  @click.prevent="dropChip(i)"
                >
                  <n-icon :component="CloseOutline" :size="12" />
                </button>
              </span>
              <input
                ref="inviteEl"
                v-model="draft"
                type="text"
                inputmode="email"
                autocomplete="off"
                spellcheck="false"
                :placeholder="chips.length ? '' : t('shell.members.emailPlaceholder')"
                data-testid="members-invite-input"
                @input="inviteError = ''"
                @keydown="onInviteKey"
                @paste="onPaste"
                @blur="commitDraft"
              />
            </label>
            <button
              type="button"
              class="mw-rolebtn"
              data-testid="members-invite-role"
              @click="openMenu('invite', $event.currentTarget)"
            >
              {{ roleName(role) }}
              <n-icon :component="ChevronDownOutline" :size="13" />
            </button>
            <button
              type="button"
              class="kit-btn primary mw-send"
              :disabled="busy || (!draft.trim() && !chips.length)"
              data-testid="members-invite-submit"
              @click="submitInvite"
            >
              <n-icon :component="PersonAddOutline" :size="15" />
              {{ t('shell.members.inviteN', { n: goodChips.length || '' }) }}
              <Kbd>{{ CAPS.enter }}</Kbd>
            </button>
          </div>
          <div class="mw-inv-meta">
            <span v-if="inviteError" class="invite-err">
              <n-icon :component="AlertCircleOutline" :size="13" />
              {{ inviteError }}
            </span>
            <span v-else class="mw-inv-hint">{{ t('shell.members.inviteHint') }}</span>
            <span class="mw-grow" />
            <span class="mw-inv-keys">
              <Kbd soft>{{ CAPS.comma }}</Kbd>
              <span>{{ t('shell.members.or') }}</span>
              <Kbd soft>{{ CAPS.space }}</Kbd>
              <span>{{ t('shell.members.nextAddress') }}</span>
            </span>
          </div>
          <div class="mw-link">
            <n-icon :component="LinkOutline" :size="15" class="mw-link-ic" />
            <span class="mw-link-l">{{ t('shell.members.linkLabel') }}</span>
            <code v-if="lastLink" class="mw-link-v link-box" data-testid="members-invite-link">{{
              lastLink
            }}</code>
            <span v-else class="mw-link-empty">{{ t('shell.members.linkEmpty') }}</span>
            <span class="mw-grow" />
            <button
              v-if="lastLink"
              type="button"
              class="kit-btn small"
              data-testid="members-copy-link"
              @click="copyLink"
            >
              <n-icon :component="CopyOutline" :size="14" />
              {{ t('shell.members.copy') }}
            </button>
            <n-tooltip :delay="300">
              <template #trigger>
                <span
                  class="kit-btn small stub"
                  aria-disabled="true"
                  data-testid="members-link-stub"
                >
                  <n-icon :component="RefreshOutline" :size="14" />
                  {{ t('shell.members.sharedLink') }}
                </span>
              </template>
              {{ t('common.stub.soon', { n: invitesTask }) }}
            </n-tooltip>
          </div>
        </section>

        <!-- Invitations waiting for an answer — on «Участники» under the field,
             and the whole of «Приглашения». -->
        <template v-if="canManage && invitationRows.length">
          <div v-if="section === 'members'" class="mw-h">
            {{ t('shell.members.waiting') }} <span class="mw-h-n">{{ invitationRows.length }}</span>
          </div>
          <div class="invites mw-list">
            <div
              v-for="inv in invitationRows"
              :key="inv.id"
              class="invite-row mw-row"
              :class="{ 'is-expired': inv.expired }"
              data-testid="invite-row"
            >
              <span class="mw-tile inv-icon"><n-icon :component="MailOutline" :size="15" /></span>
              <span class="inv-mail">{{ inv.email }}</span>
              <span class="mw-role" :class="`r-${inv.role}`">{{ roleName(inv.role) }}</span>
              <span v-if="inv.expired" class="inv-expired mw-expired">
                {{ t('shell.members.inviteExpired') }}
              </span>
              <span v-else-if="inv.until" class="inv-until">
                <n-icon :component="TimeOutline" :size="12" />
                {{ t('shell.members.inviteUntil', { date: inv.until }) }}
              </span>
              <span class="mw-grow" />
              <n-tooltip :delay="300">
                <template #trigger>
                  <span class="mw-act stub" aria-disabled="true">{{
                    inv.expired ? t('shell.members.extend') : t('shell.members.resend')
                  }}</span>
                </template>
                {{ t('common.stub.soon', { n: invitesTask }) }}
              </n-tooltip>
              <!-- Revoke stays on an expired row: clearing it out of the list is
                   the only thing left to do with it. -->
              <button
                type="button"
                class="mw-act"
                data-testid="invite-revoke"
                @click="revokeInvite(inv)"
              >
                {{ t('shell.members.revoke') }}
              </button>
            </div>
          </div>
        </template>
        <p v-else-if="section === 'invites'" class="mw-empty">
          {{ t('shell.members.noInvites') }}
        </p>

        <template v-if="section === 'members'">
          <div class="mw-h">
            {{ t('shell.members.navMembers') }} <span class="mw-h-n">{{ members.length }}</span>
            <span class="mw-grow" />
            <label class="mw-filter">
              <n-icon :component="FunnelOutline" :size="13" class="mw-filter-ic" />
              <input
                ref="filterEl"
                v-model="filter"
                spellcheck="false"
                :placeholder="t('shell.members.filter')"
                data-testid="members-filter"
                @keydown.esc.stop="((filter = ''), $event.target.blur())"
              />
              <Kbd soft>/</Kbd>
            </label>
          </div>
          <div class="members mw-list">
            <div
              v-for="m in shownMembers"
              :key="m.user_id"
              class="member mw-row"
              :class="{ ctx: menuFor === m.user_id }"
              data-testid="member-row"
            >
              <UserAvatar :user-id="m.user_id" :name="m.name" class="mw-av" />
              <div class="info">
                <span class="name">
                  {{ m.name }}
                  <span v-if="isSelf(m)" class="mw-you">{{ t('shell.members.youShort') }}</span>
                </span>
                <span class="mail">{{ m.email }}</span>
              </div>
              <!-- A control that vanishes with no word reads as a broken window,
                   so a locked row keeps its role in place and says why on hover. -->
              <span
                v-if="rowLock(m)"
                class="mw-role locked"
                :class="`r-${m.role}`"
                :title="lockReason(m)"
                :aria-label="`${roleName(m.role)} — ${lockReason(m)}`"
                data-testid="member-role-locked"
              >
                {{ roleName(m.role) }}
                <n-icon
                  v-if="canManage"
                  :component="LockClosedOutline"
                  :size="11"
                  class="mw-lock"
                />
              </span>
              <button
                v-else
                type="button"
                class="mw-role btn"
                :class="`r-${m.role}`"
                data-testid="member-role"
                @click="openMenu(m.user_id, $event.currentTarget)"
              >
                {{ roleName(m.role) }}
                <n-icon :component="ChevronDownOutline" :size="12" />
              </button>
              <button
                v-if="!rowLock(m)"
                type="button"
                class="mw-more"
                :title="t('common.action.more')"
                data-testid="member-menu"
                @click="openMenu(m.user_id, $event.currentTarget)"
              >
                <n-icon :component="EllipsisHorizontal" :size="16" />
              </button>
            </div>
            <p v-if="!shownMembers.length && filter" class="mw-empty">
              {{ t('shell.members.noMatch') }}
            </p>
          </div>
        </template>
      </div>
    </div>

    <ActionMenu
      v-model:show="menuShow"
      :anchor="() => menuAnchor"
      placement="bottom-end"
      :offset="{ x: 0, y: 6 }"
      :items="menuItems"
      :width="360"
      testid="member-role-menu"
      @select="onMenu"
    />
  </LargeWindow>
</template>

<style scoped>
.mw {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  padding: 6px 16px 0 0;
}
.lw-mobile .mw {
  padding-right: 0;
}
.mw-title {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px 10px;
  margin-bottom: 16px;
}
.mw-title h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
}
.mw-note {
  font-size: 13px;
  color: var(--t-text3);
}
.mw-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 2px 24px;
}
.mw-readonly {
  margin: 0 0 14px;
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--t-sub);
  font-size: 13px;
  color: var(--t-text2);
}
/* The invite block — on --t-sub, radius 14. */
.mw-invite {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border-radius: 14px;
  background: var(--t-sub);
}
.mw-inv-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.mw-chipfield {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  min-height: 38px;
  padding: 4px 8px;
  box-sizing: border-box;
  border-radius: var(--t-r-ctl);
  background: var(--t-input-bg);
  cursor: text;
}
.mw-chipfield:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.mw-chipfield.error {
  box-shadow: 0 0 0 2px var(--t-error);
}
.mw-chipfield input {
  flex: 1;
  min-width: 120px;
  height: 28px;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
}
.mw-chipfield input::placeholder {
  color: var(--t-placeholder);
}
.mw-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 24px;
  padding: 0 4px 0 9px;
  border-radius: 999px;
  background: var(--t-pill);
  color: var(--t-text1);
  font-size: 12.5px;
}
.mw-chip.bad {
  background: color-mix(in srgb, var(--t-error) 16%, transparent);
  color: var(--t-error);
}
.mw-chip-x {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: 0;
  border-radius: 50%;
  background: transparent;
  color: inherit;
  opacity: 0.7;
  cursor: pointer;
}
.mw-chip-x:hover {
  opacity: 1;
  background: var(--t-hover);
}
.mw-rolebtn {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 38px;
  padding: 0 12px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.mw-rolebtn:hover {
  background: var(--t-hover);
}
.mw-send {
  height: 38px;
  flex: none;
}
.mw-inv-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-size: 12px;
  color: var(--t-text3);
}
.invite-err {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--t-error);
}
.mw-inv-hint {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mw-inv-keys {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.mw-link {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 36px;
  min-width: 0;
  font-size: 13px;
}
.mw-link-ic {
  color: var(--t-text3);
  flex: none;
}
.mw-link-l {
  flex: none;
  font-weight: 600;
}
.mw-link-v {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 3px 8px;
  border-radius: 7px;
  background: var(--t-pill);
  font-size: 12.5px;
}
.mw-link-empty {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--t-text3);
  font-size: 12.5px;
}
.stub,
.kit-btn.stub {
  opacity: 0.55;
  cursor: default;
}
.mw-h {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 22px 0 8px;
  font-size: 13.5px;
  font-weight: 650;
}
.mw-h-n {
  font-weight: 500;
  color: var(--t-text3);
}
.mw-grow {
  flex: 1;
}
.mw-filter {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 220px;
  height: 32px;
  padding: 0 6px 0 10px;
  box-sizing: border-box;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  font-weight: 400;
}
.mw-filter:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.mw-filter input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
}
.mw-filter input::placeholder {
  color: var(--t-placeholder);
}
.mw-filter-ic {
  color: var(--t-text3);
}
.mw-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.mw-row {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 46px;
  padding: 3px 10px;
  border-radius: 12px;
}
.mw-row:hover,
.mw-row.ctx {
  background: var(--t-hover);
}
.mw-av {
  flex: none;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 12px;
  font-weight: 650;
}
.info {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 13.5px;
  color: var(--t-text1);
}
.mail {
  font-size: 12px;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mw-you {
  padding: 1px 6px;
  border-radius: 6px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 10.5px;
  font-weight: 650;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
/* Role pills: the owner in the accent, an admin in blue, the rest neutral. */
.mw-role {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 26px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
}
.mw-role.btn {
  cursor: pointer;
}
.mw-role.btn:hover {
  box-shadow: inset 0 0 0 1px var(--t-primary);
}
.mw-role.btn:focus-visible {
  outline: 0;
  box-shadow: 0 0 0 2px var(--t-primary);
}
.mw-role.r-owner {
  background: var(--t-accent-grad);
  color: #fff;
}
.mw-role.r-admin {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #3b82f6 14%, transparent),
    color-mix(in srgb, #3b82f6 22%, transparent)
  );
  color: color-mix(in srgb, #3b82f6 80%, var(--t-text1));
}
.mw-lock {
  opacity: 0.75;
}
.mw-more {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.mw-more:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.mw-tile {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 9px;
  background: var(--t-pill);
  color: var(--t-text3);
}
.inv-mail {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13.5px;
}
.inv-until {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.mw-expired {
  padding: 2px 8px;
  border-radius: 7px;
  background: color-mix(in srgb, var(--t-error) 14%, transparent);
  color: var(--t-error);
  font-size: 12px;
  font-weight: 600;
}
/* A dead row still belongs in the list (it is the only place to revoke it), it
   just must not read as one that is still waiting for an answer. */
.invite-row.is-expired .inv-mail,
.invite-row.is-expired .inv-icon {
  opacity: 0.6;
}
.mw-act {
  flex: none;
  height: 28px;
  padding: 0 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
}
.mw-act:not(.stub):hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.mw-empty {
  margin: 18px 0;
  text-align: center;
  font-size: 13px;
  color: var(--t-text3);
}
.mw-foot {
  min-width: 0;
}
.mw-foot-av {
  flex: none;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 7.5px;
  font-weight: 700;
}
.mw-foot-t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* Phone: the field, the role and the button stack; rows keep their pill. */
.lw-mobile .mw-inv-row {
  flex-wrap: wrap;
}
.lw-mobile .mw-chipfield {
  flex-basis: 100%;
}
.lw-mobile .mw-send {
  flex: 1;
}
.lw-mobile .mw-inv-keys,
.lw-mobile .mw-inv-hint,
.lw-mobile .mw-link-empty,
.lw-mobile .mw-send :deep(.kbd),
.lw-mobile .mw-filter :deep(.kbd) {
  display: none;
}
.lw-mobile .mw-filter {
  width: 150px;
}
.lw-mobile .mw-link {
  flex-wrap: wrap;
}
.lw-mobile .invite-row {
  flex-wrap: wrap;
}
</style>
