import { computed, ref } from 'vue'
import { documents as docsApi } from '@/api'
import { canRaiseApproval, documentApprovalState } from '@/utils/docApprovals'
import { i18n } from '@/i18n'

/**
 * Task links and approval protocols of one open document (#2732).
 *
 * The two live in one composable because they arrive on one socket nudge and are
 * read as one panel: a protocol is raised against the document, and the tasks it
 * came from are the reason it was. Splitting them would double the refetch on
 * every signature for no gain.
 *
 * Both lists are refetched whole rather than patched from a response. A link is
 * a row joined to a task's title and a protocol is a route joined to its steps —
 * reconstructing either from the mutation's return value means keeping a second
 * copy of the server's join in the client, which is exactly the thing that goes
 * stale silently.
 *
 * @returns {object} state and actions
 */
export function useDocLinks() {
  const links = ref([])
  const approvals = ref([])
  const loading = ref(false)
  const error = ref('')

  let docId = ''
  // Generation counter over the fetches below. The panel is not remounted when
  // the reader opens another document — the documents section owns one editor
  // and re-points it (#2727) — so nothing invalidates an answer that is already
  // in flight when `open()` is called again. Without this, the answer to the
  // document just left lands last and wins: the panel then lists the links of a
  // document nobody has on screen, and — louder — its approval route, so
  // `state` reports "на согласовании" about the wrong document and the sign
  // button carries the other document's approval id into `decide()`.
  let seq = 0

  // The document's single answer to "согласован ли он" — an open route if there
  // is one, else the newest closed one, else null for a document never sent.
  const state = computed(() => documentApprovalState(approvals.value))
  const canRaise = computed(() => canRaiseApproval(approvals.value))
  // Links pinned to a block, grouped by that block: the editor's gutter shows a
  // marker per block, and a lookup per paragraph would otherwise walk the whole
  // list on every render.
  const byBlock = computed(() => {
    const out = {}
    for (const l of links.value) {
      if (!l.block_id) continue
      out[l.block_id] = (out[l.block_id] || 0) + 1
    }
    return out
  })

  /** Points the composable at a document and loads both lists. */
  async function open(id) {
    docId = id || ''
    // Invalidate before clearing, not after: the request in flight was asked of
    // the previous document, and its answer would otherwise refill the lists we
    // are emptying right now.
    seq++
    links.value = []
    approvals.value = []
    error.value = ''
    if (docId) await load()
    else loading.value = false
  }

  function close() {
    docId = ''
    seq++
    links.value = []
    approvals.value = []
    error.value = ''
    // The in-flight answer is disowned above, so it will not clear the spinner
    // on its way out — a panel returning to the grid mid-load would otherwise
    // keep spinning until the next document is opened.
    loading.value = false
  }

  async function load() {
    if (!docId) return
    const mine = ++seq
    // Pinned for the round trip: `docId` is a plain variable the reader can move
    // out from under us between the two calls below.
    const id = docId
    loading.value = true
    try {
      // One await, not two: the panel renders links and protocols together, and
      // sequencing them would show it half-built for a round trip.
      const [l, a] = await Promise.all([docsApi.links(id), docsApi.approvals(id)])
      if (mine !== seq) return // another document is on screen now
      links.value = l.data || []
      approvals.value = a.data || []
      error.value = ''
    } catch (e) {
      if (mine !== seq) return // a flap on a document nobody is reading
      error.value = e.message || i18n.global.t('documents.links.error.load')
    } finally {
      if (mine === seq) loading.value = false
    }
  }

  /**
   * Links a task to the document, or to one block of it.
   * @param {object} p
   * @param {string} p.taskId task to link
   * @param {string} [p.blockId] block anchor ('' links the document as a whole)
   * @param {string} [p.quote] snippet of the anchored block, for when it is rewritten
   */
  async function link({ taskId, blockId = '', quote = '' }) {
    if (!docId || !taskId) return
    await docsApi.linkTask(docId, { task_id: taskId, block_id: blockId, quote })
    await load()
  }

  async function unlink(linkId) {
    if (!linkId) return
    await docsApi.unlinkTask(linkId)
    await load()
  }

  /**
   * Raises an approval route against the document as it stands.
   * @param {object} p
   * @param {string} [p.title] what is being agreed
   * @param {string} [p.mode] 'sequential' (default) or 'parallel'
   * @param {string[]} p.approvers user ids, in the order the route is walked
   */
  async function raise({ title = '', mode = 'sequential', approvers = [] }) {
    if (!docId || !approvers.length) return null
    const res = await docsApi.createApproval(docId, { title: title.trim(), mode, approvers })
    await load()
    return res.data
  }

  /**
   * Records the caller's signature. The server decides whether it is their turn;
   * the panel only avoids offering the button when it already knows it is not.
   */
  async function decide(approvalId, { decision, comment = '', signature = '' }) {
    if (!approvalId) return null
    const res = await docsApi.decideApproval(approvalId, { decision, comment, signature })
    await load()
    return res.data
  }

  async function cancel(approvalId) {
    if (!approvalId) return null
    const res = await docsApi.cancelApproval(approvalId)
    await load()
    return res.data
  }

  return {
    links,
    approvals,
    loading,
    error,
    state,
    canRaise,
    byBlock,
    open,
    close,
    load,
    link,
    unlink,
    raise,
    decide,
    cancel,
  }
}
