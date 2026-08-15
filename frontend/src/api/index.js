import axios from 'axios'
import { humanizeError } from '@/utils/errors'
import { reqStart, reqEnd, setOffline } from '@/composables/useConnection'
import { apiBaseURL, isTauri } from '@/utils/serverBase'

// Web: apiBaseURL() === '/api' (same-origin). Desktop (Tauri): '<server>/api',
// where <server> is the login-configured origin. See utils/serverBase.js.
const api = axios.create({ baseURL: apiBaseURL() })

// #2684 — nothing long-lived is left in localStorage on the web:
//
//  - the refresh token lives in an httpOnly cookie the backend sets when we ask
//    for it with `X-Auth-Mode: cookie` (only possible because the web build is
//    same-origin with the API);
//  - the access token lives in this module variable, so it dies with the tab.
//
// Desktop (Tauri) talks to the API cross-origin, where a host-only cookie is
// never sent, so it keeps posting the refresh token in the body and storing it
// locally. Both keep the access token in memory only.
let accessToken = ''

export function setAccessToken(token) {
  accessToken = token || ''
}

export function getAccessToken() {
  return accessToken
}

// cookieAuth is evaluated per call rather than once at import: `isTauri()` reads
// a global the webview injects, and tests toggle it between cases.
function cookieAuth() {
  return !isTauri()
}

// authModeHeaders opts a request into cookie delivery. Sent on the endpoints
// that hand out a refresh token (register/login/refresh) and on logout.
function authModeHeaders() {
  return cookieAuth() ? { 'X-Auth-Mode': 'cookie' } : {}
}

// storedRefreshToken is the desktop-only fallback: on web this is always null,
// because the token is in a cookie we deliberately cannot read.
function storedRefreshToken() {
  return cookieAuth() ? null : localStorage.getItem('tessera_refresh_token')
}

// Attach the access token on every request + track liveness for the connection
// overlay (start now, paired end in the response/error handlers below). Requests
// flagged `skipLoader` (e.g. GitLab sync, which is intentionally long and shows
// its own in-modal loader) are excluded from the global slow/offline overlay.
api.interceptors.request.use((config) => {
  if (!config.skipLoader) reqStart()
  if (accessToken) {
    config.headers.Authorization = `Bearer ${accessToken}`
  }
  return config
})

// Refresh-on-401: exchange the stored refresh token for a fresh pair and retry.
// Coalesced so concurrent 401s share one in-flight refresh.
let refreshInflight = null

async function refreshAccessToken() {
  if (refreshInflight) return refreshInflight
  const refreshToken = storedRefreshToken()
  // On web there is nothing to check up front — whether a session exists is the
  // cookie's business, and the answer is the response status.
  if (!cookieAuth() && !refreshToken) return null
  refreshInflight = axios
    .post(
      `${apiBaseURL()}/auth/refresh`,
      refreshToken ? { refresh_token: refreshToken } : {},
      // Bounded: this call gates the app's first paint on start-up, so a stalled
      // connection must fail rather than hang on a blank screen.
      { headers: authModeHeaders(), timeout: 15000 },
    )
    .then((res) => {
      const data = res.data || {}
      setAccessToken(data.access_token)
      // Only ever present in body mode (desktop); on web the rotated token went
      // back into the cookie.
      if (data.refresh_token) localStorage.setItem('tessera_refresh_token', data.refresh_token)
      return data.access_token || null
    })
    .catch(() => null)
    .finally(() => {
      refreshInflight = null
    })
  return refreshInflight
}

// restoreSession re-establishes a session on app start-up. The access token died
// with the previous page load, so the refresh cookie (desktop: the stored token)
// is the only proof we were signed in. Resolves to the new access token or null.
export function restoreSession() {
  return refreshAccessToken()
}

api.interceptors.response.use(
  (res) => {
    if (!res.config?.skipLoader) reqEnd(true)
    return res
  },
  async (err) => {
    // A response (even an error one) means we reached the server; no response
    // at all (network/DNS/timeout) means we're offline.
    const reached = !!err.response
    if (!err.config?.skipLoader) {
      reqEnd(reached)
      setOffline(!reached)
    }
    const original = err.config
    const isUnauthorized = err.response?.status === 401
    const isRefreshCall = original?.url?.includes('/auth/refresh')
    if (isUnauthorized && original && !original._retry && !isRefreshCall) {
      original._retry = true
      const newToken = await refreshAccessToken()
      if (newToken) {
        original.headers = original.headers || {}
        original.headers.Authorization = `Bearer ${newToken}`
        return api(original)
      }
    }
    if (isUnauthorized && !isRefreshCall) {
      setAccessToken('')
      localStorage.removeItem('tessera_refresh_token')
      localStorage.removeItem('tessera_user')
      window.dispatchEvent(new CustomEvent('auth:expired'))
    }
    const raw = err.response?.data?.error || err.message || 'Ошибка запроса'
    const wrapped = new Error(humanizeError(raw))
    // Keep the HTTP status reachable: most callers just show `e.message`, but a
    // few need to tell one failure from another (e.g. 409 "address taken" is
    // fixable inline, everything else is a toast).
    wrapped.status = err.response?.status
    // Distinguish "server unreachable" (network/DNS/timeout — no response at all)
    // from an HTTP error the server answered with. Callers that retry (e.g. the
    // comment composer) only retry when offline; a 4xx/5xx is not a connectivity
    // problem and must not be re-sent.
    wrapped.offline = !reached
    return Promise.reject(wrapped)
  },
)

// In-flight GET coalescing for read-only metadata. When several components mount
// in the same tick (a board open, or the GitLab settings modal and its panels)
// they fire identical GETs — members, integrations, columns, tag-prefixes — at
// once. Sharing one in-flight request collapses those duplicate round-trips.
// Each caller still gets its OWN deep clone of the data, so no consumer can
// mutate another's copy. Only genuinely concurrent calls share; once the request
// settles the key is dropped, so this never serves stale data on a later fetch.
const inflightGets = new Map()
function cloneData(d) {
  if (d == null) return d
  try {
    return structuredClone(d)
  } catch {
    return JSON.parse(JSON.stringify(d))
  }
}
function sharedGet(url, config) {
  const key = url + '::' + JSON.stringify(config?.params ?? null)
  let p = inflightGets.get(key)
  if (!p) {
    p = api.get(url, config).finally(() => inflightGets.delete(key))
    inflightGets.set(key, p)
  }
  return p.then((res) => ({ ...res, data: cloneData(res.data) }))
}

export const auth = {
  // The auth-mode header on register/login is what makes the backend put the
  // refresh token in an httpOnly cookie instead of the response body.
  register: (data) => api.post('/auth/register', data, { headers: authModeHeaders() }),
  login: (data) => api.post('/auth/login', data, { headers: authModeHeaders() }),
  // Server-side sign-out: revokes the refresh token (it would otherwise stay
  // valid for 30 days) and clears the cookie, which JS cannot delete itself.
  logout: () => {
    const stored = storedRefreshToken()
    return api.post('/auth/logout', stored ? { refresh_token: stored } : {}, {
      headers: authModeHeaders(),
    })
  },
  me: () => api.get('/auth/me'),
  // Which external login providers are enabled (for the "Continue with GitLab" button).
  providers: () => api.get('/auth/providers'),
  // Target that starts the GitLab OAuth redirect flow. On web it is a full-page
  // navigation; native clients pass their `platform` so the callback hands the session
  // back on the tessera:// deep link instead of the web SPA route (#2696).
  gitlabAuthorizeUrl: (platform) =>
    `${apiBaseURL()}/auth/gitlab/authorize${platform ? `?platform=${platform}` : ''}`,
}

export const accountFlows = {
  verifyEmail: (token) => api.post('/auth/verify-email', { token }),
  resendVerification: () => api.post('/auth/resend-verification'),
  forgotPassword: (email) => api.post('/auth/forgot-password', { email }),
  resetPassword: (token, newPassword) =>
    api.post('/auth/reset-password', { token, new_password: newPassword }),
  acceptInvitation: (token) => api.post('/invitations/accept', { token }),
}

export const users = {
  updateProfile: (data) => api.patch('/users/me', data),
  changePassword: (data) => api.put('/users/me/password', data),
  updatePreferences: (data) => api.put('/users/me/preferences', data),
  uploadAvatar: (file) => {
    const form = new FormData()
    form.append('avatar', file)
    // Let the browser set Content-Type with the multipart boundary — setting it
    // manually omits the boundary and the server can't parse the upload.
    return api.put('/users/me/avatar', form)
  },
  deleteAvatar: () => api.delete('/users/me/avatar'),
  avatarUrl: (id) => `${apiBaseURL()}/users/${id}/avatar`,
}

// Global-admin panel (every endpoint re-checks is_admin server-side).
export const admin = {
  listUsers: () => api.get('/admin/users'),
  setActive: (id, active) => api.patch(`/admin/users/${id}/active`, { active }),
  setAdmin: (id, isAdmin) => api.patch(`/admin/users/${id}/admin`, { admin: isAdmin }),
  resetLink: (id) => api.post(`/admin/users/${id}/reset-link`),
  // GitLab OAuth app config.
  getOAuth: () => api.get('/admin/oauth/gitlab'),
  setOAuth: (data) => api.put('/admin/oauth/gitlab', data),
  // Background jobs panel (instance-level).
  jobs: () => api.get('/admin/jobs', { skipLoader: true }),
  runJob: (key) =>
    api.post(`/admin/jobs/${encodeURIComponent(key)}/run`, null, { skipLoader: true }),
  cancelJob: (key) =>
    api.post(`/admin/jobs/${encodeURIComponent(key)}/cancel`, null, { skipLoader: true }),
}

export const workspaces = {
  list: () => api.get('/workspaces'),
  get: (id) => api.get(`/workspaces/${id}`),
  create: (data) => api.post('/workspaces', data),
  // Dangerous: delete a workspace with everything in it (owner only, 403 otherwise).
  remove: (id) => api.delete(`/workspaces/${id}`),
  members: (id) => sharedGet(`/workspaces/${id}/members`),
  gitlabMembers: (id) => sharedGet(`/workspaces/${id}/gitlab/members`),
  addMember: (id, data) => api.post(`/workspaces/${id}/members`, data),
  updateMemberRole: (id, userId, role) =>
    api.patch(`/workspaces/${id}/members/${userId}`, { role }),
  removeMember: (id, userId) => api.delete(`/workspaces/${id}/members/${userId}`),
  invitations: (id) => api.get(`/workspaces/${id}/invitations`),
  createInvitation: (id, data) => api.post(`/workspaces/${id}/invitations`, data),
  deleteInvitation: (id, invId) => api.delete(`/workspaces/${id}/invitations/${invId}`),
  groups: (id) => api.get(`/workspaces/${id}/groups`),
  createGroup: (id, data) => api.post(`/workspaces/${id}/groups`, data),
  projects: (id) => api.get(`/workspaces/${id}/projects`),
  createProject: (id, data) => api.post(`/workspaces/${id}/projects`, data),
  search: (id, q) => api.get(`/workspaces/${id}/search`, { params: { q } }),
  tasks: (id, params) => api.get(`/workspaces/${id}/tasks`, { params }),
  // Resolve a per-workspace task number (#252) to its task — for /board/<slug>?task=<number> links.
  taskByNumber: (id, number) => api.get(`/workspaces/${id}/tasks/by-number/${number}`),
  summary: (id) => api.get(`/workspaces/${id}/summary`),
  // Every tag across the workspace's projects — read-only, for cross-project
  // views (Home). Tags are created/listed per-project (see `projects` below).
  tags: (id) => sharedGet(`/workspaces/${id}/tags`),
  // Friendly tag-prefix names across the workspace's projects, deduped by prefix —
  // lets cross-project views render scoped tag pills («scope │ value») too.
  tagPrefixes: (id) => sharedGet(`/workspaces/${id}/tag-prefixes`),
  // Workspace-wide default estimation config; `null` clears it to the built-in default.
  setEstimation: (id, config) => api.put(`/workspaces/${id}/estimation`, config),
  // Every milestone across the workspace's projects with task rollups — for the «Этапы» screen.
  milestones: (id) => sharedGet(`/workspaces/${id}/milestones`),
  // Quick-action registry for the editor popup: built-in commands (from the
  // backend's quickact.Registry) + this workspace's custom dictionary, plus
  // can_manage — the only place the frontend learns its own workspace role.
  commands: (id) => api.get(`/workspaces/${id}/commands`),
  // Full desired state of the custom dictionary (owner/admin only).
  setCommands: (id, commands) => api.put(`/workspaces/${id}/commands`, { commands }),
}

export const projects = {
  get: (id) => api.get(`/projects/${id}`),
  update: (id, data) => api.patch(`/projects/${id}`, data),
  // Change the project's URL address (owner/admin only). Links already handed
  // out with the old address stop resolving — warn before calling.
  setSlug: (id, slug) => api.patch(`/projects/${id}/slug`, { slug }),
  move: (id, data) => api.patch(`/projects/${id}/move`, data),
  // Dangerous: move a project (with all boards/tasks) to another workspace.
  transfer: (id, data) => api.post(`/projects/${id}/transfer`, data),
  remove: (id) => api.delete(`/projects/${id}`),
  boards: (id) => api.get(`/projects/${id}/boards`),
  createBoard: (id, data) => api.post(`/projects/${id}/boards`, data),
  tags: (id) => sharedGet(`/projects/${id}/tags`),
  createTag: (id, data) => api.post(`/projects/${id}/tags`, data),
  updateTag: (tagId, data) => api.patch(`/tags/${tagId}`, data),
  deleteTag: (tagId) => api.delete(`/tags/${tagId}`),
  tagPrefixes: (id) => sharedGet(`/projects/${id}/tag-prefixes`),
  setTagPrefixes: (id, prefixes) => api.put(`/projects/${id}/tag-prefixes`, { prefixes }),
  // Per-project estimation override; `null` clears it to inherit the workspace default.
  setEstimation: (id, config) => api.put(`/projects/${id}/estimation`, config),
  // Milestones («Этап»): project-scoped.
  milestones: (id) => sharedGet(`/projects/${id}/milestones`),
  createMilestone: (id, data) => api.post(`/projects/${id}/milestones`, data),
}

export const milestones = {
  update: (id, data) => api.patch(`/milestones/${id}`, data),
  remove: (id) => api.delete(`/milestones/${id}`),
  // Explicitly create this native milestone in GitLab + link them (opt-in per milestone).
  pushToGitlab: (id) => api.post(`/milestones/${id}/gitlab`),
}

export const groups = {
  update: (id, data) => api.patch(`/groups/${id}`, data),
  move: (id, data) => api.patch(`/groups/${id}/move`, data),
  remove: (id) => api.delete(`/groups/${id}`),
}

export const boards = {
  get: (id) => api.get(`/boards/${id}`),
  // Resolve a /project/<slug>/board/<slug> pair to the board.
  resolve: (projectSlug, boardSlug) =>
    api.get('/board-by-slug', { params: { project: projectSlug, board: boardSlug } }),
  update: (id, data) => api.patch(`/boards/${id}`, data),
  setDoneColumn: (id, columnId) => api.patch(`/boards/${id}/done-column`, { column_id: columnId }),
  remove: (id) => api.delete(`/boards/${id}`),
  columns: (id) => sharedGet(`/boards/${id}/columns`),
  createColumn: (id, data) => api.post(`/boards/${id}/columns`, data),
  // params.milestone: '<slug|uuid>' scopes to one sprint, 'backlog' to no-sprint tasks.
  tasks: (id, params) => api.get(`/boards/${id}/tasks`, params ? { params } : undefined),
  subtasks: (id) => api.get(`/boards/${id}/subtasks`),
  archive: (id) => api.get(`/boards/${id}/archive`),
  // Whole-board blocking dependency graph for the Gantt view (raw edge rows).
  dependencies: (id) => api.get(`/boards/${id}/dependencies`),
  createTask: (id, data) => api.post(`/boards/${id}/tasks`, data),
  // Per-user saved board views (layouts).
  views: (id) => api.get(`/boards/${id}/views`),
  saveView: (id, data) => api.post(`/boards/${id}/views`, data), // { name, config }
  deleteView: (viewId) => api.delete(`/views/${viewId}`),
}

export const notes = {
  list: (wsId) => api.get(`/workspaces/${wsId}/notes`),
  create: (wsId, data) => api.post(`/workspaces/${wsId}/notes`, data),
  get: (id) => api.get(`/notes/${id}`),
  update: (id, data) => api.patch(`/notes/${id}`, data),
  remove: (id) => api.delete(`/notes/${id}`),
}

export const documents = {
  list: (wsId, projectId) =>
    api.get(
      `/workspaces/${wsId}/documents`,
      projectId ? { params: { project_id: projectId } } : {},
    ),
  create: (wsId, data) => api.post(`/workspaces/${wsId}/documents`, data),
  get: (id) => api.get(`/documents/${id}`),
  // Resolves a workspace-scoped slug. The response carries workspace_id so a
  // deep link can point the app at the right workspace before mounting.
  bySlug: (wsId, slug) => api.get(`/workspaces/${wsId}/documents/by-slug/${slug}`),
  update: (id, data) => api.patch(`/documents/${id}`, data),
  // Content has its own endpoint: the metadata PATCH above is what realtime and
  // the list are built around, and both deliberately exclude content (D1). It
  // carries the updated_at the client last saw — the server answers 409 when
  // that no longer matches, rather than silently overwriting someone's edit.
  // skipLoader: autosave fires while typing and must not flash the global bar.
  updateContent: (id, content, updatedAt) =>
    api.patch(`/documents/${id}/content`, { content, updated_at: updatedAt }, { skipLoader: true }),
  uploadAsset: (id, formData) => api.post(`/documents/${id}/assets`, formData),
  remove: (id, recursive = false) =>
    api.delete(`/documents/${id}${recursive ? '?recursive=true' : ''}`),
}

export const reminders = {
  list: () => api.get('/reminders'),
  create: (data) => api.post('/reminders', data),
  update: (id, data) => api.patch(`/reminders/${id}`, data),
  remove: (id) => api.delete(`/reminders/${id}`),
}

export const columns = {
  update: (id, data) => api.patch(`/columns/${id}`, data),
  move: (id, data) => api.patch(`/columns/${id}/move`, data),
  remove: (id) => api.delete(`/columns/${id}`),
}

export const tasks = {
  get: (id) => api.get(`/tasks/${id}`),
  // Board cards ship without the description (see backend task_list_dto.go); the
  // card fetches it lazily on hover. Background call — no global progress bar.
  description: (id) => api.get(`/tasks/${id}/description`, { skipLoader: true }),
  update: (id, data) => api.patch(`/tasks/${id}`, data),
  move: (id, data) => api.patch(`/tasks/${id}/move`, data),
  setParent: (id, parentId) => api.patch(`/tasks/${id}/parent`, { parent_id: parentId }),
  transfer: (id, data) => api.patch(`/tasks/${id}/transfer`, data),
  archive: (id, opts) => api.patch(`/tasks/${id}/archive`, null, { params: opts }),
  restore: (id) => api.patch(`/tasks/${id}/restore`),
  remove: (id, opts) => api.delete(`/tasks/${id}`, { params: opts }),
  addTag: (id, tagId) => api.post(`/tasks/${id}/tags`, { tag_id: tagId }),
  removeTag: (id, tagId) => api.delete(`/tasks/${id}/tags/${tagId}`),
  addAssignee: (id, userId) => api.post(`/tasks/${id}/assignees`, { user_id: userId }),
  removeAssignee: (id, userId) => api.delete(`/tasks/${id}/assignees/${userId}`),
  // Milestone («Этап»): assign (milestone id) or clear (null / clear endpoint).
  setMilestone: (id, milestoneId) =>
    api.post(`/tasks/${id}/milestone`, { milestone_id: milestoneId }),
  clearMilestone: (id) => api.delete(`/tasks/${id}/milestone`),
  pinGitlabAssignee: (id, data) => api.post(`/tasks/${id}/gitlab-assignees`, data),
  removeGitlabAssignee: (id, username) =>
    api.delete(`/tasks/${id}/gitlab-assignees/${encodeURIComponent(username)}`),
  // Per-task due-notification override (null fields = inherit the user default).
  dueNotify: (id, data) => api.patch(`/tasks/${id}/due-notify`, data),
  // Eisenhower-matrix quadrant override (quadrant 0-3, or null = derive from
  // priority + due-date). Driven by the matrix view's drag-between-quadrants.
  eisenhower: (id, quadrant) => api.patch(`/tasks/${id}/eisenhower`, { quadrant }),
  // Rich task detail (#8)
  events: (id) => api.get(`/tasks/${id}/events`),
  comments: (id) => api.get(`/tasks/${id}/comments`),
  addComment: (id, body, mentions) => api.post(`/tasks/${id}/comments`, { body, mentions }),
  // Dry-run the quick actions in a draft comment — same parser as the real
  // POST, changes nothing. Powers the «Будет применено: …» hint.
  previewCommands: (id, body) =>
    api.post(`/tasks/${id}/commands/preview`, { body }, { skipLoader: true }),
  updateComment: (commentId, body) => api.patch(`/comments/${commentId}`, { body }),
  removeComment: (commentId) => api.delete(`/comments/${commentId}`),
  relations: (id) => api.get(`/tasks/${id}/relations`),
  addRelation: (id, number, kind) => api.post(`/tasks/${id}/relations`, { number, kind }),
  removeRelation: (relationId) => api.delete(`/relations/${relationId}`),
  attachments: (id) => api.get(`/tasks/${id}/attachments`),
  // Attachment up/download can be large and slow — keep them off the global bar
  // (their call sites show local progress); a blocking overlay here froze the
  // whole task modal on a remote install.
  uploadAttachment: (id, formData) =>
    api.post(`/tasks/${id}/attachments`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
      skipLoader: true,
    }),
  downloadAttachment: (attachmentId) =>
    api.get(`/attachments/${attachmentId}/download`, {
      responseType: 'blob',
      skipLoader: true,
    }),
  removeAttachment: (attachmentId) => api.delete(`/attachments/${attachmentId}`),
}

export const uploads = {
  // Inline image for descriptions/comments → { url }. Served publicly.
  upload: (formData) =>
    api.post('/uploads', formData, { headers: { 'Content-Type': 'multipart/form-data' } }),
}

export const notifications = {
  list: () => api.get('/notifications'),
  unreadCount: () => api.get('/notifications/unread-count'),
  markRead: (id) => api.post(`/notifications/${id}/read`),
  markAllRead: () => api.post('/notifications/read-all'),
}

// Notification router: per-user delivery channels + Alertmanager-style routing
// rules (email / telegram / webhook).
export const notificationChannels = {
  list: () => api.get('/notification-channels'),
  create: (data) => api.post('/notification-channels', data),
  update: (id, data) => api.patch(`/notification-channels/${id}`, data),
  remove: (id) => api.delete(`/notification-channels/${id}`),
  // Both reach out to an external transport (SMTP / Telegram / webhook) and can
  // stall for seconds — background them so the settings dialog stays usable.
  test: (id) => api.post(`/notification-channels/${id}/test`, null, { skipLoader: true }),
  previewTemplate: (template) =>
    api.post('/notification-template-preview', { template }, { skipLoader: true }),
  // Auto-register this client as a routable "device" channel (idempotent).
  registerDevice: (data) => api.post('/notification-devices', data),
}

export const notificationRoutes = {
  list: () => api.get('/notification-routes'),
  create: (data) => api.post('/notification-routes', data),
  update: (id, data) => api.patch(`/notification-routes/${id}`, data),
  remove: (id) => api.delete(`/notification-routes/${id}`),
}

// Per-user notification scheduling defaults (lead/repeat before due, reminders).
export const notificationPrefs = {
  get: () => api.get('/notification-prefs'),
  update: (data) => api.put('/notification-prefs', data),
}

export const gitlab = {
  // Per-user GitLab connection (PAT).
  getConnection: () => api.get('/gitlab/connection'),
  connect: (data) => api.post('/gitlab/connection', data), // { base_url, token }
  disconnect: () => api.delete('/gitlab/connection'),
  // Per-workspace integration bindings (multi-binding: several GL project → board).
  listIntegrations: (wsId) => sharedGet(`/workspaces/${wsId}/gitlab/integrations`),
  createIntegration: (wsId, data) => api.post(`/workspaces/${wsId}/gitlab/integrations`, data),
  updateIntegration: (wsId, integId, data) =>
    api.put(`/workspaces/${wsId}/gitlab/integrations/${integId}`, data),
  deleteIntegration: (wsId, integId) =>
    api.delete(`/workspaces/${wsId}/gitlab/integrations/${integId}`),
  // skipLoader: sync is intentionally long and shows its own in-modal loader, so
  // it must not trigger the global slow/offline overlay. mode 'full' forces a full
  // sweep ("Полная синхронизация"); omitted → the default incremental pull.
  sync: (wsId, integId, mode) =>
    api.post(`/workspaces/${wsId}/gitlab/integrations/${integId}/sync`, null, {
      params: mode ? { mode } : undefined,
      skipLoader: true,
    }),
  // Sync journal: run/action history + retry of a failed push.
  syncRuns: (wsId, limit = 50) =>
    api.get(`/workspaces/${wsId}/gitlab/sync-runs`, { params: { limit } }),
  // One run's actions, keyset-paginated by seq WITHOUT the heavy before/after
  // diff (fetched per row on demand). Returns { items, has_more, next_after_seq }.
  // A 2500-action run used to ship as one multi-MB blocking response.
  syncRunActions: (wsId, runId, { limit, afterSeq } = {}) =>
    api.get(`/workspaces/${wsId}/gitlab/sync-runs/${runId}/actions`, {
      params: { limit, after_seq: afterSeq },
      skipLoader: true,
    }),
  // Lazily fetch one action's before/after diff JSONB → { detail }.
  syncActionDetail: (wsId, runId, actionId) =>
    api.get(`/workspaces/${wsId}/gitlab/sync-runs/${runId}/actions/${actionId}/detail`, {
      skipLoader: true,
    }),
  retryWriteback: (wsId, runId, actionId) =>
    api.post(`/workspaces/${wsId}/gitlab/sync-runs/${runId}/actions/${actionId}/retry`),
  // Create a GitLab issue from a task (returns the new link view) + the project's
  // issue templates for prefilling the description.
  createIssue: (taskId, data) => api.post(`/tasks/${taskId}/gitlab-issue`, data),
  issueTemplates: (wsId, integId) =>
    api.get(`/workspaces/${wsId}/gitlab/issue-templates`, {
      params: integId ? { integration_id: integId } : undefined,
      skipLoader: true,
    }),
  // Write-back conflicts: open-conflict inbox + interactive resolution.
  conflicts: (wsId) => api.get(`/workspaces/${wsId}/gitlab/conflicts`),
  resolveConflict: (taskId, conflictId, data) =>
    api.post(`/tasks/${taskId}/gitlab/conflicts/${conflictId}/resolve`, data), // { resolution, value? }
}

export default api
