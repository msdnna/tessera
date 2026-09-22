import { defineStore } from 'pinia'
import { ref } from 'vue'
import { gitlab as glApi } from '@/api'

// gitlab store — the current user's GitLab connection (PAT) state. The token
// itself is never held client-side; only whether we're connected and as whom.
export const useGitlabStore = defineStore('gitlab', () => {
  const connected = ref(false)
  const baseUrl = ref('')
  const username = ref('')
  const loaded = ref(false)

  async function load() {
    try {
      const { data } = await glApi.getConnection()
      connected.value = !!data.connected
      baseUrl.value = data.base_url || ''
      username.value = data.gl_username || ''
    } catch {
      connected.value = false
    } finally {
      loaded.value = true
    }
  }

  async function connect(base, token) {
    const { data } = await glApi.connect({ base_url: base, token })
    connected.value = !!data.connected
    baseUrl.value = data.base_url || ''
    username.value = data.gl_username || ''
  }

  async function disconnect() {
    await glApi.disconnect()
    connected.value = false
    username.value = ''
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
    loaded.value = false
  }

  return { connected, baseUrl, username, loaded, load, connect, disconnect, reset }
})
