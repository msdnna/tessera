// The product release (#versioning, docs/versioning.md): «Tessera 2.0» — what a
// person installs and names — over the semver of the components (web, api, …).
// Baked into the bundle from release.json at build time (vite.config.js); null
// in a build without it (tests, a bare frontend checkout).
const RAW = typeof __RELEASE__ !== 'undefined' ? __RELEASE__ : null

export const RELEASE = RAW && typeof RAW.product === 'string' ? RAW : null

// 2.0.0 → «2.0», 2.0.1 → «2.0.1» — how people say it.
export function shortVersion(v) {
  const m = /^(\d+)\.(\d+)\.(\d+)$/.exec(String(v || ''))
  if (!m) return String(v || '')
  return m[3] === '0' ? `${m[1]}.${m[2]}` : `${m[1]}.${m[2]}.${m[3]}`
}

// «Tessera 2.0», or '' without a release.
export function productLabel(release = RELEASE) {
  return release ? `Tessera ${shortVersion(release.product)}` : ''
}

// Is the server this bundle talks to the one its release ships? An older or
// newer API under a released web means a partial deploy — the admin is told.
export function apiInRelease(apiVersion, release = RELEASE) {
  const want = release?.components?.backend
  return !want || !apiVersion || want === apiVersion
}
