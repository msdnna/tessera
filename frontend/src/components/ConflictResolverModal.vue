<script setup>
// «Разрешение конфликтов» — a large window (v2, #2975, mockup ConflictWin): the
// GitLab write-back conflicts of the workspace, three versions side by side —
// the base (the last sync), Tessera and GitLab — field by field. A field changed
// on one side only is taken automatically and drawn quieter. The queue of tasks
// with conflicts is the left column.
//
// The window's one deliberate exception to «no Save button»: the decisions go
// to GitLab in one push, so they are applied together by «Применить и
// отправить» (⌘↵), enabled once every field has a version — every task of the
// queue that is fully decided goes in the same request (#3089). Esc closes the
// window and keeps the choices as a draft (stores/conflicts); «Отложить» keeps
// the draft across reloads and sends the task to the end of the queue, out of
// the batch.
//
// The server still takes one of three resolutions per conflict: theirs (GitLab
// wins, nothing is pushed), ours (Tessera wins, pushed) or manual (the given
// values are written into the task and pushed). The per-field picks map onto
// them: all GitLab → theirs, all Tessera → ours, anything else → manual with
// each field's chosen value.
//
// Opened from anywhere through the conflicts store (the conflict pill on a card,
// the GitLab window, the bell).
//
// Stages take part too (#3025): a GitLab stage changed both here and in GitLab
// (its goals, usually) is one more entry of the queue — entity 'milestone',
// fields title / description / start / due — resolved through the stage's own
// endpoint, which writes the decision to the GitLab milestone at once.
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  GitMergeOutline,
  TrailSignOutline,
  CheckmarkOutline,
  CloudUploadOutline,
  OpenOutline,
  ArrowUpOutline,
  LogoGitlab,
} from '@vicons/ionicons5'
import { gitlab as glApi, tasks as tasksApi, milestones as msApi } from '@/api'
import { useConflictsStore } from '@/stores/conflicts'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useShortcuts } from '@/composables/useShortcuts'
import { useResponsive } from '@/composables/useResponsive'
import { useFormat } from '@/composables/useFormat'
import { diffSegments } from '@/utils/linediff'
import { priorityLabel } from '@/utils/priority'
import { workspaceName } from '@/utils/defaultNames'
import { cap } from '@/utils/keyCaps'
import EmptyState from '@/components/EmptyState.vue'
import LoaderOverlay from '@/components/LoaderOverlay.vue'
import LargeWindow from './kit/LargeWindow.vue'
import ShortcutKeys from './ShortcutKeys.vue'
import Kbd from './Kbd.vue'
import './kit/kit.css'

const props = defineProps({
  show: { type: Boolean, default: false },
  wsId: { type: String, default: null },
  focusTaskId: { type: String, default: null },
  focusMilestoneId: { type: String, default: null },
})
const emit = defineEmits(['update:show', 'resolved'])

const { t } = useI18n()
const router = useRouter()
const message = useMessage()
const store = useConflictsStore()
const ws = useWorkspacesStore()
const { isMobile } = useResponsive()
const { formatDateTime } = useFormat()
const winRef = ref(null)

const conflicts = ref([])
const loading = ref(false)
const selectedId = ref(null)
const resolving = ref(false)

// Field and change-kind names come from the catalog per call (a frozen map would
// keep the language of the first render). An unknown key from the server is
// shown raw.
const FIELD_KEYS = [
  'title',
  'description',
  'start',
  'due',
  'estimate',
  'state',
  'priority',
  'assignees',
  'labels',
]
const KIND_KEYS = [
  'title_desc',
  'due',
  'estimate',
  'state',
  'priority',
  'milestone',
  'assignees',
  'labels',
]
// Free text, numbers and lists (assignees, labels — comma-separated, #3089) can
// be merged by hand; state and priority are discrete.
const MERGEABLE = ['title', 'description', 'start', 'due', 'estimate', 'assignees', 'labels']
const SIDES = ['base', 'ours', 'theirs']

const selected = computed(() => conflicts.value.find((c) => c.id === selectedId.value) || null)
const isStage = (c) => c?.entity === 'milestone'
const stageSel = computed(() => isStage(selected.value))
const index = computed(() => conflicts.value.findIndex((c) => c.id === selectedId.value))
const fields = computed(() => selected.value?.fields || [])

const fieldLabel = (f) => (FIELD_KEYS.includes(f) ? t(`gitlab.conflicts.field.${f}`) : f)
const kindLabel = (k) => (KIND_KEYS.includes(k) ? t(`gitlab.conflicts.kind.${k}`) : k)
const emptyVal = (v) => v === '' || v == null
const isText = (field) => field === 'title' || field === 'description'
function displayVal(field, v) {
  if (emptyVal(v)) return t('gitlab.conflicts.emptyValue')
  if (field === 'state' && stageSel.value)
    return t(v === 'closed' ? 'milestones.state.closed' : 'milestones.state.active')
  if (field === 'state')
    return t(v === 'closed' ? 'gitlab.conflicts.state.closed' : 'gitlab.conflicts.state.open')
  if (field === 'priority') return priorityLabel(v) || v
  return v
}
const norm = (v) => (v == null ? '' : String(v))

// The side taken without asking: a field changed on one side only, or changed
// the same way on both. null — both sides moved apart, the reader decides.
function autoPick(f) {
  const b = norm(f.base)
  const o = norm(f.ours)
  const g = norm(f.theirs)
  if (o === g) return 'ours'
  if (o === b) return 'theirs'
  if (g === b) return 'ours'
  return null
}
const draft = computed(() => (selected.value ? store.draftFor(selected.value.id) : {}))
function pickOf(f) {
  return draft.value[f.field]?.pick || autoPick(f)
}
const isAuto = (f) => !draft.value[f.field]?.pick && !!autoPick(f)
const valueOf = (f, side) => (side === 'manual' ? (draft.value[f.field]?.text ?? '') : f[side])
const decided = computed(() => fields.value.filter((f) => pickOf(f)).length)
const ready = computed(() => fields.value.length > 0 && decided.value === fields.value.length)
const leftOf = (c) => {
  const d = store.draftFor(c.id)
  return (c.fields || []).filter((f) => !(d[f.field]?.pick || autoPick(f))).length
}
// The task conflicts «Применить» sends together (#3089): the one on screen and
// every other fully decided one not put off. A stage's conflict goes alone — it
// has its own endpoint, which writes to the GitLab milestone at once.
const batch = computed(() => {
  const c = selected.value
  if (!c || isStage(c)) return c ? [c] : []
  return conflicts.value.filter(
    (x) =>
      !isStage(x) &&
      (x.id === c.id ||
        (!store.isPostponed(x.id) && (x.fields || []).length > 0 && leftOf(x) === 0)),
  )
})

function choose(f, side) {
  if (!selected.value) return
  const d = store.draftFor(selected.value.id)
  if (side === 'manual') {
    const seed = valueOf(f, pickOf(f) && pickOf(f) !== 'manual' ? pickOf(f) : 'theirs')
    d[f.field] = { pick: 'manual', text: d[f.field]?.text ?? norm(seed) }
    nextTick(() => manualEls[f.field]?.focus())
    return
  }
  d[f.field] = { pick: side }
}
function setManual(f, text) {
  const d = store.draftFor(selected.value.id)
  d[f.field] = { pick: 'manual', text }
}

// The server's three resolutions, from the per-field picks: a field whose two
// sides agree accepts either name.
function resolutionOf(c) {
  const d = store.draftFor(c.id)
  const picks = (c.fields || []).map((f) => ({ f, pick: d[f.field]?.pick || autoPick(f) }))
  const accepts = (p, side) =>
    p.pick === side ||
    (p.pick && p.pick !== 'manual' && norm(valueOf(p.f, p.pick)) === norm(p.f[side]))
  if (picks.every((p) => accepts(p, 'theirs'))) return { resolution: 'theirs' }
  if (picks.every((p) => accepts(p, 'ours'))) return { resolution: 'ours' }
  const value = {}
  for (const p of picks) value[p.f.field] = norm(valueOf(p.f, p.pick))
  return { resolution: 'manual', value }
}

async function load() {
  if (!props.wsId) return
  loading.value = true
  try {
    const { data } = await glApi.conflicts(props.wsId)
    // Put-off tasks (#3089) wait at the end of the queue.
    const list = data || []
    conflicts.value = [
      ...list.filter((c) => !store.isPostponed(c.id)),
      ...list.filter((c) => store.isPostponed(c.id)),
    ]
    // Pre-select the focused task's conflict (opened from a card), else keep
    // the current one if still there, else the first.
    const focused =
      (props.focusTaskId && conflicts.value.find((c) => c.task_id === props.focusTaskId)) ||
      (props.focusMilestoneId &&
        conflicts.value.find((c) => isStage(c) && c.milestone_id === props.focusMilestoneId))
    if (focused) selectedId.value = focused.id
    else if (!conflicts.value.find((c) => c.id === selectedId.value))
      selectedId.value = conflicts.value[0]?.id || null
    fieldCursor.value = 0
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

async function apply() {
  const c = selected.value
  if (!c || !ready.value || resolving.value) return
  const done = batch.value
  resolving.value = true
  try {
    if (isStage(c)) {
      const { data } = await msApi.resolveConflict(c.milestone_id, resolutionOf(c))
      // The decision went to GitLab in the same request; a refusal leaves the
      // stage «not synced» with its own retry.
      if (data?.gl_sync?.status === 'failed')
        message.warning(t('milestones.gitlab.failedToast', { error: data.gl_sync.error || '' }))
    } else if (done.length === 1) {
      await glApi.resolveConflict(c.task_id, c.id, resolutionOf(c))
    } else {
      await glApi.resolveConflicts(
        props.wsId,
        done.map((x) => ({ task_id: x.task_id, conflict_id: x.id, ...resolutionOf(x) })),
      )
    }
    message.success(
      done.length > 1
        ? t('gitlab.conflicts.resolvedN', done.length)
        : t('gitlab.conflicts.resolved'),
    )
    const gone = new Set(done.map((x) => x.id))
    for (const x of done) store.dropDraft(x.id)
    const at = index.value
    conflicts.value = conflicts.value.filter((x) => !gone.has(x.id))
    selectedId.value = conflicts.value[Math.min(at, conflicts.value.length - 1)]?.id || null
    fieldCursor.value = 0
    for (const x of done) emit('resolved', x)
    if (!conflicts.value.length) emit('update:show', false)
  } catch (e) {
    message.error(e.message)
  } finally {
    resolving.value = false
  }
}

// «Отложить» (#3089): keep the choices as a draft (across reloads too), send the
// task to the end of the queue and go on with the next one.
function postpone() {
  const c = selected.value
  if (!c) return
  store.postpone(c.id)
  const rest = conflicts.value.filter((x) => x.id !== c.id)
  conflicts.value = [...rest, c]
  if (rest.length) pick(rest[Math.min(Math.max(index.value, 0), rest.length - 1)])
}

function pick(c) {
  selectedId.value = c.id
  fieldCursor.value = 0
}
// Choosing a version on a put-off task brings it back into the batch.
watch(
  () => [selectedId.value, selected.value ? JSON.stringify(store.draftFor(selected.value.id)) : ''],
  ([id, now], [beforeId, before]) => {
    if (id && id === beforeId && now !== before) store.resume(id)
  },
)
function step(delta) {
  const list = conflicts.value
  if (!list.length) return
  const i = (index.value + delta + list.length) % list.length
  pick(list[i])
}

// Leave for the task (or the stage's page): close (its history entry unwinds
// first), then go.
async function openTask() {
  const c = selected.value
  if (!c) return
  let target
  if (isStage(c)) {
    if (!c.project_slug || !c.milestone_slug) return
    target = `/project/${c.project_slug}/stage/${c.milestone_slug}`
  } else {
    let boardId
    try {
      const { data } = await tasksApi.get(c.task_id)
      boardId = data?.board_id
    } catch (e) {
      message.error(e.message)
      return
    }
    if (!boardId) return
    target = `/board/${boardId}?task=${c.task_number ?? c.task_id}`
  }
  const ours = !!window.history.state?.tOverlay
  emit('update:show', false)
  if (ours) {
    await new Promise((resolve) => {
      const done = () => {
        window.removeEventListener('popstate', done)
        clearTimeout(timer)
        resolve()
      }
      const timer = setTimeout(done, 400)
      window.addEventListener('popstate', done)
    })
  }
  router.push(target)
}

// ── keys: ↑↓ the field, B / T / G its version, M merge by hand ──
const fieldCursor = ref(0)
// The focus ring of a field row is a keyboard state: drawn after a key, hidden
// after the pointer and on a phone.
const kbd = ref(false)
const manualEls = {}
const paneEl = ref(null)
function onPaneKey(e) {
  if (e.isComposing || e.target?.tagName === 'TEXTAREA' || e.target?.tagName === 'INPUT') return
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    kbd.value = true
    const n = fields.value.length
    if (n) fieldCursor.value = (fieldCursor.value + (e.key === 'ArrowDown' ? 1 : -1) + n) % n
  }
}
function keyPick(side) {
  const f = fields.value[fieldCursor.value]
  if (!f) return
  if (side === 'manual' && !MERGEABLE.includes(f.field)) return
  kbd.value = true
  choose(f, side)
}
function paneEscape() {
  const el = document.activeElement
  if (el && el.tagName === 'TEXTAREA' && paneEl.value?.contains(el)) {
    el.blur()
    paneEl.value?.focus()
    return true
  }
  return false
}
const topmost = () => !!winRef.value?.topmost()
useShortcuts(
  {
    'conflict.base': () => keyPick('base'),
    'conflict.ours': () => keyPick('ours'),
    'conflict.theirs': () => keyPick('theirs'),
    'conflict.merge': () => keyPick('manual'),
    'conflict.prev': () => step(-1),
    'conflict.next': () => step(1),
    'conflict.apply': () => apply(),
  },
  { when: topmost },
)

watch(
  () => [props.show, props.wsId, props.focusTaskId, props.focusMilestoneId],
  ([show]) => {
    if (show) load()
  },
  { immediate: true },
)
function onOpened() {
  if (!isMobile.value) paneEl.value?.focus({ preventScroll: true })
}

const sideCap = { base: 'B', ours: 'T', theirs: 'G' }
const sideName = computed(() => ({
  base: t('gitlab.conflicts.colBaseShort'),
  ours: 'Tessera',
  theirs: 'GitLab',
}))
const detectedText = computed(() =>
  selected.value?.detected_at
    ? formatDateTime(selected.value.detected_at, { day: '2-digit', month: 'short' })
    : '',
)
</script>

<template>
  <LargeWindow
    ref="winRef"
    :show="show"
    :crumbs="[workspaceName(ws.current), 'GitLab']"
    :title="t('gitlab.conflicts.windowTitle')"
    :icon="GitMergeOutline"
    tone="#fc6d26"
    :items="[]"
    :escape="paneEscape"
    testid="conflicts-window"
    @update:show="emit('update:show', $event)"
    @opened="onOpened"
  >
    <template #aside>
      <span v-if="conflicts.length > 1 && !isMobile" class="cr-aside">
        {{ t('gitlab.conflicts.position', { i: index + 1, n: conflicts.length }) }}
        <ShortcutKeys id="conflict.task" bare />
      </span>
    </template>
    <template #nav-top>
      <div class="cr-qh">{{ t('gitlab.conflicts.queue') }}</div>
      <button
        v-for="c in conflicts"
        :key="c.id"
        type="button"
        class="cr-q"
        :class="{ on: c.id === selectedId }"
        data-testid="conflict-item"
        @click="pick(c)"
      >
        <span class="cr-q-t">
          <template v-if="isStage(c)">
            <n-icon :component="TrailSignOutline" :size="13" class="cr-stage-ic" />
            {{ c.milestone_title || t('gitlab.conflicts.stageWord') }}
          </template>
          <template v-else>
            <span v-if="c.task_number" class="cr-num">#{{ c.task_number }}</span>
            {{ c.task_title || t('gitlab.conflicts.taskFallback') }}
          </template>
        </span>
        <span class="cr-q-s">
          <span class="cr-dot" :class="{ done: !leftOf(c) }" />
          {{ leftOf(c) ? t('gitlab.conflicts.left', leftOf(c)) : t('gitlab.conflicts.readyShort') }}
          · {{ kindLabel(c.change_kind) }}
          <template v-if="store.isPostponed(c.id)">
            · {{ t('gitlab.conflicts.postponed') }}</template
          >
        </span>
      </button>
    </template>
    <template #nav-foot>
      <div v-if="selected?.gl_iid" class="lw-foot-row">
        <n-icon :component="ArrowUpOutline" :size="12" />
        {{
          t(stageSel ? 'gitlab.conflicts.fromMilestone' : 'gitlab.conflicts.fromPush', {
            iid: selected.gl_iid,
            at: detectedText,
          })
        }}
      </div>
      <div class="lw-foot-row">
        {{ t(stageSel ? 'gitlab.conflicts.onePushStage' : 'gitlab.conflicts.onePush') }}
      </div>
    </template>

    <div
      ref="paneEl"
      class="cr cp-wrap"
      tabindex="-1"
      data-testid="conflicts-pane"
      @keydown="onPaneKey"
      @pointerdown="kbd = false"
    >
      <EmptyState
        v-if="!loading && !conflicts.length"
        :icon="LogoGitlab"
        tone="#fc6d26"
        :text="t('gitlab.conflicts.empty')"
      />
      <template v-else-if="selected">
        <div class="cr-title">
          <h2 v-if="stageSel">
            <span class="cr-num">{{ t('gitlab.conflicts.stageWord') }}</span>
            {{ selected.milestone_title }}
          </h2>
          <h2 v-else>
            <span v-if="selected.task_number" class="cr-num">#{{ selected.task_number }}</span>
            {{ selected.task_title || t('gitlab.conflicts.taskFallback') }}
          </h2>
          <span class="cr-grow" />
          <button type="button" class="kit-btn" data-testid="conflict-open-task" @click="openTask">
            <n-icon :component="OpenOutline" :size="15" />
            {{ stageSel ? t('gitlab.conflicts.openStage') : t('gitlab.conflicts.openTask') }}
          </button>
        </div>
        <p class="cr-hint">
          {{ stageSel ? t('gitlab.conflicts.hintStage') : t('gitlab.conflicts.hintV2') }}
        </p>

        <div class="cr-scroll t-hoverscroll">
          <div class="cr-grid cr-head">
            <span>{{ t('gitlab.conflicts.colField') }}</span>
            <span>{{ t('gitlab.conflicts.colBaseShort') }}</span>
            <span class="cr-src ours">Tessera</span>
            <span class="cr-src gl">GitLab</span>
          </div>

          <div
            v-for="(f, fi) in fields"
            :key="f.field"
            class="cr-field c-field"
            :class="{ cursor: kbd && fi === fieldCursor, auto: isAuto(f) }"
            data-testid="conflict-field"
            @click="fieldCursor = fi"
          >
            <div class="cr-grid">
              <div class="cr-fname">
                <span class="cr-fl">{{ fieldLabel(f.field) }}</span>
                <span class="cr-fsub">
                  <template v-if="isAuto(f)">
                    <n-icon :component="CheckmarkOutline" :size="11" />
                    {{
                      t('gitlab.conflicts.autoSide', {
                        side: pickOf(f) === 'ours' ? 'Tessera' : 'GitLab',
                      })
                    }}
                  </template>
                  <template v-else-if="!pickOf(f)">
                    {{ t('gitlab.conflicts.choose') }}
                    <Kbd v-if="MERGEABLE.includes(f.field) && !isMobile" soft>{{ cap('m') }}</Kbd>
                  </template>
                  <template v-else-if="pickOf(f) === 'manual'">
                    {{ t('gitlab.conflicts.manualPicked') }}
                  </template>
                </span>
              </div>
              <button
                v-for="side in SIDES"
                :key="side"
                type="button"
                class="cr-cell"
                :class="[side, { on: pickOf(f) === side, empty: emptyVal(f[side]) }]"
                :data-testid="`conflict-pick-${side}`"
                @click.stop="((fieldCursor = fi), choose(f, side))"
              >
                <span v-if="isMobile" class="cr-side">{{ sideName[side] }}</span>
                <span class="cr-val">
                  <template v-if="emptyVal(f[side])">{{
                    t('gitlab.conflicts.emptyValue')
                  }}</template>
                  <template v-else-if="isText(f.field) && side !== 'base'"
                    ><span
                      v-for="(seg, si) in diffSegments(f.base, f[side])"
                      :key="si"
                      :class="{ [`ch-${side}`]: seg.changed }"
                      >{{ seg.text }}</span
                    ></template
                  >
                  <template v-else>{{ displayVal(f.field, f[side]) }}</template>
                </span>
                <n-icon
                  v-if="pickOf(f) === side && !isAuto(f)"
                  :component="CheckmarkOutline"
                  :size="12"
                  class="cr-check"
                />
                <Kbd v-else-if="!isMobile" soft class="cr-cap">{{ sideCap[side] }}</Kbd>
              </button>
            </div>
            <div v-if="pickOf(f) === 'manual'" class="cr-manual c-manual">
              <span class="cr-fl">{{ t('gitlab.conflicts.merged') }}</span>
              <textarea
                :ref="(el) => (manualEls[f.field] = el)"
                class="kf-input kf-area"
                :rows="f.field === 'description' ? 5 : 1"
                :value="draft[f.field]?.text ?? ''"
                :placeholder="t('gitlab.conflicts.mergedPlaceholder')"
                data-testid="conflict-manual"
                @input="setManual(f, $event.target.value)"
              />
            </div>
          </div>
        </div>

        <div class="cr-foot">
          <span class="cr-progress"
            ><span
              class="cr-progress-bar"
              :style="{ width: `${fields.length ? (decided / fields.length) * 100 : 0}%` }"
          /></span>
          <span class="cr-count">{{
            t('gitlab.conflicts.decided', { done: decided, n: fields.length })
          }}</span>
          <span class="cr-grow" />
          <span v-if="!isMobile" class="cr-keys" :title="t('gitlab.conflicts.keyFieldHint')">
            <Kbd>{{ sideCap.base }}</Kbd
            ><Kbd>{{ sideCap.ours }}</Kbd
            ><Kbd>{{ sideCap.theirs }}</Kbd> {{ t('gitlab.conflicts.keyVersion') }}
          </span>
          <button
            v-if="conflicts.length > 1 && !stageSel"
            type="button"
            class="kit-btn"
            :title="t('gitlab.conflicts.postponeHint')"
            data-testid="conflict-postpone"
            @click="postpone"
          >
            {{ t('gitlab.conflicts.postpone') }}
          </button>
          <button
            type="button"
            class="kit-btn primary"
            :disabled="!ready || resolving"
            data-testid="conflict-apply"
            @click="apply"
          >
            <n-icon :component="CloudUploadOutline" :size="15" />
            {{
              batch.length > 1
                ? t('gitlab.conflicts.applyN', { n: batch.length })
                : t('gitlab.conflicts.apply')
            }}
            <ShortcutKeys v-if="!isMobile" id="conflict.apply" joined />
          </button>
        </div>
      </template>
      <LoaderOverlay :show="loading" contained :messages="[t('gitlab.conflicts.loading')]" />
    </div>
  </LargeWindow>
</template>

<style scoped>
.cr {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  padding: 6px 16px 0 0;
  outline: 0;
}
.lw-mobile .cr {
  padding-right: 0;
}
.cr-aside {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
/* The queue in the sunken column. */
.cr-stage-ic {
  vertical-align: -2px;
  margin-right: 2px;
  color: var(--t-primary);
}
.cr-qh {
  margin: 8px 10px 6px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.lw-mobile .cr-qh {
  display: none;
}
.cr-q {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 7px 10px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  text-align: left;
  cursor: pointer;
  flex: none;
  min-width: 0;
}
.lw-mobile .cr-q {
  max-width: 220px;
}
.cr-q:hover {
  background: var(--t-hover);
}
.cr-q.on {
  background: var(--t-active);
}
.cr-q:focus-visible {
  outline: 0;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.cr-q-t {
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cr-num {
  font-weight: 500;
  color: var(--t-text3);
  margin-right: 4px;
}
.cr-q-s {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
}
.cr-dot {
  flex: none;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #f0a020;
}
.cr-dot.done {
  background: #2fa86b;
}
.cr-title {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 36px;
}
.cr-title h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  min-width: 0;
}
.cr-grow {
  flex: 1;
}
.cr-hint {
  margin: 6px 0 14px;
  font-size: 13px;
  color: var(--t-text2);
}
.cr-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 2px 12px;
}
/* A field row: 112 + three columns; cells on --t-sub, radius 10. */
.cr-grid {
  display: grid;
  grid-template-columns: 112px repeat(3, minmax(0, 1fr));
  gap: 8px;
  align-items: stretch;
}
.cr-head {
  padding: 0 8px 8px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.cr-src::before {
  content: '';
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-right: 6px;
  border-radius: 3px;
  vertical-align: 0;
}
.cr-src.ours::before {
  background: var(--t-accent-grad);
}
.cr-src.gl::before {
  background: #fc6d26;
}
.cr-field {
  padding: 6px 8px;
  border-radius: 14px;
}
.cr-field + .cr-field {
  margin-top: 4px;
}
.cr-field.cursor {
  background: var(--t-hover);
  box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--t-primary) 45%, transparent);
}
.cr-fname {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 3px;
  min-width: 0;
}
.cr-fl {
  font-size: 13px;
  font-weight: 650;
}
.cr-fsub {
  display: inline-flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  font-size: 11.5px;
  color: var(--t-text3);
}
.cr-cell {
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: 6px;
  min-height: 44px;
  padding: 10px 30px 10px 12px;
  border: 0;
  border-radius: 10px;
  background: var(--t-sub);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
  min-width: 0;
}
.cr-cell:hover {
  background: var(--t-hover);
}
.cr-cell.empty {
  color: var(--t-text3);
  font-style: italic;
}
.cr-cell.on {
  background: color-mix(in srgb, var(--t-primary) 20%, var(--t-sub));
  box-shadow: 0 0 0 2px var(--t-primary);
}
/* An automatic pick: the same tone, weaker, without the ring. */
.cr-field.auto .cr-cell.on {
  background: color-mix(in srgb, var(--t-primary) 10%, var(--t-sub));
  box-shadow: none;
}
.cr-field.auto .cr-cell:not(.on) {
  opacity: 0.6;
}
.cr-cell:focus-visible {
  outline: 0;
  box-shadow: 0 0 0 2px var(--t-primary);
}
.cr-val {
  min-width: 0;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 160px;
  overflow: hidden;
  line-height: 1.45;
}
.cr-check {
  position: absolute;
  top: 8px;
  right: 8px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: #fff;
}
.cr-cap {
  position: absolute;
  top: 8px;
  right: 8px;
}
/* Inline diff: tint only the spans that diverged from the base. */
.ch-ours {
  background: color-mix(in srgb, var(--t-primary) 30%, transparent);
  border-radius: 3px;
}
.ch-theirs {
  background: color-mix(in srgb, #fc6d26 30%, transparent);
  border-radius: 3px;
}
.cr-manual {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 8px 0 2px 120px;
}
.lw-mobile .cr-manual {
  margin-left: 0;
}
.cr-foot {
  flex: none;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  padding: 10px 0 4px;
}
.cr-progress {
  width: 96px;
  height: 6px;
  border-radius: 3px;
  background: var(--t-pill);
  overflow: hidden;
}
.cr-progress-bar {
  display: block;
  height: 100%;
  border-radius: 3px;
  background: var(--t-accent-grad);
  transition: width 0.2s ease;
}
.cr-count {
  font-size: 12.5px;
  color: var(--t-text2);
}
.cr-keys {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--t-text3);
}
.kit-btn.stub {
  opacity: 0.55;
  cursor: default;
}
/* Phone: the field name over its three versions, stacked, each named. */
.lw-mobile .cr-grid {
  grid-template-columns: minmax(0, 1fr);
}
.lw-mobile .cr-head {
  display: none;
}
.cr-side {
  flex: none;
  min-width: 58px;
  font-size: 11px;
  font-weight: 650;
  font-style: normal;
  color: var(--t-text3);
}
</style>
