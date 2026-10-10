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
  // Settings (#3106): two-factor sign-in (#3081); quiet-hours exceptions
  // «Только срочное и упоминания» (#3050).
  'settings.twoFactor': 3081,
  'notifications.quietExceptions': 3050,
  // Administration (#3106): invitations across workspaces (#3079), roles and
  // permissions (#3063), a user's workspaces and last sign-in (#3078), CSV
  // export of users (#3080).
  'admin.invites': 3079,
  'admin.roles': 3063,
  'admin.userSpaces': 3078,
  'admin.csv': 3080,
  // Documents (#3106): the trash (#3006), a document's stage (#3044), template
  // categories (#3045).
  'documents.trash': 3006,
  'documents.milestone': 3044,
  'documents.templateCategories': 3045,
  // Notifications (#3107): «Принять / Отклонить» a meeting invitation right in
  // the feed (#3051), the page of all notifications with paging (#3049).
  'notifications.rsvp': 3051,
  'notifications.all': 3049,
  // Toasts (#3117): «В корзину» on the toast about a deleted task — tasks have
  // no trash yet (#3120); «Звук» in the toast settings (#3121).
  'toasts.taskTrash': 3120,
  'toasts.sound': 3121,
  // Conferences (#3107): reactions in a call (#3054).
  'meet.reactions': 3054,
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
  // The board (#3112): the star after the crumbs — favourite boards (#3115);
  // «Кто сейчас смотрит» in the board's people — presence on a board (#3116).
  'board.favorite': 3115,
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
