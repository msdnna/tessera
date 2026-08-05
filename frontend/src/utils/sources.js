// Provider-neutral dictionary of "where did this record come from" sources, used by
// the source badges (relations, and whatever gets a `source` column next). Consumers
// never name a provider — adding the second integration is one entry here.
import { LogoGitlab } from '@vicons/ionicons5'

export const SOURCES = {
  user: { label: 'Tessera', icon: null },
  gitlab: { label: 'GitLab', icon: LogoGitlab },
}

export function sourceMeta(source) {
  return SOURCES[source] || { label: source || '—', icon: null }
}

// A source that isn't the user typing it in — i.e. an integration owns the record and
// will re-create it on the next sync. Empty/absent source means legacy user data.
export function isExternalSource(source) {
  return !!source && source !== 'user'
}
