// Board templates for an empty project (#3047, mockup CatalogE «Пустой проект»).
//
// The presets themselves live on the server (handlers/board_templates.go): the
// client names one by key — POST /projects/:id/boards { template } — and the
// server seeds the columns (keyed, so captioned from board.column.defaultName),
// the view the board opens in and the project's tags. What the client keeps is
// what the picker draws: the key, the glyph of the view and the column names
// for the card's preview line.
export const BOARD_TEMPLATES = [
  {
    key: 'sprint',
    glyph: 'layout-kanban',
    columns: ['backlog', 'todo', 'in_progress', 'review', 'done'],
  },
  {
    key: 'bugs',
    glyph: 'layout-list',
    columns: ['new', 'confirmed', 'in_progress', 'verify', 'closed'],
  },
  {
    key: 'roadmap',
    glyph: 'layout-timeline',
    columns: ['ideas', 'planned', 'in_progress', 'shipped'],
  },
]

export function boardTemplate(key) {
  return BOARD_TEMPLATES.find((x) => x.key === key) || null
}

// The project page's target: the first live board, or none — the empty state.
export function firstBoard(boards) {
  return (boards || []).find((b) => !b.deleted_at) || null
}
