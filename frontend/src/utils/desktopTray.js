import { isTauri } from '@/utils/serverBase'

// The desktop tray menu is drawn by the Rust shell (desktop/src-tauri/src/tray.rs),
// which starts on the OS locale because that is the only hint it has before the
// webview knows whose session this is. From then on the reader's language is ours
// to report — nothing in the shell can observe a vue-i18n switch.
//
// Best-effort by design: on the web there is no shell, and an older shell has no
// such command. Neither is a reason to take the language switch down with it, so
// every failure is swallowed — the tray simply keeps the wording it had.
export async function syncTrayLanguage(locale) {
  if (!isTauri()) return
  try {
    const { invoke } = await import('@tauri-apps/api/core')
    await invoke('set_tray_language', { lang: locale })
  } catch {
    /* no shell, or a shell that predates the command */
  }
}
