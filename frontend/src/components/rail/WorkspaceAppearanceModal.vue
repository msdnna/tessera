<script setup>
// Name, colour and icon of a workspace — its tile on the rail (#2955). The
// name goes through the rename endpoint, the tile through its own one, so a
// failure in either leaves the other intact.
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NCard, NInput, NButton, useMessage } from 'naive-ui'
import IconColorPicker from '@/components/IconColorPicker.vue'
import RailTile from './RailTile.vue'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { workspaceName } from '@/utils/defaultNames'
import { initials } from '@/utils/initials'
import { humanizeError } from '@/utils/errors'
import { tileColor } from './workspaceTile'

const props = defineProps({
  show: { type: Boolean, default: false },
  workspace: { type: Object, default: null },
})
const emit = defineEmits(['update:show'])

const { t } = useI18n()
const message = useMessage()
const store = useWorkspacesStore()

const name = ref('')
const color = ref('')
const icon = ref('')
const mode = ref('badge')
const busy = ref(false)

watch(
  () => [props.show, props.workspace],
  () => {
    const w = props.workspace
    if (!props.show || !w) return
    name.value = workspaceName(w)
    color.value = tileColor(w)
    icon.value = w.icon || ''
    mode.value = w.icon_mode || 'badge'
  },
  { immediate: true },
)

const label = computed(() => initials(name.value) || '?')

async function save() {
  const w = props.workspace
  if (!w || !name.value.trim()) return
  busy.value = true
  try {
    if (name.value.trim() !== workspaceName(w)) {
      store.patchWorkspace((await wsApi.update(w.id, { name: name.value.trim() })).data)
    }
    const res = await wsApi.setAppearance(w.id, {
      color: color.value,
      icon: icon.value,
      icon_mode: mode.value,
    })
    store.patchWorkspace(res.data)
    emit('update:show', false)
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <n-card
      :title="t('shell.rail.appearance')"
      style="width: 460px; max-width: 94vw"
      role="dialog"
      :bordered="false"
      data-testid="ws-appearance"
    >
      <div class="wa-row">
        <RailTile :label="label" :color="color" :icon="icon" :icon-mode="mode" active />
        <IconColorPicker
          class="wa-picker"
          :icon="icon"
          :color="color"
          :mode="mode"
          :initials="label"
          @update:icon="icon = $event"
          @update:color="color = $event"
          @update:mode="mode = $event"
        />
      </div>
      <label class="wa-label">{{ t('shell.workspace.namePlaceholder') }}</label>
      <n-input v-model:value="name" :disabled="busy" @keyup.enter="save" />
      <template #footer>
        <div class="wa-actions">
          <n-button size="small" :disabled="busy" @click="emit('update:show', false)">
            {{ t('common.action.cancel') }}
          </n-button>
          <n-button
            type="primary"
            size="small"
            :loading="busy"
            :disabled="!name.trim()"
            @click="save"
          >
            {{ t('common.action.save') }}
          </n-button>
        </div>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.wa-row {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  margin-bottom: 14px;
}
.wa-picker {
  flex: 1;
  min-width: 0;
}
.wa-label {
  display: block;
  margin-bottom: 6px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.wa-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
