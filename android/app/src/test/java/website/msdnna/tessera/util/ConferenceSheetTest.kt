package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Where a released drag leaves the chat and the roster (#2896 §6, §7).
 *
 * The rule is small and the ways it goes wrong are all invisible in a
 * screenshot: a sheet is either where the thumb meant to put it or it is a
 * control that ignored a gesture, and the difference is arithmetic.
 */
class ConferenceSheetTest {
    // ── a deliberate drag ─────────────────────────────────────────────────

    @Test
    fun `pulling a half sheet up expands it`() {
        assertThat(confSheetSettle(ConfSheetDetent.HALF, dragDp = -120f, velocityDp = 0f))
            .isEqualTo(ConfSheetSettle.FULL)
    }

    @Test
    fun `pushing a half sheet down closes it`() {
        assertThat(confSheetSettle(ConfSheetDetent.HALF, dragDp = 120f, velocityDp = 0f))
            .isEqualTo(ConfSheetSettle.CLOSE)
    }

    @Test
    fun `pushing a full sheet down stops at half`() {
        // One gesture, one step: a chat expanded to read a backlog and then
        // swept shut in a single motion would take the call's controls with it.
        assertThat(confSheetSettle(ConfSheetDetent.FULL, dragDp = 120f, velocityDp = 0f))
            .isEqualTo(ConfSheetSettle.HALF)
    }

    // ── a drag that was not one ───────────────────────────────────────────

    @Test
    fun `a thumb that barely moved changes nothing`() {
        assertThat(confSheetSettle(ConfSheetDetent.HALF, dragDp = -20f, velocityDp = 0f))
            .isEqualTo(ConfSheetSettle.HALF)
        assertThat(confSheetSettle(ConfSheetDetent.FULL, dragDp = 20f, velocityDp = 0f))
            .isEqualTo(ConfSheetSettle.FULL)
    }

    // ── a flick ───────────────────────────────────────────────────────────

    @Test
    fun `a fast flick decides on its own however short it was`() {
        // The ordering that matters: a flick is a *short* gesture by definition,
        // and reading the distance first would answer «ничего не произошло» to
        // the most deliberate thing a thumb can do.
        assertThat(confSheetSettle(ConfSheetDetent.HALF, dragDp = -8f, velocityDp = -1400f))
            .isEqualTo(ConfSheetSettle.FULL)
        assertThat(confSheetSettle(ConfSheetDetent.HALF, dragDp = 8f, velocityDp = 1400f))
            .isEqualTo(ConfSheetSettle.CLOSE)
    }

    @Test
    fun `a flick down from full still only steps down`() {
        assertThat(confSheetSettle(ConfSheetDetent.FULL, dragDp = 4f, velocityDp = 1400f))
            .isEqualTo(ConfSheetSettle.HALF)
    }

    // ── the height mid-drag ───────────────────────────────────────────────

    @Test
    fun `the sheet follows the thumb`() {
        // Half a screen tall, dragged up by a fifth of one.
        assertThat(confSheetDragFraction(ConfSheetDetent.HALF, dragPx = -200f, screenPx = 1000f))
            .isWithin(TOLERANCE).of(0.7f)
    }

    @Test
    fun `it cannot be pushed off the top of the screen`() {
        assertThat(confSheetDragFraction(ConfSheetDetent.FULL, dragPx = -900f, screenPx = 1000f))
            .isWithin(TOLERANCE).of(ConfSheetDetent.FULL.fraction)
    }

    @Test
    fun `it cannot be dragged out from under its own header`() {
        assertThat(confSheetDragFraction(ConfSheetDetent.HALF, dragPx = 900f, screenPx = 1000f))
            .isWithin(TOLERANCE).of(CONF_SHEET_MIN_FRACTION)
    }

    @Test
    fun `an unmeasured screen leaves the sheet where it was`() {
        // The first frame: `BoxWithConstraints` has not reported a height yet, and
        // dividing by it would put the sheet at infinity.
        assertThat(confSheetDragFraction(ConfSheetDetent.HALF, dragPx = -200f, screenPx = 0f))
            .isWithin(TOLERANCE).of(ConfSheetDetent.HALF.fraction)
    }

    @Test
    fun `the expanded sheet still leaves a strip of the call showing`() {
        // Not `1f`: the strip is what says the panel is a sheet and can be pushed
        // back down, rather than a screen you have navigated to.
        assertThat(ConfSheetDetent.FULL.fraction).isLessThan(1f)
        assertThat(ConfSheetDetent.FULL.fraction).isGreaterThan(ConfSheetDetent.HALF.fraction)
    }

    private companion object {
        const val TOLERANCE = 0.0001f
    }
}
