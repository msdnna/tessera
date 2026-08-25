<script setup>
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NTag, NPopconfirm, NIcon, NSpin, NInput, NSwitch, useMessage } from 'naive-ui'
import { ShieldCheckmarkOutline, KeyOutline, SearchOutline, LogoGitlab } from '@vicons/ionicons5'
import { admin } from '@/api'
import { copyText } from '@/utils/clipboard'
import { useAuthStore } from '@/stores/auth'
import UserAvatar from '@/components/UserAvatar.vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import SecretInput from '@/components/SecretInput.vue'

const { t } = useI18n()
const auth = useAuthStore()
const message = useMessage()

const loading = ref(false)
const users = ref([])
const query = ref('')
const busy = ref('') // id currently mutating, to disable its row buttons

const meId = computed(() => auth.user?.id)
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return users.value
  return users.value.filter(
    (u) => u.name.toLowerCase().includes(q) || u.email.toLowerCase().includes(q),
  )
})

async function load() {
  loading.value = true
  try {
    const res = await admin.listUsers()
    users.value = res.data || []
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

async function toggleActive(u) {
  busy.value = u.id
  try {
    await admin.setActive(u.id, !u.active)
    u.active = !u.active
    message.success(
      u.active ? t('settings.admin.msg.activated') : t('settings.admin.msg.deactivated'),
    )
  } catch (e) {
    message.error(e.message)
  } finally {
    busy.value = ''
  }
}

async function toggleAdmin(u) {
  busy.value = u.id
  try {
    await admin.setAdmin(u.id, !u.is_admin)
    u.is_admin = !u.is_admin
    message.success(
      u.is_admin ? t('settings.admin.msg.adminGranted') : t('settings.admin.msg.adminRevoked'),
    )
  } catch (e) {
    message.error(e.message)
  } finally {
    busy.value = ''
  }
}

async function copyResetLink(u) {
  busy.value = u.id
  try {
    const res = await admin.resetLink(u.id)
    const link = res.data?.link || ''
    // The backend returns a path-only link when PUBLIC_URL is unset — qualify it
    // against the current origin so the copied value is always clickable.
    const full = link.startsWith('http') ? link : `${window.location.origin}${link}`
    if (await copyText(full)) message.success(t('settings.admin.msg.resetLinkCopied'))
    else message.info(full)
  } catch (e) {
    message.error(e.message)
  } finally {
    busy.value = ''
  }
}

// ── GitLab OAuth app config ──
// Verbatim technical tokens: GitLab scope names and a JSON sample. They read
// the same in every language, but they still have to come through a binding —
// as bare template text the no-bare-strings rule would (rightly) flag them.
const SCOPE_READ_API = 'read_api'
const SCOPE_API = 'api'
const ORG_MAP_SAMPLE =
  '{ "group/path": { "workspace_id": "uuid", "admins": ["login"], "users": true } }'

const oauth = ref({
  gl_base_url: '',
  client_id: '',
  client_secret: '',
  service_token: '',
  enabled: false,
  org_map: '{}',
  has_secret: false,
  has_service_token: false,
  clear_secret: false,
  clear_service_token: false,
  sudo_writeback: false,
})
const oauthSaving = ref(false)
const callbackUrl = computed(() => `${window.location.origin}/api/auth/gitlab/callback`)

async function loadOAuth() {
  try {
    const { data } = await admin.getOAuth()
    oauth.value = {
      gl_base_url: data.gl_base_url || '',
      client_id: data.client_id || '',
      client_secret: '',
      service_token: '',
      enabled: data.enabled === true,
      org_map: JSON.stringify(data.org_map ?? {}, null, 2),
      has_secret: data.has_secret === true,
      has_service_token: data.has_service_token === true,
      clear_secret: false,
      clear_service_token: false,
      sudo_writeback: data.sudo_writeback === true,
    }
  } catch {
    /* first-time config: keep defaults */
  }
}

async function saveOAuth() {
  let orgMap
  try {
    orgMap = JSON.parse(oauth.value.org_map || '{}')
  } catch {
    message.error(t('settings.admin.msg.orgMapInvalid'))
    return
  }
  oauthSaving.value = true
  try {
    const { data } = await admin.setOAuth({
      gl_base_url: oauth.value.gl_base_url.trim(),
      client_id: oauth.value.client_id.trim(),
      client_secret: oauth.value.client_secret, // empty keeps the stored one
      service_token: oauth.value.service_token, // empty keeps the stored one
      clear_client_secret: oauth.value.clear_secret, // explicit erase
      clear_service_token: oauth.value.clear_service_token,
      enabled: oauth.value.enabled,
      org_map: orgMap,
      sudo_writeback: oauth.value.sudo_writeback,
    })
    oauth.value.client_secret = ''
    oauth.value.service_token = ''
    oauth.value.clear_secret = false
    oauth.value.clear_service_token = false
    oauth.value.has_secret = data.has_secret === true
    oauth.value.has_service_token = data.has_service_token === true
    oauth.value.sudo_writeback = data.sudo_writeback === true
    message.success(t('settings.admin.msg.oauthSaved'))
  } catch (e) {
    message.error(e.response?.data?.error || e.message)
  } finally {
    oauthSaving.value = false
  }
}

onMounted(() => {
  load()
  loadOAuth()
})
</script>

<template>
  <n-spin :show="loading" :rotate="false">
    <template #icon><TesseraSpinner /></template>
    <div class="admin">
      <div class="head">
        <h2 class="title">
          <n-icon :component="ShieldCheckmarkOutline" class="title-ic" />
          {{ t('settings.admin.title') }}
        </h2>
        <span class="sub">{{ t('settings.admin.usersCount', { n: users.length }) }}</span>
      </div>

      <!-- GitLab OAuth ("Войти через GitLab") -->
      <div class="oauth-card">
        <h3 class="oauth-h">
          <n-icon :component="LogoGitlab" class="oauth-ic" /> {{ t('settings.admin.oauth.title') }}
        </h3>
        <p class="oauth-hint">
          <b>{{ t('settings.admin.oauth.hint.loginLabel') }}</b>
          <i18n-t keypath="settings.admin.oauth.hint.login" tag="span" scope="global">
            <template #scope
              ><code>{{ SCOPE_READ_API }}</code></template
            >
            <template #redirect
              ><code>{{ callbackUrl }}</code></template
            >
            <template #toggle
              ><b>{{ t('settings.admin.oauth.hint.toggle') }}</b></template
            >
          </i18n-t>
          <br />
          <b>{{ t('settings.admin.oauth.hint.syncLabel') }}</b>
          <i18n-t keypath="settings.admin.oauth.hint.sync" tag="span" scope="global">
            <template #token
              ><b>{{ t('settings.admin.oauth.hint.serviceToken') }}</b></template
            >
            <template #api
              ><code>{{ SCOPE_API }}</code></template
            >
          </i18n-t>
        </p>
        <div class="oauth-grid">
          <label>{{ t('settings.admin.oauth.field.baseUrl') }}</label>
          <n-input
            v-model:value="oauth.gl_base_url"
            size="small"
            :placeholder="t('settings.admin.oauth.placeholder.baseUrl')"
            :input-props="{ autocomplete: 'off', name: 'oauth-base-url' }"
          />
          <label>{{ t('settings.admin.oauth.field.appId') }}</label>
          <n-input
            v-model:value="oauth.client_id"
            size="small"
            :placeholder="t('settings.admin.oauth.placeholder.clientId')"
            :input-props="{ autocomplete: 'off', name: 'oauth-client-id' }"
          />
          <label>{{ t('settings.admin.oauth.field.secret') }}</label>
          <SecretInput
            v-model:value="oauth.client_secret"
            v-model:cleared="oauth.clear_secret"
            :stored="oauth.has_secret"
            size="small"
            :placeholder="t('settings.admin.oauth.placeholder.clientSecret')"
            :stored-placeholder="t('settings.admin.oauth.storedPlaceholder')"
            :input-props="{ autocomplete: 'new-password', name: 'oauth-secret' }"
          />
          <label>
            {{ t('settings.admin.oauth.field.serviceToken') }}
            <span class="oauth-sub">{{ t('settings.admin.oauth.field.serviceTokenSub') }}</span>
          </label>
          <div class="oauth-token-cell">
            <SecretInput
              v-model:value="oauth.service_token"
              v-model:cleared="oauth.clear_service_token"
              :stored="oauth.has_service_token"
              size="small"
              :placeholder="t('settings.admin.oauth.placeholder.serviceToken')"
              :stored-placeholder="t('settings.admin.oauth.storedPlaceholder')"
              :input-props="{ autocomplete: 'new-password', name: 'oauth-service-token' }"
            />
            <span v-if="oauth.clear_service_token" class="oauth-warn">
              {{ t('settings.admin.oauth.clearWarn') }}
            </span>
          </div>
          <label>
            {{ t('settings.admin.oauth.field.sudo') }}
            <span class="oauth-sub">{{ t('settings.admin.oauth.field.sudoSub') }}</span>
          </label>
          <div><n-switch v-model:value="oauth.sudo_writeback" /></div>
          <label>{{ t('settings.admin.oauth.field.enabled') }}</label>
          <div><n-switch v-model:value="oauth.enabled" /></div>
          <label>
            {{ t('settings.admin.oauth.field.orgMap') }}
            <span class="oauth-sub">{{ t('settings.admin.oauth.field.orgMapSub') }}</span>
          </label>
          <n-input
            v-model:value="oauth.org_map"
            type="textarea"
            size="small"
            :autosize="{ minRows: 4, maxRows: 14 }"
            :placeholder="ORG_MAP_SAMPLE"
            class="oauth-mono"
          />
        </div>
        <div class="oauth-foot">
          <n-button type="primary" size="small" :loading="oauthSaving" @click="saveOAuth">
            {{ t('common.action.save') }}
          </n-button>
        </div>
      </div>

      <n-input
        v-model:value="query"
        :placeholder="t('settings.admin.search')"
        clearable
        class="search"
      >
        <template #prefix><n-icon :component="SearchOutline" /></template>
      </n-input>

      <div class="list">
        <div v-for="u in filtered" :key="u.id" class="urow" :class="{ off: !u.active }">
          <UserAvatar class="ava" :user-id="u.id" :name="u.name" :src="u.avatar_url" />
          <div class="who">
            <div class="line1">
              <span class="uname">{{ u.name }}</span>
              <n-tag v-if="u.is_admin" size="small" type="warning" round>{{
                t('settings.admin.badge.admin')
              }}</n-tag>
              <n-tag v-if="u.id === meId" size="small" round>{{
                t('settings.admin.badge.you')
              }}</n-tag>
              <n-tag v-if="!u.active" size="small" type="error" round>{{
                t('settings.admin.badge.disabled')
              }}</n-tag>
              <n-tag v-if="!u.email_verified" size="small" round :bordered="false">{{
                t('settings.admin.badge.unverified')
              }}</n-tag>
            </div>
            <div class="umail">{{ u.email }}</div>
          </div>

          <div class="actions">
            <n-button
              size="small"
              quaternary
              :disabled="busy === u.id"
              :title="t('settings.admin.action.resetLink')"
              @click="copyResetLink(u)"
            >
              <template #icon><n-icon :component="KeyOutline" /></template>
            </n-button>

            <!-- Admin toggle: granting is one click; revoking confirms. Never self. -->
            <template v-if="u.id !== meId">
              <n-button
                v-if="!u.is_admin"
                size="small"
                :disabled="busy === u.id"
                @click="toggleAdmin(u)"
              >
                {{ t('settings.admin.action.makeAdmin') }}
              </n-button>
              <n-popconfirm v-else @positive-click="toggleAdmin(u)">
                <template #trigger>
                  <n-button size="small" :disabled="busy === u.id">{{
                    t('settings.admin.action.revokeAdmin')
                  }}</n-button>
                </template>
                {{ t('settings.admin.confirm.revokeAdmin', { name: u.name }) }}
              </n-popconfirm>

              <!-- Active toggle: deactivating confirms (blocks login). -->
              <n-button
                v-if="!u.active"
                size="small"
                type="primary"
                ghost
                :disabled="busy === u.id"
                @click="toggleActive(u)"
              >
                {{ t('settings.admin.action.activate') }}
              </n-button>
              <n-popconfirm v-else @positive-click="toggleActive(u)">
                <template #trigger>
                  <n-button size="small" type="error" ghost :disabled="busy === u.id">
                    {{ t('settings.admin.action.deactivate') }}
                  </n-button>
                </template>
                {{ t('settings.admin.confirm.deactivate', { name: u.name }) }}
              </n-popconfirm>
            </template>
          </div>
        </div>

        <div v-if="!filtered.length && !loading" class="empty">
          {{ t('settings.admin.empty') }}
        </div>
      </div>
    </div>
  </n-spin>
</template>

<style scoped>
.admin {
  max-width: 900px;
  margin: 0 auto;
  padding: 8px 4px 40px;
}
.head {
  margin: 4px 0 16px;
}
.title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 22px;
  font-weight: 700;
  color: var(--t-text1);
  margin: 0;
}
.title-ic {
  color: var(--t-primary);
  font-size: 22px;
}
.sub {
  font-size: 13px;
  color: var(--t-text3);
}
.search {
  margin-bottom: 14px;
  max-width: 360px;
}
.oauth-card {
  border: 1px solid var(--t-border);
  background: var(--t-surface);
  border-radius: 12px;
  padding: 14px 16px;
  margin-bottom: 18px;
}
.oauth-h {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 6px;
  font-size: 16px;
  font-weight: 700;
  color: var(--t-text1);
}
.oauth-ic {
  color: var(--t-primary);
}
.oauth-hint {
  font-size: 12px;
  color: var(--t-text3);
  margin: 0 0 12px;
  line-height: 1.5;
}
.oauth-hint code {
  background: var(--t-fill-2, rgba(140, 140, 160, 0.14));
  padding: 1px 5px;
  border-radius: 5px;
}
.oauth-grid {
  display: grid;
  grid-template-columns: 130px 1fr;
  gap: 10px 12px;
  align-items: center;
}
.oauth-grid > label {
  font-size: 13px;
  color: var(--t-text3);
}
.oauth-sub {
  display: block;
  font-size: 11px;
  opacity: 0.7;
}
.oauth-token-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.oauth-warn {
  font-size: 12px;
  color: var(--t-danger, #e5484d);
}
.oauth-mono :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}
.oauth-foot {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}
.list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.urow {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid var(--t-border);
  background: var(--t-surface);
}
.urow.off {
  opacity: 0.62;
}
.ava {
  width: 36px;
  height: 36px;
  flex: none;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 13px;
  font-weight: 600;
}
.who {
  flex: 1;
  min-width: 0;
}
.line1 {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.uname {
  font-weight: 600;
  color: var(--t-text1);
}
.umail {
  font-size: 12px;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: none;
}
.empty {
  text-align: center;
  color: var(--t-text3);
  margin-top: 40px;
}
@media (max-width: 768px) {
  .urow {
    flex-wrap: wrap;
  }
  .actions {
    width: 100%;
    justify-content: flex-end;
  }
}
</style>
