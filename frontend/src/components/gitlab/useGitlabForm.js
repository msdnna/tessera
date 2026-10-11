// The GitLab window's binding form (#2975): the state and the server calls that
// used to live in GitLabModal.vue, shared by its sections (Подключения, Правила
// лейблов, Обратная запись) through provide/inject. The logic is the same as
// before the v2 window; what is new is marked so — dirty tracking for the
// explicit save, rule order, the header's «N мин назад».
import { tourEvent } from '@/utils/tourEvents'
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from '@/composables/useMessage'
import { gitlab as glApi, projects as projApi, boards as boardsApi } from '@/api'
import { canonPrefix } from '@/utils/tagGroups'
import { useGitlabStore } from '@/stores/gitlab'
import { useWorkspacesStore } from '@/stores/workspaces'
import { priorityLabel, priorityOptions } from '@/utils/priority'
import { columnName } from '@/utils/defaultNames'
import { useFormat } from '@/composables/useFormat'

export const GITLAB_FORM = Symbol('gitlab-form')

export function useGitlabForm({ wsId, onSyncStarted } = {}) {
  const { t } = useI18n()
  const message = useMessage()
  const gl = useGitlabStore()
  const { formatDateTime } = useFormat()
  const ws = useWorkspacesStore()

  // ── connection (per-user) ──
  const baseInput = ref('')
  const tokenInput = ref('')
  const connecting = ref(false)

  async function connect() {
    const b = baseInput.value.trim()
    // `tok`, not `t`: the i18n `t` is needed two lines down for the success line —
    // shadowed, the call threw after a good connect and the catch reported it as
    // a failure (#2975).
    const tok = tokenInput.value.trim()
    if (!b || !tok) return
    connecting.value = true
    try {
      await gl.connect(b, tok)
      tokenInput.value = ''
      message.success(t('gitlab.modal.connectedAs', { name: gl.username }))
    } catch (e) {
      message.error(e.message)
    } finally {
      connecting.value = false
    }
  }
  async function disconnect() {
    try {
      await gl.disconnect()
      message.success(t('gitlab.modal.disconnected'))
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
  // Every option table below is `computed`, never a module constant: a constant is
  // evaluated once on import and would keep the language of the first render (#2799).
  // `value` is the wire format and stays as it is.
  const scopeOptions = computed(() =>
    ['all', 'assigned'].map((v) => ({ label: t(`gitlab.modal.option.scope.${v}`), value: v })),
  )
  const closedPolicyOptions = computed(() =>
    ['archive_closed_sprints', 'all', 'period'].map((v) => ({
      label: t(`gitlab.modal.option.closedPolicy.${v}`),
      value: v,
    })),
  )
  const projectPath = ref('')
  const boardId = ref(null)
  const enabled = ref(true)
  const intervalSec = ref(0)
  const fullIntervalSec = ref(86400)
  const dueSource = ref('issue_milestone')
  const startSource = ref('created')
  // relations_sync is 'off' | 'pull' on the wire ('two_way' is reserved for pushing
  // Tessera relations back), so the switch serialises to those two values.
  const relationsSync = ref(true)
  const lastSynced = ref(null)
  // ── webhook (#2594): GitLab → Tessera near-realtime trigger ──
  // The hook is wired up by hand — we show the URL and a freshly generated secret to
  // paste into the project's Settings → Webhooks. webhookSecret holds that secret for
  // this one screen only: the server never returns it again.
  const webhookEnabled = ref(false)
  const webhookUrl = ref('')
  const lastWebhook = ref(null)
  const webhookSecret = ref('')
  const webhookBusy = ref(false)
  // ── write-back (Tessera → GitLab), opt-in; all off by default ──
  const wbEnabled = ref(false) // master toggle for the whole binding table
  const wbCreate = ref(false) // allow creating GitLab issues from tasks (independent of write-back)
  const wbFetchTemplates = ref(false) // offer repo issue templates when creating
  // Issue hierarchy (#2592). Plain flags, not rows in the binding table: a binding is
  // "a field of an already-linked task changed", and creating a subtask is neither —
  // the trigger is structural and the child has no link yet.
  const wbChildren = ref(false) // push subtasks of a grouped parent as child work items
  const wbAutoGroup = ref(false) // label a linked parent as grouped on the first child
  // Empty falls back to the backend default (gitlab.DefaultGroupLabel), which the
  // placeholder below spells out. On its own line, not trailing: the ratchet scanner
  // keeps trailing `//` text on purpose, so a Cyrillic example there reads as UI.
  const wbGroupLabel = ref('')
  // Mirror Tessera-hosted attachments into GitLab's upload store on push (#2713).
  // Defaults to ON — an integration saved before this flag existed has no key, and
  // the backend reads that as enabled (dead links in issues are a bug, not a taste).
  const wbAttachments = ref(true)
  // On a write-back conflict (#3088): ask | tessera (push ours) | gitlab (take theirs).
  const wbConflictPolicy = ref('ask')
  // bindings is the customizable trigger→action table (replaces the fixed toggles).
  // Each entry: { enabled, trigger:{type,column_id,column_name,priority,completed,date_kind},
  //               action:{type,label,clear_prefix,state,date_kind,add_marker} }
  const bindings = ref([])
  const TRIGGER_TYPES = [
    'column',
    'completion',
    'priority',
    'due',
    'assignees',
    'estimate',
    'milestone',
    'title_desc',
    'labels',
    'comment',
  ]
  const triggerTypeOptions = computed(() =>
    TRIGGER_TYPES.map((v) => ({ label: t(`gitlab.modal.option.trigger.${v}`), value: v })),
  )
  const ACTION_TYPES = [
    'set_label',
    'set_state',
    'set_due',
    'set_assignees',
    'set_estimate',
    'set_milestone',
    'set_title_desc',
    'reconcile_labels',
    'post_comment',
  ]
  const actionTypeOptions = computed(() =>
    ACTION_TYPES.map((v) => ({ label: t(`gitlab.modal.option.action.${v}`), value: v })),
  )
  const stateOptions = computed(() => [
    { label: t('gitlab.modal.option.state.fromFlag'), value: '' },
    { label: t('gitlab.modal.option.state.closed'), value: 'closed' },
    { label: t('gitlab.modal.option.state.opened'), value: 'opened' },
  ])
  const dateKindOptions = computed(() =>
    ['due', 'start'].map((v) => ({ label: t(`gitlab.modal.option.dateKind.${v}`), value: v })),
  )
  const completionOptions = computed(() => [
    { label: t('gitlab.modal.option.completion.any'), value: null },
    { label: t('gitlab.modal.option.completion.became'), value: true },
    { label: t('gitlab.modal.option.completion.cleared'), value: false },
  ])
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
  const triggerLabel = (v) => triggerTypeOptions.value.find((o) => o.value === v)?.label || v
  const actionLabel = (v) => actionTypeOptions.value.find((o) => o.value === v)?.label || v
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
  // Captions follow the reader's language; the values stored in a rule do not. A
  // rule records the column's server-side name (that is what the sync matches on,
  // see internal/gitlab/writeback.go), so `value` and columnNameById stay on the
  // raw name while only the labels are translated (#2800).
  const columnOptions = ref([]) // {label:caption, value:name} — for label-rules value maps
  const columnIdOptions = ref([]) // {label:caption, value:id} — for column-move bindings
  const columnNameById = ref({}) // id → raw name, to stamp column_name on save
  const columnLabelById = ref({}) // id → caption, for the row summaries
  const saving = ref(false)
  // Only covers the POST that kicks the sync off — the sync itself runs detached on
  // the server, so nothing here blocks on it.
  const syncing = ref(false)

  const intervalOptions = computed(() => [
    { label: t('gitlab.modal.option.interval.manual'), value: 0 },
    { label: t('gitlab.modal.option.interval.min5'), value: 300 },
    { label: t('gitlab.modal.option.interval.min15'), value: 900 },
    { label: t('gitlab.modal.option.interval.hour'), value: 3600 },
  ])
  // Periodic FULL sweep (catches deletes/drift an incremental pull can't see). 0 = off
  // — a full sync then runs only on the very first sync or via «Полная синхронизация».
  const fullIntervalOptions = computed(() => [
    { label: t('gitlab.modal.option.fullInterval.off'), value: 0 },
    { label: t('gitlab.modal.option.fullInterval.h6'), value: 21600 },
    { label: t('gitlab.modal.option.fullInterval.h12'), value: 43200 },
    { label: t('gitlab.modal.option.fullInterval.d1'), value: 86400 },
    { label: t('gitlab.modal.option.fullInterval.d2'), value: 172800 },
    { label: t('gitlab.modal.option.fullInterval.w1'), value: 604800 },
  ])
  const actionOptions = computed(() =>
    ['status', 'priority', 'board', 'milestone', 'tag', 'group', 'ignore'].map((v) => ({
      label: t(`gitlab.modal.option.ruleAction.${v}`),
      value: v,
    })),
  )
  const matchTypeOptions = computed(() =>
    ['prefix', 'regex'].map((v) => ({ label: t(`gitlab.modal.option.matchType.${v}`), value: v })),
  )
  const defaultActionOptions = computed(() =>
    ['tag', 'ignore'].map((v) => ({
      label: t(`gitlab.modal.option.defaultAction.${v}`),
      value: v,
    })),
  )
  // Target options for a rule's value-map, by action.
  function mapTargetOptions(action) {
    if (action === 'status') return columnOptions.value
    if (action === 'priority') return priorityLevelOptions.value
    if (action === 'board') return boardOptions.value
    return []
  }
  const mapActions = ['status', 'priority', 'board']
  const dueSourceOptions = computed(() =>
    ['issue_milestone', 'issue', 'milestone', 'off'].map((v) => ({
      label: t(`gitlab.modal.option.dueSource.${v}`),
      value: v,
    })),
  )
  const startSourceOptions = computed(() =>
    ['created', 'milestone', 'off'].map((v) => ({
      label: t(`gitlab.modal.option.startSource.${v}`),
      value: v,
    })),
  )
  // Computed, not a module constant: the level names come from the catalog, so a
  // frozen array would keep the language of the first render (#2799).
  const priorityLevelOptions = computed(() => priorityOptions())
  // Priority qualifier for a binding trigger (null = any level). Declared here so it
  // follows priorityLevelOptions (avoids a temporal-dead-zone reference).
  const priorityQualOptions = computed(() => [
    { label: t('gitlab.modal.actions.anyPriority'), value: null },
    ...priorityLevelOptions.value,
  ])

  const lastSyncedText = computed(() =>
    lastSynced.value
      ? formatDateTime(lastSynced.value, { day: '2-digit', month: '2-digit' })
      : t('gitlab.modal.sync.never'),
  )

  const lastWebhookText = computed(() =>
    lastWebhook.value
      ? formatDateTime(lastWebhook.value, { day: '2-digit', month: '2-digit' })
      : t('gitlab.modal.webhook.never'),
  )

  // enableWebhook generates (or rotates) the shared secret. The plaintext comes back
  // exactly once, so it is held in webhookSecret until the binding is reloaded —
  // rotating without copying it means pasting a new one into GitLab again.
  async function enableWebhook() {
    if (!currentId.value) return
    webhookBusy.value = true
    try {
      const { data } = await glApi.enableWebhook(wsId(), currentId.value)
      const view = data.integration || {}
      webhookEnabled.value = view.webhook_enabled === true
      webhookUrl.value = view.webhook_url || ''
      webhookSecret.value = data.secret || ''
      // Refresh only the cached row, never via loadList(): re-applying the binding
      // would clear webhookSecret, and the secret is not fetchable a second time.
      patchCachedBinding(view)
    } catch {
      // The error the api client hands over is already humanized, but it names
      // the transport rather than the action; the webhook line says which button
      // failed, which is what the reader needs here.
      message.error(t('gitlab.modal.webhook.failed'))
    } finally {
      webhookBusy.value = false
    }
  }

  async function disableWebhook() {
    if (!currentId.value) return
    webhookBusy.value = true
    try {
      const { data } = await glApi.disableWebhook(wsId(), currentId.value)
      webhookEnabled.value = false
      webhookUrl.value = ''
      webhookSecret.value = ''
      patchCachedBinding(data || {})
    } catch {
      // The error the api client hands over is already humanized, but it names
      // the transport rather than the action; the webhook line says which button
      // failed, which is what the reader needs here.
      message.error(t('gitlab.modal.webhook.failed'))
    } finally {
      webhookBusy.value = false
    }
  }

  // patchCachedBinding folds a server view back into the loaded list, so switching
  // bindings and back shows the current webhook state without a full reload.
  function patchCachedBinding(view) {
    if (!view || !view.id) return
    const i = integrations.value.findIndex((b) => b.id === view.id)
    if (i >= 0) integrations.value[i] = { ...integrations.value[i], ...view }
  }

  // copyWebhook puts a value on the clipboard. Falls back to a notice rather than
  // failing silently: the modal may run over plain HTTP, where the API is missing.
  async function copyWebhook(value) {
    try {
      await navigator.clipboard.writeText(value)
      message.success(t('gitlab.modal.webhook.copied'))
    } catch {
      message.warning(t('gitlab.modal.webhook.copyFailed'))
    }
  }

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
      columnLabelById.value = {}
      return
    }
    try {
      const cols = (await boardsApi.columns(id)).data || []
      columnOptions.value = cols.map((c) => ({ label: columnName(c), value: c.name }))
      columnIdOptions.value = cols.map((c) => ({ label: columnName(c), value: c.id }))
      const m = {}
      const labels = {}
      for (const c of cols) {
        m[c.id] = c.name
        labels[c.id] = columnName(c)
      }
      columnNameById.value = m
      columnLabelById.value = labels
    } catch {
      columnOptions.value = []
      columnIdOptions.value = []
      columnNameById.value = {}
      columnLabelById.value = {}
    }
  }

  // loadList fetches every binding of the workspace, then selects the first (or
  // starts a fresh form when there are none).
  async function loadList() {
    if (!wsId()) return
    try {
      const { data } = await glApi.listIntegrations(wsId())
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
    return (
      b.name || b.project_path || (board ? board.label : t('gitlab.modal.binding.fallbackLabel'))
    )
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
      relationsSync.value = data.relations_sync !== 'off'
      lastSynced.value = data.last_synced_at || null
      webhookEnabled.value = data.webhook_enabled === true
      webhookUrl.value = data.webhook_url || ''
      lastWebhook.value = data.last_webhook_at || null
      webhookSecret.value = '' // shown once, on generation; never re-read
      const wb = data.writeback || {}
      wbEnabled.value = wb.enabled === true
      wbCreate.value = wb.push_create === true
      wbFetchTemplates.value = wb.fetch_templates === true
      wbChildren.value = wb.push_children === true
      wbAutoGroup.value = wb.auto_group_on_child === true
      wbGroupLabel.value = wb.group_label || ''
      wbAttachments.value = wb.push_attachments !== false
      wbConflictPolicy.value = ['tessera', 'gitlab'].includes(wb.conflict_policy)
        ? wb.conflict_policy
        : 'ask'
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
        // «лейбл → этап» (#3086): the stage name, «{value}» = the label's value.
        template: rule.template || '',
        // Per-rule switch (#3087); stored inverted, so every older rule is on.
        enabled: rule.disabled !== true,
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
    markClean()
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
    // `tr`, not `t`: the i18n `t` is in scope here and must not be shadowed.
    const tr = b.trigger
    switch (tr.type) {
      case 'column':
        return t('gitlab.modal.summary.columnMove', {
          name:
            columnLabelById.value[tr.column_id] ||
            tr.column_name ||
            t('gitlab.modal.summary.unknown'),
        })
      case 'priority':
        return tr.priority == null
          ? t('gitlab.modal.summary.priorityAny')
          : t('gitlab.modal.summary.priority', { name: priorityLabel(tr.priority) })
      case 'completion':
        return tr.completed == null
          ? t('gitlab.modal.summary.completionFlag')
          : tr.completed
            ? t('gitlab.modal.summary.completionBecame')
            : t('gitlab.modal.summary.completionCleared')
      case 'due':
        return tr.date_kind === 'start'
          ? t('gitlab.modal.summary.startChange')
          : t('gitlab.modal.summary.dueChange')
      default:
        return triggerLabel(tr.type)
    }
  }
  function bindingActionText(b) {
    const a = b.action
    switch (a.type) {
      case 'set_label':
        return t('gitlab.modal.summary.setLabel', {
          label: a.label || t('gitlab.modal.summary.unknown'),
        })
      case 'set_state':
        return a.state === 'closed'
          ? t('gitlab.modal.summary.closeIssue')
          : a.state === 'opened'
            ? t('gitlab.modal.summary.openIssue')
            : t('gitlab.modal.summary.toggleIssue')
      case 'post_comment':
        return a.add_marker
          ? t('gitlab.modal.summary.commentMarker')
          : t('gitlab.modal.summary.comment')
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
      template: '',
      enabled: true,
      label: '',
      map: [],
    })
  }
  function addMapRow(rule) {
    rule.map.push({ k: '', v: '' })
  }

  // The label rules as save() sends them — also what the rule counters and
  // «Проверить лейбл» (#3087) run on, so they describe the form, not the server.
  function rulesPayload() {
    return {
      default_column: defaultColumn.value,
      default_action: defaultAction.value,
      tag_keep_prefix: tagKeepPrefix.value,
      rules: rules.value.map((rule) => {
        const out = { match: rule.match, match_type: rule.match_type, action: rule.action }
        if (rule.action === 'tag') out.keep_prefix = rule.keep_prefix
        if (rule.action === 'milestone' && rule.template?.trim()) out.template = rule.template.trim()
        if (rule.enabled === false) out.disabled = true
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
  }

  // Counters by rule + the check of one label (#3087), on the rules as edited.
  // A binding not saved yet has no tasks to count; the check still answers.
  async function previewRules(label = '') {
    const { data } = await glApi.previewRules(wsId(), {
      integration_id: currentId.value || null,
      label_rules: rulesPayload(),
      label,
    })
    return data
  }

  async function save() {
    if (!projectPath.value.trim() || !boardId.value) {
      message.warning(t('gitlab.modal.binding.needProjectAndBoard'))
      return
    }
    const label_rules = rulesPayload()
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
        relations_sync: relationsSync.value ? 'pull' : 'off',
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
          push_children: wbChildren.value,
          // Both sub-options only mean anything with the parent flag on, so they are
          // cleared with it rather than kept as dormant state (same rule as templates).
          auto_group_on_child: wbChildren.value && wbAutoGroup.value,
          group_label: wbChildren.value ? wbGroupLabel.value.trim() : '',
          push_attachments: wbAttachments.value,
          conflict_policy: wbConflictPolicy.value,
          // The binding table fully replaces the legacy toggles; a non-empty set makes
          // the backend ignore them entirely.
          bindings: wbEnabled.value ? bindings.value.map(serializeBinding) : [],
        },
      }
      const { data } = currentId.value
        ? await glApi.updateIntegration(wsId(), currentId.value, payload)
        : await glApi.createIntegration(wsId(), payload)
      currentId.value = data.id
      lastSynced.value = data.last_synced_at || lastSynced.value
      // Refresh the binding list so the selector reflects the new/updated row.
      try {
        const list = await glApi.listIntegrations(wsId())
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
      markClean()
      message.success(t('gitlab.modal.binding.saved'))
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
      await glApi.deleteIntegration(wsId(), currentId.value)
      message.success(t('gitlab.modal.binding.deleted'))
      await loadList()
    } catch (e) {
      message.error(e.message)
    }
  }

  async function syncNow(mode) {
    if (!currentId.value) {
      message.warning(t('gitlab.modal.sync.saveFirst'))
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
      const { data } = await glApi.sync(wsId(), currentId.value, mode)
      tourEvent('gitlab.synced')
      if (data?.already_running) {
        message.warning(t('gitlab.modal.sync.alreadyRunning'))
      } else {
        message.info(
          mode === 'full' ? t('gitlab.modal.sync.startedFull') : t('gitlab.modal.sync.started'),
        )
      }
      onSyncStarted?.()
    } catch (e) {
      message.error(e.message)
    } finally {
      syncing.value = false
    }
  }

  // ── v2 (#2975): the window saves the binding explicitly — the form is one PUT
  // of the whole binding, so it keeps «изменено · Сохранить ⌘S / Сбросить» rather
  // than writing on every keystroke. `dirty` compares what save() would send with
  // what was loaded or saved last.
  function formState() {
    return JSON.stringify({
      name: name.value,
      projectPath: projectPath.value,
      boardId: boardId.value,
      enabled: enabled.value,
      intervalSec: intervalSec.value,
      fullIntervalSec: fullIntervalSec.value,
      dueSource: dueSource.value,
      startSource: startSource.value,
      relationsSync: relationsSync.value,
      scope: scope.value,
      closedPolicy: closedPolicy.value,
      closedAfter: closedAfter.value,
      defaultColumn: defaultColumn.value,
      defaultAction: defaultAction.value,
      tagKeepPrefix: tagKeepPrefix.value,
      rules: rules.value,
      wb: [
        wbEnabled.value,
        wbCreate.value,
        wbFetchTemplates.value,
        wbChildren.value,
        wbAutoGroup.value,
        wbGroupLabel.value,
        wbAttachments.value,
        wbConflictPolicy.value,
      ],
      bindings: bindings.value,
    })
  }
  const cleanState = ref('')
  function markClean() {
    cleanState.value = formState()
  }
  const dirty = computed(() => !!cleanState.value && formState() !== cleanState.value)
  // Drop the edits: the current binding as the server has it (or a fresh form).
  async function resetForm() {
    if (currentId.value) await selectBinding(currentId.value)
    else newBinding()
  }

  // Rules are matched top to bottom, the first that fits wins — so their order
  // is editable (⌥↑/⌥↓ on a row, «Выше / Ниже» in its menu).
  function moveRule(i, delta) {
    const j = i + delta
    if (j < 0 || j >= rules.value.length) return
    const next = rules.value.slice()
    ;[next[i], next[j]] = [next[j], next[i]]
    rules.value = next
  }
  function removeRule(i) {
    rules.value = rules.value.filter((_, j) => j !== i)
  }

  const currentBinding = computed(
    () => integrations.value.find((b) => b.id === currentId.value) || null,
  )
  // «2 мин назад» for the header status.
  function ago(iso) {
    if (!iso) return ''
    const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000))
    if (s < 60) return t('gitlab.window.agoNow')
    if (s < 3600) return t('gitlab.window.agoMin', { n: Math.floor(s / 60) })
    if (s < 86400) return t('gitlab.window.agoHour', { n: Math.floor(s / 3600) })
    return formatDateTime(iso, { day: '2-digit', month: 'short' })
  }
  async function loadConflictCount() {
    try {
      const { data } = await glApi.conflicts(wsId())
      conflictCount.value = (data || []).length
    } catch {
      conflictCount.value = 0
    }
  }
  const conflictCount = ref(0)

  // Opening the window: the connection, the boards and the bindings.
  async function open() {
    if (!gl.loaded) await gl.load()
    await loadBoards()
    await loadList()
    loadConflictCount()
  }

  return {
    baseInput,
    tokenInput,
    connecting,
    connect,
    disconnect,
    integrations,
    currentId,
    defaultRules,
    serviceConfigured,
    isAdmin,
    name,
    scope,
    closedPolicy,
    closedAfter,
    scopeOptions,
    closedPolicyOptions,
    projectPath,
    boardId,
    enabled,
    intervalSec,
    fullIntervalSec,
    dueSource,
    startSource,
    relationsSync,
    lastSynced,
    webhookEnabled,
    webhookUrl,
    lastWebhook,
    webhookSecret,
    webhookBusy,
    wbEnabled,
    wbCreate,
    wbFetchTemplates,
    wbChildren,
    wbAutoGroup,
    wbGroupLabel,
    wbAttachments,
    wbConflictPolicy,
    rulesPayload,
    previewRules,
    bindings,
    TRIGGER_TYPES,
    triggerTypeOptions,
    ACTION_TYPES,
    actionTypeOptions,
    stateOptions,
    dateKindOptions,
    completionOptions,
    DEFAULT_ACTION_FOR,
    triggerLabel,
    actionLabel,
    estimationUnit,
    defaultColumn,
    defaultAction,
    tagKeepPrefix,
    rules,
    boardOptions,
    boardProject,
    loadedPrefixNames,
    targetProjectId,
    columnOptions,
    columnIdOptions,
    columnNameById,
    columnLabelById,
    saving,
    syncing,
    intervalOptions,
    fullIntervalOptions,
    actionOptions,
    matchTypeOptions,
    defaultActionOptions,
    mapTargetOptions,
    mapActions,
    dueSourceOptions,
    startSourceOptions,
    priorityLevelOptions,
    priorityQualOptions,
    lastSyncedText,
    lastWebhookText,
    enableWebhook,
    disableWebhook,
    patchCachedBinding,
    copyWebhook,
    loadBoards,
    loadPrefixNames,
    loadColumns,
    loadList,
    bindingLabel,
    bindingOptions,
    selectBinding,
    newBinding,
    applyBinding,
    onBoardChange,
    normalizeBinding,
    synthesizeBindings,
    bindingTriggerText,
    bindingActionText,
    addBinding,
    removeBinding,
    onBindingTrigger,
    onBindingAction,
    serializeBinding,
    addRule,
    addMapRow,
    save,
    deleteBinding,
    syncNow,
    formState,
    markClean,
    dirty,
    resetForm,
    moveRule,
    removeRule,
    currentBinding,
    ago,
    loadConflictCount,
    conflictCount,
    open,
    gl,
    ws,
    formatDateTime,
  }
}
