// A taken project address (#3013). The server answers 409 with
// { error, suggestion, taken_by? }: `suggestion` is the first free variant of
// the address the user typed (what ⌥↵ puts into the field), `taken_by` names
// the holder — only when the caller is a member of the holder's workspace, so
// it may be missing.
//
// `inGroup` picks the create form's wording from the mockup («так уже
// называется проект в группе «Маркетинг»»); the address forms name the
// project instead («его использует проект «Сайт»»).
//
// Returns null for anything but a 409, so callers can fall through to their
// generic error handling.
export function slugTaken(e, t, { inGroup = false } = {}) {
  if (e?.status !== 409) return null
  const data = e.data && typeof e.data === 'object' ? e.data : {}
  const by = data.taken_by && typeof data.taken_by === 'object' ? data.taken_by : null
  let error
  if (by && inGroup && by.group_name)
    error = t('project.slug.takenInGroup', { group: by.group_name })
  else if (by?.name) error = t('project.slug.takenBy', { name: by.name })
  else error = t('project.slug.taken')
  const suggestion = typeof data.suggestion === 'string' ? data.suggestion : ''
  return { error, suggestion }
}
