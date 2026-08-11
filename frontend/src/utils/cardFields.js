// Which fields a board card shows: the card-size preset composes the set, the
// per-field customize-view toggles refine it inside that set.
//
// Both rules live here rather than inline in the card because the card and its
// extracted pill row both ask the question. Two copies of this predicate is the
// quiet regression this kind of split invites — a field that shows on `medium`
// in one component and hides in the other, with nothing failing loudly.

// compact shows only the title; medium a curated subset; large everything.
export const SIZE_FIELDS = {
  compact: [],
  medium: ['number', 'priority', 'due', 'tags', 'assignee'],
  large: null, // null = all fields allowed
}

// An unknown size behaves like `large` (everything allowed): a saved view from
// an older build must never blank a card.
export function sizeAllows(size, key) {
  const set = SIZE_FIELDS[size] ?? null
  return set === null ? true : set.includes(key)
}

// A field is enabled unless its customize toggle is explicitly false (a missing
// key defaults to shown → back-compat with older saved views).
export function fieldEnabled(fieldVis, key) {
  return fieldVis?.[key] !== false
}

// A field renders when the size preset allows it AND its toggle is on.
export function cardFieldVisible(size, fieldVis, key) {
  return sizeAllows(size, key) && fieldEnabled(fieldVis, key)
}
