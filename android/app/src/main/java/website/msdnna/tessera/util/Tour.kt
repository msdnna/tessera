package website.msdnna.tessera.util

import androidx.annotation.StringRes
import website.msdnna.tessera.R

/*
 * The "Get Started" guide (#2860) — the port of the web scenario (#2753/#2759/
 * #2778: data/getStarted.js + stores/tour.js), kept pure so the interesting half
 * (when a step is allowed to advance) is asserted on the JVM rather than through
 * the UI. The layer that draws it is ui/components/TourOverlay.kt, the registry of
 * anchors it points at is ui/tour/TourAnchors.kt.
 *
 * Anchor keys live here, next to the scenario that names them, the way [TestTags]
 * holds the e2e anchors: a key is a contract between a step and the element that
 * carries `Modifier.tourAnchor(key)` — grep for it to find the element.
 *
 * Unlike the web the wording rides ON the step as a resource id. There the texts
 * had to be pulled out of the data (#2799) because the store copies the list into
 * a ref on start and a baked-in string froze the language the guide started in;
 * a @StringRes is resolved by `stringResource` on every recomposition, so the same
 * trap does not exist here (same reasoning as [WhatsNewEntry]).
 *
 * Advancement follows the web's call: a step ends when the entity is actually
 * created, not when a button was pressed — a modal can be cancelled, and the guide
 * must not run ahead of the user. Steps that only open something (a menu, an
 * inline field) have nothing to create, so those do end on the tap itself.
 */

/** Anchor keys named by the scenario. Per-entity anchors are `<key>:<id>`. */
object TourKeys {
    const val WS_SWITCH = "ws-switch"
    const val PROJ_ADD = "proj-add"
    const val MENU_PROJECT = "menu-project"
    const val MENU_GROUP = "menu-group"

    /** «Добавить доску» inside a project's «⋯» menu — the board flow's in-menu
     *  step, the mirror of [MENU_PROJECT]/[MENU_GROUP] the board scenario was
     *  missing (#2860 rework). */
    const val MENU_BOARD = "menu-board"

    /** The inline field that names a new group, the mirror of [PROJECT_NAME]. The
     *  group scenario used to point at the created row via a `{group}` token that
     *  was empty until commit, which resolved to a stray group (#2860 rework). */
    const val GROUP_NAME = "group-name"
    const val PROJECT_NAME = "project-name"
    const val PROJECT_SLUG = "project-slug"
    const val PROJECT_SUBMIT = "project-submit"
    const val BOARD_NAME = "board-name"
    const val BOARD_LAYOUT = "board-layout"
    const val BOARD_ACTIONS = "board-actions"
    const val BOARD_COMPOSER = "board-composer"
    const val BOARD_CUSTOMIZE = "board-customize"
    const val WS_SEARCH = "ws-search"

    /* The card fields are prefixes, not plain keys: every card on the board carries
     * its own set (`card-due:<task>`), so the arrow lands on the one card the step
     * is about instead of on whichever card happened to register last. */
    const val CARD_PRIORITY = "card-priority:"
    const val CARD_DUE = "card-due:"
    const val CARD_TAGS = "card-tags:"
    const val CARD_ASSIGNEES = "card-assignees:"
    const val TM_DUE = "tm-due"
    const val TM_ASSIGNEES = "tm-assignees"
    const val TM_PRIORITY = "tm-priority"
    const val TM_TAGS = "tm-tags"
    const val TM_DESCRIPTION = "tm-description"
    const val TM_TABS = "tm-tabs"
    const val TM_SAVE = "tm-save"
    const val SB_FOOTER = "sb-footer"
    const val FOOTER_SETTINGS = "footer-settings"
    const val FOOTER_NOTIFICATIONS = "footer-notifications"

    /** What adds a board to project [id]. The web has a dedicated «+» on the row;
     *  here it is the row's «⋯» menu, which is where «Добавить доску» lives — hence
     *  the wording of the step differs from the web's (see `tour_board_add_body`). */
    fun boardAdd(id: String) = "board-add:$id"

    /** A board row in the tree. */
    fun boardRow(id: String) = "board-row:$id"

    /** A project row in the tree. Also the element dragged by `dnd-project`. */
    fun projectRow(id: String) = "project-row:$id"

    /** A group row in the tree. */
    fun groupRow(id: String) = "group-row:$id"

    /**
     * «Создать задачу» at the foot of the column *named* [name].
     *
     * By name and not by id, and not as a bare prefix either (#2860 rework). A
     * prefix resolves by position — topmost, then leftmost — and columns sit side by
     * side, so the moment the first one holds a card its «Создать задачу» is pushed
     * *below* the empty neighbour's and the step starts pointing at «В процессе».
     * The board the guide walks the user through creating is seeded server-side with
     * [TOUR_TODO_COLUMN] first (`defaultColumns` in `handlers/boards.go`), so the
     * name is something the scenario can name outright; the id is not.
     */
    fun columnAdd(name: String) = "$COLUMN_ADD$name"

    /** A field of the card [id] — one of the [CARD_PRIORITY] family. */
    fun cardField(prefix: String, id: String) = "$prefix$id"

    /** A card on the board. Its `place` is the name of the column it sits in.
     *  With an empty [id] this is the bare prefix, which the registry resolves to
     *  the first card in composition order — the way the web anchors "the card the
     *  user just created" without knowing its id. */
    fun taskCard(id: String) = "task-card:$id"

    /** A board column, as a drop target to un-dim. */
    fun column(name: String) = "column:$name"

    /** Prefixes the per-entity keys above, for the rules that count or track them. */
    const val PROJECT_ROW = "project-row:"
    const val GROUP_ROW = "group-row:"
    const val BOARD_ROW = "board-row:"
    const val TASK_CARD = "task-card:"
    const val COLUMN_ADD = "column-add:"

    /** A field of the task modal reports itself filled with `<key>:set`. */
    fun set(key: String) = "$key:set"
}

/** «Понятно» + «Пропустить» vs "the user acts, we only watch". */
enum class TourMode { INFO, ACTION }

/**
 * What the host does with the sidebar drawer while a step is up. The web has no
 * such thing — there the sidebar is a permanent column, here it is a drawer, and
 * a step pointing at a project row has nothing to point at while it is closed. The
 * engine drives the drawer itself instead of hoping the user guesses (#2860 plan).
 */
enum class TourSurface {
    /** Open the drawer: the step's anchor lives in the sidebar. */
    DRAWER,

    /** Close it: the step is about the board or the task modal. */
    SCREEN,

    /** Leave it as it is — a dialog is up over whatever opened it. */
    KEEP,
}

/**
 * Where the step card is parked, when adjacency to its target is the wrong place
 * for it (#2860 rework). [AUTO] tucks it under (or over) the target like a tooltip.
 * [BOTTOM]/[TOP] pin it to a screen edge instead — for a target inside a popup menu
 * (the card would be hidden behind that menu's own window) or a drag whose landing
 * zone the card would otherwise sit on. The ring still marks the target; the card
 * just steps out of the way and drops its nub.
 */
enum class TourCardGravity { AUTO, BOTTOM, TOP }

/** How an action step ends. Info steps have none — the user presses «Понятно». */
sealed interface AdvanceOn {
    /** The user tapped the step's own anchor, or [key] when it is a different one. */
    data class Tap(val key: String? = null) : AdvanceOn

    /**
     * More anchors whose key starts with [prefix] exist than when the step opened.
     * The baseline is the first report after the step became current, so re-running
     * the guide on a tree that already has projects still walks the user through
     * creating one.
     */
    data class Count(val prefix: String) : AdvanceOn

    /**
     * The anchor [key] exists at all — the task-modal fields report themselves
     * filled ([TourKeys.set]). Deliberately baseline-free: a count baseline would
     * deadlock the step on a task whose field was already filled, and only
     * «Пропустить» would get the user out (web `advanceOn.set`).
     */
    data class Set(val key: String) : AdvanceOn

    /**
     * The anchor whose key starts with [prefix] changed container. A drag cannot be
     * caught by a tap or by a field carrying a value, and [Count] would lie: "the
     * column now holds one more card" is equally true when the user *created* one
     * there. The tracked anchor reports an address (`place` — the column it sits in,
     * the group a project row lives in) and the step ends once it is a different,
     * non-empty one (web `advanceOn.moved`).
     */
    data class Moved(val prefix: String) : AdvanceOn
}

/**
 * One step of the guide.
 *
 * [anchor] is what the arrow points at and what the mask cuts out; [extra] are the
 * "show, don't ask" arrows drawn from the same card; [cut] is un-dimmed without an
 * arrow (a drop target, a button the user is about to press).
 */
data class TourStep(
    val id: String,
    val anchor: String,
    @StringRes val titleRes: Int,
    @StringRes val bodyRes: Int,
    val mode: TourMode,
    val surface: TourSurface = TourSurface.SCREEN,
    val extra: List<String> = emptyList(),
    val cut: List<String> = emptyList(),
    val advanceOn: AdvanceOn? = null,
    /** Where the card sits — [TourCardGravity.AUTO] unless the target is inside a
     *  menu the card would hide behind, or a drag zone it would cover. */
    val cardGravity: TourCardGravity = TourCardGravity.AUTO,
)

/** Ids of what the user creates while walking the guide, so the steps that follow
 *  point at *that* row rather than the first one in the tree (web `ctx`). */
data class TourContext(
    val projectId: String = "",
    val boardId: String = "",
    val groupId: String = "",
)

const val TOUR_PREFIX = "getstarted:"

/** Walked to the end. Shared with the web — one guide, one acknowledgement space. */
const val TOUR_DONE = TOUR_PREFIX + "done"

/** «Пропустить», from any step. */
const val TOUR_SKIPPED = TOUR_PREFIX + "skipped"

private const val PROJECT_TOKEN = "{project}"
private const val BOARD_TOKEN = "{board}"
private const val GROUP_TOKEN = "{group}"

/*
 * The columns a new board is seeded with, by name: the scenario points the first
 * «Создайте задачу» at the first one and asks for the card to be dragged into the
 * second. Column names come from the server and are not localised (`defaultColumns`
 * in handlers/boards.go), so the constants that match them stay Russian — hence the
 * `i18n-data` marker, the same one the column-matching patterns elsewhere carry.
 */
const val TOUR_TODO_COLUMN = "К работе" // i18n-data
const val TOUR_DOING_COLUMN = "В процессе" // i18n-data

/**
 * The scenario, in order. Mirrors `frontend/src/data/getStarted.js` step for step;
 * where Android has no equivalent surface the difference is called out on the step.
 */
val GET_STARTED: List<TourStep> = listOf(
    TourStep(
        id = "workspaces",
        anchor = TourKeys.WS_SWITCH,
        titleRes = R.string.tour_workspaces_title,
        bodyRes = R.string.tour_workspaces_body,
        mode = TourMode.INFO,
        surface = TourSurface.DRAWER,
    ),
    TourStep(
        id = "tree-add",
        anchor = TourKeys.PROJ_ADD,
        titleRes = R.string.tour_tree_add_title,
        bodyRes = R.string.tour_tree_add_body,
        mode = TourMode.ACTION,
        surface = TourSurface.DRAWER,
        advanceOn = AdvanceOn.Tap(),
    ),
    TourStep(
        id = "menu-project",
        anchor = TourKeys.MENU_PROJECT,
        titleRes = R.string.tour_menu_project_title,
        bodyRes = R.string.tour_menu_project_body,
        mode = TourMode.ACTION,
        surface = TourSurface.DRAWER,
        advanceOn = AdvanceOn.Tap(),
        // The menu is its own window on top of the shell; a card tucked under the
        // ringed item would hide behind it. Parked at the bottom, clear of it.
        cardGravity = TourCardGravity.BOTTOM,
    ),
    TourStep(
        id = "project-create",
        anchor = TourKeys.PROJECT_NAME,
        titleRes = R.string.tour_project_create_title,
        bodyRes = R.string.tour_project_create_body,
        mode = TourMode.ACTION,
        // The dialog is up over the drawer that opened it — neither is ours to move.
        surface = TourSurface.KEEP,
        extra = listOf(TourKeys.PROJECT_SLUG),
        // Un-dim «Создать» too, so the user isn't left staring at a control they
        // are supposed to press (web #2753 rework).
        cut = listOf(TourKeys.PROJECT_SUBMIT),
        advanceOn = AdvanceOn.Count(TourKeys.PROJECT_ROW),
    ),
    TourStep(
        id = "board-add",
        // Scoped to the project the user just created, so the arrow lands on its
        // «+» and not the first project's (web #2753 rework).
        anchor = TourKeys.boardAdd(PROJECT_TOKEN),
        titleRes = R.string.tour_board_add_title,
        bodyRes = R.string.tour_board_add_body,
        mode = TourMode.ACTION,
        surface = TourSurface.DRAWER,
        advanceOn = AdvanceOn.Tap(),
    ),
    TourStep(
        // The «⋯» tap opened the menu; this step lives inside it, the mirror of
        // `menu-project`. Without it the board flow tapped «⋯» and jumped straight
        // to `board-create`, whose name field does not exist while the menu is
        // open — so the overlay had nothing to point at and dimmed nothing, which
        // read as «маска пропадает вовсе» over the context menu (#2860 rework).
        id = "board-menu",
        anchor = TourKeys.MENU_BOARD,
        titleRes = R.string.tour_board_menu_title,
        bodyRes = R.string.tour_board_menu_body,
        mode = TourMode.ACTION,
        surface = TourSurface.DRAWER,
        advanceOn = AdvanceOn.Tap(),
        cardGravity = TourCardGravity.BOTTOM,
    ),
    TourStep(
        id = "board-create",
        anchor = TourKeys.BOARD_NAME,
        titleRes = R.string.tour_board_create_title,
        bodyRes = R.string.tour_board_create_body,
        mode = TourMode.ACTION,
        surface = TourSurface.KEEP,
        advanceOn = AdvanceOn.Count(TourKeys.BOARD_ROW),
    ),
    TourStep(
        id = "board-open",
        anchor = TourKeys.boardRow(BOARD_TOKEN),
        titleRes = R.string.tour_board_open_title,
        bodyRes = R.string.tour_board_open_body,
        mode = TourMode.ACTION,
        surface = TourSurface.DRAWER,
        advanceOn = AdvanceOn.Tap(),
    ),
    TourStep(
        id = "task-create",
        // «Создать задачу» of the first column of the freshly seeded board, named
        // outright — see [TourKeys.columnAdd] for why not a prefix.
        anchor = TourKeys.columnAdd(TOUR_TODO_COLUMN),
        titleRes = R.string.tour_task_create_title,
        bodyRes = R.string.tour_task_create_body,
        mode = TourMode.ACTION,
        advanceOn = AdvanceOn.Count(TourKeys.TASK_CARD),
    ),

    // ── 6. What can be set on the card itself ────────────────────────────────
    TourStep(
        id = "card-fields",
        anchor = TourKeys.CARD_PRIORITY,
        titleRes = R.string.tour_card_fields_title,
        bodyRes = R.string.tour_card_fields_body,
        mode = TourMode.INFO,
        extra = listOf(TourKeys.CARD_DUE, TourKeys.CARD_TAGS, TourKeys.CARD_ASSIGNEES),
    ),

    // ── 7. Open the card ─────────────────────────────────────────────────────
    TourStep(
        id = "card-open",
        anchor = TourKeys.taskCard(""),
        titleRes = R.string.tour_card_open_title,
        bodyRes = R.string.tour_card_open_body,
        mode = TourMode.ACTION,
        advanceOn = AdvanceOn.Tap(),
    ),

    // ── 8. Fill the task in ──────────────────────────────────────────────────
    TourStep(
        id = "tm-due",
        anchor = TourKeys.TM_DUE,
        titleRes = R.string.tour_tm_due_title,
        bodyRes = R.string.tour_tm_due_body,
        mode = TourMode.ACTION,
        advanceOn = AdvanceOn.Set(TourKeys.set(TourKeys.TM_DUE)),
    ),
    TourStep(
        id = "tm-assignees",
        anchor = TourKeys.TM_ASSIGNEES,
        titleRes = R.string.tour_tm_assignees_title,
        bodyRes = R.string.tour_tm_assignees_body,
        mode = TourMode.ACTION,
        advanceOn = AdvanceOn.Set(TourKeys.set(TourKeys.TM_ASSIGNEES)),
    ),
    TourStep(
        id = "tm-priority",
        anchor = TourKeys.TM_PRIORITY,
        titleRes = R.string.tour_tm_priority_title,
        bodyRes = R.string.tour_tm_priority_body,
        mode = TourMode.ACTION,
        advanceOn = AdvanceOn.Set(TourKeys.set(TourKeys.TM_PRIORITY)),
    ),
    TourStep(
        id = "tm-tags",
        anchor = TourKeys.TM_TAGS,
        titleRes = R.string.tour_tm_tags_title,
        bodyRes = R.string.tour_tm_tags_body,
        mode = TourMode.ACTION,
        advanceOn = AdvanceOn.Set(TourKeys.set(TourKeys.TM_TAGS)),
    ),
    TourStep(
        id = "tm-description",
        anchor = TourKeys.TM_DESCRIPTION,
        titleRes = R.string.tour_tm_description_title,
        bodyRes = R.string.tour_tm_description_body,
        mode = TourMode.ACTION,
        advanceOn = AdvanceOn.Set(TourKeys.set(TourKeys.TM_DESCRIPTION)),
    ),

    // ── 9. Tabs and saving ───────────────────────────────────────────────────
    TourStep(
        id = "tm-tabs",
        anchor = TourKeys.TM_TABS,
        titleRes = R.string.tour_tm_tabs_title,
        bodyRes = R.string.tour_tm_tabs_body,
        mode = TourMode.INFO,
    ),
    TourStep(
        id = "tm-save",
        anchor = TourKeys.TM_SAVE,
        titleRes = R.string.tour_tm_save_title,
        bodyRes = R.string.tour_tm_save_body,
        mode = TourMode.ACTION,
        advanceOn = AdvanceOn.Tap(),
    ),

    // ── 10. Board tools ──────────────────────────────────────────────────────
    TourStep(
        id = "board-tools",
        anchor = TourKeys.BOARD_LAYOUT,
        titleRes = R.string.tour_board_tools_title,
        bodyRes = R.string.tour_board_tools_body,
        mode = TourMode.INFO,
        extra = listOf(TourKeys.WS_SEARCH, TourKeys.BOARD_ACTIONS),
    ),
    TourStep(
        id = "board-composer",
        anchor = TourKeys.BOARD_COMPOSER,
        titleRes = R.string.tour_board_composer_title,
        bodyRes = R.string.tour_board_composer_body,
        mode = TourMode.INFO,
        extra = listOf(TourKeys.BOARD_CUSTOMIZE),
    ),

    // ── 10.5 Drag and drop (web #2778) ───────────────────────────────────────
    TourStep(
        id = "dnd-card",
        anchor = TourKeys.taskCard(""),
        titleRes = R.string.tour_dnd_card_title,
        bodyRes = R.string.tour_dnd_card_body,
        mode = TourMode.ACTION,
        // Don't dim the column we ask the card to be dropped into.
        cut = listOf(TourKeys.column(TOUR_DOING_COLUMN)),
        advanceOn = AdvanceOn.Moved(TourKeys.TASK_CARD),
    ),
    TourStep(
        id = "group-add",
        anchor = TourKeys.PROJ_ADD,
        titleRes = R.string.tour_group_add_title,
        bodyRes = R.string.tour_group_add_body,
        mode = TourMode.ACTION,
        surface = TourSurface.DRAWER,
        advanceOn = AdvanceOn.Tap(),
    ),
    TourStep(
        id = "menu-group",
        anchor = TourKeys.MENU_GROUP,
        titleRes = R.string.tour_menu_group_title,
        bodyRes = R.string.tour_menu_group_body,
        mode = TourMode.ACTION,
        surface = TourSurface.DRAWER,
        advanceOn = AdvanceOn.Tap(),
        cardGravity = TourCardGravity.BOTTOM,
    ),
    TourStep(
        // Points at the inline name field, the mirror of `project-create` — not at
        // the created row via a `{group}` token. That token was empty until the
        // group was committed, and an empty tail resolves by prefix to the topmost
        // existing group, so the arrow sat on a stray group and offered «Понятно»
        // before the user had made anything (#2860 rework). As an action step it
        // shows no «Понятно» and ends when a new group row actually appears.
        id = "group-create",
        anchor = TourKeys.GROUP_NAME,
        titleRes = R.string.tour_group_created_title,
        bodyRes = R.string.tour_group_created_body,
        mode = TourMode.ACTION,
        surface = TourSurface.DRAWER,
        advanceOn = AdvanceOn.Count(TourKeys.GROUP_ROW),
    ),
    TourStep(
        id = "dnd-project",
        anchor = TourKeys.projectRow(PROJECT_TOKEN),
        titleRes = R.string.tour_dnd_project_title,
        bodyRes = R.string.tour_dnd_project_body,
        mode = TourMode.ACTION,
        surface = TourSurface.DRAWER,
        cut = listOf(TourKeys.groupRow(GROUP_TOKEN)),
        // `{group}` is deliberately NOT required here: the step is about the tree
        // being draggable, and a drop into any group closes it. Demanding the one
        // just created would lock a user who missed it in with «Пропустить».
        advanceOn = AdvanceOn.Moved(TourKeys.PROJECT_ROW),
        // The drop zone is the tree itself; a card under the dragged row would sit
        // on the group the user has to drop into. Parked at the top, out of it.
        cardGravity = TourCardGravity.TOP,
    ),

    // ── 11. The other sections ───────────────────────────────────────────────
    TourStep(
        id = "nav-sections",
        anchor = "nav-notes",
        titleRes = R.string.tour_nav_sections_title,
        bodyRes = R.string.tour_nav_sections_body,
        mode = TourMode.INFO,
        surface = TourSurface.DRAWER,
        extra = listOf("nav-documents", "nav-reminders"),
    ),
    TourStep(
        id = "nav-footer",
        anchor = TourKeys.FOOTER_SETTINGS,
        titleRes = R.string.tour_nav_footer_title,
        bodyRes = R.string.tour_nav_footer_body,
        mode = TourMode.INFO,
        surface = TourSurface.DRAWER,
        extra = listOf(TourKeys.FOOTER_NOTIFICATIONS),
    ),

    // ── 12. The end ──────────────────────────────────────────────────────────
    TourStep(
        id = "done",
        anchor = TourKeys.SB_FOOTER,
        titleRes = R.string.tour_done_title,
        bodyRes = R.string.tour_done_body,
        mode = TourMode.INFO,
        surface = TourSurface.DRAWER,
    ),
)

/**
 * Walks a list of steps: which one is current, when it may advance, how the run
 * ended. Deliberately free of Compose and of Android — the ViewModel holds one of
 * these and republishes [snapshot] after every call, the tests drive it directly.
 *
 * [onAck] records the outcome (`getstarted:done` / `getstarted:skipped`); the key
 * space is shared with the web, so a guide walked there does not greet the user
 * again here.
 */
class TourEngine(private val onAck: (String) -> Unit = {}) {
    private var steps: List<TourStep> = emptyList()
    private var index: Int = -1
    private var context: TourContext = TourContext()

    var active: Boolean = false
        private set

    /** Baseline for [AdvanceOn.Count] — the first report after the step opened. */
    private var entryCount: Int? = null

    /** Same for [AdvanceOn.Moved]. `null` while nothing has been reported yet; an
     *  empty address is a legitimate baseline (a project at the root of the tree
     *  sits in no group at all), hence the separate flag. */
    private var entryPlace: String? = null
    private var placeSeen = false

    val current: TourStep? get() = if (active) steps.getOrNull(index) else null

    val isLast: Boolean get() = index >= steps.size - 1

    fun snapshot(): TourSnapshot {
        val step = current ?: return TourSnapshot()
        return TourSnapshot(
            active = true,
            step = step,
            index = index,
            total = steps.size,
            isLast = isLast,
            anchors = (listOf(step.anchor) + step.extra).map(::resolve),
            cut = step.cut.map(::resolve),
        )
    }

    /** Expands the `{project}` / `{board}` / `{group}` tokens. A token with no id yet
     *  collapses to an empty tail, which matches no registered anchor — the step
     *  waits for the entity instead of grabbing a stray row. */
    fun resolve(key: String): String = key
        .replace(PROJECT_TOKEN, context.projectId)
        .replace(BOARD_TOKEN, context.boardId)
        .replace(GROUP_TOKEN, context.groupId)

    /** The host reports the entity it just created, so the arrow lands on it. */
    fun noteCreated(next: TourContext.() -> TourContext) {
        if (!active) return
        context = context.next()
    }

    fun start(list: List<TourStep> = GET_STARTED, fromId: String? = null): Boolean {
        if (list.isEmpty()) return false
        steps = list
        index = fromId?.let { id -> list.indexOfFirst { it.id == id } }?.takeIf { it >= 0 } ?: 0
        active = true
        arm()
        return true
    }

    fun stop() {
        active = false
        index = -1
        steps = emptyList()
        context = TourContext()
        arm()
    }

    private fun arm() {
        entryCount = null
        entryPlace = null
        placeSeen = false
    }

    fun next() {
        if (!active) return
        if (isLast) {
            finish()
            return
        }
        index += 1
        arm()
    }

    /** Walked to the end. */
    fun finish() {
        if (!active) return
        stop()
        onAck(TOUR_DONE)
    }

    /** «Пропустить» — the whole guide, from any step. A guide abandoned halfway is
     *  not resumed on the next launch; it stays available from the sidebar. */
    fun skip() {
        if (!active) return
        stop()
        onAck(TOUR_SKIPPED)
    }

    /** The step's anchor never showed up (the user navigated away, the section
     *  isn't in this workspace). Move on rather than leave a card pinned to
     *  nothing. */
    fun anchorMissing(stepId: String) {
        if (current?.id != stepId) return
        next()
    }

    /**
     * The user tapped an anchored element.
     *
     * A step may name a bare prefix (`task-card:` — "the card you just made",
     * whichever id it got), and the tap always arrives under the full per-entity
     * key. So a declared prefix matches by prefix. Deliberately keyed off what the
     * step *declares*, not off the resolved string: a `{project}` token with no id
     * yet also collapses to a trailing `:`, and matching that by prefix would let a
     * tap on any row of that kind run the step ahead of the user.
     */
    fun tapped(key: String) {
        val step = current ?: return
        if (step.mode != TourMode.ACTION) return
        val on = step.advanceOn as? AdvanceOn.Tap ?: return
        val declared = on.key ?: step.anchor
        val hit = if (declared.endsWith(":")) key.startsWith(declared) else key == resolve(declared)
        if (hit) next()
    }

    /** How many anchors currently match the step's [AdvanceOn.Count] prefix (or
     *  whether its [AdvanceOn.Set] key exists — 1 or 0). */
    fun counted(n: Int) {
        val step = current ?: return
        if (step.mode != TourMode.ACTION) return
        when (step.advanceOn) {
            is AdvanceOn.Set -> if (n > 0) next()

            is AdvanceOn.Count -> {
                val base = entryCount
                if (base == null) entryCount = n else if (n > base) next()
            }

            else -> Unit
        }
    }

    /** Where the anchor tracked by an [AdvanceOn.Moved] step currently sits. The
     *  first report after the step opened is the baseline; the step ends once the
     *  address is a *different* one. An empty address never ends it: mid-drag the
     *  card is lifted out of its column for a frame, and advancing on that would
     *  close the step while it is still in the air. */
    fun located(place: String?) {
        val step = current ?: return
        if (step.mode != TourMode.ACTION || step.advanceOn !is AdvanceOn.Moved) return
        val at = place.orEmpty()
        if (!placeSeen) {
            placeSeen = true
            entryPlace = at
            return
        }
        if (at.isEmpty()) return
        if (at != entryPlace) next()
    }
}

/** What the overlay draws, recomputed after every engine call. */
data class TourSnapshot(
    val active: Boolean = false,
    val step: TourStep? = null,
    val index: Int = -1,
    val total: Int = 0,
    val isLast: Boolean = false,
    /** Resolved: the primary anchor first — the arrow head, the cutout and the
     *  "did it ever show up?" timeout all key off it. */
    val anchors: List<String> = emptyList(),
    val cut: List<String> = emptyList(),
)
