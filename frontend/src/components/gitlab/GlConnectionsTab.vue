<script setup>
// «Подключения» of the GitLab window (#2975, mockup GitlabWin fGlConn): your
// GitLab account, the workspace's bindings as cards (project → board,
// direction, state), the settings of the picked one and its webhook. A new
// binding is filled in place, without a modal of its own.
//
// #3065: several GitLab projects on one board («Ещё проект на ту же доску»), the
// account connected through OAuth («Войти через GitLab»), the token's expiry and
// a binding paused when GitLab refuses it («Переподключить»), and an import
// filter by label.
import { computed, inject, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NInput, NSelect, NSwitch } from 'naive-ui'
import DateField from '@/components/kit/DateField.vue'
import {
  LogoGitlab,
  AddOutline,
  WarningOutline,
  EllipsisHorizontal,
  SyncOutline,
  CloudDownloadOutline,
  TrashOutline,
  LogOutOutline,
  KeyOutline,
  CopyOutline,
  FlashOutline,
  PersonCircleOutline,
} from '@vicons/ionicons5'
import { GITLAB_FORM } from './useGitlabForm'
import { useFormat } from '@/composables/useFormat'
import { cap, CAPS } from '@/utils/keyCaps'
import ActionMenu from '../kit/ActionMenu.vue'
import Kbd from '../Kbd.vue'

const f = inject(GITLAB_FORM)
const { t } = useI18n()
const { formatDate } = useFormat()
const ro = computed(() => !f.isAdmin)
// A GitLab label as the filter's sample — data, not interface text.
const LABEL_SAMPLE = 'backend'

const boardLabel = (id) => f.boardOptions.find((o) => o.value === id)?.label || ''
const scopeLabel = (v) => f.scopeOptions.find((o) => o.value === v)?.label || ''
function cardState(b) {
  if (b.enabled === false) return { key: 'off', tone: '' }
  if (b.paused_at) return { key: 'paused', tone: 'err' }
  if (!f.gl.connected && !f.serviceConfigured) return { key: 'noCreds', tone: 'err' }
  return { key: 'on', tone: 'ok' }
}
const isPush = (b) => b.writeback?.enabled === true

// A token's last day (#3084), 'YYYY-MM-DD' read as a calendar day — not as an
// instant, which west of UTC would print the day before.
function dayOf(s) {
  const [y, m, d] = String(s).split('-').map(Number)
  return new Date(y, m - 1, d)
}
function expiryText(s) {
  if (!s) return ''
  const day = dayOf(s)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  if (day < today) return t('gitlab.window.tokenExpired')
  const opts = { day: 'numeric', month: 'short' }
  if (day.getFullYear() !== today.getFullYear()) opts.year = 'numeric'
  return t('gitlab.window.tokenUntil', { date: formatDate(day, opts) })
}
const expired = (s) => !!s && dayOf(s) < new Date(new Date().setHours(0, 0, 0, 0))
const accountKind = computed(() =>
  f.gl.authKind === 'oauth' ? t('gitlab.window.oauthKind') : t('gitlab.window.pat'),
)
// «Переподключить» a paused binding: under a service token only an admin can
// replace it; otherwise the account goes through the way it was connected.
function reconnect(b) {
  if (b.auth_kind === 'service') return
  if (b.auth_kind === 'oauth' && f.oauthAvailable) {
    f.connectOAuth()
    return
  }
  f.reconnecting = true
  nextTick(() => document.querySelector('[data-testid="gitlab-token"] input')?.focus())
}
const sameBoardTarget = computed(
  () => f.currentBinding?.board_id || f.integrations[0]?.board_id || null,
)
function addSameBoard() {
  if (ro.value || !sameBoardTarget.value) return
  f.addToBoard(sameBoardTarget.value)
  nextTick(() => pathEl.value?.focus?.())
}

// ── menus: a card's «⋯», the account's ──
const menuFor = ref('') // binding id | 'account'
const menuAnchor = ref(null)
const menuShow = computed({
  get: () => !!menuFor.value,
  set: (v) => {
    if (!v) menuFor.value = ''
  },
})
function openMenu(key, el) {
  menuAnchor.value = el
  menuFor.value = key
}
const menuItems = computed(() => {
  if (menuFor.value === 'account')
    return [
      {
        key: 'disconnect',
        label: t('gitlab.modal.account.disconnect'),
        icon: LogOutOutline,
        danger: true,
        confirm: {
          title: t('gitlab.window.disconnectConfirm'),
          sub: t('gitlab.window.disconnectSub'),
        },
      },
    ]
  return [
    { key: 'sync', label: t('gitlab.modal.sync.run'), icon: SyncOutline, keys: 'gitlab.sync' },
    { key: 'full', label: t('gitlab.modal.sync.full'), icon: CloudDownloadOutline },
    { gap: true },
    {
      key: 'delete',
      label: t('gitlab.modal.binding.delete'),
      icon: TrashOutline,
      danger: true,
      disabled: ro.value,
      disabledTitle: t('gitlab.modal.binding.adminOnly'),
      confirm: {
        title: t('gitlab.window.deleteConfirm'),
        sub: t('gitlab.window.deleteSub'),
      },
    },
  ]
})
async function onMenu(key) {
  const target = menuFor.value
  menuFor.value = ''
  if (target === 'account') {
    if (key === 'disconnect') await f.disconnect()
    return
  }
  if (target !== f.currentId) await f.selectBinding(target)
  if (key === 'sync') f.syncNow()
  else if (key === 'full') f.syncNow('full')
  else if (key === 'delete') f.deleteBinding()
}

function pick(b) {
  if (b.id !== f.currentId) f.selectBinding(b.id)
}

// N / «Подключить проект»: a fresh binding filled in place.
const pathEl = ref(null)
function addNew() {
  if (ro.value) return
  f.newBinding()
  nextTick(() => pathEl.value?.focus?.())
}
// Esc: an open menu first.
function escape() {
  if (menuFor.value) {
    menuFor.value = ''
    return true
  }
  return false
}
defineExpose({ addNew, escape })
</script>

<template>
  <div class="glw gc" data-testid="gitlab-connections">
    <div class="glw-title">
      <h2>{{ t('gitlab.window.nav.connections') }}</h2>
      <span class="glw-note">{{ t('gitlab.window.connNote', f.integrations.length) }}</span>
      <span class="glw-grow" />
      <button
        v-if="!ro"
        type="button"
        class="kit-btn primary"
        data-testid="gitlab-connect-project"
        @click="addNew"
      >
        <n-icon :component="AddOutline" :size="15" />
        {{ t('gitlab.window.connectProject') }}
        <Kbd>{{ cap('n') }}</Kbd>
      </button>
    </div>

    <div class="glw-scroll t-hoverscroll">
      <!-- ACCOUNT -->
      <div class="glw-h">{{ t('gitlab.modal.account.title') }}</div>
      <div class="glw-card" data-testid="gitlab-account">
        <span class="glw-tile"><n-icon :component="PersonCircleOutline" :size="20" /></span>
        <div class="glw-main">
          <template v-if="f.gl.connected">
            <div class="glw-line">
              <span class="glw-path accent-grad-text">{{ '@' + f.gl.username }}</span>
              <span class="glw-pill ok">{{ t('gitlab.window.connected') }}</span>
            </div>
            <div class="glw-sub">
              {{ f.gl.baseUrl }} · {{ accountKind }}
              <span
                v-if="f.gl.tokenExpiresAt"
                class="gc-expiry"
                :class="{ err: expired(f.gl.tokenExpiresAt) }"
                data-testid="gitlab-token-expiry"
                >· {{ expiryText(f.gl.tokenExpiresAt) }}</span
              >
            </div>
          </template>
          <template v-else>
            <div class="glw-line">
              <span class="gc-acc-t">{{ t('gitlab.window.noAccount') }}</span>
            </div>
            <div class="glw-sub">{{ t('gitlab.window.noAccountSub') }}</div>
          </template>
        </div>
        <button
          v-if="f.oauthAvailable"
          type="button"
          class="kit-btn"
          :disabled="f.connecting"
          data-testid="gitlab-oauth-connect"
          @click="f.connectOAuth"
        >
          <n-icon :component="LogoGitlab" :size="15" />
          {{ f.gl.connected ? t('gitlab.window.oauthReconnect') : t('gitlab.window.oauth') }}
        </button>
        <button
          v-if="f.gl.connected"
          type="button"
          class="glw-more"
          :title="t('common.action.more')"
          data-testid="gitlab-account-menu"
          @click="openMenu('account', $event.currentTarget)"
        >
          <n-icon :component="EllipsisHorizontal" :size="16" />
        </button>
      </div>
      <p v-if="f.serviceConfigured" class="glw-hint">
        <!-- i18n-t: only the token name is emphasised, and where it sits in the
             sentence is the translator's call. -->
        <i18n-t keypath="gitlab.modal.account.serviceHint" scope="global">
          <template #strong>
            <b>{{ t('gitlab.modal.account.serviceHintStrong') }}</b>
          </template>
        </i18n-t>
      </p>
      <div v-if="!f.gl.connected || f.reconnecting" class="glw-card col gc-pat">
        <div class="glw-row">
          <div class="glw-lbl">
            <span class="glw-lbl-t">{{ t('gitlab.modal.account.baseUrl') }}</span>
          </div>
          <div class="glw-ctl">
            <n-input
              v-model:value="f.baseInput"
              placeholder="https://gitlab.example.com"
              :input-props="{ autocomplete: 'off', name: 'gl-base-url' }"
              data-testid="gitlab-base-url"
            />
          </div>
        </div>
        <div class="glw-row">
          <div class="glw-lbl">
            <span class="glw-lbl-t">{{ t('gitlab.modal.account.token') }}</span>
            <span class="glw-lbl-s">
              <!-- The two scope names are GitLab's own identifiers: they stay
                   verbatim in every locale, only their place moves. -->
              <i18n-t keypath="gitlab.modal.account.scopeHint" scope="global">
                <template #api><b>api</b></template>
                <template #readApi><b>read_api</b></template>
              </i18n-t>
            </span>
          </div>
          <div class="glw-ctl">
            <n-input
              v-model:value="f.tokenInput"
              type="password"
              show-password-on="click"
              placeholder="glpat-…"
              :input-props="{ autocomplete: 'new-password', name: 'gl-token' }"
              data-testid="gitlab-token"
              @keyup.enter="f.connect"
            />
            <button
              type="button"
              class="kit-btn primary"
              :disabled="f.connecting || !f.baseInput.trim() || !f.tokenInput.trim()"
              data-testid="gitlab-account-connect"
              @click="f.connect"
            >
              <n-icon :component="KeyOutline" :size="15" />
              {{ t('gitlab.modal.account.connect') }}
            </button>
          </div>
        </div>
      </div>

      <!-- BINDINGS -->
      <div class="glw-h">
        {{ t('gitlab.window.projects') }}
        <span class="glw-h-n">{{ f.integrations.length }}</span>
      </div>
      <p v-if="!f.gl.connected && !f.serviceConfigured" class="glw-warn">
        <n-icon :component="WarningOutline" :size="16" />
        <span>{{ t('gitlab.modal.binding.noCredsWarning') }}</span>
      </p>
      <p v-if="ro" class="glw-hint gc-ro">{{ t('gitlab.modal.binding.readOnlyHint') }}</p>

      <div
        v-for="b in f.integrations"
        :key="b.id"
        class="glw-card click"
        :class="{ on: b.id === f.currentId, err: cardState(b).tone === 'err' }"
        tabindex="0"
        data-testid="gitlab-binding-card"
        @click="pick(b)"
        @keydown.enter.self="pick(b)"
      >
        <span class="glw-tile" :class="{ off: b.enabled === false }">
          <n-icon :component="LogoGitlab" :size="20" />
        </span>
        <div class="glw-main">
          <div class="glw-line">
            <span class="glw-path">{{ b.project_path || f.bindingLabel(b) }}</span>
            <span class="glw-pill" :class="cardState(b).tone">{{
              t(`gitlab.window.state.${cardState(b).key}`)
            }}</span>
            <span v-if="b.webhook_enabled" class="glw-pill plain">
              <n-icon :component="FlashOutline" :size="11" />
              {{ t('gitlab.window.webhookOn') }}
            </span>
          </div>
          <div class="glw-sub">
            → {{ boardLabel(b.board_id) || t('gitlab.window.noBoard') }}
            <template v-if="b.name"> · {{ b.name }}</template>
            · {{ scopeLabel(b.scope || 'all') }}
            <template v-if="b.import_labels?.length">
              · {{ t('gitlab.window.importOnly', { labels: b.import_labels.join(', ') }) }}
            </template>
            <template v-if="b.token_expires_at && !b.paused_at">
              ·
              <span :class="{ 'gc-err': expired(b.token_expires_at) }">{{
                expiryText(b.token_expires_at)
              }}</span>
            </template>
            ·
            {{
              b.last_synced_at
                ? t('gitlab.window.lastPull', { ago: f.ago(b.last_synced_at) })
                : t('gitlab.modal.sync.never')
            }}
          </div>
        </div>
        <button
          v-if="b.paused_at && b.auth_kind !== 'service'"
          type="button"
          class="kit-btn small primary"
          data-testid="gitlab-reconnect"
          @click.stop="reconnect(b)"
        >
          {{ t('gitlab.window.reconnect') }}
        </button>
        <span
          v-else-if="b.paused_at"
          class="glw-hint gc-svc"
          :title="t('gitlab.window.reconnectService')"
          >{{ t('gitlab.window.reconnectServiceShort') }}</span
        >
        <span class="glw-seg gc-dir" :title="t('gitlab.window.directionHint')">
          <span :class="{ on: !isPush(b) }">{{ t('gitlab.window.pull') }}</span>
          <span :class="{ on: isPush(b) }">{{ t('gitlab.window.pullPush') }}</span>
        </span>
        <button
          type="button"
          class="glw-more"
          :title="t('common.action.more')"
          data-testid="gitlab-binding-menu"
          @click.stop="openMenu(b.id, $event.currentTarget)"
        >
          <n-icon :component="EllipsisHorizontal" :size="16" />
        </button>
      </div>
      <button
        v-if="!ro && sameBoardTarget"
        type="button"
        class="gc-more"
        data-testid="gitlab-same-board"
        @click="addSameBoard"
      >
        <n-icon :component="AddOutline" :size="14" />
        {{ t('gitlab.window.sameBoard') }}
        <span class="gc-more-b">{{ boardLabel(sameBoardTarget) }}</span>
      </button>

      <!-- THE PICKED BINDING (or the new one) -->
      <div class="glw-h">
        <template v-if="f.currentId">
          {{ t('gitlab.window.settingsOf') }}
          <span class="glw-mono gc-of">{{ f.projectPath }}</span>
        </template>
        <template v-else>{{ t('gitlab.window.newConnection') }}</template>
      </div>
      <div class="glw-card col gc-form" data-testid="gitlab-binding-form">
        <div class="glw-row">
          <div class="glw-lbl">
            <span class="glw-lbl-t">{{ t('gitlab.modal.binding.projectPath') }}</span>
          </div>
          <div class="glw-ctl">
            <n-input
              ref="pathEl"
              v-model:value="f.projectPath"
              placeholder="group/project"
              :disabled="ro"
              data-testid="gitlab-project-path"
            />
          </div>
        </div>
        <div class="glw-row">
          <div class="glw-lbl">
            <span class="glw-lbl-t">{{ t('gitlab.modal.binding.board') }}</span>
          </div>
          <div class="glw-ctl">
            <n-select
              :value="f.boardId"
              :options="f.boardOptions"
              :placeholder="t('gitlab.modal.binding.boardPlaceholder')"
              :disabled="ro"
              data-testid="gitlab-board"
              @update:value="f.onBoardChange"
            />
          </div>
        </div>
        <div class="glw-row">
          <div class="glw-lbl">
            <span class="glw-lbl-t">{{ t('gitlab.modal.binding.name') }}</span>
          </div>
          <div class="glw-ctl">
            <n-input
              v-model:value="f.name"
              :placeholder="t('gitlab.modal.binding.namePlaceholder')"
              :disabled="ro"
            />
          </div>
        </div>
        <template v-if="f.currentId">
          <div class="glw-row">
            <div class="glw-lbl">
              <span class="glw-lbl-t">{{ t('gitlab.modal.binding.scope') }}</span>
            </div>
            <div class="glw-ctl">
              <n-select v-model:value="f.scope" :options="f.scopeOptions" :disabled="ro" />
            </div>
          </div>
          <div class="glw-row">
            <div class="glw-lbl">
              <span class="glw-lbl-t">{{ t('gitlab.window.importLabels') }}</span>
              <span class="glw-lbl-s">{{ t('gitlab.window.importLabelsHint') }}</span>
            </div>
            <div class="glw-ctl">
              <n-input
                v-model:value="f.importLabels"
                class="glw-mono"
                :placeholder="LABEL_SAMPLE"
                :disabled="ro"
                data-testid="gitlab-import-labels"
              />
            </div>
          </div>
          <div class="glw-row">
            <div class="glw-lbl">
              <span class="glw-lbl-t">{{ t('gitlab.modal.binding.closedPolicy') }}</span>
            </div>
            <div class="glw-ctl">
              <n-select
                v-model:value="f.closedPolicy"
                :options="f.closedPolicyOptions"
                :disabled="ro"
              />
              <!-- The v2 date field formats by the user's preset and starts the
                   week where they chose (#2932, #3034). -->
              <DateField
                v-if="f.closedPolicy === 'period'"
                v-model:value="f.closedAfter"
                :placeholder="t('gitlab.modal.binding.closedAfter')"
                type="date"
                :disabled="ro"
                testid="gl-closed-after"
              />
            </div>
          </div>
          <div class="glw-row">
            <div class="glw-lbl">
              <span class="glw-lbl-t">{{ t('gitlab.modal.binding.dueSource') }}</span>
            </div>
            <div class="glw-ctl">
              <n-select v-model:value="f.dueSource" :options="f.dueSourceOptions" :disabled="ro" />
            </div>
          </div>
          <div class="glw-row">
            <div class="glw-lbl">
              <span class="glw-lbl-t">{{ t('gitlab.modal.binding.startSource') }}</span>
            </div>
            <div class="glw-ctl">
              <n-select
                v-model:value="f.startSource"
                :options="f.startSourceOptions"
                :disabled="ro"
              />
            </div>
          </div>
          <div class="glw-row">
            <div class="glw-lbl">
              <span class="glw-lbl-t">{{ t('gitlab.modal.binding.relations') }}</span>
              <span class="glw-lbl-s">{{ t('gitlab.modal.binding.relationsHint') }}</span>
            </div>
            <div class="glw-ctl"><n-switch v-model:value="f.relationsSync" :disabled="ro" /></div>
          </div>
          <div class="glw-row">
            <div class="glw-lbl">
              <span class="glw-lbl-t">{{ t('gitlab.modal.binding.enabled') }}</span>
            </div>
            <div class="glw-ctl"><n-switch v-model:value="f.enabled" :disabled="ro" /></div>
          </div>
        </template>
        <div v-else class="gc-new-foot">
          <span class="glw-hint">{{ t('gitlab.window.newHint') }}</span>
          <span class="glw-grow" />
          <button
            v-if="f.integrations.length"
            type="button"
            class="kit-btn small"
            @click="f.selectBinding(f.integrations[0].id)"
          >
            {{ t('common.action.cancel') }}
          </button>
          <button
            type="button"
            class="kit-btn small primary"
            :disabled="ro || f.saving || !f.projectPath.trim() || !f.boardId"
            data-testid="gitlab-create-binding"
            @click="f.save()"
          >
            {{ t('gitlab.window.connect') }}
            <Kbd>{{ CAPS.enter }}</Kbd>
          </button>
        </div>
      </div>

      <!-- WEBHOOK (GitLab → Tessera): near-realtime on top of the polling pull,
           wired up by hand — the URL and the secret go into the project's
           Settings → Webhooks. -->
      <template v-if="f.currentId">
        <div class="glw-h">{{ t('gitlab.modal.webhook.title') }}</div>
        <div class="glw-card col gc-hook">
          <p class="glw-hint gc-hook-hint">{{ t('gitlab.modal.webhook.hint') }}</p>
          <template v-if="f.webhookEnabled">
            <div class="glw-row">
              <div class="glw-lbl">
                <span class="glw-lbl-t">{{ t('gitlab.modal.webhook.url') }}</span>
              </div>
              <div class="glw-ctl">
                <n-input :value="f.webhookUrl" readonly />
                <button type="button" class="kit-btn small" @click="f.copyWebhook(f.webhookUrl)">
                  <n-icon :component="CopyOutline" :size="13" />
                  {{ t('gitlab.modal.webhook.copy') }}
                </button>
              </div>
            </div>
            <div v-if="f.webhookSecret" class="glw-row">
              <div class="glw-lbl">
                <span class="glw-lbl-t">{{ t('gitlab.modal.webhook.secret') }}</span>
              </div>
              <div class="glw-ctl">
                <n-input :value="f.webhookSecret" readonly />
                <button type="button" class="kit-btn small" @click="f.copyWebhook(f.webhookSecret)">
                  <n-icon :component="CopyOutline" :size="13" />
                  {{ t('gitlab.modal.webhook.copy') }}
                </button>
              </div>
            </div>
            <p v-if="f.webhookSecret" class="glw-warn gc-once">
              <n-icon :component="WarningOutline" :size="16" />
              <span>{{ t('gitlab.modal.webhook.onceWarning') }}</span>
            </p>
            <div class="gc-hook-foot">
              <span class="glw-hint">{{
                t('gitlab.modal.webhook.lastEvent', { at: f.lastWebhookText })
              }}</span>
              <span class="glw-grow" />
              <button
                type="button"
                class="kit-btn small"
                :disabled="ro || f.webhookBusy"
                @click="f.enableWebhook"
              >
                {{ t('gitlab.modal.webhook.rotate') }}
              </button>
              <button
                type="button"
                class="kit-btn small"
                :disabled="ro || f.webhookBusy"
                @click="f.disableWebhook"
              >
                {{ t('gitlab.modal.webhook.disable') }}
              </button>
            </div>
          </template>
          <div v-else class="gc-hook-foot">
            <span class="glw-grow" />
            <button
              type="button"
              class="kit-btn small"
              :disabled="ro || f.webhookBusy"
              data-testid="gitlab-webhook-generate"
              @click="f.enableWebhook"
            >
              <n-icon :component="FlashOutline" :size="13" />
              {{ t('gitlab.modal.webhook.generate') }}
            </button>
          </div>
        </div>
      </template>
    </div>

    <ActionMenu
      v-model:show="menuShow"
      :anchor="() => menuAnchor"
      placement="bottom-end"
      :offset="{ x: 0, y: 6 }"
      :items="menuItems"
      :width="300"
      testid="gitlab-card-menu"
      @select="onMenu"
    />
  </div>
</template>

<style scoped>
.gc-acc-t {
  font-size: 13.5px;
  font-weight: 650;
}
.gc-expiry {
  white-space: nowrap;
}
.gc-pat {
  margin-top: 8px;
}
.gc-ro {
  margin: 0 0 10px;
}
.gc-dir {
  pointer-events: none;
}
.gc-more {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  height: 38px;
  margin-top: 6px;
  padding: 0 14px;
  border: 1px dashed var(--t-border);
  border-radius: 10px;
  background: none;
  font: inherit;
  font-size: 12.5px;
  color: var(--t-text2);
  cursor: pointer;
}
.gc-more:hover {
  color: var(--t-text1);
  border-color: color-mix(in srgb, var(--t-primary) 45%, var(--t-border));
}
.gc-more-b {
  color: var(--t-text3);
}
.gc-expiry.err,
.gc-err {
  color: var(--t-error);
}
.gc-svc {
  white-space: nowrap;
}
.gc-of {
  font-weight: 600;
  color: var(--t-text2);
}
.gc-form {
  gap: 2px;
}
.gc-new-foot,
.gc-hook-foot {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding-top: 6px;
}
.gc-hook {
  gap: 4px;
}
.gc-hook-hint {
  margin: 0 0 6px;
}
.gc-once {
  margin: 6px 0;
}
.lw-mobile .gc-dir {
  display: none;
}
</style>
