// Background jobs — the pure part of the jobs window and the export/import
// flows (#3067, #3068). The server sends a job as the registry entry
// (internal/jobs Entry) or as a history row of the same shape, and its log as
// lines numbered by `seq` within the run.

// LOG_KEEP is how many lines the client holds per run (the server's ring is
// 1000; the history keeps the last 200).
export const LOG_KEEP = 1000

// A job's stable identity on the client: its run id when it has one (every
// discrete run), else its registry key (the tick-loop workers). A live run and
// its history row share the run id, so a run that finishes keeps its place.
export const jobUid = (j) => j?.run_id || j?.key || ''

// Statuses that end a run.
export const FINAL = ['done', 'failed', 'cancelled']
export const isFinal = (j) => FINAL.includes(j?.status)

// The window's status filters: «Завершены» takes a stopped run too.
export function statusGroup(j) {
  if (j?.status === 'cancelled') return 'done'
  return j?.status || ''
}

// mergeLines appends the lines past the last seq already held — a line that
// arrives twice (two sockets, or a poll racing an event) is recognised — and
// keeps the tail.
export function mergeLines(have = [], add = [], keep = LOG_KEEP) {
  const last = have.length ? have[have.length - 1].seq : 0
  const fresh = (add || []).filter((l) => l && l.seq > last)
  if (!fresh.length) return have
  const out = [...have, ...fresh]
  return out.length > keep ? out.slice(out.length - keep) : out
}

// unionLines merges a backlog read from the API with what the socket already
// brought: by seq, in order, the tail kept.
export function unionLines(a = [], b = [], keep = LOG_KEEP) {
  const bySeq = new Map()
  for (const l of [...(a || []), ...(b || [])]) if (l) bySeq.set(l.seq, l)
  const out = [...bySeq.values()].sort((x, y) => x.seq - y.seq)
  return out.length > keep ? out.slice(out.length - keep) : out
}

// progressPct: 0–100, or null when the run reports no total.
export function progressPct(j) {
  const total = j?.progress_total || 0
  if (total <= 0) return null
  return Math.max(0, Math.min(100, Math.round(((j.progress_done || 0) / total) * 100)))
}

// mergeJobs lays the live list over the loaded one: the same run (by uid)
// takes the newer state, a new one goes on top.
export function upsertJob(list, job) {
  const uid = jobUid(job)
  const i = list.findIndex((x) => jobUid(x) === uid)
  if (i < 0) return [job, ...list]
  const out = list.slice()
  out[i] = { ...list[i], ...job }
  return out
}

// logText renders lines for the clipboard: «14:31:56 INFO text».
export function logText(lines, fmtTime = (iso) => iso) {
  return (lines || [])
    .map((l) => `${fmtTime(l.at)} ${String(l.level || '').toUpperCase()} ${l.text}`)
    .join('\n')
}
