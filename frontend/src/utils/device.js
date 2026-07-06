import { isTauri } from '@/utils/serverBase'

// Stable per-browser identity for the "device" notification channel. The id is
// generated once and kept in localStorage so this browser is a routable device
// (rules can target it specifically); the label is a friendly name for the list.
const KEY = 'tessera_device_id'

export function getDeviceId() {
  let id = localStorage.getItem(KEY)
  if (!id) {
    id =
      (typeof crypto !== 'undefined' && crypto.randomUUID && crypto.randomUUID()) ||
      `web-${Date.now()}-${Math.random().toString(36).slice(2)}`
    localStorage.setItem(KEY, id)
  }
  return id
}

export function deviceLabel() {
  const ua = navigator.userAgent || ''
  if (isTauri()) {
    // Desktop app: label by OS rather than by browser engine.
    let os = 'десктоп'
    if (/Windows/i.test(ua)) os = 'Windows'
    else if (/Mac OS X|Macintosh/i.test(ua)) os = 'macOS'
    else if (/Linux/i.test(ua)) os = 'Linux'
    return `Настольное приложение (${os})`
  }
  let browser = 'браузер'
  if (/Edg\//.test(ua)) browser = 'Edge'
  else if (/OPR\/|Opera/.test(ua)) browser = 'Opera'
  else if (/Firefox\//.test(ua)) browser = 'Firefox'
  else if (/Chrome\//.test(ua)) browser = 'Chrome'
  else if (/Safari\//.test(ua)) browser = 'Safari'
  return `Браузер (${browser})`
}

// notificationsSupported reports whether native notifications can be raised. In
// the desktop app the Tauri notification plugin provides them even when the
// WebKitGTK webview lacks `window.Notification`.
export function notificationsSupported() {
  if (isTauri()) return true
  return typeof window !== 'undefined' && 'Notification' in window
}
