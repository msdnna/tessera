package website.msdnna.tessera.ui.components

import androidx.compose.ui.geometry.Rect
import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Where the Get Started card lands and which way its nub points (#2860 rework).
 *
 * The card used to be placed against the viewport alone and drawn with a long
 * curved arrow to the target; on a phone that arrow read as a scribble and the card
 * happily parked itself under the keyboard. The rules that replaced it live in
 * [tourCardLayout] precisely so they can be asserted here rather than eyeballed on
 * a screenshot: a tooltip nub is only honest if the card is really on that side of
 * the target.
 *
 * Numbers are overlay pixels: a 1080-wide viewport, a card 12px in from each edge.
 */
class TourOverlayLayoutTest {
    private val left = 12f
    private val right = 1068f
    private val gap = 25f
    private val margin = 16f
    private val cardHeight = 200f

    private fun layout(target: Rect, bottomLimit: Float = 2000f) = tourCardLayout(
        target = target,
        cardHeight = cardHeight,
        cardLeft = left,
        cardRight = right,
        bottomLimit = bottomLimit,
        gap = gap,
        nubMargin = margin,
    )

    @Test
    fun `the card sits below the target when there is room, nub up`() {
        val result = layout(Rect(100f, 300f, 400f, 380f))

        assertThat(result.nubSide).isEqualTo(TourNubSide.BELOW)
        assertThat(result.top).isEqualTo(380f + gap)
        // Centred on the target, which is what makes the nub point at it.
        assertThat(result.nubCenterX).isEqualTo(250f)
    }

    @Test
    fun `a target near the bottom flips the card above it, nub down`() {
        val result = layout(Rect(100f, 1700f, 400f, 1780f), bottomLimit = 1900f)

        assertThat(result.nubSide).isEqualTo(TourNubSide.ABOVE)
        assertThat(result.top).isEqualTo(1700f - gap - cardHeight)
    }

    @Test
    fun `the keyboard pushes the card off the target it would have covered`() {
        val target = Rect(100f, 900f, 400f, 980f)
        // Nothing in the way: below, as usual.
        assertThat(layout(target).nubSide).isEqualTo(TourNubSide.BELOW)

        // Same target, same viewport, keyboard up — the space below is gone, so the
        // card goes above instead of hiding behind the keys (#2860 rework, point 3).
        val withIme = layout(target, bottomLimit = 1000f)
        assertThat(withIme.nubSide).isEqualTo(TourNubSide.ABOVE)
        assertThat(withIme.top).isEqualTo(900f - gap - cardHeight)
    }

    @Test
    fun `with room on neither side the card is parked and grows no nub`() {
        // A target that fills the space left over by the keyboard: below does not
        // fit, above does not fit. A nub here would point at nothing.
        val result = layout(Rect(100f, 10f, 400f, 900f), bottomLimit = 1000f)

        assertThat(result.nubSide).isEqualTo(TourNubSide.NONE)
        assertThat(result.top).isEqualTo(800f)
    }

    @Test
    fun `the nub never slides onto the card's rounded corners`() {
        // A target hard against the left edge of the screen…
        assertThat(layout(Rect(0f, 300f, 30f, 380f)).nubCenterX).isEqualTo(left + margin)
        // …and one hard against the right.
        assertThat(layout(Rect(1050f, 300f, 1080f, 380f)).nubCenterX).isEqualTo(right - margin)
    }
}
