// The open screen under another project address — the board or stage page a
// reader is on, once its project's address changed or the URL carried a former
// one (#3012). Query and hash ride along (`?task=`, `?milestone=`).
//
// Built from the path on purpose: `router.replace({ ...route, params })` keeps
// the route's `path`, and vue-router ignores `params` next to a `path` — that
// spelling silently navigated nowhere and left the old address in the bar.
export function withProjectSlug(route, slug) {
  const path = route.path.replace(/^\/project\/[^/]+/, `/project/${encodeURIComponent(slug)}`)
  return { path, query: route.query, hash: route.hash }
}
