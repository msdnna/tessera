import { computed, ref } from 'vue'
import { documents as docsApi } from '@/api'
import { i18n } from '@/i18n'
import {
  blockIdsInOrder,
  buildThreads,
  openCountByBlock,
  sortThreads,
  splitThreads,
} from '@/utils/docComments'

/**
 * Annotation threads for one open document (#2730).
 *
 * The list is refetched rather than patched in place after every write. The same
 * document is open for several people at once, so a locally spliced list is
 * stale the moment a colleague replies — and the panel is a handful of rows, not
 * a feed. The document socket nudges everyone to call `load()` again (#2730),
 * which is also what makes a nudge lost to a reconnect harmless.
 *
 * @returns {object} state and actions
 */
export function useDocComments() {
  const comments = ref([])
  const loading = ref(false)
  const error = ref('')
  // The thread the panel highlights, set by clicking a discussed block.
  const activeBlockId = ref('')
  // The document JSON the anchors are matched against, so a thread whose block
  // was deleted can be shown as detached instead of vanishing.
  const docJSON = ref(null)

  let docId = ''

  const threads = computed(() => sortThreads(buildThreads(comments.value)))
  // Document order, not just membership: the panel's card order decides whether
  // the annotation lines cross (#2730).
  const groups = computed(() => splitThreads(threads.value, blockIdsInOrder(docJSON.value)))
  const openCounts = computed(() =>
    [...openCountByBlock(threads.value)].map(([block_id, count]) => ({ block_id, count })),
  )
  const openCount = computed(() => threads.value.filter((t) => !t.resolved_at).length)

  /** Points the composable at a document and loads its threads. */
  async function open(id, json) {
    docId = id || ''
    docJSON.value = json || null
    comments.value = []
    activeBlockId.value = ''
    error.value = ''
    if (docId) await load()
  }

  /** Forgets the current document (nothing to draw while none is open). */
  function close() {
    docId = ''
    comments.value = []
    docJSON.value = null
    activeBlockId.value = ''
    error.value = ''
  }

  /** Keeps anchor matching in step with the document being edited. */
  function setDoc(json) {
    docJSON.value = json || null
  }

  async function load() {
    if (!docId) return
    loading.value = true
    try {
      const res = await docsApi.comments(docId)
      comments.value = res.data || []
      error.value = ''
    } catch (e) {
      error.value = e.message || i18n.global.t('documents.comments.error.load')
    } finally {
      loading.value = false
    }
  }

  /**
   * Starts a thread on a block (or on the document, when blockId is empty).
   * @returns {Promise<boolean>} whether it was created
   */
  async function add({ blockId = '', body, quote = '' }) {
    if (!docId || !body?.trim()) return false
    await docsApi.addComment(docId, { block_id: blockId, body: body.trim(), quote })
    if (blockId) activeBlockId.value = blockId
    await load()
    return true
  }

  /** Answers an existing thread. The server takes the anchor from the root. */
  async function reply(parentId, body) {
    if (!docId || !parentId || !body?.trim()) return false
    await docsApi.addComment(docId, { parent_id: parentId, body: body.trim() })
    await load()
    return true
  }

  async function edit(commentId, body) {
    if (!body?.trim()) return false
    await docsApi.updateComment(commentId, body.trim())
    await load()
    return true
  }

  async function resolve(commentId, resolved = true) {
    await docsApi.resolveComment(commentId, resolved)
    await load()
  }

  async function remove(commentId) {
    await docsApi.removeComment(commentId)
    await load()
  }

  return {
    comments,
    threads,
    groups,
    openCounts,
    openCount,
    loading,
    error,
    activeBlockId,
    open,
    close,
    setDoc,
    load,
    add,
    reply,
    edit,
    resolve,
    remove,
  }
}
