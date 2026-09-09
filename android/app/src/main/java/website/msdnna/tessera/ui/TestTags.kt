package website.msdnna.tessera.ui

import website.msdnna.tessera.util.ConfAudioRoute

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

    /** «Не проверять сертификат сервера» — общий якорь для экрана входа и настроек. */
    const val TLS_INSECURE_SWITCH = "tls-insecure-switch"

    /** Pre-login RU/EN cycle button (web `data-testid="auth-lang-toggle"`). */
    const val AUTH_LANG_TOGGLE = "auth-lang-toggle"

    // ── shell ──────────────────────────────────────────────────────────────

    /** Present exactly when the session gate has let us past the auth screen. */
    const val MAIN_SHELL = "main-shell"

    /**
     * «Попробовать ещё раз» on the launch gate's error screens — the only way out
     * of `Boot.ConnectError`, which is a terminal state until someone taps it.
     */
    const val BOOT_RETRY = "boot-retry"

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

    /** The «3 (6)» counter in a column header (#2850). Per-column, because the point
     *  of a spec here is «THIS column folds in its subtasks», not «some header shows
     *  a number». */
    fun columnCount(id: String) = "column-count:$id"

    // ── grouping (the killer feature: lanes = tags) ─────────────────────────

    /** The transparent overlay over a *collapsed* composer bar, which turns a tap
     *  anywhere on it into «expand me» ([website.msdnna.tessera.ui.screens]
     *  `BoardComposerBar`). It exists exactly while the bar is collapsed, so a spec
     *  can both tell the state apart and take the same route a user takes: the chips
     *  below only answer taps once it is gone. */
    const val BOARD_COMPOSER_EXPAND = "board-composer-expand"

    /** The bar's «показано: 2 (4)» counter (#2851) — one per board, unlike the
     *  per-column headers: it reports the whole filtered board. */
    const val BOARD_COMPOSER_COUNT = "board-composer-count"

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
    const val TASK_TAB_DOCUMENTS = "documents"
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

    /** «Сгруппированная» badge on a linked issue that carries the grouping label, and
     *  the per-subtask GitLab hierarchy marker / «родитель не сгруппирован» hint in the
     *  subtasks tab. All three only exist on a binding with `push_children`. */
    const val TASK_GITLAB_GROUPED = "task-gitlab-grouped"
    const val TASK_SUBTASK_GL_CHIP = "task-subtask-gl-chip"
    const val TASK_SUBTASK_GL_HINT = "task-subtask-gl-hint"

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

    // ── documents (#2735, #2894) ───────────────────────────────────────────

    /** Root of the documents section — the grid of one nesting level. */
    const val DOCUMENTS_SCREEN = "documents-screen"

    /** The reader that slides over the grid; present exactly while one is open. */
    const val DOCUMENT_READER = "document-reader"

    /** «Документ пуст» — the reader's answer for a document with no blocks.
     *  Tagged rather than matched by its words: from the outside an empty body
     *  and a body that never loaded look the same, so a spec has to be able to
     *  tell them apart, and the sentence itself is localised. */
    const val DOCUMENT_READER_EMPTY = "document-reader-empty"

    /** One tile of that grid, keyed by document id — so a spec asserts «this
     *  document», not «the second tile», and a nesting regression is visible. */
    fun documentRow(id: String) = "document-row:$id"

    // ── conferences (#2896) ────────────────────────────────────────────────

    /** Root of the conferences section — the workspace's calls. */
    const val CONFERENCES_SCREEN = "conferences-screen"

    /** The «Запланировать» button and the dialog it opens. */
    const val CONFERENCE_SCHEDULE = "conference-schedule"
    const val CONFERENCE_SCHEDULE_DIALOG = "conference-schedule-dialog"
    const val CONFERENCE_CREATE_NAME = "conference-create-name"
    const val CONFERENCE_CREATE_TTL = "conference-create-ttl"
    const val CONFERENCE_CREATE_SUBMIT = "conference-create-submit"

    /** One call in the list, and its delete button — keyed by id, so a spec says
     *  «this conference» and a sorting change doesn't rewrite what it checks. */
    fun conferenceRow(id: String) = "conference-row:$id"

    fun conferenceDelete(id: String) = "conference-delete:$id"

    /** The confirm of the delete popover — the press that actually reaches the
     *  server. The row's own button only asks. */
    const val CONFERENCE_DELETE_CONFIRM = "conference-delete-confirm"

    /** One status tab, keyed by the value it sends to the server (`live`,
     *  `scheduled`, `ended`; blank for «Все») rather than by its position —
     *  a reordered tab strip must not quietly rewrite what a spec asserts. */
    fun conferenceFilter(status: String?) = "conference-filter:${status.orEmpty().ifBlank { "all" }}"

    /** The lobby of one call (#2896 §3) and its controls. */
    const val CONFERENCE_LOBBY = "conference-lobby"
    const val CONFERENCE_BACK = "conference-back"
    const val CONFERENCE_JOIN = "conference-join"
    const val CONFERENCE_LEAVE = "conference-leave"
    const val CONFERENCE_END = "conference-end"
    const val CONFERENCE_INVITE = "conference-invite"

    /** One seat in the roster and one line of the invite picker, keyed by user —
     *  the two lists are complementary, so a spec can assert a person is in the
     *  one and not the other without depending on either's order. */
    fun conferenceSeat(userId: String) = "conference-seat:$userId"

    fun conferenceInvitee(userId: String) = "conference-invitee:$userId"

    /** The call itself (#2896 §5) — present exactly while we hold a seat. */
    const val CONFERENCE_ROOM = "conference-room"
    const val CONFERENCE_STAGE = "conference-stage"
    const val CONFERENCE_STRIP = "conference-strip"
    const val CONFERENCE_BANNER = "conference-banner"
    const val CONFERENCE_RETRY = "conference-retry"
    const val CONFERENCE_QUALITY = "conference-quality"
    const val CONFERENCE_FULLSCREEN = "conference-fullscreen"

    /** «Сбросить масштаб» over a pinched stage — the only way back to 1× that
     *  does not need a second pinch, and present only while there is something
     *  to reset. */
    const val CONFERENCE_ZOOM_RESET = "conference-zoom-reset"

    /** The room toolbar. */
    const val CONFERENCE_MIC = "conference-mic"
    const val CONFERENCE_CAM = "conference-cam"
    const val CONFERENCE_SWITCH_CAM = "conference-switch-cam"
    const val CONFERENCE_ROUTE = "conference-route"
    const val CONFERENCE_HANGUP = "conference-hangup"

    /** One tile, keyed by the participant's identity rather than by position —
     *  the stage moves with whoever is talking, so an index would assert
     *  «whoever is second» and pass through a layout that lost the speaker. */
    fun conferenceTile(identity: String) = "conference-tile:$identity"

    /** The crossed-out microphone on somebody's tile — the only thing on screen
     *  that says a participant is silent, so it needs an anchor of its own: the
     *  tile is displayed either way. */
    fun conferenceTileMuted(identity: String) = "conference-tile-muted:$identity"

    /** One output in the routing menu. */
    fun conferenceRoute(route: ConfAudioRoute) = "conference-route:${route.name.lowercase()}"

    // ── participants and moderation (#2896 §6) ─────────────────────────────

    const val CONFERENCE_HAND = "conference-hand"
    const val CONFERENCE_PEOPLE = "conference-people"
    const val CONFERENCE_PANEL = "conference-panel"
    const val CONFERENCE_PANEL_EMPTY = "conference-panel-empty"
    const val CONFERENCE_PANEL_CLOSE = "conference-panel-close"

    /** The grabber the roster sheet is pulled up by. Its own anchor rather than
     *  the sheet's: the sheet is displayed at both sizes, so a drag asserted on
     *  [CONFERENCE_PANEL] would pass without ever having moved anything. */
    const val CONFERENCE_PANEL_HANDLE = "conference-panel-handle"
    const val CONFERENCE_DENIED = "conference-denied"
    const val CONFERENCE_KICK_CONFIRM = "conference-kick-confirm"

    /** The count of raised hands on the closed panel. Separate from the button,
     *  which is there on every call — the badge is only there when somebody is
     *  waiting, so asserting on the button would say nothing about it. */
    const val CONFERENCE_HANDS = "conference-hands"

    /** One roster row, keyed by user — the order changes with the hands. */
    fun conferencePerson(userId: String) = "conference-person:$userId"

    /** The «поднял руку» marker. Separate from the row: the row is there either
     *  way, so asserting on it would pass in a build that lost the marker. */
    fun conferenceHandUp(userId: String) = "conference-hand-up:$userId"

    /** «Заглушён ведущим» — the badge that says a silence was not their choice. */
    fun conferenceForced(userId: String) = "conference-forced:$userId"

    /** Moderation. Two tags, because the server treats them differently: a host
     *  may be force-muted and may not be kicked. */
    fun conferenceForceMute(userId: String) = "conference-force-mute:$userId"

    fun conferenceKick(userId: String) = "conference-kick:$userId"

    /** Local playback — this phone only, never the room. */
    fun conferenceLocalMute(userId: String) = "conference-local-mute:$userId"

    fun conferenceVolume(userId: String) = "conference-volume:$userId"

    // ── in-call chat (#2896 §7) ────────────────────────────────────────────

    const val CONFERENCE_CHAT = "conference-chat"
    const val CONFERENCE_CHAT_OPEN = "conference-chat-open"
    const val CONFERENCE_CHAT_CLOSE = "conference-chat-close"

    /** The grabber the chat sheet is pulled up by — see [CONFERENCE_PANEL_HANDLE]
     *  for why the sheet's own tag will not do. */
    const val CONFERENCE_CHAT_HANDLE = "conference-chat-handle"
    const val CONFERENCE_CHAT_LOG = "conference-chat-log"
    const val CONFERENCE_CHAT_EMPTY = "conference-chat-empty"
    const val CONFERENCE_CHAT_OLDER = "conference-chat-older"
    const val CONFERENCE_CHAT_INPUT = "conference-chat-input"
    const val CONFERENCE_CHAT_SEND = "conference-chat-send"
    const val CONFERENCE_CHAT_ATTACH = "conference-chat-attach"

    /** A refused pick (too big, too many). Separate from the error line: one is
     *  about the file the user just chose, the other about the server. */
    const val CONFERENCE_CHAT_REFUSAL = "conference-chat-refusal"

    /** Unread lines on the closed sheet. Like the hands badge, only there when
     *  it has something to say. */
    const val CONFERENCE_CHAT_UNREAD = "conference-chat-unread"

    /** One message, keyed by id — the list grows at both ends, so an index would
     *  name a different line after every «показать более ранние». */
    fun conferenceChatMessage(id: String) = "conference-chat-message:$id"

    /** Delete, per message. Only on the ones this phone may remove, which is the
     *  whole point of asserting on it. */
    fun conferenceChatDelete(id: String) = "conference-chat-delete:$id"

    /** The confirm button of the delete dialog — the press that actually reaches
     *  the server, and the only one a spec should be able to make. */
    const val CONFERENCE_CHAT_DELETE_CONFIRM = "conference-chat-delete-confirm"

    /** An attachment on a message, and the picked-but-unsent file in the
     *  composer — different lifetimes, different tags. */
    fun conferenceChatAttachment(id: String) = "conference-chat-attachment:$id"

    fun conferenceChatPending(uri: String) = "conference-chat-pending:$uri"

    /** Takes a picked file back off the message before it is sent — the only way
     *  out of a wrong pick, since the picker replaces nothing. */
    fun conferenceChatPendingRemove(uri: String) = "conference-chat-pending-remove:$uri"

    // ── screen share and recordings (#2896 §8) ────────────────────────────

    /** The share control. One button through all four states — a spec asserts on
     *  the notice next to it to tell «в очереди» from «показываю». */
    const val CONFERENCE_SHARE = "conference-share"

    /** The line above the tiles: who holds the stage, and where we are in the
     *  queue behind them. */
    const val CONFERENCE_STAGE_NOTICE = "conference-stage-notice"

    /** Our 1-based place in the queue, only rendered while we are in it. */
    const val CONFERENCE_QUEUE_POS = "conference-queue-pos"

    /** «Начать показ» — the second press, offered exactly when the stage is ours
     *  and Android has not been asked for the display yet. */
    const val CONFERENCE_SHARE_START = "conference-share-start"

    /** Start/stop recording. Absent for a member: only a moderator may press it,
     *  and a greyed-out one advertises a capability they do not have. */
    const val CONFERENCE_RECORD = "conference-record"

    /** The red dot — «идёт запись», with the name of whoever started it. Its
     *  presence follows the room snapshot, never the press. */
    const val CONFERENCE_RECORD_DOT = "conference-record-dot"

    /** A refused start/stop, including the honest «сервис недоступен» an install
     *  without the egress worker answers with. */
    const val CONFERENCE_RECORD_ERROR = "conference-record-error"

    /** The recordings list in the lobby, and one row per recording. */
    const val CONFERENCE_RECORDINGS = "conference-recordings"

    fun conferenceRecordingRow(id: String) = "conference-recording:$id"

    fun conferenceRecordingDownload(id: String) = "conference-recording-download:$id"

    fun conferenceRecordingDelete(id: String) = "conference-recording-delete:$id"

    /** The confirm of the delete dialog — the press that actually erases the
     *  file, and the only one a spec should be able to make. */
    const val CONFERENCE_RECORDING_DELETE_CONFIRM = "conference-recording-delete-confirm"

    /** The list's own error line; the row buttons report their failures here. */
    const val CONFERENCE_RECORDINGS_ERROR = "conference-recordings-error"

    // ── the minimised call (#2896 §9) ─────────────────────────────────────

    /** The bar over the rest of the app while a call is off-screen. Its absence
     *  is as much an assertion as its presence: it must not draw over the room. */
    const val CONFERENCE_MINI = "conference-mini"

    /** The line it says about the call — one tag, because a spec asserts on the
     *  text, and «идёт звонок» where «связь потеряна» belongs is the bug. */
    const val CONFERENCE_MINI_LINE = "conference-mini-line"

    /** How many others are in the call. Absent when nobody else is — a lone «0»
     *  next to a people icon reads as a broken counter. */
    const val CONFERENCE_MINI_COUNT = "conference-mini-count"

    /** Its own microphone and hang-up. The second is the only way out of a call
     *  whose lobby is nowhere on screen. */
    const val CONFERENCE_MINI_MIC = "conference-mini-mic"

    const val CONFERENCE_MINI_HANGUP = "conference-mini-hangup"

    /** «Новый документ», and the crumb that returns the grid to the root. */
    const val DOCUMENTS_CREATE = "documents-create"
    const val DOCUMENTS_CRUMB_ROOT = "documents-crumb-root"

    /** One breadcrumb step, keyed by the container it walks back to. */
    fun documentCrumb(id: String) = "documents-crumb:$id"

    /** «К списку документов» — the way out of the open document, an item of the
     *  title menu since the section stopped drawing a header of its own. */
    const val DOCUMENT_BACK = "document-back"

    /** The open document's name in the shell's top bar: tapping it opens the
     *  menu that holds everything the second header used to. */
    const val DOCUMENT_MENU = "document-menu"

    /** One neighbouring document in that menu (the parent, or a nested one),
     *  and the bin beside it. */
    const val DOCUMENT_SWITCH_ROW = "document-switch-row"
    const val DOCUMENT_SWITCH_REMOVE = "document-switch-remove"

    /** The reader's «⋯» menu and its items. Anchored per action rather than per
     *  menu: their wording lives in the locale files and is not an anchor. */
    const val DOCUMENT_ACTIONS = "document-actions"
    const val DOCUMENT_ACTION_NESTED = "document-action-nested"
    const val DOCUMENT_ACTION_CHILDREN = "document-action-children"
    const val DOCUMENT_ACTION_RENAME = "document-action-rename"
    const val DOCUMENT_ACTION_REMOVE = "document-action-remove"

    /** The title field of the create / rename dialog, and its confirm button. */
    const val DOCUMENT_TITLE_INPUT = "document-title-input"
    const val DOCUMENT_TITLE_CONFIRM = "document-title-confirm"

    /** Confirm button of the delete dialog. */
    const val DOCUMENT_REMOVE_CONFIRM = "document-remove-confirm"

    /** The reader's outline: the toggle, the panel, its close button, one row. */
    const val DOCUMENT_TOC_OPEN = "document-toc-open"
    const val DOCUMENT_TOC = "document-toc"
    const val DOCUMENT_TOC_CLOSE = "document-toc-close"

    /** Keyed by the heading's block id — the same anchor the jump scrolls to. */
    fun documentTocRow(blockId: String) = "document-toc-row:$blockId"

    /** A PDF block, its rendered page and the two page arrows (#2733). */
    const val DOCUMENT_PDF = "document-pdf"
    const val DOCUMENT_PDF_PAGE = "document-pdf-page"
    const val DOCUMENT_PDF_PREV = "document-pdf-prev"
    const val DOCUMENT_PDF_NEXT = "document-pdf-next"

    /** A section boundary — where the page geometry changes (#2827). */
    const val DOCUMENT_SECTION_BREAK = "document-section-break"

    /** The editor (#2894 §4): the way in, the surface, and the bar over it. The
     *  text itself lives in a WebView and has no Compose semantics — what can be
     *  asserted from here is the shell around it. */
    const val DOCUMENT_EDIT = "document-edit"
    const val DOCUMENT_EDITOR = "document-editor"
    const val DOCUMENT_EDITOR_CLOSE = "document-editor-close"
    const val DOCUMENT_EDITOR_STATUS = "document-editor-status"
    const val DOCUMENT_EDITOR_RELOAD = "document-editor-reload"
    const val DOCUMENT_EDITOR_FAILED = "document-editor-failed"
    const val DOCUMENT_EDITOR_BLOCKED = "document-editor-blocked"

    /** Annotations (#2894 §5): the way in — from the reader and from the editor
     *  bar — and the sheet itself. */
    const val DOCUMENT_COMMENTS_OPEN = "document-comments-open"
    const val DOCUMENT_COMMENTS = "document-comments"
    const val DOCUMENT_COMMENTS_CLOSE = "document-comments-close"

    /** Open discussions: on the way-in button, and again in the sheet header.
     *  Two tags because a spec asserting the badge must not pass on the header. */
    const val DOCUMENT_COMMENTS_BADGE = "document-comments-badge"
    const val DOCUMENT_COMMENTS_COUNT = "document-comments-count"

    const val DOCUMENT_COMMENTS_EMPTY = "document-comments-empty"

    /** The block a new thread will hang on — the quote, and the way to drop it. */
    const val DOCUMENT_COMMENTS_ANCHOR = "document-comments-anchor"
    const val DOCUMENT_COMMENTS_UNPIN = "document-comments-unpin"

    /** The three composers: a new thread, an answer, and an edit of one's own. */
    const val DOCUMENT_COMMENT_DRAFT = "document-comment-draft"
    const val DOCUMENT_COMMENT_SEND = "document-comment-send"
    const val DOCUMENT_COMMENT_REPLY_DRAFT = "document-comment-reply-draft"
    const val DOCUMENT_COMMENT_REPLY_SEND = "document-comment-reply-send"
    const val DOCUMENT_COMMENT_EDIT_DRAFT = "document-comment-edit-draft"
    const val DOCUMENT_COMMENT_EDIT_SAVE = "document-comment-edit-save"
    const val DOCUMENT_COMMENT_REMOVE_CONFIRM = "document-comment-remove-confirm"

    /** Keyed by the comment they belong to: a thread is asserted by identity,
     *  not by «the second card» — which is exactly what re-ordering changes. */
    fun documentCommentThread(id: String) = "document-comment-thread:$id"

    fun documentCommentReply(id: String) = "document-comment-reply:$id"

    fun documentCommentResolve(id: String) = "document-comment-resolve:$id"

    fun documentCommentResolved(id: String) = "document-comment-resolved:$id"

    fun documentCommentEdit(id: String) = "document-comment-edit:$id"

    fun documentCommentRemove(id: String) = "document-comment-remove:$id"

    /** The version journal (#2894 §6): the way in — from the reader and from the
     *  editor bar — and the panel itself. */
    const val DOCUMENT_HISTORY_OPEN = "document-history-open"
    const val DOCUMENT_HISTORY = "document-history"
    const val DOCUMENT_HISTORY_CLOSE = "document-history-close"
    const val DOCUMENT_HISTORY_EMPTY = "document-history-empty"

    /** Taking a named snapshot: the way in, the name, and the confirm. */
    const val DOCUMENT_SNAPSHOT = "document-snapshot"
    const val DOCUMENT_SNAPSHOT_LABEL = "document-snapshot-label"
    const val DOCUMENT_SNAPSHOT_SAVE = "document-snapshot-save"

    /** The comparison: its heading, the rollback and its confirmation. */
    const val DOCUMENT_HISTORY_DIFF = "document-history-diff"
    const val DOCUMENT_HISTORY_SUMMARY = "document-history-summary"
    const val DOCUMENT_RESTORE = "document-restore"
    const val DOCUMENT_RESTORE_CONFIRM = "document-restore-confirm"

    /** Keyed by the version, like the comment rows: a journal is asserted by
     *  which entry, not by «the second row» — retention changes that. */
    fun documentVersionRow(id: String) = "document-version-row:$id"

    /** Task links and approval routes (#2894 §7): the way in — from the reader
     *  and from the editor bar — and the panel itself. */
    const val DOCUMENT_LINKS_OPEN = "document-links-open"
    const val DOCUMENT_LINKS_BADGE = "document-links-badge"
    const val DOCUMENT_LINKS = "document-links"
    const val DOCUMENT_LINKS_CLOSE = "document-links-close"
    const val DOCUMENT_LINKS_EMPTY = "document-links-empty"

    /** The block a new link is pinned to, and the way to drop that anchor. */
    const val DOCUMENT_LINKS_ANCHOR = "document-links-anchor"
    const val DOCUMENT_LINKS_UNPIN = "document-links-unpin"

    /** Linking a task: the way in and the picker's query. */
    const val DOCUMENT_LINK_ADD = "document-link-add"
    const val DOCUMENT_LINK_QUERY = "document-link-query"
    const val DOCUMENT_LINK_REMOVE_CONFIRM = "document-link-remove-confirm"

    /** Raising a route, and composing it. */
    const val DOCUMENT_APPROVALS_EMPTY = "document-approvals-empty"
    const val DOCUMENT_APPROVAL_RAISE = "document-approval-raise"
    const val DOCUMENT_APPROVAL_COMPOSER = "document-approval-composer"
    const val DOCUMENT_APPROVAL_TITLE = "document-approval-title"
    const val DOCUMENT_APPROVAL_MODE_SEQUENTIAL = "document-approval-mode-sequential"
    const val DOCUMENT_APPROVAL_MODE_PARALLEL = "document-approval-mode-parallel"
    const val DOCUMENT_APPROVAL_SUBMIT = "document-approval-submit"

    /** Signing: the way in, the remark, and the two outcomes. */
    const val DOCUMENT_APPROVAL_SIGN = "document-approval-sign"
    const val DOCUMENT_APPROVAL_COMMENT = "document-approval-comment"
    const val DOCUMENT_APPROVAL_APPROVE = "document-approval-approve"
    const val DOCUMENT_APPROVAL_REJECT = "document-approval-reject"
    const val DOCUMENT_APPROVAL_CANCEL_CONFIRM = "document-approval-cancel-confirm"

    /** Keyed by the row they belong to, for the same reason the versions are:
     *  a link and a route are asserted by which one, never by position. */
    fun documentLinkRow(id: String) = "document-link-row:$id"
    fun documentLinkRemove(id: String) = "document-link-remove:$id"
    fun documentLinkCandidate(taskId: String) = "document-link-candidate:$taskId"
    fun documentApprovalCard(id: String) = "document-approval-card:$id"
    fun documentApprovalStatus(id: String) = "document-approval-status:$id"
    fun documentApprovalStep(id: String) = "document-approval-step:$id"
    fun documentApprovalCancel(id: String) = "document-approval-cancel:$id"
    fun documentApproverRow(userId: String) = "document-approver-row:$userId"

    /** The task modal's «Документы» tab — the other end of the same link row. */
    fun taskDocumentRow(id: String) = "task-document-row:$id"
    fun taskDocumentRemove(id: String) = "task-document-remove:$id"

    /** Templates, file import and export (#2894 §8): the way into the gallery,
     *  the gallery itself, and the picker that lives in it. */
    const val DOCUMENTS_TEMPLATES_OPEN = "documents-templates-open"
    const val DOCUMENT_TEMPLATES = "document-templates"
    const val DOCUMENT_TEMPLATES_CLOSE = "document-templates-close"
    const val DOCUMENT_TEMPLATES_SEARCH = "document-templates-search"
    const val DOCUMENT_TEMPLATES_EMPTY = "document-templates-empty"
    const val DOCUMENT_TEMPLATES_UPLOAD = "document-templates-upload"
    const val DOCUMENT_TEMPLATE_REMOVE_CONFIRM = "document-template-remove-confirm"

    /** What the picker will take — the line that changes when the sidecar is
     *  down, and therefore the one a spec asserts degradation by. */
    const val DOCUMENT_IMPORT_HINT = "document-import-hint"

    /** What the conversion could not carry over — the line a finished import
     *  leaves behind, and the only place the reason appears. */
    const val DOCUMENT_IMPORT_NOTICE = "document-import-notice"

    /** Export lives in the reader's «…» menu; one entry per offered format. */
    const val DOCUMENT_ACTION_EXPORT = "document-action-export"
    fun documentExportFormat(format: String) = "document-export-format:$format"

    /** Gallery rows are keyed by card id — a saved template by its own id, a
     *  built-in by `builtin:<key>` — so a spec names the template it means. */
    fun documentTemplateRow(id: String) = "document-template-row:$id"
    fun documentTemplateUse(id: String) = "document-template-use:$id"
    fun documentTemplateRemove(id: String) = "document-template-remove:$id"

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

    /** The top bar's hamburger — the only way into the sidebar drawer, so a spec
     *  that needs anything in the sidebar starts here. */
    const val TOP_MENU = "top-menu"

    /** The version line in the sidebar footer — a button opening the full
     *  changelog (#2858), and the build stamp only that mode renders (#2859). */
    const val SIDEBAR_VERSION = "sidebar-version"
    const val WHATS_NEW_BUILD = "whats-new-build"

    // ── get started guide (#2860) ──────────────────────────────────────────
    //
    // The card of the running step. What it points at is addressed by the guide's
    // own anchor keys (`util/Tour.kt`, `TourKeys`), not from here: those are read
    // by the overlay at runtime, while a test tag is only ever read by a spec.

    const val TOUR_CARD = "tour-card"
    const val TOUR_NEXT = "tour-next"
    const val TOUR_SKIP = "tour-skip"

    /** «Обучение» in the sidebar footer — the guide's only entry point. */
    const val TOUR_START = "tour-start"
}
