// Who a comment outside a task mentions (#3048): a document's comment and a
// meeting's chat line are plain text, and the server notifies the members whose
// ids it is handed — the same contract as a task comment (`mentions`). The ids
// are read the way the task composer reads them (utils/mentions mentionIdsIn):
// an @-handle that resolves against the workspace roster is a mention.
//
// The roster is fetched once per workspace and kept for a minute — a burst of
// chat lines must not cost a members request each.
import { workspaces as wsApi } from '@/api'
import { buildMentionItems, mentionIdsIn } from '@/utils/mentions'

const TTL = 60 * 1000
const cache = new Map() // wsId → { at, items: Promise<rows> }

function rosterOf(wsId) {
  const hit = cache.get(wsId)
  if (hit && Date.now() - hit.at < TTL) return hit.items
  const items = wsApi
    .members(wsId)
    .then((r) => buildMentionItems(r.data || []))
    .catch(() => {
      cache.delete(wsId)
      return []
    })
  cache.set(wsId, { at: Date.now(), items })
  return items
}

// The member ids `body` mentions, in order, without repeats; [] when it
// mentions nobody (or the roster cannot be read — the comment still goes).
export async function mentionIdsFor(wsId, body) {
  if (!wsId || !body || !body.includes('@')) return []
  return mentionIdsIn(body, await rosterOf(wsId))
}

// The ids an edit ADDED — the server notifies only those, so fixing a typo does
// not ring the same people again.
export async function addedMentionIds(wsId, before, after) {
  const was = new Set(await mentionIdsFor(wsId, before))
  return (await mentionIdsFor(wsId, after)).filter((id) => !was.has(id))
}

export function resetMentionRoster() {
  cache.clear()
}
