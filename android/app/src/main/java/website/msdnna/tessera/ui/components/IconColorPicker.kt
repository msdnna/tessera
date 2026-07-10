package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.CuratedIconKeys
import website.msdnna.tessera.util.IconKind
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.classifyIcon
import website.msdnna.tessera.util.parseHexColor

/** The Naive accent palette shared by columns, projects and groups. */
val EntityPalette = listOf(
    "#7c5cff", "#2f80ed", "#0eb0a9", "#18a058", "#f0a020", "#e0533d", "#eb2f96", "#9aa0aa",
)

/**
 * Colour + icon picker sections for a project / group, laid out to drop straight
 * into a [TDropdown]'s `ColumnScope`. Mirrors the web `IconColorPicker`: a row of
 * accent swatches (plus a "no colour" reset) and a grid of curated icons (plus a
 * reset that falls back to initials / folder).
 */
@Composable
fun ColumnScopePicker(
    color: String,
    icon: String,
    onColor: (String) -> Unit,
    onIcon: (String) -> Unit,
    iconMode: String? = null,
    onIconMode: ((String) -> Unit)? = null,
    fallbackIcon: String? = null,
) {
    val c = Tessera.colors
    val tint = parseHexColor(color, c.text2)

    SectionLabel("Цвет")
    FlowRow(
        Modifier.padding(horizontal = 12.dp).padding(bottom = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        // "No colour" reset.
        Box(
            Modifier.size(22.dp).clip(CircleShape).background(c.surfaceAlt)
                .border(1.dp, c.border, CircleShape)
                .then(if (color.isBlank()) Modifier.border(2.dp, c.text1, CircleShape) else Modifier)
                .clickableNoRipple { onColor("") },
            contentAlignment = Alignment.Center,
        ) {
            IonIcon(Ion.CLOSE, size = 12.dp, tint = c.text3)
        }
        EntityPalette.forEach { hex ->
            val swatch = parseHexColor(hex, c.text3)
            val selected = color.equals(hex, ignoreCase = true)
            Box(
                Modifier.size(22.dp).clip(CircleShape).background(accentGradient(swatch))
                    .then(if (selected) Modifier.border(2.dp, c.text1, CircleShape) else Modifier)
                    .clickableNoRipple { onColor(hex) },
            )
        }
    }

    TMenuDivider()
    SectionLabel("Иконка")
    FlowRow(
        Modifier.padding(horizontal = 12.dp).padding(bottom = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        // "No icon" reset. For boards this is the default (kanban) glyph, so the
        // user sees and can re-select it; elsewhere a plain "×" (initials/folder).
        IconTile(selected = classifyIcon(icon) !is IconKind.Curated, onClick = { onIcon("") }) {
            if (fallbackIcon != null) {
                ProjectIcon(name = "", icon = "", color = color, size = 18.dp, iconMode = iconMode ?: "badge", fallbackGlyph = fallbackIcon)
            } else {
                IonIcon(Ion.CLOSE, size = 15.dp, tint = c.text3)
            }
        }
        CuratedIconKeys.forEach { key ->
            val selected = icon == key
            IconTile(selected = selected, onClick = { onIcon(key) }) {
                ProjectIcon(name = "", icon = key, color = color, size = 18.dp, iconMode = iconMode ?: "badge")
            }
        }
    }

    // Where the colour lands: the badge box (default) or the glyph itself.
    if (onIconMode != null) {
        TMenuDivider()
        SectionLabel("Что красить")
        ModeToggle(
            mode = iconMode ?: "badge",
            onMode = onIconMode,
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp),
        )
    }
}

/** Segmented "badge box vs glyph" toggle (web `.mode-toggle`). */
@Composable
private fun ModeToggle(mode: String, onMode: (String) -> Unit, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    Row(
        modifier.clip(RoundedCornerShape(6.dp)).background(c.surfaceAlt).padding(2.dp),
        horizontalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        listOf("badge" to "Бейдж", "icon" to "Иконка").forEach { (key, label) ->
            val active = (mode != "icon") == (key == "badge")
            Box(
                Modifier.weight(1f).clip(RoundedCornerShape(5.dp))
                    .background(if (active) c.surface else Color.Transparent)
                    .clickableNoRipple { onMode(key) }
                    .padding(vertical = 6.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text(label, color = if (active) c.text1 else c.text3, fontSize = 12.sp)
            }
        }
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(
        text,
        color = Tessera.colors.text3,
        fontSize = 12.sp,
        modifier = Modifier.padding(start = 14.dp, top = 6.dp, bottom = 4.dp),
    )
}

@Composable
private fun IconTile(selected: Boolean, onClick: () -> Unit, content: @Composable () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier.size(28.dp).clip(RoundedCornerShape(6.dp))
            .background(if (selected) c.hover else c.surfaceAlt)
            .then(if (selected) Modifier.border(1.5.dp, c.primary, RoundedCornerShape(6.dp)) else Modifier)
            .clickableNoRipple(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) { content() }
}
