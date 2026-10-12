import { ref } from 'vue'
import { notificationPrefs } from '@/api'

// The reader's due-notification defaults (Настройки → Уведомления), for the due
// picker's «По умолчанию — за час» (#3118). Read once per session, on the first
// picker that asks; a failed read keeps the server's own defaults (за час,
// однократно — utils/dueModel `effectiveNotify`).
const prefs = ref(null)
let asked = false

export function useNotifyDefaults() {
  if (!asked) {
    asked = true
    Promise.resolve()
      .then(() => notificationPrefs?.get?.())
      .then((r) => {
        if (r?.data) prefs.value = r.data
      })
      .catch(() => {})
  }
  return prefs
}

// Settings saved new ones: hand them over (or forget, to re-read next time).
export function setNotifyDefaults(data) {
  prefs.value = data || null
  asked = !!data
}
