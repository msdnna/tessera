// What a delete takes with it, in words (#3014): «4 проекта, 23 доски и 612
// задач». `counts` is the answer of GET /projects/:id/counts or
// /workspaces/:id/counts. Zero parts are left out — «3 доски и 0 задач» says
// nothing a shorter phrase doesn't — and an empty string means there is nothing
// to name (unknown yet, or an empty container): callers keep their generic line.
//
// A sentence with a verb puts the list after a colon («Будут удалены: …»): in
// Russian the verb agrees with the list, and «1 доска будут удалены» is what the
// mockup's word order gives for a lone count ending in 1.
const ORDER = ['projects', 'boards', 'tasks']

export function countsPhrase(t, counts) {
  if (!counts) return ''
  const parts = ORDER.filter((k) => Number(counts[k]) > 0).map((k) =>
    t(`common.counts.${k}`, Number(counts[k])),
  )
  if (parts.length === 0) return ''
  if (parts.length === 1) return parts[0]
  return t('common.counts.join', {
    list: parts.slice(0, -1).join(', '),
    last: parts[parts.length - 1],
  })
}
