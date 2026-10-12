// Code changes linked to a task (#2964) — today GitLab merge requests — as the
// task window's «Разработка» draws them. The API serves them provider-neutral
// (`code_links` on GET /tasks/:id, the `task.code_links` socket event): a state
// (open | merged | closed) with a draft flag, and the status of the latest CI run
// ('' | success | running | failed | canceled | manual). Pure — unit-tested in
// tests/codeLinks.spec.js.

// Review state → the pill: an i18n key under task.dev.state and its hue. A draft
// is an open change that is not ready yet — a neutral (flat, grey) pill, per the
// design language: only non-neutral things carry a hue.
const STATE_TONES = {
  open: '#18a058',
  merged: '#1f75cb',
  closed: '#d03050',
}

export function changeStatus(link) {
  const state = link?.state || 'open'
  if (state === 'open' && link?.draft) return { key: 'draft', color: null }
  return { key: STATE_TONES[state] ? state : 'open', color: STATE_TONES[state] || STATE_TONES.open }
}

// CI status → the line under the title: an i18n key under task.dev.ci, an icon
// name (resolved to an ionicon by the component) and its colour. No pipeline —
// null, and the line shows only the branches.
const CI = {
  success: { icon: 'success', color: '#18a058' },
  running: { icon: 'running', color: '#2f80ed' },
  failed: { icon: 'failed', color: '#d03050' },
  canceled: { icon: 'canceled', color: 'var(--t-text3)' },
  manual: { icon: 'manual', color: '#f0a020' },
}

export function ciMeta(status) {
  const m = CI[status]
  return m ? { key: status, ...m } : null
}

// «feat/board-v2 → main»; a side GitLab didn't report is left out.
export function branchLine(link) {
  const from = link?.source_branch || ''
  const to = link?.target_branch || ''
  if (from && to) return `${from} → ${to}`
  return from || to
}

// How the change is named: the server's `ref` — «!418» inside the bound GitLab
// project, «group/proj!12» for an MR of another project — or, from an older
// server, the bare number.
export function changeRef(link) {
  return link?.ref || `!${link?.number ?? ''}`
}

// Same order as the server: open changes first, then the most recently active.
export function sortCodeLinks(links) {
  return [...(links || [])].sort((a, b) => {
    const ao = a.state === 'open' ? 0 : 1
    const bo = b.state === 'open' ? 0 : 1
    if (ao !== bo) return ao - bo
    const at = a.updated_at || ''
    const bt = b.updated_at || ''
    if (at !== bt) return at < bt ? 1 : -1
    return (b.number || 0) - (a.number || 0)
  })
}

// A socket event → { taskId, links } when it is the «Разработка» of a task
// changing, else null.
export function codeLinksPatch(ev) {
  if (ev?.type !== 'task.code_links') return null
  const taskId = ev.data?.task_id
  if (!taskId) return null
  const links = Array.isArray(ev.data.code_links) ? ev.data.code_links : []
  return { taskId, links }
}

// The history line for a `code_change` journal entry: i18n key under
// task.journal and its params. «открыт !418 в feat/board-v2», «смержен !418».
export function codeChangeText(data) {
  const d = data || {}
  const state = ['open', 'merged', 'closed'].includes(d.state) ? d.state : 'open'
  const ref = changeRef(d)
  if (state === 'open' && d.branch)
    return { key: 'codeOpenedOn', params: { ref, branch: d.branch } }
  const key = { open: 'codeOpened', merged: 'codeMerged', closed: 'codeClosed' }[state]
  return { key, params: { ref } }
}
