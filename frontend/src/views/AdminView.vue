<script setup>
// Administration (v2, #3106, mockup AdminPage). The settings layout — a sunken
// section column, the open section on the right, `?s=` in the address — over
// the same admin API as before: the instance's users (admin flag, sign-in
// allowed, password-reset link), the GitLab OAuth app, the background jobs
// window. What the mockup draws without an API behind it — invitations across
// workspaces, roles, a user's workspaces and last sign-in, 2FA, CSV export — is
// a visible «Скоро · #N» stub (utils/featureStubs.js).
//
// Users are a table without lines: each row is its own --t-col surface. The
// role is a control (ActionMenu), the rest of a row's actions sit in «⋯»;
// revoking admin rights and blocking sign-in are two-step items.
import { computed, nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NIcon, NInput, NSpin, NSwitch, NTooltip } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  BuildOutline,
  ChevronDownOutline,
  CloudDownloadOutline,
  EllipsisHorizontal,
  GitBranchOutline,
  HelpCircleOutline,
  KeyOutline,
  LockClosedOutline,
  LockOpenOutline,
  LogoGitlab,
  MailOutline,
  PeopleOutline,
  PersonAddOutline,
  PersonOutline,
  SearchOutline,
  ServerOutline,
  ShieldCheckmarkOutline,
  ShieldOutline,
  SyncOutline,
} from '@vicons/ionicons5'
import { admin } from '@/api'
import { toCSV } from '@/utils/csv'
import { handOffBlob } from '@/utils/blobHandoff'
import ArmButton from '@/components/kit/ArmButton.vue'
import { copyText } from '@/utils/clipboard'
import { stubTask } from '@/utils/featureStubs'
import { humanizeError } from '@/utils/errors'
import { useAuthStore } from '@/stores/auth'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { useVersionInfo } from '@/composables/useVersionInfo'
import { useFormat } from '@/composables/useFormat'
import { useResponsive } from '@/composables/useResponsive'
import { useShortcuts } from '@/composables/useShortcuts'
import UserAvatar from '@/components/UserAvatar.vue'
import SecretInput from '@/components/SecretInput.vue'
import BrandLogo from '@/components/BrandLogo.vue'
import Kbd from '@/components/Kbd.vue'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import AdminHelpFeedback from '@/components/admin/AdminHelpFeedback.vue'
import '@/components/settings/settings.css'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const dialogs = useWsDialogsStore()
const whatsNew = useWhatsNewStore()
const message = useMessage()
const { isMobile } = useResponsive()
const { web, api: apiVersion } = useVersionInfo()
const { formatDateTime, formatDate, formatRelative } = useFormat()

const host = location.host
const SECTIONS = ['users', 'invites', 'server', 'integrations', 'help']
const section = computed(() => {
  const s = route?.query?.s
  return typeof s === 'string' && SECTIONS.includes(s) ? s : 'users'
})
function pick(key) {
  if (key !== section.value) router.replace({ query: { ...route.query, s: key } })
}
const STUB = {
  roles: stubTask('admin.roles'),
  twoFactor: stubTask('settings.twoFactor'),
}

// ── users ──
const loading = ref(false)
const users = ref([])
const query = ref('')
const filter = ref('all')
const busy = ref('')
const meId = computed(() => auth.user?.id)

const counts = computed(() => ({
  all: users.value.length,
  active: users.value.filter((u) => u.active).length,
  admins: users.value.filter((u) => u.is_admin).length,
  blocked: users.value.filter((u) => !u.active).length,
}))
const FILTERS = ['all', 'active', 'admins', 'blocked']
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  return users.value.filter((u) => {
    if (filter.value === 'active' && !u.active) return false
    if (filter.value === 'admins' && !u.is_admin) return false
    if (filter.value === 'blocked' && u.active) return false
    if (!q) return true
    return u.name.toLowerCase().includes(q) || u.email.toLowerCase().includes(q)
  })
})

// «Пространства» (#3078): the first by name and «+N»; all of them in the tooltip.
function spacesLabel(u) {
  const ws = u.workspaces || []
  if (!ws.length) return ''
  return ws.length === 1 ? ws[0].name : `${ws[0].name} +${ws.length - 1}`
}
// «Вход» (#3078): when they were last in the app — relative while it is near.
function seenLabel(u) {
  if (!u.last_seen_at) return ''
  return (
    formatRelative(u.last_seen_at) || formatDate(u.last_seen_at, { day: 'numeric', month: 'short' })
  )
}

// «Экспорт CSV» (#3080): what the list shows now (filter and search applied).
function exportCSV() {
  const text = toCSV(filtered.value, [
    { label: t('settings.admin.csvCol.name'), value: (u) => u.name },
    { label: 'Email', value: (u) => u.email },
    {
      label: t('settings.admin.col.role'),
      value: (u) =>
        u.is_admin ? t('settings.admin.role.adminShort') : t('settings.admin.role.user'),
    },
    {
      label: t('settings.admin.col.status'),
      value: (u) =>
        u.active ? t('settings.admin.status.active') : t('settings.admin.status.blocked'),
    },
    { label: t('settings.admin.csvCol.verified'), value: (u) => (u.email_verified ? 1 : 0) },
    {
      label: t('settings.admin.col.spaces'),
      value: (u) => (u.workspaces || []).map((w) => w.name).join('; '),
    },
    { label: t('settings.admin.csvCol.lastSeen'), value: (u) => u.last_seen_at || '' },
    { label: t('settings.admin.csvCol.created'), value: (u) => u.created_at },
  ])
  const day = new Date().toISOString().slice(0, 10)
  handOffBlob(new Blob([text], { type: 'text/csv;charset=utf-8' }), `tessera-users-${day}.csv`)
}

// ── invitations of the whole instance (#3079) ──
const invites = ref([])
const invLoading = ref(false)
const invFilter = ref('pending')
const INV_FILTERS = ['pending', 'expired', 'declined', 'all']
function invState(inv) {
  if (inv.declined_at) return 'declined'
  return new Date(inv.expires_at).getTime() <= Date.now() ? 'expired' : 'pending'
}
const invCounts = computed(() => {
  const c = { pending: 0, expired: 0, declined: 0, all: invites.value.length }
  for (const inv of invites.value) c[invState(inv)]++
  return c
})
const invFiltered = computed(() => {
  const q = query.value.trim().toLowerCase()
  return invites.value.filter((inv) => {
    if (invFilter.value !== 'all' && invState(inv) !== invFilter.value) return false
    if (!q) return true
    return inv.email.toLowerCase().includes(q) || inv.workspace_name.toLowerCase().includes(q)
  })
})
async function loadInvites() {
  invLoading.value = true
  try {
    const res = await admin.listInvitations()
    invites.value = res.data || []
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    invLoading.value = false
  }
}
async function revokeInvite(inv) {
  try {
    await admin.deleteInvitation(inv.id)
    invites.value = invites.value.filter((x) => x.id !== inv.id)
  } catch (e) {
    message.error(humanizeError(e))
  }
}
function openInvited() {
  invFilter.value = 'pending'
  select('invites')
}

async function load() {
  loading.value = true
  try {
    const res = await admin.listUsers()
    users.value = res.data || []
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

// ── row menus: the role control and «⋯» ──
const menu = ref({ show: false, anchor: null, kind: '', user: null })
const menuBusy = ref(false)
const menuFailed = ref('')
function openMenu(kind, u, el) {
  menuFailed.value = ''
  menu.value = { show: true, anchor: el, kind, user: u }
}
const menuItems = computed(() => {
  const u = menu.value.user
  if (!u) return []
  const pending = (item) => {
    if (menuBusy.value) return { ...item, busy: true }
    if (menuFailed.value)
      return { ...item, failed: { title: t('common.menu.failed'), sub: menuFailed.value } }
    return item
  }
  if (menu.value.kind === 'role') {
    return [
      {
        key: 'admin',
        label: t('settings.admin.role.admin'),
        icon: ShieldCheckmarkOutline,
        active: u.is_admin,
        note: u.is_admin ? t('settings.admin.role.current') : '',
      },
      u.is_admin
        ? pending({
            key: 'user',
            label: t('settings.admin.role.user'),
            icon: PersonOutline,
            danger: true,
            confirm: {
              title: t('settings.admin.confirm.revokeAdmin', { name: u.name }),
              sub: t('settings.admin.confirm.revokeAdminSub'),
            },
          })
        : {
            key: 'user',
            label: t('settings.admin.role.user'),
            icon: PersonOutline,
            active: true,
            note: t('settings.admin.role.current'),
          },
      { gap: true },
      { key: 'roles', label: t('settings.admin.nav.roles'), icon: KeyOutline, stub: STUB.roles },
    ]
  }
  const items = [
    { key: 'reset', label: t('settings.admin.action.resetLink'), icon: KeyOutline },
    { gap: true },
  ]
  if (u.active) {
    items.push(
      pending({
        key: 'block',
        label: t('settings.admin.action.deactivate'),
        icon: LockClosedOutline,
        danger: true,
        confirm: {
          title: t('settings.admin.confirm.deactivateTitle', { name: u.name }),
          sub: t('settings.admin.confirm.deactivateSub'),
        },
      }),
    )
  } else {
    items.push({
      key: 'unblock',
      label: t('settings.admin.action.activate'),
      icon: LockOpenOutline,
    })
  }
  return items
})

async function onMenu(key) {
  const u = menu.value.user
  if (!u) return
  if (key === 'admin' && !u.is_admin) return setAdmin(u, true)
  if (key === 'user' && u.is_admin) return setAdmin(u, false)
  if (key === 'reset') return copyResetLink(u)
  if (key === 'block') return setActive(u, false)
  if (key === 'unblock') return setActive(u, true)
}

async function setAdmin(u, value) {
  menuBusy.value = true
  menuFailed.value = ''
  busy.value = u.id
  try {
    await admin.setAdmin(u.id, value)
    u.is_admin = value
    menu.value.show = false
    message.success(
      value ? t('settings.admin.msg.adminGranted') : t('settings.admin.msg.adminRevoked'),
    )
  } catch (e) {
    menuFailed.value = humanizeError(e)
  } finally {
    menuBusy.value = false
    busy.value = ''
  }
}

async function setActive(u, value) {
  menuBusy.value = true
  menuFailed.value = ''
  busy.value = u.id
  try {
    await admin.setActive(u.id, value)
    u.active = value
    menu.value.show = false
    message.success(value ? t('settings.admin.msg.activated') : t('settings.admin.msg.deactivated'))
  } catch (e) {
    menuFailed.value = humanizeError(e)
  } finally {
    menuBusy.value = false
    busy.value = ''
  }
}

async function copyResetLink(u) {
  busy.value = u.id
  try {
    const res = await admin.resetLink(u.id)
    const link = res.data?.link || ''
    // A path-only link when PUBLIC_URL is unset — qualify it against this origin.
    const full = link.startsWith('http') ? link : `${window.location.origin}${link}`
    if (await copyText(full)) message.success(t('settings.admin.msg.resetLinkCopied'))
    else message.info(full)
  } catch (e) {
    message.error(e.message)
  } finally {
    busy.value = ''
  }
}

// ── keys: «/» search, ↑↓ rows, ↵ the row's menu ──
const searchRef = ref(null)
const rowEls = {}
useShortcuts({
  'admin.search': () => {
    if (section.value !== 'users') return
    searchRef.value?.focus()
  },
})
function onRowKey(e, u, i) {
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    const next = filtered.value[i + (e.key === 'ArrowDown' ? 1 : -1)]
    if (next) rowEls[next.id]?.focus()
  } else if (e.key === 'Enter' && e.target === e.currentTarget) {
    e.preventDefault()
    openMenu('row', u, e.currentTarget.querySelector('[data-row-more]') || e.currentTarget)
  }
}
function onSearchKey(e) {
  if (e.key === 'ArrowDown' && filtered.value.length) {
    e.preventDefault()
    rowEls[filtered.value[0].id]?.focus()
  }
}

// ── GitLab OAuth app (Интеграции) ──
// Verbatim technical tokens: GitLab scope names and a JSON sample.
const SCOPE_READ_API = 'read_api'
const SCOPE_API = 'api'
const ORG_MAP_SAMPLE =
  '{ "group/path": { "workspace_id": "uuid", "admins": ["login"], "users": true } }'

const oauth = ref({
  gl_base_url: '',
  client_id: '',
  client_secret: '',
  service_token: '',
  enabled: false,
  org_map: '{}',
  has_secret: false,
  has_service_token: false,
  clear_secret: false,
  clear_service_token: false,
  sudo_writeback: false,
})
const oauthSaving = ref(false)
const callbackUrl = computed(() => `${window.location.origin}/api/auth/gitlab/callback`)

async function loadOAuth() {
  try {
    const { data } = await admin.getOAuth()
    oauth.value = {
      gl_base_url: data.gl_base_url || '',
      client_id: data.client_id || '',
      client_secret: '',
      service_token: '',
      enabled: data.enabled === true,
      org_map: JSON.stringify(data.org_map ?? {}, null, 2),
      has_secret: data.has_secret === true,
      has_service_token: data.has_service_token === true,
      clear_secret: false,
      clear_service_token: false,
      sudo_writeback: data.sudo_writeback === true,
    }
  } catch {
    /* first-time config: keep defaults */
  }
}

async function saveOAuth() {
  let orgMap
  try {
    orgMap = JSON.parse(oauth.value.org_map || '{}')
  } catch {
    message.error(t('settings.admin.msg.orgMapInvalid'))
    return
  }
  oauthSaving.value = true
  try {
    const { data } = await admin.setOAuth({
      gl_base_url: oauth.value.gl_base_url.trim(),
      client_id: oauth.value.client_id.trim(),
      client_secret: oauth.value.client_secret, // empty keeps the stored one
      service_token: oauth.value.service_token, // empty keeps the stored one
      clear_client_secret: oauth.value.clear_secret,
      clear_service_token: oauth.value.clear_service_token,
      enabled: oauth.value.enabled,
      org_map: orgMap,
      sudo_writeback: oauth.value.sudo_writeback,
    })
    oauth.value.client_secret = ''
    oauth.value.service_token = ''
    oauth.value.clear_secret = false
    oauth.value.clear_service_token = false
    oauth.value.has_secret = data.has_secret === true
    oauth.value.has_service_token = data.has_service_token === true
    oauth.value.sudo_writeback = data.sudo_writeback === true
    message.success(t('settings.admin.msg.oauthSaved'))
  } catch (e) {
    message.error(e.message)
  } finally {
    oauthSaving.value = false
  }
}

// ── server ──
function fmtBuilt(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : formatDateTime(d)
}

const paneEl = ref(null)
function select(key) {
  pick(key)
  nextTick(() => paneEl.value?.scrollTo?.({ top: 0 }))
}

onMounted(() => {
  load()
  loadInvites()
  loadOAuth()
})
</script>

<template>
  <div class="av" :class="{ mobile: isMobile }" data-testid="admin-view">
    <nav class="av-nav" :aria-label="t('settings.admin.title')">
      <div v-if="!isMobile" class="av-host">
        <BrandLogo :height="20" :wordmark="false" />
        <span class="av-host-who">
          <span class="av-host-name">{{ host }}</span>
          <span class="av-host-ver">{{
            t('settings.admin.hostVersion', { version: apiVersion?.version || web.version })
          }}</span>
        </span>
      </div>
      <button
        type="button"
        class="av-item"
        :class="{ active: section === 'users' }"
        data-testid="admin-nav-users"
        @click="select('users')"
      >
        <n-icon :component="PeopleOutline" :size="16" class="av-item-ic" />
        <span class="av-item-l">{{ t('settings.admin.nav.users') }}</span>
        <span class="av-item-n">{{ users.length || '' }}</span>
      </button>
      <button
        type="button"
        class="av-item"
        :class="{ active: section === 'invites' }"
        data-testid="admin-nav-invites"
        @click="select('invites')"
      >
        <n-icon :component="MailOutline" :size="16" class="av-item-ic" />
        <span class="av-item-l">{{ t('settings.admin.nav.invites') }}</span>
        <span class="av-item-n">{{ invCounts.pending || '' }}</span>
      </button>
      <n-tooltip :delay="300" placement="right">
        <template #trigger>
          <span class="av-item stub" aria-disabled="true" data-testid="admin-nav-roles">
            <n-icon :component="KeyOutline" :size="16" class="av-item-ic" />
            <span class="av-item-l">{{ t('settings.admin.nav.roles') }}</span>
            <span class="av-pill">{{ t('common.stub.badge') }}</span>
          </span>
        </template>
        {{ t('common.stub.soon', { n: STUB.roles }) }}
      </n-tooltip>
      <span class="av-gap" />
      <button
        type="button"
        class="av-item"
        :class="{ active: section === 'server' }"
        data-testid="admin-nav-server"
        @click="select('server')"
      >
        <n-icon :component="ServerOutline" :size="16" class="av-item-ic" />
        <span class="av-item-l">{{ t('settings.admin.nav.server') }}</span>
      </button>
      <button
        type="button"
        class="av-item"
        :class="{ active: section === 'integrations' }"
        data-testid="admin-nav-integrations"
        @click="select('integrations')"
      >
        <n-icon :component="GitBranchOutline" :size="16" class="av-item-ic" />
        <span class="av-item-l">{{ t('settings.admin.nav.integrations') }}</span>
      </button>
      <button
        type="button"
        class="av-item"
        :class="{ active: section === 'help' }"
        data-testid="admin-nav-help"
        @click="select('help')"
      >
        <n-icon :component="HelpCircleOutline" :size="16" class="av-item-ic" />
        <span class="av-item-l">{{ t('settings.admin.nav.help') }}</span>
      </button>
      <button
        type="button"
        class="av-item"
        data-testid="admin-nav-jobs"
        @click="dialogs.open('jobs')"
      >
        <n-icon :component="SyncOutline" :size="16" class="av-item-ic" />
        <span class="av-item-l">{{ t('settings.admin.nav.jobs') }}</span>
      </button>
    </nav>

    <section ref="paneEl" class="av-pane" :data-section="section">
      <!-- Users -->
      <template v-if="section === 'users'">
        <header class="av-head">
          <h1 class="av-title">{{ t('settings.admin.nav.users') }}</h1>
          <span class="av-sub">{{ users.length }}</span>
          <span class="av-grow" />
          <n-button
            secondary
            :disabled="!filtered.length"
            data-testid="admin-csv"
            @click="exportCSV"
          >
            <template #icon><n-icon :component="CloudDownloadOutline" /></template>
            {{ t('settings.admin.csv') }}
          </n-button>
          <!-- An invitation leads into a workspace: it is sent from that
               workspace's «Участники» (#3079). -->
          <n-button type="primary" data-testid="admin-invite" @click="dialogs.open('members')">
            <template #icon><n-icon :component="PersonAddOutline" /></template>
            {{ t('settings.admin.invite') }}
          </n-button>
        </header>

        <div class="av-tools">
          <label class="av-search">
            <n-icon :component="SearchOutline" :size="15" class="av-search-ic" />
            <input
              ref="searchRef"
              v-model="query"
              class="av-search-in"
              :placeholder="t('settings.admin.search')"
              data-testid="admin-search"
              @keydown="onSearchKey"
              @keydown.esc="query = ''"
            />
            <Kbd v-if="!query" class="av-kbd">/</Kbd>
          </label>
          <div class="av-seg" role="tablist">
            <button
              v-for="f in FILTERS"
              :key="f"
              type="button"
              role="tab"
              class="av-seg-b"
              :class="{ on: filter === f }"
              :aria-selected="filter === f"
              :data-testid="`admin-filter-${f}`"
              @click="filter = f"
            >
              {{ t(`settings.admin.filter.${f}`) }}
              <span class="av-seg-n">{{ counts[f] }}</span>
            </button>
            <button
              type="button"
              class="av-seg-b"
              data-testid="admin-filter-invited"
              @click="openInvited"
            >
              {{ t('settings.admin.filter.invited') }}
              <span class="av-seg-n">{{ invCounts.pending }}</span>
            </button>
          </div>
        </div>

        <n-spin :show="loading && !users.length" size="small">
          <div class="av-table" role="table" data-testid="admin-users">
            <div v-if="!isMobile" class="av-thead" role="row">
              <span class="c-user">{{ t('settings.admin.col.user') }}</span>
              <span class="c-role">{{ t('settings.admin.col.role') }}</span>
              <span class="c-spaces">{{ t('settings.admin.col.spaces') }}</span>
              <span class="c-status">{{ t('settings.admin.col.status') }}</span>
              <span class="c-login">{{ t('settings.admin.col.login') }}</span>
              <n-tooltip :delay="300">
                <template #trigger>
                  <span class="c-2fa stub">{{ t('settings.admin.col.twoFactor') }}</span>
                </template>
                {{ t('common.stub.soon', { n: STUB.twoFactor }) }}
              </n-tooltip>
              <span class="c-more" />
            </div>
            <div
              v-for="(u, i) in filtered"
              :key="u.id"
              :ref="(el) => (rowEls[u.id] = el)"
              class="av-row"
              :class="{ off: !u.active }"
              role="row"
              tabindex="0"
              data-testid="admin-user"
              :data-user="u.email"
              @keydown="onRowKey($event, u, i)"
            >
              <span class="c-user">
                <UserAvatar class="av-ava" :user-id="u.id" :name="u.name" :src="u.avatar_url" />
                <span class="av-who">
                  <span class="av-name">
                    <span class="av-name-t">{{ u.name }}</span>
                    <span v-if="u.id === meId" class="av-you">{{
                      t('settings.admin.badge.you')
                    }}</span>
                  </span>
                  <span class="av-mail">
                    {{ u.email }}
                    <span v-if="!u.email_verified" class="av-unv">
                      · {{ t('settings.admin.badge.unverified') }}</span
                    >
                  </span>
                </span>
              </span>
              <span class="c-role">
                <button
                  v-if="u.id !== meId"
                  type="button"
                  class="av-role"
                  :class="{ admin: u.is_admin }"
                  :disabled="busy === u.id"
                  data-testid="admin-role"
                  @click="openMenu('role', u, $event.currentTarget)"
                >
                  {{
                    u.is_admin ? t('settings.admin.role.adminShort') : t('settings.admin.role.user')
                  }}
                  <n-icon :component="ChevronDownOutline" :size="12" />
                </button>
                <n-tooltip v-else :delay="300">
                  <template #trigger>
                    <span class="av-role static" :class="{ admin: u.is_admin }">
                      {{
                        u.is_admin
                          ? t('settings.admin.role.adminShort')
                          : t('settings.admin.role.user')
                      }}
                      <n-icon :component="LockClosedOutline" :size="11" />
                    </span>
                  </template>
                  {{ t('settings.admin.role.selfLocked') }}
                </n-tooltip>
              </span>
              <n-tooltip v-if="!isMobile && u.workspaces?.length > 1" :delay="300">
                <template #trigger>
                  <span class="c-spaces av-ellip" data-testid="admin-user-spaces">{{
                    spacesLabel(u)
                  }}</span>
                </template>
                <div v-for="w in u.workspaces" :key="w.id">{{ w.name }}</div>
              </n-tooltip>
              <span
                v-else-if="!isMobile"
                class="c-spaces av-ellip"
                :class="{ 'av-dim': !u.workspaces?.length }"
                data-testid="admin-user-spaces"
                >{{ spacesLabel(u) || '—' }}</span
              >
              <span class="c-status">
                <span class="av-dot" :class="u.active ? 'on' : 'off'" />
                {{
                  u.active ? t('settings.admin.status.active') : t('settings.admin.status.blocked')
                }}
              </span>
              <span
                v-if="!isMobile"
                class="c-login av-dim"
                :title="u.last_seen_at ? formatDateTime(u.last_seen_at) : ''"
                data-testid="admin-user-seen"
                >{{ seenLabel(u) || '—' }}</span
              >
              <span v-if="!isMobile" class="c-2fa av-dim">
                <n-icon :component="ShieldOutline" :size="15" />
              </span>
              <span class="c-more">
                <button
                  v-if="u.id !== meId"
                  type="button"
                  class="av-more"
                  data-row-more
                  :disabled="busy === u.id"
                  :aria-label="t('common.action.more')"
                  data-testid="admin-user-menu"
                  @click="openMenu('row', u, $event.currentTarget)"
                >
                  <n-icon :component="EllipsisHorizontal" :size="16" />
                </button>
                <button
                  v-else
                  type="button"
                  class="av-more"
                  :disabled="busy === u.id"
                  :title="t('settings.admin.action.resetLink')"
                  :aria-label="t('settings.admin.action.resetLink')"
                  @click="copyResetLink(u)"
                >
                  <n-icon :component="KeyOutline" :size="15" />
                </button>
              </span>
            </div>
            <p v-if="!filtered.length && !loading" class="av-empty">
              {{ t('settings.admin.empty') }}
            </p>
          </div>
        </n-spin>
      </template>

      <!-- Invitations of the whole instance (#3079) -->
      <template v-else-if="section === 'invites'">
        <header class="av-head">
          <h1 class="av-title">{{ t('settings.admin.nav.invites') }}</h1>
          <span class="av-sub">{{ invCounts.pending }}</span>
          <span class="av-grow" />
          <n-button type="primary" data-testid="admin-invite" @click="dialogs.open('members')">
            <template #icon><n-icon :component="PersonAddOutline" /></template>
            {{ t('settings.admin.invite') }}
          </n-button>
        </header>
        <div class="av-tools">
          <label class="av-search">
            <n-icon :component="SearchOutline" :size="15" class="av-search-ic" />
            <input
              v-model="query"
              class="av-search-in"
              :placeholder="t('settings.admin.invites.search')"
              data-testid="admin-inv-search"
              @keydown.esc="query = ''"
            />
          </label>
          <div class="av-seg" role="tablist">
            <button
              v-for="f in INV_FILTERS"
              :key="f"
              type="button"
              role="tab"
              class="av-seg-b"
              :class="{ on: invFilter === f }"
              :aria-selected="invFilter === f"
              :data-testid="`admin-inv-filter-${f}`"
              @click="invFilter = f"
            >
              {{ t(`settings.admin.invites.filter.${f}`) }}
              <span class="av-seg-n">{{ invCounts[f] }}</span>
            </button>
          </div>
        </div>
        <n-spin :show="invLoading && !invites.length" size="small">
          <div class="av-table" role="table" data-testid="admin-invites">
            <div v-if="!isMobile" class="av-thead av-ithead" role="row">
              <span>{{ t('settings.admin.invites.col.email') }}</span>
              <span>{{ t('settings.admin.invites.col.space') }}</span>
              <span>{{ t('settings.admin.col.role') }}</span>
              <span>{{ t('settings.admin.col.status') }}</span>
              <span>{{ t('settings.admin.invites.col.sent') }}</span>
              <span />
            </div>
            <div
              v-for="inv in invFiltered"
              :key="inv.id"
              class="av-row av-irow"
              :class="{ off: invState(inv) !== 'pending' }"
              role="row"
              data-testid="admin-invite-row"
              :data-email="inv.email"
            >
              <span class="av-ellip av-imail i-mail">
                <n-icon :component="MailOutline" :size="15" class="av-dim" />
                {{ inv.email }}
              </span>
              <span class="av-ellip i-ws">{{ inv.workspace_name }}</span>
              <span class="av-dim i-role">{{ t(`settings.admin.invites.role.${inv.role}`) }}</span>
              <span class="i-state">
                <span class="av-dot" :class="invState(inv) === 'pending' ? 'on' : 'off'" />
                {{
                  invState(inv) === 'pending'
                    ? t('settings.admin.invites.until', {
                        date: formatDate(inv.expires_at, { day: 'numeric', month: 'short' }),
                      })
                    : t(`settings.admin.invites.state.${invState(inv)}`)
                }}
              </span>
              <span class="av-dim av-ellip i-sent" :title="formatDateTime(inv.sent_at)">
                {{
                  formatRelative(inv.sent_at) ||
                  formatDate(inv.sent_at, { day: 'numeric', month: 'short' })
                }}
                <template v-if="inv.invited_by_name"> · {{ inv.invited_by_name }}</template>
              </span>
              <span class="c-more">
                <ArmButton
                  :title="t('settings.admin.invites.revoke')"
                  :confirm="t('settings.admin.invites.revokeConfirm')"
                  testid="admin-invite-revoke"
                  @confirm="revokeInvite(inv)"
                />
              </span>
            </div>
            <p v-if="!invFiltered.length && !invLoading" class="av-empty">
              {{ t('settings.admin.invites.empty') }}
            </p>
          </div>
        </n-spin>
      </template>

      <!-- Server -->
      <template v-else-if="section === 'server'">
        <header class="av-head">
          <h1 class="av-title">{{ t('settings.admin.nav.server') }}</h1>
          <span class="av-sub">{{ host }}</span>
        </header>
        <section class="st-card av-server" data-testid="admin-server">
          <div class="av-kv">
            <span class="av-k">{{ t('settings.admin.server.address') }}</span>
            <span class="av-v">{{ host }}</span>
            <span class="av-k">{{ t('settings.admin.server.api') }}</span>
            <span class="av-v">
              {{ apiVersion?.version || '—' }}
              <span v-if="apiVersion?.commit" class="av-dim">· {{ apiVersion.commit }}</span>
            </span>
            <span v-if="apiVersion?.builtAt" class="av-k">{{
              t('settings.admin.server.apiBuilt')
            }}</span>
            <span v-if="apiVersion?.builtAt" class="av-v">{{ fmtBuilt(apiVersion.builtAt) }}</span>
            <span class="av-k">{{ t('settings.admin.server.web') }}</span>
            <span class="av-v">
              {{ web.version || '—' }}
              <span v-if="web.commit" class="av-dim">· {{ web.commit }}</span>
            </span>
            <span v-if="web.builtAt" class="av-k">{{ t('settings.admin.server.webBuilt') }}</span>
            <span v-if="web.builtAt" class="av-v">{{ fmtBuilt(web.builtAt) }}</span>
          </div>
          <div class="st-actions av-left">
            <n-button secondary @click="whatsNew.openHistory()">
              {{ t('settings.admin.server.whatsNew') }}
            </n-button>
            <n-button secondary @click="dialogs.open('jobs')">
              <template #icon><n-icon :component="BuildOutline" /></template>
              {{ t('settings.admin.nav.jobs') }}
            </n-button>
          </div>
        </section>
      </template>

      <!-- Help: «Статья помогла?» totals (#3071) -->
      <template v-else-if="section === 'help'">
        <header class="av-head">
          <h1 class="av-title">{{ t('settings.admin.nav.help') }}</h1>
          <span class="av-sub">{{ t('settings.admin.help.sub') }}</span>
        </header>
        <AdminHelpFeedback />
      </template>

      <!-- Integrations: the GitLab OAuth app -->
      <template v-else>
        <header class="av-head">
          <h1 class="av-title">{{ t('settings.admin.nav.integrations') }}</h1>
          <span class="av-sub">{{ t('settings.admin.integrationsSub') }}</span>
        </header>
        <section class="st-card av-oauth" data-testid="admin-oauth">
          <div class="st-card-head">
            <span class="av-gl"><n-icon :component="LogoGitlab" :size="18" /></span>
            <span class="av-oauth-title">{{ t('settings.admin.oauth.title') }}</span>
            <span class="st-card-aside">
              <span>{{ t('settings.admin.oauth.field.enabled') }}</span>
              <n-switch v-model:value="oauth.enabled" />
            </span>
          </div>
          <p class="st-hint">
            <b>{{ t('settings.admin.oauth.hint.loginLabel') }}</b>
            <i18n-t keypath="settings.admin.oauth.hint.login" tag="span" scope="global">
              <template #scope
                ><code>{{ SCOPE_READ_API }}</code></template
              >
              <template #redirect
                ><code>{{ callbackUrl }}</code></template
              >
              <template #toggle
                ><b>{{ t('settings.admin.oauth.hint.toggle') }}</b></template
              >
            </i18n-t>
            <br />
            <b>{{ t('settings.admin.oauth.hint.syncLabel') }}</b>
            <i18n-t keypath="settings.admin.oauth.hint.sync" tag="span" scope="global">
              <template #token
                ><b>{{ t('settings.admin.oauth.hint.serviceToken') }}</b></template
              >
              <template #api
                ><code>{{ SCOPE_API }}</code></template
              >
            </i18n-t>
          </p>
          <div class="st-grid2">
            <label class="st-field">
              <span class="st-label">{{ t('settings.admin.oauth.field.baseUrl') }}</span>
              <n-input
                v-model:value="oauth.gl_base_url"
                :placeholder="t('settings.admin.oauth.placeholder.baseUrl')"
                :input-props="{ autocomplete: 'off', name: 'oauth-base-url' }"
              />
            </label>
            <label class="st-field">
              <span class="st-label">{{ t('settings.admin.oauth.field.appId') }}</span>
              <n-input
                v-model:value="oauth.client_id"
                :placeholder="t('settings.admin.oauth.placeholder.clientId')"
                :input-props="{ autocomplete: 'off', name: 'oauth-client-id' }"
              />
            </label>
          </div>
          <label class="st-field">
            <span class="st-label">{{ t('settings.admin.oauth.field.secret') }}</span>
            <SecretInput
              v-model:value="oauth.client_secret"
              v-model:cleared="oauth.clear_secret"
              :stored="oauth.has_secret"
              :placeholder="t('settings.admin.oauth.placeholder.clientSecret')"
              :stored-placeholder="t('settings.admin.oauth.storedPlaceholder')"
              :input-props="{ autocomplete: 'new-password', name: 'oauth-secret' }"
            />
          </label>
          <label class="st-field">
            <span class="st-label">
              {{ t('settings.admin.oauth.field.serviceToken') }}
              <span class="av-label-sub">{{
                t('settings.admin.oauth.field.serviceTokenSub')
              }}</span>
            </span>
            <SecretInput
              v-model:value="oauth.service_token"
              v-model:cleared="oauth.clear_service_token"
              :stored="oauth.has_service_token"
              :placeholder="t('settings.admin.oauth.placeholder.serviceToken')"
              :stored-placeholder="t('settings.admin.oauth.storedPlaceholder')"
              :input-props="{ autocomplete: 'new-password', name: 'oauth-service-token' }"
            />
            <span v-if="oauth.clear_service_token" class="st-err">
              {{ t('settings.admin.oauth.clearWarn') }}
            </span>
          </label>
          <label class="st-toggle">
            <span class="st-toggle-l">
              {{ t('settings.admin.oauth.field.sudo') }}
              <span class="st-toggle-sub">{{ t('settings.admin.oauth.field.sudoSub') }}</span>
            </span>
            <n-switch v-model:value="oauth.sudo_writeback" />
          </label>
          <label class="st-field">
            <span class="st-label">
              {{ t('settings.admin.oauth.field.orgMap') }}
              <span class="av-label-sub">{{ t('settings.admin.oauth.field.orgMapSub') }}</span>
            </span>
            <n-input
              v-model:value="oauth.org_map"
              type="textarea"
              :autosize="{ minRows: 4, maxRows: 14 }"
              :placeholder="ORG_MAP_SAMPLE"
              class="av-mono"
            />
          </label>
          <div class="st-actions">
            <n-button type="primary" :loading="oauthSaving" @click="saveOAuth">
              {{ t('common.action.save') }}
            </n-button>
          </div>
        </section>
      </template>
    </section>

    <ActionMenu
      v-model:show="menu.show"
      :anchor="() => menu.anchor"
      placement="bottom-end"
      :width="300"
      :header="menu.user?.name || ''"
      :items="menuItems"
      testid="admin-menu"
      @select="onMenu"
    />
  </div>
</template>

<style scoped>
.av {
  display: grid;
  grid-template-columns: 232px minmax(0, 1fr);
  gap: 28px;
  height: 100%;
  min-height: 0;
}
.av-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px;
  border-radius: var(--t-r-col);
  background: var(--t-sub);
  min-height: 0;
  overflow-y: auto;
}
.av-host {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 8px 14px;
  min-width: 0;
}
.av-host-who {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.av-host-name {
  font-size: 13.5px;
  font-weight: 650;
  color: var(--t-text1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.av-host-ver {
  font-size: 12px;
  color: var(--t-text3);
}
.av-item {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: none;
  height: 34px;
  padding: 0 10px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
}
.av-item:hover {
  background: var(--t-hover);
}
.av-item:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.av-item.active {
  background: var(--t-active);
  font-weight: 650;
}
.av-item.active .av-item-ic {
  color: var(--t-primary);
}
.av-item.active .av-item-ic :deep([stroke='currentColor']) {
  stroke: url(#t-accent-grad-svg);
}
.av-item.stub {
  opacity: 0.55;
  cursor: default;
}
.av-item.stub:hover {
  background: transparent;
}
.av-item-ic {
  flex: none;
  color: var(--t-text2);
}
.av-item-l {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.av-item-n {
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.av-pill {
  flex: none;
  padding: 2px 7px;
  border-radius: var(--t-r-pill);
  background: var(--t-pill);
  color: var(--t-text3);
  font-size: 10.5px;
  font-weight: 650;
}
.av-gap {
  flex: none;
  height: 14px;
}
.av-pane {
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  padding: 8px 4px 40px 0;
  container-type: inline-size;
}
.av-head {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin: 10px 0 18px;
}
.av-title {
  margin: 0;
  font-size: 26px;
  font-weight: 700;
  color: var(--t-text1);
}
.av-sub {
  font-size: 13px;
  color: var(--t-text3);
}
.av-grow {
  flex: 1;
}
.av-head .av-stub-btn {
  align-self: center;
}
.av-head .av-stub-btn + .av-stub-btn {
  margin-left: 8px;
}
.av-stub-btn {
  display: inline-flex;
}
.av-tools {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 14px;
}
.av-search {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 260px;
  height: 36px;
  padding: 0 8px 0 12px;
  box-sizing: border-box;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
}
.av-search:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.av-search-ic {
  color: var(--t-text3);
}
.av-search-in {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
}
.av-search-in::placeholder {
  color: var(--t-placeholder);
}
.av-seg {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 3px;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
}
.av-seg-b {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.av-seg-b:hover {
  color: var(--t-text1);
}
.av-seg-b.on {
  background: var(--t-card);
  color: var(--t-text1);
  font-weight: 650;
  box-shadow: var(--t-sh-card);
}
.av-seg-b:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: -2px;
}
.av-seg-b.stub {
  opacity: 0.5;
  cursor: default;
}
.av-seg-n {
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.av-table {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.av-thead,
.av-row {
  display: grid;
  grid-template-columns:
    minmax(200px, 2.2fr) 150px minmax(100px, 1.1fr) minmax(120px, 1fr)
    minmax(70px, 0.7fr) 44px 32px;
  align-items: center;
  gap: 12px;
  padding: 0 12px;
}
.av-thead {
  height: 32px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.av-ithead,
.av-irow {
  grid-template-columns:
    minmax(180px, 1.6fr) minmax(120px, 1fr) 90px minmax(120px, 1fr)
    minmax(120px, 1fr) 32px;
}
.av-ellip {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.av-imail {
  display: flex;
  align-items: center;
  gap: 8px;
}
.av-thead .stub {
  opacity: 0.6;
  cursor: default;
}
.av-row {
  min-height: 52px;
  border-radius: 12px;
  background: var(--t-col);
  color: var(--t-text1);
}
.av-row:hover {
  background: color-mix(in srgb, var(--t-col) 70%, var(--t-card));
}
.av-row:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.av-row.off {
  opacity: 0.6;
}
.c-user {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.av-ava {
  flex: none;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 12px;
  font-weight: 650;
}
.av-who {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.av-name {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.av-name-t {
  font-size: 13.5px;
  font-weight: 650;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.av-you {
  flex: none;
  padding: 1px 6px;
  border-radius: var(--t-r-pill);
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 10.5px;
  font-weight: 650;
}
.av-mail {
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.av-unv {
  color: #c98612;
}
/* Role: accent for an admin (the instance's owner-level right), neutral for
   everyone else — the mockup's «владелец» accent / «участник» --v-ctl. */
.av-role {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
.av-role.admin {
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
}
.av-role:hover:not(.static) {
  filter: brightness(1.08);
}
.av-role:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.av-role.static {
  cursor: default;
}
.c-status {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 13px;
}
.av-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.av-dot.on {
  background: #3aa675;
}
.av-dot.off {
  background: #e0533d;
}
.av-dim {
  color: var(--t-text3);
  opacity: 0.6;
}
.av-more {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.av-more:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.av-more:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.av-empty {
  margin: 30px 0;
  text-align: center;
  font-size: 13px;
  color: var(--t-text3);
}
.av-kbd {
  flex: none;
}
@media (hover: none) {
  .av-kbd {
    display: none;
  }
}
.av-server {
  max-width: 640px;
}
.av-kv {
  display: grid;
  grid-template-columns: 160px minmax(0, 1fr);
  gap: 10px 16px;
  font-size: 13.5px;
}
.av-k {
  color: var(--t-text3);
}
.av-v {
  color: var(--t-text1);
  font-variant-numeric: tabular-nums;
  overflow-wrap: anywhere;
}
.av-left {
  justify-content: flex-start;
}
.av-oauth {
  max-width: 880px;
}
.av-gl {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 9px;
  background: color-mix(in srgb, #e24329 16%, var(--t-col));
  color: #e24329;
}
.av-oauth-title {
  font-size: 14.5px;
  font-weight: 650;
}
.av-oauth .st-hint code {
  padding: 1px 5px;
  border-radius: 5px;
  background: var(--t-pill);
}
.av-label-sub {
  display: block;
  margin-top: 2px;
  font-weight: 500;
  color: var(--t-text3);
  opacity: 0.85;
}
.av-mono :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}

/* A narrow pane (a laptop with the sidebar open) drops the secondary columns
   first: the user, role and status are what a row is about. */
@container (max-width: 800px) {
  .av-thead .c-spaces,
  .av-thead .c-login,
  .av-thead .c-2fa,
  .av-row .c-spaces,
  .av-row .c-login,
  .av-row .c-2fa {
    display: none;
  }
  .av-thead,
  .av-row {
    grid-template-columns: minmax(180px, 1fr) 150px minmax(120px, 0.6fr) 32px;
  }
  .av-irow .i-role,
  .av-irow .i-sent,
  .av-ithead > :nth-child(3),
  .av-ithead > :nth-child(5) {
    display: none;
  }
  .av-ithead,
  .av-irow {
    grid-template-columns: minmax(160px, 1.4fr) minmax(100px, 1fr) minmax(110px, 1fr) 32px;
  }
}
/* Phone: chips over the section, rows stack name / role · status. */
.av.mobile {
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: auto;
}
.av.mobile .av-nav {
  flex-direction: row;
  overflow-x: auto;
  padding: 6px;
  border-radius: var(--t-r-card);
  scrollbar-width: none;
}
.av.mobile .av-gap {
  width: 6px;
  height: auto;
}
.av.mobile .av-item-l {
  overflow: visible;
}
.av.mobile .av-pane {
  overflow: visible;
  padding: 0;
}
.av.mobile .av-title {
  font-size: 22px;
}
.av.mobile .av-search {
  width: 100%;
}
.av.mobile .av-seg {
  overflow-x: auto;
  max-width: 100%;
  scrollbar-width: none;
}
.av.mobile .av-row {
  grid-template-columns: minmax(0, 1fr) auto 32px;
  grid-template-areas:
    'user user more'
    'role status more';
  gap: 6px 10px;
  padding: 10px 12px;
}
.av.mobile .c-user {
  grid-area: user;
}
.av.mobile .av-irow {
  grid-template-columns: minmax(0, 1fr) auto 32px;
  grid-template-areas:
    'mail mail more'
    'ws state more';
}
.av.mobile .i-mail {
  grid-area: mail;
}
.av.mobile .i-ws {
  grid-area: ws;
}
.av.mobile .i-state {
  grid-area: state;
  justify-self: end;
}
.av.mobile .i-role,
.av.mobile .i-sent {
  display: none;
}
.av.mobile .c-role {
  grid-area: role;
}
.av.mobile .c-status {
  grid-area: status;
  justify-self: end;
}
.av.mobile .c-more {
  grid-area: more;
}
.av.mobile .av-kv {
  grid-template-columns: minmax(0, 1fr);
  gap: 2px;
}
.av.mobile .av-v {
  margin-bottom: 8px;
}
</style>
