<script setup>
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NCard, NInput, NButton, NIcon, NTooltip, useMessage } from 'naive-ui'
import { AddOutline, TrashOutline, ArrowUpOutline, ArrowDownOutline } from '@vicons/ionicons5'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { canonCommandKey, isValidCommandKey } from '@/utils/commands'

// Editor for a workspace's custom command dictionary — the `/`-popup entries
// that are only text (they never execute; the built-in list below is what the
// backend actually runs). Saved as the complete desired state in one PUT, like
// the tag-prefix editor.
const props = defineProps({
  show: { type: Boolean, default: false },
  workspaceId: { type: String, default: '' },
})
const emit = defineEmits(['update:show'])

const { t } = useI18n()
const store = useWorkspacesStore()
const message = useMessage()
const saving = ref(false)
const rows = ref([])

// Seed from the store whenever the modal opens, so an aborted edit doesn't stick.
watch(
  () => props.show,
  (s) => {
    if (!s) return
    rows.value = (store.customCommands || []).map((c) => ({
      key: c.key,
      description: c.description || '',
    }))
  },
  { immediate: true },
)

const builtin = computed(() => store.commands.filter((c) => c.builtin))
const builtinKeys = computed(() => new Set(builtin.value.map((c) => c.key)))

// Per-row validation, shown inline — the backend rejects the whole PUT on the
// first bad key, so catching it here keeps the user from losing the other rows.
function rowError(row, i) {
  const key = canonCommandKey(row.key)
  if (!key) return t('shell.commands.error.empty')
  if (!isValidCommandKey(key)) return t('shell.commands.error.charset')
  if (builtinKeys.value.has(key)) return t('shell.commands.error.builtin')
  if (rows.value.some((r, j) => j < i && canonCommandKey(r.key) === key))
    return t('shell.commands.error.duplicate')
  if (row.description.length > 200) return t('shell.commands.error.tooLong')
  return ''
}
const errors = computed(() => rows.value.map((r, i) => rowError(r, i)))
const canSave = computed(() => errors.value.every((e) => !e))

function addRow() {
  if (rows.value.length >= 50) return
  rows.value = [...rows.value, { key: '', description: '' }]
}
function removeRow(i) {
  rows.value = rows.value.filter((_, j) => j !== i)
}
// Order in the list is order in the popup, so it has to be movable.
function move(i, delta) {
  const j = i + delta
  if (j < 0 || j >= rows.value.length) return
  const next = [...rows.value]
  ;[next[i], next[j]] = [next[j], next[i]]
  rows.value = next
}

async function save() {
  if (!canSave.value) return
  saving.value = true
  try {
    const payload = rows.value.map((r) => ({
      key: canonCommandKey(r.key),
      description: r.description.trim(),
    }))
    const res = await wsApi.setCommands(props.workspaceId, payload)
    store.setCustomCommands(res.data || [])
    emit('update:show', false)
  } catch (e) {
    message.error(e.message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal :show="show" @update:show="$emit('update:show', $event)">
    <n-card class="cmd-modal" :title="t('shell.commands.title')" :bordered="false" role="dialog">
      <div class="cmd-body">
        <p class="cmd-hint">{{ t('shell.commands.hint') }}</p>

        <div v-if="rows.length" class="cmd-rows">
          <div v-for="(row, i) in rows" :key="i" class="cmd-row">
            <div class="cmd-key">
              <n-input
                v-model:value="row.key"
                placeholder="approve"
                :status="errors[i] ? 'error' : undefined"
                :maxlength="32"
              >
                <template #prefix><span class="cmd-slash">/</span></template>
              </n-input>
              <span v-if="errors[i]" class="cmd-err">{{ errors[i] }}</span>
            </div>
            <n-input
              v-model:value="row.description"
              :placeholder="t('shell.commands.descPlaceholder')"
              :maxlength="200"
              class="cmd-desc"
            />
            <div class="cmd-actions">
              <n-button quaternary circle size="tiny" :disabled="i === 0" @click="move(i, -1)">
                <n-icon :component="ArrowUpOutline" />
              </n-button>
              <n-button
                quaternary
                circle
                size="tiny"
                :disabled="i === rows.length - 1"
                @click="move(i, 1)"
              >
                <n-icon :component="ArrowDownOutline" />
              </n-button>
              <n-button quaternary circle size="tiny" @click="removeRow(i)">
                <n-icon :component="TrashOutline" />
              </n-button>
            </div>
          </div>
        </div>
        <p v-else class="cmd-empty">{{ t('shell.commands.empty') }}</p>

        <n-button
          tertiary
          size="small"
          :disabled="rows.length >= 50"
          class="cmd-add"
          @click="addRow"
        >
          <template #icon><n-icon :component="AddOutline" /></template>
          {{ t('shell.commands.add') }}
        </n-button>

        <div class="cmd-builtin">
          <div class="cmd-blabel">{{ t('shell.commands.builtin', { count: builtin.length }) }}</div>
          <div class="cmd-bgrid">
            <n-tooltip v-for="c in builtin" :key="c.key" :disabled="!c.example">
              <template #trigger>
                <span class="cmd-chip">/{{ c.key }}</span>
              </template>
              {{ c.description }}{{ c.example ? ` · ${c.example}` : '' }}
            </n-tooltip>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="cmd-foot">
          <n-button tertiary @click="$emit('update:show', false)">
            {{ t('common.action.cancel') }}
          </n-button>
          <n-button type="primary" :loading="saving" :disabled="!canSave" @click="save">
            {{ t('common.action.save') }}
          </n-button>
        </div>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.cmd-modal {
  width: 620px;
  max-width: calc(100vw - 32px);
}
.cmd-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.cmd-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--t-text3);
}
.cmd-rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 42vh;
  overflow-y: auto;
}
.cmd-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.cmd-key {
  flex: 0 0 180px;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.cmd-slash {
  font-family: ui-monospace, SFMono-Regular, 'JetBrains Mono', Menlo, Consolas, monospace;
  color: var(--t-text3);
}
.cmd-err {
  font-size: 11px;
  color: var(--t-error, #e5484d);
}
.cmd-desc {
  flex: 1 1 auto;
}
.cmd-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  padding-top: 3px;
}
.cmd-empty {
  margin: 0;
  font-size: 13px;
  color: var(--t-text3);
}
.cmd-add {
  align-self: flex-start;
}
.cmd-builtin {
  border-top: 1px solid var(--t-border);
  padding-top: 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.cmd-blabel {
  font-size: 12px;
  color: var(--t-text3);
}
.cmd-bgrid {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
}
/* Neutral grey chips: these are reference, not accent-worthy actions. */
.cmd-chip {
  font-family: ui-monospace, SFMono-Regular, 'JetBrains Mono', Menlo, Consolas, monospace;
  font-size: 12px;
  padding: 1px 7px;
  border-radius: 6px;
  background: var(--t-hover);
  color: var(--t-text2);
  cursor: default;
}
.cmd-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
