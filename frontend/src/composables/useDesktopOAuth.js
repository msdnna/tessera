import { isTauri } from '@/utils/serverBase'

// Desktop GitLab login (#2696). The webview must never navigate to the OAuth provider
// itself — that takes the app off its own frontend with no way back, which was the bug.
// Instead the login button opens the *system* browser (where the user's GitLab session
// and password manager already live, per RFC 8252), the backend callback redirects to
// `tessera://oauth/callback#…`, and the OS wakes the app up with that URL.
//
// This is deliberately NOT part of useDesktopDeepLink.js — that one handles notification
// clicks; the name is historical and the two have nothing in common.

export const OAUTH_DEEP_LINK = 'tessera://oauth/callback'

// Fired on `window` once the handoff has been processed (detail: { error: string|null }).
// App.vue owns the deep link; the login screen only listens so it can stop waiting.
export const OAUTH_DONE_EVENT = 'oauth:desktop-done'

// How long to wait for the browser round-trip before giving up. Generous: the user may
// have to type a password and confirm 2FA. The point isn't to police the clock, it's to
// replace an eternal spinner with a message that names the likely cause — on Linux the
// scheme is registered by the installer, and an AppImage that was moved after first run
// no longer resolves tessera://.
export const OAUTH_WAIT_MS = 120_000

// parseOAuthDeepLink pulls the handoff out of a tessera://oauth/callback URL. Returns
// null for anything that isn't one, so a deep link added later for some other purpose
// doesn't get mistaken for a login. Kept pure and exported for tests.
export function parseOAuthDeepLink(url) {
  if (typeof url !== 'string' || !url.startsWith(OAUTH_DEEP_LINK)) return null
  const hash = url.slice(OAUTH_DEEP_LINK.length)
  if (hash && !hash.startsWith('#')) return null // e.g. tessera://oauth/callback-other
  const frag = new URLSearchParams(hash.replace(/^#/, ''))
  const error = frag.get('oauth_error')
  if (error) return { access: null, refresh: null, error }
  const access = frag.get('access_token')
  if (!access) return { access: null, refresh: null, error: 'no_token' }
  return { access, refresh: frag.get('refresh_token') || null, error: null }
}

// startDesktopGitlabLogin opens the system browser at the authorize endpoint. Returns
// false if the shell refused (plugin missing / ACL), so the caller can show an error
// instead of a spinner that will never resolve.
export async function startDesktopGitlabLogin(authorizeUrl) {
  try {
    const { openUrl } = await import('@tauri-apps/plugin-opener')
    await openUrl(authorizeUrl)
    return true
  } catch {
    return false
  }
}

// listenDesktopOAuth wires the deep-link delivery and calls `onHandoff` with the parsed
// result. Two delivery paths, and missing either one is a real bug:
//   - app already running → the plugin's onOpenUrl event;
//   - app was closed      → the URL is a *startup argument*, readable once via getCurrent().
// Returns an unlisten function (no-op on web / when the plugin is unavailable).
export async function listenDesktopOAuth(onHandoff) {
  if (!isTauri()) return () => {}
  let unlisten = null
  try {
    const { onOpenUrl, getCurrent } = await import('@tauri-apps/plugin-deep-link')
    const handle = (urls) => {
      for (const u of urls || []) {
        const parsed = parseOAuthDeepLink(u)
        if (parsed) onHandoff(parsed)
      }
    }
    // Cold start first: the link that launched the app is already waiting.
    try {
      handle(await getCurrent())
    } catch {
      /* nothing pending */
    }
    unlisten = await onOpenUrl(handle)
  } catch {
    /* plugin unavailable — the login screen falls back to its timeout message */
  }
  return () => {
    if (typeof unlisten === 'function') unlisten()
  }
}
