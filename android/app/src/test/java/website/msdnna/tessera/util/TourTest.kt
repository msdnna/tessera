package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * The Get Started engine (#2860) — when a step is allowed to end, and how the run
 * is recorded. Pure logic, so the cases that matter (a cancelled modal, a card
 * lifted mid-drag, a field that was already filled) are asserted here instead of
 * through an emulator.
 *
 * The scenario itself is exercised through a small stand-in list: the shipped
 * [GET_STARTED] is walked by the last test only, for the invariants every step has
 * to hold to.
 */
class TourTest {
    private val acks = mutableListOf<String>()
    private fun engine() = TourEngine(onAck = acks::add)

    // The texts are resource ids and this tier has no resources — the engine only
    // ever compares ids and anchor keys, so any distinct ints will do.
    private fun step(
        id: String,
        anchor: String,
        mode: TourMode = TourMode.ACTION,
        advanceOn: AdvanceOn? = null,
    ) = TourStep(id, anchor, titleRes = 1, bodyRes = 2, mode = mode, advanceOn = advanceOn)

    private val tap = step("tap", "proj-add", advanceOn = AdvanceOn.Tap())
    private val info = step("info", "ws-switch", mode = TourMode.INFO)

    @Test
    fun `starts on the first step`() {
        val e = engine()
        assertThat(e.start(listOf(tap, info))).isTrue()
        assertThat(e.current?.id).isEqualTo("tap")
        assertThat(e.snapshot().active).isTrue()
        assertThat(e.snapshot().total).isEqualTo(2)
    }

    @Test
    fun `an empty scenario never starts`() {
        assertThat(engine().start(emptyList())).isFalse()
    }

    @Test
    fun `resumes from a named step`() {
        val e = engine()
        e.start(listOf(tap, info), fromId = "info")
        assertThat(e.current?.id).isEqualTo("info")
        // An id that is no longer in the scenario falls back to the top rather than
        // leaving the guide on nothing.
        val other = engine()
        other.start(listOf(tap, info), fromId = "gone")
        assertThat(other.current?.id).isEqualTo("tap")
    }

    // ── tap ─────────────────────────────────────────────────────────────────

    @Test
    fun `a tap on the step's own anchor advances it`() {
        val e = engine()
        e.start(listOf(tap, info))
        e.tapped("proj-add")
        assertThat(e.current?.id).isEqualTo("info")
    }

    @Test
    fun `a tap somewhere else does not`() {
        val e = engine()
        e.start(listOf(tap, info))
        e.tapped("nav-notes")
        assertThat(e.current?.id).isEqualTo("tap")
    }

    @Test
    fun `an info step ignores taps — it ends on «Понятно»`() {
        val e = engine()
        e.start(listOf(info, tap))
        e.tapped("ws-switch")
        assertThat(e.current?.id).isEqualTo("info")
        e.next()
        assertThat(e.current?.id).isEqualTo("tap")
    }

    @Test
    fun `a tap rule may name an anchor other than the step's own`() {
        val e = engine()
        e.start(listOf(step("x", "board-name", advanceOn = AdvanceOn.Tap("board-submit")), info))
        e.tapped("board-name")
        assertThat(e.current?.id).isEqualTo("x")
        e.tapped("board-submit")
        assertThat(e.current?.id).isEqualTo("info")
    }

    @Test
    fun `a step anchored on a bare prefix ends on a tap on any of its family`() {
        // «Откройте задачу» points at `task-card:` — the card the user has just
        // made, whose id the scenario cannot know — while the tap arrives under the
        // full key. Comparing the two for equality never matched, so the step never
        // ended: the guide stayed up over the opened form and the user could not get
        // past it (#2860 rework, point 6).
        val e = engine()
        e.start(listOf(step("open", "task-card:", advanceOn = AdvanceOn.Tap()), info))

        e.tapped("board-row:b-1")
        assertThat(e.current?.id).isEqualTo("open")
        e.tapped("task-card:t-42")
        assertThat(e.current?.id).isEqualTo("info")
    }

    @Test
    fun `an unresolved token does not turn a step into a prefix match`() {
        // `board-row:{board}` with no board created yet collapses to a trailing
        // colon too — but that step is about one specific row, so a tap on any other
        // must not run it ahead of the user.
        val e = engine()
        e.start(listOf(step("open", "board-row:{board}", advanceOn = AdvanceOn.Tap()), info))

        e.tapped("board-row:someone-elses")
        assertThat(e.current?.id).isEqualTo("open")

        e.noteCreated { copy(boardId = "b-9") }
        e.tapped("board-row:b-9")
        assertThat(e.current?.id).isEqualTo("info")
    }

    // ── count ───────────────────────────────────────────────────────────────

    @Test
    fun `count takes the first report as its baseline and ends on growth`() {
        val e = engine()
        e.start(listOf(step("create", "project-name", advanceOn = AdvanceOn.Count("project-row:")), info))
        // The tree already holds two projects — re-running the guide must still walk
        // the user through creating one.
        e.counted(2)
        e.counted(2)
        assertThat(e.current?.id).isEqualTo("create")
        e.counted(3)
        assertThat(e.current?.id).isEqualTo("info")
    }

    @Test
    fun `a cancelled modal leaves the step where it was`() {
        val e = engine()
        e.start(listOf(step("create", "project-name", advanceOn = AdvanceOn.Count("project-row:")), info))
        e.counted(1)
        e.tapped("project-name") // opened the dialog, then closed it empty
        e.counted(1)
        assertThat(e.current?.id).isEqualTo("create")
    }

    @Test
    fun `the baseline is re-armed for the next step`() {
        val first = step("a", "x", advanceOn = AdvanceOn.Count("project-row:"))
        val second = step("b", "y", advanceOn = AdvanceOn.Count("board-row:"))
        val e = engine()
        e.start(listOf(first, second, info))
        e.counted(0)
        e.counted(1)
        assertThat(e.current?.id).isEqualTo("b")
        // Had the baseline survived, this report (1 > 0) would end step b too.
        e.counted(1)
        assertThat(e.current?.id).isEqualTo("b")
        e.counted(2)
        assertThat(e.current?.id).isEqualTo("info")
    }

    // ── set ─────────────────────────────────────────────────────────────────

    @Test
    fun `set ends on any match, with no baseline`() {
        val e = engine()
        e.start(listOf(step("due", "tm-due", advanceOn = AdvanceOn.Set("tm-due:set")), info))
        // The field was already filled when the step opened: a baseline here would
        // deadlock it, and only «Пропустить» would get the user out.
        e.counted(1)
        assertThat(e.current?.id).isEqualTo("info")
    }

    @Test
    fun `set waits while nothing matches`() {
        val e = engine()
        e.start(listOf(step("due", "tm-due", advanceOn = AdvanceOn.Set("tm-due:set")), info))
        e.counted(0)
        e.counted(0)
        assertThat(e.current?.id).isEqualTo("due")
    }

    // ── moved ───────────────────────────────────────────────────────────────

    @Test
    fun `moved ends once the address changes`() {
        val e = engine()
        e.start(listOf(step("dnd", "task-card:", advanceOn = AdvanceOn.Moved("task-card:")), info))
        e.located("К работе")
        e.located("К работе")
        assertThat(e.current?.id).isEqualTo("dnd")
        e.located("В процессе")
        assertThat(e.current?.id).isEqualTo("info")
    }

    @Test
    fun `a card lifted mid-drag does not end the step`() {
        val e = engine()
        e.start(listOf(step("dnd", "task-card:", advanceOn = AdvanceOn.Moved("task-card:")), info))
        e.located("К работе")
        // Between the columns the card belongs to none — advancing here would close
        // the step while it is still in the air.
        e.located(null)
        e.located("")
        assertThat(e.current?.id).isEqualTo("dnd")
    }

    @Test
    fun `an empty address is a legitimate baseline`() {
        val e = engine()
        e.start(listOf(step("dnd", "project-row:", advanceOn = AdvanceOn.Moved("project-row:")), info))
        // A project at the root of the tree sits in no group at all.
        e.located(null)
        e.located("group-7")
        assertThat(e.current?.id).isEqualTo("info")
    }

    @Test
    fun `a count report does not move a moved step`() {
        val e = engine()
        e.start(listOf(step("dnd", "task-card:", advanceOn = AdvanceOn.Moved("task-card:")), info))
        e.counted(9)
        assertThat(e.current?.id).isEqualTo("dnd")
    }

    // ── anchors and context ─────────────────────────────────────────────────

    @Test
    fun `a step whose anchor never appears is skipped`() {
        val e = engine()
        e.start(listOf(tap, info))
        e.anchorMissing("tap")
        assertThat(e.current?.id).isEqualTo("info")
    }

    @Test
    fun `a stale timeout from a step already left is ignored`() {
        val e = engine()
        e.start(listOf(tap, info))
        e.tapped("proj-add")
        e.anchorMissing("tap")
        assertThat(e.current?.id).isEqualTo("info")
    }

    @Test
    fun `anchors point at the entity the user created, not the first in the tree`() {
        val e = engine()
        e.start(listOf(step("open", TourKeys.boardRow("{board}"), advanceOn = AdvanceOn.Tap()), info))
        // Before the board exists the token collapses to a key no anchor carries,
        // so the step waits instead of grabbing a stray row.
        assertThat(e.snapshot().anchors).containsExactly("board-row:")
        e.noteCreated { copy(boardId = "b-9") }
        assertThat(e.snapshot().anchors).containsExactly("board-row:b-9")
        e.tapped("board-row:b-9")
        assertThat(e.current?.id).isEqualTo("info")
    }

    @Test
    fun `the snapshot carries the extra arrows and the cutouts, resolved`() {
        val s = TourStep(
            id = "s", anchor = "a", titleRes = 1, bodyRes = 2, mode = TourMode.INFO,
            extra = listOf("b"), cut = listOf(TourKeys.groupRow("{group}")),
        )
        val e = engine()
        e.start(listOf(s))
        e.noteCreated { copy(groupId = "g-1") }
        assertThat(e.snapshot().anchors).containsExactly("a", "b").inOrder()
        assertThat(e.snapshot().cut).containsExactly("group-row:g-1")
    }

    @Test
    fun `context is dropped when the guide ends`() {
        val e = engine()
        e.start(listOf(step("open", TourKeys.projectRow("{project}"), advanceOn = AdvanceOn.Tap())))
        e.noteCreated { copy(projectId = "p-1") }
        e.skip()
        e.start(listOf(step("open", TourKeys.projectRow("{project}"), advanceOn = AdvanceOn.Tap())))
        assertThat(e.snapshot().anchors).containsExactly("project-row:")
    }

    // ── outcome ─────────────────────────────────────────────────────────────

    @Test
    fun `walking to the end records done`() {
        val e = engine()
        e.start(listOf(info))
        assertThat(e.snapshot().isLast).isTrue()
        e.next()
        assertThat(e.snapshot().active).isFalse()
        assertThat(acks).containsExactly(TOUR_DONE)
    }

    @Test
    fun `skipping records skipped, from any step`() {
        val e = engine()
        e.start(listOf(tap, info))
        e.skip()
        assertThat(e.snapshot().active).isFalse()
        assertThat(acks).containsExactly(TOUR_SKIPPED)
    }

    @Test
    fun `a stopped guide records nothing and answers nothing`() {
        val e = engine()
        e.start(listOf(tap, info))
        e.stop()
        e.next()
        e.skip()
        e.tapped("proj-add")
        assertThat(e.snapshot().active).isFalse()
        assertThat(acks).isEmpty()
    }

    // ── the shipped scenario ────────────────────────────────────────────────

    @Test
    fun `every step has a unique id and an anchor`() {
        assertThat(GET_STARTED.map { it.id }).containsNoDuplicates()
        assertThat(GET_STARTED.filter { it.anchor.isBlank() }).isEmpty()
    }

    @Test
    fun `action steps carry a rule and info steps do not`() {
        val actionsWithoutRule = GET_STARTED.filter { it.mode == TourMode.ACTION && it.advanceOn == null }
        assertThat(actionsWithoutRule.map { it.id }).isEmpty()
        val infoWithRule = GET_STARTED.filter { it.mode == TourMode.INFO && it.advanceOn != null }
        assertThat(infoWithRule.map { it.id }).isEmpty()
    }

    @Test
    fun `the group step points at its name field, not a stray group row`() {
        // #2860 rework, point 5: the step used to anchor `group-row:{group}`, and
        // with no group yet the empty token left a bare `group-row:` prefix that
        // resolved to the topmost existing group — a stray arrow and a premature
        // «Понятно». It now points at the inline name field and ends on creation.
        val group = GET_STARTED.first { it.id == "group-create" }
        assertThat(group.anchor).isEqualTo(TourKeys.GROUP_NAME)
        assertThat(group.mode).isEqualTo(TourMode.ACTION)
        assertThat(group.advanceOn).isEqualTo(AdvanceOn.Count(TourKeys.GROUP_ROW))
        // No step anchors an unresolved group-row token any more.
        assertThat(GET_STARTED.map { it.anchor }).doesNotContain(TourKeys.groupRow("{group}"))
    }

    @Test
    fun `the board flow guides the in-menu step, mirroring the project flow`() {
        // #2860 rework, point 2: tapping «⋯» jumped straight to `board-create`,
        // whose name field does not exist while the context menu is open, so the
        // overlay dimmed nothing. A `board-menu` step now sits inside the menu.
        val ids = GET_STARTED.map { it.id }
        val add = ids.indexOf("board-add")
        val menu = ids.indexOf("board-menu")
        val create = ids.indexOf("board-create")
        assertThat(menu).isGreaterThan(add)
        assertThat(create).isGreaterThan(menu)
        assertThat(GET_STARTED[menu].anchor).isEqualTo(TourKeys.MENU_BOARD)
    }

    @Test
    fun `menu and drag steps park their card clear of the target`() {
        // #2860 rework: a card tucked under a menu item hides behind the menu's own
        // window; a card under the dragged row sits on the group to drop into.
        fun gravity(id: String) = GET_STARTED.first { it.id == id }.cardGravity
        assertThat(gravity("menu-project")).isEqualTo(TourCardGravity.BOTTOM)
        assertThat(gravity("menu-group")).isEqualTo(TourCardGravity.BOTTOM)
        assertThat(gravity("board-menu")).isEqualTo(TourCardGravity.BOTTOM)
        assertThat(gravity("dnd-project")).isEqualTo(TourCardGravity.TOP)
    }

    @Test
    fun `a moved step ends on a place reported straight from the drop`() {
        // #2860 rework, point 4: a drop into a collapsed group unmounts the moved
        // row, so its new place is never registered. The drop handler reports the
        // landing place directly (LocalTourMoved → located), which must still end
        // the step — exactly one non-empty report after the baseline.
        val e = engine()
        val moved = step("m", "project-row:x", advanceOn = AdvanceOn.Moved("project-row:"))
        e.start(listOf(moved, info))
        e.located("") // baseline: the project sat at the tree root
        e.located("g1") // dropped into a group, reported without a laid-out row
        assertThat(e.current?.id).isEqualTo("info")
    }

    @Test
    fun `the scenario can be walked from end to end`() {
        val e = engine()
        e.start()
        val seen = mutableListOf<String>()
        // Every step is walked the only way that always works — its anchor never
        // showing up — which is also the path a workspace missing a section takes.
        repeat(GET_STARTED.size) {
            val id = e.current?.id ?: return@repeat
            seen += id
            e.anchorMissing(id)
        }
        assertThat(seen).isEqualTo(GET_STARTED.map { it.id })
        assertThat(e.snapshot().active).isFalse()
        assertThat(acks).containsExactly(TOUR_DONE)
    }
}
