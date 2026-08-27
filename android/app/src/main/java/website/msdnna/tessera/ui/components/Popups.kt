package website.msdnna.tessera.ui.components

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntRect
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupPositionProvider
import androidx.compose.ui.window.PopupProperties
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.theme.TesseraWarning
import website.msdnna.tessera.ui.theme.accentGradient

/**
 * A quick fade + scale-in for content that mounts when it first appears — popups,
 * menus and dialogs are composed only while open, so this is entrance-only (there
 * is no exit frame to play). Implemented as a `graphicsLayer` tween rather than
 * `AnimatedVisibility` so the content keeps its full measured size from the first
 * frame — important inside a `Popup`, whose position provider measures the content
 * (an animation that grew the layout would make the popup jump as it sized up).
 *
 * @param transformOrigin where the scale grows from — top-centre for a dropdown
 *        falling from its anchor, centre for a centred dialog.
 */
@Composable
fun Modifier.popupAppear(transformOrigin: TransformOrigin = TransformOrigin(0.5f, 0f)): Modifier {
    var shown by remember { mutableStateOf(false) }
    LaunchedEffect(Unit) { shown = true }
    val progress by animateFloatAsState(
        targetValue = if (shown) 1f else 0f,
        animationSpec = tween(140),
        label = "popupAppear",
    )
    return this.graphicsLayer {
        alpha = progress
        val s = 0.92f + 0.08f * progress
        scaleX = s
        scaleY = s
        this.transformOrigin = transformOrigin
    }
}

/**
 * A themed dropdown / popover anchored just below its trigger — the Naive-style
 * replacement for Material's `DropdownMenu` (no tonal tint, no ripple, our
 * surface + border + soft shadow). Place it as a sibling of the trigger inside a
 * `Box`; it positions against that Box's bounds.
 */
@Composable
fun TDropdown(
    expanded: Boolean,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
    // When the content is already a self-contained surface (e.g. a segmented
    // selector), [bare] drops this popover's own surface/border/shadow so there
    // aren't two nested cards.
    bare: Boolean = false,
    // Caps the popover height and makes its content vertically scrollable — for
    // long menus (e.g. a tag filter / picker with many tags). Opt-in: contents
    // that already self-scroll must leave this off to avoid nested scrolling.
    scrollable: Boolean = false,
    content: @Composable ColumnScope.() -> Unit,
) {
    if (!expanded) return
    val c = Tessera.colors
    val provider = remember { BelowAnchorPositionProvider() }
    Popup(
        popupPositionProvider = provider,
        onDismissRequest = onDismiss,
        properties = PopupProperties(focusable = true),
    ) {
        if (bare) {
            Column(modifier.popupAppear(), content = content)
        } else {
            Column(
                modifier
                    .popupAppear()
                    .softShadow(RoundedCornerShape(RadiusMd), elevation = 6.dp)
                    .clip(RoundedCornerShape(RadiusMd))
                    .background(c.surface)
                    .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
                    .width(IntrinsicSize.Max)
                    .widthIn(min = 140.dp, max = 320.dp)
                    .then(if (scrollable) Modifier.heightIn(max = 360.dp).verticalScroll(rememberScrollState()) else Modifier)
                    .padding(vertical = 4.dp),
                content = content,
            )
        }
    }
}

/** A single row in a [TDropdown]: optional leading ionicon, label, danger tint. */
@Composable
fun TMenuItem(
    label: String,
    onClick: () -> Unit,
    icon: String? = null,
    danger: Boolean = false,
    warn: Boolean = false,
    leading: @Composable (() -> Unit)? = null,
    trailing: @Composable (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    val c = Tessera.colors
    // warn (orange) = dangerous-but-not-destructive (e.g. transfer); danger (red) =
    // destructive (delete). Mirrors the web warnIcon / dangerIcon distinction.
    val accent = when {
        danger -> TesseraDanger
        warn -> TesseraWarning
        else -> null
    }
    val color = accent ?: c.text1
    Row(
        modifier.fillMaxWidth().clickableNoRipple(onClick = onClick).padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (leading != null) {
            leading()
            Spacer(Modifier.width(10.dp))
        } else if (icon != null) {
            IonIcon(icon, size = 16.dp, tint = accent ?: c.text2)
            Spacer(Modifier.width(10.dp))
        }
        Text(label, color = color, fontSize = 14.sp, modifier = Modifier.weight(1f))
        if (trailing != null) {
            Spacer(Modifier.width(28.dp))
            trailing()
        }
    }
}

/** A thin divider between menu groups. */
@Composable
fun TMenuDivider() {
    HorizontalDivider(Modifier.padding(vertical = 4.dp), color = Tessera.colors.border)
}

/**
 * A Naive-style confirm popover (popconfirm): a message and Отмена / confirm
 * buttons, anchored below the trigger. Used for destructive quick-actions
 * instead of a full-screen Material dialog.
 */
@Composable
fun TConfirmPopover(
    expanded: Boolean,
    message: String,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
    confirmText: String = stringResource(R.string.common_delete),
    danger: Boolean = true,
) {
    if (!expanded) return
    val c = Tessera.colors
    val provider = remember { BelowAnchorPositionProvider() }
    Popup(
        popupPositionProvider = provider,
        onDismissRequest = onDismiss,
        properties = PopupProperties(focusable = true),
    ) {
        Column(
            Modifier
                .popupAppear()
                .softShadow(RoundedCornerShape(RadiusMd), elevation = 6.dp)
                .clip(RoundedCornerShape(RadiusMd))
                .background(c.surface)
                .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
                .width(IntrinsicSize.Max)
                .widthIn(min = 200.dp, max = 300.dp)
                .padding(14.dp),
        ) {
            Text(message, color = c.text1, fontSize = 14.sp)
            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                ConfirmButton(stringResource(R.string.common_cancel), filled = false, color = c.text1, onClick = onDismiss)
                Spacer(Modifier.width(8.dp))
                ConfirmButton(confirmText, filled = true, color = if (danger) TesseraDanger else c.primary, onClick = onConfirm)
            }
        }
    }
}

/** A small Naive-style popconfirm button: bordered when secondary, accent-filled
 *  when the confirming action. */
@Composable
private fun ConfirmButton(label: String, filled: Boolean, color: Color, onClick: () -> Unit) {
    val c = Tessera.colors
    val shape = RoundedCornerShape(RadiusMd)
    Box(
        Modifier
            .clip(shape)
            .then(if (filled) Modifier.background(accentGradient(color)) else Modifier.border(1.dp, c.border, shape))
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 7.dp),
    ) {
        Text(
            label,
            color = if (filled) c.onPrimary else c.text1,
            fontSize = 13.sp,
            fontWeight = FontWeight.Medium,
        )
    }
}

/** Positions a popup just below the anchor (start-aligned), flipping above when
 *  there isn't room and clamping to the window. */
class BelowAnchorPositionProvider(private val gap: Int = 6) : PopupPositionProvider {
    override fun calculatePosition(
        anchorBounds: IntRect,
        windowSize: IntSize,
        layoutDirection: LayoutDirection,
        popupContentSize: IntSize,
    ): IntOffset {
        val x = anchorBounds.left.coerceIn(0, (windowSize.width - popupContentSize.width).coerceAtLeast(0))
        val below = anchorBounds.bottom + gap
        val y = if (below + popupContentSize.height <= windowSize.height) {
            below
        } else {
            val above = anchorBounds.top - popupContentSize.height - gap
            if (above >= 0) above else (windowSize.height - popupContentSize.height).coerceAtLeast(0)
        }
        return IntOffset(x, y)
    }
}
