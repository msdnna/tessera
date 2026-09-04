package website.msdnna.tessera.ui.tour

import android.graphics.Region
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.asAndroidPath
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.GraphicsMode
import website.msdnna.tessera.ui.components.tourMaskPath

/**
 * That the guide's mask covers the screen and spares what the step points at
 * (#2860 rework, point 4).
 *
 * Asserted on the shape, because that is the only place the bug lived: every unit
 * test of the engine passed while the user's screenshots showed an undimmed sidebar
 * with a step card floating over it. The mask used to be a black rect punched
 * through with `BlendMode.Clear`, which needs an offscreen layer to punch anything
 * at all — so whether it dimmed depended on which surface composed it, and over the
 * project's context menu it dimmed nothing. Nothing about that is visible from
 * "it composed": the broken version composed fine.
 *
 * [Region] resolves the path the way the rasteriser does, even-odd fill and rounded
 * corners included, so `contains` here is the pixel that would be painted — without
 * the screenshot harness (`captureToImage` never gets its redraw under Robolectric).
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class TourMaskTest {
    private val screen = Size(1080f, 2100f)
    private val pad = 12f
    private val radius = 8f

    /** The row the first step of the scenario points at, in the open sidebar. */
    private val target = Rect(40f, 300f, 800f, 380f)

    private fun mask(cuts: List<Rect>): Region {
        val path = tourMaskPath(screen, cuts, pad, radius).asAndroidPath()
        return Region().apply {
            setPath(path, Region(0, 0, screen.width.toInt(), screen.height.toInt()))
        }
    }

    @Test
    fun `everything outside the cutout is covered`() {
        val m = mask(listOf(target))

        // Deep in the sidebar, well below the target, and the far corner of the board.
        assertThat(m.contains(100, 1800)).isTrue()
        assertThat(m.contains(1000, 100)).isTrue()
        // Just outside the padded cutout, on every side.
        assertThat(m.contains(target.center.x.toInt(), (target.top - pad).toInt() - 2)).isTrue()
        assertThat(m.contains(target.center.x.toInt(), (target.bottom + pad).toInt() + 2)).isTrue()
        assertThat(m.contains((target.left - pad).toInt() - 2, target.center.y.toInt())).isTrue()
        assertThat(m.contains((target.right + pad).toInt() + 2, target.center.y.toInt())).isTrue()
    }

    @Test
    fun `the target the step points at is left alone`() {
        val m = mask(listOf(target))

        assertThat(m.contains(target.center.x.toInt(), target.center.y.toInt())).isFalse()
        // The padding is part of the hole: the ring is drawn on that edge, and a
        // mask that stopped at the anchor itself would dim the ring's own pixels.
        assertThat(m.contains(target.left.toInt() - 2, target.center.y.toInt())).isFalse()
    }

    @Test
    fun `a step naming several anchors spares all of them`() {
        // The card-field steps ring one chip and un-dim its neighbours; the mask is
        // one path, so a second hole has to survive the even-odd fill rather than
        // fill the first one back in.
        val second = Rect(40f, 500f, 800f, 580f)
        val m = mask(listOf(target, second))

        assertThat(m.contains(target.center.x.toInt(), target.center.y.toInt())).isFalse()
        assertThat(m.contains(second.center.x.toInt(), second.center.y.toInt())).isFalse()
        // And the gap between them is still covered.
        assertThat(m.contains(target.center.x.toInt(), 440)).isTrue()
    }

    @Test
    fun `the corners of the cutout are rounded, not square`() {
        val m = mask(listOf(target))

        // A point inside the padded bounding box but outside the rounded corner:
        // covered, which is what tells a RoundRect hole from a plain Rect one.
        assertThat(m.contains((target.left - pad).toInt() + 1, (target.top - pad).toInt() + 1)).isTrue()
    }
}
