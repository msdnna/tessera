package website.msdnna.tessera.ui

/**
 * Stable anchors for the e2e tier (`app/src/test/.../e2e`).
 *
 * The specs deliberately do not select by visible text: labels here are Russian
 * copy that gets rewritten (#2610 turned «Выполнено» into «Статус»), and a spec
 * that breaks on a wording change reports a failure where there is no defect.
 * A tag is a contract — renaming one is a deliberate act that shows up in review,
 * unlike an incidental copy edit.
 *
 * Values are referenced as plain strings by `Modifier.testTag(...)` in `src/main`
 * and by `onNodeWithTag(...)` in the specs; keep both sides on these constants.
 */
object TestTags {
    // ── auth ───────────────────────────────────────────────────────────────
    const val AUTH_EMAIL = "auth-email"
    const val AUTH_NAME = "auth-name"
    const val AUTH_PASSWORD = "auth-password"
    const val AUTH_SUBMIT = "auth-submit"
    const val AUTH_ERROR = "auth-error"

    /** Switches the form between login and register. */
    const val AUTH_TOGGLE_MODE = "auth-toggle-mode"

    /** Gear in the corner that reveals the server-address popover. */
    const val AUTH_SERVER_TOGGLE = "auth-server-toggle"
    const val AUTH_SERVER_FIELD = "auth-server-field"

    // ── shell ──────────────────────────────────────────────────────────────

    /** Present exactly when the session gate has let us past the auth screen. */
    const val MAIN_SHELL = "main-shell"
}
