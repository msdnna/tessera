<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NInput, NButton, NSelect, NAvatar, NIcon, NSwitch } from 'naive-ui'
import { CloudUploadOutline, TrashOutline, CheckmarkCircle } from '@vicons/ionicons5'
import { users, accountFlows } from '@/api'
import NotificationSettings from '@/components/NotificationSettings.vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore, COLOR_THEMES } from '@/stores/theme'
import { timezoneOptions, countryOptions } from '@/utils/localeOptions'
import { useFormat } from '@/composables/useFormat'
import { isTauri, serverBase, setServerBase } from '@/utils/serverBase'
import { useApiImage } from '@/composables/useApiImage'
import { useDesktopUpdate } from '@/composables/useDesktopUpdate'

const { t } = useI18n()

// Desktop-only settings: configurable server address + self-update.
const isDesktop = isTauri()
const serverAddr = ref(serverBase())
function saveServerAddr() {
  setServerBase(serverAddr.value)
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

// Launch-at-login (desktop). Reflects/sets the OS autostart entry.
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
    autostart.value = !v // revert on failure
  } finally {
    autostartBusy.value = false
  }
}
onMounted(loadAutostart)

const auth = useAuthStore()
const theme = useThemeStore()

// Built from native Intl data (filterable selects show all on focus). Country
// names are language-dependent, so they follow the UI language instead of
// freezing on whatever it was at import time.
const tzOptions = timezoneOptions()
const countryOpts = computed(() => countryOptions(theme.language || 'ru'))

// ── profile ────────────────────────────────────────────────────────────────
const u = auth.user || {}
const profile = reactive({
  name: u.name || '',
  last_name: u.last_name || '',
  first_name: u.first_name || '',
  middle_name: u.middle_name || '',
  bio: u.bio || '',
  company: u.company || '',
  job_title: u.job_title || '',
})
const profileSaving = ref(false)
const profileSaved = ref(false)
const profileError = ref('')

async function saveProfile() {
  profileSaving.value = true
  profileError.value = ''
  try {
    const res = await users.updateProfile({ ...profile })
    auth.setUser(res.data)
    flash(profileSaved)
  } catch (e) {
    profileError.value = e.message
  } finally {
    profileSaving.value = false
  }
}

// ── avatar ───────────────────────────────────────────────────────────────────
const fileInput = ref(null)
const avatarBust = ref(0) // cache-buster so a re-upload refreshes the <img>
// Direct URL on web; axios-fetched blob: URL on desktop. The t buster changes the
// URL on re-upload so both the browser cache and the blob cache miss and refetch.
// Use the right separator — a GitLab-OAuth avatar_url is a proxy URL that already
// carries a query string (?ws=…&sig=…); a second "?" would corrupt its signature.
const avatarUrl = useApiImage(() => {
  const u = auth.user?.avatar_url
  if (!u) return ''
  return `${u}${u.includes('?') ? '&' : '?'}t=${avatarBust.value}`
})
const initials = computed(() => {
  const n = (auth.user?.name || auth.user?.email || '?').trim()
  const p = n.split(/\s+/)
  return (p.length >= 2 ? p[0][0] + p[1][0] : n.slice(0, 2)).toUpperCase()
})
const avatarError = ref('')

async function onAvatarPicked(e) {
  const file = e.target.files?.[0]
  if (!file) return
  avatarError.value = ''
  try {
    const res = await users.uploadAvatar(file)
    auth.setUser({ ...auth.user, avatar_url: res.data.avatar_url })
    avatarBust.value = Date.now()
  } catch (err) {
    avatarError.value = err.message
  } finally {
    if (fileInput.value) fileInput.value.value = ''
  }
}
async function removeAvatar() {
  try {
    await users.deleteAvatar()
    auth.setUser({ ...auth.user, avatar_url: '' })
  } catch (err) {
    avatarError.value = err.message
  }
}

// ── password ──────────────────────────────────────────────────────────────────
const pw = reactive({ current: '', next: '', confirm: '' })
const pwSaving = ref(false)
const pwSaved = ref(false)
const pwError = ref('')
const pwValid = computed(() => pw.current && pw.next.length >= 8 && pw.next === pw.confirm)

async function changePassword() {
  pwError.value = ''
  if (!pwValid.value) {
    pwError.value = t('settings.security.invalid')
    return
  }
  pwSaving.value = true
  try {
    await users.changePassword({ current_password: pw.current, new_password: pw.next })
    pw.current = pw.next = pw.confirm = ''
    flash(pwSaved)
  } catch (e) {
    pwError.value = e.message
  } finally {
    pwSaving.value = false
  }
}

// ── appearance (persists immediately via the theme store) ──────────────────────
// Every option table below is a computed, never a module-level const: built
// once at import time it would keep the labels of whatever language rendered
// first, and switching the UI language would leave the selects behind (#2799).
const themeModeOptions = computed(() => [
  { label: t('settings.appearance.mode.system'), value: 'system' },
  { label: t('settings.appearance.mode.light'), value: 'light' },
  { label: t('settings.appearance.mode.dark'), value: 'dark' },
])

// ── localization ───────────────────────────────────────────────────────────────
// Language names stay endonyms — «Русский» reads the same on an English UI.
const langOptions = computed(() => [
  { label: t('common.language.ru'), value: 'ru' },
  { label: t('settings.localization.langSoon', { name: t('common.language.en') }), value: 'en' },
])
const timeFmtOptions = computed(() => [
  { label: t('settings.localization.time.h24'), value: '24h' },
  { label: t('settings.localization.time.h12'), value: '12h' },
])
// Date format is a named preset since #2798, so the options are samples rendered
// by Intl in the current language — picking "medium" shows «31 дек. 2026 г.» on
// ru and "31 Dec 2026" on en, instead of a fixed pattern that fights the locale.
const { datePresetOptions } = useFormat()
const weekStartOptions = computed(() => [
  { label: t('settings.localization.weekStart.monday'), value: 1 },
  { label: t('settings.localization.weekStart.sunday'), value: 0 },
])

// flash a transient "saved" tick on a ref for ~2s.
function flash(r) {
  r.value = true
  setTimeout(() => (r.value = false), 2000)
}

// ── email verification ──
const verifySent = ref(false)
async function resendVerify() {
  try {
    await accountFlows.resendVerification()
  } catch {
    /* no-op mailer / network — still mark as sent so the UI settles */
  }
  verifySent.value = true
}
</script>

<template>
  <div class="settings page">
    <h1 class="title">{{ t('settings.title') }}</h1>

    <!-- Profile -->
    <section class="card">
      <h2>{{ t('settings.profile.title') }}</h2>
      <div class="avatar-row">
        <img v-if="avatarUrl" :src="avatarUrl" class="ava ava-img" alt="" />
        <n-avatar v-else round :size="72" class="ava">{{ initials }}</n-avatar>
        <div class="avatar-actions">
          <!-- Matches what the server accepts for avatars (no SVG). -->
          <input
            ref="fileInput"
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            hidden
            @change="onAvatarPicked"
          />
          <n-button size="small" @click="fileInput?.click()">
            <template #icon><n-icon :component="CloudUploadOutline" /></template>
            {{ t('settings.profile.avatar.upload') }}
          </n-button>
          <n-button v-if="auth.user?.avatar_url" size="small" quaternary @click="removeAvatar">
            <template #icon><n-icon :component="TrashOutline" /></template>
            {{ t('settings.profile.avatar.remove') }}
          </n-button>
          <div class="hint">{{ t('settings.profile.avatar.hint') }}</div>
          <div v-if="avatarError" class="err">{{ avatarError }}</div>
        </div>
      </div>

      <label class="field">
        <span>{{ t('settings.profile.email') }}</span>
        <n-input :value="auth.user?.email" disabled />
        <div class="verify-row">
          <span v-if="auth.user?.email_verified" class="saved">
            <n-icon :component="CheckmarkCircle" /> {{ t('settings.profile.verify.done') }}
          </span>
          <template v-else>
            <span class="hint">{{ t('settings.profile.verify.pending') }}</span>
            <n-button v-if="!verifySent" size="tiny" @click="resendVerify">{{
              t('settings.profile.verify.send')
            }}</n-button>
            <span v-else class="hint">{{ t('settings.profile.verify.sent') }}</span>
          </template>
        </div>
      </label>
      <label class="field">
        <span>{{ t('settings.profile.displayName') }}</span>
        <n-input
          v-model:value="profile.name"
          :placeholder="t('settings.profile.displayNamePlaceholder')"
          :input-props="{ autocomplete: 'nickname' }"
        />
      </label>
      <div class="grid3">
        <label class="field"
          ><span>{{ t('settings.profile.lastName') }}</span
          ><n-input
            v-model:value="profile.last_name"
            :input-props="{ autocomplete: 'family-name' }"
        /></label>
        <label class="field"
          ><span>{{ t('settings.profile.firstName') }}</span
          ><n-input
            v-model:value="profile.first_name"
            :input-props="{ autocomplete: 'given-name' }"
        /></label>
        <label class="field"
          ><span>{{ t('settings.profile.middleName') }}</span
          ><n-input
            v-model:value="profile.middle_name"
            :input-props="{ autocomplete: 'additional-name' }"
        /></label>
      </div>
      <div class="grid2">
        <label class="field"
          ><span>{{ t('settings.profile.company') }}</span
          ><n-input v-model:value="profile.company" :input-props="{ autocomplete: 'organization' }"
        /></label>
        <label class="field"
          ><span>{{ t('settings.profile.jobTitle') }}</span
          ><n-input
            v-model:value="profile.job_title"
            :input-props="{ autocomplete: 'organization-title' }"
        /></label>
      </div>
      <label class="field">
        <span>{{ t('settings.profile.bio') }}</span>
        <n-input
          v-model:value="profile.bio"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 5 }"
        />
      </label>
      <div class="row-end">
        <span v-if="profileError" class="err">{{ profileError }}</span>
        <transition name="fade">
          <span v-if="profileSaved" class="saved"
            ><n-icon :component="CheckmarkCircle" /> {{ t('common.state.saved') }}</span
          >
        </transition>
        <n-button type="primary" :loading="profileSaving" @click="saveProfile">{{
          t('settings.profile.save')
        }}</n-button>
      </div>
    </section>

    <!-- Security -->
    <section class="card">
      <h2>{{ t('settings.security.title') }}</h2>
      <div class="grid3">
        <label class="field"
          ><span>{{ t('settings.security.current') }}</span
          ><n-input v-model:value="pw.current" type="password" show-password-on="click"
        /></label>
        <label class="field"
          ><span>{{ t('settings.security.next') }}</span
          ><n-input v-model:value="pw.next" type="password" show-password-on="click"
        /></label>
        <label class="field"
          ><span>{{ t('settings.security.confirm') }}</span
          ><n-input v-model:value="pw.confirm" type="password" show-password-on="click"
        /></label>
      </div>
      <div class="row-end">
        <span v-if="pwError" class="err">{{ pwError }}</span>
        <transition name="fade">
          <span v-if="pwSaved" class="saved"
            ><n-icon :component="CheckmarkCircle" /> {{ t('settings.security.changed') }}</span
          >
        </transition>
        <n-button type="primary" :disabled="!pwValid" :loading="pwSaving" @click="changePassword">{{
          t('settings.security.change')
        }}</n-button>
      </div>
    </section>

    <!-- Notifications (channels + routing) -->
    <NotificationSettings />

    <!-- Appearance -->
    <section class="card">
      <h2>{{ t('settings.appearance.title') }}</h2>
      <label class="field">
        <span>{{ t('settings.appearance.theme') }}</span>
        <n-select
          :value="theme.themeMode"
          :options="themeModeOptions"
          @update:value="theme.setThemeMode"
        />
      </label>
      <div class="field">
        <span>{{ t('settings.appearance.accent') }}</span>
        <div class="swatches">
          <button
            v-for="ct in COLOR_THEMES"
            :key="ct.key"
            class="swatch"
            :class="{ active: theme.activeTheme.key === ct.key }"
            :style="{ background: ct.primary }"
            :title="ct.name || ct.key"
            @click="theme.selectColor(ct)"
          />
        </div>
      </div>
      <label class="field">
        <span>{{ t('settings.appearance.boardBackground') }}</span>
        <n-input
          :value="theme.boardBackground"
          :placeholder="t('settings.appearance.boardBackgroundPlaceholder')"
          clearable
          @update:value="theme.setBoardBackground"
        />
      </label>
    </section>

    <!-- Localization -->
    <section class="card">
      <h2>{{ t('settings.localization.title') }}</h2>
      <div class="grid2">
        <label class="field">
          <span>{{ t('settings.localization.language') }}</span>
          <n-select
            :value="theme.language"
            :options="langOptions"
            @update:value="(v) => theme.setLocale({ language: v })"
          />
        </label>
        <label class="field">
          <span>{{ t('settings.localization.weekStartLabel') }}</span>
          <n-select
            :value="theme.weekStart"
            :options="weekStartOptions"
            @update:value="(v) => theme.setLocale({ week_start: v })"
          />
        </label>
        <label class="field">
          <span>{{ t('settings.localization.timeFormat') }}</span>
          <n-select
            :value="theme.timeFormat"
            :options="timeFmtOptions"
            @update:value="(v) => theme.setLocale({ time_format: v })"
          />
        </label>
        <label class="field">
          <span>{{ t('settings.localization.dateFormat') }}</span>
          <n-select
            :value="theme.dateFormat"
            :options="datePresetOptions"
            @update:value="(v) => theme.setLocale({ date_format: v })"
          />
        </label>
        <label class="field">
          <span>{{ t('settings.localization.timezone') }}</span>
          <n-select
            :value="theme.timezone || null"
            :options="tzOptions"
            filterable
            clearable
            :placeholder="t('settings.localization.timezonePlaceholder')"
            @update:value="(v) => theme.setLocale({ timezone: v || '' })"
          />
        </label>
        <label class="field">
          <span>{{ t('settings.localization.country') }}</span>
          <n-select
            :value="theme.country || null"
            :options="countryOpts"
            filterable
            clearable
            :placeholder="t('settings.localization.countryPlaceholder')"
            @update:value="(v) => theme.setLocale({ country: v || '' })"
          />
        </label>
      </div>
      <p class="hint">{{ t('settings.localization.hint') }}</p>
    </section>

    <!-- Desktop app: server address + self-update (hidden in the browser). -->
    <section v-if="isDesktop" class="card">
      <h2>{{ t('settings.app.title') }}</h2>
      <label class="field">
        <span>{{ t('settings.app.serverAddr') }}</span>
        <n-input
          v-model:value="serverAddr"
          :placeholder="t('settings.app.serverAddrPlaceholder')"
        />
      </label>
      <div class="row-end">
        <n-button type="primary" @click="saveServerAddr">{{
          t('settings.app.saveAndRestart')
        }}</n-button>
      </div>
      <div class="field" style="margin-top: 14px">
        <span>{{ t('settings.app.autostart') }}</span>
        <label class="autostart-row">
          <n-switch :value="autostart" :loading="autostartBusy" @update:value="toggleAutostart" />
          <span>{{ t('settings.app.autostartHint') }}</span>
        </label>
      </div>
      <div class="field" style="margin-top: 14px">
        <span>{{ t('settings.app.updates') }}</span>
        <div class="row-end" style="justify-content: flex-start; gap: 10px">
          <n-button :loading="updBusy" @click="updCheck(false)">{{
            t('settings.app.check')
          }}</n-button>
          <n-button v-if="updStatus === 'available'" type="primary" @click="updInstall">
            {{ t('settings.app.install', { version: updVersion }) }}
          </n-button>
        </div>
      </div>
      <p class="hint">
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
  </div>
</template>

<style scoped>
.settings {
  max-width: 720px;
  margin: 0 auto;
  padding: 20px 16px 60px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.title {
  font-size: 22px;
  font-weight: 700;
  color: var(--t-text1);
  margin: 4px 0;
}
.card {
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 12px;
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.card h2 {
  font-size: 15px;
  font-weight: 600;
  color: var(--t-text1);
  margin: 0;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.field > span {
  font-size: 12px;
  color: var(--t-text3);
}
.verify-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
}
.grid2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.grid3 {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 12px;
}
.avatar-row {
  display: flex;
  align-items: center;
  gap: 16px;
}
.autostart-row {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 14px;
  color: var(--t-text2);
  cursor: pointer;
}
.ava {
  flex: none;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-weight: 600;
}
.ava-img {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  object-fit: cover;
}
.avatar-actions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: flex-start;
}
.hint {
  font-size: 12px;
  color: var(--t-text3);
}
.err {
  font-size: 12px;
  color: #e0533d;
}
.saved {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  color: #18a058;
}
.row-end {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
}
.swatches {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.swatch {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
  padding: 0;
}
.swatch.active {
  border-color: var(--t-text1);
  box-shadow: 0 0 0 2px var(--t-surface);
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
@media (max-width: 560px) {
  .grid2,
  .grid3 {
    grid-template-columns: 1fr;
  }
}
</style>
