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
const dueSource = ref('issue_milestone')
const lastSynced = ref(null)
const defaultColumn = ref('')
const defaultAction = ref('tag')
const tagKeepPrefix = ref(true)
// Generic rule list. Each: { match, match_type, action, keep_prefix, map:[{k,v}] }
const rules = ref([])
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
    dueSource.value = data.due_source || 'issue_milestone'
    lastSynced.value = data.last_synced_at || null
    const r = data.label_rules || {}
    defaultColumn.value = r.default_column || ''
    defaultAction.value = r.default_action || 'tag'
    tagKeepPrefix.value = r.tag_keep_prefix !== false
    rules.value = (r.rules || []).map((rule) => ({
      match: rule.match || '',
      match_type: rule.match_type || 'prefix',
      action: rule.action || 'tag',
      keep_prefix: rule.keep_prefix !== false,
      map: Object.entries(rule.value_map || {}).map(([k, v]) => ({
        k,
        v: rule.action === 'priority' ? Number(v) : v,
      })),
    }))
    if (boardId.value) await loadColumns(boardId.value)
  } catch (e) {
    message.error(e.message)
  }
}

function onBoardChange(id) {
  boardId.value = id
  loadColumns(id)
}

function addRule() {
  rules.value.push({ match: '', match_type: 'prefix', action: 'tag', keep_prefix: true, map: [] })
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

          <n-text depth="3" class="lbl">Источник срока</n-text>
          <n-select v-model:value="dueSource" :options="dueSourceOptions" size="small" />

          <n-text depth="3" class="lbl">Включена</n-text>
          <div><n-switch v-model:value="enabled" /></div>
        </div>

        <!-- Generic rule engine -->
        <h4 class="gl-h gl-h-sub">Правила лейблов</h4>
        <div class="gl-grid">
          <n-text depth="3" class="lbl">Колонка по умолчанию</n-text>
          <n-select
            v-model:value="defaultColumn"
            :options="columnOptions"
            size="small"
            placeholder="напр. К работе"
          />
          <n-text depth="3" class="lbl">Прочие лейблы</n-text>
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
            <n-button text size="tiny" type="primary" class="gl-add" @click="addMapRow(rule)">
              <n-icon :component="AddOutline" /> значение
            </n-button>
          </div>
        </div>
        <n-button text size="small" type="primary" class="gl-add" @click="addRule">
          <n-icon :component="AddOutline" /> правило
        </n-button>

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
</style>
