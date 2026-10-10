<script setup>
// The two toast lanes (v2, #3117 — PackageI, mockup ToastLayer). Mounted once
// in AppLayout.
//
//   top    — the right edge of the content, 16px in, 8px under the top bar,
//            360 wide, newest on top: «про меня», reminders, calls, the open
//            board's activity, the system, the «Пока вас не было» digest. Up
//            to three; three lie as a deck (96% · 92%) that a hover opens;
//            past that «Ещё N · Показать в ленте». An open DockPanel moves the
//            lane left by its width; the ⌘K palette hides it.
//   bottom — centred over the work area, 24px up, newest at the bottom: the
//            answers to my actions (stores/toasts `respond`) above the undo
//            toasts (stores/undo — package A's rules, unchanged).
//
// A phone (≤768px) merges both into one lane over the bottom navigation: one
// toast at a time, the rest wait («1 из 3»); a swipe down or aside past 96px
// closes, a tap is the main action, the FAB rides up over it.
//
// Keys: F6 — into the nearest lane, again — the other one, again — back where
// you were; ↑↓ walk the lane; ↵ the main action; R reply; Z · E snooze · done;
// Esc closes, ⇧Esc closes the lane. A toast never takes the focus by itself.
// Two live regions read each toast out as one sentence: polite, and assertive
// for errors.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import { ChevronForwardOutline, CloseCircleOutline } from '@vicons/ionicons5'
import Toast from './Toast.vue'
import UndoToast from '@/components/kit/UndoToast.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import SnoozeMenu from '@/components/inbox/SnoozeMenu.vue'
import { useToastsStore } from '@/stores/toasts'
import { useUndoStore, UNDO_MS } from '@/stores/undo'
import { useNotificationsStore } from '@/stores/notifications'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useInboxStore } from '@/stores/inbox'
import { usePaletteStore } from '@/stores/palette'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useShortcuts } from '@/composables/useShortcuts'
import { useResponsive } from '@/composables/useResponsive'
import { useVisualViewport } from '@/composables/useVisualViewport'
import { useInboxActions } from '@/composables/useInboxActions'
import { notificationRoute } from '@/utils/notificationRoute'
import { boards as boardsApi, projects as projApi } from '@/api'
import { humanizeError } from '@/utils/errors'
import { useMessage } from '@/composables/useMessage'
import { phoneOrder } from '@/utils/toastModel'
import { copyText } from '@/utils/clipboard'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const store = useToastsStore()
const undo = useUndoStore()
const notes = useNotificationsStore()
const ws = useWorkspacesStore()
const inbox = useInboxStore()
const palette = usePaletteStore()
const dialogs = useWsDialogsStore()
const inboxAct = useInboxActions()
const { isMobile } = useResponsive()
const { keyboardOpen } = useVisualViewport()
const message = useMessage()

// ── what the buttons do ──────────────────────────────────────────────────
function openNotification(n) {
  notes.markRead(n.id)
  if (n.workspace_id && n.workspace_id !== ws.currentId) ws.selectWorkspace(n.workspace_id)
  const to = notificationRoute(n)
  if (to) router.push(to)
}
// R: the task opens with the focus in its composer. The modal mounts after the
// route settles; wait for the field rather than reach into the modal.
function focusWhenReady(selector, ms = 4000) {
  const until = Date.now() + ms
  const tryIt = () => {
    const el = document.querySelector(selector)
    if (el) return el.focus()
    if (Date.now() < until) setTimeout(tryIt, 120)
  }
  setTimeout(tryIt, 120)
}
function reminderRecord(n) {
  const id = n.payload?.reminder_id
  return (
    inbox.reminders.find((r) => r.id === id) || {
      id,
      kind: 'reminder',
      done: false,
      message: n.payload?.message || '',
      remind_at: n.created_at,
    }
  )
}
const snooze = ref(null) // { toastId, el, rec }

// «В корзину» on the toast about a deleted task (#3120): the board the task
// stood on, with its trash panel open and the task's row lit. The notification
// carries the board (msgDeleted's payload) but no slugs — the board and its
// project are read for them.
async function openTrash(n) {
  notes.markRead(n.id)
  const boardId = n.payload?.board_id
  if (!boardId) return
  try {
    if (n.workspace_id && n.workspace_id !== ws.currentId) await ws.selectWorkspace(n.workspace_id)
    const { data: board } = await boardsApi.get(boardId)
    const project = ws.projectById(board.project_id) || (await projApi.get(board.project_id)).data
    const number = n.payload?.task_number ?? null
    router.push({
      path: `/project/${project.slug}/board/${board.slug}`,
      query: { trash: '1', ...(number != null ? { hl: String(number) } : {}) },
    })
  } catch (e) {
    message.error(humanizeError(e))
  }
}

store.bind({
  open: openNotification,
  reply(n) {
    openNotification(n)
    focusWhenReady('.tm-composer textarea')
  },
  joinCall: openNotification,
  trash: openTrash,
  snooze(n, el) {
    const x = store.top.find((y) => y.notificationId === n.id)
    if (!n.payload?.reminder_id) return
    snooze.value = { toastId: x?.id, el, rec: reminderRecord(n) }
  },
  async done(n) {
    if (!n.payload?.reminder_id) return
    await inboxAct.complete(reminderRecord(n))
    notes.markRead(n.id)
  },
  openGitlab: () => dialogs.open('gitlab'),
  openFeed: () => notes.requestFeed(),
  // An activity toast's ↵: the task on the board in front of you.
  openTask(a) {
    if (a.number != null) router.replace({ query: { ...route.query, task: a.number } })
  },
})
async function onSnoozePick(at) {
  const s = snooze.value
  snooze.value = null
  if (!s) return
  await inboxAct.snooze(s.rec, at)
  if (s.toastId) store.remove(s.toastId)
}

// ── lanes ────────────────────────────────────────────────────────────────
const top = computed(() => store.top)
const bottom = computed(() => store.bottom)
const undos = computed(() => undo.toasts)
// The deck: three and more lie under the newest until hovered or focused.
const topOpen = computed(() => store.hover.top || store.focus.top)
const deck = computed(() => top.value.length >= 3 && !topOpen.value)
const topShown = computed(() => (deck.value ? top.value.slice(0, 1) : top.value))
const topHidden = computed(() => store.holds.palette)

// An open DockPanel («Входящие», the board's customisation) owns the right
// edge: the lane moves left of it.
const dockRight = ref(16)
function measureDock() {
  let right = 16
  for (const el of document.querySelectorAll('[data-dock-panel]')) {
    const r = el.getBoundingClientRect()
    if (r.width > 0 && r.top < 120) right = Math.max(right, window.innerWidth - r.left + 12)
  }
  dockRight.value = right
}

// ── the clock ────────────────────────────────────────────────────────────
const TICK = 100
let timer = null
const anything = computed(
  () => top.value.length + bottom.value.length + undos.value.length + store.held.length > 0,
)
watch(
  anything,
  (any) => {
    if (any && !timer) timer = setInterval(onTick, TICK)
    if (!any && timer) {
      clearInterval(timer)
      timer = null
      store.now = Date.now()
    }
  },
  { immediate: true },
)
// «Не беспокоить» ends on its own; the moon must notice without a toast.
const minute = setInterval(() => (store.now = Date.now()), 30000)
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
  clearInterval(minute)
})

function onTick() {
  if (phone.value) {
    const cur = phoneCurrent.value
    const pausedNow = touching.value || store.hiddenTab
    store.tick(TICK, { only: cur && cur.lane !== 'undo' ? cur.id : -1, paused: pausedNow })
    if (cur?.lane === 'undo' && !pausedNow) undo.tick(TICK, cur.id)
    else undo.tick(0)
  } else {
    store.tick(TICK)
    undo.tick(store.hiddenTab ? 0 : TICK)
  }
  if (top.value.length) measureDock()
}

// ── holds ────────────────────────────────────────────────────────────────
function onVisibility() {
  store.setHiddenTab(document.visibilityState === 'hidden')
}
onMounted(() => {
  document.addEventListener('visibilitychange', onVisibility)
  window.addEventListener('keydown', onAnyKey, true)
  window.addEventListener('pointermove', onAnyPointer, { passive: true })
  window.addEventListener('pointerdown', onAnyPointer, { passive: true })
  store.loadPrefs()
  measureDock()
})
onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', onVisibility)
  window.removeEventListener('keydown', onAnyKey, true)
  window.removeEventListener('pointermove', onAnyPointer)
  window.removeEventListener('pointerdown', onAnyPointer)
})
watch(
  () => palette.open,
  (v) => store.setHold('palette', v),
  { immediate: true },
)
watch(keyboardOpen, (v) => store.setHold('keyboard', v), { immediate: true })
watch(isMobile, (v) => (store.phone = v), { immediate: true })
watch(
  () => route.path,
  (p) => (store.callScreen = /^\/conferences\/./.test(p)),
  { immediate: true },
)

// The key caps show while the keyboard drives; a pointer hides them.
const kbd = ref(true)
function onAnyKey(e) {
  if (!['Shift', 'Control', 'Alt', 'Meta'].includes(e.key)) kbd.value = true
}
function onAnyPointer() {
  if (kbd.value) kbd.value = false
}

const reduced = ref(
  typeof window !== 'undefined' &&
    !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches,
)

// ── live regions ─────────────────────────────────────────────────────────
const politeText = ref('')
const alertText = ref('')
const said = new Set()
const val = (v) => (typeof v === 'function' ? v() : v) || ''
function speech(x) {
  if (x.speech) return val(x.speech)
  const parts = [val(x.bold), val(x.rest)]
  if (x.batch) parts.push(String(x.count), x.tail(x.count))
  if (x.chip?.title) parts.push(`${x.chip.ref != null ? x.chip.ref + ' ' : ''}«${x.chip.title}»`)
  return parts.filter(Boolean).join(' ')
}
watch(
  () => [...top.value, ...bottom.value].map((x) => x.id),
  async () => {
    for (const x of [...top.value, ...bottom.value]) {
      if (said.has(x.id)) continue
      said.add(x.id)
      const text = speech(x)
      if (x.live === 'assertive') {
        alertText.value = ''
        await nextTick()
        alertText.value = text
      } else {
        politeText.value = ''
        await nextTick()
        politeText.value = text
      }
    }
  },
)

// ── keys ─────────────────────────────────────────────────────────────────
const topEl = ref(null)
const bottomEl = ref(null)
const phoneEl = ref(null)
const focusedId = ref(null)
let returnTo = null

function laneEls() {
  if (phone.value) return phoneEl.value && phoneItems().length ? [phoneEl.value] : []
  const out = []
  if (topEl.value && top.value.length && !topHidden.value) out.push(topEl.value)
  if (bottomEl.value && (bottom.value.length || undos.value.length)) out.push(bottomEl.value)
  return out
}
function items(lane) {
  return lane ? [...lane.querySelectorAll('.tl-item')] : []
}
function phoneItems() {
  return phoneEl.value ? items(phoneEl.value) : []
}
function focusLane(lane) {
  const first = items(lane)[0]
  if (!first) return false
  first.focus()
  return true
}
function laneOf(el) {
  return laneEls().find((l) => l.contains(el)) || null
}
function backToApp() {
  const to = returnTo
  returnTo = null
  if (to && document.contains(to)) to.focus?.()
  else document.activeElement?.blur?.()
}
function onF6() {
  const lanes = laneEls()
  if (!lanes.length) return false
  const here = laneOf(document.activeElement)
  if (!here) {
    returnTo = document.activeElement
    return focusLane(lanes[0])
  }
  const i = lanes.indexOf(here)
  if (i < lanes.length - 1) return focusLane(lanes[i + 1])
  backToApp()
  return true
}
useShortcuts({
  'undo.last': () => undo.undoLast(),
  'toast.focus': () => onF6(),
})

// Which toast a lane item stands for: ['toast' | 'undo', id].
function itemRef(el) {
  return [el.dataset.kind, Number(el.dataset.id)]
}
function onLaneKey(e, laneName) {
  const lane = e.currentTarget
  const el = e.target.closest?.('.tl-item')
  if (!el || !lane.contains(el)) return
  const list = items(lane)
  const i = list.indexOf(el)
  const [kind, id] = itemRef(el)
  const k = e.key
  let handled = true
  if (k === 'ArrowDown' || k === 'ArrowUp') {
    const next = list[(i + (k === 'ArrowDown' ? 1 : -1) + list.length) % list.length]
    next?.focus()
  } else if (k === 'Escape' && e.shiftKey) {
    if (laneName === 'bottom') {
      store.dismissLane('bottom')
      for (const u of [...undos.value]) undo.dismiss(u.id)
    } else if (laneName === 'phone') {
      store.dismissLane('top')
      store.dismissLane('bottom')
      for (const u of [...undos.value]) undo.dismiss(u.id)
    } else store.dismissLane('top')
    backToApp()
  } else if (k === 'Escape') {
    const next = list[i + 1] || list[i - 1] || null
    if (kind === 'undo') undo.dismiss(id)
    else store.dismiss(id)
    nextTick(() => {
      const again = next && document.contains(next) ? next : items(lane)[0]
      if (again) again.focus()
      else backToApp()
    })
  } else if (k === 'Enter') {
    if (e.target !== el) return // a button handles its own Enter
    if (kind === 'undo') undo.undo(id)
    else runPrimary(id)
  } else if (!e.metaKey && !e.ctrlKey && !e.altKey && /^[rze]$/i.test(k) && kind === 'toast') {
    const x = store.find(id)
    const a = x?.actions.find((b) => b.key === k.toLowerCase())
    if (!a) return
    store.run(id, a.id, el.querySelector(`[data-testid="toast-action-${a.id}"]`) || el)
  } else handled = false
  if (handled) {
    e.preventDefault()
    e.stopPropagation()
  }
}
function runPrimary(id) {
  const x = store.find(id)
  const a = x?.actions.find((b) => b.primary) || x?.actions[0]
  if (!a) return
  const el = document.querySelector(`[data-toast="${id}"] [data-testid="toast-action-${a.id}"]`)
  store.run(id, a.id, el)
}
function onFocusIn(lane, e) {
  store.focus[lane] = true
  const el = e.target.closest?.('.tl-item')
  focusedId.value = el ? el.dataset.kind + el.dataset.id : null
}
function onFocusOut(lane, e) {
  if (e.currentTarget.contains(e.relatedTarget)) return
  store.focus[lane] = false
  focusedId.value = null
}

function onRun(x, actionId, el) {
  const a = x.actions.find((b) => b.id === actionId)
  store.run(x.id, actionId, el)
  if (a?.id === 'open' || a?.id === 'join') nextTick(() => document.activeElement?.blur?.())
}
async function onCopy(x) {
  await copyText((x.details || []).join('\n'))
  store.toggleDetails(x.id)
}

function openMore() {
  store.more = 0
  store.moreFaces = []
  notes.requestFeed()
}

// ── undo toasts (package A, unchanged) ──────────────────────────────────
function undoLabel(text) {
  if (!text) return ''
  if (typeof text === 'function') return text()
  if (typeof text === 'string') return text
  return t(text.key, text.params || {})
}
function undoCaption(u) {
  if (u.status === 'done') return undoLabel(u.doneText) || t('common.undo.done')
  if (u.status === 'error') return undoLabel(u.errorText) || t('common.undo.failed')
  return undoLabel(u.text)
}

// ── the phone ────────────────────────────────────────────────────────────
const phone = computed(() => isMobile.value)
const phoneQueue = computed(() =>
  phoneOrder([
    ...top.value.map((x) => ({ ...x, src: x })),
    ...bottom.value.map((x) => ({ ...x, src: x })),
    ...undos.value.map((u) => ({ id: u.id, lane: 'undo', at: u.id, src: u })),
  ]),
)
const phoneCurrent = computed(() => phoneQueue.value[0] || null)
const queueN = computed(() => phoneQueue.value.length)
const touching = ref(false)
const drag = ref({ x: 0, y: 0, active: false })
let start = null
function onDown(e) {
  if (e.target.closest('button')) return
  touching.value = true
  start = { x: e.clientX, y: e.clientY, id: e.pointerId }
  drag.value = { x: 0, y: 0, active: false }
  e.currentTarget.setPointerCapture?.(e.pointerId)
}
function onMove(e) {
  if (!start || e.pointerId !== start.id) return
  const dx = e.clientX - start.x
  const dy = Math.max(0, e.clientY - start.y)
  drag.value = { x: dx, y: dy, active: Math.abs(dx) > 6 || dy > 6 }
}
function onUp(e) {
  if (!start || e.pointerId !== start.id) return
  touching.value = false
  start = null
  const { x, y, active } = drag.value
  const cur = phoneCurrent.value
  drag.value = { x: 0, y: 0, active: false }
  if (!cur) return
  if (Math.abs(x) > 96 || y > 96) {
    if (cur.lane === 'undo') undo.dismiss(cur.id)
    else store.dismiss(cur.id)
    return
  }
  if (!active && cur.lane !== 'undo') runPrimary(cur.id)
}
function onCancel() {
  touching.value = false
  start = null
  drag.value = { x: 0, y: 0, active: false }
}
const swipeStyle = computed(() => {
  const { x, y, active } = drag.value
  if (!active) return {}
  const far = Math.min(1, Math.max(Math.abs(x), y) / 160)
  return { transform: `translate(${x}px, ${y}px)`, opacity: 1 - far * 0.5, transition: 'none' }
})
// The FAB rides up over the toast: --toast-lift is the lane's height.
const phoneBox = ref(null)
let ro = null
watchEffect(() => {
  const root = document.documentElement.style
  if (!phone.value || !phoneCurrent.value) root.setProperty('--toast-lift', '0px')
})
watch(phoneBox, (el) => {
  ro?.disconnect()
  if (!el || typeof ResizeObserver === 'undefined') return
  ro = new ResizeObserver(() => {
    if (phone.value && phoneCurrent.value)
      document.documentElement.style.setProperty('--toast-lift', `${el.offsetHeight + 12}px`)
  })
  ro.observe(el)
})
onBeforeUnmount(() => {
  ro?.disconnect()
  document.documentElement.style.setProperty('--toast-lift', '0px')
})
</script>

<template>
  <Teleport to="body">
    <!-- Live regions: each toast is said once, as a whole sentence. -->
    <div class="tl-sr" aria-live="polite" role="status">{{ politeText }}</div>
    <div class="tl-sr" aria-live="assertive" role="alert">{{ alertText }}</div>

    <template v-if="!phone">
      <!-- ── the top lane ── -->
      <div
        v-show="!topHidden && (top.length || store.more)"
        ref="topEl"
        class="tl-top"
        :class="{ deck }"
        :style="{ right: `${dockRight}px` }"
        role="region"
        :aria-label="t('toasts.region.top')"
        data-testid="toast-lane-top"
        @mouseenter="store.hover.top = true"
        @mouseleave="store.hover.top = false"
        @focusin="onFocusIn('top', $event)"
        @focusout="onFocusOut('top', $event)"
        @keydown="onLaneKey($event, 'top')"
      >
        <div class="tl-stack" :class="{ 'tl-deck': deck }">
          <span v-if="deck" class="tl-under u2" />
          <span v-if="deck" class="tl-under u1" />
          <transition-group :name="reduced ? 'tl-fade' : 'tl-top'">
            <div
              v-for="x in topShown"
              :key="x.id"
              class="tl-item"
              tabindex="-1"
              data-kind="toast"
              :data-id="x.id"
              :aria-label="speech(x)"
            >
              <Toast
                :toast="x"
                :active="topOpen"
                :paused="topOpen || store.hiddenTab || store.holds.drag"
                :focused="focusedId === 'toast' + x.id"
                :kbd="kbd"
                :reduced="reduced"
                :now="store.now"
                @run="(id, el) => onRun(x, id, el)"
                @close="store.dismiss(x.id)"
                @details="store.toggleDetails(x.id)"
                @copy="onCopy(x)"
              />
            </div>
          </transition-group>
        </div>
        <button
          v-if="store.more"
          type="button"
          class="tl-more"
          data-testid="toast-more"
          @click="openMore"
        >
          <span class="tl-faces">
            <UserAvatar
              v-for="(f, i) in store.moreFaces"
              :key="i"
              :user-id="f.userId"
              :name="f.name"
            />
          </span>
          <span class="tl-more-t">
            <b>{{ t('toasts.more', { n: store.more }) }}</b> · {{ t('toasts.moreLink') }}
          </span>
          <n-icon :component="ChevronForwardOutline" :size="14" />
        </button>
      </div>

      <!-- ── the bottom lane ── -->
      <div
        v-show="bottom.length || undos.length"
        ref="bottomEl"
        class="tl-bottom"
        role="region"
        :aria-label="t('toasts.region.bottom')"
        data-testid="toast-lane-bottom"
        @mouseenter="store.hover.bottom = true"
        @mouseleave="store.hover.bottom = false"
        @focusin="onFocusIn('bottom', $event)"
        @focusout="onFocusOut('bottom', $event)"
        @keydown="onLaneKey($event, 'bottom')"
      >
        <transition-group :name="reduced ? 'tl-fade' : 'tl-bot'">
          <div
            v-for="x in bottom"
            :key="'t' + x.id"
            class="tl-item tl-resp"
            tabindex="-1"
            data-kind="toast"
            :data-id="x.id"
            :aria-label="speech(x)"
          >
            <Toast
              :toast="x"
              :active="store.hover.bottom || store.focus.bottom"
              :paused="store.hover.bottom || store.focus.bottom || store.hiddenTab"
              :focused="focusedId === 'toast' + x.id"
              :kbd="kbd"
              :reduced="reduced"
              :now="store.now"
              @run="(id, el) => onRun(x, id, el)"
              @close="store.dismiss(x.id)"
              @details="store.toggleDetails(x.id)"
              @copy="onCopy(x)"
            />
          </div>
          <div
            v-for="(u, i) in undos"
            :key="'u' + u.id"
            class="tl-item tl-undo"
            tabindex="-1"
            data-kind="undo"
            :data-id="u.id"
            :class="{ focused: focusedId === 'undo' + u.id }"
          >
            <UndoToast
              :caption="undoCaption(u)"
              :icon="u.icon"
              :status="u.status"
              :progress="u.left / UNDO_MS"
              :paused="u.paused > 0"
              :reduced="reduced"
              :seconds-left="Math.ceil(u.left / 1000)"
              :older="i < undos.length - 1"
              @undo="undo.undo(u.id)"
              @retry="undo.undo(u.id)"
              @close="undo.dismiss(u.id)"
              @mouseenter="undo.pause(u.id, true)"
              @mouseleave="undo.pause(u.id, false)"
              @focusin="undo.pause(u.id, true)"
              @focusout="undo.pause(u.id, false)"
            />
          </div>
        </transition-group>
      </div>
    </template>

    <!-- ── the phone: one lane, one toast at a time ── -->
    <div
      v-else
      v-show="phoneCurrent"
      ref="phoneEl"
      class="tl-phone"
      role="region"
      :aria-label="t('toasts.region.top')"
      data-testid="toast-lane-phone"
      @focusin="onFocusIn('top', $event)"
      @focusout="onFocusOut('top', $event)"
      @keydown="onLaneKey($event, 'phone')"
    >
      <div v-if="queueN > 1" class="tl-queue" data-testid="toast-queue">
        <span class="tl-dots">
          <span v-for="i in Math.min(queueN, 5)" :key="i" :class="{ on: i === 1 }" />
        </span>
        {{ t('toasts.queue', { i: 1, n: queueN }) }}
      </div>
      <div v-if="phoneCurrent" ref="phoneBox" class="tl-swipe">
        <span v-if="drag.active" class="tl-swipe-under">
          <n-icon :component="CloseCircleOutline" :size="18" />{{ t('toasts.swipe') }}
        </span>
        <div
          :key="phoneCurrent.lane + phoneCurrent.id"
          class="tl-item tl-touch"
          tabindex="-1"
          :data-kind="phoneCurrent.lane === 'undo' ? 'undo' : 'toast'"
          :data-id="phoneCurrent.id"
          :style="swipeStyle"
          @pointerdown="onDown"
          @pointermove="onMove"
          @pointerup="onUp"
          @pointercancel="onCancel"
        >
          <UndoToast
            v-if="phoneCurrent.lane === 'undo'"
            :caption="undoCaption(phoneCurrent.src)"
            :icon="phoneCurrent.src.icon"
            :status="phoneCurrent.src.status"
            :progress="phoneCurrent.src.left / UNDO_MS"
            :paused="touching"
            :reduced="reduced"
            :seconds-left="Math.ceil(phoneCurrent.src.left / 1000)"
            @undo="undo.undo(phoneCurrent.id)"
            @retry="undo.undo(phoneCurrent.id)"
            @close="undo.dismiss(phoneCurrent.id)"
          />
          <Toast
            v-else
            :toast="phoneCurrent.src"
            touch
            :paused="touching"
            :kbd="false"
            :reduced="reduced"
            :now="store.now"
            @run="(id, el) => onRun(phoneCurrent.src, id, el)"
            @close="store.dismiss(phoneCurrent.id)"
            @details="store.toggleDetails(phoneCurrent.id)"
            @copy="onCopy(phoneCurrent.src)"
          />
        </div>
      </div>
    </div>

    <SnoozeMenu
      v-if="snooze"
      :show="!!snooze"
      :anchor="() => snooze?.el"
      placement="bottom-start"
      testid="toast-snooze"
      @update:show="(v) => !v && (snooze = null)"
      @pick="onSnoozePick"
    />
  </Teleport>
</template>

<style scoped>
.tl-sr {
  position: fixed;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  clip-path: inset(50%);
  white-space: nowrap;
}

/* Over the large windows and the task modal (Naive modals sit at 2000), under
   the popovers, menus and pickers (2050). */
.tl-top,
.tl-bottom,
.tl-phone {
  position: fixed;
  z-index: 2040;
  pointer-events: none;
}
.tl-top {
  top: 60px;
  width: 360px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  transition: right 0.2s ease;
}
.tl-top > * {
  pointer-events: auto;
}
.tl-stack {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
/* The deck: the newest on top, the two before it as 96% and 92% sheets
   peeking 8px each below it. */
.tl-stack.tl-deck {
  padding-bottom: 16px;
}
.tl-under {
  position: absolute;
  left: 0;
  right: 0;
  height: 60px;
  border-radius: 14px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  transform-origin: 50% 100%;
  transition: transform 0.2s ease;
}
.tl-under.u2 {
  bottom: 0;
  transform: scale(0.92);
  opacity: 0.7;
}
.tl-under.u1 {
  bottom: 8px;
  transform: scale(0.96);
  opacity: 0.88;
}
.tl-item {
  position: relative;
  outline: none;
  border-radius: 14px;
}
.tl-undo.focused::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: var(--t-r-card);
  box-shadow: inset 0 0 0 2px var(--t-primary);
  pointer-events: none;
}
.tl-more {
  height: 36px;
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 0 8px 0 10px;
  box-sizing: border-box;
  border: 0;
  border-radius: 12px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  font: inherit;
  font-size: 12.5px;
  color: var(--t-text2);
  text-align: left;
  cursor: pointer;
}
.tl-more:hover {
  background: var(--t-card-hover);
}
.tl-more b {
  color: var(--t-text1);
  font-weight: 600;
}
.tl-more-t {
  flex: 1;
  white-space: nowrap;
}
.tl-faces {
  display: flex;
  padding-left: 6px;
}
.tl-faces :deep(.ua) {
  width: 18px;
  height: 18px;
  margin-left: -6px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 7px;
  font-weight: 700;
  box-shadow: 0 0 0 2px var(--t-pop);
}

.tl-bottom {
  left: var(--app-content-left, 0px);
  right: 0;
  bottom: 24px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.tl-bottom > * {
  pointer-events: auto;
}
.tl-resp {
  width: max-content;
  min-width: 360px;
  max-width: min(480px, calc(100vw - 32px));
}

/* ── motion: in 180 ms from the lane's edge, out 160 ms aside, the deck 200 ms ── */
.tl-top-enter-active,
.tl-bot-enter-active {
  transition:
    transform 0.18s ease,
    opacity 0.18s ease;
}
.tl-top-leave-active,
.tl-bot-leave-active {
  transition:
    transform 0.16s ease,
    opacity 0.16s ease;
}
/* Only the top lane's deck slides. The bottom lane grows upwards under the
   pointer: a FLIP move there keeps an older toast drawn where the new one has
   just landed for 200 ms, so a click aimed at the newest «Отменить» lands on
   the previous toast's — the undo of the wrong action. The undo toasts never
   slid (package A); the older ones step up at once. */
.tl-top-move {
  transition: transform 0.2s ease;
}
.tl-top-enter-from {
  opacity: 0;
  transform: translateY(-8px);
}
.tl-bot-enter-from {
  opacity: 0;
  transform: translateY(8px);
}
.tl-top-leave-to,
.tl-bot-leave-to {
  opacity: 0;
  transform: translateX(24px);
}
.tl-top-leave-active,
.tl-bot-leave-active {
  position: absolute;
}
.tl-bot-leave-active {
  position: relative;
}
.tl-fade-enter-active,
.tl-fade-leave-active {
  transition: opacity 0.12s linear;
}
.tl-fade-enter-from,
.tl-fade-leave-to {
  opacity: 0;
}

/* ── the phone: over the bottom navigation (or the keyboard), 12px in ── */
.tl-phone {
  left: 12px;
  right: 12px;
  bottom: max(calc(var(--mnav-h, 0px) + 12px), calc(var(--kb-h, 0px) + 12px));
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
}
.tl-phone > * {
  pointer-events: auto;
}
.tl-queue {
  align-self: center;
  height: 22px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 9px;
  border-radius: 999px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-card);
  font-size: 11.5px;
  color: var(--t-text2);
}
.tl-dots {
  display: flex;
  gap: 4px;
}
.tl-dots span {
  width: 6px;
  height: 6px;
  border-radius: 3px;
  background: var(--t-text3);
  opacity: 0.45;
}
.tl-dots span.on {
  background: var(--t-accent-grad);
  opacity: 1;
}
.tl-swipe {
  position: relative;
}
.tl-swipe-under {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  padding-left: 18px;
  font-size: 13px;
  color: var(--t-text3);
}
.tl-touch {
  touch-action: none;
  transition:
    transform 0.2s ease,
    opacity 0.2s ease;
}
.tl-touch :deep(.ut) {
  width: 100%;
}
</style>
