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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import website.msdnna.tessera.ui.components.clickableNoRipple
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
 * semantics node, and every tag inside it stops answering. The tap target that
 * dismisses is a transparent sibling sitting exactly above the sheet, so nothing
 * has to swallow anything; the *paint* is a separate full-bleed layer underneath
 * everything, because a scrim that stopped at the sheet's top edge would leave
 * the two rounded corners showing raw room background, and unlit corners on a
 * dark screen read as a black shim rather than as a radius.
 *
 * @param handleLabel what the grabber says to a screen reader — the one control
 *   here with no text of its own and the only way to the expanded size.
 * @param header drawn inside the drag area and handed the sheet's own dismiss:
 *   a header ✕ that called the caller's `onClose` directly would cut the sheet
 *   off mid-screen without the exit it just animated in with.
 */
@Composable
internal fun ConferenceSheet(
    tag: String,
    handleTag: String,
    handleLabel: String,
    onClose: () -> Unit,
    header: @Composable (dismiss: () -> Unit) -> Unit,
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
    val settled = remember { Animatable(ConfSheetDetent.HALF.fraction) }
    var dragged by remember { mutableFloatStateOf(0f) }

    // Non-null only while a finger is down. The height follows this directly
    // instead of being snapped into [settled] frame by frame: every snap would
    // need its own coroutine, and the last one launched routinely arrived after
    // the settle animation had already started — cancelling it, and leaving the
    // sheet frozen at whatever height the thumb let go at with its contents cut
    // off. That was the «сворачивается не до конца» bug.
    var dragFraction by remember { mutableStateOf<Float?>(null) }

    // How much of the sheet has arrived. Slide rather than grow: the contents are
    // laid out at their resting height from the first frame, so nothing reflows
    // on the way in and the chat's input does not walk up the screen.
    val appear = remember { Animatable(0f) }
    var leaving by remember { mutableStateOf(false) }

    val fraction = dragFraction ?: settled.value

    // Every way out goes through here — the scrim, the header's ✕, Back, and a
    // drag released below the dismissal threshold — so the sheet always leaves
    // the way it came.
    val dismiss: () -> Unit = {
        if (!leaving) {
            leaving = true
            scope.launch {
                appear.animateTo(0f, tween(EXIT_MS))
                onClose()
            }
        }
    }

    LaunchedEffect(Unit) { appear.animateTo(1f, tween(ENTER_MS)) }

    // Back collapses an expanded sheet before it closes it — one gesture undoing
    // one gesture. Registered inside the sheet, so it runs ahead of the lobby's
    // own handler and a full-screen chat cannot take the whole call down with it.
    BackHandler {
        if (detent == ConfSheetDetent.FULL) {
            detent = ConfSheetDetent.HALF
            scope.launch { settled.animateTo(ConfSheetDetent.HALF.fraction, tween(SETTLE_MS)) }
        } else {
            dismiss()
        }
    }

    BoxWithConstraints(Modifier.fillMaxSize()) {
        val screen = maxHeight
        val screenPx = with(density) { screen.toPx() }

        val drag = rememberDraggableState { dy ->
            dragged += dy
            // Assigned, not animated: a sheet that eased towards the finger lags
            // behind it, and a grabber that does not keep up reads as a control
            // that missed rather than as one being dragged.
            dragFraction = confSheetDragFraction(detent, dragged, screenPx)
        }

        Box(Modifier.fillMaxSize()) {
            // Paint only, no input — see the note above about the corners.
            Box(
                Modifier.fillMaxSize()
                    .background(Color.Black.copy(alpha = SCRIM_ALPHA * appear.value)),
            )
            Column(Modifier.fillMaxSize()) {
                Box(
                    Modifier.fillMaxWidth().weight(1f)
                        .clickableNoRipple(onClick = dismiss),
                )
                Column(
                    Modifier.fillMaxWidth()
                        // A measured height rather than a weight: a weight cannot be
                        // zero, and the drag runs the sheet all the way down to the
                        // dismissal threshold.
                        .height(screen * fraction)
                        .graphicsLayer { translationY = size.height * (1f - appear.value) }
                        .clip(RoundedCornerShape(topStart = SheetCorner, topEnd = SheetCorner))
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
                                // Hand the height back to [settled] where the finger
                                // left it, then move from there. Both calls run in
                                // this one coroutine, so nothing can overtake them.
                                settled.snapTo(dragFraction ?: settled.value)
                                dragFraction = null
                                when (settle) {
                                    // A dismissal keeps the height it was released at
                                    // and slides out from there: the sheet is on its
                                    // way off screen, and growing back to half first
                                    // would show it inflating as it goes.
                                    ConfSheetSettle.CLOSE -> dismiss()

                                    ConfSheetSettle.HALF -> detent = ConfSheetDetent.HALF

                                    ConfSheetSettle.FULL -> detent = ConfSheetDetent.FULL
                                }
                                if (settle != ConfSheetSettle.CLOSE) {
                                    settled.animateTo(detent.fraction, tween(SETTLE_MS))
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
                        header(dismiss)
                    }
                    content()
                }
            }
        }
    }
}

/** Long enough to read as a movement, short enough not to be waited on. */
private const val SETTLE_MS = 220

/** The way in is the slower half: arriving is what has to be noticed. */
private const val ENTER_MS = 260
private const val EXIT_MS = 180

/**
 * Deeper than the app's own [website.msdnna.tessera.ui.theme.RadiusLg]. A sheet
 * is read by its top edge and nothing else, and 12dp against a dark call is a
 * corner nobody sees — M3 gives its bottom sheets 28dp for the same reason.
 */
private val SheetCorner = 22.dp

/** Black in both themes: a scrim that took the palette's own «text» colour came
 *  out near-white on dark and washed the call out instead of dimming it. */
private const val SCRIM_ALPHA = 0.45f
