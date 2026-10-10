// The page of all notifications (#3049): the feed read from the server a page
// at a time, filtered by the server, with the unread counts per filter for the
// tabs. The bell's popover keeps reading the store's newest 50; this is what
// lies past them.
//
// Rows that arrive over the socket while the page is open land on top of the
// list when they belong under the current filter (the store's onEvent is the
// only listener; this watches its list), and the read state is shared by id —
// the store and the page hold different copies of the same row.
import { ref, reactive, watch } from 'vue'
import { notifications as api } from '@/api'
import { useNotificationsStore } from '@/stores/notifications'
import { inFilter } from '@/utils/notificationFeed'

export const PAGE_SIZE = 30

export function useNotificationPages() {
  const notes = useNotificationsStore()
  const filter = ref('all')
  const items = ref([])
  const cursor = ref(null)
  const done = ref(false)
  const loading = ref(false)
  const error = ref('')
  const counts = reactive({ total: 0, mentions: 0, assigned: 0, gitlab: 0 })

  // A filter switched mid-flight must not receive the previous filter's page.
  let seq = 0

  async function loadMore() {
    if (loading.value || done.value) return
    const mine = seq
    loading.value = true
    error.value = ''
    try {
      const params = { filter: filter.value, limit: PAGE_SIZE }
      if (cursor.value) params.cursor = cursor.value
      const { data } = await api.page(params)
      if (mine !== seq) return
      const seen = new Set(items.value.map((n) => n.id))
      items.value = [...items.value, ...(data?.items || []).filter((n) => !seen.has(n.id))]
      cursor.value = data?.next_cursor || null
      done.value = !cursor.value
    } catch (e) {
      if (mine === seq) error.value = e.message
    } finally {
      if (mine === seq) loading.value = false
    }
  }

  async function loadCounts() {
    try {
      const { data } = await api.unreadCount()
      counts.total = data?.count ?? 0
      Object.assign(counts, data?.by_group || {})
    } catch {
      /* the tabs go without numbers */
    }
  }

  function reset(next = filter.value) {
    seq++
    filter.value = next
    items.value = []
    cursor.value = null
    done.value = false
    loading.value = false
    error.value = ''
    loadCounts()
    return loadMore()
  }

  // Live arrivals: the store unshifts them; the page takes the ones it lacks.
  watch(
    () => notes.items[0]?.id,
    () => {
      const have = new Set(items.value.map((n) => n.id))
      const fresh = []
      for (const n of notes.items) {
        if (have.has(n.id)) break
        if (inFilter(n, filter.value)) fresh.push({ ...n })
      }
      if (fresh.length) {
        items.value = [...fresh, ...items.value]
        loadCounts()
      }
    },
  )

  // Read state follows the store by id: a row read anywhere (the popover, a
  // toast, another tab's resync) reads here too.
  watch(
    () => notes.items.map((n) => (n.read_at ? n.id : '')).join(),
    () => {
      const read = new Map(notes.items.filter((n) => n.read_at).map((n) => [n.id, n]))
      for (const n of items.value) {
        const s = read.get(n.id)
        if (s && !n.read_at) {
          n.read_at = s.read_at
          if (s.rsvp) n.rsvp = s.rsvp
        }
      }
    },
  )

  function markLocal(n) {
    if (n && !n.read_at) n.read_at = new Date().toISOString()
  }
  function markAllLocal() {
    const now = new Date().toISOString()
    for (const n of items.value) if (!n.read_at) n.read_at = now
    counts.total = counts.mentions = counts.assigned = counts.gitlab = 0
  }

  return {
    filter,
    items,
    done,
    loading,
    error,
    counts,
    reset,
    loadMore,
    loadCounts,
    markLocal,
    markAllLocal,
  }
}
