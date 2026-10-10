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
  // Capture without a connection: keep the record on the device and send it
  // later — #3031.
  'inbox.offline': 3031,
  // Settings (#3106): two-factor sign-in (#3081).
  'settings.twoFactor': 3081,
  // Administration (#3106): invitations across workspaces (#3079), roles and
  // permissions (#3063), a user's workspaces and last sign-in (#3078), CSV
  // export of users (#3080).
  'admin.invites': 3079,
  'admin.roles': 3063,
  'admin.userSpaces': 3078,
  'admin.csv': 3080,
  'toasts.sound': 3121,
  // Conferences (#3034): «Расшифровка» and «Итоги» of a recording need an
  // external speech-to-text / LLM service — #3125.
  'meet.transcript': 3125,
  // Large windows (#2975, package F). Members: roles and permissions, owner
  // transfer, groups, guests (#3063); resend / extend an invitation and a
  // reusable invite link (#3035).
  'members.roles': 3063,
  'members.inviteLink': 3035,
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
  // The phone (#2976, package G): «Отложить» a task by swiping its row on
  // «Мои задачи» — a task has a due date, not a snooze (#3108).
  'home.snoozeTask': 3108,
  // The board (#3112): «Кто сейчас смотрит» in the board's people — presence on
  // a board (#3116).
  'board.presence': 3116,
  // The editor (#3118): «Ссылка на фрагмент» in the bubble over read text — a
  // link to a stretch of a description or a comment needs anchors the app
  // doesn't keep yet (#3119).
  'editor.fragmentLink': 3119,
}

export function stubTask(key) {
  return FEATURE_STUBS[key] ?? null
}

export function isStub(key) {
  return key in FEATURE_STUBS
}
