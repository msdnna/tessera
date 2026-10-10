<script setup>
// Conferences section (#2864, subtask #2870): the list of the workspace's calls,
// the «Запланировать» dialog and one conference's lobby.
//
// The route is `/conferences/:id?` — ONE record with an optional param, not two
// sibling records. vue-router marks a link active only when the open route
// shares its record, so as two records opening a conference would take the
// sidebar item dark (the same trap documents fell into, #2727).
//
// Media lives in ConferenceRoom (#2871) and talks to the LiveKit SFU directly.
// What stays here is the bookkeeping — who is invited, who is in the room and
// when the call happens — and the single source of truth for membership: this
// screen's join/leave drives the room's `active`, never the reverse.
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NIcon, NDatePicker, NPopconfirm, NSpin, NTooltip } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  VideocamOutline,
  ArrowBackOutline,
  PeopleOutline,
  TimeOutline,
  LogOutOutline,
  PowerOutline,
  FlashOutline,
  CalendarOutline,
  LinkOutline,
  EllipsisHorizontal,
  TrashOutline,
  RepeatOutline,
  TrailSignOutline,
  DocumentTextOutline,
  CheckmarkOutline,
  CloseOutline,
} from '@vicons/ionicons5'
import { conferences as confApi, documents as docsApi, workspaces as wsApi } from '@/api'
import { builtinCards, builtinContent } from '@/utils/docTemplates'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useNotificationsStore } from '@/stores/notifications'
import { useAuthStore } from '@/stores/auth'
import { useConferenceSession } from '@/stores/conference'
import { useFormat } from '@/composables/useFormat'
import { useRealtime } from '@/composables/useRealtime'
import { useShortcuts } from '@/composables/useShortcuts'
import { mediaSupported, UNAVAILABLE } from '@/composables/useConfTransport'
import { hueGrad, tagPillBg } from '@/utils/gradient'
import { canModerateConference } from '@/utils/confLobby'
import { groupConferences, nearestScheduled } from '@/utils/confList'
import { leaveCallView } from '@/utils/confNav'
import { stubTask } from '@/utils/featureStubs'
import { copyText } from '@/utils/clipboard'
import EmptyState from '@/components/EmptyState.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import FormField from '@/components/kit/FormField.vue'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import '@/components/kit/kit.css'
import ConferenceRoom from '@/components/conference/ConferenceRoom.vue'
import ConferenceRecordings from '@/components/conference/ConferenceRecordings.vue'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const ws = useWorkspacesStore()
const auth = useAuthStore()
const session = useConferenceSession()
const message = useMessage()
const { firstDayOfWeek, dateTimePattern, formatDateTime, formatTime, formatDate, formatters } =
  useFormat()

const list = ref([])
const loading = ref(false)
const filter = ref('all')

// The open conference, if the route carries an id: { conference, participants }.
const detail = ref(null)
const detailLoading = ref(false)
const detailMissing = ref(false)
const busy = ref(false)
// Bumped when the server says a recording started or finished (#2877); the
// recordings panel refetches on it. A counter rather than the payload: the list
// endpoint is the one place that knows the whole truth, and a panel patched from
// an event would drift from it after the first missed frame.
const recNudge = ref(0)

const openId = computed(() => route.params.id || '')

// ── list ───────────────────────────────────────────────────────────────

// Four things ask for the list — the mount, the status radio, the workspace
// switcher and the socket — and nothing held their answers apart, so whichever
// arrived LAST won, not whichever was asked last. Clicking «Живые» and then
// «Завершённые» while the first request is still out leaves the live calls on
// screen under the «Завершённые» radio, and nothing refetches until the reader
// touches the filter again; the same race across a workspace switch shows the
// previous workspace's calls. A busy call (participants joining) fires several
// `load()`s a second, so the reordering is not hypothetical.
//
// The counter is the same device SearchBar.vue uses for its debounced query, and
// the same comparison-after-await Android has always done (#2932).
let loadSeq = 0
let detailSeq = 0

async function load() {
  // Bumped before the early return, not after: a request already in flight has to
  // be invalidated even by a call that goes on to do nothing, or its answer lands
  // later and refills the list we are clearing.
  const mine = ++loadSeq
  if (!ws.currentId) {
    list.value = []
    loading.value = false
    return
  }
  loading.value = true
  try {
    const { data } = await confApi.list(ws.currentId, filter.value === 'all' ? '' : filter.value)
    if (mine !== loadSeq) return
    list.value = data || []
  } catch (e) {
    // A superseded request's failure is about a list nobody is looking at; its
    // toast would blame the filter the reader just switched to.
    if (mine !== loadSeq) return
    message.error(e.message)
  } finally {
    // Only the request that lit the spinner puts it out.
    if (mine === loadSeq) loading.value = false
  }
}

// ── agendas (#3046) ─────────────────────────────────────────────────────
// Documents pinned to a meeting — its agenda, its protocol — as a chip on the
// row. One read for the whole list (conference_id → documents), refreshed when
// the list's workspace changes and when a document moves on the socket.
const agendas = ref({})
let agendaSeq = 0
async function loadAgendas() {
  const mine = ++agendaSeq
  const wsId = ws.currentId
  if (!wsId) {
    agendas.value = {}
    return
  }
  try {
    const { data } = await docsApi.byConferences(wsId)
    if (mine !== agendaSeq) return
    const out = {}
    for (const d of data || []) (out[d.conference_id] ||= []).push(d)
    agendas.value = out
  } catch {
    // The chips are a convenience; the list stands without them.
  }
}
const agendaOf = (c) => agendas.value[c.id]?.[0] || null
function openAgenda(doc) {
  router.push(`/documents/${doc.slug || doc.id}`)
}
// «Создать повестку» from a row's «⋯»: the built-in agenda, pinned to the
// meeting and named after it — the same document the gallery would make.
async function createAgenda(c) {
  const card = builtinCards().find((x) => x.key === 'meeting')
  try {
    const { data: doc } = await docsApi.create(ws.currentId, {
      title: `${card.title}: ${c.title}`,
      icon: card.icon,
      conference_id: c.id,
    })
    const content = builtinContent('meeting')
    if (content) await docsApi.updateContent(doc.id, content, doc.updated_at)
    openAgenda(doc)
  } catch (e) {
    message.error(e.message)
  }
}

// ── one conference ─────────────────────────────────────────────────────
// The lobby needs the guard even more than the list does: on the way in it calls
// `ensureWorkspace` with the ANSWER's workspace, so a late reply about the call
// the reader has left does not merely redraw the lobby — it drags the whole app
// into that call's workspace, retargeting the tree, the board and the socket.
async function loadDetail(id) {
  const mine = ++detailSeq
  detailLoading.value = true
  detailMissing.value = false
  try {
    const { data } = await confApi.get(id)
    if (mine !== detailSeq) return
    detail.value = data
    // An invitation notification deep-links straight here, and the backend
    // authorises a call against ITS workspace — so the lobby can open while the
    // app is still pointed at the remembered one. Every scope decision on this
    // page reads currentId (the realtime filter below, the invite roster), so it
    // has to follow the call, exactly as BoardView follows a linked board (#2721).
    // Idempotent when we are already there.
    const confWs = data?.conference?.workspace_id
    if (confWs) await ws.ensureWorkspace(confWs)
    if (mine !== detailSeq) return
    // Prefetch the workspace roster so the room's invite popover is ready before
    // it is opened; skipped for an ended call, which cannot be invited to.
    if (canInvite.value) loadMembers(confWs)
    // Opened by «Войти» / ⇧J / «Мгновенная»: join now that the lobby is here.
    if (autoJoin === id) {
      autoJoin = ''
      if (!inRoom.value) join()
    }
  } catch (e) {
    if (mine !== detailSeq) return
    // A deleted (or foreign-workspace) conference answers 404 — say so on the
    // page instead of leaving the previous conference on screen under a new id.
    detail.value = null
    detailMissing.value = true
    // `e.status`, not `e.response.status` — see the note in ConferenceChat: the
    // api client rejects with an error of its own, and this check silently
    // matched nothing, so the page said "conference not found" *and* toasted it.
    if (e.status !== 404 && e.status !== 403) message.error(e.message)
  } finally {
    if (mine === detailSeq) detailLoading.value = false
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
    message.error(e.message)
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
    message.error(e.message)
  }
}

// ── invite ─────────────────────────────────────────────────────────────
// The invitation notification/push/deep-link is #2875; this is its trigger. Any
// workspace member may invite (as a member) — the backend only gates handing out
// a host seat — so the control is shown to everyone in the room, not just hosts.
//
// The picker is a popover in the room's rail now (#2891), like the assignee
// picker: the list of invitable members is fed to the room as a prop, one click
// invites that person immediately (no batch «Отправить»), and the parent's
// refetch drops them out of `invitable` and into the room's «приглашённые» list.
const members = ref([])

// Workspace members who are not currently in the call: those with no seat, plus
// anyone who joined and left (re-inviting them is a fresh call — mirrors the
// backend's «fresh» rule, #2875). An open, un-left invitation is skipped so we
// don't offer to ring someone who is already being rung.
const invitable = computed(() => {
  const held = new Set(
    (detail.value?.participants || []).filter((p) => !p.left_at).map((p) => p.user_id),
  )
  return members.value.filter((m) => !held.has(m.user_id))
})
const canInvite = computed(() => detail.value && detail.value.conference.status !== 'ended')

// Loaded once the lobby opens, so the popover's list is ready on the first click.
// Asked for the CALL's workspace, not the remembered one: the server lets only
// members of the call's workspace be invited, so a roster read off currentId
// would offer people it then refuses and hide the colleagues actually in reach.
// Silent on failure: the popover just shows an empty list until it succeeds —
// nothing the person opening a call needs a red toast about.
async function loadMembers(wsID) {
  const scope = wsID || ws.currentId
  if (!wsApi?.members || !scope) return
  try {
    const { data } = await wsApi.members(scope)
    members.value = data || []
  } catch {
    // non-fatal; invitable stays empty
  }
}

// One click = one invite (the room's popover has no batch select). The API still
// takes an array, so this sends a single-element one and keeps the signature.
async function inviteOne(userId) {
  if (!detail.value) return
  try {
    await confApi.invite(detail.value.conference.id, [userId])
    const { data: parts } = await confApi.participants(detail.value.conference.id)
    detail.value.participants = parts || []
    message.success(t('conferences.invite.sent', { count: 1 }))
  } catch (e) {
    message.error(e.message)
  }
}

// ── schedule dialog ────────────────────────────────────────────────────
// `ttl` is how many days a recording of this call is kept (#2877); 0 means keep
// it indefinitely. Defaulted to the server's own 30 rather than left null so the
// number in the box is the number that will apply — a blank field that silently
// becomes 30 is how people find out about a retention policy by losing a file.
const RECORDING_TTL_DEFAULT = 30
const scheduleBtn = ref(null)
const dlg = ref({
  show: false,
  saving: false,
  title: '',
  description: '',
  at: null,
  ttl: RECORDING_TTL_DEFAULT,
})

function openDialog() {
  dlg.value = {
    show: true,
    saving: false,
    title: '',
    description: '',
    at: null,
    ttl: RECORDING_TTL_DEFAULT,
  }
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
      // A cleared field (v-model.number answers '') falls back to the server's
      // own default rather than being sent as 0 — «хранить вечно» is a decision,
      // not something an empty box should make on the user's behalf.
      recording_ttl_days: Number.isFinite(dlg.value.ttl) ? dlg.value.ttl : RECORDING_TTL_DEFAULT,
    })
    dlg.value.show = false
    message.success(t('conferences.create.created'))
    await load()
    if (data?.id) open(data)
  } catch (e) {
    message.error(e.message)
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
// The server's rule, not a wider one: the creator, a host in the call, or an
// owner/admin of THIS workspace (ws.canManage reads the role the workspace list
// reports for the current one, which the detail loader has already pulled across
// to the call's workspace). See utils/confLobby.js.
const canModerate = computed(() =>
  canModerateConference(
    detail.value?.conference,
    detail.value?.participants,
    me.value,
    ws.canManage,
  ),
)
// Invited but not in the room yet — never joined, or an open invitation (#2875).
// The room's rail lists these so an invitation is visible somewhere again after
// the standalone participants column was dropped (#2881).
const invited = computed(() =>
  (detail.value?.participants || []).filter((p) => !p.joined_at && !p.left_at),
)

// The conference controls live in the app topbar (#2864 round 2), but only when
// that shell is present and wide enough; otherwise the teleports fall back inline.
const topbarSlots = ref(false)
const narrow = ref(false)
const inTopbar = computed(() => topbarSlots.value && !narrow.value)
// …and the teleports themselves are not rendered until this view is mounted
// (#2876). A <teleport> resolves its target ONCE, on its own mount, and keeps
// the result for the rest of its life. On a deep link into a room — the link an
// invitation sends — the view's first render happens while the shell is still
// being built, so `#tb-slot-left` is not in the document yet: the teleport binds
// to a null target, and the moment `inTopbar` flips true (onMounted, one tick
// later) Vue tries to move its children into that null and throws
// «Cannot read properties of null (reading 'insertBefore')». The patch dies
// mid-tree, and everything after it stops updating — visibly, the theme switch
// no longer repaints the layout. Mounting after onMounted means the target is
// looked up when it actually exists.
const ready = ref(false)
let mq = null
function onMq(e) {
  narrow.value = e.matches
}

// Neutral greys stay flat per the design language, so a finished call has no
// hue at all; the two states that still matter carry the same-hue gradient.
function statusHue(status) {
  if (status === 'live') return LIVE_HUE
  if (status === 'scheduled') return 'var(--t-primary)'
  return null
}
function pillStyle(status) {
  const hue = statusHue(status)
  return hue ? { background: tagPillBg(hue) } : { background: 'var(--t-pill)' }
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

// ── v2 list (#3107 — PackageE «Встречи») ───────────────────────────────
// A clock for «идёт · 24 мин» and for picking the nearest planned call.
const now = ref(Date.now())
let clock = null
onMounted(() => (clock = setInterval(() => (now.value = Date.now()), 30000)))
onBeforeUnmount(() => clearInterval(clock))

const FILTERS = ['all', 'live', 'scheduled', 'ended']
const grouped = computed(() =>
  groupConferences(list.value, formatters.value, { now: new Date(now.value), t }),
)
const nearest = computed(() => nearestScheduled(list.value, new Date(now.value)))

function runningFor(c) {
  const at = Date.parse(c.started_at || '')
  if (!at) return ''
  const mins = Math.max(0, Math.floor((now.value - at) / 60000))
  // A reusable room (#2879) can stay «live» for days; past a day the count says
  // nothing a reader could use.
  if (mins >= 24 * 60) return ''
  return mins < 60
    ? t('conferences.list.runningMin', { n: mins })
    : t('conferences.list.runningH', { h: Math.floor(mins / 60), m: mins % 60 })
}
// The time column of a row: the planned time, the day a finished one ended, a
// dash for a plan without a time.
function rowTime(c) {
  if (c.status === 'ended')
    return c.ended_at ? formatDate(c.ended_at, { day: 'numeric', month: 'short' }) : '—'
  if (c.scheduled_at) return formatTime(c.scheduled_at)
  return '—'
}

// The row's «⋯»: copy the link, delete (irreversible → a two-step item).
const menu = ref({ show: false, conf: null, anchor: null })
function openMenu(c, e) {
  menu.value = { show: true, conf: c, anchor: e.currentTarget }
}
// «Принять / Отклонить» (#3051) in the row's menu too — the way to change an
// answer once the row only shows it.
const rsvpItems = (c) =>
  c && c.my_rsvp && c.status !== 'ended'
    ? [
        c.my_rsvp !== 'accepted' && {
          key: 'accept',
          label: t('conferences.rsvp.acceptItem'),
          icon: CheckmarkOutline,
        },
        c.my_rsvp !== 'declined' && {
          key: 'decline',
          label: t('conferences.rsvp.declineItem'),
          icon: CloseOutline,
        },
      ].filter(Boolean)
    : []
const menuItems = computed(() => [
  { key: 'link', label: t('conferences.list.copyLink'), icon: LinkOutline },
  menu.value.conf && agendaOf(menu.value.conf)
    ? { key: 'agenda', label: t('conferences.agenda.open'), icon: DocumentTextOutline }
    : { key: 'agenda-new', label: t('conferences.agenda.create'), icon: DocumentTextOutline },
  ...rsvpItems(menu.value.conf),
  { gap: true },
  {
    key: 'delete',
    label: t('conferences.actions.delete'),
    icon: TrashOutline,
    danger: true,
    confirm: { title: t('conferences.list.confirmDelete'), sub: t('conferences.confirm.delete') },
  },
])
function onMenu(key) {
  const c = menu.value.conf
  menu.value.show = false
  if (!c) return
  if (key === 'link') copyLink(c)
  else if (key === 'agenda') openAgenda(agendaOf(c))
  else if (key === 'agenda-new') createAgenda(c)
  else if (key === 'accept') respond(c, 'accepted')
  else if (key === 'decline') respond(c, 'declined')
  else if (key === 'delete') remove(c)
}

// The reader's answer to an invitation (#3051), from a row, its menu or the
// lobby. Optimistic: the row shows it at once and a refusal puts it back. The
// server closes the invitation in the feed; the bell's copy follows here.
const notes = useNotificationsStore()
async function respond(c, response) {
  if (!c || c.my_rsvp === response) return
  const prev = c.my_rsvp
  c.my_rsvp = response
  const mine = detail.value?.participants?.find((p) => p.user_id === me.value)
  const prevMine = mine?.rsvp
  if (mine && detail.value?.conference?.id === c.id) mine.rsvp = response
  try {
    await confApi.rsvp(c.id, response)
    const now = new Date().toISOString()
    for (const n of notes.items)
      if (n.kind === 'conference_invite' && n.payload?.conference_id === c.id) {
        n.rsvp = response
        if (!n.read_at) n.read_at = now
      }
  } catch (e) {
    c.my_rsvp = prev
    if (mine) mine.rsvp = prevMine
    message.error(e.message)
  }
}
// The lobby's answer: the reader's own seat in the call, when there is one.
const myRsvp = computed(
  () => detail.value?.participants?.find((p) => p.user_id === me.value)?.rsvp || null,
)
function respondHere(response) {
  const c = detail.value?.conference
  if (!c) return
  const row = list.value.find((x) => x.id === c.id) || { ...c, my_rsvp: myRsvp.value }
  respond(row, response)
  const mine = detail.value.participants.find((p) => p.user_id === me.value)
  if (mine) mine.rsvp = response
}
async function copyLink(c) {
  const ok = await copyText(`${location.origin}/conferences/${c.id}`)
  if (ok) message.success(t('conferences.list.linkCopied'))
}

// Opening a call and joining it in one go — «Войти», ⇧J, «Мгновенная»: the
// lobby joins once its detail has loaded (see loadDetail).
let autoJoin = ''
function openAndJoin(c) {
  if (!c) return
  if (openId.value === c.id) {
    if (!inRoom.value) join()
    return
  }
  autoJoin = c.id
  open(c)
}
// ⇧J: the call that is on now, else the nearest planned one.
function joinNearest() {
  if (openId.value) return inRoom.value ? null : join()
  openAndJoin(grouped.value.live[0] || nearest.value)
}

// «Мгновенная» (N J): a call that starts now — created untimed and joined.
const instantBusy = ref(false)
async function instant() {
  if (!ws.currentId || instantBusy.value) return
  instantBusy.value = true
  try {
    const { data } = await confApi.create(ws.currentId, {
      title: t('conferences.instant.title', { time: formatTime(new Date()) }),
      description: '',
      scheduled_at: null,
      recording_ttl_days: RECORDING_TTL_DEFAULT,
    })
    await load()
    if (data?.id) openAndJoin(data)
  } catch (e) {
    message.error(e.message)
  } finally {
    instantBusy.value = false
  }
}

// The room's own controls are on screen — its dock carries «Выйти». Without
// media (no secure context) the dock is not drawn, and leaving stays up top.
const roomControls = computed(
  () => mediaSupported() && session.transport.status.value !== UNAVAILABLE,
)
const inCall = computed(() => !!detail.value && inRoom.value && roomControls.value)
function minimize() {
  if (detail.value) leaveCallView(router, detail.value.conference.id)
}

const recurrenceTask = stubTask('meet.recurrence')
const stageTask = stubTask('meet.stage')

useShortcuts({
  'meet.schedule': () => ws.currentId && !openId.value && openDialog(),
  'meet.instant': () => instant(),
  'meet.join': () => joinNearest(),
})

// ── wiring ─────────────────────────────────────────────────────────────
watch(openId, (id) => {
  if (id) loadDetail(id)
  else {
    // Leaving the lobby invalidates the fetch it started, or that answer lands on
    // the list and draws the lobby back over it.
    detailSeq++
    detail.value = null
    detailMissing.value = false
    detailLoading.value = false
  }
})

// Membership drives the shared session (#2888). Joining this call starts it,
// leaving stops it — and because the session lives in the store, navigating away
// from this view leaves the call running for the mini-window rather than dropping
// it. Only our own call is stopped here: opening another conference's lobby while
// in a call must not tear the call down.
watch(
  [inRoom, () => detail.value?.conference?.id, () => detail.value?.conference?.title],
  ([joined, id, title]) => {
    if (joined && id) session.start(id, title)
    else if (id && session.activeId === id) session.stop()
  },
  { immediate: true },
)
watch(filter, load)
watch(
  () => ws.currentId,
  (id) => {
    // Switching workspaces while a conference is open leaves that conference
    // outside the visible scope — go back to the list rather than 403. Unless we
    // switched INTO the open call's own workspace, which is loadDetail pulling
    // the scope after a deep link: that is the call arriving in scope, not
    // leaving it, and bouncing there would close the lobby we just opened.
    if (openId.value && detail.value?.conference?.workspace_id !== id) {
      router.push('/conferences')
    }
    load()
    loadAgendas()
  },
)

// Live updates: a colleague starting a call must appear in the list without a
// reload, since «идёт сейчас» is the whole reason to open this section.
useRealtime(
  (ev) => {
    // A document pinned to (or unpinned from) a meeting moves a row's chip.
    if (ev.scope === ws.currentId && ev.type?.startsWith('document')) loadAgendas()
    if (ev.scope !== ws.currentId || !ev.type?.startsWith('conference')) return
    // A recording starting or finishing changes one panel, not the page: the
    // conference itself is untouched, so refetching it (and the list behind it)
    // would repaint the whole screen every time an egress worker reports in.
    if (ev.type.startsWith('conference.recording')) {
      if (openId.value) recNudge.value += 1
      return
    }
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
    if (openId.value) {
      loadDetail(openId.value)
      // A gap in the stream may have swallowed a recording event; the panel has
      // to catch up with the rest of the page rather than stay at its last frame.
      recNudge.value += 1
    }
  },
)

onMounted(() => {
  // The topbar teleport targets exist only inside the app shell; detect them so
  // a bare mount (unit test) renders the controls inline instead of throwing.
  topbarSlots.value = !!document.getElementById('tb-slot-left')
  ready.value = true
  if (typeof window !== 'undefined' && window.matchMedia) {
    mq = window.matchMedia('(max-width: 900px)')
    narrow.value = mq.matches
    mq.addEventListener?.('change', onMq)
  }
  load()
  loadAgendas()
  if (openId.value) loadDetail(openId.value)
})
onBeforeUnmount(() => mq?.removeEventListener?.('change', onMq))
</script>

<template>
  <div class="conf" :class="{ 'in-call': inCall }">
    <!-- LIST -->
    <template v-if="!openId">
      <header class="head">
        <h1 class="h">{{ $t('conferences.title') }}</h1>
        <div class="seg" role="tablist" :aria-label="$t('conferences.hint')">
          <button
            v-for="f in FILTERS"
            :key="f"
            type="button"
            role="tab"
            class="seg-btn"
            :class="{ on: filter === f }"
            :aria-selected="filter === f"
            :data-testid="`conference-filter-${f}`"
            @click="filter = f"
          >
            {{ $t(`conferences.filter.${f}`) }}
          </button>
        </div>
        <span class="grow" />
        <n-tooltip :delay="400">
          <template #trigger>
            <button
              type="button"
              class="kit-btn"
              :disabled="!ws.currentId || instantBusy"
              data-testid="conference-instant"
              @click="instant"
            >
              <n-icon :component="FlashOutline" :size="15" />
              {{ $t('conferences.instant.button') }}
            </button>
          </template>
          <span class="tip">
            {{ $t('conferences.instant.hint') }} <ShortcutKeys id="meet.instant" bare />
          </span>
        </n-tooltip>
        <button
          ref="scheduleBtn"
          type="button"
          class="kit-btn soft"
          :disabled="!ws.currentId"
          data-testid="conference-schedule"
          @click="openDialog"
        >
          <n-icon :component="CalendarOutline" :size="15" />
          {{ $t('conferences.schedule') }}
          <ShortcutKeys id="meet.schedule" bare />
        </button>
      </header>

      <n-spin :show="loading">
        <div class="rows">
          <!-- What is on right now: a card each, on the soft accent. -->
          <div
            v-for="c in grouped.live"
            :key="c.id"
            class="live"
            data-testid="conference-row"
            @click="open(c)"
          >
            <span class="live-ic"><n-icon :component="VideocamOutline" :size="22" /></span>
            <div class="live-body">
              <div class="live-top">
                <span class="live-tag">
                  <span class="live-dot" />{{ $t('conferences.list.live') }}
                  <template v-if="runningFor(c)">· {{ runningFor(c) }}</template>
                </span>
              </div>
              <div class="live-title">{{ c.title }}</div>
              <div class="meta">
                <span>{{ timeLine(c) }}</span>
                <span>{{
                  $t('conferences.row.participantCount', { count: c.participant_count })
                }}</span>
                <span v-if="c.active_count">{{
                  $t('conferences.row.activeCount', { count: c.active_count })
                }}</span>
                <button
                  v-if="agendaOf(c)"
                  type="button"
                  class="agenda-chip"
                  :title="$t('conferences.agenda.open')"
                  data-testid="conference-agenda"
                  @click.stop="openAgenda(agendaOf(c))"
                >
                  <n-icon :component="DocumentTextOutline" :size="12" />
                  <span class="agenda-chip-t">{{ agendaOf(c).title }}</span>
                </button>
              </div>
            </div>
            <button
              type="button"
              class="kit-btn primary"
              data-testid="conference-row-join"
              @click.stop="openAndJoin(c)"
            >
              {{ $t('conferences.list.join') }}
              <ShortcutKeys v-if="c === grouped.live[0]" id="meet.join" joined />
            </button>
          </div>

          <template v-for="g in grouped.groups" :key="g.key">
            <div class="group">{{ g.label }}</div>
            <div
              v-for="c in g.rows"
              :key="c.id"
              class="row"
              :class="{ ended: c.status === 'ended' }"
              data-testid="conference-row"
              @click="open(c)"
            >
              <div class="when" :class="{ next: nearest && nearest.id === c.id }">
                <span class="when-t">{{ rowTime(c) }}</span>
              </div>
              <div class="row-body">
                <div class="row-title">
                  <span class="row-title-t">{{ c.title }}</span>
                  <span v-if="c.status !== 'scheduled'" class="pill" :style="pillStyle(c.status)">
                    <span
                      :class="{ 'accent-grad-text': !!statusHue(c.status) }"
                      :style="pillTextStyle(c.status)"
                      >{{ $t(`conferences.status.${c.status}`) }}</span
                    >
                  </span>
                </div>
                <div class="meta">
                  <span v-if="c.status === 'ended'">{{ timeLine(c) }}</span>
                  <span>
                    <n-icon :component="PeopleOutline" :size="12" />
                    {{ $t('conferences.row.participantCount', { count: c.participant_count }) }}
                  </span>
                  <span v-if="c.active_count">{{
                    $t('conferences.row.activeCount', { count: c.active_count })
                  }}</span>
                  <span>{{
                    c.created_by_name
                      ? $t('conferences.row.author', { name: c.created_by_name })
                      : $t('conferences.row.authorUnknown')
                  }}</span>
                  <button
                    v-if="agendaOf(c)"
                    type="button"
                    class="agenda-chip"
                    :title="$t('conferences.agenda.open')"
                    data-testid="conference-agenda"
                    @click.stop="openAgenda(agendaOf(c))"
                  >
                    <n-icon :component="DocumentTextOutline" :size="12" />
                    <span class="agenda-chip-t">{{ agendaOf(c).title }}</span>
                  </button>
                </div>
              </div>
              <!-- «Принять / Отклонить» an invitation still unanswered (#3051);
                   once answered the row says how, and its menu changes it. -->
              <span
                v-if="c.my_rsvp === 'pending' && c.status !== 'ended'"
                class="row-rsvp"
                data-testid="conference-row-rsvp"
              >
                <button
                  type="button"
                  class="kit-btn small primary"
                  data-testid="conference-row-accept"
                  @click.stop="respond(c, 'accepted')"
                >
                  {{ $t('conferences.rsvp.accept') }}
                </button>
                <button
                  type="button"
                  class="kit-btn small"
                  data-testid="conference-row-decline"
                  @click.stop="respond(c, 'declined')"
                >
                  {{ $t('conferences.rsvp.decline') }}
                </button>
              </span>
              <span
                v-else-if="
                  (c.my_rsvp === 'accepted' || c.my_rsvp === 'declined') &&
                  c.status !== 'ended' &&
                  !(c.created_by && c.created_by === me)
                "
                class="row-answer"
                :class="c.my_rsvp"
                data-testid="conference-row-answer"
                >{{ $t(`conferences.rsvp.${c.my_rsvp}`) }}</span
              >
              <button
                v-if="nearest && nearest.id === c.id"
                type="button"
                class="kit-btn primary small"
                data-testid="conference-row-join"
                @click.stop="openAndJoin(c)"
              >
                {{ $t('conferences.list.enter') }}
                <ShortcutKeys v-if="!grouped.live.length" id="meet.join" joined />
              </button>
              <button
                type="button"
                class="row-icon"
                :aria-label="$t('conferences.list.copyLink')"
                :title="$t('conferences.list.copyLink')"
                @click.stop="copyLink(c)"
              >
                <n-icon :component="LinkOutline" :size="15" />
              </button>
              <button
                type="button"
                class="row-icon"
                :aria-label="$t('conferences.list.more')"
                data-testid="conference-row-menu"
                @click.stop="openMenu(c, $event)"
              >
                <n-icon :component="EllipsisHorizontal" :size="15" />
              </button>
            </div>
          </template>

          <empty-state
            v-if="!loading && !list.length"
            :icon="VideocamOutline"
            :title="!ws.currentId ? '' : filter === 'all' ? $t('conferences.list.emptyTitle') : ''"
            :text="
              !ws.currentId
                ? $t('conferences.noWorkspace')
                : filter === 'all'
                  ? $t('conferences.empty')
                  : $t('conferences.emptyFiltered')
            "
          >
            <button
              v-if="ws.currentId && filter === 'all'"
              type="button"
              class="kit-btn primary"
              @click="openDialog"
            >
              {{ $t('conferences.schedule') }}<ShortcutKeys id="meet.schedule" bare />
            </button>
          </empty-state>
        </div>
      </n-spin>

      <ActionMenu
        v-model:show="menu.show"
        :anchor="() => menu.anchor"
        placement="bottom-end"
        :width="260"
        :items="menuItems"
        testid="conference-menu"
        @select="onMenu"
      />
    </template>

    <!-- ONE CONFERENCE -->
    <template v-else>
      <!-- The header names the call (v2): back, the title and its state ride
           the app topbar, left of the search; the status and «Завершить» on the
           right. Both fall back inline when the topbar slots are absent — a
           narrow screen, or a unit test with no shell (#2876, #2893). -->
      <div v-if="ready" class="call-bar" :class="{ inline: !inTopbar }">
        <teleport to="#tb-slot-left" :disabled="!inTopbar">
          <span class="crumb">
            <button
              type="button"
              class="crumb-back"
              :aria-label="$t('conferences.actions.back')"
              :title="$t('conferences.actions.back')"
              data-testid="conference-back"
              @click="backToList"
            >
              <n-icon :component="ArrowBackOutline" :size="15" />
            </button>
            <span v-if="detail" class="crumb-t">{{ detail.conference.title }}</span>
            <span v-if="detail && detail.conference.status === 'live'" class="crumb-s">
              · {{ $t('conferences.list.live') }}
              <template v-if="runningFor(detail.conference)">
                {{ runningFor(detail.conference) }}
              </template>
            </span>
          </span>
        </teleport>

        <teleport v-if="detail" to="#tb-slot-right" :disabled="!inTopbar">
          <span class="call-actions">
            <!-- In the call the crumb already says it is on. -->
            <span v-if="!inCall" class="pill" :style="pillStyle(detail.conference.status)">
              <span
                :class="{ 'accent-grad-text': !!statusHue(detail.conference.status) }"
                :style="pillTextStyle(detail.conference.status)"
              >
                {{ $t(`conferences.status.${detail.conference.status}`) }}
              </span>
            </span>
            <!-- Leaving lives on the call's dock. Without media (no secure
                 context) there is no dock, and the way out stays up here. -->
            <button
              v-if="inRoom && !roomControls"
              type="button"
              class="kit-btn small"
              :disabled="busy"
              :aria-label="$t('conferences.actions.leave')"
              data-testid="conference-leave"
              @click="leave"
            >
              <n-icon :component="LogOutOutline" :size="14" />
              <template v-if="!narrow">{{ $t('conferences.actions.leave') }}</template>
            </button>
            <!-- «Завершить» ends the session for everyone; only while it is
                 live. A power glyph on a narrow screen: the dock below already
                 has a round stop for the recording. -->
            <n-popconfirm
              v-if="canModerate && detail.conference.status === 'live'"
              :positive-text="$t('conferences.actions.end')"
              @positive-click="end"
            >
              <template #trigger>
                <button
                  type="button"
                  class="kit-btn small"
                  :aria-label="$t('conferences.actions.end')"
                  data-testid="conference-end"
                >
                  <n-icon :component="PowerOutline" :size="14" />
                  <template v-if="!narrow">{{ $t('conferences.actions.end') }}</template>
                </button>
              </template>
              {{ $t('conferences.confirm.end') }}
            </n-popconfirm>
          </span>
        </teleport>
      </div>

      <n-spin :show="detailLoading" class="detail-spin">
        <empty-state
          v-if="detailMissing"
          :icon="VideocamOutline"
          :text="$t('conferences.detail.notFound')"
        />
        <div v-else-if="detail" class="detail" :class="{ 'in-call': inCall }">
          <!-- The lobby: the call before you are in it. -->
          <section v-if="!inRoom" class="lobby" data-testid="conference-lobby">
            <span class="lobby-ic"><n-icon :component="VideocamOutline" :size="24" /></span>
            <div class="lobby-body">
              <h2 class="lobby-title">{{ detail.conference.title }}</h2>
              <div class="meta">
                <span>
                  <n-icon :component="TimeOutline" :size="12" />{{ timeLine(detail.conference) }}
                </span>
                <span>{{
                  $t('conferences.row.participantCount', {
                    count: detail.participants.length,
                  })
                }}</span>
                <span v-if="detail.conference.created_by_name">{{
                  $t('conferences.row.author', { name: detail.conference.created_by_name })
                }}</span>
              </div>
              <p v-if="detail.conference.description" class="desc">
                {{ detail.conference.description }}
              </p>
              <!-- The reader's answer to the invitation (#3051). -->
              <div
                v-if="myRsvp && detail.conference.status !== 'ended'"
                class="lobby-rsvp"
                data-testid="conference-rsvp"
                :data-rsvp="myRsvp"
              >
                <span class="lobby-rsvp-l">{{ $t('conferences.rsvp.title') }}</span>
                <button
                  type="button"
                  class="kit-btn small"
                  :class="{ chosen: myRsvp === 'accepted' }"
                  :aria-pressed="myRsvp === 'accepted'"
                  data-testid="conference-rsvp-accept"
                  @click="respondHere('accepted')"
                >
                  <n-icon v-if="myRsvp === 'accepted'" :component="CheckmarkOutline" :size="13" />
                  {{ $t('conferences.rsvp.accepted') }}
                </button>
                <button
                  type="button"
                  class="kit-btn small"
                  :class="{ chosen: myRsvp === 'declined' }"
                  :aria-pressed="myRsvp === 'declined'"
                  data-testid="conference-rsvp-decline"
                  @click="respondHere('declined')"
                >
                  {{ $t('conferences.rsvp.declined') }}
                </button>
              </div>
            </div>
            <!-- Always enabled: a conference is a reusable room (#2879), so even
                 a legacy "ended" one is joined rather than being a dead end. -->
            <button
              type="button"
              class="kit-btn primary big"
              :disabled="busy"
              data-testid="conference-join"
              @click="join"
            >
              <n-icon :component="VideocamOutline" :size="16" />
              {{ $t('conferences.actions.join') }}
              <ShortcutKeys id="meet.join" joined />
            </button>
          </section>

          <!-- The room once you are in it — or, without media, the reason why
               there will be none (the lobby says the rest). -->
          <div v-if="inRoom || !roomControls" class="room">
            <conference-room
              :conference-id="detail.conference.id"
              :active="inRoom"
              :ended="detail.conference.status === 'ended'"
              :can-invite="canInvite"
              :invitable="invitable"
              :invited="invited"
              @hangup="leave"
              @invite="inviteOne"
              @minimize="minimize"
            />
          </div>

          <!-- Recordings of this call (#2877): what is left of past meetings,
               below the lobby. Renders nothing until there is something to list. -->
          <conference-recordings
            v-if="!inCall"
            :conference-id="detail.conference.id"
            :can-moderate="canModerate"
            :nudge="recNudge"
          />
        </div>
      </n-spin>
    </template>

    <!-- SCHEDULE: a popover form at «Запланировать» (v2 interaction model,
         #2969 / #2983) — it replaced the modal. -->
    <PopoverForm
      v-model:show="dlg.show"
      :anchor="() => scheduleBtn || null"
      placement="bottom-end"
      :offset="{ x: 0, y: 6 }"
      :width="400"
      :title="$t('conferences.create.title')"
      :subtitle="$t('conferences.create.subtitle')"
      :primary="$t('conferences.create.submit')"
      :busy-label="$t('common.form.saving')"
      :busy="dlg.saving"
      :disabled="!dlg.title.trim()"
      hint="tab"
      testid="conference"
      @submit="submit"
    >
      <FormField :label="$t('conferences.create.name')">
        <input
          v-model="dlg.title"
          class="kf-input"
          :placeholder="$t('conferences.create.namePlaceholder')"
          data-testid="conference-title"
        />
      </FormField>
      <FormField :label="$t('conferences.create.at')" :message="$t('conferences.create.atHint')">
        <n-date-picker
          v-model:value="dlg.at"
          type="datetime"
          clearable
          :first-day-of-week="firstDayOfWeek"
          :format="dateTimePattern"
          :placeholder="$t('conferences.create.at')"
        />
      </FormField>
      <FormField>
        <textarea
          v-model="dlg.description"
          class="kf-input kf-area"
          rows="3"
          :placeholder="$t('conferences.create.descriptionPlaceholder')"
        />
      </FormField>
      <!-- Retention for this call's recordings (#2877): a property of the
           meeting, decided before a recording exists. -->
      <FormField :label="$t('conferences.create.ttl')" :message="$t('conferences.create.ttlHint')">
        <input
          v-model.number="dlg.ttl"
          class="kf-input"
          type="number"
          min="0"
          max="3650"
          data-testid="conference-ttl"
        />
      </FormField>
      <!-- A stage and a repeat are #3053 / #3052: visible, inactive. -->
      <div class="form-stubs">
        <n-tooltip :delay="300">
          <template #trigger>
            <span class="form-stub" data-testid="conference-stage-stub">
              <n-icon :component="TrailSignOutline" :size="13" />
              {{ $t('conferences.create.stage') }}
            </span>
          </template>
          {{ $t('common.stub.soon', { n: stageTask }) }}
        </n-tooltip>
        <n-tooltip :delay="300">
          <template #trigger>
            <span class="form-stub" data-testid="conference-repeat-stub">
              <n-icon :component="RepeatOutline" :size="13" />
              {{ $t('conferences.create.repeat') }}
            </span>
          </template>
          {{ $t('common.stub.soon', { n: recurrenceTask }) }}
        </n-tooltip>
      </div>
    </PopoverForm>
  </div>
</template>

<style scoped>
/* v2 (#3107 — PackageE «Встречи»): no frames — the live call is a soft-accent
   card, rows are --t-col surfaces with a time column, days are group labels. */
.conf {
  width: 100%;
  max-width: 1080px;
  margin: 0 auto;
}
.conf.in-call {
  max-width: none;
  /* The call fills the work area: the viewport less the 52px topbar and the
     page's padding. */
  height: calc(100vh - 52px - 2 * var(--page-pad, 16px));
}
@media (max-width: 900px) {
  .conf.in-call {
    height: auto;
  }
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 18px;
}
.head > * {
  flex: none;
}
/* The filter gives way first when the row is tight — it scrolls, the buttons
   keep their labels. */
.head > .seg {
  flex: 0 1 auto;
  min-width: 0;
}
.tip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.h {
  margin: 0;
  font-size: 24px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--t-text1);
}
.grow {
  flex: 1;
}
.seg {
  display: inline-flex;
  gap: 2px;
  padding: 3px;
  border-radius: 10px;
  background: var(--t-sub);
  max-width: 100%;
  overflow-x: auto;
  scrollbar-width: none;
}
.seg::-webkit-scrollbar {
  display: none;
}
.seg-btn {
  flex: none;
  height: 28px;
  padding: 0 9px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
}
.seg-btn:hover {
  color: var(--t-text1);
}
.seg-btn.on {
  background: var(--t-pop);
  color: var(--t-text1);
  font-weight: 600;
  box-shadow: var(--t-sh-card);
}
.seg-btn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
/* «Запланировать»: the soft accent of the mockup's header button. */
.kit-btn.soft {
  background: color-mix(in srgb, var(--t-primary) 18%, var(--t-ctl));
  color: var(--t-text1);
  font-weight: 600;
}
.kit-btn.soft:hover {
  background: color-mix(in srgb, var(--t-primary) 26%, var(--t-ctl));
}
.kit-btn.big {
  height: 40px;
  padding: 0 8px 0 14px;
  font-size: 14px;
}
.rows {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-height: 120px;
}
/* The call on now: a soft-accent card. */
.live {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 14px;
  padding: 16px 18px;
  border-radius: 16px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 14%, var(--t-col)),
    color-mix(in srgb, var(--t-primary) 22%, var(--t-col))
  );
  cursor: pointer;
}
.live-ic {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 14px;
  background: var(--t-accent-grad);
  color: #fff;
}
.live-body {
  flex: 1;
  min-width: 0;
}
.live-tag {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  font-weight: 700;
  color: #e0533d;
}
.live-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #e0533d;
  box-shadow: 0 0 0 3px color-mix(in srgb, #e0533d 25%, transparent);
}
.live-title {
  margin: 3px 0 2px;
  font-size: 17px;
  font-weight: 700;
  color: var(--t-text1);
}
.group {
  margin: 14px 4px 6px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.row {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 58px;
  padding: 8px 10px 8px 14px;
  box-sizing: border-box;
  border-radius: 12px;
  background: var(--t-col);
  cursor: pointer;
}
.row:hover {
  background: color-mix(in srgb, var(--t-col) 70%, var(--t-hover));
}
.row.ended .row-title-t {
  color: var(--t-text2);
}
.when {
  flex: none;
  width: 64px;
}
.when-t {
  font-size: 15px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: var(--t-text1);
}
.row.ended .when-t {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text3);
}
.when.next .when-t {
  color: var(--t-primary);
}
.row-body {
  flex: 1;
  min-width: 0;
}
.row-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.row-title-t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  font-weight: 650;
  color: var(--t-text1);
}
.meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 12px;
  margin-top: 2px;
  font-size: 12px;
  color: var(--t-text3);
}
.meta > span {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
/* The meeting's agenda (задача 3046): a pill in the meta line, a door to it. */
.agenda-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: 260px;
  height: 20px;
  padding: 0 8px 0 6px;
  border: 0;
  border-radius: var(--t-r-pill);
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.agenda-chip:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.agenda-chip:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px var(--t-primary);
}
.agenda-chip-t {
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
/* «Принять / Отклонить» (#3051): the unanswered invitation's two buttons, and
   once answered a quiet word — accented for «иду», muted for «не иду». */
.row-rsvp {
  flex: none;
  display: inline-flex;
  gap: 6px;
}
.row-answer {
  flex: none;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
}
.row-answer.accepted {
  color: var(--t-primary);
}
.lobby-rsvp {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 12px;
}
.lobby-rsvp-l {
  font-size: 12.5px;
  color: var(--t-text3);
  margin-right: 4px;
}
.lobby-rsvp .kit-btn.chosen {
  background: color-mix(in srgb, var(--t-primary) 14%, var(--t-card));
  color: var(--t-primary);
  font-weight: 600;
}
.row-icon {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.row-icon:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.row-icon:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
/* The status pill: flat interior, same-hue gradient on the border-box. */
.pill {
  flex: none;
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  border: 1px solid transparent;
  border-radius: 7px;
  font-size: 11px;
  white-space: nowrap;
  color: var(--t-text3);
}
.call-actions .pill {
  height: 28px;
  padding: 0 10px;
  border-radius: 8px;
}

/* ── one conference ── */
.crumb {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-size: 13.5px;
}
.crumb-back {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.crumb-back:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.crumb-t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 700;
  color: var(--t-text1);
}
.crumb-s {
  flex: none;
  color: var(--t-text3);
}
.call-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
/* The wrapper around both teleports: no box while the controls are in the
   topbar, one row when they fall back inline. */
.call-bar {
  display: contents;
}
.call-bar.inline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
/* Inline, the name gives way — the controls on the right keep their place. */
.call-bar.inline .crumb {
  flex: 1;
}
.call-bar.inline .call-actions {
  flex: none;
}
.detail-spin,
.detail-spin :deep(.n-spin-content) {
  height: 100%;
}
.detail.in-call {
  height: 100%;
}
.detail.in-call .room {
  height: 100%;
}
/* The lobby card. */
.lobby {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 14px;
  padding: 20px;
  border-radius: 18px;
  background: var(--t-col);
}
.lobby-ic {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  border-radius: 16px;
  background: var(--t-accent-grad);
  color: #fff;
}
.lobby-body {
  flex: 1;
  min-width: 0;
}
.lobby-title {
  margin: 2px 0 4px;
  font-size: 20px;
  font-weight: 700;
  color: var(--t-text1);
}
.desc {
  margin: 10px 0 0;
  font-size: 13.5px;
  line-height: 1.55;
  color: var(--t-text2);
  white-space: pre-wrap;
}
.form-stubs {
  display: flex;
  gap: 14px;
  padding-top: 2px;
}
.form-stub {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12.5px;
  color: var(--t-text3);
  opacity: 0.75;
  cursor: default;
}
/* No keyboard on a touch screen — no key caps either. */
@media (hover: none) and (pointer: coarse) {
  .conf :deep(.sk) {
    display: none;
  }
}
@media (max-width: 760px) {
  .head {
    flex-wrap: wrap;
  }
  .h {
    font-size: 20px;
    flex: 1 0 100%;
  }
  .head .grow {
    display: none;
  }
  .head > .kit-btn,
  .head > :deep(.kit-btn) {
    flex: 1;
  }
  .live {
    flex-wrap: wrap;
  }
  .live > .kit-btn {
    flex: 1 0 100%;
  }
  .row-icon:not([data-testid]) {
    display: none;
  }
  .when {
    width: 52px;
  }
  .lobby {
    flex-direction: column;
  }
  .lobby .kit-btn.big {
    align-self: stretch;
  }
}
</style>
