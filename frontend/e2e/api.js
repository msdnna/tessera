// Thin API client for e2e seeding. Everything a spec needs *as a precondition*
// is created here over HTTP rather than by clicking through the UI: seeding via
// the UI makes every spec depend on every other spec's screens, so one broken
// board screen would fail the auth spec too.
//
// Talks to the backend directly (E2E_API_URL, default :8092), NOT through the
// Vite preview proxy — the proxy only exists for the browser.

const BASE = process.env.E2E_API_URL || 'http://localhost:8092/api'

async function call(method, path, { token, body } = {}) {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  })
  const text = await res.text()
  if (!res.ok) {
    throw new Error(`${method} ${path} → ${res.status}: ${text.slice(0, 300)}`)
  }
  return text ? JSON.parse(text) : null
}

export const api = {
  get: (p, token) => call('GET', p, { token }),
  post: (p, body, token) => call('POST', p, { token, body }),
  put: (p, body, token) => call('PUT', p, { token, body }),
  patch: (p, body, token) => call('PATCH', p, { token, body }),
  put: (p, body, token) => call('PUT', p, { token, body }),
  del: (p, token) => call('DELETE', p, { token }),
}

// waitForBackend polls /health so the suite fails with "backend never came up"
// instead of a wall of confusing 404s from individual specs.
export async function waitForBackend(timeoutMs = 60000) {
  const deadline = Date.now() + timeoutMs
  let last = ''
  while (Date.now() < deadline) {
    try {
      const h = await api.get('/health')
      if (h?.ok) return
      last = JSON.stringify(h)
    } catch (e) {
      last = e.message
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  throw new Error(`backend at ${BASE} not healthy within ${timeoutMs}ms; last: ${last}`)
}

// A fresh user per invocation: each run gets its own workspace and its own data,
// so parallel workers never collide and `tessera_test` needs no cleanup between
// runs. `runId` is passed in (not generated here) so global-setup and the
// fixtures agree on which user a run belongs to.
export function newCredentials(runId, suffix = '') {
  return {
    email: `e2e+${runId}${suffix}@test.local`,
    name: `E2E ${runId}${suffix}`,
    password: 'e2e-password-123',
  }
}

// Every user the suite creates is registered seconds ago, i.e. exactly the
// audience the Get Started guide opens itself for (#2753) — and its arrows and
// dimming mask would land on top of whatever the spec is about to click. The
// opt-out lives here, at the single point users are made, rather than in each
// spec that happens to invite a second participant: those "mate" users are the
// easiest ones to forget. get-started.spec.js passes { guide: true } to meet the
// guide on purpose.
export async function register(creds, { guide = false } = {}) {
  const res = await api.post('/auth/register', creds)
  if (!guide) await skipTour(res.access_token)
  // The run's language belongs here for the same reason the tour opt-out does:
  // registration is the single point users are made. A freshly registered account
  // gets the server default (`ru`), so on `E2E_LANG=en` the "mate" a spec invites
  // used to read its panel in Russian while the author read the same panel in
  // English — a locale split inside one assertion, and nothing to do with the
  // screen under test.
  if (RUN_LANGUAGE !== 'ru') await setLanguage(res.access_token, RUN_LANGUAGE)
  return { token: res.access_token, user: res.user }
}

export async function login(email, password) {
  const res = await api.post('/auth/login', { email, password })
  return { token: res.access_token, user: res.user }
}

// seedBoard builds the full chain a board screen needs: workspace → group →
// project → board. The board comes with 4 default columns from the backend
// (`handlers/boards.go defaultColumns`), so we read them back rather than
// creating our own — that way the spec exercises the same shape a real user sees.
export async function seedBoard(token, label) {
  const ws = await api.post('/workspaces', { name: `E2E ${label}` }, token)
  const group = await api.post(`/workspaces/${ws.id}/groups`, { name: 'E2E группа' }, token)
  const project = await api.post(
    `/workspaces/${ws.id}/projects`,
    { name: `Проект ${label}`, group_id: group.id },
    token,
  )
  const board = await api.post(`/projects/${project.id}/boards`, { name: `Доска ${label}` }, token)
  const columns = await api.get(`/boards/${board.id}/columns`, token)
  return { ws, group, project, board, columns }
}

// Opt a user out of the Get Started guide. Same trap the What's-New modal fell
// into in #2749, except the guide can't baseline itself silently (it *is* the
// thing new users get), so the ack is written as "skipped". Called from
// register() — see the note there.
export async function skipTour(token) {
  return api.post('/users/me/acknowledgements', { key: 'getstarted:skipped' }, token)
}

// The interface language of a run (#2800). `E2E_LANG=en` runs the whole suite
// against the English UI: every locator is a data-testid since wave 0 of #2799,
// so a spec that fails only on `en` is pointing at a Russian string still baked
// into the markup — that is the check, not a nuisance.
//
// It lives here rather than in global-setup.js because register() needs it, and
// global-setup already imports this module.
export const RUN_LANGUAGE = process.env.E2E_LANG === 'en' ? 'en' : 'ru'

// The server preference is the source of truth — the theme store overwrites its
// localStorage cache from /users/me during bootstrap, so seeding only the browser
// side would flip the UI back to Russian a moment after the first paint.
export async function setLanguage(token, language) {
  // The endpoint is a full upsert, not a patch: read the current bundle off
  // /auth/me and send it back with one field changed, or the run would also
  // reset theme, week start and the date preset to whatever a bare body implies.
  const me = await api.get('/auth/me', token)
  return api.put('/users/me/preferences', { ...me.preferences, language }, token)
}

export async function createTask(token, boardId, columnId, title, extra = {}) {
  return api.post(`/boards/${boardId}/tasks`, { column_id: columnId, title, ...extra }, token)
}
