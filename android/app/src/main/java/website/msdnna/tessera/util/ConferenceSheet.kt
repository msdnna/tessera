package website.msdnna.tessera.util

/*
 * The rules the in-call sheets run on (#2896 §7, §6).
 *
 * The chat and the roster are the two things somebody reads *during* a call, and
 * a sheet that covers the whole screen to show four messages hides the very
 * meeting it belongs to. So they open at half height and grow to full on a pull,
 * the way an M3 bottom sheet does — and where a drag lets go is decided here,
 * away from the composable, because it is the part that is wrong in ways a
 * screenshot does not show.
 */

/** How much of the screen an in-call sheet takes. */
enum class ConfSheetDetent(val fraction: Float) {
    /** Half the screen: the call stays visible above the sheet. */
    HALF(0.5f),

    /**
     * Nearly all of it. Not `1f`: the strip of call left showing is what says
     * the sheet is a sheet and can be pushed back down — a panel flush with the
     * status bar reads as a screen you have navigated to, and people look for a
     * Back gesture that would leave the call.
     */
    FULL(0.94f),
}

/** What a released drag does to the sheet. */
enum class ConfSheetSettle {
    HALF,
    FULL,
    CLOSE,
}

/**
 * Below this the sheet is not a sheet any more — a drag that would leave a strip
 * of chat too short to read is a dismissal that has not admitted it yet.
 */
const val CONF_SHEET_MIN_FRACTION = 0.18f

/** A drag shorter than this is a slip of the thumb, not an intent. */
const val CONF_SHEET_DRAG_DP = 56f

/** Past this a flick decides on its own, however short it was (dp per second). */
const val CONF_SHEET_FLING_DP = 700f

/**
 * Where a released drag leaves the sheet.
 *
 * [dragDp] is the whole gesture, positive downwards; [velocityDp] is what it was
 * doing when the thumb came off, in dp per second.
 *
 * Velocity is read before distance, and that ordering is the whole point: a flick
 * is a short gesture by definition, and a rule that measured the distance first
 * would answer «ничего не произошло» to the fastest, most deliberate thing a
 * thumb can do.
 *
 * Down from [ConfSheetDetent.FULL] lands on [ConfSheetDetent.HALF] rather than
 * closing — one gesture, one step. A chat expanded to read a backlog and then
 * flicked shut in a single motion would take the call's own controls with it.
 */
fun confSheetSettle(from: ConfSheetDetent, dragDp: Float, velocityDp: Float): ConfSheetSettle = when {
    velocityDp <= -CONF_SHEET_FLING_DP -> ConfSheetSettle.FULL

    velocityDp >= CONF_SHEET_FLING_DP -> stepDown(from)

    dragDp <= -CONF_SHEET_DRAG_DP -> ConfSheetSettle.FULL

    dragDp >= CONF_SHEET_DRAG_DP -> stepDown(from)

    // Neither far enough nor fast enough: back where it came from, which is what
    // makes an abandoned drag feel like nothing happened rather than like a
    // control that missed.
    from == ConfSheetDetent.FULL -> ConfSheetSettle.FULL

    else -> ConfSheetSettle.HALF
}

private fun stepDown(from: ConfSheetDetent): ConfSheetSettle =
    if (from == ConfSheetDetent.FULL) ConfSheetSettle.HALF else ConfSheetSettle.CLOSE

/**
 * The height a sheet is drawn at mid-drag, as a share of the screen.
 *
 * Clamped at both ends so the gesture cannot push the sheet off the top of the
 * screen or drag it out from under its own header: past [ConfSheetDetent.FULL]
 * there is nothing left to uncover, and below [CONF_SHEET_MIN_FRACTION] the
 * release is going to close it anyway.
 */
fun confSheetDragFraction(from: ConfSheetDetent, dragPx: Float, screenPx: Float): Float {
    if (screenPx <= 0f) return from.fraction
    val moved = from.fraction - dragPx / screenPx
    return moved.coerceIn(CONF_SHEET_MIN_FRACTION, ConfSheetDetent.FULL.fraction)
}
