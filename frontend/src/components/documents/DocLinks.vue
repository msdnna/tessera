<script setup>
import { computed, ref } from 'vue'
import { NButton, NIcon, NInput, NPopconfirm, NPopover, NSelect, NText } from 'naive-ui'
import { CheckmarkCircleOutline, CloseOutline, LinkOutline } from '@vicons/ionicons5'
import { workspaces as wsApi } from '@/api'
import { useFormat } from '@/composables/useFormat'
import {
  APPROVAL_STATUS_LABEL,
  STEP_STATUS_LABEL,
  approvalProgress,
  canDecideNow,
  orderedSteps,
  stepState,
} from '@/utils/docApprovals'

// Task links and approval protocols of the open document (#2732). Both belong in
// one panel: a protocol is raised against the document, and the tasks it came
// from are the reason it exists.
const props = defineProps({
  links: { type: Array, default: () => [] },
  approvals: { type: Array, default: () => [] },
  // Whose signature the panel may offer — the socket's welcome frame tells the
  // view who "me" is in this room.
  userId: { type: String, default: '' },
  wsId: { type: String, default: '' },
  canRaise: { type: Boolean, default: true },
  loading: { type: Boolean, default: false },
  error: { type: String, default: '' },
  // The block a new link would be pinned to, and the snippet captured for it.
  // Empty means the link is filed against the document as a whole.
  anchorBlockId: { type: String, default: '' },
  anchorQuote: { type: String, default: '' },
})
const emit = defineEmits(['link', 'unlink', 'raise', 'decide', 'cancel', 'close', 'open-task'])
const { formatDate } = useFormat()

// ── linking a task ──
const picking = ref(false)
const query = ref('')
const tasks = ref([])

async function ensureTasks() {
  if (tasks.value.length || !props.wsId) return
  try {
    const res = await wsApi.tasks(props.wsId, { include_subtasks: 1 })
    tasks.value = res.data || []
  } catch {
    // Non-fatal: the panel still lists what is linked, it just cannot offer the
    // picker. An error toast here would fire on every document opened offline.
  }
}

function openPicker() {
  picking.value = true
  ensureTasks()
}

// Already-linked tasks are dropped from the picker rather than shown and
// refused: re-linking the same pair is a no-op server-side, so offering it would
// be a button that appears to do nothing.
const linkedIds = computed(() => new Set(props.links.map((l) => l.task_id)))
const candidates = computed(() => {
  const q = query.value.trim().toLowerCase()
  return tasks.value
    .filter((t) => t.number != null && !linkedIds.value.has(t.id))
    .filter((t) => !q || `#${t.number}`.includes(q) || (t.title || '').toLowerCase().includes(q))
    .slice(0, 50)
})

function chooseTask(t) {
  picking.value = false
  query.value = ''
  emit('link', { taskId: t.id, blockId: props.anchorBlockId, quote: props.anchorQuote })
}

// ── raising a route ──
const raising = ref(false)
const routeTitle = ref('')
const routeMode = ref('sequential')
const routeApprovers = ref([])
const members = ref([])

const modeOptions = [
  { label: 'По очереди', value: 'sequential' },
  { label: 'Одновременно', value: 'parallel' },
]

const memberOptions = computed(() =>
  members.value.map((m) => ({ label: m.name || m.email, value: m.user_id })),
)

async function openRaise() {
  raising.value = true
  if (members.value.length || !props.wsId) return
  try {
    const res = await wsApi.members(props.wsId)
    members.value = res.data || []
  } catch {
    // Same reasoning as the task picker: the panel degrades to read-only rather
    // than shouting about a list it only needs when composing a route.
  }
}

function submitRoute() {
  if (!routeApprovers.value.length) return
  emit('raise', {
    title: routeTitle.value,
    mode: routeMode.value,
    approvers: routeApprovers.value,
  })
  routeTitle.value = ''
  routeApprovers.value = []
  raising.value = false
}

// ── signing ──
// One draft remark shared by the route being decided: only one signature can be
// in flight at a time, and a per-step draft would survive a refetch it no longer
// matches.
const decidingId = ref('')
const decisionComment = ref('')

function startDecide(id) {
  decidingId.value = decidingId.value === id ? '' : id
  decisionComment.value = ''
}

function submitDecision(approval, decision) {
  emit('decide', { id: approval.id, decision, comment: decisionComment.value })
  decidingId.value = ''
  decisionComment.value = ''
}

function mayDecide(approval) {
  return canDecideNow(approval, props.userId)
}

function progressLabel(approval) {
  const { signed, total } = approvalProgress(approval)
  return `${signed} из ${total}`
}

function fmtDate(v) {
  if (!v) return ''
  return formatDate(v, { day: 'numeric', month: 'short' })
}
</script>

<template>
  <aside class="doc-links" data-testid="doc-links">
    <div class="panel-head">
      <n-icon :component="LinkOutline" :size="16" />
      <span class="panel-title">Связи и согласование</span>
      <n-text v-if="loading" depth="3">…</n-text>
      <span class="grow" />
      <n-button quaternary size="tiny" @click="emit('close')">Закрыть</n-button>
    </div>

    <n-text v-if="error" type="error" class="empty">{{ error }}</n-text>

    <div class="panel-body">
      <n-text depth="3" class="section-title">Задачи</n-text>
      <p v-if="!links.length && !loading" class="empty">
        Связанных задач нет. Связь можно поставить на весь документ или на выбранный блок.
      </p>

      <div v-for="l in links" :key="l.id" class="link" data-testid="doc-link">
        <button type="button" class="link-main" @click="emit('open-task', l)">
          <span class="link-title">{{ l.task_title || 'Задача' }}</span>
          <!-- The quote is what says *which* clause the task hangs on once that
               clause has been rewritten; without it an anchored link degrades to
               a link on the document. -->
          <span v-if="l.block_id" class="anchor" :title="l.quote">
            {{ l.quote || 'фрагмент документа' }}
          </span>
        </button>
        <n-popconfirm
          :positive-button-props="{ type: 'error' }"
          positive-text="Убрать"
          @positive-click="emit('unlink', l.id)"
        >
          <template #trigger>
            <button class="act" title="Убрать связь">
              <n-icon :component="CloseOutline" />
            </button>
          </template>
          Убрать связь с задачей?
        </n-popconfirm>
      </div>

      <n-popover
        trigger="manual"
        :show="picking"
        placement="bottom-start"
        :width="300"
        @clickoutside="picking = false"
      >
        <template #trigger>
          <n-button size="tiny" block data-testid="doc-link-add" @click="openPicker">
            <template #icon><n-icon :component="LinkOutline" /></template>
            {{ anchorBlockId ? 'Связать блок с задачей' : 'Связать задачу' }}
          </n-button>
        </template>
        <div class="picker">
          <n-input
            v-model:value="query"
            size="tiny"
            placeholder="№ или название"
            data-testid="doc-link-query"
          />
          <p v-if="!candidates.length" class="empty">Ничего не найдено</p>
          <button
            v-for="t in candidates"
            :key="t.id"
            type="button"
            class="pick"
            @click="chooseTask(t)"
          >
            <span class="pick-num">#{{ t.number }}</span>
            <span class="pick-title">{{ t.title }}</span>
          </button>
        </div>
      </n-popover>

      <n-text depth="3" class="section-title">Протоколы согласования</n-text>
      <p v-if="!approvals.length && !loading" class="empty">
        Документ на согласование не отправлялся.
      </p>

      <!-- Newest first, as the server returns them: the open route is the one
           being asked about, and closed ones are the journal behind it. -->
      <div
        v-for="a in approvals"
        :key="a.id"
        class="approval"
        :class="a.status"
        data-testid="doc-approval"
      >
        <div class="approval-head">
          <span class="status">{{ APPROVAL_STATUS_LABEL[a.status] || a.status }}</span>
          <span class="progress">{{ progressLabel(a) }}</span>
        </div>
        <span v-if="a.title" class="approval-title">{{ a.title }}</span>
        <!-- Which text is being agreed. A protocol that cannot name its revision
             is a signature on a moving target. -->
        <span class="meta">
          Версия {{ a.version_revision }} · {{ a.created_by_name || 'неизвестно' }} ·
          {{ fmtDate(a.created_at) }}
        </span>

        <div class="steps">
          <div v-for="s in orderedSteps(a)" :key="s.id" class="step" :class="stepState(a, s)">
            <n-icon v-if="s.status === 'approved'" :component="CheckmarkCircleOutline" :size="13" />
            <span class="step-name">{{ s.approver_name }}</span>
            <span class="step-status">{{ STEP_STATUS_LABEL[s.status] || s.status }}</span>
            <span v-if="s.comment" class="step-comment">{{ s.comment }}</span>
          </div>
        </div>

        <div v-if="mayDecide(a)" class="decide">
          <n-button
            v-if="decidingId !== a.id"
            size="tiny"
            type="primary"
            data-testid="doc-approval-sign"
            @click="startDecide(a.id)"
          >
            Подписать
          </n-button>
          <template v-else>
            <n-input
              v-model:value="decisionComment"
              size="tiny"
              type="textarea"
              :rows="2"
              placeholder="Комментарий (необязательно)"
              data-testid="doc-approval-comment"
            />
            <div class="row">
              <n-button
                size="tiny"
                type="primary"
                data-testid="doc-approval-approve"
                @click="submitDecision(a, 'approved')"
              >
                Согласовать
              </n-button>
              <n-button size="tiny" type="error" ghost @click="submitDecision(a, 'rejected')">
                Отклонить
              </n-button>
              <n-button size="tiny" quaternary @click="startDecide(a.id)">Отмена</n-button>
            </div>
          </template>
        </div>

        <n-popconfirm v-if="a.status === 'pending'" @positive-click="emit('cancel', a.id)">
          <template #trigger>
            <n-button size="tiny" quaternary block>Отозвать</n-button>
          </template>
          Маршрут закроется, подписи останутся в протоколе.
        </n-popconfirm>
      </div>

      <div class="raise">
        <n-button
          v-if="!raising"
          size="tiny"
          block
          :disabled="!canRaise"
          :title="canRaise ? '' : 'Документ уже на согласовании'"
          data-testid="doc-approval-raise"
          @click="openRaise"
        >
          Отправить на согласование
        </n-button>
        <template v-else>
          <n-input
            v-model:value="routeTitle"
            size="tiny"
            placeholder="Что согласуем"
            data-testid="doc-approval-title"
          />
          <n-select v-model:value="routeMode" size="tiny" :options="modeOptions" />
          <!-- Order matters in a sequential route, so the picker keeps the order
               the names were added in — that list *is* the route. -->
          <n-select
            v-model:value="routeApprovers"
            multiple
            filterable
            size="tiny"
            placeholder="Согласующие"
            data-testid="doc-approval-approvers"
            :options="memberOptions"
          />
          <div class="row">
            <n-button
              size="tiny"
              type="primary"
              :disabled="!routeApprovers.length"
              data-testid="doc-approval-submit"
              @click="submitRoute"
            >
              Отправить
            </n-button>
            <n-button size="tiny" quaternary @click="raising = false">Отмена</n-button>
          </div>
        </template>
      </div>
    </div>
  </aside>
</template>

<style scoped>
/* Content of the sidebar since task 2738, not a column of its own — see the
   same note in DocHistory.vue. Width, border and overlay live in `.side`.
   (Task numbers are spelled without the hash inside style blocks: the theme
   guard in cx-doc-editor.spec.js reads them as literal colours.) */
.doc-links {
  display: flex;
  flex-direction: column;
  width: 100%;
  flex: 1;
  min-height: 0;
  gap: 8px;
}
.panel-head {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--t-text2);
}
.panel-title {
  font-size: 13px;
  font-weight: 600;
}
.grow {
  flex: 1;
}
.panel-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.section-title {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  margin-top: 4px;
}
.link {
  display: flex;
  align-items: flex-start;
  gap: 4px;
}
.link-main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
  border: none;
  background: none;
  text-align: left;
  cursor: pointer;
  padding: 4px 6px;
  border-radius: 6px;
  font: inherit;
  color: inherit;
}
.link-main:hover {
  background: var(--t-hover);
}
.link-title {
  font-size: 12px;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.anchor {
  font-size: 11px;
  color: var(--t-text3);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 1;
  line-clamp: 1;
  -webkit-box-orient: vertical;
}
.act {
  flex: none;
  border: none;
  background: none;
  cursor: pointer;
  color: var(--t-text3);
  padding: 4px;
  border-radius: 6px;
}
.act:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.picker {
  max-height: 280px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.pick {
  display: flex;
  align-items: baseline;
  gap: 6px;
  width: 100%;
  text-align: left;
  border: none;
  background: transparent;
  border-radius: 6px;
  padding: 5px 6px;
  cursor: pointer;
  font: inherit;
  color: inherit;
}
.pick:hover {
  background: var(--t-hover);
}
.pick-num {
  flex: none;
  font-size: 11px;
  color: var(--t-text3);
}
.pick-title {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.approval {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
}
/* The open route takes the accent; closed ones stay neutral — the panel is read
   to find out what is being asked of you now. */
.approval.pending {
  border-color: transparent;
  background:
    linear-gradient(var(--t-surface), var(--t-surface)) padding-box,
    var(--t-accent-grad) border-box;
  border: 1px solid transparent;
}
.approval-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 6px;
  font-size: 11px;
}
.status {
  font-weight: 600;
  color: var(--t-text2);
}
.approval.approved .status {
  color: var(--t-primary);
}
/* Rejection is the one outcome that has to be unmissable, and the project's
   channel for that is the error gradient on the text itself — not a hardcoded
   red, which cannot follow the theme. */
.approval.rejected .status {
  background-image: var(--t-error-grad);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.progress,
.meta {
  color: var(--t-text3);
  font-size: 11px;
}
.approval-title {
  font-size: 12px;
  color: var(--t-text1);
}
.steps {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 2px;
}
/* Every step states its outcome in words — "подписал" / "отклонил" / "ждёт" —
   and the border only reinforces it. Nothing here depends on telling one hue
   from another, which is what the two commonest forms of colour blindness take
   away. */
.step {
  display: flex;
  align-items: baseline;
  gap: 4px;
  flex-wrap: wrap;
  font-size: 11px;
  padding-left: 6px;
  border-left: 2px solid var(--t-border);
}
.step.signed,
.step.current {
  border-left-color: var(--t-primary);
}
/* A gradient cannot be a border-color, so the border is made transparent and
   painted through border-box — the same trick the card above uses, and the one
   the project uses wherever a gradient has to reach a border. */
.step.rejected {
  border-left-color: transparent;
  background:
    linear-gradient(var(--t-surface), var(--t-surface)) padding-box,
    var(--t-error-grad) border-box;
}
.step-name {
  color: var(--t-text1);
}
.step.waiting .step-name {
  color: var(--t-text3);
}
.step.signed .step-status {
  color: var(--t-primary);
}
.step-status,
.step-comment {
  color: var(--t-text3);
}
.step-comment {
  width: 100%;
  word-break: break-word;
}
.decide,
.raise {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 4px;
}
.row {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.empty {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--t-text3);
}
</style>
