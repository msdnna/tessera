import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import { setI18nLocale } from '@/i18n'
import { GET_STARTED } from '@/data/getStarted'
import { roleLabel } from '@/utils/mentions'
import { deviceLabel } from '@/utils/device'
import { COLOR_THEMES } from '@/stores/theme'
import DownloadMenu from '@/components/DownloadMenu.vue'
import LoaderOverlay from '@/components/LoaderOverlay.vue'
import TourOverlay from '@/components/TourOverlay.vue'
import { useTourStore } from '@/stores/tour'

// Wave 10 of #2799 — the last of the interface: the onboarding guide, the
// app-download menu, the update/connection/version chrome and the two shared
// widgets (tags, icon picker).
//
// The guide is the sharpest case in the wave. Its store *snapshots* the scenario
// when the guide starts, so any wording carried on a step would keep the
// language it started in for the whole run — and a run lasts as long as someone
// takes to walk it. The text is therefore resolved by id, per render.

afterEach(async () => {
  await setI18nLocale('ru')
})

describe('the guide', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('re-renders the running step after a switch', async () => {
    // jsdom lays nothing out and the overlay reads a 0×0 anchor as "not on
    // screen yet", so the anchor needs a hand-made box (same trick as
    // cx-tour-overlay.spec.js). The popover is teleported to <body>.
    const el = document.createElement('button')
    el.setAttribute('data-tour', 'ws-switch')
    el.getBoundingClientRect = () => ({ left: 100, top: 60, width: 40, height: 24, right: 140, bottom: 84 })
    document.body.appendChild(el)

    useTourStore().start([{ id: 'workspaces', anchor: 'ws-switch', mode: 'info' }])
    const w = mount(TourOverlay)
    await nextTick()
    await nextTick()

    const pop = () => document.querySelector('[data-testid="tour-pop"]').textContent
    expect(pop()).toContain('Пространства')
    expect(pop()).toContain('Пропустить')

    await setI18nLocale('en')
    await nextTick()
    expect(pop()).toContain('Workspaces')
    expect(pop()).not.toContain('Пространства')
    expect(pop()).toContain('Skip')

    w.unmount()
    document.body.innerHTML = ''
  })

  it('carries no wording on the scenario itself', () => {
    // The regression this wave fixes: text on the step object survives the
    // store's snapshot and freezes. Anchors and modes are what belongs here.
    for (const s of GET_STARTED) {
      expect(s.title, s.id).toBeUndefined()
      expect(s.body, s.id).toBeUndefined()
    }
  })
})

describe('the download menu', () => {
  const groups = [
    {
      key: 'linux',
      name: 'Linux',
      icon: {},
      version: '1.0.0',
      single: false,
      variants: [
        { format: 'appimage', url: '/a', recommended: true },
        { format: 'exe', url: '/b' },
        // A format no catalogue knows — a newer manifest naming a build this
        // client has never heard of must not print a missing key at the user.
        { format: 'snap', url: '/c' },
      ],
    },
  ]

  it('names formats from the catalogue, not from the manifest', async () => {
    const w = mount(DownloadMenu, { props: { groups } })
    expect(w.text()).toContain('Скачать приложение')
    expect(w.text()).toContain('Установщик (.exe)')
    expect(w.text()).toContain('рекоменд.')
    expect(w.text()).toContain('snap')

    await setI18nLocale('en')
    expect(w.text()).toContain('Download the app')
    expect(w.text()).toContain('Installer (.exe)')
    expect(w.text()).toContain('recommended')
    // Product names are the same in both locales; the wrapper around them is not.
    expect(w.text()).toContain('AppImage')
    expect(w.text()).toContain('snap')
    w.unmount()
  })
})

describe('the loading overlay', () => {
  it('takes its default caption from the catalogue', async () => {
    // Two callers pass no captions at all. A Russian default on the prop would
    // never be empty, so no fallback inside the component could ever fire — the
    // trap that left ConfirmByName and the document editor untranslatable.
    const w = mount(LoaderOverlay, { props: { show: true } })
    expect(w.text()).toContain('Загружаем…')

    await setI18nLocale('en')
    expect(w.text()).toContain('Loading…')
    w.unmount()
  })

  it('still prefers captions the caller passes', () => {
    const w = mount(LoaderOverlay, { props: { show: true, messages: ['Синхронизируем…'] } })
    expect(w.text()).toContain('Синхронизируем…')
    w.unmount()
  })
})

describe('accent schemes', () => {
  it('keep the palette as a token and the name in the catalogue', async () => {
    for (const ct of COLOR_THEMES) {
      expect(ct.name, ct.key).toBeUndefined()
      expect(ct.primary, ct.key).toMatch(/^#/)
    }
  })
})

describe('labels resolved outside a setup context', () => {
  it('translates a member role per call', async () => {
    expect(roleLabel('owner')).toBe('Владелец')
    await setI18nLocale('en')
    expect(roleLabel('owner')).toBe('Owner')
    // An unknown role still shows the raw code rather than an invented one.
    expect(roleLabel('guest')).toBe('guest')
    expect(roleLabel(null)).toBe('')
  })

  it('names this browser in the current language', async () => {
    vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue('Mozilla/5.0 Firefox/128.0')
    expect(deviceLabel()).toBe('Браузер (Firefox)')
    await setI18nLocale('en')
    expect(deviceLabel()).toBe('Browser (Firefox)')
    vi.restoreAllMocks()
  })
})
