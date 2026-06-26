package website.msdnna.tessera.ui.components

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.selection.LocalTextSelectionColors
import androidx.compose.foundation.text.selection.TextSelectionColors
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.ui.theme.AccentGradientStrengthSubtle
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient

enum class TButtonKind { Primary, Secondary, Ghost }

/** Button matching the web's Naive UI buttons (8dp radius, accent fill). */
@Composable
fun TButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    kind: TButtonKind = TButtonKind.Primary,
    enabled: Boolean = true,
    loading: Boolean = false,
    icon: String? = null,
) {
    val c = Tessera.colors
    val bg = when (kind) {
        TButtonKind.Primary -> c.primary
        TButtonKind.Secondary -> c.surfaceAlt
        TButtonKind.Ghost -> Color.Transparent
    }
    val fg = when (kind) {
        TButtonKind.Primary -> c.onPrimary
        else -> c.text1
    }
    val border = if (kind == TButtonKind.Secondary) BorderStroke(1.dp, c.border) else null
    val clickable = enabled && !loading

    // Accent fill (Primary) gets the soft diagonal gradient; neutral kinds stay flat.
    val fillColor = if (clickable) bg else bg.copy(alpha = 0.5f)
    val fill = if (kind == TButtonKind.Primary) accentGradient(fillColor) else SolidColor(fillColor)

    Box(
        modifier = modifier
            .clip(RoundedCornerShape(RadiusMd))
            .then(if (border != null) Modifier.border(border, RoundedCornerShape(RadiusMd)) else Modifier)
            .background(fill)
            .clickableNoRipple(enabled = clickable, onClick = onClick)
            .heightIn(min = 40.dp)
            .padding(horizontal = 18.dp, vertical = 9.dp),
        contentAlignment = Alignment.Center,
    ) {
        if (loading) {
            CircularProgressIndicator(modifier = Modifier.padding(2.dp), strokeWidth = 2.dp, color = fg)
        } else if (icon != null) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                IonIcon(icon, size = 16.dp, tint = fg)
                Text(text, color = fg, fontSize = 14.sp, fontWeight = FontWeight.Medium)
            }
        } else {
            Text(text, color = fg, fontSize = 14.sp, fontWeight = FontWeight.Medium)
        }
    }
}

/** Labelled single-line text field, bordered like the web inputs. */
@Composable
fun TTextField(
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    label: String? = null,
    placeholder: String = "",
    singleLine: Boolean = true,
    isPassword: Boolean = false,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default,
) {
    val c = Tessera.colors
    val interaction = remember { MutableInteractionSource() }
    val focused by interaction.collectIsFocusedAsState()
    val borderColor = if (focused) c.primary else c.border

    Column(modifier) {
        if (label != null) {
            Text(
                label,
                color = c.text2,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
                modifier = Modifier.padding(bottom = 6.dp),
            )
        }
        val selectionColors = TextSelectionColors(handleColor = c.primary, backgroundColor = c.primary.copy(alpha = 0.3f))
        CompositionLocalProvider(LocalTextSelectionColors provides selectionColors) {
            BasicTextField(
                value = value,
                onValueChange = onValueChange,
                singleLine = singleLine,
                textStyle = TextStyle(color = c.text1, fontSize = 14.sp),
                cursorBrush = SolidColor(c.primary),
                interactionSource = interaction,
                visualTransformation = if (isPassword) PasswordVisualTransformation() else VisualTransformation.None,
                keyboardOptions = keyboardOptions,
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(RadiusMd))
                    .background(c.surface)
                    .border(1.dp, borderColor, RoundedCornerShape(RadiusMd))
                    .padding(horizontal = 12.dp, vertical = 11.dp),
                decorationBox = { inner ->
                    Box(contentAlignment = Alignment.CenterStart) {
                        if (value.isEmpty() && placeholder.isNotEmpty()) {
                            Text(placeholder, color = c.placeholder, fontSize = 14.sp)
                        }
                        inner()
                    }
                },
            )
        }
    }
}

/** Card-style surface, matching the web's `--t-surface` cards. */
@Composable
fun TCard(
    modifier: Modifier = Modifier,
    padding: PaddingValues = PaddingValues(16.dp),
    content: @Composable () -> Unit,
) {
    val c = Tessera.colors
    Box(
        modifier = modifier
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.cardSurface)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
            .padding(padding),
    ) { content() }
}

/** Inline form error / hint row. */
@Composable
fun TFormError(message: String?, modifier: Modifier = Modifier) {
    if (message.isNullOrBlank()) return
    Row(modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Start) {
        Text(message, color = Color(0xFFE0533D), fontSize = 13.sp)
    }
}

/** Pill toggle matching the web's Naive `n-switch` (no Material thumb/ripple). */
@Composable
fun TSwitch(checked: Boolean, onCheckedChange: (Boolean) -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true) {
    val c = Tessera.colors
    val trackW = 40.dp
    val trackH = 22.dp
    val knob = 18.dp
    val track by animateColorAsState(
        if (checked) c.primary else if (c.isDark) c.surfaceAlt else c.border,
        label = "switch-track",
    )
    val knobOffset by animateDpAsState(
        if (checked) trackW - knob - 2.dp else 2.dp,
        label = "switch-knob",
    )
    // The active (accent) track gets the soft gradient; the off state stays flat neutral.
    val trackFill = if (checked) accentGradient(track) else SolidColor(track)
    Box(
        modifier
            .then(if (enabled) Modifier else Modifier.alpha(0.4f))
            .size(trackW, trackH)
            .clip(CircleShape)
            .background(trackFill)
            .then(if (enabled) Modifier.clickableNoRipple { onCheckedChange(!checked) } else Modifier),
        contentAlignment = Alignment.CenterStart,
    ) {
        Box(
            Modifier
                .offset(x = knobOffset)
                .size(knob)
                .clip(CircleShape)
                .background(Color.White),
        )
    }
}

/** A single underline-tab definition: a label and an optional count badge. */
data class TabItem(val label: String, val count: Int = 0)

/**
 * Horizontally scrollable underline tabs, matching the web's `n-tabs type=line`:
 * the active label is accent-coloured with a primary underline; counts show as a
 * small rounded badge.
 */
@Composable
fun UnderlineTabs(
    tabs: List<TabItem>,
    selected: Int,
    onSelect: (Int) -> Unit,
    modifier: Modifier = Modifier,
) {
    val c = Tessera.colors
    Column(modifier.fillMaxWidth()) {
        Row(
            Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState()),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            tabs.forEachIndexed { i, tab ->
                val active = i == selected
                Box(Modifier.padding(end = 18.dp)) {
                    // IntrinsicSize.Max sizes the column to its widest child (the
                    // label row), so a fillMaxWidth underline spans exactly the
                    // tab text + badge — even inside this horizontally-scrolling
                    // (unbounded-width) row, where plain fillMaxWidth collapses to 0.
                    Column(Modifier.width(IntrinsicSize.Max).clickableNoRipple { onSelect(i) }) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(
                                tab.label,
                                color = if (active) c.primary else c.text2,
                                fontSize = 14.sp,
                                fontWeight = if (active) FontWeight.SemiBold else FontWeight.Medium,
                                style = if (active) TextStyle(brush = accentGradient(c.primary, AccentGradientStrengthSubtle)) else TextStyle.Default,
                                modifier = Modifier.padding(vertical = 8.dp),
                            )
                            if (tab.count > 0) {
                                Spacer(Modifier.width(5.dp))
                                Box(
                                    Modifier
                                        .clip(CircleShape)
                                        .background(if (active) accentGradient(c.primary, AccentGradientStrengthSubtle) else SolidColor(c.surfaceAlt))
                                        .padding(horizontal = 6.dp, vertical = 1.dp),
                                ) {
                                    Text(
                                        tab.count.toString(),
                                        color = if (active) c.onPrimary else c.text3,
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.SemiBold,
                                    )
                                }
                            }
                        }
                        Box(
                            Modifier
                                .fillMaxWidth()
                                .height(2.dp)
                                .background(if (active) accentGradient(c.primary, AccentGradientStrengthSubtle) else SolidColor(Color.Transparent)),
                        )
                    }
                }
            }
        }
        HorizontalDivider(color = c.border, thickness = 1.dp)
    }
}
