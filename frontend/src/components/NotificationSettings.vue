<script setup>
import { ref, reactive, computed, onMounted, watch, nextTick } from 'vue'
import { NInput, NButton, NSelect, NSwitch, NIcon, NModal, NCard, NTag, NSpin } from 'naive-ui'
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
import { useAuthStore } from '@/stores/auth'
import EmptyState from '@/components/EmptyState.vue'
import SecretInput from '@/components/SecretInput.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useThemeStore } from '@/stores/theme'

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

// Channel type registry — extend here as the backend gains senders.
const TYPE_META = {
  email: { label: 'Email', icon: MailOutline },
  telegram: { label: 'Telegram', icon: PaperPlaneOutline },
  webhook: { label: 'Webhook', icon: GlobeOutline },
  shoutrrr: { label: 'Shoutrrr (любой сервис)', icon: ShareSocialOutline },
  device: { label: 'Системные уведомления', icon: PhonePortraitOutline },
}
// `device` channels auto-register per client and aren't manually addable.
const typeOptions = Object.entries(TYPE_META)
  .filter(([v]) => v !== 'device')
  .map(([value, m]) => ({ value, label: m.label }))
const allTypeOptions = Object.entries(TYPE_META).map(([value, m]) => ({ value, label: m.label }))

const myDeviceId = getDeviceId()
const notifPermission = ref(notificationsSupported() ? Notification.permission : 'unsupported')
async function requestNotifPermission() {
  if (!notificationsSupported()) return
  notifPermission.value = await Notification.requestPermission()
}

const KIND_META = {
  assigned: 'Назначение задачи',
  comment: 'Комментарии',
  mention: 'Упоминания',
  updated: 'Изменение задачи',
  moved: 'Перемещение задачи',
  archived: 'Архивирование',
  due_soon: 'Скоро дедлайн',
  reminder: 'Напоминания (по времени)',
  integration_sync: 'Синхронизация интеграций',
}

// Minute presets for the due-date schedule selects.
const LEAD_OPTIONS = [
  { label: 'В момент дедлайна', value: 0 },
  { label: 'За 15 минут', value: 15 },
  { label: 'За 30 минут', value: 30 },
  { label: 'За час', value: 60 },
  { label: 'За 3 часа', value: 180 },
  { label: 'За день', value: 1440 },
  { label: 'За 2 дня', value: 2880 },
]
const REPEAT_OPTIONS = [
  { label: 'Однократно', value: 0 },
  { label: 'Каждый час', value: 60 },
  { label: 'Каждые 3 часа', value: 180 },
  { label: 'Каждые 6 часов', value: 360 },
  { label: 'Каждый день', value: 1440 },
]
const DIGEST_OPTIONS = [
  { label: 'Выключено (сразу)', value: 0 },
  { label: 'Каждые 5 минут', value: 5 },
  { label: 'Каждые 15 минут', value: 15 },
  { label: 'Каждые 30 минут', value: 30 },
  { label: 'Раз в час', value: 60 },
]
const kindOptions = Object.entries(KIND_META).map(([value, label]) => ({ value, label }))

const wsOptions = computed(() => (wsStore.list || []).map((w) => ({ value: w.id, label: w.name })))
const channelOptions = computed(() =>
  channels.value.map((c) => ({
    value: c.id,
    label: c.label || TYPE_META[c.type]?.label || c.type,
  })),
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

function channelName(id) {
  const c = channels.value.find((x) => x.id === id)
  return c ? c.label || TYPE_META[c.type]?.label || c.type : '—'
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
  if (!confirm(`Удалить канал «${c.label || TYPE_META[c.type]?.label || c.type}»?`)) return
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
    testState[c.id] = { ok: true, msg: res.data?.warning || 'Тест отправлен' }
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

// Avoid literal {{ }} in the Vue template (it would be parsed as interpolation):
// these strings are data, rendered verbatim.
const templateDefaultHint = 'по умолчанию: {{.Text}} + ссылка'
const syntaxHint =
  'Синтаксис Go-шаблонов: {{.Field}}, {{if .X}}…{{end}}. Пусто = шаблон по умолчанию.'
const TEMPLATE_FIELDS = [
  { token: '{{.Text}}', desc: 'Готовый текст уведомления' },
  { token: '{{.Title}}', desc: 'Заголовок по типу события' },
  { token: '{{.Kind}}', desc: 'Тип события (assigned/comment/…)' },
  { token: '{{.TaskNumber}}', desc: 'Номер задачи' },
  { token: '{{.TaskTitle}}', desc: 'Заголовок задачи' },
  { token: '{{.Actor}}', desc: 'Кто инициировал' },
  { token: '{{.Workspace}}', desc: 'Название пространства' },
  { token: '{{.Link}}', desc: 'Ссылка на приложение' },
]

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
  if (!confirm('Удалить правило маршрутизации?')) return
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

function kindsSummary(r) {
  const ks = r.matcher?.kinds
  if (!ks || !ks.length) return 'Любые события'
  return ks.map((k) => KIND_META[k] || k).join(', ')
}
function wsSummary(r) {
  const id = r.matcher?.workspace_id
  if (!id) return 'все пространства'
  return wsOptions.value.find((w) => w.value === id)?.label || 'выбранное пространство'
}
</script>

<template>
  <section class="card">
    <div class="head">
      <h2>Уведомления</h2>
    </div>
    <p class="hint">
      Внутренние уведомления (колокольчик, мобильное приложение) приходят всегда. Здесь
      настраиваются внешние каналы доставки и правила, по которым уведомления в них попадают.
    </p>

    <n-spin v-if="loading" size="small" />
    <div v-else-if="loadError" class="err">{{ loadError }}</div>
    <template v-else>
      <!-- Channels -->
      <div class="block">
        <div class="block-head">
          <span class="block-title">Каналы</span>
          <n-button size="tiny" dashed @click="openChannelNew">
            <template #icon><n-icon :component="AddOutline" /></template>
            Добавить канал
          </n-button>
        </div>
        <empty-state
          v-if="!channels.length"
          :icon="NotificationsOutline"
          text="Каналов пока нет"
          size="small"
        />
        <div v-for="c in channels" :key="c.id" class="item" :class="{ off: !c.enabled }">
          <n-icon :component="TYPE_META[c.type]?.icon || GlobeOutline" class="item-ico" />
          <div class="item-main">
            <div class="item-title">
              {{ c.label || TYPE_META[c.type]?.label || c.type }}
              <n-tag v-if="c.verified" size="tiny" type="success" round :bordered="false">
                проверен
              </n-tag>
              <n-tag v-if="isThisDevice(c)" size="tiny" type="info" round :bordered="false">
                это устройство
              </n-tag>
            </div>
            <div class="item-sub">{{ TYPE_META[c.type]?.label || c.type }}</div>
            <div v-if="isThisDevice(c) && notifPermission !== 'granted'" class="item-test bad">
              <template v-if="notifPermission === 'denied'">
                Уведомления запрещены в браузере — включите их в настройках сайта.
              </template>
              <template v-else>Нужно разрешение браузера на уведомления.</template>
            </div>
            <div
              v-if="testState[c.id]"
              class="item-test"
              :class="testState[c.id].ok ? 'ok' : 'bad'"
            >
              <template v-if="testState[c.id].pending">отправка…</template>
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
              Разрешить
            </n-button>
            <n-button v-if="c.type !== 'device'" size="tiny" tertiary @click="testChannel(c)">
              <template #icon><n-icon :component="FlashOutline" /></template>
              Тест
            </n-button>
            <n-button size="tiny" quaternary @click="openChannelEdit(c)">
              <template #icon><n-icon :component="CreateOutline" /></template>
            </n-button>
            <n-button size="tiny" quaternary @click="removeChannel(c)">
              <template #icon><n-icon :component="TrashOutline" /></template>
            </n-button>
          </div>
        </div>
      </div>

      <!-- Routes -->
      <div class="block">
        <div class="block-head">
          <span class="block-title">Правила маршрутизации</span>
          <n-button size="tiny" dashed :disabled="!channels.length" @click="openRouteNew">
            <template #icon><n-icon :component="AddOutline" /></template>
            Добавить правило
          </n-button>
        </div>
        <p class="hint sub">
          Правила проверяются сверху вниз — срабатывает первое подходящее. Уведомление уходит в его
          каналы (или никуда, если правило «заглушает»).
        </p>
        <empty-state
          v-if="!routes.length"
          :icon="ShareSocialOutline"
          text="Правил нет — внешние каналы не получают уведомления"
          size="small"
        />
        <div v-for="r in routes" :key="r.id" class="item" :class="{ off: !r.enabled }">
          <div class="item-main">
            <div class="item-title">{{ kindsSummary(r) }}</div>
            <div class="item-sub">
              <template v-if="r.options?.mute">
                <n-tag size="tiny" type="warning" round :bordered="false">заглушено</n-tag>
              </template>
              <template v-else>
                → {{ (r.channel_ids || []).map(channelName).join(', ') || '— каналы не выбраны' }}
              </template>
              · {{ wsSummary(r) }}
            </div>
          </div>
          <div class="item-actions">
            <n-switch :value="r.enabled" size="small" @update:value="() => toggleRoute(r)" />
            <n-button size="tiny" quaternary @click="openRouteEdit(r)">
              <template #icon><n-icon :component="CreateOutline" /></template>
            </n-button>
            <n-button size="tiny" quaternary @click="removeRoute(r)">
              <template #icon><n-icon :component="TrashOutline" /></template>
            </n-button>
          </div>
        </div>
      </div>

      <!-- Scheduling defaults (due dates + reminders) -->
      <div class="block">
        <div class="block-head">
          <span class="block-title">Дедлайны и напоминания</span>
        </div>
        <p class="hint sub">
          Когда уведомлять о сроках задач. Можно переопределить для конкретной задачи — кликом по
          дате на карточке. Дробить частоту без спама помогают тихие часы (silence).
        </p>
        <label class="sched-row">
          <n-switch v-model:value="prefs.due_enabled" size="small" />
          <span>Уведомлять о приближении дедлайна задач</span>
        </label>
        <div class="grid2">
          <label class="field">
            <span>Напоминать</span>
            <n-select
              v-model:value="prefs.due_lead_minutes"
              :options="LEAD_OPTIONS"
              :disabled="!prefs.due_enabled"
            />
          </label>
          <label class="field">
            <span>Повтор</span>
            <n-select
              v-model:value="prefs.due_repeat_minutes"
              :options="REPEAT_OPTIONS"
              :disabled="!prefs.due_enabled"
            />
          </label>
        </div>
        <label class="sched-row">
          <n-switch v-model:value="prefs.reminder_enabled" size="small" />
          <span>Доставлять напоминания (reminders) во внешние каналы</span>
        </label>
        <label class="field">
          <span>Группировать в сводку (дайджест)</span>
          <n-select v-model:value="prefs.digest_minutes" :options="DIGEST_OPTIONS" />
        </label>
        <p class="hint">
          Уведомления за окно объединяются в одно сообщение на канал — меньше шума при всплесках.
        </p>
        <label class="sched-row">
          <n-switch v-model:value="prefs.quiet_enabled" size="small" />
          <span>Тихие часы (не беспокоить)</span>
        </label>
        <template v-if="prefs.quiet_enabled">
          <div class="grid2">
            <label class="field">
              <span>С</span>
              <n-select v-model:value="prefs.quiet_start_minutes" :options="TIME_OPTIONS" />
            </label>
            <label class="field">
              <span>До</span>
              <n-select v-model:value="prefs.quiet_end_minutes" :options="TIME_OPTIONS" />
            </label>
          </div>
          <p class="hint">
            В это окно внешние уведомления придерживаются и приходят после его окончания. Внутренние
            (колокольчик) — всегда. Время — по вашему часовому поясу{{
              theme.timezone ? ` (${theme.timezone})` : ''
            }}.
          </p>
        </template>
        <div class="row-end">
          <transition name="fade">
            <span v-if="prefsSaved" class="saved-tick">Сохранено</span>
          </transition>
          <n-button size="small" type="primary" :loading="prefsSaving" @click="savePrefs">
            Сохранить
          </n-button>
        </div>
      </div>
    </template>

    <!-- Channel editor modal -->
    <n-modal v-model:show="chModal">
      <n-card
        class="modal"
        :title="chEditing ? 'Изменить канал' : 'Новый канал'"
        closable
        @close="chModal = false"
      >
        <div class="form">
          <label class="field">
            <span>Тип</span>
            <n-select
              v-model:value="chForm.type"
              :options="chEditing ? allTypeOptions : typeOptions"
              :disabled="!!chEditing"
            />
          </label>
          <label class="field">
            <span>Название</span>
            <n-input v-model:value="chForm.label" placeholder="Напр. «Мой телеграм»" />
          </label>

          <!-- email -->
          <template v-if="chForm.type === 'email'">
            <label class="field">
              <span>Адрес</span>
              <n-input v-model:value="chForm.config.address" placeholder="you@example.com" />
            </label>
          </template>

          <!-- telegram -->
          <template v-else-if="chForm.type === 'telegram'">
            <label class="field">
              <span>Chat ID</span>
              <n-input v-model:value="chForm.config.chat_id" placeholder="123456789 или @канал" />
            </label>
            <label class="field">
              <span>Bot token</span>
              <n-input
                v-model:value="chForm.secret.bot_token"
                type="password"
                show-password-on="click"
                :placeholder="chEditing ? 'оставьте пустым, чтобы не менять' : '123456:ABC-...'"
              />
            </label>
            <p class="hint">
              Создайте бота через @BotFather, отправьте ему сообщение и укажите свой chat_id.
            </p>
          </template>

          <!-- device (auto-registered client) -->
          <template v-else-if="chForm.type === 'device'">
            <p class="hint">
              Это устройство/браузер. Системные уведомления приходят, когда приложение открыто и
              правило направляет события сюда. Можно переименовать, включить/выключить и удалить.
            </p>
          </template>

          <!-- shoutrrr (generic) -->
          <template v-else-if="chForm.type === 'shoutrrr'">
            <label class="field">
              <span>Service URL</span>
              <n-input
                v-model:value="chForm.secret.url"
                type="password"
                show-password-on="click"
                :placeholder="
                  chEditing
                    ? 'оставьте пустым, чтобы не менять'
                    : 'slack://… · discord://… · ntfy://…'
                "
              />
            </label>
            <p class="hint">
              Любой сервис из shoutrrr: slack, discord, ntfy, gotify, matrix, pushover, teams и др.
              Формат URL — см. документацию shoutrrr (containrrr.dev/shoutrrr/services).
            </p>
          </template>

          <!-- webhook -->
          <template v-else-if="chForm.type === 'webhook'">
            <label class="field">
              <span>URL</span>
              <n-input v-model:value="chForm.config.url" placeholder="https://…" />
            </label>
            <label class="field">
              <span>Метод</span>
              <n-input v-model:value="chForm.config.method" placeholder="POST (по умолчанию)" />
            </label>
            <label class="field">
              <span>Заголовок Authorization (необязательно)</span>
              <SecretInput
                v-model:value="chForm.secret.auth_header"
                v-model:cleared="chForm.clearSecret"
                :stored="chEditing ? chForm.hasSecret : false"
                placeholder="Bearer …"
                stored-placeholder="оставьте пустым, чтобы не менять"
              />
            </label>
          </template>

          <div class="field">
            <span>Шаблон сообщения</span>
            <div class="tpl-row">
              <code class="tpl-inline">{{ chForm.template || templateDefaultHint }}</code>
              <n-button size="tiny" tertiary @click="openTemplate">Изменить</n-button>
            </div>
          </div>

          <div v-if="chErr" class="err">{{ chErr }}</div>
        </div>
        <template #footer>
          <div class="row-end">
            <n-button @click="chModal = false">Отмена</n-button>
            <n-button type="primary" :loading="chSaving" @click="saveChannel">Сохранить</n-button>
          </div>
        </template>
      </n-card>
    </n-modal>

    <!-- Template editor modal -->
    <n-modal v-model:show="tplModal">
      <n-card class="modal tpl-modal" title="Шаблон сообщения" closable @close="tplModal = false">
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
                placeholder="Пусто — шаблон по умолчанию"
                @scroll="syncScroll"
              ></textarea>
            </div>
            <div class="tpl-preview">
              <div class="tpl-preview-head">Предпросмотр (на примере данных)</div>
              <div v-if="tplError" class="err mono">{{ tplError }}</div>
              <pre v-else class="prev mono">{{ tplPreview }}</pre>
            </div>
          </div>
          <div class="tpl-hints">
            <div class="hints-title">Поля</div>
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
            <n-button quaternary @click="clearTemplate">Сбросить</n-button>
            <n-button @click="tplModal = false">Отмена</n-button>
            <n-button type="primary" @click="applyTemplate">Применить</n-button>
          </div>
        </template>
      </n-card>
    </n-modal>

    <!-- Route editor modal -->
    <n-modal v-model:show="rtModal">
      <n-card
        class="modal"
        :title="rtEditing ? 'Изменить правило' : 'Новое правило'"
        closable
        @close="rtModal = false"
      >
        <div class="form">
          <label class="field">
            <span>События (пусто = любые)</span>
            <n-select v-model:value="rtForm.kinds" :options="kindOptions" multiple clearable />
          </label>
          <label class="field">
            <span>Пространство (пусто = все)</span>
            <n-select
              v-model:value="rtForm.workspace_id"
              :options="wsOptions"
              clearable
              placeholder="Все пространства"
            />
          </label>
          <label class="field mute">
            <n-switch v-model:value="rtForm.mute" size="small" />
            <span>Заглушить (не отправлять никуда)</span>
          </label>
          <label v-if="!rtForm.mute" class="field">
            <span>Каналы доставки</span>
            <n-select v-model:value="rtForm.channel_ids" :options="channelOptions" multiple />
          </label>
          <div v-if="rtErr" class="err">{{ rtErr }}</div>
        </div>
        <template #footer>
          <div class="row-end">
            <n-button @click="rtModal = false">Отмена</n-button>
            <n-button type="primary" :loading="rtSaving" @click="saveRoute">Сохранить</n-button>
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
