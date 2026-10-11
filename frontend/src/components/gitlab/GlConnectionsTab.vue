<script setup>
// «Подключения» of the GitLab window (#2975, mockup GitlabWin fGlConn): your
// GitLab account, the workspace's bindings as cards (project → board,
// direction, state), the settings of the picked one and its webhook. A new
// binding is filled in place, without a modal of its own.
//
// Several GitLab projects on one board, the OAuth connection and the token's
// expiry are #3065 — drawn, not wired.
import { computed, inject, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NInput, NSelect, NSwitch, NTooltip } from 'naive-ui'
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
import { stubTask } from '@/utils/featureStubs'
import { cap, CAPS } from '@/utils/keyCaps'
import ActionMenu from '../kit/ActionMenu.vue'
import Kbd from '../Kbd.vue'

const f = inject(GITLAB_FORM)
const { t } = useI18n()
const stub = stubTask('gitlab.connections')
const ro = computed(() => !f.isAdmin)

const boardLabel = (id) => f.boardOptions.find((o) => o.value === id)?.label || ''
const scopeLabel = (v) => f.scopeOptions.find((o) => o.value === v)?.label || ''
function cardState(b) {
  if (b.enabled === false) return { key: 'off', tone: '' }
  if (!f.gl.connected && !f.serviceConfigured) return { key: 'noCreds', tone: 'err' }
  return { key: 'on', tone: 'ok' }
}
const isPush = (b) => b.writeback?.enabled === true

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
              {{ f.gl.baseUrl }} · {{ t('gitlab.window.pat') }}
              <n-tooltip :delay="300">
                <template #trigger>
                  <span class="glw-stub gc-expiry">· {{ t('gitlab.window.tokenExpiry') }}</span>
                </template>
                {{ t('common.stub.soon', { n: stub }) }}
              </n-tooltip>
            </div>
          </template>
          <template v-else>
            <div class="glw-line">
              <span class="gc-acc-t">{{ t('gitlab.window.noAccount') }}</span>
            </div>
            <div class="glw-sub">{{ t('gitlab.window.noAccountSub') }}</div>
          </template>
        </div>
        <n-tooltip :delay="300">
          <template #trigger>
            <span
              class="kit-btn glw-stub glw-hide-m"
              aria-disabled="true"
              data-testid="gitlab-oauth-stub"
            >
              <n-icon :component="LogoGitlab" :size="15" />
              {{ t('gitlab.window.oauth') }}
            </span>
          </template>
          {{ t('common.stub.soon', { n: stub }) }}
        </n-tooltip>
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
      <div v-if="!f.gl.connected" class="glw-card col gc-pat">
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
            ·
            {{
              b.last_synced_at
                ? t('gitlab.window.lastPull', { ago: f.ago(b.last_synced_at) })
                : t('gitlab.modal.sync.never')
            }}
          </div>
        </div>
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
      <n-tooltip :delay="300">
        <template #trigger>
          <div class="gc-more-stub glw-stub" aria-disabled="true">
            <n-icon :component="AddOutline" :size="14" />
            {{ t('gitlab.window.sameBoard') }}
            <span class="gc-soon">{{ t('common.stub.badge') }}</span>
          </div>
        </template>
        {{ t('common.stub.soon', { n: stub }) }}
      </n-tooltip>

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
.gc-more-stub {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 38px;
  margin-top: 6px;
  padding: 0 14px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.gc-soon {
  padding: 1px 6px;
  border-radius: 6px;
  background: var(--t-pill);
  font-size: 11px;
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
