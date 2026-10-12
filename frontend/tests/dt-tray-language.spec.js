// The desktop tray menu is drawn by the Rust shell, before the webview boots and
// outside anything vue-i18n can reach — so switching the interface language has
// to be *told* to it. Without that the tray keeps the language the shell started
// in (the OS locale, or the fallback) while the window next to it is in the other
// one (#2932).
//
// The shell side is covered by cargo tests in desktop/src-tauri/src/tray.rs; this
// is the web side: that the language switch reaches the bridge at all, that it is
// a no-op off the desktop, and that a shell which fails to answer never takes the
// language switch down with it.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

const invoke = vi.fn()
vi.mock('@tauri-apps/api/core', () => ({ invoke: (...args) => invoke(...args) }))

let tauri = false
vi.mock('@/utils/serverBase', async (importOriginal) => {
  const mod = await importOriginal()
  return { ...mod, isTauri: () => tauri }
})

const { setI18nLocale } = await import('@/i18n')
const { syncTrayLanguage } = await import('@/utils/desktopTray')

describe('desktop tray language', () => {
  beforeEach(() => {
    invoke.mockReset()
    invoke.mockResolvedValue(undefined)
    tauri = true
  })

  afterEach(async () => {
    tauri = false
    await setI18nLocale('ru')
  })

  it('tells the shell which language the tray should be in', async () => {
    await syncTrayLanguage('en')
    expect(invoke).toHaveBeenCalledWith('set_tray_language', { lang: 'en' })
  })

  it('rides along with the interface language switch', async () => {
    // The bell, the window title and the tray have to agree; nobody remembers to
    // call the bridge by hand from every place that flips the language, so the
    // single entry point for switching does it.
    await setI18nLocale('en')
    expect(invoke).toHaveBeenCalledWith('set_tray_language', { lang: 'en' })
    invoke.mockClear()
    await setI18nLocale('ru')
    expect(invoke).toHaveBeenCalledWith('set_tray_language', { lang: 'ru' })
  })

  it('sends the locale that actually took effect, not the one asked for', async () => {
    // An unsupported language degrades to the fallback in the webview; the tray
    // has to degrade with it instead of being told about a language we do not ship.
    const active = await setI18nLocale('de')
    expect(active).toBe('ru')
    expect(invoke).toHaveBeenCalledWith('set_tray_language', { lang: 'ru' })
  })

  it('stays out of the way on the web', async () => {
    tauri = false
    await setI18nLocale('en')
    await syncTrayLanguage('en')
    expect(invoke).not.toHaveBeenCalled()
  })

  it('never lets a silent shell break the language switch', async () => {
    // An older shell has no such command and answers with an error; the webview
    // still has to finish switching.
    invoke.mockRejectedValue(new Error('command set_tray_language not found'))
    await expect(setI18nLocale('en')).resolves.toBe('en')
  })
})
