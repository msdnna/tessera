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
  LogoGitlab,
  ChevronDownOutline,
  TimeOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import { gitlab as glApi, projects as projApi, boards as boardsApi } from '@/api'
import { canonPrefix } from '@/utils/tagGroups'
import LoaderOverlay from '@/components/LoaderOverlay.vue'
import GitLabJournalModal from '@/components/GitLabJournalModal.vue'
import ConflictResolverModal from '@/components/ConflictResolverModal.vue'
import { useGitlabStore } from '@/stores/gitlab'
import { useWorkspacesStore } from '@/stores/workspaces'
import { PRIORITY_LABELS } from '@/styles/tokens'

const props = defineProps({
  show: { type: Boolean, default: false },
  wsId: { type: String, default: null },
})
const emit = defineEmits(['update:show', 'synced'])

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

// ── integration (per-workspace) ──
const projectPath = ref('')
const boardId = ref(null)
const enabled = ref(true)
const intervalSec = ref(0)
const dueSource = ref('issue_milestone')
const startSource = ref('created')
const lastSynced = ref(null)
// ── write-back (Tessera → GitLab), opt-in; all off by default ──
const wbEnabled = ref(false)
const wbState = ref(false)
const wbPriority = ref(false)
const wbComments = ref(false)
const wbLabels = ref(false)
const wbDue = ref(false)
const wbAssignees = ref(false)
const wbEstimate = ref(false)
const wbMilestone = ref(false) // push the task's GitLab-linked milestone to the issue
const wbTitleDesc = ref(false) // push task title/description to the issue (conflict-checked)
const wbCreate = ref(false) // allow creating GitLab issues from tasks (independent of write-back)
const wbFetchTemplates = ref(false) // offer repo issue templates when creating
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
const columnOptions = ref([])
const saving = ref(false)
const syncing = ref(false)
// The sync runs in the background (so a large batch can't drop the long request);
// we poll the journal for the result. Meanwhile, friendly cross-fading captions on
// the same branded LoaderOverlay used by the connection overlay.
const SYNC_MESSAGES = [
  'Подключаемся к GitLab…',
  'Загружаем задачи из проектов…',
  'Сопоставляем метки и обновляем доски…',
  'Почти готово…',
]

const intervalOptions = [
  { label: 'Вручную (выкл.)', value: 0 },
  { label: 'Каждые 5 минут', value: 300 },
  { label: 'Каждые 15 минут', value: 900 },
  { label: 'Каждый час', value: 3600 },
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
    if (rule.match_type === 'prefix') rule.label = loadedPrefixNames.value[canonPrefix(rule.match)] || ''
  }
}

async function loadColumns(id) {
  if (!id) {
    columnOptions.value = []
    return
  }
  try {
    const cols = (await boardsApi.columns(id)).data || []
    columnOptions.value = cols.map((c) => ({ label: c.name, value: c.name }))
  } catch {
    columnOptions.value = []
  }
}

async function loadIntegration() {
  if (!props.wsId) return
  try {
    const { data } = await glApi.getIntegration(props.wsId)
    projectPath.value = data.project_path || ''
    boardId.value = data.board_id || null
    enabled.value = data.enabled !== false
    intervalSec.value = data.sync_interval_sec || 0
    dueSource.value = data.due_source || 'issue_milestone'
    startSource.value = data.start_source || 'created'
    lastSynced.value = data.last_synced_at || null
    const wb = data.writeback || {}
    wbEnabled.value = wb.enabled === true
    wbState.value = wb.push_state === true
    wbPriority.value = wb.push_priority === true
    wbComments.value = wb.push_comments === true
    wbLabels.value = wb.push_labels === true
    wbDue.value = wb.push_due === true
    wbAssignees.value = wb.push_assignees === true
    wbEstimate.value = wb.push_estimate === true
    wbMilestone.value = wb.push_milestone === true
    wbTitleDesc.value = wb.push_title_desc === true
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
    if (boardId.value) await loadColumns(boardId.value)
    await loadPrefixNames()
  } catch (e) {
    message.error(e.message)
  }
}

function onBoardChange(id) {
  boardId.value = id
  loadColumns(id)
  loadPrefixNames()
}

function addRule() {
  rules.value.push({ match: '', match_type: 'prefix', action: 'tag', keep_prefix: true, label: '', map: [] })
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
    const { data } = await glApi.setIntegration(props.wsId, {
      project_path: projectPath.value.trim(),
      board_id: boardId.value,
      enabled: enabled.value,
      sync_interval_sec: Number(intervalSec.value),
      due_source: dueSource.value,
      start_source: startSource.value,
      label_rules,
      writeback: {
        enabled: wbEnabled.value,
        push_state: wbState.value,
        push_priority: wbPriority.value,
        push_comments: wbComments.value,
        push_labels: wbLabels.value,
        push_due: wbDue.value,
        push_assignees: wbAssignees.value,
        push_estimate: wbEstimate.value && estimationUnit.value === 'time',
        push_milestone: wbMilestone.value,
        push_title_desc: wbTitleDesc.value,
        push_create: wbCreate.value,
        fetch_templates: wbCreate.value && wbFetchTemplates.value,
      },
    })
    lastSynced.value = data.last_synced_at || lastSynced.value
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

async function syncNow() {
  syncing.value = true
  try {
    // Baseline: the newest existing run, so we can tell our new run apart.
    let baseline = 0
    try {
      const prev = (await glApi.syncRuns(props.wsId, 1)).data || []
      if (prev[0]) baseline = new Date(prev[0].started_at).getTime()
    } catch {
      /* journal not critical for starting */
    }
    // The sync now runs in the background (large batches used to drop the long
    // request). Kick it off, then poll the journal for the run to finish.
    await glApi.sync(props.wsId)
    const startedAt = Date.now()
    const MAX_MS = 30 * 60 * 1000
    let settled = false
    while (Date.now() - startedAt < MAX_MS) {
      await new Promise((r) => setTimeout(r, 2000))
      let runs
      try {
        runs = (await glApi.syncRuns(props.wsId, 5)).data || []
      } catch {
        continue
      }
      const run = runs.find(
        (r) => r.kind === 'pull' && r.finished_at && new Date(r.started_at).getTime() > baseline,
      )
      if (!run) continue
      settled = true
      if (run.status === 'error') {
        message.error('Синхронизация не удалась: ' + (run.error || 'ошибка'))
      } else {
        const created = run.created_count || 0
        const updated = run.updated_count || 0
        lastSynced.value = run.finished_at
        message.success(`Синхронизировано: ${created + updated} задач (+${created} новых, ${updated} обновлено)`)
        emit('synced')
      }
      break
    }
    if (!settled) {
      message.info('Синхронизация выполняется в фоне — результат появится в журнале')
    }
  } catch (e) {
    message.error(e.message)
  } finally {
    syncing.value = false
  }
}

// ── sync journal + write-back conflicts ──
const journalShow = ref(false)
const conflictShow = ref(false)
const conflictCount = ref(0)
const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
const syncMenu = computed(() => [
  { label: 'Журнал синхронизации', key: 'journal', icon: menuIcon(TimeOutline) },
  {
    label: conflictCount.value ? `Конфликты (${conflictCount.value})` : 'Конфликты',
    key: 'conflicts',
    icon: menuIcon(WarningOutline),
    disabled: !conflictCount.value,
  },
])
function onSyncMenu(key) {
  if (key === 'journal') journalShow.value = true
  else if (key === 'conflicts') conflictShow.value = true
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
    await loadIntegration()
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
      <n-card class="gl-card" style="width: 580px; max-width: 94vw" role="dialog">
        <template #header>
          <span class="gl-title">
            <n-icon :component="LogoGitlab" class="grad-icon" /> GitLab
          </span>
        </template>

        <div class="gl-scroll">
      <!-- ACCOUNT -->
      <section class="gl-sec">
        <h4 class="gl-h">Аккаунт</h4>
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
            <n-input v-model:value="baseInput" size="small" placeholder="https://gitlab.example.com" />
            <n-text depth="3" class="lbl">Токен (PAT, scope read_api)</n-text>
            <n-input
              v-model:value="tokenInput"
              type="password"
              show-password-on="click"
              size="small"
              placeholder="glpat-…"
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

      <!-- INTEGRATION (only when connected) -->
      <section v-if="gl.connected" class="gl-sec">
        <h4 class="gl-h">Интеграция пространства</h4>
        <div class="gl-grid">
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

          <n-text depth="3" class="lbl">Источник срока</n-text>
          <n-select v-model:value="dueSource" :options="dueSourceOptions" size="small" />

          <n-text depth="3" class="lbl">Источник начала</n-text>
          <n-select v-model:value="startSource" :options="startSourceOptions" size="small" />

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
            «Создание issue из задачи» добавляет в модалку задачи (под свойством
            «Родитель») кнопку «Создать issue в GitLab» — issue открывается из свойств и
            описания задачи под токеном владельца интеграции, после чего задача
            становится синхронизированной. Работает независимо от обратной записи
            изменений ниже. «Получение issue-templates» подтягивает шаблоны
            <code>.gitlab/issue_templates/*.md</code> — их можно выбрать над редактором
            описания перед созданием.
          </n-text>
        </p>
        <div v-if="wbEnabled" class="gl-grid">
          <n-text depth="3" class="lbl">Статус (закрыть/открыть issue)</n-text>
          <div><n-switch v-model:value="wbState" size="small" /></div>

          <n-text depth="3" class="lbl">Приоритет (метка P:)</n-text>
          <div><n-switch v-model:value="wbPriority" size="small" /></div>

          <n-text depth="3" class="lbl">Комментарии (как заметки)</n-text>
          <div><n-switch v-model:value="wbComments" size="small" /></div>

          <n-text depth="3" class="lbl">Теги (метки тег-неймспейсов)</n-text>
          <div><n-switch v-model:value="wbLabels" size="small" /></div>

          <n-text depth="3" class="lbl">Заголовок и описание</n-text>
          <div><n-switch v-model:value="wbTitleDesc" size="small" /></div>

          <n-text depth="3" class="lbl">Этап (milestone issue)</n-text>
          <div><n-switch v-model:value="wbMilestone" size="small" /></div>

          <n-text depth="3" class="lbl">Срок (due date issue)</n-text>
          <div><n-switch v-model:value="wbDue" size="small" /></div>

          <n-text depth="3" class="lbl">Исполнители (assignees issue)</n-text>
          <div><n-switch v-model:value="wbAssignees" size="small" /></div>

          <n-text depth="3" class="lbl">Оценка (timeEstimate)</n-text>
          <div class="est-wb">
            <n-switch v-model:value="wbEstimate" size="small" :disabled="estimationUnit !== 'time'" />
            <n-text v-if="estimationUnit !== 'time'" depth="3" class="est-hint">
              Доступно только когда единица оценки доски — «время».
            </n-text>
          </div>
        </div>
        <p v-if="wbEnabled" class="gl-wb-hint">
          <n-text depth="3">
            Изменения линкованных задач отправляются в GitLab под токеном владельца
            интеграции (нужен scope «api»). Статус — только открыть/закрыть issue по
            границе колонки «Готово»; метки «S:» не трогаются. Теги синхронизируют
            только метки тег-неймспейсов (не «S:»/«P:»); срок ставится на сам issue.
            Исполнители: участники проекта GitLab назначаются в пикере исполнителей
            (резолв в GL-аккаунт по members), assignee_ids issue перезаписываются.
            Оценка синхронизируется двусторонне (GL timeEstimate ↔ задача), только при
            единице оценки «время».
          </n-text>
        </p>

        <!-- Generic rule engine -->
        <h4 class="gl-h gl-h-sub">Правила меток</h4>
        <div class="gl-grid gl-grid-rules">
          <n-text depth="3" class="lbl">Колонка по умолчанию</n-text>
          <n-select
            v-model:value="defaultColumn"
            :options="columnOptions"
            size="small"
            placeholder="напр. К работе"
          />
          <n-text depth="3" class="lbl">Прочие метки</n-text>
          <n-select v-model:value="defaultAction" :options="defaultActionOptions" size="small" />
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
            <n-button dashed size="tiny" type="primary" class="gl-add" @click="addMapRow(rule)">
              <template #icon><n-icon :component="AddOutline" /></template>
              Значение
            </n-button>
          </div>
        </div>
        <n-button dashed size="small" type="primary" class="gl-add" @click="addRule">
          <template #icon><n-icon :component="AddOutline" /></template>
          Правило
        </n-button>

        <div class="gl-footer">
          <span class="gl-synced">Последняя синхронизация: {{ lastSyncedText }}</span>
          <div class="gl-footer-btns">
            <n-button-group size="medium">
              <n-button :loading="syncing" @click="syncNow">
                <template #icon><n-icon :component="SyncOutline" /></template>
                Синхронизировать
              </n-button>
              <n-dropdown trigger="click" :options="syncMenu" @select="onSyncMenu">
                <n-button :disabled="syncing" class="gl-sync-caret">
                  <template #icon><n-icon :component="ChevronDownOutline" /></template>
                </n-button>
              </n-dropdown>
            </n-button-group>
            <n-button type="primary" size="medium" :loading="saving" @click="save">Сохранить</n-button>
          </div>
        </div>
      </section>
        </div>
      </n-card>
      <!-- Branded loader while a sync runs — dims/blurs the whole modal card. -->
      <loader-overlay :show="syncing" contained :messages="SYNC_MESSAGES" :interval="2600" />
    </div>
  </n-modal>
  <git-lab-journal-modal v-model:show="journalShow" :ws-id="wsId" />
  <conflict-resolver-modal
    v-model:show="conflictShow"
    :ws-id="wsId"
    @resolved="loadConflictCount"
  />
</template>

<style scoped>
/* Caret button on the split "Синхронизировать" control keeps a tight width. */
.gl-sync-caret {
  padding-left: 6px;
  padding-right: 6px;
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
  margin: 8px 0 0;
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
