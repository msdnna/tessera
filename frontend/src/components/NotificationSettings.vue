<script setup>
import { ref, reactive, computed, onMounted, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NInput,
  NButton,
  NSelect,
  NSwitch,
  NIcon,
  NModal,
  NCard,
  NTag,
  NSpin,
  NPopconfirm,
} from 'naive-ui'
import {
  MailOutline,
  PaperPlaneOutline,
  GlobeOutline,
  AddOutline,
  TrashOutline,
  CreateOutline,
  FlashOutline,
  ShareSocialOutline,
  PhonePortraitOutline,
  NotificationsOutline,
} from '@vicons/ionicons5'
import {
  notificationChannels as chApi,
  notificationRoutes as rtApi,
  notificationPrefs as prefsApi,
} from '@/api'
import { getDeviceId, notificationsSupported } from '@/utils/device'
import { workspaceName } from '@/utils/defaultNames'
import { useAuthStore } from '@/stores/auth'
import EmptyState from '@/components/EmptyState.vue'
import SecretInput from '@/components/SecretInput.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useThemeStore } from '@/stores/theme'

const { t } = useI18n()
const auth = useAuthStore()
const wsStore = useWorkspacesStore()
const theme = useThemeStore()

// Half-hour time-of-day options (minutes since midnight) for the quiet window.
const TIME_OPTIONS = Array.from({ length: 48 }, (_, i) => {
  const m = i * 30
  const hh = String(Math.floor(m / 60)).padStart(2, '0')
  const mm = String(m % 60).padStart(2, '0')
  return { label: `${hh}:${mm}`, value: m }
})

const channels = ref([])
const routes = ref([])
const loading = ref(true)
const loadError = ref('')

// Channel type registry — extend here as the backend gains senders. Only the
// icon lives here; the label is looked up per render, so switching the UI
// language relabels the selects instead of leaving them on the first language
// the component saw (#2799).
const TYPE_ICONS = {
  email: MailOutline,
  telegram: PaperPlaneOutline,
  webhook: GlobeOutline,
  shoutrrr: ShareSocialOutline,
  device: PhonePortraitOutline,
}
const CHANNEL_TYPES = Object.keys(TYPE_ICONS)
function typeLabel(type) {
  return TYPE_ICONS[type] ? t(`settings.notifications.channelType.${type}`) : type
}
// `device` channels auto-register per client and aren't manually addable.
const typeOptions = computed(() =>
  CHANNEL_TYPES.filter((v) => v !== 'device').map((value) => ({ value, label: typeLabel(value) })),
)
const allTypeOptions = computed(() =>
  CHANNEL_TYPES.map((value) => ({ value, label: typeLabel(value) })),
)

const myDeviceId = getDeviceId()
const notifPermission = ref(notificationsSupported() ? Notification.permission : 'unsupported')
async function requestNotifPermission() {
  if (!notificationsSupported()) return
  notifPermission.value = await Notification.requestPermission()
}

const EVENT_KINDS = [
  'assigned',
  'comment',
  'mention',
  'updated',
  'moved',
  'archived',
  'due_soon',
  'reminder',
  'integration_sync',
]
function kindLabel(kind) {
  return EVENT_KINDS.includes(kind) ? t(`settings.notifications.kind.${kind}`) : kind
}

// Minute presets for the schedule selects. The values are the contract with the
// backend; only the labels are translated, and — like every option table here —
// they are rebuilt per render rather than frozen at import.
const LEAD_VALUES = [0, 15, 30, 60, 180, 1440, 2880]
const REPEAT_VALUES = [0, 60, 180, 360, 1440]
const DIGEST_VALUES = [0, 5, 15, 30, 60]
const LEAD_OPTIONS = computed(() =>
  LEAD_VALUES.map((value) => ({ value, label: t(`settings.notifications.lead.${value}`) })),
)
const REPEAT_OPTIONS = computed(() =>
  REPEAT_VALUES.map((value) => ({ value, label: t(`settings.notifications.repeat.${value}`) })),
)
const DIGEST_OPTIONS = computed(() =>
  DIGEST_VALUES.map((value) => ({ value, label: t(`settings.notifications.digest.${value}`) })),
)
const kindOptions = computed(() => EVENT_KINDS.map((value) => ({ value, label: kindLabel(value) })))

const wsOptions = computed(() =>
  (wsStore.list || []).map((w) => ({ value: w.id, label: workspaceName(w) })),
)
const channelOptions = computed(() =>
  channels.value.map((c) => ({ value: c.id, label: channelTitle(c) })),
)

// per-user scheduling defaults (due dates + reminders)
const prefs = reactive({
  due_enabled: true,
  due_lead_minutes: 60,
  due_repeat_minutes: 0,
  reminder_enabled: true,
  quiet_enabled: false,
  quiet_start_minutes: 1320,
  quiet_end_minutes: 480,
  quiet_tz: '',
  digest_minutes: 0,
})
const prefsSaving = ref(false)
const prefsSaved = ref(false)

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [c, r, p] = await Promise.all([chApi.list(), rtApi.list(), prefsApi.get()])
    channels.value = c.data || []
    routes.value = r.data || []
    Object.assign(prefs, p.data || {})
  } catch (e) {
    loadError.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}
async function savePrefs() {
  prefsSaving.value = true
  try {
    // Track the user's timezone so the quiet window is evaluated in their local time.
    prefs.quiet_tz = theme.timezone || prefs.quiet_tz
    const res = await prefsApi.update({ ...prefs })
    Object.assign(prefs, res.data || {})
    prefsSaved.value = true
    setTimeout(() => (prefsSaved.value = false), 2000)
  } catch (e) {
    loadError.value = e.response?.data?.error || e.message
  } finally {
    prefsSaving.value = false
  }
}
onMounted(() => {
  if (!wsStore.list?.length) wsStore.load?.()
  load()
})

function channelTitle(c) {
  return c.label || typeLabel(c.type)
}
function channelName(id) {
  const c = channels.value.find((x) => x.id === id)
  return c ? channelTitle(c) : '—'
}
// `notification_routes.channel_ids` is a plain uuid[] with no FK, and the backend
// doesn't scrub it on channel delete — so warn up front instead of silently
// leaving routes pointing at a dead channel (they'd render as «—»).
function usedInRoutesHint(c) {
  const n = routes.value.filter((r) => (r.channel_ids || []).includes(c.id)).length
  if (!n) return ''
  return ` ${t('settings.notifications.confirm.channelUsedIn', n, { named: { n } })}`
}
// This browser's own device channel (for the «это устройство» badge + permission).
function isThisDevice(c) {
  return c.type === 'device' && c.config?.device_id === myDeviceId
}

// ── channel editor ───────────────────────────────────────────
const chModal = ref(false)
const chEditing = ref(null) // null = create
const chForm = reactive({
  type: 'email',
  label: '',
  config: {},
  secret: {},
  template: '',
  enabled: true,
  hasSecret: false, // a secret is already stored on the server
  clearSecret: false, // erase the stored secret on save
})
const chSaving = ref(false)
const chErr = ref('')

function openChannelNew() {
  chEditing.value = null
  Object.assign(chForm, {
    type: 'email',
    label: '',
    config: { address: auth.user?.email || '' },
    secret: {},
    template: '',
    enabled: true,
    hasSecret: false,
    clearSecret: false,
  })
  chErr.value = ''
  chModal.value = true
}
function openChannelEdit(c) {
  chEditing.value = c
  Object.assign(chForm, {
    type: c.type,
    label: c.label || '',
    config: { ...(c.config || {}) },
    secret: {},
    template: c.template || '',
    enabled: c.enabled,
    hasSecret: c.has_secret === true,
    clearSecret: false,
  })
  chErr.value = ''
  chModal.value = true
}
async function saveChannel() {
  chSaving.value = true
  chErr.value = ''
  try {
    const payload = {
      type: chForm.type,
      label: chForm.label,
      config: chForm.config,
      secret: chForm.secret,
      clear_secret: chForm.clearSecret,
      template: chForm.template,
      enabled: chForm.enabled,
    }
    if (chEditing.value) await chApi.update(chEditing.value.id, payload)
    else await chApi.create(payload)
    chModal.value = false
    await load()
  } catch (e) {
    chErr.value = e.response?.data?.error || e.message
  } finally {
    chSaving.value = false
  }
}
async function removeChannel(c) {
  await chApi.remove(c.id)
  await load()
}
async function toggleChannel(c) {
  await chApi.update(c.id, {
    type: c.type,
    label: c.label,
    config: c.config || {},
    secret: {},
    enabled: !c.enabled,
  })
  await load()
}

// test sends are per-channel; track transient state by id
const testState = reactive({})
async function testChannel(c) {
  testState[c.id] = { pending: true }
  try {
    const res = await chApi.test(c.id)
    testState[c.id] = { ok: true, msg: res.data?.warning || t('settings.notifications.testSent') }
    await load()
  } catch (e) {
    testState[c.id] = { ok: false, msg: e.response?.data?.error || e.message }
  }
}

// ── template editor (separate modal) ─────────────────────────
const tplModal = ref(false)
const tplDraft = ref('')
const tplPreview = ref('')
const tplError = ref('')
const taRef = ref(null)
const hlRef = ref(null)

// Go template actions, not vue-i18n ones. They stay in code rather than in the
// bundle for two reasons: they are a backend contract, identical in every
// language, and a literal `{{ }}` in a message would be eaten by vue-i18n's own
// interpolation. Only the descriptions are translated.
const TEMPLATE_TOKENS = [
  ['text', '{{.Text}}'],
  ['title', '{{.Title}}'],
  ['kind', '{{.Kind}}'],
  ['taskNumber', '{{.TaskNumber}}'],
  ['taskTitle', '{{.TaskTitle}}'],
  ['actor', '{{.Actor}}'],
  ['workspace', '{{.Workspace}}'],
  ['link', '{{.Link}}'],
]
const templateDefaultHint = computed(() =>
  t('settings.notifications.template.defaultHint', { token: '{{.Text}}' }),
)
const syntaxHint = computed(() =>
  t('settings.notifications.template.syntax', { field: '{{.Field}}', cond: '{{if .X}}…{{end}}' }),
)
const TEMPLATE_FIELDS = computed(() =>
  TEMPLATE_TOKENS.map(([key, token]) => ({
    token,
    desc: t(`settings.notifications.template.field.${key}`),
  })),
)

function escapeHtml(s) {
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}
// Highlight {{ ... }} template actions over the (escaped) source for the backdrop.
const highlighted = computed(() => {
  const h = escapeHtml(tplDraft.value || '').replace(
    /\{\{[^}]*\}\}/g,
    (m) => `<span class="tok">${m}</span>`,
  )
  return h.endsWith('\n') ? h + ' ' : h // keep the last empty line visible
})

function syncScroll() {
  if (hlRef.value && taRef.value) {
    hlRef.value.scrollTop = taRef.value.scrollTop
    hlRef.value.scrollLeft = taRef.value.scrollLeft
  }
}

let tplTimer = null
function schedulePreview() {
  clearTimeout(tplTimer)
  tplTimer = setTimeout(runPreview, 300)
}
async function runPreview() {
  try {
    const res = await chApi.previewTemplate(tplDraft.value)
    if (res.data.ok) {
      tplPreview.value = res.data.text
      tplError.value = ''
    } else {
      tplError.value = res.data.error
    }
  } catch (e) {
    tplError.value = e.response?.data?.error || e.message
  }
}

function openTemplate() {
  tplDraft.value = chForm.template || ''
  tplPreview.value = ''
  tplError.value = ''
  tplModal.value = true
  nextTick(runPreview)
}
function insertField(token) {
  const ta = taRef.value
  if (!ta) {
    tplDraft.value += token
    return
  }
  const s = ta.selectionStart ?? tplDraft.value.length
  const e = ta.selectionEnd ?? s
  tplDraft.value = tplDraft.value.slice(0, s) + token + tplDraft.value.slice(e)
  nextTick(() => {
    ta.focus()
    ta.selectionStart = ta.selectionEnd = s + token.length
  })
}
function applyTemplate() {
  chForm.template = tplDraft.value
  tplModal.value = false
}
function clearTemplate() {
  tplDraft.value = ''
}
watch(tplDraft, schedulePreview)

// ── route editor ─────────────────────────────────────────────
const rtModal = ref(false)
const rtEditing = ref(null)
const rtForm = reactive({
  kinds: [],
  workspace_id: null,
  channel_ids: [],
  mute: false,
  enabled: true,
})
const rtSaving = ref(false)
const rtErr = ref('')

function openRouteNew() {
  rtEditing.value = null
  Object.assign(rtForm, {
    kinds: [],
    workspace_id: null,
    channel_ids: [],
    mute: false,
    enabled: true,
  })
  rtErr.value = ''
  rtModal.value = true
}
function openRouteEdit(r) {
  rtEditing.value = r
  Object.assign(rtForm, {
    kinds: r.matcher?.kinds || [],
    workspace_id: r.matcher?.workspace_id || null,
    channel_ids: r.channel_ids || [],
    mute: r.options?.mute || false,
    enabled: r.enabled,
  })
  rtErr.value = ''
  rtModal.value = true
}
async function saveRoute() {
  rtSaving.value = true
  rtErr.value = ''
  try {
    const matcher = {}
    if (rtForm.kinds.length) matcher.kinds = rtForm.kinds
    if (rtForm.workspace_id) matcher.workspace_id = rtForm.workspace_id
    const payload = {
      matcher,
      channel_ids: rtForm.mute ? [] : rtForm.channel_ids,
      options: { mute: rtForm.mute },
      enabled: rtForm.enabled,
    }
    if (rtEditing.value)
      await rtApi.update(rtEditing.value.id, { ...payload, position: rtEditing.value.position })
    else await rtApi.create(payload)
    rtModal.value = false
    await load()
  } catch (e) {
    rtErr.value = e.response?.data?.error || e.message
  } finally {
    rtSaving.value = false
  }
}
async function removeRoute(r) {
  await rtApi.remove(r.id)
  await load()
}
async function toggleRoute(r) {
  await rtApi.update(r.id, {
    position: r.position,
    matcher: r.matcher || {},
    channel_ids: r.channel_ids || [],
    options: r.options || {},
    enabled: !r.enabled,
  })
  await load()
}

// Two keys rather than a conditional suffix glued onto one: the timezone lands
// mid-sentence in some languages, and a bare « (Europe/Moscow)» appended to a
// finished sentence cannot be moved by a translator.
const quietHint = computed(() =>
  theme.timezone
    ? t('settings.notifications.schedule.quietHintTz', { tz: theme.timezone })
    : t('settings.notifications.schedule.quietHint'),
)

function kindsSummary(r) {
  const ks = r.matcher?.kinds
  if (!ks || !ks.length) return t('settings.notifications.route.anyEvent')
  return ks.map(kindLabel).join(', ')
}
function wsSummary(r) {
  const id = r.matcher?.workspace_id
  if (!id) return t('settings.notifications.route.allWorkspaces')
  return (
    wsOptions.value.find((w) => w.value === id)?.label ||
    t('settings.notifications.route.someWorkspace')
  )
}
</script>

<template>
  <section class="card">
    <div class="head">
      <h2>{{ t('settings.notifications.title') }}</h2>
    </div>
    <p class="hint">{{ t('settings.notifications.hint') }}</p>

    <n-spin v-if="loading" size="small" />
    <div v-else-if="loadError" class="err">{{ loadError }}</div>
    <template v-else>
      <!-- Channels -->
      <div class="block">
        <div class="block-head">
          <span class="block-title">{{ t('settings.notifications.channels.title') }}</span>
          <n-button size="tiny" dashed @click="openChannelNew">
            <template #icon><n-icon :component="AddOutline" /></template>
            {{ t('settings.notifications.channels.add') }}
          </n-button>
        </div>
        <empty-state
          v-if="!channels.length"
          :icon="NotificationsOutline"
          :text="t('settings.notifications.channels.empty')"
          size="small"
        />
        <div v-for="c in channels" :key="c.id" class="item" :class="{ off: !c.enabled }">
          <n-icon :component="TYPE_ICONS[c.type] || GlobeOutline" class="item-ico" />
          <div class="item-main">
            <div class="item-title">
              {{ channelTitle(c) }}
              <n-tag v-if="c.verified" size="tiny" type="success" round :bordered="false">
                {{ t('settings.notifications.channels.verified') }}
              </n-tag>
              <n-tag v-if="isThisDevice(c)" size="tiny" type="info" round :bordered="false">
                {{ t('settings.notifications.channels.thisDevice') }}
              </n-tag>
            </div>
            <div class="item-sub">{{ typeLabel(c.type) }}</div>
            <div v-if="isThisDevice(c) && notifPermission !== 'granted'" class="item-test bad">
              <template v-if="notifPermission === 'denied'">
                {{ t('settings.notifications.channels.permDenied') }}
              </template>
              <template v-else>{{ t('settings.notifications.channels.permNeeded') }}</template>
            </div>
            <div
              v-if="testState[c.id]"
              class="item-test"
              :class="testState[c.id].ok ? 'ok' : 'bad'"
            >
              <template v-if="testState[c.id].pending">{{
                t('settings.notifications.channels.testing')
              }}</template>
              <template v-else>{{ testState[c.id].msg }}</template>
            </div>
          </div>
          <div class="item-actions">
            <n-switch :value="c.enabled" size="small" @update:value="() => toggleChannel(c)" />
            <n-button
              v-if="isThisDevice(c) && notifPermission === 'default'"
              size="tiny"
              tertiary
              @click="requestNotifPermission"
            >
              <template #icon><n-icon :component="NotificationsOutline" /></template>
              {{ t('settings.notifications.channels.allow') }}
            </n-button>
            <n-button v-if="c.type !== 'device'" size="tiny" tertiary @click="testChannel(c)">
              <template #icon><n-icon :component="FlashOutline" /></template>
              {{ t('settings.notifications.channels.test') }}
            </n-button>
            <n-button size="tiny" quaternary @click="openChannelEdit(c)">
              <template #icon><n-icon :component="CreateOutline" /></template>
            </n-button>
            <n-popconfirm
              :positive-button-props="{ type: 'error' }"
              :positive-text="t('common.action.delete')"
              @positive-click="removeChannel(c)"
            >
              <template #trigger>
                <n-button size="tiny" quaternary>
                  <template #icon><n-icon :component="TrashOutline" /></template>
                </n-button>
              </template>
              {{ t('settings.notifications.confirm.deleteChannel', { name: channelTitle(c) })
              }}{{ usedInRoutesHint(c) }}
            </n-popconfirm>
          </div>
        </div>
      </div>

      <!-- Routes -->
      <div class="block">
        <div class="block-head">
          <span class="block-title">{{ t('settings.notifications.routes.title') }}</span>
          <n-button size="tiny" dashed :disabled="!channels.length" @click="openRouteNew">
            <template #icon><n-icon :component="AddOutline" /></template>
            {{ t('settings.notifications.routes.add') }}
          </n-button>
        </div>
        <p class="hint sub">{{ t('settings.notifications.routes.hint') }}</p>
        <empty-state
          v-if="!routes.length"
          :icon="ShareSocialOutline"
          :text="t('settings.notifications.routes.empty')"
          size="small"
        />
        <div v-for="r in routes" :key="r.id" class="item" :class="{ off: !r.enabled }">
          <div class="item-main">
            <div class="item-title">{{ kindsSummary(r) }}</div>
            <div class="item-sub">
              <template v-if="r.options?.mute">
                <n-tag size="tiny" type="warning" round :bordered="false">{{
                  t('settings.notifications.routes.muted')
                }}</n-tag>
              </template>
              <template v-else>
                →
                {{
                  (r.channel_ids || []).map(channelName).join(', ') ||
                  t('settings.notifications.routes.noChannels')
                }}
              </template>
              · {{ wsSummary(r) }}
            </div>
          </div>
          <div class="item-actions">
            <n-switch :value="r.enabled" size="small" @update:value="() => toggleRoute(r)" />
            <n-button size="tiny" quaternary @click="openRouteEdit(r)">
              <template #icon><n-icon :component="CreateOutline" /></template>
            </n-button>
            <n-popconfirm
              :positive-button-props="{ type: 'error' }"
              :positive-text="t('common.action.delete')"
              @positive-click="removeRoute(r)"
            >
              <template #trigger>
                <n-button size="tiny" quaternary>
                  <template #icon><n-icon :component="TrashOutline" /></template>
                </n-button>
              </template>
              {{ t('settings.notifications.confirm.deleteRoute') }}
            </n-popconfirm>
          </div>
        </div>
      </div>

      <!-- Scheduling defaults (due dates + reminders) -->
      <div class="block">
        <div class="block-head">
          <span class="block-title">{{ t('settings.notifications.schedule.title') }}</span>
        </div>
        <p class="hint sub">{{ t('settings.notifications.schedule.hint') }}</p>
        <label class="sched-row">
          <n-switch v-model:value="prefs.due_enabled" size="small" />
          <span>{{ t('settings.notifications.schedule.dueEnabled') }}</span>
        </label>
        <div class="grid2">
          <label class="field">
            <span>{{ t('settings.notifications.schedule.lead') }}</span>
            <n-select
              v-model:value="prefs.due_lead_minutes"
              :options="LEAD_OPTIONS"
              :disabled="!prefs.due_enabled"
            />
          </label>
          <label class="field">
            <span>{{ t('settings.notifications.schedule.repeat') }}</span>
            <n-select
              v-model:value="prefs.due_repeat_minutes"
              :options="REPEAT_OPTIONS"
              :disabled="!prefs.due_enabled"
            />
          </label>
        </div>
        <label class="sched-row">
          <n-switch v-model:value="prefs.reminder_enabled" size="small" />
          <span>{{ t('settings.notifications.schedule.reminderEnabled') }}</span>
        </label>
        <label class="field">
          <span>{{ t('settings.notifications.schedule.digest') }}</span>
          <n-select v-model:value="prefs.digest_minutes" :options="DIGEST_OPTIONS" />
        </label>
        <p class="hint">{{ t('settings.notifications.schedule.digestHint') }}</p>
        <label class="sched-row">
          <n-switch v-model:value="prefs.quiet_enabled" size="small" />
          <span>{{ t('settings.notifications.schedule.quiet') }}</span>
        </label>
        <template v-if="prefs.quiet_enabled">
          <div class="grid2">
            <label class="field">
              <span>{{ t('settings.notifications.schedule.quietFrom') }}</span>
              <n-select v-model:value="prefs.quiet_start_minutes" :options="TIME_OPTIONS" />
            </label>
            <label class="field">
              <span>{{ t('settings.notifications.schedule.quietTo') }}</span>
              <n-select v-model:value="prefs.quiet_end_minutes" :options="TIME_OPTIONS" />
            </label>
          </div>
          <p class="hint">{{ quietHint }}</p>
        </template>
        <div class="row-end">
          <transition name="fade">
            <span v-if="prefsSaved" class="saved-tick">{{ t('common.state.saved') }}</span>
          </transition>
          <n-button size="small" type="primary" :loading="prefsSaving" @click="savePrefs">
            {{ t('common.action.save') }}
          </n-button>
        </div>
      </div>
    </template>

    <!-- Channel editor modal -->
    <n-modal v-model:show="chModal">
      <n-card
        class="modal"
        :title="
          chEditing
            ? t('settings.notifications.channelForm.editTitle')
            : t('settings.notifications.channelForm.newTitle')
        "
        closable
        @close="chModal = false"
      >
        <div class="form">
          <label class="field">
            <span>{{ t('settings.notifications.channelForm.type') }}</span>
            <n-select
              v-model:value="chForm.type"
              :options="chEditing ? allTypeOptions : typeOptions"
              :disabled="!!chEditing"
            />
          </label>
          <label class="field">
            <span>{{ t('settings.notifications.channelForm.label') }}</span>
            <n-input
              v-model:value="chForm.label"
              :placeholder="t('settings.notifications.channelForm.labelPlaceholder')"
            />
          </label>

          <!-- email -->
          <template v-if="chForm.type === 'email'">
            <label class="field">
              <span>{{ t('settings.notifications.channelForm.address') }}</span>
              <n-input
                v-model:value="chForm.config.address"
                :placeholder="t('settings.notifications.channelForm.addressPlaceholder')"
              />
            </label>
          </template>

          <!-- telegram -->
          <template v-else-if="chForm.type === 'telegram'">
            <label class="field">
              <span>{{ t('settings.notifications.channelForm.chatId') }}</span>
              <n-input
                v-model:value="chForm.config.chat_id"
                :placeholder="t('settings.notifications.channelForm.chatIdPlaceholder')"
              />
            </label>
            <label class="field">
              <span>{{ t('settings.notifications.channelForm.botToken') }}</span>
              <n-input
                v-model:value="chForm.secret.bot_token"
                type="password"
                show-password-on="click"
                :placeholder="
                  chEditing
                    ? t('settings.notifications.channelForm.keepSecret')
                    : t('settings.notifications.channelForm.botTokenPlaceholder')
                "
              />
            </label>
            <p class="hint">{{ t('settings.notifications.channelForm.telegramHint') }}</p>
          </template>

          <!-- device (auto-registered client) -->
          <template v-else-if="chForm.type === 'device'">
            <p class="hint">{{ t('settings.notifications.channelForm.deviceHint') }}</p>
          </template>

          <!-- shoutrrr (generic) -->
          <template v-else-if="chForm.type === 'shoutrrr'">
            <label class="field">
              <span>{{ t('settings.notifications.channelForm.serviceUrl') }}</span>
              <n-input
                v-model:value="chForm.secret.url"
                type="password"
                show-password-on="click"
                :placeholder="
                  chEditing
                    ? t('settings.notifications.channelForm.keepSecret')
                    : t('settings.notifications.channelForm.serviceUrlPlaceholder')
                "
              />
            </label>
            <p class="hint">{{ t('settings.notifications.channelForm.shoutrrrHint') }}</p>
          </template>

          <!-- webhook -->
          <template v-else-if="chForm.type === 'webhook'">
            <label class="field">
              <span>{{ t('settings.notifications.channelForm.url') }}</span>
              <n-input
                v-model:value="chForm.config.url"
                :placeholder="t('settings.notifications.channelForm.urlPlaceholder')"
              />
            </label>
            <label class="field">
              <span>{{ t('settings.notifications.channelForm.method') }}</span>
              <n-input
                v-model:value="chForm.config.method"
                :placeholder="t('settings.notifications.channelForm.methodPlaceholder')"
              />
            </label>
            <label class="field">
              <span>{{ t('settings.notifications.channelForm.authHeader') }}</span>
              <SecretInput
                v-model:value="chForm.secret.auth_header"
                v-model:cleared="chForm.clearSecret"
                :stored="chEditing ? chForm.hasSecret : false"
                :placeholder="t('settings.notifications.channelForm.authHeaderPlaceholder')"
                :stored-placeholder="t('settings.notifications.channelForm.keepSecret')"
              />
            </label>
          </template>

          <div class="field">
            <span>{{ t('settings.notifications.template.title') }}</span>
            <div class="tpl-row">
              <code class="tpl-inline">{{ chForm.template || templateDefaultHint }}</code>
              <n-button size="tiny" tertiary @click="openTemplate">{{
                t('common.action.edit')
              }}</n-button>
            </div>
          </div>

          <div v-if="chErr" class="err">{{ chErr }}</div>
        </div>
        <template #footer>
          <div class="row-end">
            <n-button @click="chModal = false">{{ t('common.action.cancel') }}</n-button>
            <n-button type="primary" :loading="chSaving" @click="saveChannel">{{
              t('common.action.save')
            }}</n-button>
          </div>
        </template>
      </n-card>
    </n-modal>

    <!-- Template editor modal -->
    <n-modal v-model:show="tplModal">
      <n-card
        class="modal tpl-modal"
        :title="t('settings.notifications.template.title')"
        closable
        @close="tplModal = false"
      >
        <div class="tpl-grid">
          <div class="tpl-left">
            <div class="editor">
              <!-- safe: `highlighted` HTML-escapes the source before wrapping {{…}} in spans -->
              <!-- eslint-disable-next-line vue/no-v-html -->
              <pre ref="hlRef" class="hl" v-html="highlighted"></pre>
              <textarea
                ref="taRef"
                v-model="tplDraft"
                class="ta"
                spellcheck="false"
                :placeholder="t('settings.notifications.template.editorPlaceholder')"
                @scroll="syncScroll"
              ></textarea>
            </div>
            <div class="tpl-preview">
              <div class="tpl-preview-head">
                {{ t('settings.notifications.template.preview') }}
              </div>
              <div v-if="tplError" class="err mono">{{ tplError }}</div>
              <pre v-else class="prev mono">{{ tplPreview }}</pre>
            </div>
          </div>
          <div class="tpl-hints">
            <div class="hints-title">{{ t('settings.notifications.template.fields') }}</div>
            <button
              v-for="f in TEMPLATE_FIELDS"
              :key="f.token"
              type="button"
              class="hint-field"
              @click="insertField(f.token)"
            >
              <code>{{ f.token }}</code>
              <span>{{ f.desc }}</span>
            </button>
            <p class="hint">{{ syntaxHint }}</p>
          </div>
        </div>
        <template #footer>
          <div class="row-end">
            <n-button quaternary @click="clearTemplate">{{
              t('settings.notifications.template.reset')
            }}</n-button>
            <n-button @click="tplModal = false">{{ t('common.action.cancel') }}</n-button>
            <n-button type="primary" @click="applyTemplate">{{
              t('settings.notifications.template.apply')
            }}</n-button>
          </div>
        </template>
      </n-card>
    </n-modal>

    <!-- Route editor modal -->
    <n-modal v-model:show="rtModal">
      <n-card
        class="modal"
        :title="
          rtEditing
            ? t('settings.notifications.routeForm.editTitle')
            : t('settings.notifications.routeForm.newTitle')
        "
        closable
        @close="rtModal = false"
      >
        <div class="form">
          <label class="field">
            <span>{{ t('settings.notifications.routeForm.kinds') }}</span>
            <n-select v-model:value="rtForm.kinds" :options="kindOptions" multiple clearable />
          </label>
          <label class="field">
            <span>{{ t('settings.notifications.routeForm.workspace') }}</span>
            <n-select
              v-model:value="rtForm.workspace_id"
              :options="wsOptions"
              clearable
              :placeholder="t('settings.notifications.routeForm.workspacePlaceholder')"
            />
          </label>
          <label class="field mute">
            <n-switch v-model:value="rtForm.mute" size="small" />
            <span>{{ t('settings.notifications.routeForm.mute') }}</span>
          </label>
          <label v-if="!rtForm.mute" class="field">
            <span>{{ t('settings.notifications.routeForm.channels') }}</span>
            <n-select v-model:value="rtForm.channel_ids" :options="channelOptions" multiple />
          </label>
          <div v-if="rtErr" class="err">{{ rtErr }}</div>
        </div>
        <template #footer>
          <div class="row-end">
            <n-button @click="rtModal = false">{{ t('common.action.cancel') }}</n-button>
            <n-button type="primary" :loading="rtSaving" @click="saveRoute">{{
              t('common.action.save')
            }}</n-button>
          </div>
        </template>
      </n-card>
    </n-modal>
  </section>
</template>

<style scoped>
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.card h2 {
  font-size: 15px;
  font-weight: 600;
  color: var(--t-text1);
  margin: 0;
}
.block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.block-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.block-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
}
.item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--t-border);
  border-radius: 10px;
  background: var(--t-surface2, transparent);
}
.item.off {
  opacity: 0.55;
}
.item-ico {
  font-size: 20px;
  color: var(--t-text2);
  flex: none;
}
.item-main {
  flex: 1;
  min-width: 0;
}
.item-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;
  color: var(--t-text1);
}
.item-sub {
  font-size: 12px;
  color: var(--t-text3);
  margin-top: 2px;
}
.item-test {
  font-size: 12px;
  margin-top: 3px;
}
.item-test.ok {
  color: #18a058;
}
.item-test.bad {
  color: #e0533d;
}
.item-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: none;
}
.hint {
  font-size: 12px;
  color: var(--t-text3);
  margin: 0;
}
.hint.sub {
  margin-top: -2px;
}
.err {
  font-size: 12px;
  color: #e0533d;
}
.modal {
  width: 460px;
  max-width: 92vw;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.field > span {
  font-size: 12px;
  color: var(--t-text3);
}
.field.mute {
  flex-direction: row;
  align-items: center;
  gap: 8px;
}
.field.mute > span {
  font-size: 13px;
  color: var(--t-text1);
}
.row-end {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
}
.sched-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--t-text1);
}
.grid2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.saved-tick {
  font-size: 13px;
  color: #18a058;
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
@media (max-width: 560px) {
  .grid2 {
    grid-template-columns: 1fr;
  }
}
.tpl-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tpl-inline {
  flex: 1;
  min-width: 0;
  font-family: ui-monospace, monospace;
  font-size: 12px;
  color: var(--t-text2);
  background: var(--t-surface-alt);
  border: 1px solid var(--t-border);
  border-radius: 6px;
  padding: 5px 8px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* template editor modal */
.tpl-modal {
  width: 720px;
  max-width: 94vw;
}
.tpl-grid {
  display: grid;
  grid-template-columns: 1fr 220px;
  gap: 14px;
}
.tpl-left {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}
.editor {
  position: relative;
  height: 170px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  overflow: hidden;
  background: var(--t-surface-alt);
}
.hl,
.ta {
  margin: 0;
  padding: 10px;
  border: 0;
  box-sizing: border-box;
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  font-family: ui-monospace, 'JetBrains Mono', monospace;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
  overflow-wrap: break-word;
  tab-size: 2;
}
.hl {
  overflow: hidden;
  color: var(--t-text1);
  pointer-events: none;
  z-index: 0;
}
.hl :deep(.tok) {
  color: var(--t-primary);
  font-weight: 600;
}
.ta {
  overflow: auto;
  resize: none;
  background: transparent;
  color: transparent;
  caret-color: var(--t-text1);
  z-index: 1;
}
.tpl-preview {
  border: 1px solid var(--t-border);
  border-radius: 8px;
  padding: 8px 10px;
  background: var(--t-surface);
}
.tpl-preview-head {
  font-size: 11px;
  color: var(--t-text3);
  margin-bottom: 4px;
}
.mono {
  font-family: ui-monospace, monospace;
  font-size: 12px;
}
.prev {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--t-text1);
  min-height: 18px;
}
.tpl-hints {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.hints-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text1);
}
.hint-field {
  display: flex;
  flex-direction: column;
  gap: 1px;
  text-align: left;
  background: var(--t-surface-alt);
  border: 1px solid var(--t-border);
  border-radius: 6px;
  padding: 5px 8px;
  cursor: pointer;
}
.hint-field:hover {
  border-color: var(--t-primary);
}
.hint-field code {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  color: var(--t-primary);
}
.hint-field span {
  font-size: 11px;
  color: var(--t-text3);
}
@media (max-width: 560px) {
  .tpl-grid {
    grid-template-columns: 1fr;
  }
}
</style>
