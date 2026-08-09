// Body for the full-replace `PUT /tasks/:id` used by every *inline* edit — card
// pills, context menus, quick actions — where the task object at hand came from
// a board/list payload rather than from the task modal.
//
// Description is deliberately OMITTED. The list payloads strip it (see
// backend/handlers/task_list_dto.go: `has_description` travels, the text does
// not), so those task objects have no `description` field at all — while the
// backend treats description as tri-state: absent = keep the stored text,
// present (including "") = replace it. Sending `description: t.description || ''`
// from here would blank the description of every task edited from a card or a
// context menu. The modal, which does load the text, sends its own body.
export function taskBasePatch(t) {
  return {
    title: t.title,
    priority: t.priority || 0,
    due_date: t.due_date || null,
    start_date: t.start_date || null,
    recurrence: t.recurrence || null,
    completed: !!t.completed_at,
  }
}
