import { defineStore } from 'pinia'
import { ref } from 'vue'
import { jobUid, mergeLines, unionLines } from '@/utils/jobs'

// Live background jobs from the socket (#3067): every `job.updated` event —
// `{ job, lines }` — lands here, keyed by the run. The jobs window reads it for
// the live list and the open job's log; an export or an import waits on its own
// run through it (the server sends a member's own runs to them, and every run to
// the administrators).
export const useJobsStore = defineStore('jobs', () => {
  // uid → { job, lines }
  const runs = ref({})
  // Bumped on every update, for watchers that need "something changed".
  const tick = ref(0)

  function apply(job, lines = []) {
    const uid = jobUid(job)
    if (!uid) return
    const prev = runs.value[uid]
    runs.value = {
      ...runs.value,
      [uid]: { job: { ...(prev?.job || {}), ...job }, lines: mergeLines(prev?.lines || [], lines) },
    }
    tick.value++
  }

  function onEvent(ev) {
    if (ev?.type !== 'job.updated' || !ev.data?.job) return
    apply(ev.data.job, ev.data.lines)
  }

  const get = (uid) => runs.value[uid] || null

  // Lines read from the API (the open job's backlog) go in the same buffer.
  function addLines(uid, lines) {
    const prev = runs.value[uid]
    runs.value = {
      ...runs.value,
      [uid]: { job: prev?.job || { run_id: uid }, lines: unionLines(prev?.lines || [], lines) },
    }
  }

  function reset() {
    runs.value = {}
  }

  return { runs, tick, apply, onEvent, get, addLines, reset }
})
