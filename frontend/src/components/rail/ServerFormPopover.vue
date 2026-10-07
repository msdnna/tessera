<script setup>
// Add / edit one entry of the desktop server rail (#2930) — a popover form at
// the rail since the v2 interaction model (#2969 / #2983); it replaced
// ServerEditModal.
//
// «Удалить сервер» purges the profile's session namespace and, for the last
// profile, swaps in a fresh one aimed at the public default — the typed address
// is then gone from the app entirely. One click away from «Сохранить» is too
// close for that, so it takes two presses (the second within 4 s).
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { NSwitch } from 'naive-ui'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import FormField from '@/components/kit/FormField.vue'
import IconColorPicker from '@/components/IconColorPicker.vue'
import { DEFAULT_COLOR, normalizeUrl, serverLabel, serverUrlMessage } from '@/utils/servers'
import { initials } from '@/utils/initials'

const props = defineProps({
  show: { type: Boolean, default: false },
  // null = add a new server; a profile = edit that one.
  server: { type: Object, default: null },
  anchor: { type: Function, default: () => null },
})
const emit = defineEmits(['update:show', 'save', 'remove'])

const { t } = useI18n()

const name = ref('')
const url = ref('')
const color = ref(DEFAULT_COLOR)
const icon = ref('')
const insecure = ref(false)
const armed = ref(false)
let disarm = null

watch(
  () => props.show,
  (open) => {
    armed.value = false
    if (!open) return
    name.value = props.server?.name || ''
    url.value = props.server?.url || ''
    color.value = props.server?.color || DEFAULT_COLOR
    icon.value = props.server?.icon || ''
    insecure.value = !!props.server?.insecure
  },
  { immediate: true },
)
onBeforeUnmount(() => clearTimeout(disarm))

// The address is the only required field — the name falls back to the host.
const cleanUrl = computed(() => normalizeUrl(url.value))
// The rule itself lives in utils/servers.js — the login-screen popover and the
// settings screen guard the same registry and have to refuse the same addresses.
const urlError = computed(() => (url.value.trim() ? serverUrlMessage(cleanUrl.value, t) : ''))
const canSave = computed(() => !!cleanUrl.value && !serverUrlMessage(cleanUrl.value, t))
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

function onRemove() {
  if (!props.server) return
  if (!armed.value) {
    armed.value = true
    clearTimeout(disarm)
    disarm = setTimeout(() => (armed.value = false), 4000)
    return
  }
  clearTimeout(disarm)
  armed.value = false
  emit('remove', props.server.id)
}
</script>

<template>
  <PopoverForm
    :show="show"
    :anchor="anchor"
    :offset="{ x: 8, y: -8 }"
    :width="360"
    :title="server ? t('shell.servers.editTitle') : t('shell.servers.addTitle')"
    :subtitle="server ? server.name || serverLabel(server.url) : ''"
    :primary="t('common.action.save')"
    :disabled="!canSave"
    testid="server-form"
    @update:show="emit('update:show', $event)"
    @submit="save"
  >
    <FormField :label="t('shell.servers.url')" :error="urlError">
      <input
        v-model="url"
        class="kf-input"
        :placeholder="t('shell.servers.urlPlaceholder')"
        spellcheck="false"
        data-testid="server-form-url"
      />
    </FormField>
    <FormField :label="t('shell.servers.name')">
      <input
        v-model="name"
        class="kf-input"
        :placeholder="serverLabel(cleanUrl) || t('shell.servers.namePlaceholder')"
        data-testid="server-form-name"
      />
    </FormField>
    <FormField :label="t('common.picker.label')">
      <IconColorPicker
        :icon="icon"
        :color="color"
        mode="badge"
        :show-mode="false"
        :initials="preview"
        allow-upload
        @update:icon="icon = $event"
        @update:color="color = $event || DEFAULT_COLOR"
      />
    </FormField>
    <!-- Honest label: TLS verification cannot be turned off from JavaScript,
         and the platform switches that can (WebKitGTK / WebView2) apply to
         the whole application rather than to one profile. The flag is stored
         so the setting survives; wiring it to the webview is its own task. -->
    <FormField :message="insecure ? t('shell.servers.insecureNote') : ''">
      <label class="sf-switch">
        <n-switch v-model:value="insecure" size="small" />
        <span>{{ t('shell.servers.insecure') }}</span>
      </label>
    </FormField>

    <template v-if="server" #foot>
      <button
        type="button"
        class="sf-remove"
        :class="{ armed }"
        data-testid="server-form-remove"
        @click="onRemove"
      >
        {{ armed ? t('shell.servers.removeArmed') : t('shell.servers.remove') }}
      </button>
    </template>
  </PopoverForm>
</template>

<style scoped>
.sf-switch {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--t-text1);
  cursor: pointer;
}
.sf-remove {
  height: 30px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: #e0533d;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.sf-remove:hover {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 14%, var(--t-pop)),
    color-mix(in srgb, #e0533d 24%, var(--t-pop))
  );
}
.sf-remove.armed {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 86%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 86%, #fff)
  );
  color: #fff;
  font-weight: 600;
}
</style>
