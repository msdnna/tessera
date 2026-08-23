// Turn backend / network error strings into friendly, localized text for display.
// The backend surfaces two ugly shapes we must tame before showing a user:
//   • gin/validator binding errors, e.g.
//     "Key: 'Email' Error:Field validation for 'Email' failed on the 'email' tag"
//   • bare English sentinels, e.g. "invalid credentials".
// Anything we recognise is mapped; short unknown messages pass through; clearly
// internal noise falls back to a generic line. Applied centrally in the axios
// response interceptor so every `message.error(e.message)` caller benefits.
//
// This runs outside a setup context (the interceptor is plain module code), so
// the catalog is reached through `i18n.global.t` rather than useI18n(). The
// lookup happens per call, not at import time — an error raised after the user
// switched language must be phrased in the new one (#2799).
import { i18n } from '@/i18n'

// The keys are the wire format — exactly what the backend sends — so they stay
// English; only the catalog key on the right-hand side is ours to translate.
const SENTINELS = {
  'invalid credentials': 'invalidCredentials',
  'invalid email or password': 'invalidCredentials',
  'email already registered': 'emailRegistered',
  'email already exists': 'emailRegistered',
  'email already in use': 'emailInUse',
  'user already exists': 'userExists',
  'user not found': 'userNotFound',
  'invalid or expired token': 'linkInvalid',
  'invalid token': 'linkInvalid',
  'token expired': 'linkExpired',
  'account is deactivated': 'accountDeactivated',
  'account deactivated': 'accountDeactivated',
  forbidden: 'forbidden',
  unauthorized: 'unauthorized',
  'not found': 'notFound',
}

function t(key, params) {
  return i18n.global.t(`errors.${key}`, params)
}

// Collapse a gin/validator error ("...failed on the 'X' tag") into one friendly
// sentence based on which validation tags failed.
function fromValidator(raw) {
  const tags = [...raw.matchAll(/failed on the '(\w+)' tag/g)].map((m) => m[1])
  if (!tags.length) return null
  if (tags.includes('email')) return t('validation.email')
  if (tags.every((tag) => tag === 'required')) return t('validation.required')
  if (tags.includes('min')) return t('validation.tooShort')
  if (tags.includes('max')) return t('validation.tooLong')
  return t('validation.generic')
}

export function humanizeError(raw) {
  if (!raw) return t('generic')
  const msg = String(raw).trim()
  const low = msg.toLowerCase()
  // hasOwn, not a bare lookup: `low` comes from the server, and "constructor"
  // would otherwise hit Object.prototype and be mapped as if it were a sentinel.
  if (Object.hasOwn(SENTINELS, low)) return t(`sentinel.${SENTINELS[low]}`)
  if (low.startsWith('key:') && low.includes('validation')) {
    return fromValidator(msg) || t('validation.generic')
  }
  if (
    low.includes('network error') ||
    low.includes('timeout') ||
    low.includes('econnrefused') ||
    low.includes('failed to fetch')
  ) {
    return t('network')
  }
  return msg
}
