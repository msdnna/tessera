// Formatting for the task's activity feed — comment timestamps and journal lines.
// Extracted from TaskModal so the comments and history tabs share one implementation
// (and so the journal wording is unit-testable without mounting the modal).
import { i18n } from '@/i18n'
import { PRIORITY_KEYS, priorityLabel } from '@/utils/priority'
import { defaultFormatters } from '@/utils/format'

// Short "12 янв., 14:03" stamp used on comment and journal rows. `fmt` is a
// formatter set from useFormat() — it carries the language, the timezone and the
// 12/24h preference; without one the defaults apply (tests).
export function fmtWhen(d, fmt = defaultFormatters()) {
  return fmt.formatDateTime(d, { day: '2-digit', month: 'short' })
}

// Group the flat comment list the API returns into threads: a root plus the
// replies hanging off it. The server sends a flat array (so Android and the MCP
// server keep working) and the tree is assembled here.
//
// A reply whose parent is missing from the list — deleted, or filtered out —
// is promoted to a root of its own rather than dropped: losing someone's text
// because its parent went away is worse than showing it unindented.
export function groupThreads(comments) {
  const list = comments || []
  const byId = new Map(list.map((c) => [c.id, c]))
  const threads = []
  const byRoot = new Map()
  for (const c of list) {
    const parentId = c.parent_id && byId.has(c.parent_id) ? c.parent_id : null
    if (!parentId) {
      const t = { root: c, replies: [] }
      threads.push(t)
      byRoot.set(c.id, t)
      continue
    }
    const t = byRoot.get(parentId)
    if (t) t.replies.push(c)
    // Parent exists but its thread hasn't been seen yet (an out-of-order list):
    // fall back to a root so the comment stays visible.
    else threads.push({ root: c, replies: [] })
  }
  return threads
}

// Human sentence for a journal event, appended after the actor's name.
//
// Called from render code but living outside a setup context, so the catalog is
// reached through `i18n.global.t`. The lookup is per call — a component that
// renders the feed re-runs this when the language flips (#2799).
//
// Two events come in a "with target" and a "bare" flavour: `moved` and
// `attachment` only name the destination/file when the journal recorded one.
// That is two separate keys rather than an interpolated suffix, because the
// wording around the name is not the same in every language.
export function eventText(e) {
  const t = i18n.global.t
  const d = e.data || {}
  switch (e.kind) {
    case 'created':
      return t('task.journal.created')
    case 'renamed':
      return t('task.journal.renamed', { to: d.to ?? '' })
    case 'description':
      return t('task.journal.description')
    // An out-of-range level keeps printing the raw value: a number the user can
    // quote back is more useful than silently calling it "no priority".
    case 'priority':
      return t('task.journal.priority', {
        to: PRIORITY_KEYS[Number(d.to)] === undefined ? d.to : priorityLabel(d.to),
      })
    case 'due':
      return d.set ? t('task.journal.dueSet') : t('task.journal.dueCleared')
    case 'completed':
      return t('task.journal.completed')
    case 'reopened':
      return t('task.journal.reopened')
    case 'recurred':
      return t('task.journal.recurred')
    case 'moved':
      return d.to ? t('task.journal.movedTo', { to: d.to }) : t('task.journal.moved')
    case 'assigned':
      return t('task.journal.assigned')
    case 'unassigned':
      return t('task.journal.unassigned')
    case 'archived':
      return t('task.journal.archived')
    case 'restored':
      return t('task.journal.restored')
    case 'comment':
      return t('task.journal.comment')
    case 'relation':
      return t('task.journal.relation', { ref: d.related ?? '' })
    case 'attachment':
      return d.filename
        ? t('task.journal.attachmentNamed', { name: d.filename })
        : t('task.journal.attachment')
    default:
      return e.kind
  }
}
