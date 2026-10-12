import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  inbox as inboxApi,
  notes as notesApi,
  reminders as remApi,
  workspaces as wsApi,
} from '@/api'
import { firedCount, remindersByTask, tagsByProject } from '@/utils/inbox'
import { buildMentionItems } from '@/utils/mentions'
import { sGet, sSet } from '@/utils/serverStorage'
import { tourEvent } from '@/utils/tourEvents'
import { enqueue, flush, newClientId, queued } from '@/utils/captureQueue'

// «Входящие» (#2972): the user's reminders and notes of the current workspace,
// the panel's state and the capture popover's. One copy for the whole shell —
// the panel, the top-bar counter, the sidebar row, the start screen, the task
// modal and the card pills all read it, so a card's bell pill costs nothing:
// the feed was loaded once for the workspace (#2998: no request per card).
//
// Records keep the server's row plus `kind` ('reminder' | 'note'). The socket
// keeps the copy fresh (onEvent): reminder events are addressed to their owner,
// note events to whoever may read the note (a private one — to its author).
//
// Tags and mentions (#3040): the store also holds the workspace's members and
// tags — the capture resolves «@человек» and «#тег» against them, the records
// draw their pills from them (a project tag takes its colour).

export const WIDTH_KEY = 'tessera_inbox_width'
export const MIN_W = 320
export const MAX_W = 520
const DEFAULT_W = 388

const clampW = (w) => Math.min(MAX_W, Math.max(MIN_W, Math.round(w) || DEFAULT_W))

export const useInboxStore = defineStore('inbox', () => {
  const wsId = ref(null)
  const reminders = ref([])
  const notes = ref([])
  const members = ref([])
  const wsTags = ref([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  // The clock the feed is grouped by; ticked by the shell every 30 s so a
  // reminder slides into «Сработало» without a reload.
  const now = ref(new Date())

  // ── the panel ──
  const panelOpen = ref(false)
  const filter = ref('all')
  // The tag the feed is narrowed to (a tagKey), '' — none (#3040).
  const tagFilter = ref('')
  // The record under the keyboard cursor: 'reminder:<id>' | 'note:<id>'.
  const selected = ref('')
  // The record opened in place for editing, and the field to focus there
  // ('tags' — from «Теги» in the record's menu).
  const expanded = ref('')
  const editFocus = ref('')
  // Bumped to ask the panel to move focus into the list (G I).
  const focusSeq = ref(0)
  const width = ref(clampW(Number(sGet(WIDTH_KEY)) || DEFAULT_W))

  // ── the capture popover (#2996) ──
  // { kind: 'note' | 'reminder' | null (by the text), text, taskNumber? }
  const capture = ref(null)
  // The last saved line — ↑ in the empty field brings it back.
  const lastCaptured = ref('')

  let seq = 0
  let capSeq = 0
  let reloadTimer = 0

  const fired = computed(() => firedCount(reminders.value, now.value))
  const byTask = computed(() => remindersByTask(reminders.value))
  const total = computed(() => reminders.value.filter((r) => !r.done).length + notes.value.length)
  const tagsOf = computed(() => tagsByProject(wsTags.value))
  // The @-popup rows of the record editors: Tessera members only — a record is
  // never pushed to GitLab, so the inserted handle is the member's own.
  const mentionItems = computed(() =>
    buildMentionItems(
      members.value.map((m) => ({ ...m, gl_username: '' })),
      [],
      {},
    ),
  )

  const tag = (kind) => (r) => ({ ...r, kind })

  async function load(id = wsId.value) {
    if (!id) return
    if (id !== wsId.value) {
      reminders.value = []
      notes.value = []
      members.value = []
      wsTags.value = []
      tagFilter.value = ''
      loaded.value = false
      selected.value = ''
      expanded.value = ''
    }
    wsId.value = id
    const mine = ++seq
    loading.value = true
    error.value = ''
    loadPeople(id, mine)
    try {
      const { data } = await inboxApi.list(id)
      if (mine !== seq) return
      reminders.value = (data?.reminders || []).map(tag('reminder'))
      notes.value = (data?.notes || []).map(tag('note'))
      loaded.value = true
      now.value = new Date()
      // Reachable again: whatever waited on the device goes out now.
      flushQueue()
    } catch (e) {
      if (mine === seq) error.value = e?.message || 'error'
    } finally {
      if (mine === seq) loading.value = false
    }
  }

  // Members and tags ride beside the feed; neither is worth an error state —
  // without them the pills fall back to plain text.
  async function loadPeople(id, mine) {
    const [m, tg] = await Promise.allSettled([wsApi.members(id), wsApi.tags(id)])
    if (mine !== seq) return
    if (m.status === 'fulfilled') members.value = m.value?.data || []
    if (tg.status === 'fulfilled') wsTags.value = tg.value?.data || []
  }

  function scheduleReload() {
    clearTimeout(reloadTimer)
    reloadTimer = setTimeout(() => load(), 400)
  }

  function tick() {
    now.value = new Date()
  }

  // Does a reminder belong to this workspace's feed? Its own workspace, or none
  // (old reminders outside every workspace are shown everywhere, #3041).
  const ownsReminder = (r) => !r.workspace_id || r.workspace_id === wsId.value

  function upsert(list, rec, chipFrom) {
    const i = list.value.findIndex((x) => x.id === rec.id)
    const prev = i >= 0 ? list.value[i] : null
    // The socket and the plain endpoints carry the row without the task chip:
    // keep the chip while the link is the same, re-read the feed for a new one.
    let task = rec.task
    if (task === undefined) {
      if (!rec.task_id) task = null
      else if (prev?.task && prev.task.id === rec.task_id) task = prev.task
      else task = chipFrom?.(rec.task_id) ?? null
      if (rec.task_id && !task) scheduleReload()
    }
    const next = { ...prev, ...rec, task }
    if (i >= 0) list.value = list.value.map((x, j) => (j === i ? next : x))
    else list.value = [...list.value, next]
    return next
  }

  function drop(list, id) {
    list.value = list.value.filter((x) => x.id !== id)
    if (selected.value.endsWith(id)) selected.value = ''
    if (expanded.value.endsWith(id)) expanded.value = ''
  }

  function chipOf(taskId) {
    for (const r of [...reminders.value, ...notes.value]) if (r.task?.id === taskId) return r.task
    return null
  }

  function putReminder(r) {
    if (!ownsReminder(r)) return drop(reminders, r.id)
    return upsert(reminders, tag('reminder')(r), chipOf)
  }
  function putNote(n) {
    if (n.workspace_id !== wsId.value) return drop(notes, n.id)
    return upsert(notes, tag('note')(n), chipOf)
  }

  function onEvent(ev) {
    const d = ev?.data
    if (!d) return
    switch (ev.type) {
      case 'reminder.created':
      case 'reminder.updated':
        putReminder(d)
        break
      case 'reminder.deleted':
        drop(reminders, d.id)
        break
      case 'note.created':
      case 'note.updated':
        if (ev.scope === wsId.value) putNote(d)
        break
      case 'note.deleted':
        if (ev.scope === wsId.value) drop(notes, d.id)
        break
    }
  }

  // ── mutations: each answers with the saved record, already in the feed ──
  // Creating names the record on the client (client_id, #3042): without a
  // connection the capture waits on the device ({ queued: true }) and goes out
  // when the connection is back — a resend can't make a twin.
  async function createNote(data) {
    const body = { client_id: newClientId(), ...data }
    try {
      const res = await notesApi.create(wsId.value, body)
      tourEvent('inbox.captured')
      return putNote(res.data)
    } catch (e) {
      if (!e?.offline) throw e
      pending.value = enqueue({ kind: 'note', wsId: wsId.value, data: body })
      return { queued: true }
    }
  }
  async function createReminder(data) {
    const body = { client_id: newClientId(), workspace_id: wsId.value, ...data }
    try {
      const res = await remApi.create(body)
      tourEvent('inbox.captured')
      return putReminder(res.data)
    } catch (e) {
      if (!e?.offline) throw e
      pending.value = enqueue({ kind: 'reminder', wsId: wsId.value, data: body })
      return { queued: true }
    }
  }

  // ── the offline queue (#3042) ──
  const pending = ref(queued().length)
  const dropped = ref(0)
  let flushing = null
  function flushQueue() {
    if (flushing || !queued().length) return flushing
    flushing = flush((item) =>
      item.kind === 'reminder' ? remApi.create(item.data) : notesApi.create(item.wsId, item.data),
    )
      .then((r) => {
        for (const { item, res } of r.sent) {
          if (item.wsId !== wsId.value) continue
          if (item.kind === 'reminder') putReminder(res.data)
          else putNote(res.data)
        }
        if (r.sent.length) tourEvent('inbox.captured')
        dropped.value += r.dropped.length
        pending.value = r.left
        return r
      })
      .finally(() => (flushing = null))
    return flushing
  }
  if (typeof window !== 'undefined') window.addEventListener('online', () => flushQueue())
  async function updateReminder(r, patch) {
    const res = await remApi.update(r.id, patch)
    return putReminder(res.data)
  }
  async function updateNote(n, patch) {
    const res = await notesApi.update(n.id, { title: n.title, body: n.body, ...patch })
    return putNote(res.data)
  }
  async function snooze(r, at) {
    const res = await remApi.snooze(r.id, new Date(at).toISOString())
    tourEvent('inbox.handled')
    return putReminder(res.data)
  }
  async function setDone(r, done) {
    if (done) tourEvent('inbox.handled')
    return updateReminder(r, { done })
  }
  async function remove(rec) {
    if (rec.kind === 'note') {
      await notesApi.remove(rec.id)
      drop(notes, rec.id)
    } else {
      await remApi.remove(rec.id)
      drop(reminders, rec.id)
    }
  }
  // Brings a deleted record back as it was (the undo toast): a new row with
  // the same content — the id changes, nothing reads it.
  async function restore(rec) {
    if (rec.kind === 'note') {
      return createNote({
        title: rec.title,
        body: rec.body,
        task_id: rec.task_id || undefined,
        project_id: rec.project_id || undefined,
        visibility: rec.visibility,
        tags: rec.tags || [],
        mentions: rec.mentions || [],
      })
    }
    const back = await createReminder({
      message: rec.message,
      body: rec.body,
      remind_at: rec.remind_at,
      task_id: rec.task_id || undefined,
      workspace_id: rec.workspace_id || undefined,
      tags: rec.tags || [],
      mentions: rec.mentions || [],
    })
    if (rec.done) return setDone(back, true)
    return back
  }

  // ── panel ──
  function open({ focus = false, filter: f } = {}) {
    panelOpen.value = true
    if (f) filter.value = f
    if (focus) focusSeq.value++
    if (!loaded.value) load()
  }
  function close() {
    panelOpen.value = false
    expanded.value = ''
  }
  function toggle(opts) {
    if (panelOpen.value) close()
    else open(opts)
  }
  function setWidth(w) {
    width.value = clampW(w)
    sSet(WIDTH_KEY, String(width.value))
  }
  // Opens the panel on one record (a search result, a deep link).
  function reveal(kind, id) {
    filter.value = 'all'
    tagFilter.value = ''
    selected.value = `${kind}:${id}`
    if (kind === 'note') expanded.value = `note:${id}`
    open()
  }

  // ── capture ──
  // opts: { kind, text, inline } — `inline` is the copy inside the task modal.
  function startCapture(opts = {}) {
    capture.value = {
      kind: opts.kind || null,
      text: opts.text || '',
      inline: !!opts.inline,
      seq: ++capSeq,
    }
  }
  function endCapture() {
    capture.value = null
  }

  return {
    pending,
    dropped,
    flushQueue,
    wsId,
    reminders,
    notes,
    members,
    wsTags,
    tagsOf,
    mentionItems,
    loading,
    loaded,
    error,
    now,
    fired,
    byTask,
    total,
    panelOpen,
    filter,
    tagFilter,
    selected,
    expanded,
    editFocus,
    focusSeq,
    width,
    capture,
    lastCaptured,
    load,
    tick,
    onEvent,
    createNote,
    createReminder,
    updateReminder,
    updateNote,
    snooze,
    setDone,
    remove,
    restore,
    open,
    close,
    toggle,
    setWidth,
    reveal,
    startCapture,
    endCapture,
  }
})
