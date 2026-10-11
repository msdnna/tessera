// Feature stubs (v2 redesign, epic #2966).
//
// The v2 mockups draw some controls whose feature doesn't exist yet. The
// control is still built where the mockup puts it — visible, inactive, with a
// «Скоро · #N» tooltip — and every such spot is registered here under a key
// pointing at the task that will make it real. Shipping that task means grepping
// its key, wiring the control and deleting the entry; an empty registry is the
// goal.
//
// Use: `stubTask('project.slugSuggest')` → 3013, and `isStub(key)` to branch.

export const FEATURE_STUBS = {
  // Settings (#3106): two-factor sign-in (#3081).
  'settings.twoFactor': 3081,
  'admin.roles': 3076,
  // GitLab: several projects on one board, OAuth connection, the token's
  // expiry (#3065); label → stage rules, rule switches and counters, the
  // conflict policy, «Отложить» in the resolver (#3066); import and export
  // (#3068).
  'gitlab.connections': 3065,
  'gitlab.rules': 3066,
  'gitlab.importExport': 3068,
  // Background jobs: history, steps and progress, the live log, the queue
  // (#3067).
  'jobs.history': 3067,
  // Workspace commands: palette macros and task templates (#3069).
  'commands.macros': 3069,
}

export function stubTask(key) {
  return FEATURE_STUBS[key] ?? null
}

export function isStub(key) {
  return key in FEATURE_STUBS
}
