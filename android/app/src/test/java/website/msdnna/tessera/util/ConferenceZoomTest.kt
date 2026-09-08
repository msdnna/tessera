package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Pinch-zoom on the stage (#2896 §8).
 *
 * Every test here is about a clamp, because that is the only way this feature
 * fails in practice: «изображение уехало и не возвращается» is a stage panned
 * past its own edge with nothing on screen offering a way back.
 */
class ConferenceZoomTest {
    private val tile = Size(1000f, 600f)

    private data class Size(val w: Float, val h: Float)

    private fun step(from: ConfZoom, zoomBy: Float = 1f, panX: Float = 0f, panY: Float = 0f) =
        confZoomStep(from, zoomBy, panX, panY, tile.w, tile.h)

    // ── the scale ─────────────────────────────────────────────────────────

    @Test
    fun `a spread magnifies the stage`() {
        assertThat(step(CONF_ZOOM_NONE, zoomBy = 2f).scale).isWithin(TOLERANCE).of(2f)
    }

    @Test
    fun `it never goes below life size`() {
        // Pinching in on an untouched tile is what a thumb does by accident; a
        // stage that shrank inside its own frame would be a bug with no gesture
        // to undo it.
        assertThat(step(CONF_ZOOM_NONE, zoomBy = 0.2f).scale).isWithin(TOLERANCE).of(CONF_ZOOM_MIN)
    }

    @Test
    fun `it stops at four times`() {
        assertThat(step(ConfZoom(scale = 3f), zoomBy = 4f).scale).isWithin(TOLERANCE).of(CONF_ZOOM_MAX)
    }

    // ── panning ───────────────────────────────────────────────────────────

    @Test
    fun `a magnified stage can be dragged around`() {
        val moved = step(ConfZoom(scale = 2f), panX = 50f, panY = 30f)
        assertThat(moved.x).isWithin(TOLERANCE).of(100f)
        assertThat(moved.y).isWithin(TOLERANCE).of(60f)
    }

    @Test
    fun `an untouched stage cannot be dragged at all`() {
        // A picture that fills its tile exactly has no slack, and letting a
        // one-finger drag move it is how a stage ends up half black.
        val moved = step(CONF_ZOOM_NONE, panX = 300f, panY = 300f)
        assertThat(moved).isEqualTo(CONF_ZOOM_NONE)
    }

    @Test
    fun `the picture may not be dragged past its own edge`() {
        val moved = step(ConfZoom(scale = 2f), panX = 5000f, panY = 5000f)
        // Half the tile's own size of slack at 2×, in each direction.
        assertThat(moved.x).isWithin(TOLERANCE).of(500f)
        assertThat(moved.y).isWithin(TOLERANCE).of(300f)
    }

    @Test
    fun `zooming back out pulls the offsets in with it`() {
        // A stage magnified, panned into a corner and pinched back out would
        // otherwise settle showing a band of empty tile beside the picture.
        val out = step(ConfZoom(scale = 4f, x = 1500f, y = 900f), zoomBy = 0.25f)
        assertThat(out.scale).isWithin(TOLERANCE).of(CONF_ZOOM_MIN)
        assertThat(out.x).isWithin(TOLERANCE).of(0f)
        assertThat(out.y).isWithin(TOLERANCE).of(0f)
    }

    // ── what the reset affordance keys off ────────────────────────────────

    @Test
    fun `only a magnified stage offers a way back`() {
        assertThat(CONF_ZOOM_NONE.zoomed).isFalse()
        assertThat(ConfZoom(scale = 1.5f).zoomed).isTrue()
        // Floating-point drift from a pinch that ended where it started must not
        // leave a reset button hanging over an untouched picture.
        assertThat(ConfZoom(scale = 1.0000001f).zoomed).isFalse()
    }

    private companion object {
        const val TOLERANCE = 0.0001f
    }
}
