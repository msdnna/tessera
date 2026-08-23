import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { NMessageProvider } from 'naive-ui'
import { i18n, setI18nLocale } from '@/i18n'
import GitLabModal from '@/components/GitLabModal.vue'

// Wave 7 of #2799 — the GitLab integration modal, the single largest screen of
// the extraction. Two things are worth pinning here and nowhere else:
//
//  1. The modal is almost entirely built out of option tables (triggers, actions,
//     intervals, sources, label rules). Every one of them used to be a module
//     constant, evaluated once on import — the exact shape that survives a naive
//     extraction and then freezes on the language of the first render. The tests
//     below switch the locale on a mounted modal and demand the labels follow.
//  2. Two messages carry characters that vue-i18n reads as syntax: `@` starts a
//     linked message and `|` separates plural forms. Both are escaped in the
//     bundles, and an unescaped one fails loudly rather than in production.

vi.mock('@/api', () => {
  const ok = (data) => Promise.resolve({ data })
  return {
    gitlab: {
      getConnection: () => ok({ connected: false }),
      listIntegrations: () =>
        ok({ integrations: [], default_rules: {}, service_configured: false, is_admin: true }),
      conflicts: () => ok([]),
    },
    projects: { boards: () => ok([]), tagPrefixes: () => ok([]) },
    boards: { columns: () => ok([]) },
  }
})

// The modal calls useMessage(), which only resolves inside a message provider.
function mountModal() {
  return mount(
    {
      render: () =>
        h(NMessageProvider, null, {
          default: () => h(GitLabModal, { show: true, wsId: 'ws-1' }),
        }),
    },
    { attachTo: document.body },
  )
}

// n-modal teleports its body to <body>, so the rendered text is read there
// rather than off the wrapper.
const bodyText = () => document.body.textContent

describe('GitLab modal follows a language switch', () => {
  beforeEach(() => setActivePinia(createPinia()))

  afterEach(async () => {
    await setI18nLocale('ru')
    document.body.innerHTML = ''
  })

  it('re-renders its headings and hints', async () => {
    const wrapper = mountModal()
    await nextTick()
    await nextTick()
    expect(bodyText()).toContain('Аккаунт')
    expect(bodyText()).toContain('Обратная запись в GitLab')

    await setI18nLocale('en')
    await nextTick()
    expect(bodyText()).toContain('Account')
    expect(bodyText()).toContain('Write-back to GitLab')
    expect(bodyText()).not.toContain('Обратная запись в GitLab')
    wrapper.unmount()
  })

  // The regression this wave's traps point at: a select's options are built once
  // and keep the old language while the labels around them change.
  it('re-renders the option tables, not just the labels around them', async () => {
    const wrapper = mountModal()
    await nextTick()
    await nextTick()
    // Auto-sync interval: selected value 0 → "Вручную (выкл.)" is on screen.
    expect(bodyText()).toContain('Вручную (выкл.)')

    await setI18nLocale('en')
    await nextTick()
    expect(bodyText()).toContain('Manually (off)')
    expect(bodyText()).not.toContain('Вручную (выкл.)')
    wrapper.unmount()
  })
})

describe('messages with vue-i18n syntax characters', () => {
  const t = (key, args) => i18n.global.t(key, args)

  // `@` opens a linked message (@:key / @{expr}); the bundles escape it as {'@'}.
  it('renders the login prefix as a literal @', () => {
    expect(t('gitlab.modal.connectedAs', { name: 'octocat' })).toBe('GitLab подключён как @octocat')
    i18n.global.locale.value = 'en'
    expect(t('gitlab.modal.connectedAs', { name: 'octocat' })).toBe('GitLab connected as @octocat')
    i18n.global.locale.value = 'ru'
  })

  // `|` separates plural forms: unescaped, the regex example in the rule-match
  // placeholder would silently collapse to "S: либо ^(T".
  it('keeps the pipe inside the rule-match example', () => {
    expect(t('gitlab.modal.rules.matchPlaceholder')).toBe('S: либо ^(T|C): ')
    i18n.global.locale.value = 'en'
    expect(t('gitlab.modal.rules.matchPlaceholder')).toBe('S: or ^(T|C): ')
    i18n.global.locale.value = 'ru'
  })

  // The grouped-task placeholder mirrors gitlab.DefaultGroupLabel on the server:
  // it is a label written into GitLab, so it reads the same in both locales.
  it('spells the backend default group label identically in both locales', () => {
    const ru = t('gitlab.modal.writeback.groupLabelPlaceholder')
    i18n.global.locale.value = 'en'
    expect(t('gitlab.modal.writeback.groupLabelPlaceholder')).toBe(ru)
    i18n.global.locale.value = 'ru'
    expect(ru).toBe('M: Сгруппированная задача')
  })
})
