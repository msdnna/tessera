<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { NInput, NButton, NSelect, NAvatar, NIcon, NSwitch } from 'naive-ui'
import { CloudUploadOutline, TrashOutline, CheckmarkCircle } from '@vicons/ionicons5'
import { users, accountFlows } from '@/api'
import NotificationSettings from '@/components/NotificationSettings.vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore, COLOR_THEMES } from '@/stores/theme'
import { timezoneOptions, countryOptions } from '@/utils/localeOptions'
import { isTauri, serverBase, setServerBase } from '@/utils/serverBase'
import { useApiImage } from '@/composables/useApiImage'
import { useDesktopUpdate } from '@/composables/useDesktopUpdate'

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

// Built once from native Intl data (filterable selects show all on focus).
const tzOptions = timezoneOptions()
const countryOpts = countryOptions(theme.language || 'ru')

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
// Direct URL on web; axios-fetched blob: URL on desktop. The ?t buster changes the
// URL on re-upload so both the browser cache and the blob cache miss and refetch.
const avatarUrl = useApiImage(() =>
  auth.user?.avatar_url ? `${auth.user.avatar_url}?t=${avatarBust.value}` : '',
)
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
    pwError.value = 'Проверьте поля: новый пароль ≥ 8 символов и совпадает с подтверждением'
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
const themeModeOptions = [
  { label: 'Системная', value: 'system' },
  { label: 'Светлая', value: 'light' },
  { label: 'Тёмная', value: 'dark' },
]

// ── localization ───────────────────────────────────────────────────────────────
const langOptions = [
  { label: 'Русский', value: 'ru' },
  { label: 'English (скоро)', value: 'en' },
]
const timeFmtOptions = [
  { label: '24-часовой', value: '24h' },
  { label: '12-часовой (AM/PM)', value: '12h' },
]
const dateFmtOptions = [
  { label: '31.12.2026', value: 'dd.MM.yyyy' },
  { label: '2026-12-31', value: 'yyyy-MM-dd' },
  { label: '12/31/2026', value: 'MM/dd/yyyy' },
  { label: '31/12/2026', value: 'dd/MM/yyyy' },
]
const weekStartOptions = [
  { label: 'Понедельник', value: 1 },
  { label: 'Воскресенье', value: 0 },
]

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
    <h1 class="title">Настройки</h1>

    <!-- Profile -->
    <section class="card">
      <h2>Профиль</h2>
      <div class="avatar-row">
        <img v-if="avatarUrl" :src="avatarUrl" class="ava ava-img" alt="" />
        <n-avatar v-else round :size="72" class="ava">{{ initials }}</n-avatar>
        <div class="avatar-actions">
          <input ref="fileInput" type="file" accept="image/*" hidden @change="onAvatarPicked" />
          <n-button size="small" @click="fileInput?.click()">
            <template #icon><n-icon :component="CloudUploadOutline" /></template>
            Загрузить
          </n-button>
          <n-button v-if="auth.user?.avatar_url" size="small" quaternary @click="removeAvatar">
            <template #icon><n-icon :component="TrashOutline" /></template>
            Убрать
          </n-button>
          <div class="hint">PNG/JPEG/GIF/WebP, до 2 МБ</div>
          <div v-if="avatarError" class="err">{{ avatarError }}</div>
        </div>
      </div>

      <label class="field">
        <span>Email (логин)</span>
        <n-input :value="auth.user?.email" disabled />
        <div class="verify-row">
          <span v-if="auth.user?.email_verified" class="saved">
            <n-icon :component="CheckmarkCircle" /> Почта подтверждена
          </span>
          <template v-else>
            <span class="hint">Почта не подтверждена</span>
            <n-button v-if="!verifySent" size="tiny" @click="resendVerify"
              >Отправить письмо</n-button
            >
            <span v-else class="hint">Письмо отправлено</span>
          </template>
        </div>
      </label>
      <label class="field">
        <span>Отображаемое имя</span>
        <n-input
          v-model:value="profile.name"
          placeholder="Как вас показывать"
          :input-props="{ autocomplete: 'nickname' }"
        />
      </label>
      <div class="grid3">
        <label class="field"
          ><span>Фамилия</span
          ><n-input v-model:value="profile.last_name" :input-props="{ autocomplete: 'family-name' }"
        /></label>
        <label class="field"
          ><span>Имя</span
          ><n-input v-model:value="profile.first_name" :input-props="{ autocomplete: 'given-name' }"
        /></label>
        <label class="field"
          ><span>Отчество</span
          ><n-input
            v-model:value="profile.middle_name"
            :input-props="{ autocomplete: 'additional-name' }"
        /></label>
      </div>
      <div class="grid2">
        <label class="field"
          ><span>Место работы</span
          ><n-input v-model:value="profile.company" :input-props="{ autocomplete: 'organization' }"
        /></label>
        <label class="field"
          ><span>Должность</span
          ><n-input
            v-model:value="profile.job_title"
            :input-props="{ autocomplete: 'organization-title' }"
        /></label>
      </div>
      <label class="field">
        <span>О себе</span>
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
            ><n-icon :component="CheckmarkCircle" /> Сохранено</span
          >
        </transition>
        <n-button type="primary" :loading="profileSaving" @click="saveProfile"
          >Сохранить профиль</n-button
        >
      </div>
    </section>

    <!-- Security -->
    <section class="card">
      <h2>Безопасность</h2>
      <div class="grid3">
        <label class="field"
          ><span>Текущий пароль</span
          ><n-input v-model:value="pw.current" type="password" show-password-on="click"
        /></label>
        <label class="field"
          ><span>Новый пароль</span
          ><n-input v-model:value="pw.next" type="password" show-password-on="click"
        /></label>
        <label class="field"
          ><span>Повторите новый</span
          ><n-input v-model:value="pw.confirm" type="password" show-password-on="click"
        /></label>
      </div>
      <div class="row-end">
        <span v-if="pwError" class="err">{{ pwError }}</span>
        <transition name="fade">
          <span v-if="pwSaved" class="saved"
            ><n-icon :component="CheckmarkCircle" /> Пароль изменён</span
          >
        </transition>
        <n-button type="primary" :disabled="!pwValid" :loading="pwSaving" @click="changePassword"
          >Сменить пароль</n-button
        >
      </div>
    </section>

    <!-- Notifications (channels + routing) -->
    <NotificationSettings />

    <!-- Appearance -->
    <section class="card">
      <h2>Оформление</h2>
      <label class="field">
        <span>Тема</span>
        <n-select
          :value="theme.themeMode"
          :options="themeModeOptions"
          @update:value="theme.setThemeMode"
        />
      </label>
      <div class="field">
        <span>Акцент</span>
        <div class="swatches">
          <button
            v-for="t in COLOR_THEMES"
            :key="t.key"
            class="swatch"
            :class="{ active: theme.activeTheme.key === t.key }"
            :style="{ background: t.primary }"
            :title="t.name || t.key"
            @click="theme.selectColor(t)"
          />
        </div>
      </div>
      <label class="field">
        <span>Фон досок (CSS-цвет или URL картинки)</span>
        <n-input
          :value="theme.boardBackground"
          placeholder="например #0e0e12 или https://…/bg.jpg"
          clearable
          @update:value="theme.setBoardBackground"
        />
      </label>
    </section>

    <!-- Localization -->
    <section class="card">
      <h2>Локализация и форматы</h2>
      <div class="grid2">
        <label class="field">
          <span>Язык</span>
          <n-select
            :value="theme.language"
            :options="langOptions"
            @update:value="(v) => theme.setLocale({ language: v })"
          />
        </label>
        <label class="field">
          <span>Начало недели</span>
          <n-select
            :value="theme.weekStart"
            :options="weekStartOptions"
            @update:value="(v) => theme.setLocale({ week_start: v })"
          />
        </label>
        <label class="field">
          <span>Формат времени</span>
          <n-select
            :value="theme.timeFormat"
            :options="timeFmtOptions"
            @update:value="(v) => theme.setLocale({ time_format: v })"
          />
        </label>
        <label class="field">
          <span>Формат даты</span>
          <n-select
            :value="theme.dateFormat"
            :options="dateFmtOptions"
            @update:value="(v) => theme.setLocale({ date_format: v })"
          />
        </label>
        <label class="field">
          <span>Часовой пояс</span>
          <n-select
            :value="theme.timezone || null"
            :options="tzOptions"
            filterable
            clearable
            placeholder="Europe/Moscow"
            @update:value="(v) => theme.setLocale({ timezone: v || '' })"
          />
        </label>
        <label class="field">
          <span>Страна</span>
          <n-select
            :value="theme.country || null"
            :options="countryOpts"
            filterable
            clearable
            placeholder="Выберите страну"
            @update:value="(v) => theme.setLocale({ country: v || '' })"
          />
        </label>
      </div>
      <p class="hint">
        Форматы применяются к календарям и датам. Переключение языка интерфейса появится позже.
      </p>
    </section>

    <!-- Desktop app: server address + self-update (hidden in the browser). -->
    <section v-if="isDesktop" class="card">
      <h2>Приложение</h2>
      <label class="field">
        <span>Адрес сервера</span>
        <n-input v-model:value="serverAddr" placeholder="https://tessera.msdnna.website" />
      </label>
      <div class="row-end">
        <n-button type="primary" @click="saveServerAddr">Сохранить и перезапустить</n-button>
      </div>
      <div class="field" style="margin-top: 14px">
        <span>Автозапуск</span>
        <label class="autostart-row">
          <n-switch
            :value="autostart"
            :loading="autostartBusy"
            @update:value="toggleAutostart"
          />
          <span>Запускать Tessera при входе в систему (свёрнутой в трей)</span>
        </label>
      </div>
      <div class="field" style="margin-top: 14px">
        <span>Обновления</span>
        <div class="row-end" style="justify-content: flex-start; gap: 10px">
          <n-button :loading="updBusy" @click="updCheck(false)">Проверить обновления</n-button>
          <n-button v-if="updStatus === 'available'" type="primary" @click="updInstall">
            Установить {{ updVersion }} и перезапустить
          </n-button>
        </div>
      </div>
      <p class="hint">
        <template v-if="updStatus === 'none'">Установлена последняя версия.</template>
        <template v-else-if="updStatus === 'available'"
          >Доступна версия {{ updVersion }}.</template
        >
        <template v-else-if="updStatus === 'downloading'">Загрузка обновления…</template>
        <template v-else-if="updStatus === 'error'">Ошибка обновления: {{ updError }}</template>
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
