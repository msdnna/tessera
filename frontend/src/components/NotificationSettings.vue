<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { NInput, NButton, NSelect, NSwitch, NIcon, NModal, NCard, NTag, NEmpty, NSpin } from 'naive-ui'
import {
  MailOutline,
  PaperPlaneOutline,
  GlobeOutline,
  AddOutline,
  TrashOutline,
  CreateOutline,
  FlashOutline,
} from '@vicons/ionicons5'
import { notificationChannels as chApi, notificationRoutes as rtApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useWorkspacesStore } from '@/stores/workspaces'

const auth = useAuthStore()
const wsStore = useWorkspacesStore()

const channels = ref([])
const routes = ref([])
const loading = ref(true)
const loadError = ref('')

// Channel type registry — extend here as the backend gains senders.
const TYPE_META = {
  email: { label: 'Email', icon: MailOutline },
  telegram: { label: 'Telegram', icon: PaperPlaneOutline },
  webhook: { label: 'Webhook', icon: GlobeOutline },
}
const typeOptions = Object.entries(TYPE_META).map(([value, m]) => ({ value, label: m.label }))

const KIND_META = {
  assigned: 'Назначение задачи',
  comment: 'Комментарии',
  mention: 'Упоминания',
  due_soon: 'Скоро дедлайн',
}
const kindOptions = Object.entries(KIND_META).map(([value, label]) => ({ value, label }))

const wsOptions = computed(() => (wsStore.list || []).map((w) => ({ value: w.id, label: w.name })))
const channelOptions = computed(() =>
  channels.value.map((c) => ({ value: c.id, label: c.label || TYPE_META[c.type]?.label || c.type })),
)

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [c, r] = await Promise.all([chApi.list(), rtApi.list()])
    channels.value = c.data || []
    routes.value = r.data || []
  } catch (e) {
    loadError.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
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

// ── channel editor ───────────────────────────────────────────
const chModal = ref(false)
const chEditing = ref(null) // null = create
const chForm = reactive({ type: 'email', label: '', config: {}, secret: {}, enabled: true })
const chSaving = ref(false)
const chErr = ref('')

function openChannelNew() {
  chEditing.value = null
  Object.assign(chForm, {
    type: 'email',
    label: '',
    config: { address: auth.user?.email || '' },
    secret: {},
    enabled: true,
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
    enabled: c.enabled,
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

// ── route editor ─────────────────────────────────────────────
const rtModal = ref(false)
const rtEditing = ref(null)
const rtForm = reactive({ kinds: [], workspace_id: null, channel_ids: [], mute: false, enabled: true })
const rtSaving = ref(false)
const rtErr = ref('')

function openRouteNew() {
  rtEditing.value = null
  Object.assign(rtForm, { kinds: [], workspace_id: null, channel_ids: [], mute: false, enabled: true })
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
    if (rtEditing.value) await rtApi.update(rtEditing.value.id, { ...payload, position: rtEditing.value.position })
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
        <n-empty v-if="!channels.length" description="Каналов пока нет" size="small" />
        <div v-for="c in channels" :key="c.id" class="item" :class="{ off: !c.enabled }">
          <n-icon :component="TYPE_META[c.type]?.icon || GlobeOutline" class="item-ico" />
          <div class="item-main">
            <div class="item-title">
              {{ c.label || TYPE_META[c.type]?.label || c.type }}
              <n-tag v-if="c.verified" size="tiny" type="success" round :bordered="false">
                проверен
              </n-tag>
            </div>
            <div class="item-sub">{{ TYPE_META[c.type]?.label || c.type }}</div>
            <div v-if="testState[c.id]" class="item-test" :class="testState[c.id].ok ? 'ok' : 'bad'">
              <template v-if="testState[c.id].pending">отправка…</template>
              <template v-else>{{ testState[c.id].msg }}</template>
            </div>
          </div>
          <div class="item-actions">
            <n-switch :value="c.enabled" size="small" @update:value="() => toggleChannel(c)" />
            <n-button size="tiny" tertiary @click="testChannel(c)">
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
        <n-empty
          v-if="!routes.length"
          description="Правил нет — внешние каналы не получают уведомления"
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
    </template>

    <!-- Channel editor modal -->
    <n-modal v-model:show="chModal">
      <n-card class="modal" :title="chEditing ? 'Изменить канал' : 'Новый канал'" closable @close="chModal = false">
        <div class="form">
          <label class="field">
            <span>Тип</span>
            <n-select v-model:value="chForm.type" :options="typeOptions" :disabled="!!chEditing" />
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
              <n-input
                v-model:value="chForm.secret.auth_header"
                type="password"
                show-password-on="click"
                :placeholder="chEditing ? 'оставьте пустым, чтобы не менять' : 'Bearer …'"
              />
            </label>
          </template>

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

    <!-- Route editor modal -->
    <n-modal v-model:show="rtModal">
      <n-card class="modal" :title="rtEditing ? 'Изменить правило' : 'Новое правило'" closable @close="rtModal = false">
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
  justify-content: flex-end;
  gap: 10px;
}
</style>
