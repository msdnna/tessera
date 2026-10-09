<script setup>
// «Ещё» (v2, #2976 — package G «Ещё · настройки»): the fifth tab of the phone's
// bottom navigation. It gathers what the desktop keeps in the sidebar's rows
// and its footer: the profile, stages, documents, meetings, notifications, the
// theme and the accent, the language, help, the administration and signing
// out. Each row is a 56px target on the card level.
//
// A phone screen: on a wider window there is no fifth tab, and the address
// opens the settings instead.
import { computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import {
  ChevronForwardOutline,
  TrailSignOutline,
  DocumentTextOutline,
  VideocamOutline,
  NotificationsOutline,
  LanguageOutline,
  HelpCircleOutline,
  SparklesOutline,
  SchoolOutline,
  SettingsOutline,
  ShieldCheckmarkOutline,
  ServerOutline,
  LogOutOutline,
  PhonePortraitOutline,
  SunnyOutline,
  MoonOutline,
} from '@vicons/ionicons5'
import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useNotificationsStore } from '@/stores/notifications'
import { useThemeStore, COLOR_THEMES } from '@/stores/theme'
import { useHelpStore } from '@/stores/help'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { useTourLauncher } from '@/composables/useTourLauncher'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useResponsive } from '@/composables/useResponsive'
import { useVersionInfo } from '@/composables/useVersionInfo'
import { workspaceName } from '@/utils/defaultNames'
import { swatchBg } from '@/utils/gradient'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const ws = useWorkspacesStore()
const notes = useNotificationsStore()
const theme = useThemeStore()
const help = useHelpStore()
const whatsNew = useWhatsNewStore()
const { launch: launchTour } = useTourLauncher()
const dialogs = useWsDialogsStore()
const { isMobile } = useResponsive()
const { web } = useVersionInfo()

onMounted(() => {
  if (!isMobile.value) router.replace('/settings')
})
watch(isMobile, (m) => !m && router.replace('/settings'))

// Each section's tile in a hue of its own, from the accent palette.
const hue = (key) => COLOR_THEMES.find((c) => c.key === key)?.primary || 'var(--t-primary)'
const PLACES = computed(() => [
  {
    key: 'milestones',
    label: 'shell.nav.milestones',
    icon: TrailSignOutline,
    tone: hue('purple'),
    to: '/milestones',
  },
  {
    key: 'documents',
    label: 'shell.nav.documents',
    icon: DocumentTextOutline,
    tone: hue('blue'),
    to: '/documents',
  },
  {
    key: 'conferences',
    label: 'shell.nav.conferences',
    icon: VideocamOutline,
    tone: hue('teal'),
    to: '/conferences',
  },
  {
    key: 'notifications',
    label: 'shell.tools.notifications',
    icon: NotificationsOutline,
    tone: hue('orange'),
    to: '/notifications',
    count: notes.unread,
  },
])
const MODES = [
  { key: 'system', icon: PhonePortraitOutline },
  { key: 'light', icon: SunnyOutline },
  { key: 'dark', icon: MoonOutline },
]
const tile = (tone) => ({
  background: `linear-gradient(to top right, color-mix(in srgb, ${tone} 16%, transparent), color-mix(in srgb, ${tone} 26%, transparent))`,
  color: tone,
})

const language = computed(() => t(`common.language.${theme.language === 'en' ? 'en' : 'ru'}`))
const host = typeof location !== 'undefined' ? location.host : ''

// The phone's «Основы» (#3000) — a bottom sheet over the navigation.
function startTour() {
  launchTour('start')
}
function logout() {
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <div class="mv" data-testid="more-page">
    <h1 class="mv-title">{{ t('shell.user.more') }}</h1>

    <button
      type="button"
      class="mv-card mv-me"
      data-testid="more-profile"
      @click="router.push('/settings')"
    >
      <UserAvatar
        :user-id="auth.user?.id || ''"
        :name="auth.user?.name || auth.user?.email || ''"
        class="mv-av"
      />
      <span class="mv-who">
        <span class="mv-name">{{ auth.user?.name || t('shell.user.profile') }}</span>
        <span class="mv-sub">{{ auth.user?.email }} · {{ workspaceName(ws.current) }}</span>
      </span>
      <n-icon :component="ChevronForwardOutline" :size="18" class="mv-chev" />
    </button>

    <div class="mv-card">
      <button
        v-for="p in PLACES"
        :key="p.key"
        type="button"
        class="mv-row"
        :data-testid="`more-${p.key}`"
        @click="router.push(p.to)"
      >
        <span class="mv-tile" :style="tile(p.tone)"><n-icon :component="p.icon" :size="20" /></span>
        <span class="mv-l">{{ t(p.label) }}</span>
        <span v-if="p.count" class="mv-count">{{ p.count > 99 ? '99+' : p.count }}</span>
        <n-icon :component="ChevronForwardOutline" :size="18" class="mv-chev" />
      </button>
    </div>

    <div class="mv-sec">{{ t('shell.mobile.appearance') }}</div>
    <div class="mv-card mv-look">
      <div class="mv-look-l">{{ t('settings.appearance.theme') }}</div>
      <div class="mv-seg" role="radiogroup" :aria-label="t('settings.appearance.theme')">
        <button
          v-for="m in MODES"
          :key="m.key"
          type="button"
          role="radio"
          class="mv-seg-b"
          :class="{ on: theme.themeMode === m.key }"
          :aria-checked="theme.themeMode === m.key"
          :data-testid="`more-theme-${m.key}`"
          @click="theme.setThemeMode(m.key)"
        >
          <n-icon :component="m.icon" :size="16" />
          {{ t(`settings.appearance.mode.${m.key}`) }}
        </button>
      </div>
      <div class="mv-accent">
        <span class="mv-look-l">{{ t('settings.appearance.accent') }}</span>
        <div class="mv-sw" role="radiogroup" :aria-label="t('settings.appearance.accent')">
          <button
            v-for="ct in COLOR_THEMES"
            :key="ct.key"
            type="button"
            role="radio"
            class="mv-sw-b"
            :class="{ on: theme.activeTheme?.key === ct.key }"
            :aria-checked="theme.activeTheme?.key === ct.key"
            :aria-label="t(`settings.appearance.color.${ct.key}`)"
            :style="{ backgroundImage: swatchBg(ct.primary), '--sw': ct.primary }"
            @click="theme.selectColor(ct)"
          />
        </div>
      </div>
    </div>

    <div class="mv-card">
      <button
        type="button"
        class="mv-row"
        data-testid="more-language"
        @click="router.push({ path: '/settings', query: { s: 'locale' } })"
      >
        <span class="mv-tile neutral"><n-icon :component="LanguageOutline" :size="20" /></span>
        <span class="mv-l">
          {{ t('settings.localization.language') }}
          <span class="mv-l-sub">{{ language }}</span>
        </span>
        <n-icon :component="ChevronForwardOutline" :size="18" class="mv-chev" />
      </button>
      <button type="button" class="mv-row" data-testid="more-help" @click="help.openCenter()">
        <span class="mv-tile neutral"><n-icon :component="HelpCircleOutline" :size="20" /></span>
        <span class="mv-l">{{ t('shell.user.helpCenter') }}</span>
        <n-icon :component="ChevronForwardOutline" :size="18" class="mv-chev" />
      </button>
      <button
        type="button"
        class="mv-row"
        data-testid="more-whatsnew"
        @click="whatsNew.openHistory()"
      >
        <span class="mv-tile neutral"><n-icon :component="SparklesOutline" :size="20" /></span>
        <span class="mv-l">{{ t('shell.mobile.whatsNew') }}</span>
        <n-icon :component="ChevronForwardOutline" :size="18" class="mv-chev" />
      </button>
      <button type="button" class="mv-row" data-testid="more-tour" @click="startTour">
        <span class="mv-tile neutral"><n-icon :component="SchoolOutline" :size="20" /></span>
        <span class="mv-l">{{ t('shell.user.tour') }}</span>
        <n-icon :component="ChevronForwardOutline" :size="18" class="mv-chev" />
      </button>
    </div>

    <div class="mv-card">
      <button
        type="button"
        class="mv-row"
        data-testid="more-settings"
        @click="router.push('/settings')"
      >
        <span class="mv-tile neutral"><n-icon :component="SettingsOutline" :size="20" /></span>
        <span class="mv-l">{{ t('shell.user.settings') }}</span>
        <n-icon :component="ChevronForwardOutline" :size="18" class="mv-chev" />
      </button>
      <template v-if="auth.isAdmin">
        <button
          type="button"
          class="mv-row"
          data-testid="more-admin"
          @click="router.push('/admin')"
        >
          <span class="mv-tile neutral"
            ><n-icon :component="ShieldCheckmarkOutline" :size="20"
          /></span>
          <span class="mv-l">{{ t('shell.user.admin') }}</span>
          <n-icon :component="ChevronForwardOutline" :size="18" class="mv-chev" />
        </button>
        <button
          type="button"
          class="mv-row"
          data-testid="jobs-button"
          @click="dialogs.open('jobs')"
        >
          <span class="mv-tile neutral"><n-icon :component="ServerOutline" :size="20" /></span>
          <span class="mv-l">{{ t('shell.tools.jobs') }}</span>
          <n-icon :component="ChevronForwardOutline" :size="18" class="mv-chev" />
        </button>
      </template>
      <button type="button" class="mv-row danger" data-testid="logout" @click="logout">
        <span class="mv-tile"><n-icon :component="LogOutOutline" :size="20" /></span>
        <span class="mv-l">{{ t('shell.user.logout') }}</span>
      </button>
    </div>

    <div class="mv-ver">Tessera {{ web.version }} · {{ host }}</div>
  </div>
</template>

<style scoped>
.mv {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 560px;
  margin: 0 auto;
  padding: 4px 2px 24px;
}
.mv-title {
  margin: 8px 6px 6px;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--t-text1);
}
.mv-card {
  display: flex;
  flex-direction: column;
  padding: 4px;
  border-radius: var(--t-r-card);
  background: var(--t-card);
}
.mv-me {
  flex-direction: row;
  align-items: center;
  gap: 14px;
  padding: 14px 12px 14px 14px;
  border: 0;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.mv-av {
  flex: none;
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 20px;
  font-weight: 700;
}
.mv-who {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.mv-name {
  font-size: 18px;
  font-weight: 650;
  color: var(--t-text1);
}
.mv-sub {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: var(--t-text3);
}
.mv-row {
  display: flex;
  align-items: center;
  gap: 14px;
  min-height: 56px;
  padding: 8px 10px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 15.5px;
  font-weight: 550;
  text-align: left;
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
.mv-row:active,
.mv-me:active {
  background: var(--t-hover);
}
.mv-tile {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 12px;
}
.mv-tile.neutral {
  background: var(--t-pill);
  color: var(--t-text2);
}
.mv-l {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.mv-l-sub {
  font-size: 13px;
  font-weight: 400;
  color: var(--t-text3);
}
.mv-chev {
  flex: none;
  color: var(--t-text3);
}
.mv-count {
  min-width: 22px;
  height: 22px;
  padding: 0 7px;
  box-sizing: border-box;
  border-radius: 999px;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  line-height: 22px;
  text-align: center;
}
.mv-row.danger {
  color: var(--t-error);
}
.mv-row.danger .mv-tile {
  background: color-mix(in srgb, var(--t-error) 14%, transparent);
}
.mv-sec {
  margin: 10px 8px 0;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.mv-look {
  gap: 12px;
  padding: 14px;
}
.mv-look-l {
  font-size: 15.5px;
  font-weight: 550;
  color: var(--t-text1);
}
.mv-seg {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 2px;
  padding: 3px;
  border-radius: 12px;
  background: var(--t-sub);
}
.mv-seg-b {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 0;
  height: 40px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13.5px;
  white-space: nowrap;
  cursor: pointer;
}
.mv-seg-b.on {
  background: var(--t-pop);
  color: var(--t-text1);
  font-weight: 650;
  box-shadow: var(--t-sh-card);
}
.mv-accent {
  display: flex;
  align-items: center;
  gap: 12px;
}
.mv-sw {
  flex: 1;
  display: flex;
  justify-content: space-between;
  gap: 4px;
}
.mv-sw-b {
  flex: none;
  width: 34px;
  height: 34px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  cursor: pointer;
}
.mv-sw-b.on {
  box-shadow:
    0 0 0 2px var(--t-card),
    0 0 0 4px var(--sw);
}
.mv-ver {
  margin-top: 4px;
  text-align: center;
  font-size: 12.5px;
  color: var(--t-text3);
}
</style>
