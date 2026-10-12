<script setup>
// The account screens' frame (v2, #3107 — PackageE «Вход и аккаунт»): the
// «Моих задач» aurora over the top of the page, held still (a form is being
// filled in under it), and one glass card in the middle — the logo, a title
// with a line under it, then the slotted form. Login, registration, recovery,
// the new password, the e-mail check, the invitation, the OAuth hand-off and
// the 404 all share it.
//
// The page's own tools sit in a bar along the bottom: language, theme and —
// on the web — the app download on the left, the build and the host on the
// right. The desktop client puts its server address into the card's head as a
// pill (the address is part of signing in there, not a page preference).
//
// Field / button styling for the slotted form lives in main.css under `.auth`
// (scoped styles can't reach slotted nodes); Naive's sizes come from the nested
// config provider below.
import { ref, computed, onMounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NPopover, NInput, NButton, NConfigProvider } from 'naive-ui'
import {
  SunnyOutline,
  MoonOutline,
  ServerOutline,
  LanguageOutline,
  ChevronDownOutline,
} from '@vicons/ionicons5'
import { useThemeStore } from '@/stores/theme'
import { SUPPORTED_LOCALES, normalizeLocale } from '@/i18n'
import { isTauri, serverBase, setServerBase } from '@/utils/serverBase'
import { serverUrlMessage } from '@/utils/servers'
import DownloadAppButton from '@/components/DownloadAppButton.vue'
import BrandLogo from '@/components/BrandLogo.vue'
import HomeAurora from '@/components/home/HomeAurora.vue'

const props = defineProps({
  title: { type: String, default: '' },
  // The line under the title («Задачи, документы и встречи команды»).
  subtitle: { type: String, default: '' },
  // A badge over the title for the screens that are a state rather than a form
  // (recovery, the e-mail check, the invitation): an ionicons5 component…
  icon: { type: [Object, Function], default: null },
  // …in its own tone — a key in amber, a letter in blue. Defaults to the accent.
  tone: { type: String, default: '' },
})

const { t } = useI18n()
const theme = useThemeStore()

// The badge tone: a soft wash of the hue (see-through, it sits on the glass)
// with the glyph in the hue itself.
const badgeStyle = computed(() => {
  const hue = props.tone || 'var(--t-primary)'
  return { background: `color-mix(in srgb, ${hue} 18%, transparent)`, color: hue }
})

// Desktop only: let the user point the app at a self-hosted server before
// logging in (mirrors the Android server-URL setting). Empty = built-in default.
const desktop = isTauri()
const showServer = ref(false)
const serverInput = ref(serverBase())
// Same rule as the rail's edit modal. An address that is not an absolute
// http(s) origin is refused rather than stored: it would leave axios with a
// relative baseURL and — because re-pointing a profile at another host purges
// its namespace — take the session of the server being aimed at with it.
const serverError = computed(() => serverUrlMessage(serverInput.value, t))
function saveServer() {
  if (!setServerBase(serverInput.value)) return
  // Reload so axios (baseURL) and the WS reconnect against the new origin.
  location.reload()
}
// The pill names the server the client talks to now: its host, or the page's
// own when no address was set.
const serverHost = computed(() => {
  try {
    return new URL(serverBase() || location.origin).host
  } catch {
    return location.host
  }
})

// Language toggle. With two locales a cycle button beats a picker, and the label
// is the current language's own tag ("RU"/"EN") — on two locales that says both
// what you're on and what you'd get. setLocale persists to localStorage and only
// PUTs prefs when signed in, so pressing it here (no token) is a purely local
// switch.
const activeLocale = computed(() => normalizeLocale(theme.language))
const nextLocale = computed(() => {
  const i = SUPPORTED_LOCALES.indexOf(activeLocale.value)
  return SUPPORTED_LOCALES[(i + 1) % SUPPORTED_LOCALES.length]
})
function cycleLocale() {
  theme.setLocale({ language: nextLocale.value })
}

// The bottom-right line: which build is this, and which server is it.
const version = typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : ''
const host = typeof location !== 'undefined' ? location.host : ''

// The focus goes to the first empty field (PackageE: «Фокус — в первое пустое
// поле»), so the remembered e-mail lands the reader in the password. Not on a
// touch screen: a keyboard sliding over the card the moment it appears hides the
// very form it opened.
const body = ref(null)
onMounted(async () => {
  if (window.matchMedia?.('(pointer: coarse)')?.matches) return
  await nextTick()
  const fields = [...(body.value?.querySelectorAll('input:not([type="hidden"])') || [])]
  fields.find((el) => !el.value && !el.disabled)?.focus()
})

// Naive's own sizes for the slotted form: 40px fields with the 10px control
// radius, a 42px submit (size="large"), labels in the small caption weight. The
// colours stay the app's — this provider only merges these keys over them.
const overrides = {
  Input: { heightMedium: '40px', borderRadius: '10px', fontSizeMedium: '14px' },
  Button: { heightLarge: '42px', borderRadiusLarge: '10px', fontSizeLarge: '14px' },
  Form: { labelFontSizeTopMedium: '12px', labelPaddingVertical: '0 0 6px 2px' },
}
</script>

<template>
  <div class="auth">
    <!-- The element is the root, the provider inside it: the app's route
         <Transition mode="out-in"> needs one element to animate, and an abstract
         provider at the root left it a fragment — the leave never finished and
         the next screen never mounted. No comment above the root either: in a
         dev build it is a node of its own. -->
    <n-config-provider :theme-overrides="overrides" abstract>
      <HomeAurora frozen class="auth-aurora" />

      <main class="auth-stage">
        <div class="auth-card">
          <header class="auth-card-head">
            <BrandLogo class="auth-logo" :height="22" :mark="false" />
            <!-- Desktop only: the server this client signs in to. -->
            <n-popover
              v-if="desktop"
              v-model:show="showServer"
              trigger="click"
              placement="bottom-end"
            >
              <template #trigger>
                <button
                  class="auth-server"
                  type="button"
                  :title="t('shell.auth.server')"
                  :aria-label="t('shell.auth.server')"
                  data-testid="auth-server"
                >
                  <n-icon :component="ServerOutline" :size="13" />
                  <span class="auth-server-host">{{ serverHost }}</span>
                  <n-icon :component="ChevronDownOutline" :size="12" />
                </button>
              </template>
              <div class="auth-server-pop">
                <div class="auth-server-label">{{ t('shell.auth.server') }}</div>
                <n-input
                  v-model:value="serverInput"
                  :placeholder="t('shell.auth.serverPlaceholder')"
                  size="small"
                  @keyup.enter="saveServer"
                />
                <div v-if="serverError" class="auth-server-error">{{ serverError }}</div>
                <n-button
                  size="small"
                  type="primary"
                  block
                  :disabled="!!serverError"
                  @click="saveServer"
                >
                  {{ t('common.action.save') }}
                </n-button>
              </div>
            </n-popover>
          </header>

          <span v-if="icon" class="auth-badge" :style="badgeStyle" data-testid="auth-badge">
            <n-icon :component="icon" :size="22" />
          </span>
          <h1 v-if="title" class="auth-title">{{ title }}</h1>
          <p v-if="subtitle" class="auth-sub">{{ subtitle }}</p>

          <div ref="body" class="auth-body">
            <slot />
          </div>
        </div>
      </main>

      <footer class="auth-bar">
        <div class="auth-tools">
          <button
            class="auth-tool"
            type="button"
            data-testid="auth-lang-toggle"
            :title="t('shell.auth.languageSwitchTo', { lang: t(`common.language.${nextLocale}`) })"
            :aria-label="
              t('shell.auth.languageSwitchTo', { lang: t(`common.language.${nextLocale}`) })
            "
            @click="cycleLocale"
          >
            <n-icon :component="LanguageOutline" :size="14" class="auth-tool-ic" />{{
              activeLocale.toUpperCase()
            }}
          </button>
          <button
            class="auth-tool"
            type="button"
            data-testid="auth-theme-toggle"
            :aria-label="theme.isDark ? t('shell.auth.themeLight') : t('shell.auth.themeDark')"
            @click="theme.toggle()"
          >
            <n-icon :component="theme.isDark ? SunnyOutline : MoonOutline" :size="14" />
            <span>{{ t('shell.auth.theme') }}</span>
          </button>
          <!-- Web only: the app download (the desktop client updates itself). -->
          <DownloadAppButton v-if="!desktop" class="auth-dl" />
        </div>
        <span class="auth-build">{{ t('shell.auth.build', { version, host }) }}</span>
      </footer>
    </n-config-provider>
  </div>
</template>

<style scoped>
.auth {
  position: relative;
  overflow: hidden;
  min-height: 100vh;
  min-height: 100dvh;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  background: var(--t-bg);
}
/* The aurora band reaches the page's edges. */
.auth-aurora {
  z-index: 0;
}

.auth-stage {
  position: relative;
  z-index: 1;
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 72px 24px 24px;
  animation: auth-rise 0.5s cubic-bezier(0.22, 1, 0.36, 1) both;
}
@media (prefers-reduced-motion: reduce) {
  .auth-stage {
    animation: none;
  }
}
@keyframes auth-rise {
  from {
    opacity: 0;
    transform: translateY(12px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* The glass card: the popover level at 92% over a blur, so the aurora reads
   under its top edge (PackageE «Отступления»). No outline — a surface and a
   shadow. */
.auth-card {
  width: 100%;
  max-width: 420px;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  padding: 28px;
  border-radius: 18px;
  background: color-mix(in srgb, var(--t-pop) 92%, transparent);
  -webkit-backdrop-filter: blur(20px);
  backdrop-filter: blur(20px);
  box-shadow: var(--t-sh-pop);
}
.auth-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 28px;
  margin-bottom: 22px;
}
.auth-logo {
  flex: none;
}
.auth-server {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 60%;
  height: 28px;
  padding: 0 10px;
  border: 0;
  border-radius: 999px;
  background: var(--t-ctl);
  color: var(--t-text2);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.auth-server:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.auth-server-host {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.auth-server-pop {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 260px;
}
.auth-server-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
}
.auth-server-error {
  font-size: 12px;
  color: #e0533d;
}

/* A state screen's badge: 44px, the control radius doubled, the hue's soft fill. */
.auth-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  margin-bottom: 18px;
  border-radius: 12px;
}
.auth-title {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
  line-height: 1.2;
  letter-spacing: -0.01em;
  color: var(--t-text1);
}
.auth-sub {
  margin: 8px 0 0;
  font-size: 13.5px;
  line-height: 1.5;
  color: var(--t-text2);
}
.auth-body {
  margin-top: 20px;
}
.auth-title:last-child,
.auth-sub:last-child {
  margin-bottom: 0;
}

/* The page's bar along the bottom: language, theme, download | build · host. */
.auth-bar {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  padding: 0 28px 20px;
}
.auth-tools {
  display: flex;
  align-items: center;
  gap: 8px;
}
.auth-tool {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  background: var(--t-ctl);
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  font-weight: 500;
  cursor: pointer;
}
.auth-tool:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.auth-tool:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.auth-tool-ic {
  margin-right: 0;
}
.auth-build {
  font-size: 12px;
  color: var(--t-text3);
}

/* Phone: the card takes the width, the bar wraps under it. */
@media (max-width: 600px) {
  .auth-stage {
    align-items: flex-start;
    padding: 56px 14px 18px;
  }
  .auth-card {
    padding: 22px 20px;
  }
  .auth-bar {
    justify-content: center;
    padding: 0 14px 18px;
  }
  .auth-build {
    width: 100%;
    text-align: center;
  }
}
</style>
