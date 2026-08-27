// Which help article answers «что это за экран?» for a given route (#2794).
//
// Routes here carry no `name`, so the mapping is by path — the same shape the
// router declares them in (see router/index.js). Order matters: the first rule
// that matches wins, so board paths are listed before the bare-slug fallbacks.
const RULES = [
  [/^\/(?:project\/[^/]+\/board|board)(?:\/|$)/, 'boards-and-tasks'],
  [/^\/documents(?:\/|$)/, 'documents'],
  [/^\/notes(?:\/|$)/, 'notes'],
  [/^\/reminders(?:\/|$)/, 'reminders'],
  [/^\/milestones(?:\/|$)/, 'milestones'],
  [/^\/settings(?:\/|$)/, 'faq'],
  [/^\/admin(?:\/|$)/, 'faq'],
]

// null means «no hint here»: on the help centre itself the article is already
// on screen, and unknown screens get the guide's first article instead.
export function helpSlugForPath(path) {
  const p = String(path || '/')
  if (/^\/help(?:\/|$)/.test(p)) return null
  for (const [re, slug] of RULES) {
    if (re.test(p)) return slug
  }
  return 'first-steps'
}
