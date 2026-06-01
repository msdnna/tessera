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

export const workspaces = {
  list: () => api.get('/workspaces'),
  get: (id) => api.get(`/workspaces/${id}`),
  create: (data) => api.post('/workspaces', data),
  members: (id) => api.get(`/workspaces/${id}/members`),
  addMember: (id, data) => api.post(`/workspaces/${id}/members`, data),
  groups: (id) => api.get(`/workspaces/${id}/groups`),
  createGroup: (id, data) => api.post(`/workspaces/${id}/groups`, data),
  projects: (id) => api.get(`/workspaces/${id}/projects`),
  createProject: (id, data) => api.post(`/workspaces/${id}/projects`, data),
  tags: (id) => api.get(`/workspaces/${id}/tags`),
  createTag: (id, data) => api.post(`/workspaces/${id}/tags`, data),
}

export const projects = {
  get: (id) => api.get(`/projects/${id}`),
  update: (id, data) => api.patch(`/projects/${id}`, data),
  remove: (id) => api.delete(`/projects/${id}`),
  boards: (id) => api.get(`/projects/${id}/boards`),
  createBoard: (id, data) => api.post(`/projects/${id}/boards`, data),
}

export const boards = {
  get: (id) => api.get(`/boards/${id}`),
  update: (id, data) => api.patch(`/boards/${id}`, data),
  remove: (id) => api.delete(`/boards/${id}`),
  columns: (id) => api.get(`/boards/${id}/columns`),
  createColumn: (id, data) => api.post(`/boards/${id}/columns`, data),
  tasks: (id) => api.get(`/boards/${id}/tasks`),
  createTask: (id, data) => api.post(`/boards/${id}/tasks`, data),
}

export const tasks = {
  get: (id) => api.get(`/tasks/${id}`),
  update: (id, data) => api.patch(`/tasks/${id}`, data),
  move: (id, data) => api.patch(`/tasks/${id}/move`, data),
  remove: (id) => api.delete(`/tasks/${id}`),
  addTag: (id, tagId) => api.post(`/tasks/${id}/tags`, { tag_id: tagId }),
  removeTag: (id, tagId) => api.delete(`/tasks/${id}/tags/${tagId}`),
  addAssignee: (id, userId) => api.post(`/tasks/${id}/assignees`, { user_id: userId }),
  removeAssignee: (id, userId) => api.delete(`/tasks/${id}/assignees/${userId}`),
}

export default api
