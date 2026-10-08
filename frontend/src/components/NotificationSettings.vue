<script setup>
// Settings → «Уведомления» (v2, #3106, mockup SettingsPage). Same model as
// before — external channels, routing rules checked top to bottom, per-user
// schedule defaults — on v2 surfaces:
//   · channels are cards with their switch, the rest of a channel's actions in
//     a «⋯» ActionMenu (deletion is its two-step item, naming the rules that
//     still deliver there);
//   · the channel and rule editors are popover forms at the button that opened
//     them, not centred modals; the template editor stays a window — it is a
//     two-pane editor with a live preview, tier 2;
//   · the schedule defaults save themselves (the mockup: «сохраняется сразу»),
//     like the switches already did.
// The mockup's event × channel matrix is a different model from our first-match
// rules; the rules are kept and drawn as rows (see #3106).
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NIcon, NInput, NModal, NSelect, NSpin, NSwitch, NTooltip } from 'naive-ui'
import {
  AddOutline,
  CreateOutline,
  EllipsisHorizontal,
  FlashOutline,
  GlobeOutline,
  MailOutline,
  MoonOutline,
  NotificationsOutline,
  PaperPlaneOutline,
  PhonePortraitOutline,
  ShareSocialOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import {
  notificationChannels as chApi,
  notificationRoutes as rtApi,
  notificationPrefs as prefsApi,
} from '@/api'
import { getDeviceId, notificationsSupported } from '@/utils/device'
import { resolvedTimezone } from '@/utils/format'
import { useFormat } from '@/composables/useFormat'
import { workspaceName } from '@/utils/defaultNames'
import { humanizeError } from '@/utils/errors'
import { softFill } from '@/utils/gradient'
import { stubTask } from '@/utils/featureStubs'
import { useAuthStore } from '@/stores/auth'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useThemeStore } from '@/stores/theme'
import SecretInput from '@/components/SecretInput.vue'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import FormField from '@/components/kit/FormField.vue'
import '@/components/settings/settings.css'

const { t } = useI18n()
const auth = useAuthStore()
const wsStore = useWorkspacesStore()
const theme = useThemeStore()
const { formatTime } = useFormat()

// Half-hour options (minutes since midnight) for the quiet window. Labels go
// through the user's clock preference; `timeZone: null` — wall-clock times of
// day, evaluated in `quiet_tz` on the server.
const TIME_OPTIONS = computed(() =>
  Array.from({ length: 48 }, (_, i) => {
    const m = i * 30
    return {
      label: formatTime(new Date(2026, 0, 1, Math.floor(m / 60), m % 60), { timeZone: null }),
      value: m,
    }
  }),
)

const channels = ref([])
const routes = ref([])
const loading = ref(true)
const loadError = ref('')

// Channel type registry — icon and tone; the label is looked up per render so a
// language switch relabels it (#2799). A channel's tone colours its badge; a
// switched-off channel is drawn neutral.
const TYPES = {
  email: { icon: MailOutline, tone: '#f0a020' },
  telegram: { icon: PaperPlaneOutline, tone: '#2aa3d8' },
  webhook: { icon: GlobeOutline, tone: '' },
  shoutrrr: { icon: ShareSocialOutline, tone: '#7c5cff' },
  device: { icon: PhonePortraitOutline, tone: '#3aa675' },
}
const CHANNEL_TYPES = Object.keys(TYPES)
function typeLabel(type) {
  return TYPES[type] ? t(`settings.notifications.channelType.${type}`) : type
}
// `device` channels register themselves per client and aren't addable by hand.
const typeOptions = computed(() =>
  CHANNEL_TYPES.filter((v) => v !== 'device').map((value) => ({ value, label: typeLabel(value) })),
)
const allTypeOptions = computed(() =>
  CHANNEL_TYPES.map((value) => ({ value, label: typeLabel(value) })),
)
function badgeStyle(c) {
  const tone = TYPES[c.type]?.tone
  if (!tone || !c.enabled) return {}
  return { background: softFill(tone), color: tone }
}

const myDeviceId = getDeviceId()
const notifPermission = ref(notificationsSupported() ? Notification.permission : 'unsupported')
async function requestNotifPermission() {
  if (!notificationsSupported()) return
  notifPermission.value = await Notification.requestPermission()
}

// Every kind the server routes (backend internal/notify/locale.go `kinds`), in
// that order. `test` is not here: it is the probe, never a stored notification.
const EVENT_KINDS = [
  'assigned',
  'comment',
  'mention',
  'updated',
  'moved',
  'archived',
  'deleted',
  'unassigned',
  'completed',
  'reopened',
  'due_soon',
  'reminder',
  'integration_sync',
  'conference_invite',
]
function kindLabel(kind) {
  return EVENT_KINDS.includes(kind) ? t(`settings.notifications.kind.${kind}`) : kind
}

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

// ── schedule defaults (due dates + reminders), saved as they change ──
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
const prefsError = ref('')
// Set while the server's answer is copied back in, so that copy is not taken
// for an edit and saved again.
let applying = true

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [c, r, p] = await Promise.all([chApi.list(), rtApi.list(), prefsApi.get()])
    channels.value = c.data || []
    routes.value = r.data || []
    applying = true
    Object.assign(prefs, p.data || {})
    await nextTick()
    applying = false
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}

async function savePrefs() {
  prefsError.value = ''
  try {
    // Quiet hours run on the server's clock in `quiet_tz`; an empty profile zone
    // would be UTC there, not «по вашему часовому поясу» as the hint promises —
    // so send the browser's own zone, the one this screen renders in.
    prefs.quiet_tz = resolvedTimezone(theme.timezone) || prefs.quiet_tz
    const res = await prefsApi.update({ ...prefs })
    applying = true
    Object.assign(prefs, res.data || {})
    await nextTick()
    applying = false
  } catch (e) {
    applying = false
    prefsError.value = e.message
  }
}
let prefsTimer = null
watch(
  prefs,
  () => {
    if (applying) return
    clearTimeout(prefsTimer)
    prefsTimer = setTimeout(savePrefs, 500)
  },
  { deep: true },
)
defineExpose({ savePrefs })

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
// What the channel points at, under its title.
function channelDetail(c) {
  const cfg = c.config || {}
  return cfg.address || cfg.chat_id || cfg.url || ''
}
// The server scrubs a deleted channel out of `channel_ids`, so a rule that named
// only this channel is left delivering nowhere — said before the deletion.
function usedInRoutesHint(c) {
  const n = routes.value.filter((r) => (r.channel_ids || []).includes(c.id)).length
  if (!n) return ''
  return t('settings.notifications.confirm.channelUsedIn', n, { named: { n } })
}
function isThisDevice(c) {
  return c.type === 'device' && c.config?.device_id === myDeviceId
}

// ── «⋯» menus ──
const menu = reactive({ show: false, anchor: null, kind: '', item: null, busy: false })
const failed = ref('')
function openMenu(kind, item, e) {
  menu.kind = kind
  menu.item = item
  menu.anchor = e.currentTarget
  failed.value = ''
  menu.show = true
}
const menuItems = computed(() => {
  const it = menu.item
  if (!it) return []
  if (menu.kind === 'channel') {
    const items = []
    if (isThisDevice(it) && notifPermission.value === 'default') {
      items.push({
        key: 'allow',
        label: t('settings.notifications.channels.allow'),
        icon: NotificationsOutline,
      })
    }
    if (it.type !== 'device') {
      items.push({
        key: 'test',
        label: t('settings.notifications.channels.test'),
        icon: FlashOutline,
      })
    }
    items.push({ key: 'edit', label: t('common.action.edit'), icon: CreateOutline })
    items.push({ gap: true })
    items.push(
      deleteItem(
        t('settings.notifications.confirm.deleteChannel', { name: channelTitle(it) }),
        usedInRoutesHint(it),
      ),
    )
    return items
  }
  return [
    { key: 'edit', label: t('common.action.edit'), icon: CreateOutline },
    { gap: true },
    deleteItem(t('settings.notifications.confirm.deleteRoute'), ''),
  ]
})
function deleteItem(title, sub) {
  const item = {
    key: 'remove',
    label: t('common.action.delete'),
    danger: true,
    confirm: { title, sub: sub || t('settings.notifications.confirm.irreversible') },
  }
  if (menu.busy) item.busy = true
  else if (failed.value) item.failed = { title: t('common.menu.failed'), sub: failed.value }
  return item
}
async function onMenu(key) {
  const it = menu.item
  if (!it) return
  if (key === 'allow') return requestNotifPermission()
  if (key === 'test') return testChannel(it)
  if (key === 'edit') {
    const anchor = menu.anchor
    if (menu.kind === 'channel') openChannelEdit(it, anchor)
    else openRouteEdit(it, anchor)
    return
  }
  if (key === 'remove') {
    menu.busy = true
    failed.value = ''
    try {
      if (menu.kind === 'channel') await chApi.remove(it.id)
      else await rtApi.remove(it.id)
      menu.show = false
      await load()
    } catch (e) {
      failed.value = humanizeError(e)
    } finally {
      menu.busy = false
    }
  }
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

const testState = reactive({})
async function testChannel(c) {
  testState[c.id] = { pending: true }
  try {
    const res = await chApi.test(c.id)
    // A probe that went through can still carry a warning (no SMTP on the
    // server) — a sentinel the interceptor never saw, so translate it here.
    const warning = res.data?.warning
    testState[c.id] = {
      ok: true,
      msg: warning ? humanizeError(warning) : t('settings.notifications.testSent'),
    }
    await load()
  } catch (e) {
    testState[c.id] = { ok: false, msg: e.message }
  }
}

// ── channel editor (popover form) ──
const chForm = reactive({
  show: false,
  anchor: null,
  editing: null,
  type: 'email',
  label: '',
  config: {},
  secret: {},
  template: '',
  enabled: true,
  hasSecret: false,
  clearSecret: false,
  saving: false,
  error: '',
})
function openChannelNew(e) {
  Object.assign(chForm, {
    anchor: e.currentTarget,
    editing: null,
    type: 'email',
    label: '',
    config: { address: auth.user?.email || '' },
    secret: {},
    template: '',
    enabled: true,
    hasSecret: false,
    clearSecret: false,
    error: '',
  })
  chForm.show = true
}
function openChannelEdit(c, anchor) {
  Object.assign(chForm, {
    anchor,
    editing: c,
    type: c.type,
    label: c.label || '',
    config: { ...(c.config || {}) },
    secret: {},
    template: c.template || '',
    enabled: c.enabled,
    hasSecret: c.has_secret === true,
    clearSecret: false,
    error: '',
  })
  chForm.show = true
}
async function saveChannel() {
  chForm.saving = true
  chForm.error = ''
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
    if (chForm.editing) await chApi.update(chForm.editing.id, payload)
    else await chApi.create(payload)
    chForm.show = false
    await load()
  } catch (e) {
    chForm.error = e.message
  } finally {
    chForm.saving = false
  }
}

// ── template editor (a window: two panes and a live preview) ──
const tplModal = ref(false)
const tplDraft = ref('')
const tplPreview = ref('')
const tplError = ref('')
const taRef = ref(null)
const hlRef = ref(null)

// Go template actions — a backend contract, identical in every language, and a
// literal `{{ }}` in a message would be eaten by vue-i18n's interpolation.
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
const highlighted = computed(() => {
  const h = escapeHtml(tplDraft.value || '').replace(
    /\{\{[^}]*\}\}/g,
    (m) => `<span class="tok">${m}</span>`,
  )
  return h.endsWith('\n') ? h + ' ' : h
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
    tplError.value = e.message
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
watch(tplDraft, schedulePreview)

// ── rule editor (popover form) ──
const rtForm = reactive({
  show: false,
  anchor: null,
  editing: null,
  kinds: [],
  workspace_id: null,
  channel_ids: [],
  mute: false,
  enabled: true,
  saving: false,
  error: '',
})
function openRouteNew(e) {
  Object.assign(rtForm, {
    anchor: e.currentTarget,
    editing: null,
    kinds: [],
    workspace_id: null,
    channel_ids: [],
    mute: false,
    enabled: true,
    error: '',
  })
  rtForm.show = true
}
function openRouteEdit(r, anchor) {
  Object.assign(rtForm, {
    anchor,
    editing: r,
    kinds: r.matcher?.kinds || [],
    workspace_id: r.matcher?.workspace_id || null,
    channel_ids: r.channel_ids || [],
    mute: r.options?.mute || false,
    enabled: r.enabled,
    error: '',
  })
  rtForm.show = true
}
async function saveRoute() {
  rtForm.saving = true
  rtForm.error = ''
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
    if (rtForm.editing)
      await rtApi.update(rtForm.editing.id, { ...payload, position: rtForm.editing.position })
    else await rtApi.create(payload)
    rtForm.show = false
    await load()
  } catch (e) {
    rtForm.error = e.message
  } finally {
    rtForm.saving = false
  }
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

// Two keys rather than a suffix: the zone lands mid-sentence in some languages.
const quietHint = computed(() =>
  theme.timezone
    ? t('settings.notifications.schedule.quietHintTz', { tz: theme.timezone })
    : t('settings.notifications.schedule.quietHint'),
)
const quietExceptionsTask = stubTask('notifications.quietExceptions')

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
  <div class="ns" data-testid="notification-settings">
    <p class="st-hint ns-lead">{{ t('settings.notifications.hint') }}</p>

    <n-spin v-if="loading" size="small" />
    <div v-else-if="loadError" class="st-err">{{ loadError }}</div>
    <template v-else>
      <!-- Channels -->
      <div class="ns-channels" data-testid="notif-channels">
        <article
          v-for="c in channels"
          :key="c.id"
          class="ns-ch"
          :class="{ off: !c.enabled }"
          data-testid="notif-channel"
        >
          <div class="ns-ch-top">
            <span class="ns-badge" :style="badgeStyle(c)">
              <n-icon :component="TYPES[c.type]?.icon || GlobeOutline" :size="17" />
            </span>
            <div class="ns-ch-main">
              <div class="ns-ch-title">
                <span class="ns-ch-name">{{ channelTitle(c) }}</span>
                <span v-if="c.verified" class="ns-tag ok">{{
                  t('settings.notifications.channels.verified')
                }}</span>
                <span v-if="isThisDevice(c)" class="ns-tag">{{
                  t('settings.notifications.channels.thisDevice')
                }}</span>
              </div>
              <div class="ns-ch-sub">
                {{ typeLabel(c.type)
                }}<template v-if="channelDetail(c)"> · {{ channelDetail(c) }}</template>
              </div>
            </div>
            <button
              type="button"
              class="ns-more static"
              :aria-label="t('common.action.more')"
              data-testid="notif-channel-menu"
              @click="openMenu('channel', c, $event)"
            >
              <n-icon :component="EllipsisHorizontal" :size="16" />
            </button>
            <n-switch
              :value="c.enabled"
              size="small"
              :aria-label="channelTitle(c)"
              @update:value="() => toggleChannel(c)"
            />
          </div>
          <div v-if="isThisDevice(c) && notifPermission !== 'granted'" class="ns-line bad">
            <template v-if="notifPermission === 'denied'">
              {{ t('settings.notifications.channels.permDenied') }}
            </template>
            <template v-else>{{ t('settings.notifications.channels.permNeeded') }}</template>
          </div>
          <div v-if="testState[c.id]" class="ns-line" :class="testState[c.id].ok ? 'ok' : 'bad'">
            <template v-if="testState[c.id].pending">{{
              t('settings.notifications.channels.testing')
            }}</template>
            <template v-else>{{ testState[c.id].msg }}</template>
          </div>
        </article>
        <button
          type="button"
          class="ns-ch ns-add"
          data-testid="notif-channel-add"
          @click="openChannelNew"
        >
          <n-icon :component="AddOutline" :size="18" />
          {{ t('settings.notifications.channels.add') }}
        </button>
      </div>

      <!-- Rules -->
      <section class="st-card ns-rules" data-testid="notif-routes">
        <div class="st-card-head">
          <span class="st-card-title">{{ t('settings.notifications.routes.title') }}</span>
          <span class="st-card-aside">
            <n-button
              size="small"
              secondary
              :disabled="!channels.length"
              data-testid="notif-route-add"
              @click="openRouteNew"
            >
              <template #icon><n-icon :component="AddOutline" /></template>
              {{ t('settings.notifications.routes.add') }}
            </n-button>
          </span>
        </div>
        <p class="st-hint">{{ t('settings.notifications.routes.hint') }}</p>
        <p v-if="!routes.length" class="ns-empty">
          {{ t('settings.notifications.routes.empty') }}
        </p>
        <div
          v-for="(r, i) in routes"
          :key="r.id"
          class="ns-rule"
          :class="{ off: !r.enabled }"
          data-testid="notif-route"
        >
          <span class="ns-rule-n">{{ i + 1 }}</span>
          <div class="ns-rule-main">
            <div class="ns-rule-title">{{ kindsSummary(r) }}</div>
            <div class="ns-rule-sub">
              <span v-if="r.options?.mute" class="ns-tag warn">{{
                t('settings.notifications.routes.muted')
              }}</span>
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
          <n-switch
            :value="r.enabled"
            size="small"
            :aria-label="kindsSummary(r)"
            @update:value="() => toggleRoute(r)"
          />
          <button
            type="button"
            class="ns-more static"
            :aria-label="t('common.action.more')"
            data-testid="notif-route-menu"
            @click="openMenu('route', r, $event)"
          >
            <n-icon :component="EllipsisHorizontal" :size="16" />
          </button>
        </div>
      </section>

      <!-- Schedule defaults -->
      <div class="ns-sched">
        <section class="st-card" data-testid="notif-schedule">
          <div class="st-card-head">
            <n-icon :component="TimeOutline" :size="16" class="ns-head-ic" />
            <span class="st-card-title">{{ t('settings.notifications.schedule.title') }}</span>
          </div>
          <p class="st-hint">{{ t('settings.notifications.schedule.hint') }}</p>
          <label class="st-toggle">
            <span class="st-toggle-l">{{ t('settings.notifications.schedule.dueEnabled') }}</span>
            <n-switch v-model:value="prefs.due_enabled" size="small" />
          </label>
          <div class="st-grid2">
            <label class="st-field">
              <span class="st-label">{{ t('settings.notifications.schedule.lead') }}</span>
              <n-select
                v-model:value="prefs.due_lead_minutes"
                :options="LEAD_OPTIONS"
                :disabled="!prefs.due_enabled"
              />
            </label>
            <label class="st-field">
              <span class="st-label">{{ t('settings.notifications.schedule.repeat') }}</span>
              <n-select
                v-model:value="prefs.due_repeat_minutes"
                :options="REPEAT_OPTIONS"
                :disabled="!prefs.due_enabled"
              />
            </label>
          </div>
          <label class="st-toggle">
            <span class="st-toggle-l">{{
              t('settings.notifications.schedule.reminderEnabled')
            }}</span>
            <n-switch v-model:value="prefs.reminder_enabled" size="small" />
          </label>
          <label class="st-field">
            <span class="st-label">{{ t('settings.notifications.schedule.digest') }}</span>
            <n-select v-model:value="prefs.digest_minutes" :options="DIGEST_OPTIONS" />
          </label>
          <p class="st-hint">{{ t('settings.notifications.schedule.digestHint') }}</p>
        </section>

        <section class="st-card" data-testid="notif-quiet">
          <label class="st-toggle">
            <n-icon :component="MoonOutline" :size="16" class="ns-head-ic" />
            <span class="st-toggle-l ns-quiet-title">{{
              t('settings.notifications.schedule.quiet')
            }}</span>
            <n-switch v-model:value="prefs.quiet_enabled" size="small" />
          </label>
          <div class="ns-quiet" :class="{ off: !prefs.quiet_enabled }">
            <n-select
              v-model:value="prefs.quiet_start_minutes"
              :options="TIME_OPTIONS"
              :disabled="!prefs.quiet_enabled"
              :aria-label="t('settings.notifications.schedule.quietFrom')"
            />
            <span class="ns-dash">–</span>
            <n-select
              v-model:value="prefs.quiet_end_minutes"
              :options="TIME_OPTIONS"
              :disabled="!prefs.quiet_enabled"
              :aria-label="t('settings.notifications.schedule.quietTo')"
            />
          </div>
          <p class="st-hint">{{ quietHint }}</p>
          <n-tooltip :delay="300">
            <template #trigger>
              <label class="st-toggle st-stub" data-testid="notif-quiet-exceptions">
                <span class="st-toggle-l">{{
                  t('settings.notifications.schedule.quietExceptions')
                }}</span>
                <span class="st-soon">{{ t('common.stub.badge') }}</span>
              </label>
            </template>
            {{ t('common.stub.soon', { n: quietExceptionsTask }) }}
          </n-tooltip>
          <div v-if="prefsError" class="st-err">{{ prefsError }}</div>
        </section>
      </div>
    </template>

    <ActionMenu
      v-model:show="menu.show"
      :anchor="() => menu.anchor"
      placement="bottom-end"
      :width="300"
      :header="
        menu.item
          ? menu.kind === 'channel'
            ? channelTitle(menu.item)
            : kindsSummary(menu.item)
          : ''
      "
      :items="menuItems"
      testid="notif-menu"
      @select="onMenu"
    />

    <PopoverForm
      v-model:show="chForm.show"
      :anchor="() => chForm.anchor"
      placement="bottom-start"
      :offset="{ x: 0, y: 6 }"
      :width="380"
      :title="
        chForm.editing
          ? t('settings.notifications.channelForm.editTitle')
          : t('settings.notifications.channelForm.newTitle')
      "
      :primary="t('common.action.save')"
      :busy="chForm.saving"
      hint="tab"
      testid="notif-channel-form"
      @submit="saveChannel"
    >
      <FormField :label="t('settings.notifications.channelForm.type')">
        <n-select
          v-model:value="chForm.type"
          :options="chForm.editing ? allTypeOptions : typeOptions"
          :disabled="!!chForm.editing"
        />
      </FormField>
      <FormField :label="t('settings.notifications.channelForm.label')">
        <input
          v-model="chForm.label"
          class="kf-input"
          :placeholder="t('settings.notifications.channelForm.labelPlaceholder')"
        />
      </FormField>
      <FormField
        v-if="chForm.type === 'email'"
        :label="t('settings.notifications.channelForm.address')"
      >
        <input
          v-model="chForm.config.address"
          class="kf-input"
          :placeholder="t('settings.notifications.channelForm.addressPlaceholder')"
        />
      </FormField>
      <template v-else-if="chForm.type === 'telegram'">
        <FormField :label="t('settings.notifications.channelForm.chatId')">
          <input
            v-model="chForm.config.chat_id"
            class="kf-input"
            :placeholder="t('settings.notifications.channelForm.chatIdPlaceholder')"
          />
        </FormField>
        <FormField
          :label="t('settings.notifications.channelForm.botToken')"
          :message="t('settings.notifications.channelForm.telegramHint')"
        >
          <n-input
            v-model:value="chForm.secret.bot_token"
            type="password"
            show-password-on="click"
            :placeholder="
              chForm.editing
                ? t('settings.notifications.channelForm.keepSecret')
                : t('settings.notifications.channelForm.botTokenPlaceholder')
            "
          />
        </FormField>
      </template>
      <p v-else-if="chForm.type === 'device'" class="st-hint">
        {{ t('settings.notifications.channelForm.deviceHint') }}
      </p>
      <FormField
        v-else-if="chForm.type === 'shoutrrr'"
        :label="t('settings.notifications.channelForm.serviceUrl')"
        :message="t('settings.notifications.channelForm.shoutrrrHint')"
      >
        <n-input
          v-model:value="chForm.secret.url"
          type="password"
          show-password-on="click"
          :placeholder="
            chForm.editing
              ? t('settings.notifications.channelForm.keepSecret')
              : t('settings.notifications.channelForm.serviceUrlPlaceholder')
          "
        />
      </FormField>
      <template v-else-if="chForm.type === 'webhook'">
        <FormField :label="t('settings.notifications.channelForm.url')">
          <input
            v-model="chForm.config.url"
            class="kf-input"
            :placeholder="t('settings.notifications.channelForm.urlPlaceholder')"
          />
        </FormField>
        <FormField :label="t('settings.notifications.channelForm.method')">
          <input
            v-model="chForm.config.method"
            class="kf-input"
            :placeholder="t('settings.notifications.channelForm.methodPlaceholder')"
          />
        </FormField>
        <FormField :label="t('settings.notifications.channelForm.authHeader')">
          <SecretInput
            v-model:value="chForm.secret.auth_header"
            v-model:cleared="chForm.clearSecret"
            :stored="chForm.editing ? chForm.hasSecret : false"
            :placeholder="t('settings.notifications.channelForm.authHeaderPlaceholder')"
            :stored-placeholder="t('settings.notifications.channelForm.keepSecret')"
          />
        </FormField>
      </template>
      <FormField :label="t('settings.notifications.template.title')">
        <div class="ns-tpl-row">
          <code class="ns-tpl-inline">{{ chForm.template || templateDefaultHint }}</code>
          <n-button size="small" secondary @click="openTemplate">{{
            t('common.action.edit')
          }}</n-button>
        </div>
      </FormField>
      <div v-if="chForm.error" class="st-err" role="alert">{{ chForm.error }}</div>
    </PopoverForm>

    <PopoverForm
      v-model:show="rtForm.show"
      :anchor="() => rtForm.anchor"
      placement="bottom-end"
      :offset="{ x: 0, y: 6 }"
      :width="380"
      :title="
        rtForm.editing
          ? t('settings.notifications.routeForm.editTitle')
          : t('settings.notifications.routeForm.newTitle')
      "
      :primary="t('common.action.save')"
      :busy="rtForm.saving"
      hint="tab"
      testid="notif-route-form"
      @submit="saveRoute"
    >
      <FormField :label="t('settings.notifications.routeForm.kinds')">
        <n-select v-model:value="rtForm.kinds" :options="kindOptions" multiple clearable />
      </FormField>
      <FormField :label="t('settings.notifications.routeForm.workspace')">
        <n-select
          v-model:value="rtForm.workspace_id"
          :options="wsOptions"
          clearable
          :placeholder="t('settings.notifications.routeForm.workspacePlaceholder')"
        />
      </FormField>
      <label class="st-toggle">
        <span class="st-toggle-l">{{ t('settings.notifications.routeForm.mute') }}</span>
        <n-switch v-model:value="rtForm.mute" size="small" />
      </label>
      <FormField v-if="!rtForm.mute" :label="t('settings.notifications.routeForm.channels')">
        <n-select v-model:value="rtForm.channel_ids" :options="channelOptions" multiple />
      </FormField>
      <div v-if="rtForm.error" class="st-err" role="alert">{{ rtForm.error }}</div>
    </PopoverForm>

    <!-- Template editor: tier 2 — over the channel form, hence the z-index. -->
    <n-modal v-model:show="tplModal" :z-index="2100">
      <div class="ns-tpl-win" role="dialog" aria-modal="true">
        <header class="ns-tpl-head">
          <span class="ns-tpl-title">{{ t('settings.notifications.template.title') }}</span>
        </header>
        <div class="ns-tpl-grid">
          <div class="ns-tpl-left">
            <div class="ns-editor">
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
            <div class="ns-tpl-preview">
              <div class="ns-tpl-preview-head">
                {{ t('settings.notifications.template.preview') }}
              </div>
              <div v-if="tplError" class="st-err mono">{{ tplError }}</div>
              <pre v-else class="prev mono">{{ tplPreview }}</pre>
            </div>
          </div>
          <div class="ns-tpl-hints">
            <div class="ns-hints-title">{{ t('settings.notifications.template.fields') }}</div>
            <button
              v-for="f in TEMPLATE_FIELDS"
              :key="f.token"
              type="button"
              class="ns-hint-field"
              @click="insertField(f.token)"
            >
              <code>{{ f.token }}</code>
              <span>{{ f.desc }}</span>
            </button>
            <p class="st-hint">{{ syntaxHint }}</p>
          </div>
        </div>
        <footer class="ns-tpl-foot">
          <n-button quaternary @click="tplDraft = ''">{{
            t('settings.notifications.template.reset')
          }}</n-button>
          <span class="ns-grow" />
          <n-button secondary @click="tplModal = false">{{ t('common.action.cancel') }}</n-button>
          <n-button type="primary" @click="applyTemplate">{{
            t('settings.notifications.template.apply')
          }}</n-button>
        </footer>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.ns {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.ns-lead {
  margin-top: -8px;
}
.ns-channels {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
  gap: 12px;
}
.ns-ch {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  padding: 14px;
  border-radius: var(--t-r-pop);
  background: var(--t-col);
  color: var(--t-text1);
}
.ns-ch-top {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}
.ns-badge {
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
.ns-ch-main {
  flex: 1;
  min-width: 0;
}
.ns-ch-title {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.ns-ch-name {
  font-size: 14px;
  font-weight: 650;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ns-ch-sub {
  margin-top: 2px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ns-ch.off .ns-ch-name,
.ns-ch.off .ns-ch-sub {
  color: var(--t-text3);
}
.ns-tag {
  flex: none;
  padding: 1px 7px;
  border-radius: var(--t-r-pill);
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 10.5px;
  font-weight: 650;
}
.ns-tag.ok {
  background: color-mix(in srgb, #3aa675 16%, var(--t-pill));
  color: #3aa675;
}
.ns-tag.warn {
  background: color-mix(in srgb, #f0a020 16%, var(--t-pill));
  color: #c98612;
}
.ns-line {
  font-size: 12px;
  line-height: 1.4;
}
.ns-line.ok {
  color: #3aa675;
}
.ns-line.bad {
  color: var(--t-error);
}
/* «⋯» — revealed on hover/focus on a pointer device, always there on touch. */
.ns-more {
  flex: none;
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
  opacity: 0;
}
.ns-ch:hover .ns-more,
.ns-rule:hover .ns-more,
.ns-more:focus-visible {
  opacity: 1;
}
/* On a channel card the «⋯» takes no room of its own: with a pointer it floats
   over the end of the title on hover (the card's colour fades the text under
   it), so three cards in a row keep their names. On touch it is in the row. */
@media (hover: hover) {
  .ns-ch {
    position: relative;
  }
  .ns-ch .ns-more {
    position: absolute;
    top: 16px;
    right: 56px;
    background: var(--t-col);
    box-shadow: -10px 0 8px var(--t-col);
  }
  .ns-ch .ns-more:hover {
    background: var(--t-hover);
  }
}
.ns-more:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.ns-more:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
@media (hover: none) {
  .ns-more {
    opacity: 1;
  }
}
.ns-add {
  flex-direction: row;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 64px;
  border: 0;
  background: transparent;
  box-shadow: inset 0 0 0 1.5px var(--t-col);
  color: var(--t-text2);
  font: inherit;
  font-size: 13.5px;
  font-weight: 600;
  cursor: pointer;
}
.ns-add:hover {
  background: var(--t-col);
  color: var(--t-text1);
}
.ns-add:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.ns-rules {
  gap: 10px;
}
.ns-empty {
  margin: 0;
  padding: 14px;
  border-radius: var(--t-r-card);
  background: var(--t-card);
  font-size: 13px;
  color: var(--t-text3);
}
.ns-rule {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 48px;
  padding: 6px 8px 6px 12px;
  border-radius: 12px;
  background: var(--t-card);
}
.ns-rule.off {
  opacity: 0.6;
}
.ns-rule-n {
  flex: none;
  width: 18px;
  font-size: 12px;
  font-weight: 650;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.ns-rule-main {
  flex: 1;
  min-width: 0;
}
.ns-rule-title {
  font-size: 13.5px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ns-rule-sub {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 2px;
  font-size: 12px;
  color: var(--t-text3);
}
.ns-sched {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr);
  gap: 14px;
  align-items: start;
}
.ns-head-ic {
  flex: none;
  color: var(--t-text3);
}
.ns-quiet-title {
  font-weight: 650;
}
.ns-quiet {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ns-quiet.off {
  opacity: 0.6;
}
.ns-dash {
  color: var(--t-text3);
}
.ns-tpl-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ns-tpl-inline {
  flex: 1;
  min-width: 0;
  padding: 7px 10px;
  border-radius: 9px;
  background: var(--t-input-bg);
  color: var(--t-text2);
  font-family: ui-monospace, monospace;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* template editor window */
.ns-tpl-win {
  display: flex;
  flex-direction: column;
  gap: 14px;
  width: min(760px, 94vw);
  padding: 18px 20px;
  border-radius: var(--t-r-col);
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  color: var(--t-text1);
  --t-surface: var(--t-pop);
}
.ns-tpl-title {
  font-size: 15px;
  font-weight: 650;
}
.ns-tpl-grid {
  display: grid;
  grid-template-columns: 1fr 230px;
  gap: 14px;
}
.ns-tpl-left {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}
.ns-editor {
  position: relative;
  height: 180px;
  overflow: hidden;
  border-radius: var(--t-r-ctl);
  background: var(--t-input-bg);
}
.ns-editor:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.hl,
.ta {
  position: absolute;
  inset: 0;
  box-sizing: border-box;
  width: 100%;
  height: 100%;
  margin: 0;
  padding: 10px 12px;
  border: 0;
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
  outline: 0;
  background: transparent;
  color: transparent;
  caret-color: var(--t-text1);
  z-index: 1;
}
.ns-tpl-preview {
  padding: 10px 12px;
  border-radius: var(--t-r-ctl);
  background: var(--t-col);
}
.ns-tpl-preview-head {
  margin-bottom: 4px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.mono {
  font-family: ui-monospace, monospace;
  font-size: 12px;
}
.prev {
  margin: 0;
  min-height: 18px;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--t-text1);
}
.ns-tpl-hints {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.ns-hints-title {
  font-size: 12px;
  font-weight: 650;
  color: var(--t-text2);
}
.ns-hint-field {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 6px 9px;
  border: 0;
  border-radius: 7px;
  background: var(--t-col);
  text-align: left;
  font: inherit;
  cursor: pointer;
}
.ns-hint-field:hover {
  background: var(--t-hover);
}
.ns-hint-field code {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  color: var(--t-primary);
}
.ns-hint-field span {
  font-size: 11px;
  color: var(--t-text3);
}
.ns-tpl-foot {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ns-grow {
  flex: 1;
}
@media (max-width: 900px) {
  .ns-sched {
    grid-template-columns: minmax(0, 1fr);
  }
}
@media (max-width: 560px) {
  .ns-tpl-grid {
    grid-template-columns: 1fr;
  }
}
</style>
