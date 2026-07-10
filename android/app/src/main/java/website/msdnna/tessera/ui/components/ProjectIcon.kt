package website.msdnna.tessera.ui.components

import android.util.Base64
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import java.nio.ByteBuffer
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.IconKind
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.classifyIcon
import website.msdnna.tessera.util.parseHexColor

/**
 * Project / group glyph mirroring the web `ProjectIcon` + the parent's badge box.
 * The colour lands on the BOX (`iconMode == "badge"`, default) or on the GLYPH
 * (`iconMode == "icon"`), per the per-entity toggle (backend `icon_mode`):
 *  - badge + colour → a soft-gradient tile with a neutral (white/dark) glyph;
 *  - icon  / no colour → a transparent box with the glyph tinted (or neutral);
 *  - raw SVG / data-URL image → rendered via Coil, untinted (own colours);
 *  - empty → a folder (groups) or initials (projects).
 */
@Composable
fun ProjectIcon(
    name: String,
    icon: String,
    color: String,
    modifier: Modifier = Modifier,
    size: Dp = 22.dp,
    fallbackFolder: Boolean = false,
    iconMode: String = "badge",
    fallbackGlyph: String? = null,
) {
    val c = Tessera.colors
    val parsed = parseHexColor(color, c.text2)
    val hasColor = color.isNotBlank() && !color.equals("transparent", ignoreCase = true)
    val kind = classifyIcon(icon)
    val tintableGlyph = kind is IconKind.Curated ||
        (kind is IconKind.None && (fallbackFolder || fallbackGlyph != null))
    val badge = iconMode != "icon" && hasColor && tintableGlyph

    // Badge mode: the colour fills the box, the glyph rides neutral (web `.gicon`).
    if (badge) {
        val onBox = if (parsed.luminance() > 0.6f) Color(0xFF1F1F1F) else Color.White
        Box(
            modifier.size(size).clip(RoundedCornerShape(5.dp)).background(accentGradient(parsed)),
            contentAlignment = Alignment.Center,
        ) {
            val glyph = (kind as? IconKind.Curated)?.ion ?: fallbackGlyph ?: Ion.FOLDER
            IonIcon(glyph, size = size * 0.66f, tint = onBox)
        }
        return
    }

    // Transparent box (icon mode, or no colour): the glyph carries the colour.
    val glyphTint = if (iconMode == "icon" && hasColor) parsed else c.text2
    when (kind) {
        is IconKind.Curated -> IonIcon(
            kind.ion, modifier = modifier, size = size, tint = glyphTint, gradient = iconMode == "icon" && hasColor,
        )

        is IconKind.Svg -> AsyncImage(
            model = ByteBuffer.wrap(kind.markup.toByteArray(Charsets.UTF_8)),
            contentDescription = null,
            modifier = modifier.size(size),
        )

        is IconKind.Image -> AsyncImage(
            model = decodeDataUrl(kind.data),
            contentDescription = null,
            modifier = modifier.size(size),
        )

        IconKind.None -> when {
            fallbackGlyph != null -> IonIcon(
                fallbackGlyph, modifier = modifier, size = size,
                tint = if (iconMode == "icon" && hasColor) parsed else c.text3,
                gradient = iconMode == "icon" && hasColor,
            )

            fallbackFolder -> IonIcon(
                Ion.FOLDER, modifier = modifier, size = size,
                tint = if (iconMode == "icon" && hasColor) parsed else c.text3,
            )

            else -> InitialsTile(name, color, modifier, size, plain = iconMode == "icon" && hasColor)
        }
    }
}

@Composable
private fun InitialsTile(name: String, color: String, modifier: Modifier, size: Dp, plain: Boolean = false) {
    val c = Tessera.colors
    val glyph = name.trim().take(2).uppercase().ifBlank { "?" }
    // Icon mode: initials are plain tinted text (no box). Otherwise a coloured tile.
    if (plain) {
        Box(modifier.size(size), contentAlignment = Alignment.Center) {
            Text(glyph, color = parseHexColor(color, c.primary), fontSize = (size.value * 0.42f).sp, fontWeight = FontWeight.SemiBold)
        }
        return
    }
    val tile = parseHexColor(color, c.primary)
    val onTile = if (tile.luminance() > 0.6f) Color(0xFF1F1F1F) else Color.White
    Box(
        modifier.size(size).clip(RoundedCornerShape(6.dp)).background(accentGradient(tile)),
        contentAlignment = Alignment.Center,
    ) {
        Text(glyph, color = onTile, fontSize = (size.value * 0.42f).sp, fontWeight = FontWeight.SemiBold)
    }
}

private fun decodeDataUrl(data: String): Any {
    val comma = data.indexOf(',')
    if (comma < 0) return ByteBuffer.wrap(data.toByteArray(Charsets.UTF_8))
    val meta = data.substring(0, comma)
    val payload = data.substring(comma + 1)
    return if (meta.contains("base64")) {
        ByteBuffer.wrap(Base64.decode(payload, Base64.DEFAULT))
    } else {
        ByteBuffer.wrap(payload.toByteArray(Charsets.UTF_8))
    }
}

private fun Color.luminance(): Float = 0.2126f * red + 0.7152f * green + 0.0722f * blue
