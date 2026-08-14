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

    // ── board ──────────────────────────────────────────────────────────────
    //
    // Board anchors are per-entity: the id comes from the seeded fixture, so a
    // spec asserts «this column / this card», not «the third one from the left».
    // An index-based anchor would keep passing after a sorting regression.

    /** The kanban lane for a column (or a tag/milestone lane), expanded or collapsed. */
    fun boardColumn(id: String) = "board-column:$id"

    /** Reveals the inline «new card» field at the foot of a column. */
    fun columnAddTask(id: String) = "column-add-task:$id"

    /** That inline field itself, once revealed. */
    fun columnTaskInput(id: String) = "column-task-input:$id"

    /** A task card as rendered on the board. Drag ghosts deliberately carry no
     *  tag (see [website.msdnna.tessera.ui.components.TaskCard]'s `anchored`),
     *  so this stays unique while a card or column is being dragged. */
    fun taskCard(id: String) = "task-card:$id"

    /** Tile at the right end of the status lanes that starts a new column. */
    const val BOARD_ADD_COLUMN = "board-add-column"

    /** The inline field the tile above reveals. */
    const val BOARD_COLUMN_INPUT = "board-column-input"

    // ── task modal ─────────────────────────────────────────────────────────

    /** Root of the open task modal — present exactly while a task is open. */
    const val TASK_MODAL = "task-modal"

    const val TASK_TITLE = "task-title"

    /** The description editor's text area (absent while it shows the preview tab). */
    const val TASK_DESCRIPTION = "task-description"

    /** Footer button that commits title + description. */
    const val TASK_SAVE = "task-save"

    /** The column chip in the status row, and one row per column in its picker.
     *  Only the status row is anchored: the same chip renders for every subtask
     *  ([website.msdnna.tessera.ui.screens] `ColumnChipPicker(mini = true)`), and
     *  tagging it there would put several nodes under one tag on any task that
     *  has subtasks. */
    const val TASK_STATUS = "task-status"

    fun taskStatusOption(columnId: String) = "task-status-option:$columnId"

    /** The priority chip and its picker rows, keyed by level (0 = none). */
    const val TASK_PRIORITY = "task-priority"

    fun taskPriorityOption(level: Int) = "task-priority-option:$level"

    /** Composer at the foot of the Комментарии tab, and its send button. */
    const val TASK_COMMENT_INPUT = "task-comment-input"
    const val TASK_COMMENT_SUBMIT = "task-comment-submit"
}
