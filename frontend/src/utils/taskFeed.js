// Formatting for the task's activity feed — comment timestamps and journal lines.
// Extracted from TaskModal so the comments and history tabs share one implementation
// (and so the journal wording is unit-testable without mounting the modal).
import { i18n } from '@/i18n'
import { PRIORITY_KEYS, priorityLabel } from '@/utils/priority'
import { defaultFormatters } from '@/utils/format'
import { columnName } from '@/utils/defaultNames'
import { sourceMeta } from '@/utils/sources'
import { codeChangeText } from '@/utils/codeLinks'

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
// `attachment`/`attachment_removed` only name the destination/file when the
// journal recorded one.
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
    case 'start':
      return d.set ? t('task.journal.startSet') : t('task.journal.startCleared')
    case 'estimate':
      return d.set ? t('task.journal.estimateSet') : t('task.journal.estimateCleared')
    case 'completed':
      return t('task.journal.completed')
    case 'reopened':
      return t('task.journal.reopened')
    case 'recurred':
      return t('task.journal.recurred')
    case 'moved':
      // A column seeded with the board is captioned from its name_key (#2800),
      // the same as everywhere else the column is named; a column the user named
      // has no key and reads verbatim. Entries older than the key fall back to
      // the recorded string.
      return d.to
        ? t('task.journal.movedTo', {
            to: columnName({ name: d.to, name_key: d.to_key }, { t, te: i18n.global.te }),
          })
        : t('task.journal.moved')
    case 'assigned':
      return t('task.journal.assigned')
    case 'unassigned':
      return t('task.journal.unassigned')
    // The journal stores the tag's name, not its id — a renamed or deleted tag
    // must not turn old history into a dangling reference. An entry that carries
    // no name (a tag whose name was empty) reads as the bare phrase rather than
    // one with a hole where the name should be.
    case 'tagged':
      return d.tag ? t('task.journal.taggedWith', { tag: d.tag }) : t('task.journal.tagged')
    case 'untagged':
      return d.tag ? t('task.journal.untaggedWith', { tag: d.tag }) : t('task.journal.untagged')
    case 'archived':
      return t('task.journal.archived')
    case 'restored':
      return t('task.journal.restored')
    // The trash (#3004) — not the archive: a delete and its undo.
    case 'trashed':
      return t('task.journal.trashed')
    case 'untrashed':
      return t('task.journal.untrashed')
    case 'comment':
      return t('task.journal.comment')
    case 'relation':
      return t('task.journal.relation', { ref: d.related ?? '' })
    case 'relation_removed':
      return t('task.journal.relationRemoved', { ref: d.related ?? '' })
    // One aggregated entry per comment (a five-command comment must not bury the
    // rest of the history), so the line names the commands instead of saying
    // "ran quick actions" — otherwise a silent /close reads like nothing happened.
    case 'commands': {
      const list = (keys) => (Array.isArray(keys) ? keys : []).map((k) => `/${k}`).join(', ')
      const applied = list(d.applied)
      const failed = list(d.failed)
      if (applied && failed) return t('task.journal.commandsMixed', { applied, failed })
      if (failed) return t('task.journal.commandsFailed', { failed })
      if (applied) return t('task.journal.commandsApplied', { applied })
      return t('task.journal.commands')
    }
    // Written by the integrations, not by a person. The provider is captioned
    // through the shared source dictionary so the journal doesn't invent a second
    // spelling of a name the relation badges already print.
    case 'synced':
      return t('task.journal.synced', { source: sourceMeta(d.source).label })
    // A merge request appeared on the task, or was merged / closed (#2964). The
    // row's «actor» is the provider (see TaskCommentsTab), the text the event.
    case 'code_change': {
      const { key, params } = codeChangeText(d)
      return t(`task.journal.${key}`, params)
    }
    case 'attachment':
      return d.filename
        ? t('task.journal.attachmentNamed', { name: d.filename })
        : t('task.journal.attachment')
    case 'attachment_removed':
      return d.filename
        ? t('task.journal.attachmentRemovedNamed', { name: d.filename })
        : t('task.journal.attachmentRemoved')
    // Like the tag, the stage is recorded by name: it may be renamed or deleted
    // (the task's milestone_id is ON DELETE SET NULL) and the line has to keep
    // reading. An entry with no name — a stage deleted before the unpin was
    // journalled — falls back to the bare phrase.
    case 'milestone':
      return d.title
        ? t('task.journal.milestoneNamed', { name: d.title })
        : t('task.journal.milestone')
    // Учёт времени (#2965): minutes as hours and minutes («1 ч 30 м»).
    case 'time_spent': {
      const m = Math.round(Number(d.minutes) || 0)
      const parts = []
      if (m >= 60) parts.push(t('task.estimate.short.hour', { n: Math.floor(m / 60) }))
      if (m % 60 || !parts.length) parts.push(t('task.estimate.short.minute', { n: m % 60 }))
      return t('task.journal.timeSpent', { v: parts.join(' ') })
    }
    case 'milestone_removed':
      return d.title
        ? t('task.journal.milestoneRemovedNamed', { name: d.title })
        : t('task.journal.milestoneRemoved')
    default:
      return e.kind
  }
}
