// Re-ordering a card inside a tag/milestone lane used to be a phantom gesture (#2932).
//
// `planColDrop` has a rule for `added` and one for `removed` in those groupings —
// set the milestone, add/remove the tag — and none at all for a bare `moved`, because
// there is no move to persist: `position` orders a card within its COLUMN, and a
// tag/milestone lane collects cards out of every column of the board, so the cards a
// dropped card lands between there are not its neighbours in that order.
//
// The card list stayed sortable in every grouping anyway (`:disabled` only guarded the
// archive). So Sortable re-ordered the lane on drop, `onColChange` dispatched an empty
// intent list, and the `scheduleReload()` it ends with rebuilt the lists ~200 ms later
// and put the card back where it started — no request, no error, no explanation, while
// the identical drag one grouping over (status) persisted. The fix hands `:sort` the
// same answer the planner gives, so the lane simply doesn't re-order.
//
// Two halves are checked here, because either alone can be green while the feature is
// broken: that vuedraggable really forwards `sort` down to SortableJS (it is not a
// declared prop — an unknown attribute could just as easily land on the DOM node), and
// that the board's card list is the one that binds it.
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { mount } from '@vue/test-utils'
import draggable from 'vuedraggable'
import { canSortWithinLane } from '@/utils/boardDnd'

const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

// The `<draggable …>` opening tags, quote-aware: attribute values here contain `>`
// inside expressions, which a lazy `[^>]*` regex would cut in half (same scanner as
// cx-dnd-touch.spec.js).
function openingTags(source) {
  const tags = []
  const needle = '<draggable'
  let i = source.indexOf(needle)
  while (i !== -1) {
    let quote = null
    for (let j = i + needle.length; j < source.length; j++) {
      const ch = source[j]
      if (quote) {
        if (ch === quote) quote = null
      } else if (ch === '"' || ch === "'") {
        quote = ch
      } else if (ch === '>') {
        tags.push(source.slice(i, j + 1))
        break
      }
    }
    i = source.indexOf(needle, i + needle.length)
  }
  return tags
}

// The board's card list: the one bound to a column's bucket, with the shared
// "tasks" group that makes every lane a drop target for every other.
const cardList = openingTags(board).find(
  (tag) => tag.includes('group="tasks"') && tag.includes(':list="lists[dcol.key]"'),
)

function mountList(sort) {
  return mount(draggable, {
    props: { list: [{ id: 'a' }, { id: 'b' }], itemKey: 'id', sort },
    slots: { item: '<div class="row" />' },
  })
}

describe('vuedraggable forwards `sort` to SortableJS', () => {
  it('passes it through as a Sortable option, not as a DOM attribute', () => {
    const w = mountList(false)
    expect(w.vm._sortable.options.sort).toBe(false)
    expect(w.element.hasAttribute('sort')).toBe(false)
    w.unmount()
  })

  it('leaves the list sortable when it is true', () => {
    const w = mountList(true)
    expect(w.vm._sortable.options.sort).toBe(true)
    w.unmount()
  })

  it('and re-applies it live when the binding flips', async () => {
    // The board flips grouping without remounting the columns, so a one-shot read
    // at mount time would leave the previous grouping's answer in place.
    const w = mountList(true)
    await w.setProps({ sort: false })
    expect(w.vm._sortable.options.sort).toBe(false)
    w.unmount()
  })
})

describe('the board binds it on the card list (#2932)', () => {
  it('finds the card draggable', () => {
    // A sanity floor: a renamed binding would otherwise make every assertion
    // below pass over `undefined`.
    expect(cardList).toBeTruthy()
  })

  it('binds :sort rather than leaving the lane unconditionally sortable', () => {
    expect(cardList).toContain(':sort="canSortCards"')
  })

  it('and derives that from the grouping through the shared rule', () => {
    expect(board).toContain('canSortWithinLane')
    expect(board).toMatch(/canSortCards\s*=\s*computed\(\(\)\s*=>\s*canSortWithinLane\(groupMode\./)
  })

  it('keeps the archive guard and the touch settings it already had', () => {
    // `:sort` governs re-ordering only; the archive must stay undraggable outright.
    expect(cardList).toContain(':disabled="readOnly"')
    expect(cardList).toContain(':touch-start-threshold="6"')
  })

  it('leaves the column-reorder list alone — columns are not cards', () => {
    const colList = openingTags(board).find((tag) => tag.includes(':disabled="!canReorderColumns"'))
    expect(colList).toBeTruthy()
    expect(colList).not.toContain(':sort=')
  })
})

describe('the rule the binding carries', () => {
  it('re-orders inside status columns and nowhere else', () => {
    expect(canSortWithinLane('status')).toBe(true)
    expect(['tag', 'milestone'].map(canSortWithinLane)).toEqual([false, false])
  })
})
