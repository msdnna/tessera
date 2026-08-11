import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createRouter, createMemoryHistory } from 'vue-router'

// The real router (src/router/index.js) uses createWebHistory + lazy view imports
// that pull the whole app tree under jsdom. To exercise the *guard* in isolation we
// rebuild an equivalent memory router with the same route metas and the same guard
// logic, then assert the redirect decisions. A separate module-import smoke below
// confirms the real module loads and exports a router.

const Stub = { template: '<div />' }

// Route table mirrors src/router/index.js metas (the guard only reads meta + path).
const routes = [
  { path: '/login', component: Stub, meta: { public: true } },
  { path: '/register', component: Stub, meta: { public: true } },
  { path: '/forgot-password', component: Stub, meta: { open: true } },
  { path: '/recover', alias: '/reset-password', component: Stub, meta: { open: true } },
  { path: '/verify-email', component: Stub, meta: { open: true } },
  { path: '/invite', component: Stub },
  { path: '/oauth/callback', component: Stub, meta: { open: true } },
  { path: '/', component: Stub },
  { path: '/notes', component: Stub },
  { path: '/admin', component: Stub, meta: { admin: true } },
  { path: '/:pathMatch(.*)*', component: Stub, meta: { open: true } },
]

// Stand-in for the auth store the real guard reads. Since #2684 the access token
// lives in memory, so "signed in" is store state, not a localStorage key.
const session = { token: '', user: null }

// The guard, copied verbatim from src/router/index.js — kept in sync by test
// intent, with `auth` resolved from the stand-in above instead of useAuthStore().
function guard(to) {
  const auth = { isAuthenticated: !!session.token, isAdmin: !!session.user?.is_admin }
  if (to.meta.open) return
  if (!to.meta.public && !auth.isAuthenticated)
    return { path: '/login', query: { next: to.fullPath } }
  if (to.meta.public && auth.isAuthenticated) return { path: '/' }
  if (to.meta.admin && !auth.isAdmin) return { path: '/' }
}

function makeRouter() {
  const r = createRouter({ history: createMemoryHistory(), routes })
  r.beforeEach(guard)
  return r
}

describe('router guard', () => {
  beforeEach(() => {
    localStorage.clear()
    session.token = ''
    session.user = null
  })

  it('redirects an unauthenticated user from a protected route to /login with next', async () => {
    const r = makeRouter()
    await r.push('/notes')
    expect(r.currentRoute.value.path).toBe('/login')
    expect(r.currentRoute.value.query.next).toBe('/notes')
  })

  it('lets an authenticated user reach a protected route', async () => {
    session.token = 'tok'
    const r = makeRouter()
    await r.push('/notes')
    expect(r.currentRoute.value.path).toBe('/notes')
  })

  it('bounces an authenticated user away from a public (login) page to /', async () => {
    session.token = 'tok'
    const r = makeRouter()
    await r.push('/login')
    expect(r.currentRoute.value.path).toBe('/')
  })

  it('always allows open email-link routes regardless of auth', async () => {
    const r = makeRouter()
    await r.push('/verify-email')
    expect(r.currentRoute.value.path).toBe('/verify-email')
    await r.push('/reset-password') // alias of /recover, open
    expect(r.currentRoute.value.meta.open).toBe(true)
  })

  it('routes an unknown path to the open catch-all (404) instead of bouncing to login', async () => {
    const r = makeRouter()
    await r.push('/this/does/not/exist')
    // The catch-all matched (meta.open) — no /login redirect.
    expect(r.currentRoute.value.path).not.toBe('/login')
    expect(r.currentRoute.value.matched.at(-1)?.path).toBe('/:pathMatch(.*)*')
  })

  it('keeps a non-admin authenticated user off /admin', async () => {
    session.token = 'tok'
    session.user = { is_admin: false }
    const r = makeRouter()
    await r.push('/admin')
    expect(r.currentRoute.value.path).toBe('/')
  })

  it('lets an admin reach /admin', async () => {
    session.token = 'tok'
    session.user = { is_admin: true }
    const r = makeRouter()
    await r.push('/admin')
    expect(r.currentRoute.value.path).toBe('/admin')
  })
})

// Module smoke: the real router file imports and exposes a configured router. Guard
// is registered but not exercised (no navigation), so no heavy view chunks load.
describe('router module', () => {
  beforeEach(() => vi.resetModules())

  it('exports a router instance with the expected named routes', async () => {
    const mod = await import('@/router/index.js')
    const router = mod.default
    expect(router).toBeTruthy()
    expect(typeof router.beforeEach).toBe('function')
    const paths = router.getRoutes().map((r) => r.path)
    expect(paths).toContain('/login')
    expect(paths).toContain('/admin')
    expect(paths.some((p) => p.includes('pathMatch'))).toBe(true)
  })
})
