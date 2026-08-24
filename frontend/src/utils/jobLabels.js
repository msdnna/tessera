// Names the background-jobs panel's rows in the reader's language (доработка 3 of
// #2800).
//
// The server sends each job's name and current operation twice: as a catalog key —
// `name_key` (+ `name_arg` for the part that is data, e.g. a GitLab integration
// label) and `current_op_key` — and as the Russian string it used to send alone.
// We render the key here and keep that string as the fallback, so a job whose key
// this bundle doesn't know yet reads as a phrase instead of a bare key. Same shape
// as notificationText (#2801).
import { i18n } from '@/i18n'

// A worker's registry key doubles as its name key (the roster is fixed, see
// handlers/jobs.go), so worker names live under a different branch of the catalog
// than the composite name of a discrete run.
const namePath = (job) =>
  job.kind === 'worker' ? `jobs.worker.${job.name_key}` : `jobs.name.${job.name_key}`

export function jobName(job, { t = i18n.global.t, te = i18n.global.te } = {}) {
  if (!job) return ''
  const path = namePath(job)
  if (job.name_key && te(path)) return t(path, { arg: job.name_arg || '' })
  return job.name || ''
}

export function jobOpText(job, { t = i18n.global.t, te = i18n.global.te } = {}) {
  if (!job) return ''
  const path = `jobs.op.${job.current_op_key}`
  if (job.current_op_key && te(path)) return t(path)
  return job.current_op || ''
}
