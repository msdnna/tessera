import { ref } from 'vue'

/**
 * Autosave for the document editor.
 *
 * There is no prior art in the app to copy — notes save on a button, the task
 * modal on an explicit apply — so the mechanics are spelled out here. The
 * debounce is the easy half; the parts that actually matter are:
 *
 *  • **flush**, so the last edit is not lost when the view unmounts, the route
 *    changes or the tab closes. Losing the final keystrokes is the one bug that
 *    destroys trust in autosave entirely;
 *  • **serialisation**, because two PATCHes in flight land in network order —
 *    the winner would be the fastest request, not the newest edit;
 *  • **stopping on 409**, because a conflict means someone else's version is on
 *    the server and retrying would overwrite it.
 *
 * @param {(content: object) => Promise<{updated_at?: string}>} saveFn performs the write
 * @param {object} [opts]
 * @param {number} [opts.delay] debounce window in ms
 * @returns {object} state refs + `schedule`, `flush`, `resolveConflict`, `cancel`
 */
export function useDocAutosave(saveFn, opts = {}) {
  const delay = opts.delay ?? 800

  const saving = ref(false)
  const dirty = ref(false)
  const conflict = ref(false)
  const error = ref('')
  const savedAt = ref(null)

  let timer = null
  let pending = null
  let running = null

  function schedule(content) {
    if (conflict.value) return
    pending = content
    dirty.value = true
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      timer = null
      run()
    }, delay)
  }

  async function run() {
    if (running) return running
    if (pending === null || conflict.value) return
    running = (async () => {
      // Loop rather than recurse: edits made while a request is in flight are
      // picked up by the next turn, and the caller awaiting flush() waits for
      // all of them, not just the first.
      while (pending !== null && !conflict.value) {
        const content = pending
        pending = null
        saving.value = true
        try {
          const res = await saveFn(content)
          savedAt.value = res?.updated_at ?? savedAt.value
          error.value = ''
          if (pending === null) dirty.value = false
        } catch (e) {
          if (e?.status === 409) {
            conflict.value = true
          } else {
            // Keep the newest content queued so the next flush retries it —
            // unless newer content already arrived while we were failing.
            if (pending === null) pending = content
            error.value = e?.message || 'Не удалось сохранить'
          }
          break
        } finally {
          saving.value = false
        }
      }
    })()
    try {
      await running
    } finally {
      running = null
    }
  }

  /** Sends whatever is queued right now and resolves when the queue is empty. */
  async function flush() {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
    await run()
  }

  /** Drops queued content without sending it (used after a hard reload). */
  function cancel() {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
    pending = null
    dirty.value = false
  }

  /** Clears the conflict flag after the caller reloaded the server's version. */
  function resolveConflict(updatedAt) {
    conflict.value = false
    error.value = ''
    pending = null
    dirty.value = false
    savedAt.value = updatedAt ?? savedAt.value
  }

  return { saving, dirty, conflict, error, savedAt, schedule, flush, cancel, resolveConflict }
}
