// Dropping a card into the tag grouping's «Без тегов» lane used to do nothing (#2932).
//
// `planColDrop`'s tag rules knew only the lane's OWN tag — add it on `added`, take it
// off on `removed` — and the catch-all lane has none (`dcol.tag` is null, see
// displayColumns). So the drop produced an empty intent list while the lane stayed a
// live drop target like every other one: the card moved on screen, no request went
// out, and the debounced reload rebuilt the lists ~200 ms later and put it back. The
// same drop one grouping over persists — milestone's «Без этапа» clears the milestone.
//
// The fix reads the lane for what it means: the card must end up carrying none of the
// tags that form lanes. That is checked here as a round trip rather than as a list of
// intents (ut-boardDnd pins those) — plan the drop, apply what it asks the API for,
// rebuild the lanes, and see where the card actually stands. A rule that removed the
// wrong tags, or too few, lands the card back in a tag lane and fails the same way the
// reload used to.
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { planColDrop } from '@/utils/boardDnd'
import { buildLists, NO_LANE } from '@/utils/boardLists'

const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

const RED = { id: 'tagRed', name: 'сложность::высокая' }
const BLUE = { id: 'tagBlue', name: 'сложность::низкая' }
const AREA = { id: 'tagArea', name: 'область::бэкенд' }

// The catch-all lane as the board builds it: same shape as displayColumns' last entry.
const noTagsLane = { key: NO_LANE, name: 'Без тегов', tag: null }

// Apply what the planner asked the API for, the way the backend would.
function applyIntents(card, intents) {
  let tagIds = [...(card.tag_ids || [])]
  for (const it of intents) {
    if (it.op === 'addTag') tagIds = [...new Set([...tagIds, it.tagId])]
    else if (it.op === 'removeTag') tagIds = tagIds.filter((id) => id !== it.tagId)
    else throw new Error(`unexpected op ${it.op}`)
  }
  return { ...card, tag_ids: tagIds }
}

function laneOf(card, groupTags) {
  const lists = buildLists({ groupMode: 'tag', groupTags, tasks: [card] })
  return Object.entries(lists)
    .filter(([, cards]) => cards.length)
    .map(([key]) => key)
}

describe('a card dropped into «Без тегов» stays there (#2932)', () => {
  it('lands in the catch-all lane it was dropped on', () => {
    const groupTags = [RED, BLUE]
    const card = { id: 't1', tag_ids: ['tagRed'] }
    expect(laneOf(card, groupTags)).toEqual(['tagRed']) // where it came from

    const intents = planColDrop({
      groupMode: 'tag',
      evt: { added: { element: card } },
      dcol: noTagsLane,
      groupTags,
    })
    expect(laneOf(applyIntents(card, intents), groupTags)).toEqual([NO_LANE])
  })

  it('does it for a card standing in several tag lanes at once', () => {
    // Tag grouping draws such a card in every one of its lanes; leaving any tag on
    // would leave a copy of it behind in that lane.
    const groupTags = [RED, BLUE]
    const card = { id: 't1', tag_ids: ['tagRed', 'tagBlue'] }
    const intents = planColDrop({
      groupMode: 'tag',
      evt: { added: { element: card } },
      dcol: noTagsLane,
      groupTags,
    })
    expect(laneOf(applyIntents(card, intents), groupTags)).toEqual([NO_LANE])
  })

  it('keeps the tags this grouping does not stand for', () => {
    // Narrowed to «Тег · Сложность»: only сложность:: tags are lanes, so the drop
    // means "no сложность:: tag" and must not quietly strip the card's область:: one.
    const groupTags = [RED, BLUE]
    const card = { id: 't1', tag_ids: ['tagRed', AREA.id] }
    const intents = planColDrop({
      groupMode: 'tag',
      evt: { added: { element: card } },
      dcol: noTagsLane,
      groupTags,
    })
    const after = applyIntents(card, intents)
    expect(after.tag_ids).toEqual([AREA.id])
    expect(laneOf(after, groupTags)).toEqual([NO_LANE])
  })

  it('asks for nothing when the card carries no lane tag already', () => {
    // Re-ordering inside the catch-all is not a thing (canSortWithinLane), but a card
    // can arrive here from the SAME lane after a rejected drag; no request either way.
    const card = { id: 't1', tag_ids: [AREA.id] }
    expect(
      planColDrop({
        groupMode: 'tag',
        evt: { added: { element: card } },
        dcol: noTagsLane,
        groupTags: [RED, BLUE],
      }),
    ).toEqual([])
  })

  it('leaves the drop a no-op when the lane set is unknown', () => {
    // Defensive: no grouping tags handed in means nothing is known to be a lane, and
    // guessing would strip tags the board never drew as lanes.
    const card = { id: 't1', tag_ids: ['tagRed'] }
    expect(
      planColDrop({ groupMode: 'tag', evt: { added: { element: card } }, dcol: noTagsLane }),
    ).toEqual([])
  })
})

describe('the board hands the planner the lanes it drew', () => {
  it('passes groupTags into planColDrop', () => {
    // Without this the rule above degrades to the defensive no-op — green unit tests,
    // dead feature on the board.
    expect(board).toMatch(/planColDrop\(\{[\s\S]*?groupTags:\s*groupTags\.value/)
  })

  it('dispatches removeTag intents as API calls', () => {
    expect(board).toContain("it.op === 'removeTag'")
    expect(board).toMatch(/tasksApi\.removeTag\(it\.id,\s*it\.tagId\)/)
  })

  it('and the catch-all lane is a real column in the tag grouping', () => {
    // `key: '__none__'` with a null tag — the lane whose drop this is about.
    expect(board).toMatch(/key:\s*'__none__',[\s\S]{0,200}?tag:\s*null/)
  })
})
