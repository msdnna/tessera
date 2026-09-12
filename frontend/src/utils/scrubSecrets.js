// Redaction of secret-bearing URL parameters, for anything that leaves the
// browser as telemetry.
//
// The links Tessera mails out carry their credential in the query string —
// /recover?token=…, /verify-email?token=…, /invite?token=… — and the OAuth
// handoff carries one in the fragment (#access_token=…). Sentry's browser SDK
// records the full location on every event, every navigation breadcrumb and
// every pageload/navigation transaction, so without this a password-recovery
// token (i.e. an account takeover) would be sitting in the Sentry project.
//
// Mirrors backend/internal/observability/redact.go — keep the two lists in step.

const SENSITIVE_PARAMS = new Set([
  'code',
  'state',
  'token',
  'secret',
  'sig',
  'access_token',
  'refresh_token',
])

// redactPairs blanks the value of every sensitive parameter in an `a=1&b=2`
// string. Unrelated parameters keep their value and their position, so the
// telemetry stays useful for debugging routing.
function redactPairs(pairs) {
  return pairs
    .split('&')
    .map((pair) => {
      const eq = pair.indexOf('=')
      const key = eq === -1 ? pair : pair.slice(0, eq)
      if (!SENSITIVE_PARAMS.has(key)) return pair
      return eq === -1 ? key : `${key}=***`
    })
    .join('&')
}

// scrubUrl redacts the query string and the fragment of a URL. Works on
// absolute URLs, root-relative paths and bare query strings alike; anything
// that isn't a string comes back untouched.
export function scrubUrl(value) {
  if (typeof value !== 'string' || value === '') return value

  let base = value
  let hash = ''
  const h = base.indexOf('#')
  if (h !== -1) {
    // The fragment is where the OAuth handoff puts access/refresh tokens, so it
    // gets the same treatment as the query — it is `a=b&c=d` shaped too.
    hash = `#${redactPairs(base.slice(h + 1))}`
    base = base.slice(0, h)
  }

  const q = base.indexOf('?')
  if (q !== -1) base = `${base.slice(0, q)}?${redactPairs(base.slice(q + 1))}`

  return base + hash
}

// scrubQueryString is scrubUrl for a value that is *only* a query string (the
// `url.query` span attribute), with or without its leading '?'.
function scrubQueryString(value) {
  if (typeof value !== 'string' || value === '') return value
  return value.startsWith('?') ? `?${redactPairs(value.slice(1))}` : redactPairs(value)
}

// scrubData rewrites the string values of a span/trace `data` bag in place.
function scrubData(data) {
  if (!data || typeof data !== 'object') return
  for (const [key, value] of Object.entries(data)) {
    if (typeof value !== 'string') continue
    data[key] = key.toLowerCase().includes('quer') ? scrubQueryString(value) : scrubUrl(value)
  }
}

// scrubEvent strips secrets from every place the SDK records a URL: the request
// it attaches to errors, the breadcrumb trail, and — for transactions — the
// trace context and each span's attributes.
export function scrubEvent(event) {
  if (!event) return event

  if (event.request) {
    event.request.url = scrubUrl(event.request.url)
    if (event.request.query_string) {
      event.request.query_string = scrubQueryString(event.request.query_string)
    }
  }
  if (Array.isArray(event.breadcrumbs)) event.breadcrumbs.forEach(scrubBreadcrumb)
  scrubData(event.contexts?.trace?.data)
  if (Array.isArray(event.spans)) event.spans.forEach((span) => scrubData(span?.data))

  return event
}

// scrubBreadcrumb cleans a single crumb. Navigation crumbs carry from/to paths
// (query included); fetch/xhr crumbs carry the request url.
export function scrubBreadcrumb(crumb) {
  if (!crumb) return crumb
  const data = crumb.data
  if (data && typeof data === 'object') {
    for (const key of ['from', 'to', 'url']) {
      if (typeof data[key] === 'string') data[key] = scrubUrl(data[key])
    }
  }
  return crumb
}
