package website.msdnna.tessera.e2e

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.ui.TestTags

/**
 * Grouping the kanban by tags — the feature Tessera exists for (CLAUDE.md: lanes
 * become tags, «этого нет у аналогов — не ломать»). A regression here is the one
 * that matters most, and it is invisible to the other specs: they all run on the
 * default status lanes, which keep working when tag lanes break.
 *
 * The assertions are about *where each card landed*, not merely that lanes
 * appeared. Lane tags alone would still pass if every card fell into «Без тега» —
 * i.e. with the grouping switched on and doing nothing.
 */
@RunWith(RobolectricTestRunner::class)
class TagGroupingE2eTest {
    private val e2e = E2eRule()
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `switching to tag grouping lays the cards out by their tags`() {
        val fixture = e2e.fixture
        val backend = E2eBackend.createTag(fixture, "app::backend")
        val android = E2eBackend.createTag(fixture, "app::android")

        val tagged = E2eBackend.createTask(fixture, "backend card ${System.nanoTime()}")
        val untagged = E2eBackend.createTask(fixture, "untagged card ${System.nanoTime()}")
        E2eBackend.addTaskTag(fixture, tagged.id, backend.id)

        compose.setBoardContent(fixture)
        compose.selectGrouping(TestTags.BOARD_GROUP_TAGS)

        // A lane per project tag, plus the trailing catch-all — and the status
        // columns are gone, which is what «columns = tags» means.
        compose.awaitTag(TestTags.boardColumn(backend.id))
        compose.awaitTag(TestTags.boardColumn(android.id))
        compose.awaitTag(TestTags.boardColumn(UNTAGGED_LANE))
        compose.onNodeWithTag(TestTags.boardColumn(fixture.firstColumn.id)).assertDoesNotExist()

        // Each card under its own lane. Scoping by ancestor is the whole point:
        // an unscoped `taskCard` assertion passes wherever the card ended up.
        compose.awaitCardInLane(backend.id, tagged.id)
        compose.awaitCardInLane(UNTAGGED_LANE, untagged.id)
    }

    @Test
    fun `narrowing to a tag namespace keeps only that namespace's lanes`() {
        val fixture = e2e.fixture
        val android = E2eBackend.createTag(fixture, "app::android")
        val bug = E2eBackend.createTag(fixture, "type::bug")

        val appCard = E2eBackend.createTask(fixture, "android card ${System.nanoTime()}")
        val bugCard = E2eBackend.createTask(fixture, "bug card ${System.nanoTime()}")
        E2eBackend.addTaskTag(fixture, appCard.id, android.id)
        E2eBackend.addTaskTag(fixture, bugCard.id, bug.id)

        compose.setBoardContent(fixture)
        compose.selectGrouping(TestTags.boardGroupTagPrefix(APP_NAMESPACE))

        compose.awaitTag(TestTags.boardColumn(android.id))
        // The other namespace's tag is not a lane in this mode…
        compose.onNodeWithTag(TestTags.boardColumn(bug.id)).assertDoesNotExist()
        // …and its card is not dropped from the board, it falls into «Без тега»,
        // which here means «no tag *in this namespace*». Asserting the card's
        // presence (rather than just the missing lane) is what separates a
        // narrowed board from one that quietly lost half its cards.
        compose.awaitCardInLane(UNTAGGED_LANE, bugCard.id)
        compose.awaitCardInLane(android.id, appCard.id)
    }

    @Test
    fun `switching back to statuses restores the status lanes`() {
        val fixture = e2e.fixture
        val tag = E2eBackend.createTag(fixture, "app::android")
        val task = E2eBackend.createTask(fixture, "there and back ${System.nanoTime()}")
        E2eBackend.addTaskTag(fixture, task.id, tag.id)

        compose.setBoardContent(fixture)
        compose.selectGrouping(TestTags.BOARD_GROUP_TAGS)
        compose.awaitTag(TestTags.boardColumn(tag.id))

        compose.selectGrouping(TestTags.BOARD_GROUP_STATUS)

        // Back to the seeded columns, with the card in the one it was created in.
        fixture.columns.forEach { compose.awaitTag(TestTags.boardColumn(it.id)) }
        compose.onNodeWithTag(TestTags.boardColumn(tag.id)).assertDoesNotExist()
        compose.awaitCardInLane(fixture.firstColumn.id, task.id)
    }

    private companion object {
        /** Lane id of the trailing catch-all built by `BoardViews.tagLanes`. */
        const val UNTAGGED_LANE = "none"

        /** Namespace of «app::…» tags, as `tagNamespace` derives it (separator included). */
        const val APP_NAMESPACE = "app::"
    }
}
