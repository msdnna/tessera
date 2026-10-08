<script setup>
import { computed, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NIcon, NPopover, NTooltip, NAvatar, NDropdown } from 'naive-ui'
import {
  EllipsisHorizontalOutline,
  HelpCircleOutline,
  LibraryOutline,
  LogOutOutline,
  SchoolOutline,
  ServerOutline,
  SettingsOutline,
  ShieldCheckmarkOutline,
} from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useTourLauncher } from '@/composables/useTourLauncher'
import { useHelpStore } from '@/stores/help'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useApiImage } from '@/composables/useApiImage'
import { initials as initialsOf } from '@/utils/initials'

const props = defineProps({
  mobile: { type: Boolean, default: false },
  collapsed: { type: Boolean, default: false },
})

const { t } = useI18n()
const authStore = useAuthStore()
const router = useRouter()
const { launch: launchTour } = useTourLauncher()
const help = useHelpStore()
const dialogs = useWsDialogsStore()

// useApiImage: direct URL on web; an axios-fetched blob: URL on desktop (the
// webview can't load the remote '/api/…/avatar' <img> directly).
const avatarUrl = useApiImage(() => authStore.user?.avatar_url || '')
const isAdmin = computed(() => authStore.isAdmin)
function openSettings() {
  router.push('/settings')
}
function openAdmin() {
  router.push('/admin')
}
function openHelp() {
  help.openCenter()
}

// The tour's permanent entry point (#2753): the autostart only ever fires once
// per account, so this is how anyone re-runs it — «Основы» from the start, from
// «Мои задачи» (on a phone: the phone's «Основы», #3000).
function startTour() {
  launchTour('start')
}

// One «Помощь» button in the footer covering both ways of answering "how does
// this work?" — the guide walks the UI, the help centre is read. They used to be
// two separate controls in two different places (a footer icon and a sidebar
// item); a single question-mark menu is where a reader looks for either.
// Computed, not a plain array: the labels come from the locale, and switching
// the language has to redraw the menu.
const helpOptions = computed(() => [
  {
    key: 'tour',
    label: t('shell.user.tour'),
    icon: () => h(NIcon, { component: SchoolOutline }),
    props: { 'data-help-menu': 'tour' },
  },
  {
    key: 'center',
    label: t('shell.user.helpCenter'),
    icon: () => h(NIcon, { component: LibraryOutline }),
    props: { 'data-help-menu': 'center' },
  },
])

// Admin and logout are the rare actions: in the v2 footer (#2953) they sit in a
// «⋯» menu so the name and e-mail keep their room at the 232px sidebar width.
// Background jobs run for the whole instance, not a workspace — they joined the
// administration here when the workspace tool row went away.
const moreOptions = computed(() => [
  ...(isAdmin.value
    ? [
        {
          key: 'jobs',
          label: t('shell.tools.jobs'),
          icon: () => h(NIcon, { component: ServerOutline }),
          props: { 'data-testid': 'jobs-button' },
        },
        {
          key: 'admin',
          label: t('shell.user.admin'),
          icon: () => h(NIcon, { component: ShieldCheckmarkOutline }),
        },
      ]
    : []),
  {
    key: 'logout',
    label: t('shell.user.logout'),
    icon: () => h(NIcon, { component: LogOutOutline }),
    props: { 'data-testid': 'logout' },
  },
])

function onMoreSelect(key) {
  if (key === 'admin') openAdmin()
  else if (key === 'jobs') dialogs.open('jobs')
  else logout()
}

function onHelpSelect(key) {
  if (key === 'tour') startTour()
  else openHelp()
}

// The one initials rule (utils/initials), not a local re-derivation of it: this
// disc sits on every screen next to the very cards whose avatars use the shared
// helper, and the copy that used to live here knew only the two-word case. A
// dot-separated handle — «a.fokin», the shape a GitLab roster hands us — came out
// as «A.», a letter and a punctuation mark, while the same person was «AF» on
// their card. The e-mail fallback below made it the common case, not the exotic one.
const initials = computed(() => initialsOf(authStore.user?.name || authStore.user?.email || '?'))

// Only the collapsed desktop rail uses the avatar-popover; the mobile drawer is
// wide enough for the inline name + icon buttons (settings / admin / logout).
const compact = computed(() => props.collapsed)

function logout() {
  authStore.logout()
  router.push('/login')
}
</script>

<template>
  <div class="sb-footer" :class="{ collapsed }" data-tour="sb-footer">
    <n-popover v-if="compact" trigger="click" :placement="collapsed ? 'right-end' : 'top-start'">
      <template #trigger>
        <img v-if="avatarUrl" :src="avatarUrl" class="ava ava-img" alt="" />
        <n-avatar v-else round :size="32" class="ava">{{ initials }}</n-avatar>
      </template>
      <div class="user-pop">
        <div class="up-name">{{ authStore.user?.name || t('shell.user.profile') }}</div>
        <div class="up-mail">{{ authStore.user?.email }}</div>
        <!-- Same anchor on both shapes of the footer (compact popover / full
             row): only one of them is ever in the DOM. -->
        <n-button size="small" block data-tour="footer-settings" @click="openSettings">
          <template #icon><n-icon :component="SettingsOutline" /></template>
          {{ t('shell.user.settings') }}
        </n-button>
        <!-- The collapsed rail spells the two help entries out as rows instead of
             nesting a dropdown inside this popover: same two destinations as the
             expanded footer's «Помощь» menu, one click closer. -->
        <n-button size="small" block data-tour="footer-tour" @click="startTour">
          <template #icon><n-icon :component="SchoolOutline" /></template>
          {{ t('shell.user.tour') }}
        </n-button>
        <n-button size="small" block data-help-center-open @click="openHelp">
          <template #icon><n-icon :component="LibraryOutline" /></template>
          {{ t('shell.user.helpCenter') }}
        </n-button>
        <n-button v-if="isAdmin" size="small" block @click="dialogs.open('jobs')">
          <template #icon><n-icon :component="ServerOutline" /></template>
          {{ t('shell.tools.jobs') }}
        </n-button>
        <n-button v-if="isAdmin" size="small" block @click="openAdmin">
          <template #icon><n-icon :component="ShieldCheckmarkOutline" /></template>
          {{ t('shell.user.admin') }}
        </n-button>
        <n-button size="small" block data-testid="logout" @click="logout">
          <template #icon><n-icon :component="LogOutOutline" /></template>
          {{ t('shell.user.logout') }}
        </n-button>
      </div>
    </n-popover>
    <div v-else class="user">
      <img v-if="avatarUrl" :src="avatarUrl" class="ava ava-img" alt="" @click="openSettings" />
      <n-avatar v-else round :size="30" class="ava" @click="openSettings">{{ initials }}</n-avatar>
      <span class="uwho" @click="openSettings">
        <span class="uname">{{ authStore.user?.name || t('shell.user.profile') }}</span>
        <span v-if="authStore.user?.email" class="umail">{{ authStore.user.email }}</span>
      </span>
      <!-- «Помощь» (#2792): one question-mark button opening the guide or the
           help centre. The centre is a modal, so picking it keeps the board
           behind it — that is why it is here and no longer a sidebar item. -->
      <n-dropdown
        trigger="click"
        placement="top-start"
        :options="helpOptions"
        @select="onHelpSelect"
      >
        <n-tooltip>
          <template #trigger>
            <n-button
              quaternary
              circle
              size="small"
              :aria-label="t('shell.user.help')"
              data-tour="footer-help"
              data-nav="help"
            >
              <n-icon :component="HelpCircleOutline" />
            </n-button>
          </template>
          {{ t('shell.user.help') }}
        </n-tooltip>
      </n-dropdown>
      <n-tooltip>
        <template #trigger>
          <n-button
            quaternary
            circle
            size="small"
            :aria-label="t('shell.user.settings')"
            data-tour="footer-settings"
            @click="openSettings"
          >
            <n-icon :component="SettingsOutline" />
          </n-button>
        </template>
        {{ t('shell.user.settings') }}
      </n-tooltip>
      <n-dropdown trigger="click" placement="top-end" :options="moreOptions" @select="onMoreSelect">
        <n-button
          quaternary
          circle
          size="small"
          :aria-label="t('shell.user.more')"
          data-testid="user-menu"
        >
          <n-icon :component="EllipsisHorizontalOutline" />
        </n-button>
      </n-dropdown>
    </div>
  </div>
</template>

<style scoped>
/* v2 (#2953): a capsule one level below the sidebar instead of a line above. */
.sb-footer {
  margin: 8px;
  padding: 8px;
  border-radius: 12px;
  background: var(--t-foot);
}
.sb-footer.collapsed {
  display: flex;
  justify-content: center;
  margin: 8px auto;
  padding: 5px;
}
.user {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ava {
  flex: none;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.ava-img {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  object-fit: cover;
}
.uwho {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  cursor: pointer;
}
.uname,
.umail {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.uname {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
}
.umail {
  font-size: 11.5px;
  color: var(--t-text3);
}
.user-pop {
  width: 200px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
/* Left-align the icon+label inside the block buttons (default is centred). */
.user-pop :deep(.n-button__content) {
  width: 100%;
  justify-content: flex-start;
}
.up-name {
  font-weight: 600;
  color: var(--t-text1);
}
.up-mail {
  font-size: 12px;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
