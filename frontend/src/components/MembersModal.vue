<script setup>
// «Участники пространства» — a large window (v2, #2975, mockup MembersWin):
// invite by email (addresses become chips, a bad one is marked), the
// workspace's shared invitation link (#3062: role, lifetime and limit in its
// menu, «Копировать ⌘C», «Сбросить»), invitations waiting for an answer with
// «Ещё раз» / «Продлить» (#3061) and the roster with a role menu. Every change
// lands at once; revoking an invitation is undone from its toast, removing a
// member is a two-step item of the role menu.
// invite by email (addresses become chips, a bad one is marked), the link of
// the latest invitation, invitations waiting for an answer and the roster with
// a role menu. Every change lands at once; revoking an invitation is undone
// from its toast, removing a member is a two-step item of the role menu.
//
// Members v2 (#3063): each row says the member's groups and when they were
// last here; the role menu hands the workspace over («Передать владение…», the
// owner only, the name typed in); the reader's own row leaves the workspace;
// access requests wait above the invitations; «Группы» and «Роли и права» are
// real sections. The guest role and the role editor stay «Скоро» (#3075/#3076).
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
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
  LogOutOutline,
  HandRightOutline,
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
import { linkMeta, linkUses, shortLink, LINK_TTL_DAYS, LINK_USES } from '@/utils/inviteLink'
import { workspaceName } from '@/utils/defaultNames'
import { serverBase } from '@/utils/serverBase'
import { humanizeError } from '@/utils/errors'
import { useMembersStore } from '@/stores/members'
import { lastActivity, activityLabel, groupsOf } from '@/utils/memberActivity'
import {
  LOCK_VIEWER,
  LOCK_OWNER,
  LOCK_SELF,
  memberRowLock,
  addMemberRefusal,
} from '@/utils/members'
import LargeWindow from './kit/LargeWindow.vue'
import TransferOwnerPop from './members/TransferOwnerPop.vue'
import MemberGroupsPane from './members/MemberGroupsPane.vue'
import RolesPane from './members/RolesPane.vue'
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
const { formatDate, formatRelative } = useFormat()
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
const myRole = computed(() => workspace.value?.my_role || '')
const canManage = computed(() => ['owner', 'admin'].includes(myRole.value))
const isOwner = computed(() => myRole.value === 'owner')
const membersStore = useMembersStore()
const groups = computed(() => membersStore.groupsFor(props.wsId))
const accessReqs = ref([])

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
//
// A declined one (#3059) is dead the same way, and says so; «Продлить» brings
// either back to life (#3061).
const invitationRows = computed(() =>
  invitations.value.map((inv) => {
    const until = inv.expires_at ? new Date(inv.expires_at) : null
    const expired = !!until && until.getTime() <= Date.now()
    const declined = !!inv.declined_at
    // formatRelative is silent past this week (sent last Saturday, read on a
    // Monday) — the row then fell back to no «отправлено» at all; a date says it.
    const rel = inv.sent_at
      ? formatRelative(inv.sent_at) || formatDate(inv.sent_at, { day: 'numeric', month: 'short' })
      : ''
    return {
      ...inv,
      expired,
      declined,
      dead: expired || declined,
      until: until && !expired ? formatDate(until) : '',
      sent: rel ? rel.charAt(0).toLowerCase() + rel.slice(1) : '',
    }
  }),
)
const pendingCount = computed(() => invitationRows.value.filter((i) => !i.dead).length)

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
    accessReqs.value = []
    lastLink.value = ''
    shared.value = null
    sharedUrl.value = ''
    inviteError.value = ''
    chips.value = []
    draft.value = ''
    filter.value = ''
  }
  const [m, inv, link, reqs] = await Promise.allSettled([
    wsApi.members(wsId),
    canManage.value ? wsApi.invitations(wsId) : Promise.resolve({ data: [] }),
    canManage.value
      ? Promise.resolve().then(() => wsApi.inviteLink(wsId))
      : Promise.resolve({ data: { link: null } }),
    canManage.value ? wsApi.accessRequests(wsId) : Promise.resolve({ data: [] }),
    membersStore.loadGroups(wsId, { force: true }),
  ])
  // Switched away while these were in flight: the answers are about the
  // workspace that is no longer on screen.
  if (wsId !== props.wsId) return
  if (m.status === 'fulfilled') members.value = m.value.data || []
  else message.error(m.reason.message)
  if (inv.status === 'fulfilled') invitations.value = inv.value.data || []
  else message.error(inv.reason.message)
  // The shared link is a convenience on top of the list: a failed read leaves
  // the row offering to create one rather than a second error toast.
  const live = link.status === 'fulfilled' ? link.value?.data?.link || null : null
  if (!live || live.id !== shared.value?.id) sharedUrl.value = ''
  shared.value = live
  if (reqs.status === 'fulfilled') accessReqs.value = reqs.value.data || []
}

// ── a member's line: groups and activity (#3073/#3074) ──
const now = ref(Date.now())
function memberGroups(m) {
  return groupsOf(m.user_id, groups.value).join(', ')
}
function activity(m) {
  const a = lastActivity(m, now.value)
  const l = activityLabel(a.at, now.value)
  const text =
    l.key === 'date'
      ? formatDate(new Date(a.at), { day: 'numeric', month: 'short' })
      : t(`shell.members.activity.${l.key}`, { n: l.n }, l.n || 1)
  const parts = []
  if (m.last_seen_at)
    parts.push(
      t('shell.members.seenAt', {
        at: formatDate(new Date(m.last_seen_at), {
          day: 'numeric',
          month: 'short',
          hour: '2-digit',
          minute: '2-digit',
        }),
      }),
    )
  if (m.last_active_at)
    parts.push(
      t('shell.members.activeAt', {
        at: formatDate(new Date(m.last_active_at), {
          day: 'numeric',
          month: 'short',
          hour: '2-digit',
          minute: '2-digit',
        }),
      }),
    )
  return { text, online: a.online, title: parts.join(' · ') }
}

// ── access requests (#3077) ──
const reqBusy = ref('')
async function decide(r, approve) {
  if (reqBusy.value) return
  reqBusy.value = r.id
  try {
    if (approve) {
      await wsApi.approveAccess(props.wsId, r.id, 'member')
      message.success(t('shell.members.requestApproved', { name: r.user_name || r.user_email }))
    } else {
      await wsApi.declineAccess(props.wsId, r.id)
      message.info(t('shell.members.requestDeclined', { name: r.user_name || r.user_email }))
    }
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    reqBusy.value = ''
    await load()
  }
}
function reqWhen(r) {
  const l = activityLabel(new Date(r.created_at).getTime(), now.value)
  return l.key === 'date'
    ? formatDate(new Date(r.created_at), { day: 'numeric', month: 'short' })
    : t(`shell.members.activity.${l.key}`, { n: l.n }, l.n || 1)
}

// ── the workspace's own link: a non-member who opens it can ask to be let in ──
const spaceLink = computed(() =>
  props.wsId ? `${serverBase() || window.location.origin}/space/${props.wsId}` : '',
)
async function copySpaceLink() {
  if (!spaceLink.value) return
  if (await copyText(spaceLink.value)) message.success(t('shell.members.linkCopied'))
  else message.info(spaceLink.value)
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
        lastLinkFor.value = e
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
  menuFor.value &&
  menuFor.value !== 'invite' &&
  menuFor.value !== 'link' &&
  menuFor.value !== 'self'
    ? members.value.find((m) => m.user_id === menuFor.value) || null
    : null,
)
const guestTask = stubTask('members.guests')
const menuItems = computed(() => {
  if (menuFor.value === 'link') return linkMenuItems.value
  // The reader's own row: the one thing to do with it is to leave (#3063). The
  // owner can't — the workspace would be left without one — and is told so.
  if (menuFor.value === 'self') {
    return [
      {
        key: 'leave',
        label: t('shell.members.leave'),
        icon: LogOutOutline,
        danger: true,
        testid: 'member-leave',
        ...(isOwner.value
          ? { disabled: true, disabledTitle: t('shell.members.leaveOwner') }
          : {
              confirm: {
                title: t('shell.members.leaveConfirm'),
                sub: workspaceName(workspace.value),
              },
            }),
      },
    ]
  }
  const cur = menuFor.value === 'invite' ? role.value : menuMember.value?.role
  const items = []
  if (menuFor.value !== 'invite') {
    // «Владелец — передать владение…»: the owner's alone (the server says the
    // same); an admin sees why it is greyed.
    items.push({
      key: 'owner',
      label: t('shell.members.owner'),
      note: t('shell.members.transfer'),
      icon: ShieldCheckmarkOutline,
      caps: [cap(1)],
      testid: 'member-transfer',
      ...(isOwner.value
        ? {}
        : { disabled: true, disabledTitle: t('shell.members.transferOwnerOnly') }),
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
      note: t('shell.members.guestNote'),
      icon: PeopleCircleOutline,
      stub: guestTask,
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
  if (target === 'link') {
    await onLinkMenu(key)
    return
  }
  if (target === 'invite') {
    if (key === 'admin' || key === 'member') role.value = key
    return
  }
  if (target === 'self') {
    if (key === 'leave') await leave()
    return
  }
  const m = menuMember.value
  if (!m) return
  if (key === 'owner') {
    if (!isOwner.value) return
    transferFor.value = m
    menuFor.value = null
    transferShow.value = true
    return
  }
  if (key === 'remove') {
    menuFor.value = null
    await remove(m.user_id)
  } else if ((key === 'admin' || key === 'member') && key !== m.role) {
    await changeRole(m.user_id, key)
  }
}

// ── handing the workspace over (#3072) ──
const transferFor = ref(null)
const transferShow = ref(false)
async function onTransferred(m) {
  message.success(t('shell.members.transferDone', { name: m.name || m.email }))
  // The server dropped our socket so the role re-reads on reconnect; the list
  // (my_role, hence every manager control here) is re-read now rather than
  // whenever that lands.
  await wsStore.resync()
  await load()
}

// ── leaving (#3063) ──
async function leave() {
  menuFor.value = null
  try {
    await wsApi.leave(props.wsId)
  } catch (e) {
    message.error(humanizeError(e))
    return
  }
  message.success(t('shell.members.left', { name: workspaceName(workspace.value) }))
  close()
  await wsStore.resync()
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

// «Ещё раз» on a waiting invitation, «Продлить» on an expired or declined one
// (#3061): the server issues a new link and a fresh week and mails it again —
// the new link is offered for copying, as after an invitation.
const resending = ref('')
async function resendInvite(inv) {
  if (resending.value) return
  resending.value = inv.id
  try {
    const res = await wsApi.resendInvitation(props.wsId, inv.id)
    lastLink.value = res.data?.link || ''
    lastLinkFor.value = inv.email
    message.success(
      t(inv.dead ? 'shell.members.extended' : 'shell.members.resent', { email: inv.email }),
    )
    await load()
  } catch (e) {
    if (e.status === 429) message.info(t('shell.members.resendWait'))
    else message.error(e.message)
  } finally {
    resending.value = ''
  }
}

// ── the shared invitation link (#3062) ──
// One live link per workspace. Only its hash is kept on the server, so the URL
// exists here only after this window created it — reopened later, the row says
// the link is hidden and «Сбросить» issues a fresh one. Any change of its role,
// lifetime or limit issues a new link too (the old one stops working).
const shared = ref(null)
const sharedUrl = ref('')
const sharedBusy = ref(false)
const sharedMeta = computed(() => (shared.value ? linkMeta(shared.value, t) : ''))
const sharedUses = computed(() => (shared.value ? linkUses(shared.value, t) : ''))
const sharedShort = computed(() => shortLink(sharedUrl.value))

async function issueShared(params, toastKey) {
  if (sharedBusy.value) return
  sharedBusy.value = true
  const wsId = props.wsId
  try {
    const res = await wsApi.createInviteLink(wsId, params)
    if (wsId !== props.wsId) return
    shared.value = res.data?.link || null
    sharedUrl.value = res.data?.link?.url || ''
    if (toastKey) message.success(t(toastKey))
  } catch (e) {
    message.error(e.message)
  } finally {
    sharedBusy.value = false
  }
}
function currentParams(over = {}) {
  const s = shared.value
  const days = s
    ? Math.max(1, Math.round((new Date(s.expires_at) - new Date(s.created_at)) / 864e5))
    : 7
  return {
    role: s?.role || 'member',
    ttl_days: LINK_TTL_DAYS.includes(days) ? days : 7,
    max_uses: s?.max_uses || 25,
    ...over,
  }
}
function createShared() {
  return issueShared(currentParams(), 'shell.members.sharedCreated')
}
function resetShared() {
  return issueShared(currentParams(), 'shell.members.sharedReset')
}
async function revokeShared() {
  try {
    await wsApi.revokeInviteLink(props.wsId)
    shared.value = null
    sharedUrl.value = ''
    message.success(t('shell.members.sharedOff'))
  } catch (e) {
    message.error(e.message)
  }
}
async function copyShared() {
  if (!sharedUrl.value) return
  if (await copyText(sharedUrl.value)) message.success(t('shell.members.linkCopied'))
  else message.info(sharedUrl.value)
}
const linkMenuItems = computed(() => {
  const p = currentParams()
  return [
    {
      key: 'link-role',
      label: t('shell.members.sharedRole'),
      note: roleName(p.role),
      icon: ShieldCheckmarkOutline,
      children: [
        { key: 'role:member', label: roleName('member'), checked: p.role === 'member' },
        { key: 'role:admin', label: roleName('admin'), checked: p.role === 'admin' },
      ],
    },
    {
      key: 'link-ttl',
      label: t('shell.members.sharedTtl'),
      note: t('shell.members.days', { n: p.ttl_days }, p.ttl_days),
      icon: TimeOutline,
      children: LINK_TTL_DAYS.map((d) => ({
        key: `ttl:${d}`,
        label: t('shell.members.days', { n: d }, d),
        checked: p.ttl_days === d,
      })),
    },
    {
      key: 'link-uses',
      label: t('shell.members.sharedUses'),
      note: shared.value
        ? sharedUses.value
        : t('shell.members.people', { n: p.max_uses }, p.max_uses),
      icon: PeopleOutline,
      children: LINK_USES.map((n) => ({
        key: `uses:${n}`,
        label: t('shell.members.people', { n }, n),
        checked: p.max_uses === n,
      })),
    },
    { gap: true },
    {
      key: 'link-off',
      label: t('shell.members.sharedRevoke'),
      icon: TrashOutline,
      danger: true,
      confirm: { title: t('shell.members.sharedRevokeConfirm'), sub: sharedShort.value || '' },
      testid: 'members-shared-off',
    },
  ]
})
async function onLinkMenu(key) {
  menuFor.value = null
  if (key === 'link-off') return revokeShared()
  const [kind, value] = String(key).split(':')
  if (!value) return
  const over =
    kind === 'role' ? { role: value } : kind === 'ttl' ? { ttl_days: +value } : { max_uses: +value }
  await issueShared(currentParams(over), shared.value ? 'shell.members.sharedChanged' : '')
}
const lastLinkFor = ref('')

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

const sectionTitle = computed(
  () =>
    ({
      invites: t('shell.members.navInvites'),
      groups: t('shell.members.navGroups'),
      roles: t('shell.members.navRoles'),
    })[section.value] || t('shell.members.navMembers'),
)
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
          count: invitations.value.length + accessReqs.value.length || null,
        },
      ]
    : []),
  {
    key: 'groups',
    icon: PeopleCircleOutline,
    label: t('shell.members.navGroups'),
    count: groups.value.length || null,
  },
  {
    key: 'roles',
    icon: ShieldCheckmarkOutline,
    label: t('shell.members.navRoles'),
  },
])

function focusInvite() {
  if (section.value !== 'members') section.value = 'members'
  nextTick(() => inviteEl.value?.focus())
}
// Esc inside the window: an open menu closes itself; a typed filter is cleared
// first; then the window.
const groupsPane = ref(null)
function paneEscape() {
  if (transferShow.value) {
    transferShow.value = false
    return true
  }
  if (section.value === 'groups' && groupsPane.value?.escape()) return true
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
    // N on «Группы» names a new group.
    'win.new': () => {
      if (section.value === 'groups' && canManage.value) groupsPane.value?.focusNew()
    },
    'win.filter': () => {
      section.value = 'members'
      nextTick(() => filterEl.value?.focus())
    },
  },
  { when: topmost },
)
// ⌘C copies the shared link — only while there is one to copy and nothing is
// selected: copying text out of the window keeps working as everywhere.
useShortcuts(
  { 'members.copyLink': copyShared },
  {
    when: () =>
      topmost() &&
      !!sharedUrl.value &&
      section.value === 'members' &&
      !String(window.getSelection?.() || ''),
  },
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
    if (show) {
      now.value = Date.now()
      load().then(takeDraft)
    } else {
      menuFor.value = null
      transferShow.value = false
    }
  },
)
// The roster, the access requests or the groups changed elsewhere (socket,
// stores/members): re-read while the window is open.
watch(
  () => membersStore.rev[props.wsId],
  () => props.show && load(),
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
        <h2>{{ sectionTitle }}</h2>
        <span v-if="section === 'groups'" class="mw-note">
          {{ t('shell.members.groupsSummary', groups.length) }}
        </span>
        <span v-else-if="section !== 'roles'" class="mw-note">
          {{ t('shell.members.summary', members.length) }}
          <template v-if="canManage && pendingCount">
            · {{ t('shell.members.pendingSummary', pendingCount) }}
          </template>
          <template v-if="canManage && accessReqs.length">
            · {{ t('shell.members.requestsSummary', accessReqs.length) }}
          </template>
        </span>
      </div>

      <div v-if="section === 'groups'" class="mw-scroll t-hoverscroll">
        <MemberGroupsPane
          ref="groupsPane"
          :ws-id="wsId"
          :members="members"
          :can-manage="canManage"
        />
      </div>
      <div v-else-if="section === 'roles'" class="mw-scroll t-hoverscroll">
        <RolesPane :my-role="myRole" />
      </div>
      <div v-else class="mw-scroll t-hoverscroll">
        <!-- Every door below this line is requireManager. Saying so once beats
             controls that answer 403, and beats silence — the roster is still
             worth the trip. -->
        <template v-if="!canManage">
          <p class="mw-readonly hint">{{ t('shell.members.readOnly') }}</p>
          <div class="mw-link" data-testid="members-space-link-row">
            <n-icon :component="LinkOutline" :size="15" class="mw-link-ic" />
            <span class="mw-link-l">{{ t('shell.members.spaceLink') }}</span>
            <span class="mw-link-empty">{{ t('shell.members.spaceLinkHint') }}</span>
            <span class="mw-grow" />
            <button type="button" class="kit-btn small" @click="copySpaceLink">
              <n-icon :component="CopyOutline" :size="14" />
              {{ t('shell.members.copy') }}
            </button>
          </div>
        </template>

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
          <!-- The link of the invitation just sent: with SMTP off it is the only
               copy of that token anywhere (#2932). -->
          <div v-if="lastLink" class="mw-link mw-last">
            <n-icon :component="MailOutline" :size="15" class="mw-link-ic" />
            <span class="mw-link-l">{{
              lastLinkFor
                ? t('shell.members.lastLinkFor', { email: lastLinkFor })
                : t('shell.members.lastLink')
            }}</span>
            <code class="mw-link-v link-box" data-testid="members-invite-link">{{ lastLink }}</code>
            <span class="mw-grow" />
            <button
              type="button"
              class="kit-btn small"
              data-testid="members-copy-link"
              @click="copyLink"
            >
              <n-icon :component="CopyOutline" :size="14" />
              {{ t('shell.members.copy') }}
            </button>
          </div>
          <!-- The workspace's shared link (#3062). -->
          <div class="mw-link mw-shared" data-testid="members-shared-link">
            <n-icon :component="LinkOutline" :size="15" class="mw-link-ic" />
            <span class="mw-link-l">{{ t('shell.members.linkLabel') }}</span>
            <template v-if="shared">
              <code
                v-if="sharedUrl"
                class="mw-link-v link-box"
                :title="sharedUrl"
                data-testid="members-shared-url"
                >{{ sharedShort }}</code
              >
              <span
                v-else
                class="mw-link-empty mw-link-hidden"
                :title="t('shell.members.sharedHidden')"
                data-testid="members-shared-hidden"
                >{{ t('shell.members.sharedHiddenShort') }}</span
              >
              <button
                type="button"
                class="mw-link-meta"
                :class="{ ctx: menuFor === 'link' }"
                :title="t('shell.members.sharedUsesTitle', { uses: sharedUses })"
                data-testid="members-shared-meta"
                @click="openMenu('link', $event.currentTarget)"
              >
                {{ sharedMeta }}
                <n-icon :component="ChevronDownOutline" :size="12" />
              </button>
              <span class="mw-grow" />
              <button
                v-if="sharedUrl"
                type="button"
                class="kit-btn small"
                data-testid="members-shared-copy"
                @click="copyShared"
              >
                <n-icon :component="CopyOutline" :size="14" />
                {{ t('shell.members.copy') }}
                <ShortcutKeys id="members.copyLink" soft joined />
              </button>
              <button
                type="button"
                class="kit-btn small"
                :disabled="sharedBusy"
                data-testid="members-shared-reset"
                @click="resetShared"
              >
                <n-icon :component="RefreshOutline" :size="14" />
                {{ t('shell.members.sharedResetBtn') }}
              </button>
            </template>
            <template v-else>
              <span class="mw-link-empty">{{ t('shell.members.sharedNone') }}</span>
              <span class="mw-grow" />
              <button
                type="button"
                class="kit-btn small"
                :disabled="sharedBusy"
                data-testid="members-shared-create"
                @click="createShared"
              >
                <n-icon :component="LinkOutline" :size="14" />
                {{ t('shell.members.sharedCreate') }}
              </button>
            </template>
          </div>
          <!-- The workspace's own address (#3077): whoever opens it without
               being a member can ask to be let in — the requests land below. -->
          <div class="mw-link" data-testid="members-space-link-row">
            <n-icon :component="HandRightOutline" :size="15" class="mw-link-ic" />
            <span class="mw-link-l">{{ t('shell.members.spaceLink') }}</span>
            <code class="mw-link-v" data-testid="members-space-link">{{ spaceLink }}</code>
            <span class="mw-grow" />
            <button
              type="button"
              class="kit-btn small"
              data-testid="members-copy-space-link"
              @click="copySpaceLink"
            >
              <n-icon :component="CopyOutline" :size="14" />
              {{ t('shell.members.copy') }}
            </button>
          </div>
        </section>

        <!-- Access requests (#3077): people who opened a link to this space and
             asked to be let in. «Впустить» makes them a member; the role can be
             changed afterwards like anyone's. -->
        <template v-if="canManage && accessReqs.length">
          <div class="mw-h">
            {{ t('shell.members.requests') }} <span class="mw-h-n">{{ accessReqs.length }}</span>
          </div>
          <div class="mw-list">
            <div
              v-for="r in accessReqs"
              :key="r.id"
              class="mw-row mw-req"
              data-testid="access-request-row"
            >
              <UserAvatar :user-id="r.user_id" :name="r.user_name" class="mw-av" />
              <div class="info">
                <span class="name">{{ r.user_name || r.user_email }}</span>
                <span class="mail">
                  {{ r.user_email }}
                  <template v-if="r.board_name">
                    · {{ t('shell.members.requestFrom', { board: r.board_name }) }}</template
                  >
                </span>
                <span v-if="r.message" class="mw-req-msg">{{
                  t('shell.members.requestMessage', { text: r.message })
                }}</span>
              </div>
              <span class="inv-until">{{ reqWhen(r) }}</span>
              <button
                type="button"
                class="kit-btn small primary"
                :disabled="reqBusy === r.id"
                data-testid="access-approve"
                @click="decide(r, true)"
              >
                {{ t('shell.members.requestApprove') }}
              </button>
              <button
                type="button"
                class="mw-act"
                :disabled="reqBusy === r.id"
                data-testid="access-decline"
                @click="decide(r, false)"
              >
                {{ t('shell.members.requestDecline') }}
              </button>
            </div>
          </div>
        </template>

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
              :class="{ 'is-expired': inv.dead }"
              data-testid="invite-row"
            >
              <span class="mw-tile inv-icon"><n-icon :component="MailOutline" :size="15" /></span>
              <span class="inv-mail">{{ inv.email }}</span>
              <span class="mw-role" :class="`r-${inv.role}`">{{ roleName(inv.role) }}</span>
              <span
                v-if="inv.declined"
                class="inv-expired mw-expired"
                data-testid="invite-declined"
              >
                {{ t('shell.members.inviteDeclined') }}
              </span>
              <span v-else-if="inv.expired" class="inv-expired mw-expired">
                {{ t('shell.members.inviteExpired') }}
              </span>
              <template v-else>
                <span v-if="inv.sent" class="inv-sent">{{
                  t('shell.members.sentAgo', { when: inv.sent })
                }}</span>
                <span v-if="inv.until" class="inv-until">
                  <n-icon :component="TimeOutline" :size="12" />
                  {{ t('shell.members.inviteUntil', { date: inv.until }) }}
                </span>
              </template>
              <span class="mw-grow" />
              <button
                type="button"
                class="mw-act"
                :disabled="resending === inv.id"
                data-testid="invite-resend"
                @click="resendInvite(inv)"
              >
                {{ inv.dead ? t('shell.members.extend') : t('shell.members.resend') }}
              </button>
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
        <p v-else-if="section === 'invites' && !accessReqs.length" class="mw-empty">
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
            <template v-for="m in shownMembers" :key="m.user_id">
              <div
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
                <span class="mw-groups" data-testid="member-groups-cell">{{
                  memberGroups(m)
                }}</span>
                <span class="mw-seen" :title="activity(m).title" data-testid="member-activity">
                  <span v-if="activity(m).online" class="mw-dot" />
                  {{ activity(m).text }}
                </span>
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
                  v-if="!rowLock(m) || isSelf(m)"
                  type="button"
                  class="mw-more"
                  :title="t('common.action.more')"
                  data-testid="member-menu"
                  @click="openMenu(isSelf(m) ? 'self' : m.user_id, $event.currentTarget)"
                >
                  <n-icon :component="EllipsisHorizontal" :size="16" />
                </button>
                <span v-else class="mw-more-gap" />
              </div>
              <TransferOwnerPop
                v-if="transferShow && transferFor?.user_id === m.user_id"
                v-model:show="transferShow"
                :ws-id="wsId"
                :ws-name="workspaceName(workspace)"
                :member="transferFor"
                @done="onTransferred"
              />
            </template>
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
/* The shared link's URL takes what the row can spare, not the leftovers. */
/* One line, as in the mockup: the URL gives way first (down to a stub), the
   label, the settings and the buttons keep their size. */
.mw-shared > :not(.mw-link-v):not(.mw-grow) {
  flex-shrink: 0;
}
.mw-shared .mw-link-v {
  flex: 0 1 220px;
  min-width: 56px;
}
.mw-link-hidden {
  font-family: var(--t-mono, ui-monospace, monospace);
  letter-spacing: 0.02em;
}
/* «Участник · 6 дней · 3 из 25» — the link's settings, its menu on a click. */
.mw-link-meta {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 26px;
  padding: 0 8px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
  white-space: nowrap;
}
.mw-link-meta:hover,
.mw-link-meta.ctx {
  background: var(--t-hover);
  color: var(--t-text1);
}
.mw-last {
  min-height: 32px;
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
/* Groups and activity (#3063) — the mockup's two quiet columns. */
.mw-groups {
  flex: 0 0 160px;
  min-width: 0;
  font-size: 12.5px;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mw-seen {
  flex: 0 0 110px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.mw-dot {
  flex: none;
  width: 6px;
  height: 6px;
  border-radius: 3px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #18a058 86%, #000),
    #18a058 50%,
    color-mix(in srgb, #18a058 86%, #fff)
  );
}
.mw-more-gap {
  flex: none;
  width: 28px;
}
.mw-req .info {
  gap: 1px;
}
.mw-req-msg {
  font-size: 12.5px;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.lw-mobile .mw-groups,
.lw-mobile .mw-seen {
  display: none;
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
.inv-sent {
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
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
.mw-act:disabled {
  opacity: 0.5;
  cursor: default;
}
.mw-act:not(.stub):not(:disabled):hover {
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
.lw-mobile .mw-link-empty:not(.mw-link-hidden),
.lw-mobile .inv-until,
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
