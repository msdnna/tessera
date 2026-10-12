import { defineStore } from 'pinia'
import { ref } from 'vue'
import { gitlab as glApi } from '@/api'

// gitlab store — the current user's GitLab connection state: a PAT or an OAuth
// grant (#3083). The token itself is never held client-side; only whether we're
// connected, as whom, how, and until when the PAT holds (#3084).
export const useGitlabStore = defineStore('gitlab', () => {
  const connected = ref(false)
  const baseUrl = ref('')
  const username = ref('')
  const authKind = ref('') // 'pat' | 'oauth'
  const tokenExpiresAt = ref(null) // 'YYYY-MM-DD' | null (never / unknown)
  const loaded = ref(false)

  function apply(data) {
    connected.value = !!data.connected
    baseUrl.value = data.base_url || ''
    username.value = data.gl_username || ''
    authKind.value = data.auth_kind || (data.connected ? 'pat' : '')
    tokenExpiresAt.value = data.token_expires_at || null
  }

  async function load() {
    try {
      const { data } = await glApi.getConnection()
      apply(data)
    } catch {
      connected.value = false
    } finally {
      loaded.value = true
    }
  }

  async function connect(base, token) {
    const { data } = await glApi.connect({ base_url: base, token })
    apply(data)
  }

  // «Войти через GitLab» (#3083): the server answers GitLab's authorize URL and
  // the page goes there; GitLab sends the browser back to `returnTo` with
  // ?gitlab_oauth=connected (or the reason it failed).
  async function connectOAuth(returnTo) {
    const { data } = await glApi.connectOAuth(returnTo)
    window.location.assign(data.url)
  }

  async function disconnect() {
    await glApi.disconnect()
    connected.value = false
    username.value = ''
    authKind.value = ''
    tokenExpiresAt.value = null
  }

  // A GitLab connection belongs to one account, so it goes when the session does
  // (#2932). `loaded` above is a latch — the settings screen paints from the
  // store as soon as it is set and never re-reads — so a stale connection is not
  // a flash: the next reader in the same tab is told they are signed in to
  // GitLab as somebody else, and the integration controls that state gates
  // (create issue, link, sync) are offered on their behalf.
  function reset() {
    connected.value = false
    baseUrl.value = ''
    username.value = ''
    authKind.value = ''
    tokenExpiresAt.value = null
    loaded.value = false
  }

  return {
    connected,
    baseUrl,
    username,
    authKind,
    tokenExpiresAt,
    loaded,
    load,
    connect,
    connectOAuth,
    disconnect,
    reset,
  }
})
