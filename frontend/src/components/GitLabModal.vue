<script setup>
import { ref, watch, computed } from 'vue'
import {
  NModal,
  NCard,
  NInput,
  NSelect,
  NSwitch,
  NButton,
  NText,
  NIcon,
  NPopconfirm,
  useMessage,
} from 'naive-ui'
import { TrashOutline, AddOutline, SyncOutline, GitBranchOutline } from '@vicons/ionicons5'
import { gitlab as glApi, projects as projApi, boards as boardsApi } from '@/api'
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
const lastSynced = ref(null)
const statusPrefix = ref('S: ')
const priorityPrefix = ref('P: ')
const defaultColumn = ref('')
const tagMode = ref('all')
const tagKeepPrefix = ref(true)
const statusRows = ref([]) // [{ k: glStatus, v: columnName }]
const prioRows = ref([]) // [{ k: glPriority, v: level }]
const boardOptions = ref([])
const columnOptions = ref([])
const saving = ref(false)
const syncing = ref(false)

const intervalOptions = [
  { label: 'Вручную (выкл.)', value: 0 },
  { label: 'Каждые 5 минут', value: 300 },
  { label: 'Каждые 15 минут', value: 900 },
  { label: 'Каждый час', value: 3600 },
]
const tagModeOptions = [
  { label: 'Создавать теги', value: 'all' },
  { label: 'Игнорировать', value: 'ignore' },
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
  for (const p of ws.projects) {
    try {
      const bs = (await projApi.boards(p.id)).data || []
      for (const b of bs) all.push({ label: `${p.name} / ${b.name}`, value: b.id })
    } catch {
      /* skip a project we can't read */
    }
  }
  boardOptions.value = all
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
    lastSynced.value = data.last_synced_at || null
    const r = data.label_rules || {}
    statusPrefix.value = r.status_prefix ?? 'S: '
    priorityPrefix.value = r.priority_prefix ?? 'P: '
    defaultColumn.value = r.default_column || ''
    tagMode.value = r.tag_mode || 'all'
    tagKeepPrefix.value = r.tag_keep_prefix !== false
    statusRows.value = Object.entries(r.status_to_column || {}).map(([k, v]) => ({ k, v }))
    prioRows.value = Object.entries(r.priority_map || {}).map(([k, v]) => ({ k, v: Number(v) }))
    if (boardId.value) await loadColumns(boardId.value)
  } catch (e) {
    message.error(e.message)
  }
}

function onBoardChange(id) {
  boardId.value = id
  loadColumns(id)
}

function addStatusRow() {
  statusRows.value.push({ k: '', v: '' })
}
function addPrioRow() {
  prioRows.value.push({ k: '', v: 0 })
}

async function save() {
  if (!projectPath.value.trim() || !boardId.value) {
    message.warning('Укажите путь к проекту GitLab и доску назначения')
    return
  }
  const label_rules = {
    status_prefix: statusPrefix.value,
    status_to_column: Object.fromEntries(
      statusRows.value.filter((r) => r.k && r.v).map((r) => [r.k, r.v]),
    ),
    default_column: defaultColumn.value,
    priority_prefix: priorityPrefix.value,
    priority_map: Object.fromEntries(
      prioRows.value.filter((r) => r.k !== '').map((r) => [r.k, Number(r.v)]),
    ),
    tag_mode: tagMode.value,
    tag_keep_prefix: tagKeepPrefix.value,
  }
  saving.value = true
  try {
    const { data } = await glApi.setIntegration(props.wsId, {
      project_path: projectPath.value.trim(),
      board_id: boardId.value,
      enabled: enabled.value,
      sync_interval_sec: Number(intervalSec.value),
      label_rules,
    })
    lastSynced.value = data.last_synced_at || lastSynced.value
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
    const { data } = await glApi.sync(props.wsId)
    lastSynced.value = new Date().toISOString()
    message.success(`Синхронизировано: ${data.total} задач (+${data.created} новых, ${data.updated} обновлено)`)
    emit('synced')
  } catch (e) {
    message.error(e.message)
  } finally {
    syncing.value = false
  }
}

watch(
  () => [props.show, props.wsId],
  async ([show]) => {
    if (!show) return
    if (!gl.loaded) await gl.load()
    await loadBoards()
    await loadIntegration()
  },
  { immediate: false },
)
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <n-card
      style="width: 580px; max-width: 94vw; max-height: 88vh; overflow-y: auto"
      role="dialog"
    >
      <template #header>
        <span class="gl-title">
          <n-icon :component="GitBranchOutline" class="grad-icon" /> GitLab
        </span>
      </template>

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

          <n-text depth="3" class="lbl">Включена</n-text>
          <div><n-switch v-model:value="enabled" /></div>
        </div>

        <!-- Rule engine -->
        <h4 class="gl-h gl-h-sub">Правила лейблов → доска</h4>

        <div class="gl-grid">
          <n-text depth="3" class="lbl">Префикс статуса</n-text>
          <n-input v-model:value="statusPrefix" size="small" placeholder="S: " />
          <n-text depth="3" class="lbl">Колонка по умолчанию</n-text>
          <n-select
            v-model:value="defaultColumn"
            :options="columnOptions"
            size="small"
            placeholder="напр. К работе"
          />
        </div>

        <div class="gl-rules">
          <div class="gl-rules-head">
            <span>Статус GitLab</span><span>→ Колонка</span><span></span>
          </div>
          <div v-for="(row, i) in statusRows" :key="`s${i}`" class="gl-rule">
            <n-input v-model:value="row.k" size="small" placeholder="In review" />
            <n-select v-model:value="row.v" :options="columnOptions" size="small" placeholder="колонка" />
            <n-button text size="tiny" type="error" @click="statusRows.splice(i, 1)">
              <n-icon :component="TrashOutline" />
            </n-button>
          </div>
          <n-button text size="tiny" type="primary" class="gl-add" @click="addStatusRow">
            <n-icon :component="AddOutline" /> статус
          </n-button>
        </div>

        <div class="gl-grid gl-grid-top">
          <n-text depth="3" class="lbl">Префикс приоритета</n-text>
          <n-input v-model:value="priorityPrefix" size="small" placeholder="P: " />
        </div>
        <div class="gl-rules">
          <div class="gl-rules-head">
            <span>Приоритет GitLab</span><span>→ Уровень</span><span></span>
          </div>
          <div v-for="(row, i) in prioRows" :key="`p${i}`" class="gl-rule">
            <n-input v-model:value="row.k" size="small" placeholder="Critical" />
            <n-select v-model:value="row.v" :options="priorityLevelOptions" size="small" />
            <n-button text size="tiny" type="error" @click="prioRows.splice(i, 1)">
              <n-icon :component="TrashOutline" />
            </n-button>
          </div>
          <n-button text size="tiny" type="primary" class="gl-add" @click="addPrioRow">
            <n-icon :component="AddOutline" /> приоритет
          </n-button>
        </div>

        <div class="gl-grid gl-grid-top">
          <n-text depth="3" class="lbl">Прочие лейблы</n-text>
          <n-select v-model:value="tagMode" :options="tagModeOptions" size="small" />
          <n-text depth="3" class="lbl">Сохранять префикс тега</n-text>
          <div><n-switch v-model:value="tagKeepPrefix" /></div>
        </div>

        <div class="gl-footer">
          <span class="gl-synced">Последний синк: {{ lastSyncedText }}</span>
          <div class="gl-footer-btns">
            <n-button size="small" :loading="syncing" @click="syncNow">
              <template #icon><n-icon :component="SyncOutline" /></template>
              Синхронизировать
            </n-button>
            <n-button type="primary" size="small" :loading="saving" @click="save">Сохранить</n-button>
          </div>
        </div>
      </section>
    </n-card>
  </n-modal>
</template>

<style scoped>
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
.gl-grid-top {
  margin-top: 12px;
}
.lbl {
  font-size: 12px;
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
</style>
