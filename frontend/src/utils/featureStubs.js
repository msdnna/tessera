// Feature stubs (v2 redesign, epic #2966).
//
// The v2 mockups draw some controls whose feature doesn't exist yet. The
// control is still built where the mockup puts it — visible, inactive, with a
// «Скоро · #N» tooltip — and every such spot is registered here under a key
// pointing at the task that will make it real. Shipping that task means grepping
// its key, wiring the control and deleting the entry; an empty registry is the
// goal.
//
// Use: `stubTask('project.archive')` → 3005, and `isStub(key)` to branch.

export const FEATURE_STUBS = {
  // Project archive: menu item «Архивировать» (E) — #3005.
  'project.archive': 3005,
  // Free-variant suggestion and «address taken by X» — #3013.
  'project.slugSuggest': 3013,
  // Old project addresses keep redirecting for 30 days — #3012.
  'project.slugAliases': 3012,
  // Counts in the delete confirmations («3 доски и 148 задач») — #3014.
  'delete.counts': 3014,
  // Editing, closing and reopening a GitLab-sourced stage with the change
  // written back to GitLab (#3010) — until then such a stage is read-only here.
  'stage.gitlabWrite': 3010,
  // «Входящие» (#2972): #tags and @mentions of a record are read by the capture
  // but not stored yet — #3040.
  'inbox.tags': 3040,
  // Capture without a connection: keep the record on the device and send it
  // later — #3031.
  'inbox.offline': 3031,
}

export function stubTask(key) {
  return FEATURE_STUBS[key] ?? null
}

export function isStub(key) {
  return key in FEATURE_STUBS
}
