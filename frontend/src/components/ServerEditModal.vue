<script setup>
// Add / edit one entry of the desktop server rail (#2930).
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NCard, NInput, NButton, NSwitch, NAlert, NPopconfirm } from 'naive-ui'
import { DEFAULT_COLOR, normalizeUrl, serverLabel, serverUrlMessage } from '@/utils/servers'
import { initials } from '@/utils/initials'
import { hueGrad } from '@/utils/gradient'
import IconColorPicker from './IconColorPicker.vue'
import ProjectIcon from './ProjectIcon.vue'

const props = defineProps({
  show: { type: Boolean, default: false },
  // null = add a new server; a profile = edit that one.
  server: { type: Object, default: null },
})
const emit = defineEmits(['update:show', 'save', 'remove'])

const { t } = useI18n()

const name = ref('')
const url = ref('')
const color = ref(DEFAULT_COLOR)
const icon = ref('')
const insecure = ref(false)

watch(
  () => props.show,
  (open) => {
    if (!open) return
    name.value = props.server?.name || ''
    url.value = props.server?.url || ''
    color.value = props.server?.color || DEFAULT_COLOR
    icon.value = props.server?.icon || ''
    insecure.value = !!props.server?.insecure
  },
  { immediate: true },
)

// The address is the only required field — the name falls back to the host.
const cleanUrl = computed(() => normalizeUrl(url.value))
// The rule itself lives in utils/servers.js — the login-screen popover and the
// settings screen guard the same registry and have to refuse the same addresses.
const urlError = computed(() => serverUrlMessage(cleanUrl.value, t))
const canSave = computed(() => !!cleanUrl.value && !urlError.value)

const preview = computed(() => initials(name.value || serverLabel(cleanUrl.value)))

function save() {
  if (!canSave.value) return
  emit('save', {
    name: name.value.trim(),
    url: cleanUrl.value,
    color: color.value || DEFAULT_COLOR,
    icon: icon.value || '',
    insecure: insecure.value,
  })
  emit('update:show', false)
}
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <div class="se-wrap">
      <n-card class="se-card" style="width: 460px; max-width: 94vw" role="dialog">
        <template #header>
          {{ server ? t('shell.servers.editTitle') : t('shell.servers.addTitle') }}
        </template>

        <div class="se-row">
          <div class="se-preview" :style="{ backgroundImage: hueGrad(color) }">
            <ProjectIcon v-if="icon" :icon="icon" :initials="preview" :size="20" />
            <template v-else>{{ preview }}</template>
          </div>
          <IconColorPicker
            class="se-picker"
            :icon="icon"
            :color="color"
            mode="badge"
            :show-mode="false"
            :initials="preview"
            allow-upload
            @update:icon="icon = $event"
            @update:color="color = $event || DEFAULT_COLOR"
          />
        </div>

        <label class="se-label">{{ t('shell.servers.name') }}</label>
        <n-input
          v-model:value="name"
          :placeholder="serverLabel(cleanUrl) || t('shell.servers.namePlaceholder')"
        />

        <label class="se-label">{{ t('shell.servers.url') }}</label>
        <n-input v-model:value="url" :placeholder="t('shell.servers.urlPlaceholder')" />
        <div v-if="urlError" class="se-error">{{ urlError }}</div>

        <div class="se-switch">
          <n-switch v-model:value="insecure" size="small" />
          <span>{{ t('shell.servers.insecure') }}</span>
        </div>
        <!-- Honest label: TLS verification cannot be turned off from JavaScript,
             and the platform switches that can (WebKitGTK / WebView2) apply to
             the whole application rather than to one profile. The flag is stored
             so the setting survives; wiring it to the webview is its own task. -->
        <n-alert v-if="insecure" type="warning" :bordered="false" class="se-note">
          {{ t('shell.servers.insecureNote') }}
        </n-alert>

        <template #footer>
          <div class="se-actions">
            <!-- Removing purges the profile's session namespace and, for the last
                 profile, swaps in a fresh one aimed at the public default — the
                 typed address is then gone from the app entirely. One click away
                 from «Сохранить» is too close for that, so it asks first, the way
                 the board asks before deleting a column. -->
            <n-popconfirm
              v-if="server"
              :positive-button-props="{
                type: 'error',
                'data-testid': 'server-edit-remove-confirm',
              }"
              :positive-text="t('shell.servers.remove')"
              @positive-click="emit('remove', server.id)"
            >
              <template #trigger>
                <n-button quaternary type="error">{{ t('shell.servers.remove') }}</n-button>
              </template>
              {{
                t('shell.servers.removeConfirm', { name: server.name || serverLabel(server.url) })
              }}
            </n-popconfirm>
            <span class="se-spacer" />
            <n-button quaternary @click="emit('update:show', false)">
              {{ t('common.action.cancel') }}
            </n-button>
            <n-button type="primary" :disabled="!canSave" @click="save">
              {{ t('common.action.save') }}
            </n-button>
          </div>
        </template>
      </n-card>
    </div>
  </n-modal>
</template>

<style scoped>
.se-wrap {
  display: flex;
  justify-content: center;
}
.se-row {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  margin-bottom: 4px;
}
.se-preview {
  width: 44px;
  height: 44px;
  flex: none;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 16px;
  color: #fff;
  overflow: hidden;
}
.se-picker {
  width: 100%;
}
.se-label {
  display: block;
  margin: 12px 0 4px;
  font-size: 12px;
  color: var(--t-text3);
}
.se-error {
  margin-top: 4px;
  font-size: 12px;
  color: #d03050;
}
.se-switch {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
  font-size: 13px;
}
.se-note {
  margin-top: 8px;
  font-size: 12px;
}
.se-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.se-spacer {
  flex: 1;
}
</style>
