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
  let browser = 'браузер'
  if (/Edg\//.test(ua)) browser = 'Edge'
  else if (/OPR\/|Opera/.test(ua)) browser = 'Opera'
  else if (/Firefox\//.test(ua)) browser = 'Firefox'
  else if (/Chrome\//.test(ua)) browser = 'Chrome'
  else if (/Safari\//.test(ua)) browser = 'Safari'
  return `Браузер (${browser})`
}

// notificationsSupported reports whether the Web Notifications API is available.
export function notificationsSupported() {
  return typeof window !== 'undefined' && 'Notification' in window
}
