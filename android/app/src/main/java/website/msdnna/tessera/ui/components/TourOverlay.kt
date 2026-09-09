package website.msdnna.tessera.ui.components

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.RoundRect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathFillType
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.layout.positionOnScreen
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlin.math.max
import kotlin.math.min
import kotlinx.coroutines.delay
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.tour.LocalTourAnchors
import website.msdnna.tessera.ui.tour.LocalTourSnapshot
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.TourCardGravity
import website.msdnna.tessera.util.TourMode
import website.msdnna.tessera.util.TourSnapshot

private val CardInset = 12.dp
private val CardGap = 10.dp
private val CutPadding = 6.dp

/** The tooltip nub: how wide its base is, how far it sticks out, and how close to
 *  the card's rounded corner it is allowed to slide. */
private val NubWidth = 18.dp
private val NubHeight = 9.dp
private val NubMargin = 16.dp

/** How long a step waits for its anchor before giving up on it. The anchor may be
 *  a row that hasn't been laid out yet (the drawer is still sliding open), so this
 *  is generous — but finite, or a step whose target this workspace simply doesn't
 *  have would pin the card to nothing forever. */
private const val AnchorTimeoutMs = 4000L

/** Where the step card ended up relative to what it points at — which edge carries
 *  the nub, or neither when the card had to be parked (a target under the keyboard,
 *  a card taller than the space left over). */
enum class TourNubSide { ABOVE, BELOW, NONE }

/** The geometry of one step, in the overlay's own pixels. Computed by
 *  [tourCardLayout] so the placement rules can be asserted on the JVM — they are the
 *  half of this file a screenshot test would be a poor way to pin down. */
data class TourCardLayout(val top: Float, val nubSide: TourNubSide, val nubCenterX: Float)

/**
 * Places the step card against [target] the way a tooltip places itself.
 *
 * Below the target when it fits, above it otherwise, and pinned to the bottom of
 * what is left when neither works. [bottomLimit] is the last usable row of pixels —
 * the viewport minus the keyboard, because a card the keyboard covers is a card the
 * user cannot press «Понятно» on (#2860 rework, point 3).
 *
 * The nub sits on the edge facing the target and is centred on it, clamped so it
 * cannot slide onto the card's rounded corners. When the card is parked there is
 * nothing honest to point at, so it gets no nub at all rather than one aimed at
 * whatever happens to be under it.
 */
fun tourCardLayout(
    target: Rect,
    cardHeight: Float,
    cardLeft: Float,
    cardRight: Float,
    bottomLimit: Float,
    gap: Float,
    nubMargin: Float,
    topLimit: Float = 0f,
    gravity: TourCardGravity = TourCardGravity.AUTO,
): TourCardLayout {
    // Pinned to an edge, out of the way of a popup menu or a drag zone — the ring
    // still marks the target, so the card carries no nub of its own (#2860 rework).
    when (gravity) {
        TourCardGravity.BOTTOM ->
            return TourCardLayout((bottomLimit - cardHeight).coerceAtLeast(topLimit), TourNubSide.NONE, 0f)

        TourCardGravity.TOP ->
            return TourCardLayout(topLimit, TourNubSide.NONE, 0f)

        TourCardGravity.AUTO -> Unit
    }
    val below = target.bottom + gap
    val above = target.top - gap - cardHeight
    val side = when {
        below + cardHeight <= bottomLimit -> TourNubSide.BELOW
        above >= 0f -> TourNubSide.ABOVE
        else -> TourNubSide.NONE
    }
    val top = when (side) {
        TourNubSide.BELOW -> below
        TourNubSide.ABOVE -> above
        TourNubSide.NONE -> (bottomLimit - cardHeight).coerceAtLeast(0f)
    }
    // A card narrower than its own corners is not a thing that happens, but the
    // clamp has to survive it rather than produce a reversed range.
    val slack = (cardRight - cardLeft - 2 * nubMargin).coerceAtLeast(0f)
    val lo = cardLeft + (cardRight - cardLeft - slack) / 2
    return TourCardLayout(top, side, target.center.x.coerceIn(lo, lo + slack))
}

/**
 * The Get Started guide, drawn (#2860, web `TourOverlay.vue`).
 *
 * Mounted at the top of the app shell, above the board *and* above the task form,
 * because the scenario walks through both. It draws three things: the dimming mask
 * with a cutout around what the step points at, a tooltip nub from the card to that
 * cutout, and the step card itself.
 *
 * Nothing here consumes the pointer except the card itself — the mask is a
 * drawing, not a shield (`pointer-events: none` on the web). An action step ends
 * because the user did the thing, so the thing has to stay reachable.
 */
@Composable
fun TourOverlay(
    snapshot: TourSnapshot,
    onNext: () -> Unit,
    onSkip: () -> Unit,
    onAnchorMissing: (String) -> Unit,
) {
    val step = snapshot.step ?: return
    val anchors = LocalTourAnchors.current
    val density = LocalDensity.current
    var origin by remember { mutableStateOf(Offset.Zero) }
    var cardHeight by remember { mutableIntStateOf(0) }
    // Read through the inset, not through `imePadding()`: the mask has to keep
    // covering the whole surface while the card moves out of the keyboard's way.
    val imeBottom = WindowInsets.ime.getBottom(density)

    BoxWithConstraints(
        Modifier
            .fillMaxSize()
            .onGloballyPositioned { origin = it.positionOnScreen() },
    ) {
        // Anchors are measured in screen coordinates (see TourAnchors) — bring them
        // into this overlay's own space, wherever on the screen its window sits.
        fun local(key: String): Rect? =
            anchors.find(key)?.rect?.translate(-origin.x, -origin.y)

        val target = local(snapshot.anchors.firstOrNull().orEmpty())
        val extras = snapshot.anchors.drop(1).mapNotNull(::local)
        val cuts = snapshot.cut.mapNotNull(::local)

        // The anchor never showed up: move on rather than leave the card pinned to
        // nothing. Re-armed per step, and cancelled the moment the rect arrives.
        LaunchedEffect(step.id, target == null) {
            if (target != null) return@LaunchedEffect
            delay(AnchorTimeoutMs)
            onAnchorMissing(step.id)
        }
        if (target == null) return@BoxWithConstraints

        // Anchors that stand next to each other are one highlighted block — a single
        // rounded cutout and ring, rather than each row/pill outlined on its own
        // (#2860 rework, point 2). Ones that don't (the bell in the header, the gear
        // in the footer) keep a square each: merging those would light up the whole
        // sidebar between them. `cut` stays separate either way — it un-dims a drop
        // target or a button, not part of the block.
        val rings = tourRings(target, extras, group = step.groupAnchors)
        TourMask(rings = rings, cuts = cuts)
        // The card is placed against the block it points at; without grouping that is
        // the step's primary anchor, and the extras are only lit, not aimed at.
        val region = rings.first()

        val inset = with(density) { CardInset.toPx() }
        val gap = with(density) { CardGap.toPx() + NubHeight.toPx() + CutPadding.toPx() }
        val layout = tourCardLayout(
            target = region,
            cardHeight = cardHeight.toFloat(),
            cardLeft = inset,
            cardRight = constraints.maxWidth - inset,
            bottomLimit = (constraints.maxHeight - imeBottom).toFloat(),
            gap = gap,
            nubMargin = with(density) { NubMargin.toPx() },
            topLimit = WindowInsets.statusBars.getTop(density).toFloat(),
            gravity = step.cardGravity,
        )

        Box(
            Modifier
                .offset { IntOffset(0, layout.top.toInt()) }
                .padding(horizontal = CardInset)
                .onSizeChanged { cardHeight = it.height },
        ) {
            TourCard(snapshot, onNext = onNext, onSkip = onSkip)
        }
        if (layout.nubSide != TourNubSide.NONE && cardHeight > 0) {
            TourNub(
                side = layout.nubSide,
                centerX = layout.nubCenterX,
                cardTop = layout.top,
                cardBottom = layout.top + cardHeight,
            )
        }
    }
}

/**
 * What the step outlines: one block covering [target] and [extras] when they belong
 * together ([group]), otherwise a square each, primary first.
 *
 * Grouping is per step because adjacency is: the sidebar's sections and a card's
 * pills sit shoulder to shoulder and read as one thing, while the notifications bell
 * lives in the header and the settings gear in the footer — the rect enclosing those
 * two is the whole sidebar (#2860 review).
 */
fun tourRings(target: Rect, extras: List<Rect>, group: Boolean): List<Rect> =
    if (group) listOf(boundingRect(listOf(target) + extras)) else listOf(target) + extras

/** The smallest rect covering all of [rects] — a step's primary anchor and its
 *  extras highlighted as one block instead of each outlined alone (#2860 rework). */
fun boundingRect(rects: List<Rect>): Rect {
    var l = rects.first().left
    var t = rects.first().top
    var r = rects.first().right
    var b = rects.first().bottom
    for (rect in rects) {
        l = min(l, rect.left)
        t = min(t, rect.top)
        r = max(r, rect.right)
        b = max(b, rect.bottom)
    }
    return Rect(l, t, r, b)
}

/** A cutout: the anchor's rect, grown by [pad] on every side and rounded, which is
 *  what both the hole in the mask and the ring around it are drawn from. */
fun tourCutout(rect: Rect, pad: Float, radius: Float) = RoundRect(
    Rect(rect.left - pad, rect.top - pad, rect.right + pad, rect.bottom + pad),
    CornerRadius(radius),
)

/**
 * The mask's silhouette: the whole [size], minus a cutout over each of [cuts].
 *
 * One even-odd path rather than a black rect punched through with `BlendMode.Clear`:
 * clearing only works inside an offscreen layer, which makes the mask depend on
 * where in the hierarchy it happens to be composed — and the guide draws itself from
 * three different surfaces (#2860 rework, point 4). A path has no such dependency.
 *
 * Pulled out of the drawing so the shape can be asserted directly (`TourMaskTest`):
 * this is the one part of the overlay where "it composed without throwing" says
 * nothing at all — the version this replaces composed fine and dimmed nothing.
 */
fun tourMaskPath(size: Size, cuts: List<Rect>, pad: Float, radius: Float): Path =
    Path().apply {
        fillType = PathFillType.EvenOdd
        addRect(Rect(Offset.Zero, size))
        for (rect in cuts) addRoundRect(tourCutout(rect, pad, radius))
    }

/**
 * The dimming mask: everything but the cutouts goes dark, so the eye has one place
 * to land, and every rect in [rings] — what the step actually points at — is outlined.
 */
@Composable
private fun TourMask(rings: List<Rect>, cuts: List<Rect>) {
    val c = Tessera.colors
    Canvas(Modifier.fillMaxSize()) {
        val pad = CutPadding.toPx()
        val radius = RadiusSm.toPx()
        drawPath(
            tourMaskPath(size, rings + cuts, pad, radius),
            Color.Black.copy(alpha = 0.5f),
        )
        for (ring in rings) {
            drawPath(
                Path().apply { addRoundRect(tourCutout(ring, pad, radius)) },
                color = c.primary,
                style = Stroke(width = 2.dp.toPx()),
            )
        }
    }
}

/**
 * Dims and rings the guide's target *inside a menu popup* (#2860 rework, points
 * 1-2). The shell's overlay lives in the activity window, so a [website.msdnna.
 * tessera.ui.components.TDropdown] — its own window, drawn on top — hid it: the
 * ring landed behind the menu and the neighbouring item stayed lit. This paints
 * in the menu's own window, over the rows but consuming nothing (a [Canvas] takes
 * no pointer input), so the ringed row stays tappable while the rest goes dark.
 *
 * Only when the step's target actually falls inside this menu. A menu opened for
 * another reason mid-guide, or one whose target sits out in the shell (the «⋯» a
 * step rings before its item exists), is left untouched.
 */
@Composable
fun BoxScope.TourMenuScrim() {
    val snapshot = LocalTourSnapshot.current
    val anchors = LocalTourAnchors.current
    if (!snapshot.active || snapshot.step == null) return
    val key = snapshot.anchors.firstOrNull().orEmpty()
    val c = Tessera.colors
    var origin by remember { mutableStateOf(Offset.Zero) }
    Canvas(
        Modifier.matchParentSize().onGloballyPositioned { origin = it.positionOnScreen() },
    ) {
        val target = anchors.find(key)?.rect?.translate(-origin.x, -origin.y) ?: return@Canvas
        // Ours only if the target lands within this menu's bounds — the same rect
        // in screen space, brought local by subtracting this canvas's origin.
        if (!Rect(Offset.Zero, size).overlaps(target)) return@Canvas
        val pad = CutPadding.toPx()
        val radius = RadiusSm.toPx()
        drawPath(tourMaskPath(size, listOf(target), pad, radius), Color.Black.copy(alpha = 0.5f))
        drawPath(
            Path().apply { addRoundRect(tourCutout(target, pad, radius)) },
            color = c.primary,
            style = Stroke(width = 2.dp.toPx()),
        )
    }
}

/** The tooltip's nub, on the card edge that faces the target. Its base overlaps the
 *  card by a hair so the card's own border does not show through as a seam. */
@Composable
private fun TourNub(side: TourNubSide, centerX: Float, cardTop: Float, cardBottom: Float) {
    val c = Tessera.colors
    val density = LocalDensity.current
    val h = with(density) { NubHeight.toPx() }
    val halfWidth = with(density) { NubWidth.toPx() } / 2
    val up = side == TourNubSide.BELOW
    val top = if (up) cardTop - h else cardBottom
    Canvas(
        Modifier
            .offset { IntOffset((centerX - halfWidth).toInt(), top.toInt()) }
            .size(NubWidth, NubHeight),
    ) {
        val w = size.width
        val tipY = if (up) 0f else size.height
        val baseY = if (up) size.height else 0f
        val overlap = if (up) 1.dp.toPx() else -1.dp.toPx()
        drawPath(
            Path().apply {
                moveTo(0f, baseY)
                lineTo(w / 2, tipY)
                lineTo(w, baseY)
                lineTo(w, baseY + overlap)
                lineTo(0f, baseY + overlap)
                close()
            },
            color = c.surface,
        )
        drawPath(
            Path().apply {
                moveTo(0f, baseY)
                lineTo(w / 2, tipY)
                lineTo(w, baseY)
            },
            color = c.primary.copy(alpha = 0.55f),
            style = Stroke(width = 1.dp.toPx()),
        )
    }
}

/** Title, body and the step's controls. An info step offers «Понятно» (or «Готово»
 *  on the last one); an action step only offers «Пропустить» — it ends when the
 *  user does the thing it asks for. */
@Composable
private fun TourCard(snapshot: TourSnapshot, onNext: () -> Unit, onSkip: () -> Unit) {
    val step = snapshot.step ?: return
    val c = Tessera.colors
    Column(
        Modifier
            .popupAppear(TransformOrigin(0.1f, 0f))
            .fillMaxWidth()
            .softShadow(RoundedCornerShape(RadiusMd), elevation = 8.dp)
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.surface)
            .border(1.dp, c.primary.copy(alpha = 0.55f), RoundedCornerShape(RadiusMd))
            .padding(horizontal = 14.dp, vertical = 13.dp)
            .testTag(TestTags.TOUR_CARD),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier.size(20.dp).clip(CircleShape).background(accentGradient(c.primary)),
                contentAlignment = Alignment.Center,
            ) {
                IonIcon(Ion.STAR, size = 12.dp, tint = c.onPrimary)
            }
            Spacer(Modifier.width(8.dp))
            Text(
                stringResource(step.titleRes),
                color = c.text1, fontSize = 13.sp, fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.weight(1f))
            Text("${snapshot.index + 1}/${snapshot.total}", color = c.text3, fontSize = 11.sp)
        }
        Spacer(Modifier.height(6.dp))
        Text(stringResource(step.bodyRes), color = c.text3, fontSize = 12.sp, lineHeight = 17.sp)
        Spacer(Modifier.height(11.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
            if (step.mode == TourMode.INFO) {
                TButton(
                    stringResource(if (snapshot.isLast) R.string.common_done else R.string.common_got_it),
                    onClick = onNext,
                    modifier = Modifier.testTag(TestTags.TOUR_NEXT),
                )
            }
            TButton(
                stringResource(R.string.tour_skip),
                onClick = onSkip,
                kind = TButtonKind.Ghost,
                modifier = Modifier.testTag(TestTags.TOUR_SKIP),
            )
        }
    }
}
