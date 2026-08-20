import { useAuthStore } from '@/stores/auth'

// Shared "is this account brand new?" test (#2749 → #2753).
//
// A brand-new account is one created on/after the running build: it never
// updated *into* anything, so the What's-New changelog would be noise — and it
// is exactly the audience the Get Started guide is written for.
//
// Lives here rather than in stores/whatsNew.js because both the changelog and
// the guide key off it, and the guide must not import the changelog store just
// to ask this question.
//
// Unknown timestamps fall back to "not brand new", which never happens in
// practice: the server always sends created_at and the build always stamps a
// date. For the changelog that fallback means "show" (safe: a returning user
// sees the release notes); for the guide it means "stay quiet" (safe: an
// unexpected tour is more intrusive than a missing one).
const buildDate = typeof __BUILD_DATE__ !== 'undefined' ? __BUILD_DATE__ : ''

export function isBrandNewAccount() {
  const created = useAuthStore().user?.created_at
  if (!created || !buildDate) return false
  const c = new Date(created).getTime()
  const b = new Date(buildDate).getTime()
  if (Number.isNaN(c) || Number.isNaN(b)) return false
  return c >= b
}
