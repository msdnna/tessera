import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression guard for #2932. The backend derives the client IP from the
// forwarded chain (gin reads X-Forwarded-For before X-Real-IP) and the auth rate
// limiter keys its buckets on it. This config set X-Real-IP but never
// X-Forwarded-For, so with the shipped trusted-proxy list every request arrived
// as this container's own address: /auth/login, /auth/register and /auth/refresh
// shared one bucket for the whole instance, and ten logins in six seconds
// answered 429 to everybody.
//
// $proxy_add_x_forwarded_for, not $remote_addr: it appends the peer we saw to
// whatever came in, which keeps an outer edge's chain (deploy/Caddyfile) intact
// while still making the last entry one nginx wrote. A bare $http_x_forwarded_for
// would hand a caller its own bucket to choose. Neither vite dev nor preview run
// this file, so a static check is the only thing that catches a revert.
const root = process.cwd()
const conf = readFileSync(resolve(root, 'nginx.conf'), 'utf8')
// Strip comments so prose naming a header can't satisfy (or break) a check.
const live = conf.replace(/#[^\n]*/g, '')

describe('frontend/nginx.conf X-Forwarded-For', () => {
  it('forwards the client chain on the API proxy', () => {
    const api = live.match(/location\s+\/api\/\s*\{([\s\S]*?)\n {4}\}/)
    expect(api, 'no /api/ location block').not.toBeNull()
    expect(api[1]).toMatch(/proxy_set_header\s+X-Forwarded-For\s+\$proxy_add_x_forwarded_for\s*;/)
  })

  it('never passes a client-supplied chain through untouched', () => {
    expect(live).not.toMatch(/proxy_set_header\s+X-Forwarded-For\s+\$http_x_forwarded_for\s*;/)
  })
})
