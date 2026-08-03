<script setup>
import { ref, watch, computed, h } from 'vue'
import {
  NModal,
  NCard,
  NInput,
  NSelect,
  NSwitch,
  NButton,
  NButtonGroup,
  NBadge,
  NDropdown,
  NText,
  NIcon,
  NPopconfirm,
  useMessage,
} from 'naive-ui'
import {
  TrashOutline,
  AddOutline,
  SyncOutline,
  CloudDownloadOutline,
  LogoGitlab,
  ChevronDownOutline,
  TimeOutline,
  WarningOutline,
  CreateOutline,
  CloseOutline,
} from '@vicons/ionicons5'
import { gitlab as glApi, projects as projApi, boards as boardsApi } from '@/api'
import { canonPrefix } from '@/utils/tagGroups'
import GitLabJournalPanel from '@/components/GitLabJournalPanel.vue'
import ConflictResolverPanel from '@/components/ConflictResolverPanel.vue'
import { useGitlabStore } from '@/stores/gitlab'
import { useWorkspacesStore } from '@/stores/workspaces'
import { PRIORITY_LABELS } from '@/styles/tokens'

const props = defineProps({
  show: { type: Boolean, default: false },
  wsId: { type: String, default: null },
})
// No 'synced' emit any more — a background sync reports itself over the workspace
// socket (`integration.sync`), which every client picks up, not just the starter.
const emit = defineEmits(['update:show'])

const message = useMessage()
const gl = useGitlabStore()
const ws = useWorkspacesStore()

// ── connection (per-user) ──
const baseInput = ref('')
const tokenInput = ref('')
const connecting = ref(false)

async function connect() {
  const b = baseInput.value.trim()
  const t = tokenInput.value.trim()
  if (!b || !t) return
  connecting.value = true
  try {
    await gl.connect(b, t)
    tokenInput.value = ''
    message.success(`GitLab подключён как @${gl.username}`)
  } catch (e) {
    message.error(e.message)
  } finally {
    connecting.value = false
  }
}
async function disconnect() {
  try {
    await gl.disconnect()
    message.success('GitLab отключён')
  } catch (e) {
    message.error(e.message)
  }
}

// ── integration bindings (per-workspace, multi-binding) ──
// A workspace can mirror several GitLab projects, each into its own board. The
// form below edits the currently-selected binding; `currentId === null` means a
// new, not-yet-saved binding.
const integrations = ref([]) // list of binding views
const currentId = ref(null) // selected binding id, or null for a new one
const defaultRules = ref(null) // server-provided default rules for new bindings
// Instance-wide service token configured (admin) → bindings work without a personal
// PAT. isAdmin → the current user may create/edit/delete bindings.
const serviceConfigured = ref(false)
const isAdmin = ref(false)
const name = ref('')
const scope = ref('all') // 'all' | 'assigned'
const closedPolicy = ref('archive_closed_sprints') // 'all' | 'archive_closed_sprints' | 'period'
const closedAfter = ref(null) // epoch ms for 'period'
const scopeOptions = [
  { label: 'Все задачи проекта', value: 'all' },
  { label: 'Только назначенные на меня', value: 'assigned' },
]
const closedPolicyOptions = [
  { label: 'Закрытые из закрытых этапов — в архив', value: 'archive_closed_sprints' },
  { label: 'Все закрытые — на доску (в «Готово»)', value: 'all' },
  { label: 'Только закрытые за период', value: 'period' },
]
const projectPath = ref('')
const boardId = ref(null)
const enabled = ref(true)
const intervalSec = ref(0)
const fullIntervalSec = ref(86400)
const dueSource = ref('issue_milestone')
const startSource = ref('created')
const lastSynced = ref(null)
// ── write-back (Tessera → GitLab), opt-in; all off by default ──
const wbEnabled = ref(false) // master toggle for the whole binding table
const wbCreate = ref(false) // allow creating GitLab issues from tasks (independent of write-back)
const wbFetchTemplates = ref(false) // offer repo issue templates when creating
// bindings is the customizable trigger→action table (replaces the fixed toggles).
// Each entry: { enabled, trigger:{type,column_id,column_name,priority,completed,date_kind},
//               action:{type,label,clear_prefix,state,date_kind,add_marker} }
const bindings = ref([])
// The modal expands into a right pane for editing either the write-back action
// bindings or the GL→Tessera label-parsing rules. null = collapsed (single pane).
const rightMode = ref(null) // null | 'actions' | 'rules' | 'journal' | 'conflicts'
const RIGHT_TITLES = {
  actions: 'Действия обратной записи',
  rules: 'Правила разбора меток GitLab',
  journal: 'Журнал синхронизации',
  conflicts: 'Конфликты',
}
function openRight(mode) {
  rightMode.value = rightMode.value === mode ? null : mode
}

const triggerTypeOptions = [
  { label: 'Перемещение в колонку', value: 'column' },
  { label: 'Флаг «Выполнено»', value: 'completion' },
  { label: 'Изменение приоритета', value: 'priority' },
  { label: 'Изменение срока', value: 'due' },
  { label: 'Изменение исполнителей', value: 'assignees' },
  { label: 'Изменение оценки', value: 'estimate' },
  { label: 'Изменение этапа', value: 'milestone' },
  { label: 'Заголовок / описание', value: 'title_desc' },
  { label: 'Изменение тегов', value: 'labels' },
  { label: 'Новый комментарий', value: 'comment' },
]
const actionTypeOptions = [
  { label: 'Установить метку', value: 'set_label' },
  { label: 'Закрыть / открыть issue', value: 'set_state' },
  { label: 'Установить срок', value: 'set_due' },
  { label: 'Установить исполнителей', value: 'set_assignees' },
  { label: 'Установить оценку', value: 'set_estimate' },
  { label: 'Установить этап', value: 'set_milestone' },
  { label: 'Обновить заголовок/описание', value: 'set_title_desc' },
  { label: 'Синхронизировать теги', value: 'reconcile_labels' },
  { label: 'Написать комментарий', value: 'post_comment' },
]
const stateOptions = [
  { label: 'Из флага «Выполнено»', value: '' },
  { label: 'Закрыть issue', value: 'closed' },
  { label: 'Открыть issue', value: 'opened' },
]
const dateKindOptions = [
  { label: 'Срок (due)', value: 'due' },
  { label: 'Начало (start) — для issue игнорируется', value: 'start' },
]
const completionOptions = [
  { label: 'Любое изменение', value: null },
  { label: 'Стало «Выполнено»', value: true },
  { label: 'Снято «Выполнено»', value: false },
]
// The sensible default action for a freshly-picked trigger.
const DEFAULT_ACTION_FOR = {
  column: 'set_label',
  completion: 'set_state',
  priority: 'set_label',
  due: 'set_due',
  assignees: 'set_assignees',
  estimate: 'set_estimate',
  milestone: 'set_milestone',
  title_desc: 'set_title_desc',
  labels: 'reconcile_labels',
  comment: 'post_comment',
}
const triggerLabel = (v) => triggerTypeOptions.find((o) => o.value === v)?.label || v
const actionLabel = (v) => actionTypeOptions.find((o) => o.value === v)?.label || v
// Resolved estimation unit of the integration board (from the integration GET);
// the estimate toggle is only meaningful when it's "time".
const estimationUnit = ref('time')
const defaultColumn = ref('')
const defaultAction = ref('tag')
const tagKeepPrefix = ref(true)
// Generic rule list. Each: { match, match_type, action, keep_prefix, map:[{k,v}] }
const rules = ref([])
const boardOptions = ref([])
const boardProject = ref({}) // board id → project id, for prefix-name targeting
// Prefix display names loaded for the integration project (canonical → label).
// Kept so save() can merge rule-bound names without clobbering names set for
// prefixes that have no rule here.
const loadedPrefixNames = ref({})
const targetProjectId = computed(() => boardProject.value[boardId.value] || null)
const columnOptions = ref([]) // {label:name, value:name} — for label-rules value maps
const columnIdOptions = ref([]) // {label:name, value:id} — for column-move bindings
const columnNameById = ref({}) // id → name, to stamp column_name on save
const saving = ref(false)
// Only covers the POST that kicks the sync off — the sync itself runs detached on
// the server, so nothing here blocks on it.
const syncing = ref(false)
// Journal panel instance, so a freshly started run shows up immediately when the
// pane was already open (null until the pane is rendered — it loads on mount).
const journalRef = ref(null)

const intervalOptions = [
  { label: 'Вручную (выкл.)', value: 0 },
  { label: 'Каждые 5 минут', value: 300 },
  { label: 'Каждые 15 минут', value: 900 },
  { label: 'Каждый час', value: 3600 },
]
// Periodic FULL sweep (catches deletes/drift an incremental pull can't see). 0 = off
// — a full sync then runs only on the very first sync or via «Полная синхронизация».
const fullIntervalOptions = [
  { label: 'Не форсировать (только вручную)', value: 0 },
  { label: 'Раз в 6 часов', value: 21600 },
  { label: 'Раз в 12 часов', value: 43200 },
  { label: 'Раз в сутки', value: 86400 },
  { label: 'Раз в 2 суток', value: 172800 },
  { label: 'Раз в неделю', value: 604800 },
]
const actionOptions = [
  { label: 'Статус → колонка', value: 'status' },
  { label: 'Приоритет', value: 'priority' },
  { label: 'Доска (роутинг)', value: 'board' },
  { label: 'Тег', value: 'tag' },
  { label: 'Группировка (подзадачи)', value: 'group' },
  { label: 'Игнорировать', value: 'ignore' },
]
const matchTypeOptions = [
  { label: 'Префикс', value: 'prefix' },
  { label: 'Regex', value: 'regex' },
]
const defaultActionOptions = [
  { label: 'Создавать тег', value: 'tag' },
  { label: 'Игнорировать', value: 'ignore' },
]
// Target options for a rule's value-map, by action.
function mapTargetOptions(action) {
  if (action === 'status') return columnOptions.value
  if (action === 'priority') return priorityLevelOptions
  if (action === 'board') return boardOptions.value
  return []
}
const mapActions = ['status', 'priority', 'board']
const dueSourceOptions = [
  { label: 'Issue, иначе срок Milestone', value: 'issue_milestone' },
  { label: 'Только из Issue', value: 'issue' },
  { label: 'Только из Milestone', value: 'milestone' },
  { label: 'Не синхронизировать', value: 'off' },
]
const startSourceOptions = [
  { label: 'Дата создания задачи', value: 'created' },
  { label: 'Начало Milestone', value: 'milestone' },
  { label: 'Не синхронизировать', value: 'off' },
]
const priorityLevelOptions = PRIORITY_LABELS.map((label, value) => ({ label, value }))
// Priority qualifier for a binding trigger (null = any level). Declared here so it
// follows priorityLevelOptions (avoids a temporal-dead-zone reference).
const priorityQualOptions = [{ label: 'Любой приоритет', value: null }, ...priorityLevelOptions]

const lastSyncedText = computed(() =>
  lastSynced.value
    ? new Date(lastSynced.value).toLocaleString('ru-RU', {
        day: '2-digit',
        month: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
      })
    : 'ещё не синхронизировано',
)

async function loadBoards() {
  const all = []
  const bp = {}
  for (const p of ws.projects) {
    try {
      const bs = (await projApi.boards(p.id)).data || []
      for (const b of bs) {
        all.push({ label: `${p.name} / ${b.name}`, value: b.id })
        bp[b.id] = p.id
      }
    } catch {
      /* skip a project we can't read */
    }
  }
  boardOptions.value = all
  boardProject.value = bp
}

// Load the integration project's prefix display names and prefill each
// prefix-rule's friendly name. Re-runs when the target board (→ project) changes.
async function loadPrefixNames() {
  loadedPrefixNames.value = {}
  const pid = targetProjectId.value
  if (pid) {
    try {
      const { data } = await projApi.tagPrefixes(pid)
      const m = {}
      for (const p of data || []) m[p.prefix] = p.label
      loadedPrefixNames.value = m
    } catch {
      /* a fresh project may have none */
    }
  }
  for (const rule of rules.value) {
    if (rule.match_type === 'prefix')
      rule.label = loadedPrefixNames.value[canonPrefix(rule.match)] || ''
  }
}

async function loadColumns(id) {
  if (!id) {
    columnOptions.value = []
    columnIdOptions.value = []
    columnNameById.value = {}
    return
  }
  try {
    const cols = (await boardsApi.columns(id)).data || []
    columnOptions.value = cols.map((c) => ({ label: c.name, value: c.name }))
    columnIdOptions.value = cols.map((c) => ({ label: c.name, value: c.id }))
    const m = {}
    for (const c of cols) m[c.id] = c.name
    columnNameById.value = m
  } catch {
    columnOptions.value = []
    columnIdOptions.value = []
    columnNameById.value = {}
  }
}

// loadList fetches every binding of the workspace, then selects the first (or
// starts a fresh form when there are none).
async function loadList() {
  if (!props.wsId) return
  try {
    const { data } = await glApi.listIntegrations(props.wsId)
    integrations.value = data.integrations || []
    defaultRules.value = data.default_rules || null
    serviceConfigured.value = data.service_configured === true
    isAdmin.value = data.is_admin === true
    if (integrations.value.length) {
      await selectBinding(integrations.value[0].id)
    } else {
      newBinding()
    }
  } catch (e) {
    message.error(e.message)
  }
}

// bindingLabel renders a short caption for the selector.
function bindingLabel(b) {
  const board = boardOptions.value.find((o) => o.value === b.board_id)
  return b.name || b.project_path || (board ? board.label : 'привязка')
}
const bindingOptions = computed(() =>
  integrations.value.map((b) => ({ label: bindingLabel(b), value: b.id })),
)

async function selectBinding(id) {
  const b = integrations.value.find((x) => x.id === id)
  if (!b) {
    newBinding()
    return
  }
  currentId.value = b.id
  await applyBinding(b)
}

// newBinding resets the form for an unsaved binding, seeding server default rules.
function newBinding() {
  currentId.value = null
  applyBinding({ label_rules: defaultRules.value || {} })
}

async function applyBinding(data) {
  name.value = data.name || ''
  scope.value = data.scope || 'all'
  closedPolicy.value = data.closed_policy || 'archive_closed_sprints'
  closedAfter.value = data.closed_after ? new Date(data.closed_after).getTime() : null
  {
    projectPath.value = data.project_path || ''
    boardId.value = data.board_id || null
    enabled.value = data.enabled !== false
    intervalSec.value = data.sync_interval_sec || 0
    fullIntervalSec.value = data.full_sync_interval_sec ?? 86400
    dueSource.value = data.due_source || 'issue_milestone'
    startSource.value = data.start_source || 'created'
    lastSynced.value = data.last_synced_at || null
    const wb = data.writeback || {}
    wbEnabled.value = wb.enabled === true
    wbCreate.value = wb.push_create === true
    wbFetchTemplates.value = wb.fetch_templates === true
    estimationUnit.value = data.estimation_unit || 'time'
    const r = data.label_rules || {}
    defaultColumn.value = r.default_column || ''
    defaultAction.value = r.default_action || 'tag'
    tagKeepPrefix.value = r.tag_keep_prefix !== false
    rules.value = (r.rules || []).map((rule) => ({
      match: rule.match || '',
      match_type: rule.match_type || 'prefix',
      action: rule.action || 'tag',
      keep_prefix: rule.keep_prefix !== false,
      label: '', // friendly prefix name, filled by loadPrefixNames()
      map: Object.entries(rule.value_map || {}).map(([k, v]) => ({
        k,
        v: rule.action === 'priority' ? Number(v) : v,
      })),
    }))
    // Bindings: an explicit set wins; otherwise synthesize from the legacy toggles
    // (using the just-parsed rules for priority inversion) so a legacy integration
    // opens with an equivalent, editable default set.
    bindings.value =
      Array.isArray(wb.bindings) && wb.bindings.length
        ? wb.bindings.map(normalizeBinding)
        : synthesizeBindings(wb)
    if (boardId.value) await loadColumns(boardId.value)
    await loadPrefixNames()
  }
}

function onBoardChange(id) {
  boardId.value = id
  loadColumns(id)
  loadPrefixNames()
}

// ── write-back bindings ──

// normalizeBinding fills a stored binding to the full UI shape (nullable qualifiers
// stay null = "any"; clear_prefix defaults true only for a brand-new set_label).
function normalizeBinding(b) {
  const t = b.trigger || {}
  const a = b.action || {}
  return {
    enabled: b.enabled !== false,
    trigger: {
      type: t.type || 'column',
      column_id: t.column_id || '',
      column_name: t.column_name || '',
      priority: t.priority === 0 || t.priority ? t.priority : null,
      completed: typeof t.completed === 'boolean' ? t.completed : null,
      date_kind: t.date_kind || (t.type === 'due' ? 'due' : ''),
    },
    action: {
      type: a.type || 'set_label',
      label: a.label || '',
      clear_prefix: a.clear_prefix === true, // stored explicitly (no omitempty) → trust it
      state: a.state || '',
      date_kind: a.date_kind || (a.type === 'set_due' ? 'due' : ''),
      add_marker: a.add_marker === true,
    },
  }
}

// synthesizeBindings mirrors the backend's legacy-toggle synthesis so a pre-bindings
// integration shows the equivalent default set (editable). Priority fans out to one
// per-level set_label from the priority rule's inverted value map.
function synthesizeBindings(wb) {
  if (wb.enabled !== true) return []
  const out = []
  const mk = (trigger, action) => out.push(normalizeBinding({ enabled: true, trigger, action }))
  if (wb.push_state) mk({ type: 'completion' }, { type: 'set_state', state: '' })
  if (wb.push_priority) {
    const pr = rules.value.find((r) => r.action === 'priority' && r.match_type === 'prefix')
    if (pr) {
      const byLevel = {}
      let ambiguous = false
      for (const m of pr.map) {
        if (m.v == null || m.v === '') continue
        if (byLevel[m.v] != null) ambiguous = true
        byLevel[m.v] = m.k
      }
      if (!ambiguous) {
        Object.entries(byLevel)
          .sort((x, y) => Number(x[0]) - Number(y[0]))
          .forEach(([lvl, val]) =>
            mk(
              { type: 'priority', priority: Number(lvl) },
              { type: 'set_label', label: pr.match + val, clear_prefix: true },
            ),
          )
      }
    }
  }
  if (wb.push_comments) mk({ type: 'comment' }, { type: 'post_comment' })
  if (wb.push_labels) mk({ type: 'labels' }, { type: 'reconcile_labels' })
  if (wb.push_due) mk({ type: 'due', date_kind: 'due' }, { type: 'set_due', date_kind: 'due' })
  if (wb.push_assignees) mk({ type: 'assignees' }, { type: 'set_assignees' })
  if (wb.push_estimate) mk({ type: 'estimate' }, { type: 'set_estimate' })
  if (wb.push_milestone) mk({ type: 'milestone' }, { type: 'set_milestone' })
  if (wb.push_title_desc) mk({ type: 'title_desc' }, { type: 'set_title_desc' })
  return out
}

// bindingTriggerText / bindingActionText render the compact row summary.
function bindingTriggerText(b) {
  const t = b.trigger
  switch (t.type) {
    case 'column':
      return `Перенос → «${columnNameById.value[t.column_id] || t.column_name || '?'}»`
    case 'priority':
      return t.priority == null ? 'Приоритет (любой)' : `Приоритет: ${PRIORITY_LABELS[t.priority]}`
    case 'completion':
      return t.completed == null
        ? 'Флаг «Выполнено»'
        : t.completed
          ? 'Стало «Выполнено»'
          : 'Снято «Выполнено»'
    case 'due':
      return t.date_kind === 'start' ? 'Изменение начала' : 'Изменение срока'
    default:
      return triggerLabel(t.type)
  }
}
function bindingActionText(b) {
  const a = b.action
  switch (a.type) {
    case 'set_label':
      return `метка «${a.label || '?'}»`
    case 'set_state':
      return a.state === 'closed'
        ? 'закрыть issue'
        : a.state === 'opened'
          ? 'открыть issue'
          : 'закрыть/открыть issue'
    case 'post_comment':
      return a.add_marker ? 'комментарий (+маркер)' : 'комментарий'
    default:
      return actionLabel(a.type).toLowerCase()
  }
}

// Bindings are edited inline (as cards in the right pane), like the label rules —
// no separate draft. Adding appends a blank card; changing a card's trigger/action
// resets the now-irrelevant qualifiers so the card stays coherent.
function addBinding() {
  bindings.value.push(
    normalizeBinding({
      enabled: true,
      trigger: { type: 'column' },
      action: { type: 'set_label', clear_prefix: true },
    }),
  )
}
function removeBinding(i) {
  bindings.value.splice(i, 1)
}
function onBindingTrigger(b, type) {
  b.trigger.type = type
  b.trigger.column_id = ''
  b.trigger.column_name = ''
  b.trigger.priority = null
  b.trigger.completed = null
  b.trigger.date_kind = type === 'due' ? 'due' : ''
  onBindingAction(b, DEFAULT_ACTION_FOR[type] || 'set_label')
}
function onBindingAction(b, type) {
  b.action.type = type
  if (type === 'set_label' && !b.action.label) b.action.clear_prefix = true
  if (type === 'set_due' && !b.action.date_kind) b.action.date_kind = 'due'
}

// serializeBinding strips a UI binding to the wire shape (only relevant fields).
function serializeBinding(b) {
  const t = { type: b.trigger.type }
  if (t.type === 'column') {
    t.column_id = b.trigger.column_id
    t.column_name = columnNameById.value[b.trigger.column_id] || b.trigger.column_name || ''
  }
  if (t.type === 'priority' && b.trigger.priority != null) t.priority = b.trigger.priority
  if (t.type === 'completion' && b.trigger.completed != null) t.completed = b.trigger.completed
  if (t.type === 'due') t.date_kind = b.trigger.date_kind || 'due'
  const a = { type: b.action.type }
  if (a.type === 'set_label') {
    a.label = b.action.label.trim()
    a.clear_prefix = b.action.clear_prefix
  }
  if (a.type === 'set_state') a.state = b.action.state || ''
  if (a.type === 'set_due') a.date_kind = b.action.date_kind || 'due'
  if (a.type === 'post_comment') a.add_marker = b.action.add_marker
  return { enabled: b.enabled !== false, trigger: t, action: a }
}

function addRule() {
  rules.value.push({
    match: '',
    match_type: 'prefix',
    action: 'tag',
    keep_prefix: true,
    label: '',
    map: [],
  })
}
function addMapRow(rule) {
  rule.map.push({ k: '', v: '' })
}

async function save() {
  if (!projectPath.value.trim() || !boardId.value) {
    message.warning('Укажите путь к проекту GitLab и доску назначения')
    return
  }
  const label_rules = {
    default_column: defaultColumn.value,
    default_action: defaultAction.value,
    tag_keep_prefix: tagKeepPrefix.value,
    rules: rules.value.map((rule) => {
      const out = { match: rule.match, match_type: rule.match_type, action: rule.action }
      if (rule.action === 'tag') out.keep_prefix = rule.keep_prefix
      if (mapActions.includes(rule.action)) {
        out.value_map = Object.fromEntries(
          rule.map
            .filter((m) => m.k !== '' && m.v !== '' && m.v != null)
            .map((m) => [m.k, String(m.v)]),
        )
      }
      return out
    }),
  }
  saving.value = true
  try {
    const payload = {
      name: name.value.trim(),
      project_path: projectPath.value.trim(),
      board_id: boardId.value,
      enabled: enabled.value,
      sync_interval_sec: Number(intervalSec.value),
      full_sync_interval_sec: Number(fullIntervalSec.value),
      due_source: dueSource.value,
      start_source: startSource.value,
      scope: scope.value,
      closed_policy: closedPolicy.value,
      closed_after:
        closedPolicy.value === 'period' && closedAfter.value
          ? new Date(closedAfter.value).toISOString()
          : null,
      label_rules,
      writeback: {
        enabled: wbEnabled.value,
        push_create: wbCreate.value,
        fetch_templates: wbCreate.value && wbFetchTemplates.value,
        // The binding table fully replaces the legacy toggles; a non-empty set makes
        // the backend ignore them entirely.
        bindings: wbEnabled.value ? bindings.value.map(serializeBinding) : [],
      },
    }
    const { data } = currentId.value
      ? await glApi.updateIntegration(props.wsId, currentId.value, payload)
      : await glApi.createIntegration(props.wsId, payload)
    currentId.value = data.id
    lastSynced.value = data.last_synced_at || lastSynced.value
    // Refresh the binding list so the selector reflects the new/updated row.
    try {
      const list = await glApi.listIntegrations(props.wsId)
      integrations.value = list.data.integrations || []
    } catch {
      /* non-critical */
    }
    // Persist friendly prefix names to the target project. Merge over the loaded
    // set so prefixes without a rule here aren't dropped; a blanked name removes.
    const pid = targetProjectId.value
    if (pid) {
      const merged = { ...loadedPrefixNames.value }
      for (const rule of rules.value) {
        if (rule.match_type !== 'prefix') continue
        const k = canonPrefix(rule.match)
        if (!k) continue
        const label = (rule.label || '').trim()
        if (label) merged[k] = label
        else delete merged[k]
      }
      await projApi.setTagPrefixes(
        pid,
        Object.entries(merged).map(([prefix, label]) => ({ prefix, label })),
      )
      loadedPrefixNames.value = merged
    }
    message.success('Настройки интеграции сохранены')
  } catch (e) {
    message.error(e.message)
  } finally {
    saving.value = false
  }
}

async function deleteBinding() {
  if (!currentId.value) {
    newBinding()
    return
  }
  try {
    await glApi.deleteIntegration(props.wsId, currentId.value)
    message.success('Привязка удалена')
    await loadList()
  } catch (e) {
    message.error(e.message)
  }
}

async function syncNow(mode) {
  if (!currentId.value) {
    message.warning('Сначала сохраните привязку')
    return
  }
  syncing.value = true
  try {
    // Fire-and-forget: the backend runs the pull in the background and notifies the
    // user who started it when it ends (kind `integration_sync`). We only open the
    // journal, where the run shows up live as "выполняется" — no blocking overlay,
    // no polling, so a multi-minute batch doesn't hold the modal hostage. The main
    // button does an incremental pull; the dropdown's "Полная синхронизация" passes
    // mode='full'.
    const { data } = await glApi.sync(props.wsId, currentId.value, mode)
    if (data?.already_running) {
      message.warning('Синхронизация уже выполняется — дождитесь её завершения')
    } else {
      message.info(
        mode === 'full'
          ? 'Полная синхронизация запущена в фоне — уведомим по завершении'
          : 'Синхронизация запущена в фоне — уведомим по завершении',
      )
    }
    openRight('journal')
    journalRef.value?.reload()
  } catch (e) {
    message.error(e.message)
  } finally {
    syncing.value = false
  }
}

// ── sync journal + write-back conflicts ──
// Both now open inside the modal's right pane (rightMode), not separate modals.
// The active section's menu item is highlighted so it's clear what's open.
const conflictCount = ref(0)
const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
const syncMenu = computed(() => [
  {
    label: 'Полная синхронизация',
    key: 'full',
    icon: menuIcon(CloudDownloadOutline),
    disabled: syncing.value,
  },
  { type: 'divider', key: 'd1' },
  {
    label: 'Журнал синхронизации',
    key: 'journal',
    icon: menuIcon(TimeOutline),
    props: { class: rightMode.value === 'journal' ? 'gl-menu-active' : '' },
  },
  {
    label: conflictCount.value ? `Конфликты (${conflictCount.value})` : 'Конфликты',
    key: 'conflicts',
    icon: menuIcon(WarningOutline),
    disabled: !conflictCount.value,
    props: { class: rightMode.value === 'conflicts' ? 'gl-menu-active' : '' },
  },
])
function onSyncMenu(key) {
  if (key === 'full') syncNow('full')
  else if (key === 'journal') openRight('journal')
  else if (key === 'conflicts') openRight('conflicts')
}
async function loadConflictCount() {
  try {
    const { data } = await glApi.conflicts(props.wsId)
    conflictCount.value = (data || []).length
  } catch {
    conflictCount.value = 0
  }
}

watch(
  () => [props.show, props.wsId],
  async ([show]) => {
    if (!show) return
    if (!gl.loaded) await gl.load()
    await loadBoards()
    await loadList()
    loadConflictCount()
  },
  { immediate: false },
)
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <!-- Wrap is the non-scrolling positioned frame: the sync loader fills it to
         dim/blur the WHOLE card (header + edges), while the card body scrolls in
         its own inner gl-scroll. -->
    <div class="gl-modal-wrap">
      <n-card
        class="gl-card"
        :style="{ width: rightMode ? 'min(1400px, 94vw)' : '580px', maxWidth: '94vw' }"
        role="dialog"
      >
        <template #header>
          <span class="gl-title">
            <n-icon :component="LogoGitlab" class="grad-icon" /> GitLab
          </span>
        </template>

        <div class="gl-panes">
          <div class="gl-left gl-scroll t-hoverscroll">
            <!-- ACCOUNT -->
            <section class="gl-sec">
              <h4 class="gl-h">Аккаунт</h4>
              <p v-if="serviceConfigured" class="gl-wb-hint">
                <n-text depth="3">
                  Синхронизация идёт под <b>системным сервисным токеном</b> (задаётся админом в
                  панели администрирования). Личный токен ниже подключать не обязательно — он нужен
                  лишь как запасной вариант для операций от вашего имени.
                </n-text>
              </p>
              <template v-if="gl.connected">
                <div class="gl-conn">
                  <div>
                    <span class="gl-user accent-grad-text">@{{ gl.username }}</span>
                    <span class="gl-url">{{ gl.baseUrl }}</span>
                  </div>
                  <n-popconfirm
                    :positive-button-props="{ type: 'error' }"
                    positive-text="Отключить"
                    @positive-click="disconnect"
                  >
                    <template #trigger>
                      <n-button text size="small" type="error">Отключить</n-button>
                    </template>
                    Отключить аккаунт GitLab? Интеграции перестанут синхронизироваться.
                  </n-popconfirm>
                </div>
              </template>
              <template v-else>
                <div class="gl-grid">
                  <n-text depth="3" class="lbl">URL GitLab</n-text>
                  <n-input
                    v-model:value="baseInput"
                    size="small"
                    placeholder="https://gitlab.example.com"
                    :input-props="{ autocomplete: 'off', name: 'gl-base-url' }"
                  />
                  <n-text depth="3" class="lbl">Токен (PAT, scope read_api)</n-text>
                  <n-input
                    v-model:value="tokenInput"
                    type="password"
                    show-password-on="click"
                    size="small"
                    placeholder="glpat-…"
                    :input-props="{ autocomplete: 'new-password', name: 'gl-token' }"
                    @keyup.enter="connect"
                  />
                </div>
                <div class="gl-actions">
                  <n-button type="primary" size="small" :loading="connecting" @click="connect">
                    Подключить
                  </n-button>
                </div>
              </template>
            </section>

            <!-- INTEGRATION — available when a personal PAT is connected OR an instance
           service token is configured (admin). -->
            <section v-if="gl.connected || serviceConfigured" class="gl-sec">
              <h4 class="gl-h">Привязки GitLab → доска</h4>
              <p v-if="!isAdmin" class="gl-wb-hint">
                <n-text depth="3">
                  Изменять привязки может только администратор. Вам доступен просмотр и запуск
                  синхронизации.
                </n-text>
              </p>
              <!-- Multi-binding selector: pick a binding to edit, add a new one, or
             delete the current one. -->
              <div class="gl-bindbar">
                <n-select
                  :value="currentId"
                  :options="bindingOptions"
                  size="small"
                  placeholder="Новая привязка"
                  :consistent-menu-width="false"
                  style="flex: 1 1 auto"
                  @update:value="selectBinding"
                />
                <n-button
                  size="small"
                  tertiary
                  title="Новая привязка"
                  :disabled="!isAdmin"
                  @click="newBinding"
                >
                  <template #icon><n-icon :component="AddOutline" /></template>
                </n-button>
                <n-popconfirm
                  v-if="currentId && isAdmin"
                  :positive-button-props="{ type: 'error' }"
                  positive-text="Удалить"
                  @positive-click="deleteBinding"
                >
                  <template #trigger>
                    <n-button size="small" tertiary type="error" title="Удалить привязку">
                      <template #icon><n-icon :component="TrashOutline" /></template>
                    </n-button>
                  </template>
                  Удалить привязку? Синхронизированные задачи останутся, связь с GitLab пропадёт.
                </n-popconfirm>
              </div>
              <div class="gl-grid">
                <n-text depth="3" class="lbl">Название (необязательно)</n-text>
                <n-input v-model:value="name" size="small" placeholder="напр. Скрам-борд" />

                <n-text depth="3" class="lbl">Проект GitLab (полный путь)</n-text>
                <n-input v-model:value="projectPath" size="small" placeholder="group/project" />

                <n-text depth="3" class="lbl">Доска назначения</n-text>
                <n-select
                  :value="boardId"
                  :options="boardOptions"
                  size="small"
                  placeholder="Выберите доску"
                  @update:value="onBoardChange"
                />

                <n-text depth="3" class="lbl">Автосинхронизация</n-text>
                <n-select v-model:value="intervalSec" :options="intervalOptions" size="small" />

                <n-text depth="3" class="lbl">Полная синхронизация</n-text>
                <n-select
                  v-model:value="fullIntervalSec"
                  :options="fullIntervalOptions"
                  size="small"
                />

                <n-text depth="3" class="lbl">Источник срока</n-text>
                <n-select v-model:value="dueSource" :options="dueSourceOptions" size="small" />

                <n-text depth="3" class="lbl">Источник начала</n-text>
                <n-select v-model:value="startSource" :options="startSourceOptions" size="small" />

                <n-text depth="3" class="lbl">Что переносить</n-text>
                <n-select v-model:value="scope" :options="scopeOptions" size="small" />

                <n-text depth="3" class="lbl">Закрытые задачи</n-text>
                <n-select
                  v-model:value="closedPolicy"
                  :options="closedPolicyOptions"
                  size="small"
                />

                <template v-if="closedPolicy === 'period'">
                  <n-text depth="3" class="lbl">Закрытые не старше</n-text>
                  <n-date-picker v-model:value="closedAfter" type="date" size="small" clearable />
                </template>

                <n-text depth="3" class="lbl">Включена</n-text>
                <div><n-switch v-model:value="enabled" /></div>
              </div>

              <!-- Write-back (Tessera → GitLab), opt-in; all off by default -->
              <h4 class="gl-h gl-h-sub">Обратная запись в GitLab</h4>
              <div class="gl-grid">
                <n-text depth="3" class="lbl">Включить запись</n-text>
                <div><n-switch v-model:value="wbEnabled" /></div>

                <n-text depth="3" class="lbl">Создание issue из задачи</n-text>
                <div><n-switch v-model:value="wbCreate" /></div>

                <template v-if="wbCreate">
                  <n-text depth="3" class="lbl">Получение issue-templates из проекта</n-text>
                  <div><n-switch v-model:value="wbFetchTemplates" size="small" /></div>
                </template>
              </div>
              <p class="gl-wb-hint">
                <n-text depth="3">
                  «Создание issue из задачи» добавляет в модалку задачи (под свойством «Родитель»)
                  кнопку «Создать issue в GitLab» — issue открывается из свойств и описания задачи
                  под токеном владельца интеграции, после чего задача становится синхронизированной.
                  Работает независимо от обратной записи изменений ниже. «Получение issue-templates»
                  подтягивает шаблоны
                  <code>.gitlab/issue_templates/*.md</code> — их можно выбрать над редактором
                  описания перед созданием.
                </n-text>
              </p>
              <!-- Uniform "configure" buttons: each opens the right pane in its mode. The
             lists/editors themselves live in the right pane. -->
              <div class="gl-cfg-row">
                <div class="gl-cfg-text">
                  <n-text depth="3" class="gl-cfg-title">Действия обратной записи</n-text>
                  <n-text depth="3" class="gl-cfg-hint">
                    Что делать на issue GitLab при изменениях задачи. Перенос в колонку ставит метку
                    статуса и не закрывает issue.
                  </n-text>
                </div>
                <n-button
                  size="small"
                  :tertiary="rightMode !== 'actions'"
                  :type="rightMode === 'actions' ? 'primary' : 'default'"
                  :disabled="!wbEnabled"
                  @click="openRight('actions')"
                >
                  <template #icon><n-icon :component="CreateOutline" /></template>
                  Настроить действия<template v-if="wbEnabled && bindings.length">
                    ({{ bindings.length }})</template
                  >
                </n-button>
              </div>
              <div class="gl-cfg-row">
                <div class="gl-cfg-text">
                  <n-text depth="3" class="gl-cfg-title">Правила разбора меток GitLab</n-text>
                  <n-text depth="3" class="gl-cfg-hint">
                    Как входящие метки GitLab превращаются в статус, приоритет и теги при
                    синхронизации.
                  </n-text>
                </div>
                <n-button
                  size="small"
                  :tertiary="rightMode !== 'rules'"
                  :type="rightMode === 'rules' ? 'primary' : 'default'"
                  @click="openRight('rules')"
                >
                  <template #icon><n-icon :component="CreateOutline" /></template>
                  Настроить правила
                </n-button>
              </div>

              <div class="gl-footer">
                <span class="gl-synced">Последняя синхронизация: {{ lastSyncedText }}</span>
                <div class="gl-footer-btns">
                  <n-badge
                    :value="conflictCount"
                    :max="9"
                    :show="conflictCount > 0"
                    color="#e0922f"
                    title="Есть неразрешённые конфликты — откройте «Конфликты» в меню рядом"
                  >
                    <n-button-group size="medium">
                      <n-button :loading="syncing" @click="syncNow()">
                        <template #icon><n-icon :component="SyncOutline" /></template>
                        Синхронизировать
                      </n-button>
                      <n-dropdown trigger="click" :options="syncMenu" @select="onSyncMenu">
                        <n-button :disabled="syncing" class="gl-sync-caret">
                          <template #icon><n-icon :component="ChevronDownOutline" /></template>
                        </n-button>
                      </n-dropdown>
                    </n-button-group>
                  </n-badge>
                  <n-button
                    type="primary"
                    size="medium"
                    :loading="saving"
                    :disabled="!isAdmin"
                    :title="isAdmin ? '' : 'Изменять привязки может только администратор'"
                    @click="save"
                  >
                    Сохранить
                  </n-button>
                </div>
              </div>
            </section>
          </div>
          <!-- RIGHT PANE — expands the modal to edit the write-back actions or the
               GL→Tessera label-parsing rules. -->
          <div v-if="rightMode" class="gl-right gl-scroll t-hoverscroll">
            <div class="gl-right-head">
              <span class="gl-right-title">
                {{ RIGHT_TITLES[rightMode] }}
              </span>
              <n-button text size="small" aria-label="Закрыть" @click="rightMode = null">
                <n-icon :component="CloseOutline" />
              </n-button>
            </div>

            <!-- ACTIONS: write-back binding cards -->
            <template v-if="rightMode === 'actions'">
              <p class="gl-wb-hint">
                <n-text depth="3">
                  Каждое действие связывает событие в задаче Tessera с действием на issue GitLab
                  (под токеном владельца интеграции, scope «api»). По умолчанию набор повторяет
                  прежнее поведение записи.
                </n-text>
              </p>
              <div v-if="!bindings.length" class="gl-binds-empty">
                <n-text depth="3">Пока нет действий. Добавьте первое ниже.</n-text>
              </div>
              <div
                v-for="(b, bi) in bindings"
                :key="bi"
                class="gl-rcard"
                :class="{ 'gl-bind-off': !b.enabled }"
              >
                <div class="gl-bcard-head">
                  <n-switch v-model:value="b.enabled" size="small" :disabled="!isAdmin" />
                  <span class="gl-bcard-sum">
                    {{ bindingTriggerText(b) }} →
                    <span class="accent-grad-text">{{ bindingActionText(b) }}</span>
                  </span>
                  <n-button
                    text
                    size="tiny"
                    type="error"
                    title="Удалить"
                    :disabled="!isAdmin"
                    @click="removeBinding(bi)"
                  >
                    <n-icon :component="TrashOutline" />
                  </n-button>
                </div>
                <div class="gl-grid">
                  <n-text depth="3" class="lbl">Действие в Tessera</n-text>
                  <n-select
                    :value="b.trigger.type"
                    :options="triggerTypeOptions"
                    size="small"
                    :disabled="!isAdmin"
                    @update:value="(v) => onBindingTrigger(b, v)"
                  />

                  <template v-if="b.trigger.type === 'column'">
                    <n-text depth="3" class="lbl">Целевая колонка</n-text>
                    <n-select
                      v-model:value="b.trigger.column_id"
                      :options="columnIdOptions"
                      size="small"
                      placeholder="Выберите колонку"
                      :disabled="!isAdmin"
                    />
                  </template>
                  <template v-else-if="b.trigger.type === 'priority'">
                    <n-text depth="3" class="lbl">Приоритет</n-text>
                    <n-select
                      v-model:value="b.trigger.priority"
                      :options="priorityQualOptions"
                      size="small"
                      :disabled="!isAdmin"
                    />
                  </template>
                  <template v-else-if="b.trigger.type === 'completion'">
                    <n-text depth="3" class="lbl">Условие</n-text>
                    <n-select
                      v-model:value="b.trigger.completed"
                      :options="completionOptions"
                      size="small"
                      :disabled="!isAdmin"
                    />
                  </template>
                  <template v-else-if="b.trigger.type === 'due'">
                    <n-text depth="3" class="lbl">Тип срока</n-text>
                    <n-select
                      v-model:value="b.trigger.date_kind"
                      :options="dateKindOptions"
                      size="small"
                      :disabled="!isAdmin"
                    />
                  </template>

                  <n-text depth="3" class="lbl">Действие в GitLab</n-text>
                  <n-select
                    :value="b.action.type"
                    :options="actionTypeOptions"
                    size="small"
                    :disabled="!isAdmin"
                    @update:value="(v) => onBindingAction(b, v)"
                  />

                  <template v-if="b.action.type === 'set_label'">
                    <n-text depth="3" class="lbl">Метка</n-text>
                    <n-input
                      v-model:value="b.action.label"
                      size="small"
                      placeholder="напр. S: In Progress"
                      :disabled="!isAdmin"
                    />
                    <n-text depth="3" class="lbl">Снимать метки того же префикса</n-text>
                    <div>
                      <n-switch
                        v-model:value="b.action.clear_prefix"
                        size="small"
                        :disabled="!isAdmin"
                      />
                    </div>
                  </template>
                  <template v-else-if="b.action.type === 'set_state'">
                    <n-text depth="3" class="lbl">Состояние issue</n-text>
                    <n-select
                      v-model:value="b.action.state"
                      :options="stateOptions"
                      size="small"
                      :disabled="!isAdmin"
                    />
                  </template>
                  <template v-else-if="b.action.type === 'set_due'">
                    <n-text depth="3" class="lbl">Тип срока</n-text>
                    <n-select
                      v-model:value="b.action.date_kind"
                      :options="dateKindOptions"
                      size="small"
                      :disabled="!isAdmin"
                    />
                  </template>
                  <template v-else-if="b.action.type === 'post_comment'">
                    <n-text depth="3" class="lbl">Добавлять маркер Tessera</n-text>
                    <div>
                      <n-switch
                        v-model:value="b.action.add_marker"
                        size="small"
                        :disabled="!isAdmin"
                      />
                    </div>
                  </template>
                </div>
              </div>
              <n-button
                dashed
                size="small"
                type="primary"
                class="gl-add"
                :disabled="!isAdmin"
                @click="addBinding"
              >
                <template #icon><n-icon :component="AddOutline" /></template>
                Действие
              </n-button>
            </template>

            <!-- RULES: GL → Tessera label parsing -->
            <template v-else-if="rightMode === 'rules'">
              <div class="gl-grid gl-grid-rules">
                <n-text depth="3" class="lbl">Колонка по умолчанию</n-text>
                <n-select
                  v-model:value="defaultColumn"
                  :options="columnOptions"
                  size="small"
                  placeholder="напр. К работе"
                />
                <n-text depth="3" class="lbl">Прочие метки</n-text>
                <n-select
                  v-model:value="defaultAction"
                  :options="defaultActionOptions"
                  size="small"
                />
                <n-text depth="3" class="lbl">Сохранять префикс тега</n-text>
                <div><n-switch v-model:value="tagKeepPrefix" /></div>
              </div>

              <div v-for="(rule, ri) in rules" :key="ri" class="gl-rcard">
                <div class="gl-rrow">
                  <n-input
                    v-model:value="rule.match"
                    size="small"
                    placeholder="S: либо ^(T|C): "
                    class="gl-rmatch"
                  />
                  <n-select
                    v-model:value="rule.match_type"
                    :options="matchTypeOptions"
                    size="small"
                    class="gl-rtype"
                  />
                  <n-select
                    v-model:value="rule.action"
                    :options="actionOptions"
                    size="small"
                    class="gl-raction"
                  />
                  <n-button text size="tiny" type="error" @click="rules.splice(ri, 1)">
                    <n-icon :component="TrashOutline" />
                  </n-button>
                </div>
                <div v-if="rule.match_type === 'prefix'" class="gl-ropt">
                  <n-text depth="3" class="lbl">Понятное имя</n-text>
                  <n-input
                    v-model:value="rule.label"
                    size="small"
                    placeholder="напр. Статус"
                    class="gl-rname"
                  />
                </div>
                <div v-if="rule.action === 'tag'" class="gl-ropt">
                  <n-text depth="3" class="lbl">Сохранять префикс</n-text>
                  <n-switch v-model:value="rule.keep_prefix" size="small" />
                </div>
                <div v-if="mapActions.includes(rule.action)" class="gl-rmap">
                  <div v-for="(m, mi) in rule.map" :key="mi" class="gl-rule">
                    <n-input
                      v-model:value="m.k"
                      size="small"
                      :placeholder="
                        rule.action === 'board'
                          ? 'Future'
                          : rule.action === 'priority'
                            ? 'Critical'
                            : 'In review'
                      "
                    />
                    <n-select
                      v-model:value="m.v"
                      :options="mapTargetOptions(rule.action)"
                      size="small"
                      placeholder="→ значение"
                    />
                    <n-button text size="tiny" type="error" @click="rule.map.splice(mi, 1)">
                      <n-icon :component="TrashOutline" />
                    </n-button>
                  </div>
                  <n-button
                    dashed
                    size="tiny"
                    type="primary"
                    class="gl-add"
                    @click="addMapRow(rule)"
                  >
                    <template #icon><n-icon :component="AddOutline" /></template>
                    Значение
                  </n-button>
                </div>
              </div>
              <n-button dashed size="small" type="primary" class="gl-add" @click="addRule">
                <template #icon><n-icon :component="AddOutline" /></template>
                Правило
              </n-button>
            </template>

            <!-- JOURNAL: sync run history -->
            <git-lab-journal-panel
              v-else-if="rightMode === 'journal'"
              ref="journalRef"
              class="gl-pane-fill"
              :ws-id="wsId"
            />

            <!-- CONFLICTS: write-back conflict resolver -->
            <conflict-resolver-panel
              v-else-if="rightMode === 'conflicts'"
              class="gl-pane-fill"
              :ws-id="wsId"
              @resolved="loadConflictCount"
            />
          </div>
        </div>
      </n-card>
    </div>
  </n-modal>
</template>

<style scoped>
/* Caret button on the split "Синхронизировать" control keeps a tight width. */
.gl-sync-caret {
  padding-left: 6px;
  padding-right: 6px;
}
</style>

<!-- Unscoped: the sync dropdown is teleported, so the active-item highlight
     (shows which section — journal/conflicts — is open in the right pane)
     can't be reached by a scoped selector. -->
<style>
.n-dropdown-option.gl-menu-active .n-dropdown-option-body,
.n-dropdown-option-body.gl-menu-active {
  color: var(--t-primary);
  font-weight: 600;
  background: color-mix(in srgb, var(--t-primary) 12%, transparent);
}
</style>

<style scoped>
/* Non-scrolling frame around the card: the sync loader fills it to cover the
   whole card (the radius is shared so the overlay's rounded corners line up).
   gl-scroll holds the scrollable body so the loader never scrolls out of view. */
.gl-modal-wrap {
  position: relative;
  max-width: 94vw;
  border-radius: 12px;
}
.gl-card {
  border-radius: 12px;
}
.gl-scroll {
  max-height: 76vh;
  overflow-y: auto;
}
.gl-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}
.gl-sec {
  margin-bottom: 18px;
}
.gl-h {
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.4px;
  color: var(--t-text3);
  margin: 0 0 10px;
}
.gl-h-sub {
  margin-top: 18px;
  border-top: 1px solid var(--t-border);
  padding-top: 14px;
}
/* Binding selector row: dropdown + add/delete buttons aligned to its right. */
.gl-bindbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.gl-grid {
  display: grid;
  grid-template-columns: 160px 1fr;
  gap: 8px 12px;
  align-items: center;
}
/* Grid children default to min-width:auto, letting a wide select/input push the
   column past the modal edge — pin to 0 so fields shrink to fit. */
.gl-grid > *,
.gl-rrow > *,
.gl-rule > *,
.gl-rmap > * {
  min-width: 0;
}
.gl-grid-top {
  margin-top: 12px;
}
/* Breathing room between the defaults grid (ends with the prefix toggle) and the
   per-prefix rule cards that follow. */
.gl-grid-rules {
  margin-bottom: 14px;
}
.gl-wb-hint {
  margin: 8px 0 12px;
  font-size: 12px;
  line-height: 1.4;
}
.lbl {
  font-size: 12px;
}
.est-wb {
  display: flex;
  align-items: center;
  gap: 8px;
}
.est-hint {
  font-size: 11px;
  line-height: 1.3;
}
.gl-actions {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}
.gl-conn {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.gl-user {
  font-weight: 600;
  margin-right: 8px;
}
.gl-url {
  font-size: 12px;
  color: var(--t-text3);
}
.gl-rules {
  margin-top: 10px;
}
.gl-rules-head {
  display: grid;
  grid-template-columns: 1fr 1fr 24px;
  gap: 8px;
  font-size: 11px;
  color: var(--t-text3);
  text-transform: uppercase;
  letter-spacing: 0.3px;
  padding: 0 2px 4px;
}
.gl-rule {
  display: grid;
  grid-template-columns: 1fr 1fr 24px;
  gap: 8px;
  align-items: center;
  margin-bottom: 6px;
}
.gl-rcard {
  border: 1px solid var(--t-border);
  border-radius: 8px;
  padding: 10px;
  margin-bottom: 8px;
}
.gl-rrow {
  display: grid;
  grid-template-columns: 1fr 96px 150px 24px;
  gap: 8px;
  align-items: center;
}
.gl-ropt {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 8px;
}
.gl-rname {
  flex: 1;
  max-width: 240px;
}
.gl-rmap {
  margin-top: 8px;
  padding-left: 10px;
  border-left: 2px solid var(--t-border);
}
.gl-add {
  margin-top: 2px;
}
.gl-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 18px;
  border-top: 1px solid var(--t-border);
  padding-top: 14px;
}
.gl-synced {
  font-size: 12px;
  color: var(--t-text3);
}
.gl-footer-btns {
  display: flex;
  gap: 8px;
}

/* Two-pane body: left = main settings, right = expandable editor. Each pane
   scrolls independently (both carry .gl-scroll). */
.gl-panes {
  display: flex;
  align-items: stretch;
}
/* Both panes share the extra width when the modal expands (basis + grow), so a
   wide screen gives each a comfortable working area instead of a fixed 400px. */
.gl-left {
  flex: 1 1 540px;
  min-width: 0;
}
.gl-right {
  flex: 1 1 560px;
  min-width: 0;
  border-left: 1px solid var(--t-border);
  padding-left: 16px;
  margin-left: 16px;
  /* Column layout so an embedded panel (journal/conflicts) can flex-fill the
     pane's full height instead of sitting short at the top. */
  display: flex;
  flex-direction: column;
}
/* Journal / conflicts panels fill the remaining pane height below the header. */
.gl-pane-fill {
  flex: 1 1 auto;
  min-height: 0;
}
.gl-right-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
.gl-right-title {
  font-size: 13px;
  font-weight: 600;
}
/* Left-pane "configure" rows: label/hint + a button that opens the right pane. */
.gl-cfg-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 0;
  border-top: 1px solid var(--t-border);
}
.gl-cfg-text {
  min-width: 0;
}
.gl-cfg-title {
  display: block;
  font-size: 13px;
  color: var(--t-text2);
}
.gl-cfg-hint {
  display: block;
  font-size: 12px;
  line-height: 1.35;
}
/* Empty-state + per-binding card header (in the right pane). */
.gl-binds-empty {
  padding: 8px 2px 12px;
  font-size: 12px;
}
.gl-bind-off {
  opacity: 0.6;
}
.gl-bcard-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.gl-bcard-sum {
  flex: 1 1 auto;
  min-width: 0;
  font-size: 12px;
  color: var(--t-text2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Narrow screens: stack the two panes vertically so nothing overflows. */
@media (max-width: 900px) {
  .gl-panes {
    flex-direction: column;
  }
  .gl-right {
    flex: 1 1 auto;
    width: auto;
    border-left: none;
    padding-left: 0;
    margin-left: 0;
    border-top: 1px solid var(--t-border);
    padding-top: 14px;
    margin-top: 14px;
  }
}

/* Narrow screens: stack label-over-field, tighten the rule grids so nothing
   spills past the modal, and drop the footer text onto its own line above the
   buttons (instead of squeezing it into a wrapping column). */
@media (max-width: 560px) {
  .gl-grid {
    grid-template-columns: 104px 1fr;
    gap: 8px;
  }
  .gl-rrow {
    grid-template-columns: 1fr 80px 96px 22px;
    gap: 6px;
  }
  .gl-rule,
  .gl-rules-head {
    grid-template-columns: 1fr 1fr 22px;
  }
  .gl-footer {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;
  }
  .gl-footer-btns {
    justify-content: flex-end;
  }
}
</style>
