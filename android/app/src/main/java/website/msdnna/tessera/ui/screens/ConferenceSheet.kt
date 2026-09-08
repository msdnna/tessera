package website.msdnna.tessera.ui.screens

import androidx.activity.compose.BackHandler
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.ConfSheetDetent
import website.msdnna.tessera.util.ConfSheetSettle
import website.msdnna.tessera.util.confSheetDragFraction
import website.msdnna.tessera.util.confSheetSettle

/**
 * The frame both in-call sheets are drawn in — the chat (#2896 §7) and the roster
 * (§6), which are the same object with different contents.
 *
 * It opens at half the screen and pulls up to nearly all of it, the way an M3
 * bottom sheet does. Half is the interesting one: the two things people read
 * during a call are a message and a list of names, and a sheet that covered the
 * meeting to show either would be closed again immediately — the sheet is *for*
 * watching and reading at once.
 *
 * The scrim and the sheet are siblings rather than parent and child, and that is
 * not a layout preference: a `clickable` wrapped around the sheet — the obvious
 * way to keep a tap on a row off the toolbar underneath — makes it a merging
 * semantics node, and every tag inside it stops answering. Stacked this way the
 * scrim covers exactly what the sheet does not, so nothing has to swallow
 * anything.
 *
 * @param handleLabel what the grabber says to a screen reader — the one control
 *   here with no text of its own and the only way to the expanded size.
 */
@Composable
internal fun ConferenceSheet(
    tag: String,
    handleTag: String,
    handleLabel: String,
    onClose: () -> Unit,
    header: @Composable () -> Unit,
    content: @Composable ColumnScope.() -> Unit,
) {
    val c = Tessera.colors
    val density = LocalDensity.current
    val scope = rememberCoroutineScope()

    // Where the sheet is resting, and where it is right now. The two differ only
    // during a drag and during the animation that follows one — but they have to
    // be separate, because the rule that decides where a release lands reasons
    // about the detent it started from, not about the pixel it stopped at.
    var detent by remember { mutableStateOf(ConfSheetDetent.HALF) }
    val fraction = remember { Animatable(ConfSheetDetent.HALF.fraction) }
    var dragged by remember { mutableFloatStateOf(0f) }

    // Back collapses an expanded sheet before it closes it — one gesture undoing
    // one gesture. Registered inside the sheet, so it runs ahead of the lobby's
    // own handler and a full-screen chat cannot take the whole call down with it.
    BackHandler {
        if (detent == ConfSheetDetent.FULL) {
            detent = ConfSheetDetent.HALF
            scope.launch { fraction.animateTo(ConfSheetDetent.HALF.fraction, tween(SETTLE_MS)) }
        } else {
            onClose()
        }
    }

    BoxWithConstraints(Modifier.fillMaxSize()) {
        val screen = maxHeight
        val screenPx = with(density) { screen.toPx() }

        val drag = rememberDraggableState { dy ->
            dragged += dy
            // Snapped, not animated: a sheet that eased towards the finger lags
            // behind it, and a grabber that does not keep up reads as a control
            // that missed rather than as one being dragged.
            scope.launch { fraction.snapTo(confSheetDragFraction(detent, dragged, screenPx)) }
        }

        Column(Modifier.fillMaxSize()) {
            Box(
                Modifier.fillMaxWidth().weight(1f)
                    .background(c.text1.copy(alpha = 0.4f))
                    .clickableNoRipple(onClick = onClose),
            )
            Column(
                Modifier.fillMaxWidth()
                    // A measured height rather than a weight: a weight cannot be
                    // zero, and the drag runs the sheet all the way down to the
                    // dismissal threshold.
                    .height(screen * fraction.value)
                    .clip(RoundedCornerShape(topStart = RadiusLg, topEnd = RadiusLg))
                    .background(c.surface)
                    .testTag(tag),
            ) {
                Column(
                    Modifier.fillMaxWidth().draggable(
                        state = drag,
                        orientation = Orientation.Vertical,
                        onDragStarted = { dragged = 0f },
                        onDragStopped = { velocity ->
                            val settle = confSheetSettle(
                                from = detent,
                                dragDp = with(density) { dragged.toDp().value },
                                velocityDp = with(density) { velocity.toDp().value },
                            )
                            when (settle) {
                                ConfSheetSettle.CLOSE -> onClose()
                                ConfSheetSettle.HALF -> detent = ConfSheetDetent.HALF
                                ConfSheetSettle.FULL -> detent = ConfSheetDetent.FULL
                            }
                            // A dismissal leaves the sheet where the finger left
                            // it: the caller is about to stop composing this, and
                            // animating back to half first would show it growing
                            // on its way out.
                            if (settle != ConfSheetSettle.CLOSE) {
                                fraction.animateTo(detent.fraction, tween(SETTLE_MS))
                            }
                        },
                    ),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Box(
                        Modifier.padding(top = 8.dp, bottom = 2.dp)
                            .size(width = 34.dp, height = 4.dp)
                            .clip(RoundedCornerShape(2.dp))
                            .background(c.text3)
                            .semantics { contentDescription = handleLabel }
                            .testTag(handleTag),
                    )
                    // Inside the drag area with the grabber: a 4dp bar is a
                    // target nobody hits, and every sheet worth copying is
                    // dragged by its whole header.
                    header()
                }
                content()
            }
        }
    }
}

/** Long enough to read as a movement, short enough not to be waited on. */
private const val SETTLE_MS = 220
