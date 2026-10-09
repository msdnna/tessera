<script setup>
// Settings → «Профиль» (v2, #3106). Same fields and the same explicit save as
// before; only the surfaces changed. The avatar is still checked before the
// upload (#2932): the server's own refusal is a bare English sentence.
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NIcon, NInput } from 'naive-ui'
import { CheckmarkCircle, CloudUploadOutline, TrashOutline } from '@vicons/ionicons5'
import { accountFlows, users } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useApiImage } from '@/composables/useApiImage'
import { AVATAR_MAX_BYTES, AVATAR_TYPES, avatarPickError } from '@/utils/avatar'
import { initials as initialsOf } from '@/utils/initials'
import { formatFileSize } from '@/utils/docPdf'
import './settings.css'

const { t } = useI18n()
const auth = useAuthStore()

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
const saving = ref(false)
const saved = ref(false)
const error = ref('')

async function save() {
  saving.value = true
  error.value = ''
  try {
    const res = await users.updateProfile({ ...profile })
    auth.setUser(res.data)
    saved.value = true
    setTimeout(() => (saved.value = false), 2000)
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
  }
}

// ── avatar ──
const fileInput = ref(null)
const avatarBust = ref(0)
// A GitLab-OAuth avatar_url is a proxy URL that already carries a query string;
// a second «?» would corrupt its signature.
const avatarUrl = useApiImage(() => {
  const url = auth.user?.avatar_url
  if (!url) return ''
  return `${url}${url.includes('?') ? '&' : '?'}t=${avatarBust.value}`
})
const initials = computed(() => initialsOf(auth.user?.name || auth.user?.email || '?'))
const avatarError = ref('')

async function onAvatarPicked(e) {
  const file = e.target.files?.[0]
  if (!file) return
  avatarError.value = ''
  const bad = avatarPickError(file)
  if (bad) {
    avatarError.value =
      bad === 'tooLarge'
        ? t('settings.profile.avatar.tooLarge', { limit: formatFileSize(AVATAR_MAX_BYTES) })
        : t('settings.profile.avatar.badType')
    if (fileInput.value) fileInput.value.value = ''
    return
  }
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
  <div class="sp">
    <section class="st-card sp-id" data-testid="settings-avatar">
      <img v-if="avatarUrl" :src="avatarUrl" class="sp-ava" alt="" />
      <span v-else class="sp-ava sp-ava-ini">{{ initials }}</span>
      <div class="sp-who">
        <div class="sp-name">{{ auth.user?.name || auth.user?.email }}</div>
        <div class="sp-mail">
          <span>{{ auth.user?.email }}</span>
          <span v-if="auth.user?.email_verified" class="st-ok">
            <n-icon :component="CheckmarkCircle" /> {{ t('settings.profile.verify.done') }}
          </span>
          <template v-else>
            <span class="sp-pending">{{ t('settings.profile.verify.pending') }}</span>
            <n-button v-if="!verifySent" size="tiny" secondary @click="resendVerify">{{
              t('settings.profile.verify.send')
            }}</n-button>
            <span v-else class="sp-pending">{{ t('settings.profile.verify.sent') }}</span>
          </template>
        </div>
        <div class="sp-ava-actions">
          <!-- Same allowlist the picked file is then checked against (no SVG). -->
          <input
            ref="fileInput"
            type="file"
            :accept="AVATAR_TYPES.join(',')"
            hidden
            @change="onAvatarPicked"
          />
          <n-button size="small" secondary @click="fileInput?.click()">
            <template #icon><n-icon :component="CloudUploadOutline" /></template>
            {{ t('settings.profile.avatar.upload') }}
          </n-button>
          <n-button v-if="auth.user?.avatar_url" size="small" quaternary @click="removeAvatar">
            <template #icon><n-icon :component="TrashOutline" /></template>
            {{ t('settings.profile.avatar.remove') }}
          </n-button>
          <span class="st-hint">{{ t('settings.profile.avatar.hint') }}</span>
        </div>
        <div v-if="avatarError" class="st-err" role="alert">{{ avatarError }}</div>
      </div>
    </section>

    <section class="st-card">
      <label class="st-field">
        <span class="st-label">{{ t('settings.profile.displayName') }}</span>
        <n-input
          v-model:value="profile.name"
          :placeholder="t('settings.profile.displayNamePlaceholder')"
          :input-props="{ autocomplete: 'nickname' }"
        />
      </label>
      <div class="st-grid3">
        <label class="st-field">
          <span class="st-label">{{ t('settings.profile.lastName') }}</span>
          <n-input
            v-model:value="profile.last_name"
            :input-props="{ autocomplete: 'family-name' }"
          />
        </label>
        <label class="st-field">
          <span class="st-label">{{ t('settings.profile.firstName') }}</span>
          <n-input
            v-model:value="profile.first_name"
            :input-props="{ autocomplete: 'given-name' }"
          />
        </label>
        <label class="st-field">
          <span class="st-label">{{ t('settings.profile.middleName') }}</span>
          <n-input
            v-model:value="profile.middle_name"
            :input-props="{ autocomplete: 'additional-name' }"
          />
        </label>
      </div>
      <div class="st-grid2">
        <label class="st-field">
          <span class="st-label">{{ t('settings.profile.company') }}</span>
          <n-input
            v-model:value="profile.company"
            :input-props="{ autocomplete: 'organization' }"
          />
        </label>
        <label class="st-field">
          <span class="st-label">{{ t('settings.profile.jobTitle') }}</span>
          <n-input
            v-model:value="profile.job_title"
            :input-props="{ autocomplete: 'organization-title' }"
          />
        </label>
      </div>
      <label class="st-field">
        <span class="st-label">{{ t('settings.profile.bio') }}</span>
        <n-input
          v-model:value="profile.bio"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 5 }"
        />
      </label>
      <div class="st-actions">
        <span v-if="error" class="st-err">{{ error }}</span>
        <transition name="st-fade">
          <span v-if="saved" class="st-ok">
            <n-icon :component="CheckmarkCircle" /> {{ t('common.state.saved') }}
          </span>
        </transition>
        <n-button
          type="primary"
          :loading="saving"
          data-testid="settings-profile-save"
          @click="save"
        >
          {{ t('settings.profile.save') }}
        </n-button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.sp {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.sp-id {
  flex-direction: row;
  align-items: flex-start;
  gap: 18px;
}
.sp-ava {
  flex: none;
  width: 72px;
  height: 72px;
  border-radius: 50%;
  object-fit: cover;
}
.sp-ava-ini {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 24px;
  font-weight: 650;
}
.sp-who {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}
.sp-name {
  font-size: 17px;
  font-weight: 700;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sp-mail {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 13px;
  color: var(--t-text2);
}
.sp-pending {
  font-size: 12.5px;
  color: var(--t-text3);
}
.sp-ava-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 4px;
}
.st-fade-enter-active,
.st-fade-leave-active {
  transition: opacity 0.3s ease;
}
.st-fade-enter-from,
.st-fade-leave-to {
  opacity: 0;
}
@media (max-width: 560px) {
  .sp-id {
    flex-direction: column;
    align-items: center;
    text-align: center;
  }
  .sp-mail,
  .sp-ava-actions {
    justify-content: center;
  }
}
</style>
