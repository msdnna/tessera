import { defineStore } from 'pinia'
import { computed, reactive, ref } from 'vue'
import { notificationPrefs as prefsApi } from '@/api'
import { i18n } from '@/i18n'
import { takeRecentFailure } from '@/utils/lastFailure'
import { buildNotificationToast } from '@/utils/notificationToast'
import {
  ACTIVITY_DELAY,
  ACTIVITY_TWIN_MS,
  BATCH_WINDOW,
  DURATION,
  RANK,
  activityAllowed,
  awayGroup,
  batchKey,
  defaultPrefs,
  dndActive,
  inQuietHours,
  insertBottom,
  insertTop,
  normalizePrefs,
  passesQuiet,
} from '@/utils/toastModel'

// The in-app toasts (v2, #3117 — PackageI): one frame, two lanes.
//
//   bottom — the answer to my own action: errors, warnings, success, info
//            (the old naive `useMessage`, see composables/useMessage.js). The
//            undo toasts (stores/undo) share this lane on screen but keep their
//            own store and rules (package A): up to three, ⌘Z, 7 s.
//   top    — what came from outside: «про меня» (the same record as the bell's
//            feed row), reminders, calls, the open board's activity, the
//            system, the «Пока вас не было» digest.
//
// The rules (durations, who gives way, what passes «Не беспокоить», folding a
// burst of activity) are pure and live in utils/toastModel.js; this store holds
// the lanes, the clock, the settings and the holds. The host (components/toast/
// ToastLanes.vue) draws the lanes, drives the clock and wires the actions to
// the router.
//
// Text is never resolved at push time: fields are strings or functions read
// when drawn, so a toast outliving a language switch speaks the new language.

let seq = 0
const t = (...a) => i18n.global.t(...a)

export const useToastsStore = defineStore('toasts', () => {
  const top = ref([]) // newest first
  const bottom = ref([]) // oldest first — the lane grows upwards
  // Toasts waiting for a hold to lift: a card being dragged; on a phone, the
  // on-screen keyboard (about-me and activity only).
  const held = ref([])
  // «Ещё N · Показать в ленте»: about-me toasts pushed out of a full deck — their
  // rows stay unread in the feed. The faces of the last three.
  const more = ref(0)
  const moreFaces = ref([])

  // ── settings (notification_prefs, #3117) ──
  const server = reactive({
    loaded: false,
    toasts_enabled: true,
    quiet_toasts: false,
    quiet_enabled: false,
    quiet_start_minutes: 1320,
    quiet_end_minutes: 480,
    quiet_tz: '',
    dnd_until: null,
  })
  const prefs = ref(defaultPrefs())
  const now = ref(Date.now())
  const dnd = computed(() => dndActive(server.dnd_until, new Date(now.value)))

  // ── holds and pauses ──
  const holds = reactive({ drag: false, palette: false, keyboard: false })
  const hover = reactive({ top: false, bottom: false })
  const focus = reactive({ top: false, bottom: false })
  const hiddenTab = ref(false)
  // A phone shows one lane, one toast at a time (set by the host).
  const phone = ref(false)
  // On a call's own screen only mentions and calls get through.
  const callScreen = ref(false)

  // «Доступно обновление» after «Позже»: a chip in the top bar until reload.
  const updateChip = ref(false)

  // Cards to pulse on the open board: { ids, seq }.
  const pulse = ref({ ids: [], seq: 0 })

  // What the action buttons of about-me toasts do — set by the host, which has
  // the router: open, reply, joinCall, snooze, done, openGitlab, openFeed,
  // reload.
  let act = {}
  function bind(handlers) {
    act = handlers || {}
  }
  // «Обновить» of the update toast and the top bar's chip: the service worker's
  // reload, handed in by App.vue (web only).
  let reloader = null
  function setReloader(fn) {
    reloader = fn
  }
  function reload() {
    reloader?.()
  }

  // ── the frame of a toast ──
  function make(spec) {
    const kind = spec.kind || 'info'
    const duration = spec.duration ?? DURATION[kind] ?? DURATION.info
    return {
      id: ++seq,
      lane: 'top',
      at: Date.now(),
      rank: RANK[kind] ?? 0,
      look: 'card',
      bold: '',
      rest: '',
      actions: [],
      live: 'polite',
      count: 0,
      bump: 0,
      detailsOpen: false,
      ...spec,
      kind,
      duration,
      left: duration,
    }
  }

  function find(id) {
    return top.value.find((x) => x.id === id) || bottom.value.find((x) => x.id === id)
  }

  function remove(id) {
    const x = find(id)
    top.value = top.value.filter((y) => y.id !== id)
    bottom.value = bottom.value.filter((y) => y.id !== id)
    return x
  }

  // Is a top-lane toast of this kind allowed on screen right now?
  function topAllowed(kind) {
    if (!server.toasts_enabled) return false
    if (phone.value && !prefs.value.phone) return false
    if (callScreen.value && kind !== 'mention' && kind !== 'call') return false
    if (passesQuiet(kind)) return true
    return !dnd.value && !inQuietHours(server, new Date())
  }

  function waits(item) {
    if (holds.drag && item.tone !== 'red') return true
    return (
      phone.value &&
      holds.keyboard &&
      item.lane === 'top' &&
      ['me', 'mention', 'assign', 'activity'].includes(item.kind)
    )
  }

  function show(item) {
    if (waits(item)) {
      held.value = [...held.value, item]
      return item
    }
    if (item.lane === 'bottom') {
      const { list } = insertBottom(bottom.value, item)
      bottom.value = list
      return item
    }
    const { list, evicted } = insertTop(top.value, item)
    top.value = list
    for (const x of evicted) {
      if (x.notificationId) {
        more.value++
        if (x.lead?.avatar) moreFaces.value = [x.lead.avatar, ...moreFaces.value].slice(0, 3)
      }
    }
    if (list.includes(item) && item.taskIds?.length) firePulse(item.taskIds)
    return item
  }

  // Lift the holds: whatever waited comes out, in order.
  function release() {
    const waiting = held.value
    held.value = []
    for (const x of waiting) show(x)
  }
  function setHold(name, on) {
    if (holds[name] === !!on) return
    holds[name] = !!on
    if (!on) release()
  }

  // ── the clock ──
  // tick advances the timers by dt ms. A lane under the pointer or the focus
  // stands still as a whole (not one toast of it); so does everything while the
  // tab is hidden or a card is dragged, and the top lane while the palette hides
  // it. `only` — the phone: just the one toast on screen runs.
  function tick(dt, { only = null, paused = false } = {}) {
    now.value = Date.now()
    const still = (lane) =>
      hiddenTab.value ||
      holds.drag ||
      hover[lane] ||
      focus[lane] ||
      (lane === 'top' && holds.palette)
    for (const lane of ['top', 'bottom']) {
      const list = lane === 'top' ? top : bottom
      let gone = false
      for (const x of list.value) {
        if (!x.duration) continue
        if (only != null ? x.id !== only || paused : still(lane)) continue
        x.left -= dt
        if (x.left <= 0) gone = true
      }
      if (gone) {
        const out = list.value.filter((x) => x.duration && x.left <= 0)
        list.value = list.value.filter((x) => !(x.duration && x.left <= 0))
        for (const x of out) x.onExpire?.()
      }
    }
    if (!top.value.length) {
      more.value = 0
      moreFaces.value = []
    }
  }

  // ── closing and acting ──
  function dismiss(id) {
    const x = remove(id)
    x?.onClose?.()
  }
  function dismissLane(lane) {
    const list = lane === 'top' ? top.value : bottom.value
    for (const x of [...list]) dismiss(x.id)
    if (lane === 'top') {
      more.value = 0
      moreFaces.value = []
    }
  }
  function clear() {
    top.value = []
    bottom.value = []
    held.value = []
    more.value = 0
    moreFaces.value = []
  }

  // act runs a toast's button. Most close the toast; `keep` ones (Подробнее,
  // Отложить — it opens a menu) leave it up, `dismiss` ones only close it.
  async function run(id, actionId, el) {
    const x = find(id)
    const a = x?.actions.find((b) => b.id === actionId)
    if (!a || a.stub || a.disabled) return
    if (a.dismiss) return dismiss(id)
    if (!a.keep) remove(id)
    try {
      await a.run?.(el, x)
    } catch {
      /* the action reports its own failure */
    }
  }

  // The bump of a folded batch: the number turns into a gradient pill for
  // 600 ms and the timer starts over (PackageI «Обновление пачки»).
  function bumpToast(x) {
    x.left = x.duration
    x.bump = Date.now()
  }

  function firePulse(ids) {
    pulse.value = { ids: [...ids], seq: pulse.value.seq + 1 }
  }

  // ── «про меня» from the socket ──
  const seen = new Set()
  // Task id → when it last came to me as a notification: the board's twin
  // event about it shows no activity toast (PackageI).
  const recentMe = new Map()
  // The digest while the tab is hidden.
  const away = reactive({ since: null, counts: {} })
  // Changes to one task folded into one toast within the window.
  const meFold = new Map()

  function onSocket(ev, meId) {
    if (ev?.type !== 'notification' || !ev.data || ev.data.user_id !== meId) return
    const n = ev.data.notification
    if (!n?.id || seen.has(n.id)) return
    seen.add(n.id)
    notification(n)
  }

  function notification(n) {
    if (n.task_id) recentMe.set(n.task_id, Date.now())
    const spec = buildNotificationToast(n, { act })
    // A twin activity toast already up yields to this one.
    if (n.task_id)
      top.value = top.value.filter((x) => !(x.kind === 'activity' && x.taskId === n.task_id))
    if (hiddenTab.value) {
      // The OS notification speaks while the tab is hidden; the toast does
      // not pile up — one digest greets the reader on return.
      const g = awayGroup(n)
      if (g) away.counts[g] = (away.counts[g] || 0) + 1
      return null
    }
    if (!topAllowed(spec.kind)) return null
    if (spec.kind !== 'call' && spec.kind !== 'reminder' && prefs.value.me[spec.category] === false)
      return null
    if (spec.category === 'changes' && foldMe(n, spec)) return null
    const item = make(spec)
    if (n.task_id) meFold.set(n.task_id, { id: item.id, first: Date.now(), actors: [n] })
    return show(item)
  }

  // Several changes to one task within the window fold into one toast: one
  // person — «изменил вашу задачу» with the changes listed; several — «3
  // изменения» with both faces (PackageI «Свёртка пачек»).
  function foldMe(n, spec) {
    if (!prefs.value.batch || !n.task_id) return false
    const f = meFold.get(n.task_id)
    if (!f || Date.now() - f.first > BATCH_WINDOW) return false
    const x = top.value.find((y) => y.id === f.id)
    if (!x) return false
    f.actors.push(n)
    const people = [...new Set(f.actors.map((a) => a.actor_id || a.payload?.actor || ''))]
    const lines = f.actors.map((a) => buildNotificationToast(a, { act }).rest)
    x.diff = null
    if (people.length === 1) {
      x.bold = spec.bold
      x.rest = () => t('toasts.me.changedTask')
      x.list = lines
      x.lead = { ...spec.lead, badge: 'CreateOutline', tone: 'acc' }
    } else {
      const names = [...new Set(f.actors.map((a) => (a.payload?.actor || '').split(' ')[0]))]
      const count = f.actors.length
      x.bold = () => t('toasts.me.changes', count, { named: { n: count } })
      x.rest = () => t('toasts.me.inYourTask')
      x.list = null
      x.line = () => names.filter(Boolean).join(', ')
      const a = f.actors[0]
      const b = f.actors.find((y) => (y.actor_id || '') !== (a.actor_id || '')) || a
      x.lead = {
        duo: [
          { userId: a.actor_id || '', name: a.payload?.actor || '' },
          { userId: b.actor_id || '', name: b.payload?.actor || '' },
        ],
      }
    }
    x.notificationId = n.id
    bumpToast(x)
    return true
  }

  // ── the tab hides and comes back ──
  function setHiddenTab(hidden) {
    if (hidden === hiddenTab.value) return
    hiddenTab.value = hidden
    if (hidden) {
      away.since = new Date().toISOString()
      away.counts = {}
      return
    }
    const counts = away.counts
    const since = away.since
    away.counts = {}
    away.since = null
    const total = Object.values(counts).reduce((a, b) => a + b, 0)
    if (!total || !topAllowed('system')) return
    show(
      make({
        lane: 'top',
        kind: 'system',
        duration: DURATION.away,
        lead: { tile: 'NotificationsOutline', tone: 'acc' },
        bold: () => t('toasts.away.title'),
        awayCounts: counts,
        time: since,
        actions: [
          {
            id: 'feed',
            label: () => t('toasts.away.open'),
            caps: ['↵'],
            primary: true,
            run: () => act.openFeed?.(),
          },
        ],
        always: true,
      }),
    )
  }

  // ── the open board's activity ──
  const batches = new Map()

  // activity: { taskId, number, title, verb, actorId, actorName, self, column }
  function activity(a) {
    if (hiddenTab.value || !topAllowed('activity')) return
    if (!activityAllowed(prefs.value, a)) return
    setTimeout(() => activityNow(a), ACTIVITY_DELAY)
  }

  function activityNow(a) {
    if (a.taskId && Date.now() - (recentMe.get(a.taskId) || 0) < ACTIVITY_TWIN_MS) return
    const key = batchKey(a)
    const nowMs = Date.now()
    let b = batches.get(key)
    if (!b || nowMs - b.first > BATCH_WINDOW) {
      b = { first: nowMs, events: [], singles: [], toastId: null }
      batches.set(key, b)
    }
    if (!b.events.some((e) => e.taskId === a.taskId)) b.events.push(a)
    const n = b.events.length
    if (!prefs.value.batch || n < prefs.value.batchMin) {
      const item = make(activityToast(a))
      b.singles.push(item.id)
      show(item)
      return
    }
    const open = b.toastId && top.value.find((x) => x.id === b.toastId)
    if (open) {
      open.count = n
      open.taskIds = b.events.map((e) => e.taskId)
      bumpToast(open)
      firePulse([a.taskId])
      return
    }
    top.value = top.value.filter((x) => !b.singles.includes(x.id))
    const item = make(batchToast(b.events))
    b.toastId = item.id
    show(item)
  }

  const firstName = (a) =>
    a.self ? t('toasts.act.you') : (a.actorName || '').split(' ')[0] || t('toasts.act.someone')

  function activityToast(a) {
    return {
      lane: 'top',
      kind: 'activity',
      look: 'act',
      taskId: a.taskId,
      taskIds: [a.taskId],
      lead: { avatar: { userId: a.actorId || '', name: a.actorName || '' } },
      bold: () => firstName(a),
      rest: () =>
        t(`toasts.activity.${a.verb}`, {
          ref: a.number ?? '',
          title: a.title || '',
          column: a.column || '',
        }),
      actions: [
        {
          id: 'open',
          label: () => t('toasts.act.open'),
          caps: ['↵'],
          primary: true,
          hidden: true,
          run: () => act.openTask?.(a),
        },
      ],
    }
  }

  function batchToast(events) {
    const a = events[0]
    return {
      lane: 'top',
      kind: 'activity',
      look: 'act',
      batch: true,
      count: events.length,
      taskIds: events.map((e) => e.taskId),
      lead: { avatar: { userId: a.actorId || '', name: a.actorName || '' } },
      bold: () => firstName(a),
      rest: () => t(`toasts.batch.${a.verb}`),
      tail(count) {
        return t(`toasts.batch.tail.${a.verb === 'moved' ? 'moved' : 'other'}`, count, {
          named: { n: count, column: a.column || '' },
        })
      },
      actions: [
        {
          id: 'show',
          label: () => t('toasts.act.show'),
          caps: ['↵'],
          primary: true,
          keep: true,
          run: (el, x) => firePulse(x?.taskIds || []),
        },
      ],
    }
  }

  // ── a new version is deployed ──
  function update() {
    if (updateChip.value || top.value.some((x) => x.kind === 'update')) return
    if (!topAllowed('system')) {
      updateChip.value = true
      return
    }
    show(
      make({
        lane: 'top',
        kind: 'system',
        duration: DURATION.update,
        update: true,
        lead: { tile: 'CloudDownloadOutline', tone: 'acc' },
        bold: () => t('toasts.update.title'),
        line: () => t('toasts.update.text'),
        actions: [
          {
            id: 'reload',
            label: () => t('toasts.update.reload'),
            caps: ['↵'],
            primary: true,
            run: () => reload(),
          },
          {
            id: 'later',
            label: () => t('toasts.update.later'),
            run: () => (updateChip.value = true),
          },
        ],
        // × is «Позже» too: the update does not go away, it moves to the chip.
        onClose: () => (updateChip.value = true),
        always: true,
      }),
    )
  }

  // ── answers to my own actions (the message API) ──
  // kind: error | warning | success | info | loading. `content` is a string or
  // a function returning one. opts: { retry, line, action: {label, run},
  // duration } — retry makes an error wait for the reader.
  function respond(kind, content, opts = {}) {
    const base = {
      lane: 'bottom',
      bold: content,
      time: null,
    }
    let spec
    if (kind === 'error') {
      const f = takeRecentFailure()
      const actions = []
      if (opts.retry)
        actions.push({
          id: 'retry',
          label: () => t('toasts.err.retry'),
          caps: ['↵'],
          primary: true,
          run: opts.retry,
        })
      const details = f ? failureLines(f) : null
      if (details)
        actions.push({
          id: 'details',
          label: () => t('toasts.err.details'),
          keep: true,
          details: true,
        })
      spec = {
        ...base,
        kind: 'error',
        tone: 'red',
        lead: { tile: f?.offline ? 'CloudOfflineOutline' : 'AlertCircleOutline', tone: 'red' },
        line: opts.line || (f ? failureLine(f) : ''),
        details,
        actions,
        live: 'assertive',
        always: true,
        duration: opts.retry ? DURATION.errorRetry : (opts.duration ?? DURATION.error),
      }
    } else if (kind === 'warning') {
      spec = {
        ...base,
        kind: 'warning',
        tone: 'or',
        lead: { tile: 'WarningOutline', tone: 'or' },
        line: opts.line || '',
        always: true,
        duration: opts.duration ?? DURATION.warning,
      }
    } else if (kind === 'success') {
      spec = {
        ...base,
        kind: 'success',
        tone: 'gr',
        lead: { tile: 'CheckmarkOutline', tone: 'gr' },
        line: opts.line || '',
        always: true,
        duration: opts.duration ?? DURATION.success,
      }
    } else {
      spec = {
        ...base,
        kind: kind === 'loading' ? 'loading' : 'info',
        tone: 'neu',
        lead: { tile: 'InformationCircleOutline', tone: 'neu', spin: kind === 'loading' },
        line: opts.line || '',
        always: true,
        duration: kind === 'loading' ? 0 : (opts.duration ?? DURATION.info),
      }
    }
    if (opts.action)
      spec.actions = [
        ...(spec.actions || []),
        {
          id: 'action',
          label: opts.action.label,
          caps: ['↵'],
          primary: true,
          run: opts.action.run,
        },
      ]
    const item = show(make(spec))
    return {
      id: item.id,
      destroy: () => remove(item.id),
    }
  }

  // «Подробнее» opens the request lines and swaps itself for «Скопировать».
  function toggleDetails(id) {
    const x = find(id)
    if (!x) return
    x.detailsOpen = !x.detailsOpen
    x.left = x.duration
  }

  // Sign-out: the departing reader's toasts and settings go; the update chip
  // (about the build, not the person) stays.
  function resetSession() {
    top.value = []
    held.value = []
    more.value = 0
    moreFaces.value = []
    seen.clear()
    recentMe.clear()
    meFold.clear()
    batches.clear()
    Object.assign(server, {
      loaded: false,
      toasts_enabled: true,
      quiet_toasts: false,
      quiet_enabled: false,
      quiet_start_minutes: 1320,
      quiet_end_minutes: 480,
      quiet_tz: '',
      dnd_until: null,
    })
    prefs.value = defaultPrefs()
  }

  // ── settings ──
  function applyServer(data) {
    if (!data) return
    for (const k of Object.keys(server)) if (k in data) server[k] = data[k]
    server.loaded = true
    prefs.value = normalizePrefs(data.toasts)
  }
  async function loadPrefs() {
    try {
      const res = await prefsApi.get()
      applyServer(res.data)
    } catch {
      /* offline: the defaults stand */
    }
  }
  async function setDnd(until) {
    const prev = server.dnd_until
    server.dnd_until = until ? new Date(until).toISOString() : null
    try {
      const res = await prefsApi.dnd(server.dnd_until)
      applyServer(res.data)
    } catch (e) {
      server.dnd_until = prev
      throw e
    }
  }

  return {
    top,
    bottom,
    held,
    more,
    moreFaces,
    server,
    prefs,
    dnd,
    now,
    holds,
    hover,
    focus,
    hiddenTab,
    phone,
    callScreen,
    updateChip,
    pulse,
    bind,
    setReloader,
    reload,
    make,
    find,
    show,
    remove,
    tick,
    dismiss,
    dismissLane,
    clear,
    run,
    setHold,
    onSocket,
    notification,
    setHiddenTab,
    activity,
    update,
    respond,
    toggleDetails,
    firePulse,
    applyServer,
    resetSession,
    loadPrefs,
    setDnd,
  }
})

// The error toast's line three: what kind of refusal it was.
function failureLine(f) {
  if (f.offline) return ''
  if (f.status >= 500) return t('toasts.err.server', { status: f.status })
  if (f.status) return t('toasts.err.refused', { status: f.status })
  return ''
}

// «Подробнее»: the request, the server's own words, the request id and time.
function failureLines(f) {
  const lines = [`${f.method} ${f.url} → ${f.status || '—'}`]
  if (f.raw) lines.push(f.raw)
  const at = new Date(f.at).toLocaleTimeString('en-GB', { hour12: false })
  lines.push(f.requestId ? `request-id ${f.requestId} · ${at}` : at)
  return lines
}
