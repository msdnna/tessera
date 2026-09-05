package website.msdnna.tessera.ui.components

import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed

/**
 * Click handling without the Material ripple — the web UI uses flat hover/press
 * states, so a ripple would look out of place. Disabled clicks are inert.
 */
fun Modifier.clickableNoRipple(
    enabled: Boolean = true,
    // What the tap does, for a screen reader. Only worth passing where the tap
    // target's own text does not already say it — a whole row whose visible
    // content describes a thing rather than the action it performs.
    onClickLabel: String? = null,
    onClick: () -> Unit,
): Modifier = composed {
    val interaction = remember { MutableInteractionSource() }
    clickable(
        interactionSource = interaction,
        indication = null,
        enabled = enabled,
        onClickLabel = onClickLabel,
        onClick = onClick,
    )
}

/** [clickableNoRipple] with a long-press handler (e.g. tap = open, hold = edit). */
fun Modifier.combinedClickableNoRipple(
    onClick: () -> Unit,
    onLongClick: (() -> Unit)? = null,
): Modifier = composed {
    val interaction = remember { MutableInteractionSource() }
    combinedClickable(
        interactionSource = interaction,
        indication = null,
        onClick = onClick,
        onLongClick = onLongClick,
    )
}
