import { ref } from 'vue'
import { isTauri } from '@/utils/serverBase'

// Desktop self-update via the Tauri updater plugin. All methods are no-ops on
// web (isTauri() === false). The updater checks the signed manifest at
// plugins.updater.endpoints (see desktop/src-tauri/tauri.conf.json).
export function useDesktopUpdate() {
  const isDesktop = isTauri()
  const busy = ref(false)
  // '' | 'checking' | 'available' | 'none' | 'downloading' | 'error'
  const status = ref('')
  const newVersion = ref('')
  const error = ref('')
  let pending = null // the Update handle returned by check()

  // check looks for an update. `auto` suppresses the "you're up to date" state so
  // a silent launch check stays quiet. Returns true when an update is available.
  async function check(auto = false) {
    if (!isDesktop) return false
    busy.value = true
    status.value = 'checking'
    error.value = ''
    try {
      const { check: checkUpdate } = await import('@tauri-apps/plugin-updater')
      pending = await checkUpdate()
      if (pending) {
        newVersion.value = pending.version
        status.value = 'available'
        return true
      }
      status.value = auto ? '' : 'none'
      return false
    } catch (e) {
      status.value = 'error'
      error.value = e?.message || String(e)
      return false
    } finally {
      busy.value = false
    }
  }

  // install downloads + applies the pending update, then relaunches the app.
  async function install() {
    if (!isDesktop || !pending) return
    busy.value = true
    status.value = 'downloading'
    try {
      await pending.downloadAndInstall()
      const { relaunch } = await import('@tauri-apps/plugin-process')
      await relaunch()
    } catch (e) {
      status.value = 'error'
      error.value = e?.message || String(e)
    } finally {
      busy.value = false
    }
  }

  return { isDesktop, busy, status, newVersion, error, check, install }
}
