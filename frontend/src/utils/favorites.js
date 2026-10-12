// Favourite boards (#3115) — the pure half of stores/favorites.js.

// favoriteNeighbors names the rows around index `i` the way the move endpoint
// takes them: before_id — the row above, after_id — the row below. An end of
// the list leaves its side out.
export function favoriteNeighbors(list, i) {
  const out = {}
  const prev = list[i - 1]
  const next = list[i + 1]
  if (prev) out.before_id = prev.board_id
  if (next) out.after_id = next.board_id
  return out
}

// favoritePatch applies a board or project event to the rows it names — the
// name, address and icon a sidebar row draws. Returns the new list, or null
// when no row was touched (so the caller can skip a pointless re-render).
export function favoritePatch(list, type, data) {
  if (!data?.id) return null
  let hit = false
  const out = list.map((f) => {
    if (type === 'board.updated' && f.board_id === data.id) {
      hit = true
      return {
        ...f,
        name: data.name ?? f.name,
        slug: data.slug ?? f.slug,
        icon: data.icon ?? f.icon,
        color: data.color ?? f.color,
        icon_mode: data.icon_mode ?? f.icon_mode,
      }
    }
    if (type === 'project.updated' && f.project_id === data.id) {
      hit = true
      return {
        ...f,
        project_name: data.name ?? f.project_name,
        project_slug: data.slug ?? f.project_slug,
        project_color: data.color ?? f.project_color,
      }
    }
    return f
  })
  return hit ? out : null
}

// favoriteFirst orders board search results with the reader's favourites on
// top, keeping the server's order within each half (the palette, #3115).
export function favoriteFirst(boards, isFavorite) {
  const fav = []
  const rest = []
  for (const b of boards) (isFavorite(b.id) ? fav : rest).push(b)
  return [...fav, ...rest]
}
