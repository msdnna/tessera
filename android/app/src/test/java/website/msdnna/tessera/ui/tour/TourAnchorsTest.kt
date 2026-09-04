package website.msdnna.tessera.ui.tour

import androidx.compose.ui.geometry.Rect
import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * The anchor registry of the Get Started guide (#2860).
 *
 * The rules that pick *which* element a step points at live here rather than in the
 * engine, and the scenario leans on them wherever it names a family rather than one
 * element: «Откройте задачу» points at the bare `task-card:` prefix and must land on
 * the card the user just made, and the card-field steps point at `card-priority:`
 * and must land on the topmost card. Resolved by position, not by insertion order —
 * a hash map promises nothing about the latter.
 *
 * «Создайте задачу» used to be resolved this way too and no longer is: columns sit
 * side by side, so "topmost, then leftmost" walked off to the neighbouring column
 * the moment the first one held a card (#2860 rework, point 5). It names its column
 * outright now; the prefix rules below are still what the other steps ride on.
 */
class TourAnchorsTest {
    private fun at(left: Float, top: Float) = Rect(left, top, left + 100f, top + 40f)

    @Test
    fun `an exact key wins over the prefix scan`() {
        val anchors = TourAnchors()
        anchors.put("task-card:a", TourAnchor(at(0f, 100f)))
        anchors.put("task-card:b", TourAnchor(at(0f, 0f)))

        assertThat(anchors.find("task-card:a")?.rect).isEqualTo(at(0f, 100f))
    }

    @Test
    fun `a prefix resolves to the topmost anchor, then the leftmost`() {
        val anchors = TourAnchors()
        anchors.put("column-add:third", TourAnchor(at(600f, 400f)))
        anchors.put("column-add:first", TourAnchor(at(0f, 400f)))
        anchors.put("column-add:second", TourAnchor(at(300f, 400f)))

        assertThat(anchors.find("column-add:")?.rect).isEqualTo(at(0f, 400f))

        // A row above outranks anything to its left.
        anchors.put("column-add:above", TourAnchor(at(900f, 10f)))
        assertThat(anchors.find("column-add:")?.rect).isEqualTo(at(900f, 10f))
    }

    @Test
    fun `a key that is not a prefix and has no exact match resolves to nothing`() {
        val anchors = TourAnchors()
        anchors.put("tm-due", TourAnchor(at(0f, 0f)))

        assertThat(anchors.find("tm-dueXX")).isNull()
        // An empty key is what an unresolved `{project}` token collapses to: the step
        // must wait for the entity, not grab the first row it can see.
        assertThat(anchors.find("")).isNull()
    }

    @Test
    fun `count is what the create-something steps watch`() {
        val anchors = TourAnchors()
        assertThat(anchors.count("project-row:")).isEqualTo(0)

        anchors.put("project-row:a", TourAnchor(at(0f, 0f)))
        anchors.put("project-row:b", TourAnchor(at(0f, 40f)))
        anchors.put("group-row:g", TourAnchor(at(0f, 80f)))

        assertThat(anchors.count("project-row:")).isEqualTo(2)
        // A field of the task form reports itself filled by existing at all.
        assertThat(anchors.count("tm-due:set")).isEqualTo(0)
        anchors.put("tm-due:set", TourAnchor(at(0f, 0f)))
        assertThat(anchors.count("tm-due:set")).isEqualTo(1)
    }

    @Test
    fun `the tracked anchor reports the container it sits in`() {
        val anchors = TourAnchors()
        anchors.put("task-card:a", TourAnchor(at(0f, 0f), place = "К работе"))

        assertThat(anchors.placeOf("task-card:")).isEqualTo("К работе")

        // The same card re-registers after the drop — that is the whole signal the
        // `dnd-card` step gets, since a move destroys and re-lays-out nothing else.
        anchors.put("task-card:a", TourAnchor(at(300f, 0f), place = "В процессе"))
        assertThat(anchors.placeOf("task-card:")).isEqualTo("В процессе")
    }

    @Test
    fun `an anchor that leaves composition stops being found`() {
        val anchors = TourAnchors()
        anchors.put("board-row:a", TourAnchor(at(0f, 0f)))
        anchors.remove("board-row:a")

        assertThat(anchors.find("board-row:a")).isNull()
        assertThat(anchors.count("board-row:")).isEqualTo(0)
        assertThat(anchors.placeOf("board-row:")).isNull()
    }
}
