import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression guard for #2922: GitLab OAuth on an HTTPS instance behind a
// TLS-terminating edge (Caddy on :8443 → this nginx → backend) came back to
// http://host:8443 and failed. The cause was nginx overwriting the
// X-Forwarded-Proto the edge had set with its own $scheme, which is 'http' on
// the plain edge→nginx hop, so the backend built an http:// redirect_uri on an
// https site. Neither vite dev nor preview run this config, so a static check
// of the deployed nginx.conf is the only thing that can catch a revert.
const root = process.cwd()
const conf = readFileSync(resolve(root, 'nginx.conf'), 'utf8')
// Strip comments so prose mentioning $scheme can't satisfy (or break) a check.
const live = conf.replace(/#[^\n]*/g, '')

describe('frontend/nginx.conf X-Forwarded-Proto', () => {
  it('derives the forwarded scheme from the edge, falling back to $scheme', () => {
    const map = live.match(/map\s+\$http_x_forwarded_proto\s+\$forwarded_proto\s*\{([\s\S]*?)\}/)
    expect(map, 'no map for $forwarded_proto').not.toBeNull()
    // Passes the upstream value through, and only uses this hop's $scheme when
    // no proxy set one — a single-proxy deploy stays unchanged.
    expect(map[1]).toMatch(/default\s+\$http_x_forwarded_proto\s*;/)
    expect(map[1]).toMatch(/''\s+\$scheme\s*;/)
  })

  it('sends the mapped scheme upstream, never a raw $scheme, on the API proxy', () => {
    const api = live.match(/location\s+\/api\/\s*\{([\s\S]*?)\n {4}\}/)
    expect(api, 'no /api/ location block').not.toBeNull()
    expect(api[1]).toMatch(/proxy_set_header\s+X-Forwarded-Proto\s+\$forwarded_proto\s*;/)
    expect(api[1]).not.toMatch(/proxy_set_header\s+X-Forwarded-Proto\s+\$scheme\s*;/)
  })
})
