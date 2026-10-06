import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// The app's own origin serves the SPA shell, and until #2932 it served it with
// no security headers at all: the hardening in the codebase (CSP + nosniff on
// /api/uploads and the signed document assets, see backend/handlers/
// attachments.go and document_assets.go) covers only the file routes the
// backend answers, and nothing sat in front of them — deploy/Caddyfile is a
// bare reverse_proxy, so whatever nginx emits is what the browser sees.
//
// Neither vite dev nor preview runs this config, so a static check of the
// deployed nginx.conf is the only thing that can catch a revert — same reason
// nginxForwardedProto.spec.js exists next door.
const root = process.cwd()
const conf = readFileSync(resolve(root, 'nginx.conf'), 'utf8')
const headers = readFileSync(resolve(root, 'security-headers.conf'), 'utf8')
// Strip comments so prose mentioning a header can't satisfy a check.
const live = conf.replace(/#[^\n]*/g, '')
const liveHeaders = headers.replace(/#[^\n]*/g, '')

// The shared snippet, as nginx.conf includes it.
const INCLUDE = 'include /etc/nginx/security-headers.conf;'

/** Every `location … { … }` block in the server, by its matcher. */
function locations(src) {
  const out = []
  const re = /location\s+([^{]+?)\s*\{([\s\S]*?)\n {4}\}/g
  for (const m of src.matchAll(re)) out.push({ matcher: m[1], body: m[2] })
  return out
}

describe('frontend/nginx.conf security headers', () => {
  it('denies framing, pins the declared type and trims the referrer', () => {
    // frame-ancestors 'none' is the clickjacking guard; X-Frame-Options is the
    // same statement for anything that predates it. The app embeds no iframe of
    // its own (the recorder template at /rec/egress is opened top-level by the
    // egress browser), so DENY costs nothing.
    expect(liveHeaders).toMatch(
      /add_header\s+Content-Security-Policy\s+"frame-ancestors 'none'"\s+always\s*;/,
    )
    expect(liveHeaders).toMatch(/add_header\s+X-Frame-Options\s+"DENY"\s+always\s*;/)
    expect(liveHeaders).toMatch(/add_header\s+X-Content-Type-Options\s+"nosniff"\s+always\s*;/)
    expect(liveHeaders).toMatch(
      /add_header\s+Referrer-Policy\s+"strict-origin-when-cross-origin"\s+always\s*;/,
    )
  })

  // Per-location rather than once at server{}, and this is the part that bites:
  // nginx's add_header is not additive across levels — a location declaring any
  // add_header of its own drops every inherited one. Four locations here set
  // Cache-Control for the PWA shell (#2748), and index.html is exactly the page
  // an attacker wants in a frame, so a single server-level declaration would
  // miss the routes that matter most. Declaring it at server{} as well would
  // not fix that and would additionally stack a second CSP onto /api/uploads,
  // which the backend already answers with its own.
  //
  // The rule is "serves it itself → carries the headers", so the check is
  // keyed on proxy_pass rather than on a hand-maintained list: a new static
  // location added later fails this until it includes the snippet.
  it('includes them in every location the origin serves itself', () => {
    const own = locations(live).filter((l) => !/proxy_pass/.test(l.body))
    expect(own.length, 'no self-served locations parsed').toBeGreaterThan(0)
    const offenders = own.filter((l) => !l.body.includes(INCLUDE)).map((l) => l.matcher)
    expect(offenders, 'these locations serve the origin with no security headers').toEqual([])
  })

  it('leaves the proxied routes to their upstream', () => {
    // /api/ carries the backend's own per-route CSP (default-src 'none';
    // sandbox on uploads and document assets). Adding a second CSP here would
    // stack policies on those responses for no gain, and neither proxy is a
    // framing target — so they deliberately stay out.
    for (const matcher of ['/api/', '/livekit/']) {
      const loc = locations(live).find((l) => l.matcher === matcher)
      expect(loc, `no ${matcher} location`).toBeDefined()
      expect(loc.body).not.toContain(INCLUDE)
    }
  })
})
