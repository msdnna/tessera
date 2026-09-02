package website.msdnna.tessera.ui.components

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
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
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.CompositingStrategy
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.graphics.graphicsLayer
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
import kotlinx.coroutines.delay
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.tour.LocalTourAnchors
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.TourMode
import website.msdnna.tessera.util.TourSnapshot

private val CardInset = 12.dp
private val CardGap = 14.dp
private val CutPadding = 6.dp

/** How long a step waits for its anchor before giving up on it. The anchor may be
 *  a row that hasn't been laid out yet (the drawer is still sliding open), so this
 *  is generous — but finite, or a step whose target this workspace simply doesn't
 *  have would pin the card to nothing forever. */
private const val AnchorTimeoutMs = 4000L

/**
 * The Get Started guide, drawn (#2860, web `TourOverlay.vue`).
 *
 * Mounted at the top of the app shell, above the board *and* above the task form,
 * because the scenario walks through both. It draws three things: the dimming mask
 * with a cutout around what the step points at, one [SpotlightArrow] per anchor,
 * and the step card.
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

        TourMask(cuts = listOf(target) + extras + cuts)

        val gap = with(density) { CardGap.toPx() }
        val inset = with(density) { CardInset.toPx() }
        // Below the target by default; above it when the card would fall off the
        // bottom — the same rule the one-shot sidebar hint plays by.
        val below = target.bottom + gap
        val fitsBelow = below + cardHeight <= constraints.maxHeight
        val cardTop = if (fitsBelow) below else (target.top - gap - cardHeight).coerceAtLeast(0f)
        val cardEdge = if (fitsBelow) cardTop - 4f else cardTop + cardHeight + 4f

        for (rect in listOf(target) + extras) {
            SpotlightArrow(
                start = Offset(inset + with(density) { 26.dp.toPx() }, cardEdge),
                tip = Offset(
                    rect.center.x.coerceIn(rect.left + 6f, rect.right - 6f),
                    if (rect.top > cardTop) rect.top - 3f else rect.bottom + 3f,
                ),
                target = rect,
                // An arbitrary control (a chip, a card field) is its own width —
                // unlike a full-width sidebar row, there is no side padding to
                // discount, and insetting the ring would cut into the target.
                ringInset = 0.dp,
            )
        }

        Box(
            Modifier
                .offset { IntOffset(0, cardTop.toInt()) }
                .padding(horizontal = CardInset)
                .onSizeChanged { cardHeight = it.height },
        ) {
            TourCard(snapshot, onNext = onNext, onSkip = onSkip)
        }
    }
}

/** The dimming mask: everything but [cuts] goes dark, so the eye has one place to
 *  land. Drawn into an offscreen layer — that is what makes BlendMode.Clear punch
 *  a hole instead of painting black. */
@Composable
private fun TourMask(cuts: List<Rect>) {
    Canvas(
        Modifier
            .fillMaxSize()
            .graphicsLayer(compositingStrategy = CompositingStrategy.Offscreen),
    ) {
        drawRect(Color.Black.copy(alpha = 0.5f))
        val pad = CutPadding.toPx()
        for (rect in cuts) {
            drawRoundRect(
                color = Color.Transparent,
                topLeft = Offset(rect.left - pad, rect.top - pad),
                size = Size(rect.width + pad * 2, rect.height + pad * 2),
                cornerRadius = CornerRadius(RadiusSm.toPx()),
                blendMode = BlendMode.Clear,
            )
        }
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
