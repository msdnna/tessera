import axios from 'axios'
import { humanizeError } from '@/utils/errors'
import { reqStart, reqEnd, setOffline } from '@/composables/useConnection'
import { apiBaseURL } from '@/utils/serverBase'

// Web: apiBaseURL() === '/api' (same-origin). Desktop (Tauri): '<server>/api',
// where <server> is the login-configured origin. See utils/serverBase.js.
const api = axios.create({ baseURL: apiBaseURL() })

// Attach the access token on every request + track liveness for the connection
// overlay (start now, paired end in the response/error handlers below). Requests
// flagged `skipLoader` (e.g. GitLab sync, which is intentionally long and shows
// its own in-modal loader) are excluded from the global slow/offline overlay.
api.interceptors.request.use((config) => {
  if (!config.skipLoader) reqStart()
  const token = localStorage.getItem('tessera_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Refresh-on-401: exchange the stored refresh token for a fresh pair and retry.
// Coalesced so concurrent 401s share one in-flight refresh.
let refreshInflight = null

async function refreshAccessToken() {
  if (refreshInflight) return refreshInflight
  const refreshToken = localStorage.getItem('tessera_refresh_token')
  if (!refreshToken) return null
  refreshInflight = axios
    .post(`${apiBaseURL()}/auth/refresh`, { refresh_token: refreshToken })
    .then((res) => {
      const data = res.data || {}
      if (data.access_token) localStorage.setItem('tessera_token', data.access_token)
      if (data.refresh_token) localStorage.setItem('tessera_refresh_token', data.refresh_token)
      return data.access_token || null
    })
    .catch(() => null)
    .finally(() => {
      refreshInflight = null
    })
  return refreshInflight
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
      localStorage.removeItem('tessera_token')
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
    return Promise.reject(wrapped)
  },
)

export const auth = {
  register: (data) => api.post('/auth/register', data),
  login: (data) => api.post('/auth/login', data),
  me: () => api.get('/auth/me'),
  // Which external login providers are enabled (for the "Continue with GitLab" button).
  providers: () => api.get('/auth/providers'),
  // Full-page navigation target that starts the GitLab OAuth redirect flow.
  gitlabAuthorizeUrl: () => `${apiBaseURL()}/auth/gitlab/authorize`,
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
  members: (id) => api.get(`/workspaces/${id}/members`),
  gitlabMembers: (id) => api.get(`/workspaces/${id}/gitlab/members`),
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
  tags: (id) => api.get(`/workspaces/${id}/tags`),
  // Friendly tag-prefix names across the workspace's projects, deduped by prefix —
  // lets cross-project views render scoped tag pills («scope │ value») too.
  tagPrefixes: (id) => api.get(`/workspaces/${id}/tag-prefixes`),
  // Workspace-wide default estimation config; `null` clears it to the built-in default.
  setEstimation: (id, config) => api.put(`/workspaces/${id}/estimation`, config),
  // Every milestone across the workspace's projects with task rollups — for the «Этапы» screen.
  milestones: (id) => api.get(`/workspaces/${id}/milestones`),
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
  tags: (id) => api.get(`/projects/${id}/tags`),
  createTag: (id, data) => api.post(`/projects/${id}/tags`, data),
  updateTag: (tagId, data) => api.patch(`/tags/${tagId}`, data),
  deleteTag: (tagId) => api.delete(`/tags/${tagId}`),
  tagPrefixes: (id) => api.get(`/projects/${id}/tag-prefixes`),
  setTagPrefixes: (id, prefixes) => api.put(`/projects/${id}/tag-prefixes`, { prefixes }),
  // Per-project estimation override; `null` clears it to inherit the workspace default.
  setEstimation: (id, config) => api.put(`/projects/${id}/estimation`, config),
  // Milestones («Этап»): project-scoped.
  milestones: (id) => api.get(`/projects/${id}/milestones`),
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
  columns: (id) => api.get(`/boards/${id}/columns`),
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
  updateComment: (commentId, body) => api.patch(`/comments/${commentId}`, { body }),
  removeComment: (commentId) => api.delete(`/comments/${commentId}`),
  relations: (id) => api.get(`/tasks/${id}/relations`),
  addRelation: (id, number, kind) => api.post(`/tasks/${id}/relations`, { number, kind }),
  removeRelation: (relationId) => api.delete(`/relations/${relationId}`),
  attachments: (id) => api.get(`/tasks/${id}/attachments`),
  uploadAttachment: (id, formData) =>
    api.post(`/tasks/${id}/attachments`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    }),
  downloadAttachment: (attachmentId) =>
    api.get(`/attachments/${attachmentId}/download`, { responseType: 'blob' }),
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
  test: (id) => api.post(`/notification-channels/${id}/test`),
  previewTemplate: (template) => api.post('/notification-template-preview', { template }),
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
  listIntegrations: (wsId) => api.get(`/workspaces/${wsId}/gitlab/integrations`),
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
  syncRunActions: (wsId, runId) => api.get(`/workspaces/${wsId}/gitlab/sync-runs/${runId}/actions`),
  retryWriteback: (wsId, runId, actionId) =>
    api.post(`/workspaces/${wsId}/gitlab/sync-runs/${runId}/actions/${actionId}/retry`),
  // Create a GitLab issue from a task (returns the new link view) + the project's
  // issue templates for prefilling the description.
  createIssue: (taskId, data) => api.post(`/tasks/${taskId}/gitlab-issue`, data),
  issueTemplates: (wsId, integId) =>
    api.get(
      `/workspaces/${wsId}/gitlab/issue-templates`,
      integId ? { params: { integration_id: integId } } : undefined,
    ),
  // Write-back conflicts: open-conflict inbox + interactive resolution.
  conflicts: (wsId) => api.get(`/workspaces/${wsId}/gitlab/conflicts`),
  resolveConflict: (taskId, conflictId, data) =>
    api.post(`/tasks/${taskId}/gitlab/conflicts/${conflictId}/resolve`, data), // { resolution, value? }
}

export default api
