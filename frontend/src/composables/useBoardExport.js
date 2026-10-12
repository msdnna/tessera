import { useI18n } from 'vue-i18n'
import { boards as boardsApi, jobs as jobsApi } from '@/api'
import { useMessage } from '@/composables/useMessage'
import { downloadJobFile } from '@/composables/useJobFile'
import { useJobsStore } from '@/stores/jobs'
import { humanizeError } from '@/utils/errors'
import { isFinal } from '@/utils/jobs'

// How often a run is re-read while the socket stays quiet (a missed event, a
// reconnect) — the socket usually answers first.
const POLL_MS = 1500

// waitForJob resolves with the run once it is over: on the socket's update
// (stores/jobs) or on a poll, whichever comes first.
export function waitForJob(runId, { store, poll = POLL_MS } = {}) {
  return new Promise((resolve, reject) => {
    let timer = 0
    let stopWatch = null
    const finish = (job) => {
      clearTimeout(timer)
      stopWatch?.()
      resolve(job)
    }
    const check = () => {
      const live = store?.get(runId)?.job
      if (live && isFinal(live)) finish(live)
      return !!(live && isFinal(live))
    }
    if (store) {
      stopWatch = store.$subscribe(() => check())
    }
    const tick = async () => {
      if (check()) return
      try {
        const { data } = await jobsApi.get(runId)
        if (isFinal(data)) return finish(data)
      } catch (e) {
        clearTimeout(timer)
        stopWatch?.()
        return reject(e)
      }
      timer = setTimeout(tick, poll)
    }
    timer = setTimeout(tick, 300)
  })
}

// useBoardExport runs an export (#3094): queue the job, wait for it in a loading
// toast, hand the file over. taskIds — the cards the current view shows; null
// exports the whole board.
export function useBoardExport() {
  const { t } = useI18n()
  const message = useMessage()
  const store = useJobsStore()

  async function runExport(boardId, format, taskIds = null) {
    const busy = message.loading(t('board.transfer.exporting', { format: format.toUpperCase() }), {
      line: taskIds ? t('board.transfer.exportFiltered', { n: taskIds.length }) : '',
    })
    try {
      const body = { format }
      if (taskIds) body.task_ids = taskIds
      const { data } = await boardsApi.exportTasks(boardId, body)
      const job = await waitForJob(data.job.run_id, { store })
      busy.destroy()
      if (job.status !== 'done') {
        message.error(job.error || t('board.transfer.exportFailed'), {
          retry: () => runExport(boardId, format, taskIds),
        })
        return null
      }
      await downloadJobFile(job)
      return job
    } catch (e) {
      busy.destroy()
      message.error(humanizeError(e), { retry: () => runExport(boardId, format, taskIds) })
      return null
    }
  }

  return { runExport }
}
