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

    /** The «⋯» in a column header, which opens rename / colour / «Завершающая» /
     *  delete. Only status lanes carry it: group the board by tags or milestones
     *  and the headers stop being columns, so the button is gone. */
    fun columnMenu(id: String) = "column-menu:$id"

    /** The colour swatches inside that menu — the one part of it that is neither a
     *  labelled row nor a text item, so a spec (and the screenshot run) anchors on
     *  it to tell «the menu is open» from «the tap missed». */
    const val COLUMN_MENU_COLORS = "column-menu-colors"

    // ── grouping (the killer feature: lanes = tags) ─────────────────────────

    /** The transparent overlay over a *collapsed* composer bar, which turns a tap
     *  anywhere on it into «expand me» ([website.msdnna.tessera.ui.screens]
     *  `BoardComposerBar`). It exists exactly while the bar is collapsed, so a spec
     *  can both tell the state apart and take the same route a user takes: the chips
     *  below only answer taps once it is gone. */
    const val BOARD_COMPOSER_EXPAND = "board-composer-expand"

    /** The always-present grouping chip in the composer bar; opens the mode menu. */
    const val BOARD_GROUP = "board-group"

    /** Rows of that menu. Only the two always-present modes get a fixed tag —
     *  «По этапам» / «По исполнителю» appear conditionally and belong to specs
     *  that seed the condition. */
    const val BOARD_GROUP_STATUS = "board-group-status"
    const val BOARD_GROUP_TAGS = "board-group-tags"

    /** One row per tag namespace present on the board, keyed by the canonical
     *  prefix («S: ») rather than its friendly label, which the project renames. */
    fun boardGroupTagPrefix(prefix: String) = "board-group-tag-prefix:$prefix"

    /** The inline field the tile above reveals. */
    const val BOARD_COLUMN_INPUT = "board-column-input"

    // ── right-hand toolbar ─────────────────────────────────────────────────

    /** The gear that opens «Вид доски» ([website.msdnna.tessera.ui.screens]
     *  `BoardCustomizePanel`), and the folder that opens the saved-views popover.
     *  Both live in the row that hides while the composer bar is expanded, so a
     *  spec that wants them must collapse the bar first — same as a reader does. */
    const val BOARD_CUSTOMIZE = "board-customize"
    const val BOARD_SAVED_VIEWS = "board-saved-views"

    /** Root of the open «Вид доски» dialog — present exactly while it is open. */
    const val BOARD_CUSTOMIZE_PANEL = "board-customize-panel"

    /** Root of the tag manager dialog. Opened from the app bar's overflow, which
     *  lives above `BoardScreen` — a caller that owns the flag (a spec, the
     *  screenshot run) can render it without reproducing that menu. */
    const val TAG_MANAGER = "tag-manager"

    // ── task modal ─────────────────────────────────────────────────────────

    /** Root of the open task modal — present exactly while a task is open. */
    const val TASK_MODAL = "task-modal"

    const val TASK_TITLE = "task-title"

    /** The description editor's text area (absent while it shows the preview tab). */
    const val TASK_DESCRIPTION = "task-description"

    /** A tab in the modal's strip, keyed by role rather than by its localised
     *  label. «Описание» is the first tab and the one open on load (#2754). */
    fun taskTab(key: String) = "task-tab:$key"

    const val TASK_TAB_DESCRIPTION = "description"
    const val TASK_TAB_COMMENTS = "comments"
    const val TASK_TAB_SUBTASKS = "subtasks"
    const val TASK_TAB_RELATIONS = "relations"
    const val TASK_TAB_FILES = "files"
    const val TASK_TAB_HISTORY = "history"

    /** Footer button that commits title + description. */
    const val TASK_SAVE = "task-save"

    /** The column chip in the status row, and one row per column in its picker.
     *  Only the status row is anchored: the same chip renders for every subtask
     *  ([website.msdnna.tessera.ui.screens] `ColumnChipPicker(mini = true)`), and
     *  tagging it there would put several nodes under one tag on any task that
     *  has subtasks. */
    const val TASK_STATUS = "task-status"

    fun taskStatusOption(columnId: String) = "task-status-option:$columnId"

    /** «Создать issue» in the GitLab row of an unlinked task, and the issue-template
     *  picker beside it. Both only exist on a board whose GitLab binding allows
     *  creation (`push_create` / `fetch_templates`). */
    const val TASK_GITLAB_CREATE = "task-gitlab-create"
    const val TASK_GITLAB_TEMPLATE = "task-gitlab-template"

    /** The priority chip and its picker rows, keyed by level (0 = none). */
    const val TASK_PRIORITY = "task-priority"

    fun taskPriorityOption(level: Int) = "task-priority-option:$level"

    /** Composer at the foot of the Комментарии tab, and its send button. */
    const val TASK_COMMENT_INPUT = "task-comment-input"
    const val TASK_COMMENT_SUBMIT = "task-comment-submit"

    /** The in-thread reply composer — at most one is open at a time. */
    const val TASK_REPLY_INPUT = "task-reply-input"
    const val TASK_REPLY_SUBMIT = "task-reply-submit"

    /**
     * One row of the Связи / История tabs, keyed by the thing it shows.
     *
     * Per row rather than per list on purpose: both tabs render their empty state
     * inside the same container, so a list-level anchor would already be there
     * while the detail request is still in flight — and a screenshot taken then
     * photographs «пока ничего нет».
     */
    fun taskRelationRow(relatedTaskId: String) = "task-relation-row:$relatedTaskId"
    fun taskEventRow(eventId: String) = "task-event-row:$eventId"

    // ── documents (read-only, #2735) ───────────────────────────────────────

    /** Root of the documents section — the tree of the workspace's documents. */
    const val DOCUMENTS_SCREEN = "documents-screen"

    /** The reader that slides over the tree; present exactly while one is open. */
    const val DOCUMENT_READER = "document-reader"

    /** One row of that tree, keyed by document id — so a spec asserts «this
     *  document», not «the second row», and a nesting regression is visible. */
    fun documentRow(id: String) = "document-row:$id"

    // ── help centre (#2795) ────────────────────────────────────────────────

    /** Root of the help section — the category navigation over the bundled manual. */
    const val HELP_NAV = "help-nav"

    /** Its search field, and one row per article (or per search hit), keyed by
     *  slug: a spec asserts «this article», not «the third row», so a reordering
     *  of the manual doesn't quietly rewrite what a test checks. */
    const val HELP_SEARCH = "help-search"

    fun helpRow(slug: String) = "help-row:$slug"

    /** The article reader that slides over the navigation, present exactly while
     *  an article is open. */
    const val HELP_ARTICLE = "help-article"

    /** The «this text describes the web version» note, shown exactly on the
     *  articles that have no mobile rewrite yet (#2795). */
    const val HELP_DESKTOP_NOTE = "help-desktop-note"

    /** The «not translated yet, showing the original» note (#2809), shown when a
     *  non-Russian reader opens an article that has no translation. */
    const val HELP_NOT_TRANSLATED_NOTE = "help-not-translated-note"

    // ── what's new / spotlight (#2766) ──────────────────────────────────────

    /** The post-update changelog card and its «Понятно». Present only while the
     *  user has releases to catch up on, so a spec can assert both states. */
    const val WHATS_NEW_CARD = "whats-new-card"
    const val WHATS_NEW_DISMISS = "whats-new-dismiss"

    /** The sidebar hint that follows the card, and its «Понятно». One at a time. */
    const val SPOTLIGHT_CARD = "spotlight-card"
    const val SPOTLIGHT_DISMISS = "spotlight-dismiss"
}
