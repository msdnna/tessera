package website.msdnna.tessera.ui.components

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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
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
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.theme.accentGradient

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
            Column(modifier, content = content)
        } else {
            Column(
                modifier
                    .softShadow(RoundedCornerShape(RadiusMd), elevation = 6.dp)
                    .clip(RoundedCornerShape(RadiusMd))
                    .background(c.surface)
                    .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
                    .width(IntrinsicSize.Max)
                    .widthIn(min = 140.dp, max = 320.dp)
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
    trailing: @Composable (() -> Unit)? = null,
) {
    val c = Tessera.colors
    val color = if (danger) TesseraDanger else c.text1
    Row(
        Modifier.fillMaxWidth().clickableNoRipple(onClick = onClick).padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (icon != null) {
            IonIcon(icon, size = 16.dp, tint = if (danger) TesseraDanger else c.text2)
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
    confirmText: String = "Удалить",
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
                ConfirmButton("Отмена", filled = false, color = c.text1, onClick = onDismiss)
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
