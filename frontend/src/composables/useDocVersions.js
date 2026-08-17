import { computed, ref } from 'vue'
import { documents as docsApi } from '@/api'
import { diffDocs, diffSummary } from '@/utils/docDiff'

/**
 * Version journal of one open document (#2731).
 *
 * Bodies are fetched one version at a time and cached here: the journal lists
 * fifty entries and a document is up to a megabyte of JSON, so the list endpoint
 * carries none of them. The cache is keyed by version id and never invalidated —
 * a version is immutable once its session is over, and the only mutable one (the
 * session currently being typed into) is refetched with the list.
 *
 * @returns {object} state and actions
 */
export function useDocVersions() {
  const versions = ref([])
  const loading = ref(false)
  const error = ref('')
  // The version being looked at, and the one it is compared against. `compareId`
  // empty means "compare with the newest", which is the question people actually
  // ask of a journal ("что изменилось с тех пор").
  const selectedId = ref('')
  const compareId = ref('')
  const bodies = ref({})

  let docId = ''

  const selected = computed(() => versions.value.find((v) => v.id === selectedId.value) || null)
  const baseline = computed(() => {
    if (compareId.value) return versions.value.find((v) => v.id === compareId.value) || null
    return versions.value[0] || null
  })

  // The comparison is only meaningful once both bodies are in hand; until then
  // the panel shows its loading state rather than an empty "no changes" diff,
  // which would read as an answer.
  const ready = computed(
    () =>
      !!selected.value &&
      !!baseline.value &&
      !!bodies.value[selected.value.id] &&
      !!bodies.value[baseline.value.id],
  )
  const rows = computed(() => {
    if (!ready.value) return []
    // Older on the left: the diff reads "what changed to get from that version to
    // this one", and a journal is walked backwards in time.
    const [older, newer] =
      selected.value.revision <= baseline.value.revision
        ? [selected.value, baseline.value]
        : [baseline.value, selected.value]
    return diffDocs(bodies.value[older.id], bodies.value[newer.id])
  })
  const summary = computed(() => diffSummary(rows.value))

  /** Points the composable at a document and loads its journal. */
  async function open(id) {
    docId = id || ''
    versions.value = []
    bodies.value = {}
    selectedId.value = ''
    compareId.value = ''
    error.value = ''
    if (docId) await load()
  }

  function close() {
    docId = ''
    versions.value = []
    bodies.value = {}
    selectedId.value = ''
    compareId.value = ''
    error.value = ''
  }

  async function load() {
    if (!docId) return
    loading.value = true
    try {
      const res = await docsApi.versions(docId)
      versions.value = res.data || []
      error.value = ''
      // The newest entry is the live session and its body changes as people
      // type, so a cached copy of it would show the comparison as it was some
      // minutes ago and call it current.
      const live = versions.value[0]
      if (live) delete bodies.value[live.id]
      if (selectedId.value && !versions.value.some((v) => v.id === selectedId.value)) {
        selectedId.value = ''
      }
    } catch (e) {
      error.value = e.message || 'Не удалось загрузить историю'
    } finally {
      loading.value = false
    }
  }

  /** Fetches a version's body once, caching it. */
  async function body(id) {
    if (!id) return null
    if (bodies.value[id]) return bodies.value[id]
    const res = await docsApi.version(id)
    bodies.value = { ...bodies.value, [id]: res.data?.content || null }
    return bodies.value[id]
  }

  /** Opens a version for comparison, loading both sides. */
  async function select(id) {
    selectedId.value = id === selectedId.value ? '' : id
    if (!selectedId.value) return
    try {
      await Promise.all([body(selectedId.value), body(baseline.value?.id)])
      error.value = ''
    } catch (e) {
      error.value = e.message || 'Не удалось загрузить версию'
    }
  }

  /** Sets the version the selection is compared against ('' = the newest). */
  async function compareWith(id) {
    compareId.value = id || ''
    if (baseline.value) await body(baseline.value.id)
  }

  /**
   * Takes a named snapshot of the document as it stands.
   * @param {string} label free-text name, may be empty
   */
  async function snapshot(label = '') {
    if (!docId) return null
    const res = await docsApi.snapshot(docId, label.trim())
    await load()
    return res.data
  }

  /**
   * Rolls the document back to a version. The caller reloads the editor from the
   * document the server returns — the restore writes a new updated_at, and an
   * editor still holding the old one would answer the user's next keystroke with
   * a conflict.
   */
  async function restore(id) {
    if (!id) return null
    const res = await docsApi.restoreVersion(id)
    selectedId.value = ''
    await load()
    return res.data
  }

  return {
    versions,
    loading,
    error,
    selectedId,
    compareId,
    selected,
    baseline,
    ready,
    rows,
    summary,
    open,
    close,
    load,
    select,
    compareWith,
    snapshot,
    restore,
  }
}
