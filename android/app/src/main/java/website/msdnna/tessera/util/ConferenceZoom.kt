package website.msdnna.tessera.util

/*
 * Pinch-zoom on the stage (#2896 §8).
 *
 * A shared desktop is 1920 pixels wide on a 411dp phone: the tile shows it whole
 * and legible to nobody. Zoom is the only thing that makes a shared screen worth
 * watching from a phone at all, so the stage takes a pinch — and what a pinch is
 * allowed to do is decided here, because «изображение уехало и не возвращается»
 * is a clamping bug, not a drawing one.
 */

/** How the stage is currently transformed. */
data class ConfZoom(
    val scale: Float = 1f,
    val x: Float = 0f,
    val y: Float = 0f,
) {
    /** Whether anything is magnified — the reset affordance keys off this. */
    val zoomed: Boolean get() = scale > 1f + ZOOM_EPSILON
}

/** Untouched: what the stage looks like before anyone pinches it. */
val CONF_ZOOM_NONE = ConfZoom()

/** A tile drawn 1:1 — below this there is nothing to pan around. */
const val CONF_ZOOM_MIN = 1f

/**
 * Four times. Enough to read a line of code in a shared IDE, and short of the
 * point where a phone is panning around single letters of a stream it is
 * receiving at the tile's own resolution anyway.
 */
const val CONF_ZOOM_MAX = 4f

private const val ZOOM_EPSILON = 0.001f

/**
 * Folds one pinch-and-drag frame into the transform.
 *
 * [zoomBy] is the gesture's relative scale for this frame (1f = no change), and
 * [panX]/[panY] the finger movement in pixels. [width]/[height] are the tile's,
 * which is what the offsets are clamped against.
 *
 * The clamp is the reason this is a function and not three lines in a modifier:
 * the picture may never be dragged past its own edge, so the offset a pan is
 * allowed is a function of the scale it happens at — and a pinch back out has to
 * pull the offsets in with it, or a stage zoomed in, panned to a corner and
 * zoomed back out settles showing a band of empty tile.
 */
fun confZoomStep(
    current: ConfZoom,
    zoomBy: Float,
    panX: Float,
    panY: Float,
    width: Float,
    height: Float,
): ConfZoom {
    val scale = (current.scale * zoomBy).coerceIn(CONF_ZOOM_MIN, CONF_ZOOM_MAX)
    // Panning is scaled with the picture: at 4× a finger crossing the tile has to
    // cross the source once, not four times, or the far edge is unreachable.
    val x = current.x + panX * scale
    val y = current.y + panY * scale
    return clampZoom(ConfZoom(scale, x, y), width, height)
}

/**
 * Pulls a transform back inside the tile.
 *
 * At 1× the answer is always dead centre: a picture that fills its tile exactly
 * has no slack, and letting it be dragged anyway is how a stage ends up half
 * black with no way back short of leaving the call.
 */
fun clampZoom(zoom: ConfZoom, width: Float, height: Float): ConfZoom {
    if (zoom.scale <= CONF_ZOOM_MIN + ZOOM_EPSILON) return ConfZoom(CONF_ZOOM_MIN, 0f, 0f)
    val slackX = width * (zoom.scale - 1f) / 2f
    val slackY = height * (zoom.scale - 1f) / 2f
    return zoom.copy(
        x = zoom.x.coerceIn(-slackX, slackX),
        y = zoom.y.coerceIn(-slackY, slackY),
    )
}
