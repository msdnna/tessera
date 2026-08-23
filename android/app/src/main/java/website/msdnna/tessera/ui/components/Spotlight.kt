package website.msdnna.tessera.ui.components

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathMeasure
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlin.math.hypot
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.WhatsNewSpotlight

private val CardGap = 12.dp
private val CardInset = 10.dp

/** Where the arrow tip lands: past the row's icon, on its label — not at the far
 *  right of a full-width nav row, which would read as pointing at nothing. */
private val TipInset = 44.dp
private val HeadLength = 11.dp

/**
 * The one-shot hint pointing at a sidebar item (#2766, web `SidebarSpotlight.vue`):
 * a card with a curved gradient arrow that draws itself towards the item, which
 * pulses in place. Shown after the "Что нового" card, one hint at a time.
 *
 * Unlike the web it lives INSIDE the sidebar — a phone keeps the sidebar in a
 * drawer, so there is no on-screen rail to point at from the outside. [target] is
 * the row's rect in this overlay's own coordinates; the host mounts the overlay
 * only while the drawer is open, and nothing here consumes taps, so the item the
 * hint points at stays tappable.
 */
@Composable
fun SidebarSpotlight(spot: WhatsNewSpotlight, target: Rect?, onDismiss: () -> Unit) {
    if (target == null) return
    val density = LocalDensity.current
    var cardHeight by remember { mutableIntStateOf(0) }

    BoxWithConstraints(Modifier.fillMaxSize()) {
        val gap = with(density) { CardGap.toPx() }
        val inset = with(density) { CardInset.toPx() }
        val tipInset = with(density) { TipInset.toPx() }
        // Below the row by default; above it when the card would fall off the
        // bottom (a hint on a low item, or a short screen in landscape).
        val below = target.bottom + gap
        val fitsBelow = below + cardHeight <= constraints.maxHeight
        val cardTop = if (fitsBelow) below else (target.top - gap - cardHeight).coerceAtLeast(0f)

        // The arrow spans the near edges: card → row.
        val start = Offset(
            inset + with(density) { 26.dp.toPx() },
            if (fitsBelow) cardTop - 4f else cardTop + cardHeight + 4f,
        )
        val tip = Offset(target.left + tipInset, if (fitsBelow) target.bottom + 3f else target.top - 3f)
        SpotlightArrow(start = start, tip = tip, target = target)

        Box(
            Modifier
                .offset { IntOffset(0, cardTop.toInt()) }
                .padding(horizontal = CardInset)
                .onSizeChanged { cardHeight = it.height },
        ) {
            SpotlightCard(spot, onDismiss)
        }
    }
}

/** The curved arrow plus the breathing ring around the target row. */
@Composable
private fun SpotlightArrow(start: Offset, tip: Offset, target: Rect) {
    val c = Tessera.colors
    val draw by animateFloatAsState(
        targetValue = 1f,
        animationSpec = tween(durationMillis = 560, delayMillis = 160),
        label = "spotlight-draw",
    )
    val pulse by rememberInfiniteTransition(label = "spotlight-ring").animateFloat(
        initialValue = 0.14f,
        targetValue = 0.42f,
        animationSpec = infiniteRepeatable(tween(1100, easing = LinearEasing), RepeatMode.Reverse),
        label = "spotlight-pulse",
    )

    Canvas(Modifier.fillMaxSize()) {
        // Ring: a rounded outline hugging the row (inset to match the row's own
        // 8dp side padding), breathing so the eye lands on it.
        val pad = 2.dp.toPx()
        val sidePad = 8.dp.toPx()
        drawRoundRect(
            color = c.primary.copy(alpha = pulse),
            topLeft = Offset(target.left + sidePad, target.top - pad),
            size = Size(target.width - sidePad * 2, target.height + pad * 2),
            cornerRadius = CornerRadius(RadiusSm.toPx()),
            style = Stroke(width = 1.5.dp.toPx()),
        )

        // A shallow bow towards the row: over this short distance the control
        // points stay near the ends, so the curve reads as a flick, not a loop.
        val dx = tip.x - start.x
        val dy = tip.y - start.y
        val bow = hypot(dx, dy) * 0.22f
        val path = Path().apply {
            moveTo(start.x, start.y)
            cubicTo(
                start.x - bow * 0.4f, start.y + dy * 0.35f,
                tip.x - bow, tip.y - dy * 0.25f,
                tip.x, tip.y,
            )
        }
        val measure = PathMeasure().apply { setPath(path, false) }
        val head = HeadLength.toPx()
        // The stroke stops short of the tip so its round cap hides behind the
        // arrowhead instead of blunting the point.
        val body = Path()
        measure.getSegment(0f, (measure.length - head) * draw, body, true)
        drawPath(
            path = body,
            brush = accentGradient(c.primary),
            style = Stroke(width = 2.4.dp.toPx(), cap = StrokeCap.Round),
        )

        if (draw > 0.98f) {
            val base = measure.getPosition(measure.length - head)
            val dir = measure.getTangent(measure.length)
            val halfW = 5.dp.toPx()
            drawPath(
                path = Path().apply {
                    moveTo(tip.x, tip.y)
                    lineTo(base.x - dir.y * halfW, base.y + dir.x * halfW)
                    lineTo(base.x + dir.y * halfW, base.y - dir.x * halfW)
                    close()
                },
                color = c.primary,
            )
        }
    }
}

@Composable
private fun SpotlightCard(spot: WhatsNewSpotlight, onDismiss: () -> Unit) {
    val c = Tessera.colors
    Column(
        Modifier
            .popupAppear(TransformOrigin(0.1f, 0f))
            .fillMaxWidth()
            .softShadow(RoundedCornerShape(RadiusMd), elevation = 8.dp)
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.surface)
            .border(1.dp, c.primary.copy(alpha = 0.55f), RoundedCornerShape(RadiusMd))
            .padding(horizontal = 13.dp, vertical = 12.dp)
            .testTag(TestTags.SPOTLIGHT_CARD),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier.size(20.dp).clip(CircleShape).background(accentGradient(c.primary)),
                contentAlignment = Alignment.Center,
            ) {
                IonIcon(Ion.STAR, size = 12.dp, tint = c.onPrimary)
            }
            Spacer(Modifier.width(8.dp))
            Text(spot.title, color = c.text1, fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
        }
        Spacer(Modifier.size(5.dp))
        Text(spot.body, color = c.text3, fontSize = 12.sp, lineHeight = 17.sp)
        Spacer(Modifier.size(10.dp))
        TButton(
            stringResource(R.string.common_got_it),
            onClick = onDismiss,
            modifier = Modifier.testTag(TestTags.SPOTLIGHT_DISMISS),
        )
    }
}
