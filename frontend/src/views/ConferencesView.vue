<script setup>
// Conferences section (#2864, subtask #2870): the list of the workspace's calls,
// the «Запланировать» dialog and one conference's lobby.
//
// The route is `/conferences/:id?` — ONE record with an optional param, not two
// sibling records. vue-router marks a link active only when the open route
// shares its record, so as two records opening a conference would take the
// sidebar item dark (the same trap documents fell into, #2727).
//
// Media is deliberately absent here: it belongs to the LiveKit SFU and arrives
// with the media core. What this screen owns is the bookkeeping — who is
// invited, who is in the room, and when the call happens.
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NIcon,
  NInput,
  NModal,
  NCard,
  NDatePicker,
  NRadioGroup,
  NRadioButton,
  NPopconfirm,
  NSpin,
  NTooltip,
  useMessage,
} from 'naive-ui'
import {
  VideocamOutline,
  TrashOutline,
  ArrowBackOutline,
  PeopleOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import { conferences as confApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useFormat } from '@/composables/useFormat'
import { useRealtime } from '@/composables/useRealtime'
import { hueGrad, tagPillBg } from '@/utils/gradient'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const ws = useWorkspacesStore()
const auth = useAuthStore()
const message = useMessage()
const { firstDayOfWeek, dateTimePattern, formatDateTime } = useFormat()

const list = ref([])
const loading = ref(false)
const filter = ref('all')

// The open conference, if the route carries an id: { conference, participants }.
const detail = ref(null)
const detailLoading = ref(false)
const detailMissing = ref(false)
const busy = ref(false)

const openId = computed(() => route.params.id || '')

// ── list ───────────────────────────────────────────────────────────────
async function load() {
  if (!ws.currentId) {
    list.value = []
    return
  }
  loading.value = true
  try {
    const { data } = await confApi.list(ws.currentId, filter.value === 'all' ? '' : filter.value)
    list.value = data || []
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

// ── one conference ─────────────────────────────────────────────────────
async function loadDetail(id) {
  detailLoading.value = true
  detailMissing.value = false
  try {
    const { data } = await confApi.get(id)
    detail.value = data
  } catch (e) {
    // A deleted (or foreign-workspace) conference answers 404 — say so on the
    // page instead of leaving the previous conference on screen under a new id.
    detail.value = null
    detailMissing.value = true
    if (e.response?.status !== 404 && e.response?.status !== 403) message.error(e.message)
  } finally {
    detailLoading.value = false
  }
}

function open(conf) {
  router.push(`/conferences/${conf.id}`)
}
function backToList() {
  router.push('/conferences')
  load()
}

// ── membership actions ─────────────────────────────────────────────────
// join/leave answer the refreshed conference, because the first arrival flips a
// scheduled call to live and the last exit ends it.
async function act(fn, toast) {
  if (!detail.value || busy.value) return
  busy.value = true
  try {
    const { data } = await fn(detail.value.conference.id)
    if (data?.conference) detail.value.conference = data.conference
    const { data: parts } = await confApi.participants(detail.value.conference.id)
    detail.value.participants = parts || []
    if (toast) message.success(t(toast))
  } catch (e) {
    message.error(e.response?.data?.error || e.message)
  } finally {
    busy.value = false
  }
}

const join = () => act(confApi.join, 'conferences.toast.joined')
const leave = () => act(confApi.leave, 'conferences.toast.left')
const end = () => act(confApi.end, 'conferences.toast.ended')

async function remove(conf) {
  try {
    await confApi.remove(conf.id)
    message.success(t('conferences.toast.deleted'))
    if (openId.value === conf.id) backToList()
    else await load()
  } catch (e) {
    message.error(e.response?.data?.error || e.message)
  }
}

// ── schedule dialog ────────────────────────────────────────────────────
const dlg = ref({ show: false, saving: false, title: '', description: '', at: null })

function openDialog() {
  dlg.value = { show: true, saving: false, title: '', description: '', at: null }
}

async function submit() {
  const title = dlg.value.title.trim()
  if (!title) {
    message.warning(t('conferences.create.required'))
    return
  }
  dlg.value.saving = true
  try {
    const { data } = await confApi.create(ws.currentId, {
      title,
      description: dlg.value.description.trim(),
      scheduled_at: dlg.value.at ? new Date(dlg.value.at).toISOString() : null,
    })
    dlg.value.show = false
    message.success(t('conferences.create.created'))
    await load()
    if (data?.id) open(data)
  } catch (e) {
    message.error(e.response?.data?.error || e.message)
  } finally {
    dlg.value.saving = false
  }
}

// ── derived ────────────────────────────────────────────────────────────
// Naive's success green — the codebase has no token for it, and a live call is
// the one place in this view that needs a hue other than the accent.
const LIVE_HUE = '#18a058'

const me = computed(() => auth.user?.id || '')
const myPart = computed(() =>
  (detail.value?.participants || []).find((p) => p.user_id === me.value),
)
const inRoom = computed(() => !!myPart.value?.joined_at && !myPart.value?.left_at)
const canModerate = computed(() => {
  const conf = detail.value?.conference
  if (!conf) return false
  return conf.created_by === me.value || myPart.value?.role === 'host' || auth.isAdmin
})

// Neutral greys stay flat per the design language, so a finished call has no
// hue at all; the two states that still matter carry the same-hue gradient.
function statusHue(status) {
  if (status === 'live') return LIVE_HUE
  if (status === 'scheduled') return 'var(--t-primary)'
  return null
}
function pillStyle(status) {
  const hue = statusHue(status)
  return hue ? { background: tagPillBg(hue) } : { borderColor: 'var(--t-border)' }
}
// The pill's caption rides the same hue as its border, through the global
// .accent-grad-text helper (background-clip: text) with --grad overridden.
function pillTextStyle(status) {
  const hue = statusHue(status)
  return hue ? { '--grad': hueGrad(hue) } : {}
}

function when(ts) {
  return formatDateTime(ts, { day: '2-digit', month: 'short' })
}

// The one time line a row shows: a finished call is described by its end, a
// live one by its start, and a plan by the time it is planned for.
function timeLine(c) {
  if (c.status === 'ended' && c.ended_at)
    return t('conferences.row.endedAt', { when: when(c.ended_at) })
  if (c.status === 'live' && c.started_at)
    return t('conferences.row.startedAt', { when: when(c.started_at) })
  if (c.scheduled_at) return t('conferences.row.scheduledAt', { when: when(c.scheduled_at) })
  return t('conferences.row.noTime')
}

function presence(p) {
  if (p.left_at) return t('conferences.presence.left')
  if (p.joined_at) return t('conferences.presence.inRoom')
  return t('conferences.presence.invited')
}

// ── wiring ─────────────────────────────────────────────────────────────
watch(openId, (id) => {
  if (id) loadDetail(id)
  else {
    detail.value = null
    detailMissing.value = false
  }
})
watch(filter, load)
watch(
  () => ws.currentId,
  () => {
    // Switching workspaces while a conference is open leaves that conference
    // outside the visible scope — go back to the list rather than 403.
    if (openId.value) router.push('/conferences')
    load()
  },
)

// Live updates: a colleague starting a call must appear in the list without a
// reload, since «идёт сейчас» is the whole reason to open this section.
useRealtime(
  (ev) => {
    if (ev.scope !== ws.currentId || !ev.type?.startsWith('conference')) return
    load()
    if (openId.value && ev.type.includes('participant')) {
      confApi
        .participants(openId.value)
        .then(({ data }) => {
          if (detail.value) detail.value.participants = data || []
        })
        .catch(() => {})
    } else if (openId.value) {
      loadDetail(openId.value)
    }
  },
  () => {
    load()
    if (openId.value) loadDetail(openId.value)
  },
)

onMounted(() => {
  load()
  if (openId.value) loadDetail(openId.value)
})
</script>

<template>
  <div class="conf">
    <!-- LIST -->
    <template v-if="!openId">
      <div class="head">
        <div class="head-text">
          <h2 class="h">{{ $t('conferences.title') }}</h2>
          <div class="sub">{{ $t('conferences.hint') }}</div>
        </div>
        <div class="head-actions">
          <n-radio-group v-model:value="filter" size="small">
            <n-radio-button value="all">{{ $t('conferences.filter.all') }}</n-radio-button>
            <n-radio-button value="live">{{ $t('conferences.filter.live') }}</n-radio-button>
            <n-radio-button value="scheduled">
              {{ $t('conferences.filter.scheduled') }}
            </n-radio-button>
            <n-radio-button value="ended">{{ $t('conferences.filter.ended') }}</n-radio-button>
          </n-radio-group>
          <n-button type="primary" :disabled="!ws.currentId" @click="openDialog">
            {{ $t('conferences.schedule') }}
          </n-button>
        </div>
      </div>

      <n-spin :show="loading">
        <div class="rows">
          <div
            v-for="c in list"
            :key="c.id"
            class="row"
            data-testid="conference-row"
            @click="open(c)"
          >
            <span class="pill" :style="pillStyle(c.status)">
              <span
                :class="{ 'accent-grad-text': !!statusHue(c.status) }"
                :style="pillTextStyle(c.status)"
              >
                {{ $t(`conferences.status.${c.status}`) }}
              </span>
            </span>
            <div class="row-body">
              <div class="row-title">{{ c.title }}</div>
              <div class="row-meta">
                <span class="meta-item">
                  <n-icon :component="TimeOutline" :size="13" />{{ timeLine(c) }}
                </span>
                <span class="meta-item">
                  <n-icon :component="PeopleOutline" :size="13" />{{
                    $t('conferences.row.participantCount', { count: c.participant_count })
                  }}
                </span>
                <span v-if="c.active_count" class="meta-item">
                  {{ $t('conferences.row.activeCount', { count: c.active_count }) }}
                </span>
                <span class="meta-item">
                  {{
                    c.created_by_name
                      ? $t('conferences.row.author', { name: c.created_by_name })
                      : $t('conferences.row.authorUnknown')
                  }}
                </span>
              </div>
            </div>
            <!-- .stop: the row itself navigates, and the confirm popup must not
                 open the conference behind it. -->
            <n-popconfirm
              :positive-button-props="{ type: 'error' }"
              :positive-text="$t('conferences.actions.delete')"
              @positive-click="remove(c)"
            >
              <template #trigger>
                <n-button text size="tiny" type="error" @click.stop>
                  <n-icon :component="TrashOutline" />
                </n-button>
              </template>
              {{ $t('conferences.confirm.delete') }}
            </n-popconfirm>
          </div>

          <empty-state
            v-if="!loading && !list.length"
            :icon="VideocamOutline"
            :text="
              !ws.currentId
                ? $t('conferences.noWorkspace')
                : filter === 'all'
                  ? $t('conferences.empty')
                  : $t('conferences.emptyFiltered')
            "
          />
        </div>
      </n-spin>
    </template>

    <!-- ONE CONFERENCE -->
    <template v-else>
      <n-button text class="back" @click="backToList">
        <n-icon :component="ArrowBackOutline" />
        {{ $t('conferences.actions.back') }}
      </n-button>

      <n-spin :show="detailLoading">
        <empty-state
          v-if="detailMissing"
          :icon="VideocamOutline"
          :text="$t('conferences.detail.notFound')"
        />
        <div v-else-if="detail" class="detail">
          <div class="head">
            <div class="head-text">
              <h2 class="h">{{ detail.conference.title }}</h2>
              <div class="sub">{{ timeLine(detail.conference) }}</div>
            </div>
            <div class="head-actions">
              <span class="pill" :style="pillStyle(detail.conference.status)">
                <span
                  :class="{ 'accent-grad-text': !!statusHue(detail.conference.status) }"
                  :style="pillTextStyle(detail.conference.status)"
                >
                  {{ $t(`conferences.status.${detail.conference.status}`) }}
                </span>
              </span>
              <n-button
                v-if="!inRoom"
                type="primary"
                :loading="busy"
                :disabled="detail.conference.status === 'ended'"
                data-testid="conference-join"
                @click="join"
              >
                {{ $t('conferences.actions.join') }}
              </n-button>
              <n-button v-else :loading="busy" data-testid="conference-leave" @click="leave">
                {{ $t('conferences.actions.leave') }}
              </n-button>
              <n-popconfirm
                v-if="canModerate && detail.conference.status !== 'ended'"
                :positive-text="$t('conferences.actions.end')"
                @positive-click="end"
              >
                <template #trigger>
                  <n-button quaternary>{{ $t('conferences.actions.end') }}</n-button>
                </template>
                {{ $t('conferences.confirm.end') }}
              </n-popconfirm>
            </div>
          </div>

          <p v-if="detail.conference.description" class="desc">
            {{ detail.conference.description }}
          </p>

          <div class="panes">
            <n-card size="small" :title="$t('conferences.detail.roomTitle')" class="room">
              <empty-state
                :icon="VideocamOutline"
                size="small"
                :text="$t('conferences.detail.roomPending')"
              />
            </n-card>

            <n-card size="small" :title="$t('conferences.detail.participants')" class="people">
              <div v-if="detail.participants.length" class="plist">
                <div v-for="p in detail.participants" :key="p.user_id" class="person">
                  <div class="person-name">{{ p.user_name }}</div>
                  <div class="person-meta">
                    <n-tooltip>
                      <template #trigger>
                        <span class="role">{{ $t(`conferences.role.${p.role}`) }}</span>
                      </template>
                      {{ p.user_email }}
                    </n-tooltip>
                    <span class="dot">·</span>
                    <span :class="['presence', { live: p.joined_at && !p.left_at }]">
                      {{ presence(p) }}
                    </span>
                  </div>
                </div>
              </div>
              <div v-else class="none">{{ $t('conferences.detail.noParticipants') }}</div>
            </n-card>
          </div>
        </div>
      </n-spin>
    </template>

    <!-- SCHEDULE -->
    <!-- style, not a scoped rule: naive teleports the modal out of this
         component, where scoped CSS no longer reaches it. -->
    <n-modal
      v-model:show="dlg.show"
      preset="card"
      style="max-width: 460px"
      :bordered="false"
      :title="$t('conferences.create.title')"
    >
      <div class="form">
        <n-input
          v-model:value="dlg.title"
          :placeholder="$t('conferences.create.namePlaceholder')"
          data-testid="conference-title"
          @keyup.enter="submit"
        />
        <n-input
          v-model:value="dlg.description"
          type="textarea"
          :rows="3"
          :placeholder="$t('conferences.create.descriptionPlaceholder')"
        />
        <n-date-picker
          v-model:value="dlg.at"
          type="datetime"
          clearable
          :first-day-of-week="firstDayOfWeek"
          :format="dateTimePattern"
          :placeholder="$t('conferences.create.at')"
        />
        <div class="hint">{{ $t('conferences.create.atHint') }}</div>
      </div>
      <template #footer>
        <div class="foot">
          <n-button quaternary @click="dlg.show = false">
            {{ $t('conferences.create.cancel') }}
          </n-button>
          <n-button
            type="primary"
            :loading="dlg.saving"
            data-testid="conference-submit"
            @click="submit"
          >
            {{ $t('conferences.create.submit') }}
          </n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.conf {
  width: 100%;
}
.head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.h {
  margin: 0;
  font-size: 20px;
  color: var(--t-text1);
}
.sub {
  font-size: 12px;
  color: var(--t-text3);
}
.head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 8px;
  cursor: pointer;
}
.row:hover {
  background: var(--t-hover);
}
.row-body {
  flex: 1;
  min-width: 0;
}
.row-title {
  color: var(--t-text1);
  font-weight: 500;
}
.row-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 12px;
  color: var(--t-text3);
}
.meta-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
/* The status pill: flat interior, same-hue gradient on the border-box so it
   follows the corner radius, and the same hue again on the glyphs. */
.pill {
  flex: none;
  padding: 2px 10px;
  border: 1px solid transparent;
  border-radius: 999px;
  font-size: 11px;
  line-height: 18px;
  white-space: nowrap;
  color: var(--t-text3);
}
.back {
  margin-bottom: 12px;
}
.desc {
  margin: 0 0 16px;
  color: var(--t-text2);
  white-space: pre-wrap;
}
.panes {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 280px;
  gap: 12px;
  align-items: start;
}
@media (max-width: 760px) {
  .panes {
    grid-template-columns: minmax(0, 1fr);
  }
}
.plist {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.person-name {
  color: var(--t-text1);
}
.person-meta {
  display: flex;
  gap: 5px;
  font-size: 12px;
  color: var(--t-text3);
}
.presence.live {
  color: #18a058;
}
.none {
  font-size: 12px;
  color: var(--t-text3);
}
.form {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.hint {
  font-size: 12px;
  color: var(--t-text3);
}
.foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
