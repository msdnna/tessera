<script setup>
// Settings (v2, #3106, mockup SettingsPage): a sunken section column on the
// left (--t-sub, radius 18 — the «Проект» window's layout), the open section on
// the right. One section at a time; the open one is in the address
// (`?s=notifications`) so a link and Back land on it. What each section does is
// unchanged — the forms moved into components/settings/*.
import { computed, nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NIcon, NTooltip } from 'naive-ui'
import {
  BulbOutline,
  ColorPaletteOutline,
  DesktopOutline,
  KeypadOutline,
  LanguageOutline,
  NotificationsOutline,
  PersonOutline,
  ShieldCheckmarkOutline,
} from '@vicons/ionicons5'
import UserAvatar from '@/components/UserAvatar.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import NotificationSettings from '@/components/NotificationSettings.vue'
import SettingsProfile from '@/components/settings/SettingsProfile.vue'
import SettingsSecurity from '@/components/settings/SettingsSecurity.vue'
import SettingsAppearance from '@/components/settings/SettingsAppearance.vue'
import SettingsLocale from '@/components/settings/SettingsLocale.vue'
import SettingsApp from '@/components/settings/SettingsApp.vue'
import { useAuthStore } from '@/stores/auth'
import { useQuickStore } from '@/stores/quick'
import { useResponsive } from '@/composables/useResponsive'
import { isTauri } from '@/utils/serverBase'
import { stubTask } from '@/utils/featureStubs'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const quick = useQuickStore()
const { isMobile } = useResponsive()
const isDesktop = isTauri()

const SECTIONS = {
  profile: { icon: PersonOutline, comp: SettingsProfile },
  security: { icon: ShieldCheckmarkOutline, comp: SettingsSecurity },
  appearance: { icon: ColorPaletteOutline, comp: SettingsAppearance },
  locale: { icon: LanguageOutline, comp: SettingsLocale },
  notifications: { icon: NotificationsOutline, comp: NotificationSettings },
  app: { icon: DesktopOutline, comp: SettingsApp },
}
const MAIN = ['profile', 'security', 'appearance', 'locale', 'notifications']

const section = computed(() => {
  const s = route?.query?.s
  return typeof s === 'string' && SECTIONS[s] ? s : 'profile'
})
const current = computed(() => SECTIONS[section.value])
const hintsTask = stubTask('settings.hints')

function pick(key) {
  if (key === section.value) return
  router.replace({ query: { ...route.query, s: key } })
}

// ↑↓ walk the column like a list: the focus moves and a section opens with it.
const navEl = ref(null)
function onNavKey(e) {
  if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
  const items = [...(navEl.value?.querySelectorAll('[data-nav]') || [])]
  if (!items.length) return
  e.preventDefault()
  let i = items.indexOf(document.activeElement)
  if (i < 0) i = items.findIndex((el) => el.dataset.nav === section.value)
  const next = items[(i + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length]
  next.focus()
  if (SECTIONS[next.dataset.nav]) pick(next.dataset.nav)
}

// On a phone the column is a strip of chips: bring the open one into view.
onMounted(() => {
  if (!isMobile.value) return
  nextTick(() =>
    navEl.value
      ?.querySelector('.sv-item.active')
      ?.scrollIntoView?.({ block: 'nearest', inline: 'center' }),
  )
})

const paneEl = ref(null)
function select(key) {
  pick(key)
  nextTick(() => paneEl.value?.scrollTo?.({ top: 0 }))
}
</script>

<template>
  <div class="sv" :class="{ mobile: isMobile }" data-testid="settings-view">
    <nav
      ref="navEl"
      class="sv-nav"
      :aria-label="t('settings.title')"
      data-testid="settings-nav"
      @keydown="onNavKey"
    >
      <div v-if="!isMobile" class="sv-me">
        <UserAvatar
          class="sv-me-ava"
          :user-id="auth.user?.id"
          :name="auth.user?.name || auth.user?.email"
          :src="auth.user?.avatar_url"
        />
        <span class="sv-me-who">
          <span class="sv-me-name">{{ auth.user?.name || auth.user?.email }}</span>
          <span class="sv-me-mail">{{ auth.user?.email }}</span>
        </span>
      </div>
      <button
        v-for="key in MAIN"
        :key="key"
        type="button"
        class="sv-item"
        :class="{ active: section === key }"
        :aria-current="section === key ? 'page' : undefined"
        :data-nav="key"
        :data-testid="`settings-nav-${key}`"
        @click="select(key)"
      >
        <n-icon :component="SECTIONS[key].icon" :size="16" class="sv-item-ic" />
        <span class="sv-item-l">{{ t(`settings.nav.${key}`) }}</span>
      </button>
      <span class="sv-gap" />
      <button
        type="button"
        class="sv-item"
        :class="{ active: section === 'app' }"
        data-nav="app"
        data-testid="settings-nav-app"
        @click="select('app')"
      >
        <n-icon :component="DesktopOutline" :size="16" class="sv-item-ic" />
        <span class="sv-item-l">{{ t('settings.nav.app') }}</span>
        <span v-if="!isDesktop" class="sv-pill">{{ t('settings.nav.desktopPill') }}</span>
      </button>
      <button
        v-if="!isMobile"
        type="button"
        class="sv-item"
        data-nav="keys"
        data-testid="settings-nav-keys"
        @click="quick.sheetOpen = true"
      >
        <n-icon :component="KeypadOutline" :size="16" class="sv-item-ic" />
        <span class="sv-item-l">{{ t('settings.nav.keys') }}</span>
        <ShortcutKeys id="nav.shortcuts" class="sv-item-k" />
      </button>
      <n-tooltip :delay="300" placement="right">
        <template #trigger>
          <span class="sv-item stub" data-testid="settings-nav-hints" aria-disabled="true">
            <n-icon :component="BulbOutline" :size="16" class="sv-item-ic" />
            <span class="sv-item-l">{{ t('settings.nav.hints') }}</span>
            <span class="sv-pill">{{ t('common.stub.badge') }}</span>
          </span>
        </template>
        {{ t('common.stub.soon', { n: hintsTask }) }}
      </n-tooltip>
      <span v-if="!isMobile" class="sv-grow" />
      <div v-if="!isMobile" class="sv-foot">
        <ShortcutKeys id="go.settings" bare />
        <span>{{ t('settings.nav.footHint') }}</span>
      </div>
    </nav>

    <section ref="paneEl" class="sv-pane" :data-section="section">
      <header class="sv-head">
        <h1 class="sv-title">{{ t(`settings.nav.${section}`) }}</h1>
        <span class="sv-sub">{{ t(`settings.sub.${section}`) }}</span>
      </header>
      <div class="sv-body">
        <component :is="current.comp" />
      </div>
    </section>
  </div>
</template>

<style scoped>
.sv {
  display: grid;
  grid-template-columns: 232px minmax(0, 1fr);
  gap: 28px;
  height: 100%;
  min-height: 0;
}
.sv-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px;
  border-radius: var(--t-r-col);
  background: var(--t-sub);
  min-height: 0;
  overflow-y: auto;
}
.sv-me {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 8px 14px;
  min-width: 0;
}
.sv-me-ava {
  flex: none;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 13px;
  font-weight: 650;
}
.sv-me-who {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.sv-me-name {
  font-size: 13.5px;
  font-weight: 650;
  color: var(--t-text1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sv-me-mail {
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sv-item {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: none;
  height: 34px;
  padding: 0 10px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
}
.sv-item:hover {
  background: var(--t-hover);
}
.sv-item:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.sv-item.active {
  background: var(--t-active);
  font-weight: 650;
}
.sv-item.active .sv-item-ic {
  color: var(--t-primary);
}
.sv-item.active .sv-item-ic :deep([stroke='currentColor']) {
  stroke: url(#t-accent-grad-svg);
}
.sv-item.stub {
  opacity: 0.55;
  cursor: default;
}
.sv-item.stub:hover {
  background: transparent;
}
.sv-item-ic {
  flex: none;
  color: var(--t-text2);
}
.sv-item-l {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sv-pill {
  flex: none;
  padding: 2px 7px;
  border-radius: var(--t-r-pill);
  background: var(--t-pill);
  color: var(--t-text3);
  font-size: 10.5px;
  font-weight: 650;
}
.sv-gap {
  height: 14px;
  flex: none;
}
.sv-grow {
  flex: 1;
}
.sv-foot {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 8px 4px;
  font-size: 11.5px;
  color: var(--t-text3);
}
.sv-pane {
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  padding: 8px 4px 40px 0;
}
.sv-head {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin: 10px 0 20px;
}
.sv-title {
  margin: 0;
  font-size: 26px;
  font-weight: 700;
  color: var(--t-text1);
}
.sv-sub {
  font-size: 13px;
  color: var(--t-text3);
}
.sv-body {
  max-width: 880px;
}
/* Without hover there is no keyboard to press: no key caps (v2 rule). */
@media (hover: none) {
  .sv-item-k,
  .sv-foot {
    display: none;
  }
}

/* Phone: the column becomes a strip of chips over the section. */
.sv.mobile {
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: auto;
}
.sv.mobile .sv-nav {
  flex-direction: row;
  overflow-x: auto;
  overflow-y: hidden;
  padding: 6px;
  border-radius: var(--t-r-card);
  scrollbar-width: none;
}
.sv.mobile .sv-gap {
  width: 6px;
  height: auto;
}
.sv.mobile .sv-item {
  height: 32px;
}
.sv.mobile .sv-item-l {
  overflow: visible;
}
.sv.mobile .sv-pane {
  overflow: visible;
  padding: 0;
}
.sv.mobile .sv-head {
  margin: 6px 0 14px;
}
.sv.mobile .sv-title {
  font-size: 22px;
}
</style>
