<script setup>
// Settings → «Приложение» (v2, #3106): the desktop client's own settings —
// server address, launch at login, self-update. In a browser there is nothing
// to set here, so the section says so and offers the client instead.
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NInput, NSwitch } from 'naive-ui'
import { isTauri, serverBase, setServerBase } from '@/utils/serverBase'
import { serverUrlMessage } from '@/utils/servers'
import { useDesktopUpdate } from '@/composables/useDesktopUpdate'
import DownloadAppButton from '@/components/DownloadAppButton.vue'
import './settings.css'

const { t } = useI18n()
const isDesktop = isTauri()

const serverAddr = ref(serverBase())
// An address that is not an absolute http(s) origin is refused, not stored.
const serverAddrError = computed(() => serverUrlMessage(serverAddr.value, t))
function saveServerAddr() {
  if (!setServerBase(serverAddr.value)) return
  location.reload()
}

const {
  busy: updBusy,
  status: updStatus,
  newVersion: updVersion,
  error: updError,
  check: updCheck,
  install: updInstall,
} = useDesktopUpdate()

const autostart = ref(false)
const autostartBusy = ref(false)
async function loadAutostart() {
  if (!isDesktop) return
  try {
    const { isEnabled } = await import('@tauri-apps/plugin-autostart')
    autostart.value = await isEnabled()
  } catch {
    /* plugin unavailable — leave off */
  }
}
async function toggleAutostart(v) {
  autostartBusy.value = true
  try {
    const mod = await import('@tauri-apps/plugin-autostart')
    if (v) await mod.enable()
    else await mod.disable()
    autostart.value = v
  } catch {
    autostart.value = !v
  } finally {
    autostartBusy.value = false
  }
}
onMounted(loadAutostart)
</script>

<template>
  <div class="sx">
    <template v-if="isDesktop">
      <section class="st-card">
        <label class="st-field">
          <span class="st-label">{{ t('settings.app.serverAddr') }}</span>
          <n-input
            v-model:value="serverAddr"
            :placeholder="t('settings.app.serverAddrPlaceholder')"
          />
        </label>
        <div v-if="serverAddrError" class="st-err">{{ serverAddrError }}</div>
        <div class="st-actions">
          <n-button type="primary" :disabled="!!serverAddrError" @click="saveServerAddr">{{
            t('settings.app.saveAndRestart')
          }}</n-button>
        </div>
      </section>
      <section class="st-card">
        <label class="st-toggle">
          <span class="st-toggle-l">
            {{ t('settings.app.autostart') }}
            <span class="st-toggle-sub">{{ t('settings.app.autostartHint') }}</span>
          </span>
          <n-switch :value="autostart" :loading="autostartBusy" @update:value="toggleAutostart" />
        </label>
      </section>
      <section class="st-card">
        <div class="st-card-head">
          <span class="st-card-title">{{ t('settings.app.updates') }}</span>
        </div>
        <div class="sx-upd">
          <n-button secondary :loading="updBusy" @click="updCheck(false)">{{
            t('settings.app.check')
          }}</n-button>
          <n-button v-if="updStatus === 'available'" type="primary" @click="updInstall">
            {{ t('settings.app.install', { version: updVersion }) }}
          </n-button>
        </div>
        <p class="st-hint">
          <template v-if="updStatus === 'none'">{{ t('settings.app.upToDate') }}</template>
          <template v-else-if="updStatus === 'available'">{{
            t('settings.app.available', { version: updVersion })
          }}</template>
          <template v-else-if="updStatus === 'downloading'">{{
            t('settings.app.downloading')
          }}</template>
          <template v-else-if="updStatus === 'error'">{{
            t('settings.app.error', { error: updError })
          }}</template>
        </p>
      </section>
    </template>
    <section v-else class="st-card" data-testid="settings-app-web">
      <p class="st-hint">{{ t('settings.app.webOnly') }}</p>
      <div class="sx-upd"><DownloadAppButton /></div>
    </section>
  </div>
</template>

<style scoped>
.sx {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.sx-upd {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}
</style>
