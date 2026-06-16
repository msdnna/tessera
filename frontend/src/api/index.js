import axios from 'axios'

const api = axios.create({ baseURL: '/api' })

// Attach the access token on every request.
api.interceptors.request.use((config) => {
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
    .post('/api/auth/refresh', { refresh_token: refreshToken })
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
  (res) => res,
  async (err) => {
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
    const msg = err.response?.data?.error || err.message || 'Ошибка запроса'
    return Promise.reject(new Error(msg))
  },
)

export const auth = {
  register: (data) => api.post('/auth/register', data),
  login: (data) => api.post('/auth/login', data),
  me: () => api.get('/auth/me'),
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
  avatarUrl: (id) => `/api/users/${id}/avatar`,
}

// Global-admin panel (every endpoint re-checks is_admin server-side).
export const admin = {
  listUsers: () => api.get('/admin/users'),
  setActive: (id, active) => api.patch(`/admin/users/${id}/active`, { active }),
  setAdmin: (id, isAdmin) => api.patch(`/admin/users/${id}/admin`, { admin: isAdmin }),
  resetLink: (id) => api.post(`/admin/users/${id}/reset-link`),
}

export const workspaces = {
  list: () => api.get('/workspaces'),
  get: (id) => api.get(`/workspaces/${id}`),
  create: (data) => api.post('/workspaces', data),
  members: (id) => api.get(`/workspaces/${id}/members`),
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
  summary: (id) => api.get(`/workspaces/${id}/summary`),
  tags: (id) => api.get(`/workspaces/${id}/tags`),
  createTag: (id, data) => api.post(`/workspaces/${id}/tags`, data),
  updateTag: (tagId, data) => api.patch(`/tags/${tagId}`, data),
  deleteTag: (tagId) => api.delete(`/tags/${tagId}`),
}

export const projects = {
  get: (id) => api.get(`/projects/${id}`),
  update: (id, data) => api.patch(`/projects/${id}`, data),
  move: (id, data) => api.patch(`/projects/${id}/move`, data),
  remove: (id) => api.delete(`/projects/${id}`),
  boards: (id) => api.get(`/projects/${id}/boards`),
  createBoard: (id, data) => api.post(`/projects/${id}/boards`, data),
}

export const groups = {
  update: (id, data) => api.patch(`/groups/${id}`, data),
  move: (id, data) => api.patch(`/groups/${id}/move`, data),
  remove: (id) => api.delete(`/groups/${id}`),
}

export const boards = {
  get: (id) => api.get(`/boards/${id}`),
  update: (id, data) => api.patch(`/boards/${id}`, data),
  setDoneColumn: (id, columnId) => api.patch(`/boards/${id}/done-column`, { column_id: columnId }),
  remove: (id) => api.delete(`/boards/${id}`),
  columns: (id) => api.get(`/boards/${id}/columns`),
  createColumn: (id, data) => api.post(`/boards/${id}/columns`, data),
  tasks: (id) => api.get(`/boards/${id}/tasks`),
  subtasks: (id) => api.get(`/boards/${id}/subtasks`),
  archive: (id) => api.get(`/boards/${id}/archive`),
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
}

export const notificationRoutes = {
  list: () => api.get('/notification-routes'),
  create: (data) => api.post('/notification-routes', data),
  update: (id, data) => api.patch(`/notification-routes/${id}`, data),
  remove: (id) => api.delete(`/notification-routes/${id}`),
}

export const gitlab = {
  // Per-user GitLab connection (PAT).
  getConnection: () => api.get('/gitlab/connection'),
  connect: (data) => api.post('/gitlab/connection', data), // { base_url, token }
  disconnect: () => api.delete('/gitlab/connection'),
  // Per-workspace integration config + manual sync.
  getIntegration: (wsId) => api.get(`/workspaces/${wsId}/gitlab/integration`),
  setIntegration: (wsId, data) => api.put(`/workspaces/${wsId}/gitlab/integration`, data),
  sync: (wsId) => api.post(`/workspaces/${wsId}/gitlab/sync`),
}

export default api
