import * as Sentry from '@sentry/vue'
import { apiBaseURL, absolutizeApiUrl } from './serverBase'

// Browser error + performance telemetry (#2787), driven entirely by the
// backend's runtime config (GET /api/client-config) — nothing is baked into the
// bundle. If the server reports no DSN (telemetry disabled, or config
// unreachable) Sentry is never initialised and the app starts normally, so this
// module is safe to ship before the self-hosted Sentry exists.
//
// Events go through the tunnel the backend advertises (`sentry.tunnel`), never
// straight to the DSN host: the self-hosted Sentry is LAN-only, and ad-blockers
// drop requests to hosts with "sentry" in the name. The browser only ever talks
// to the API it is already talking to.

// How long to wait for the config before giving up and starting without
// telemetry. The app must never be held hostage by a slow/absent backend.
const CONFIG_TIMEOUT_MS = 3000

// fetchSentryConfig returns the `sentry` block of /api/client-config, or null
// when telemetry is off or the config can't be reached.
async function fetchSentryConfig() {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), CONFIG_TIMEOUT_MS)
  try {
    // Deliberately plain fetch on apiBaseURL() rather than the axios client:
    // this runs before/alongside bootstrap(), and the api instance carries
    // auth-refresh and connection-overlay interceptors that a telemetry probe
    // has no business triggering. apiBaseURL() (not a hardcoded '/api') because
    // the Tauri desktop build talks to the backend cross-origin.
    const res = await fetch(`${apiBaseURL()}/client-config`, {
      headers: { Accept: 'application/json' },
      signal: controller.signal,
    })
    if (!res.ok) return null
    const body = await res.json()
    return body?.sentry || null
  } catch {
    return null // unreachable/aborted — start the app without Sentry
  } finally {
    clearTimeout(timer)
  }
}

// initSentry initialises the SDK if the backend hands out a DSN. Resolves to
// true when Sentry was started, false otherwise. Never rejects: telemetry
// failing to start is not an application error.
export async function initSentry(app, router) {
  const sentry = await fetchSentryConfig()
  if (!sentry?.dsn) return false

  Sentry.init({
    app,
    dsn: sentry.dsn,
    environment: sentry.environment,
    release: `tessera-web@${typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : '0.0.0'}`,
    // The relay path from the backend. absolutizeApiUrl() is a no-op on web
    // (same-origin '/api/sentry-tunnel') and prefixes the configured server
    // origin on desktop, where a root-relative URL would hit the Tauri custom
    // protocol instead of the API.
    tunnel: absolutizeApiUrl(sentry.tunnel) || undefined,
    tracesSampleRate: sentry.tracesSampleRate ?? 0.1,
    integrations: [Sentry.browserTracingIntegration({ router })],
    // Noise from browser internals / extensions that isn't our bug. The
    // ResizeObserver pair fires from layout-observing UI (the board and the
    // document editor both use it) and is benign by spec.
    ignoreErrors: [
      'ResizeObserver loop limit exceeded',
      'ResizeObserver loop completed with undelivered notifications.',
    ],
  })
  return true
}
