// «Статья помогла?» (#3071): a vote is a per-user acknowledgement — the key
// carries the article, the language it was read in and the answer. The server
// sums the same keys for the administration (GET /admin/help-feedback), so the
// shape here and in handlers/help_feedback.go must agree.
export const HELP_VOTE_PREFIX = 'helpful:'

export function helpVoteKey(slug, lang, vote) {
  return `${HELP_VOTE_PREFIX}${slug}:${lang}:${vote === 'yes' ? 'yes' : 'no'}`
}
