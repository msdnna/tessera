package website.msdnna.tessera.ui.components

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.sin
import website.msdnna.tessera.ui.theme.AccentGradientStrength
import website.msdnna.tessera.ui.theme.AccentThemes
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.theme.accentGradientTint

/*
 * Workbench for dialling in the soft diagonal accent gradient (the "степень
 * градиентности"). Open this file in Android Studio's Split/Design pane.
 *
 * The top block sweeps a single button through several gradient strengths so
 * you can eyeball where "barely noticeable" lands; below it the chosen
 * [AccentGradientStrength] is applied to every accent surface (button, chip,
 * avatar, active toggle, tab underline, accent border) and to all seven accent
 * themes, in light and dark. To change the app-wide default, edit
 * `AccentGradientStrength` and re-render.
 *
 * Pipette check: the exact centre of any gradient button equals its flat base
 * colour — that is the design contract, verify it here.
 */

/** A standalone gradient pill, independent of [TButton], for the strength sweep. */
@Composable
private fun GradientPill(
    text: String,
    base: Color,
    strength: Float,
    onColor: Color = Color.White,
    modifier: Modifier = Modifier,
) {
    Box(
        modifier
            .clip(RoundedCornerShape(RadiusMd))
            .background(accentGradient(base, strength))
            .heightIn(min = 40.dp)
            .padding(horizontal = 18.dp, vertical = 9.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(text, color = onColor, fontSize = 14.sp, fontWeight = FontWeight.Medium)
    }
}

/**
 * Ghost (outlined) button — border + glyph + optional label tinted by ONE
 * continuous gradient via [accentGradientTint], so the whole control reads as a
 * single gradient unit. Icon-only when [label] is null.
 *
 * NB: the glyph is a `Canvas`-drawn vector (star / trash) rather than the app's
 * `IonIcon`, purely so it renders in Android Studio's @Preview — Coil/SVG does
 * not. The real ghost buttons keep `IonIcon`; the gradient lands on it
 * identically, since it is drawn into the same offscreen layer the tint
 * composites over. This preview is here exactly to confirm that.
 */
@Composable
private fun GhostButton(label: String?, base: Color, strength: Float, glyph: @Composable (Color) -> Unit) {
    Box(
        Modifier
            .clip(RoundedCornerShape(RadiusMd))
            .accentGradientTint(base, strength)
            .border(1.dp, base.copy(alpha = 0.6f), RoundedCornerShape(RadiusMd))
            .heightIn(min = 40.dp)
            .then(if (label == null) Modifier.width(40.dp) else Modifier.padding(horizontal = 14.dp)),
        contentAlignment = Alignment.Center,
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            glyph(base)
            if (label != null) Text(label, color = base, fontSize = 14.sp, fontWeight = FontWeight.Medium)
        }
    }
}

/** Filled five-point star glyph — accent ("archive") stand-in for the preview. */
@Composable
private fun StarGlyph(color: Color) = Canvas(Modifier.size(18.dp)) {
    val cx = size.width / 2f
    val cy = size.height / 2f
    val outer = size.minDimension / 2f
    val inner = outer * 0.45f
    val path = Path()
    for (i in 0 until 10) {
        val angle = -PI / 2 + i * PI / 5 // start pointing up
        val r = if (i % 2 == 0) outer else inner
        val x = cx + (r * cos(angle)).toFloat()
        val y = cy + (r * sin(angle)).toFloat()
        if (i == 0) path.moveTo(x, y) else path.lineTo(x, y)
    }
    path.close()
    drawPath(path, color)
}

/** Filled trash-can glyph — danger ("delete") stand-in for the preview. */
@Composable
private fun TrashGlyph(color: Color) = Canvas(Modifier.size(18.dp)) {
    val w = size.width
    val h = size.height
    drawRect(color, topLeft = Offset(w * 0.38f, h * 0.10f), size = Size(w * 0.24f, h * 0.07f)) // handle
    drawRect(color, topLeft = Offset(w * 0.16f, h * 0.18f), size = Size(w * 0.68f, h * 0.10f)) // lid
    drawRoundRect(color, topLeft = Offset(w * 0.24f, h * 0.32f), size = Size(w * 0.52f, h * 0.56f), cornerRadius = CornerRadius(w * 0.06f)) // body
}

/**
 * Underline tabs with text. The ACTIVE tab's label + underline are wrapped in a
 * single [accentGradientTint], so they share one continuous diagonal gradient
 * (one whole), while inactive tabs stay flat neutral.
 */
@Composable
private fun TabsDemo(strength: Float) {
    val c = Tessera.colors
    val tabs = listOf("Комментарии", "Подзадачи", "Связи")
    Row(horizontalArrangement = Arrangement.spacedBy(18.dp)) {
        tabs.forEachIndexed { i, t ->
            val active = i == 0
            Column(
                Modifier
                    .width(IntrinsicSize.Max)
                    .then(if (active) Modifier.accentGradientTint(c.primary, strength) else Modifier),
            ) {
                Text(
                    t,
                    color = if (active) c.primary else c.text2,
                    fontSize = 14.sp,
                    fontWeight = if (active) FontWeight.SemiBold else FontWeight.Medium,
                    modifier = Modifier.padding(vertical = 8.dp),
                )
                Box(
                    Modifier
                        .fillMaxWidth()
                        .height(2.dp)
                        .background(if (active) c.primary else Color.Transparent),
                )
            }
        }
    }
}

private val StrengthSweep = listOf(0.0f, 0.08f, 0.14f, 0.20f, 0.28f)

@Composable
private fun Workbench(dark: Boolean) {
    TesseraTheme(isDark = dark) {
        val c = Tessera.colors
        Column(
            Modifier
                .width(340.dp)
                .background(c.bg)
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text("Сила градиента (центр = базовый цвет)", color = c.text2, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
            StrengthSweep.forEach { s ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        if (s == AccentGradientStrength) "%.2f ◀".format(s) else "%.2f".format(s),
                        color = c.text3,
                        fontSize = 11.sp,
                        modifier = Modifier.width(56.dp),
                    )
                    GradientPill("Кнопка", c.primary, s, c.onPrimary, Modifier.weight(1f))
                }
            }

            Spacer(Modifier.height(4.dp))
            Text("Все акценты @ %.2f".format(AccentGradientStrength), color = c.text2, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
            AccentThemes.chunked(2).forEach { row ->
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    row.forEach { a ->
                        GradientPill(a.name, a.primary, AccentGradientStrength, a.onPrimary, Modifier.weight(1f))
                    }
                    if (row.size == 1) Spacer(Modifier.weight(1f))
                }
            }

            Spacer(Modifier.height(4.dp))
            Text("Элементы @ %.2f".format(AccentGradientStrength), color = c.text2, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                // Initials avatar
                Box(
                    Modifier.size(34.dp).clip(RoundedCornerShape(8.dp)).background(accentGradient(c.primary)),
                    contentAlignment = Alignment.Center,
                ) { Text("MS", color = c.onPrimary, fontSize = 13.sp, fontWeight = FontWeight.SemiBold) }
                // Active toggle
                Box(
                    Modifier.size(40.dp, 22.dp).clip(CircleShape).background(accentGradient(c.primary)),
                    contentAlignment = Alignment.CenterEnd,
                ) { Box(Modifier.padding(end = 2.dp).size(18.dp).clip(CircleShape).background(Color.White)) }
                // Chip (filled accent)
                Box(
                    Modifier.clip(RoundedCornerShape(4.dp)).background(accentGradient(c.primary)).padding(horizontal = 8.dp, vertical = 3.dp),
                ) { Text("чип", color = c.onPrimary, fontSize = 11.sp, fontWeight = FontWeight.Medium) }
            }
            // Tab underline
            Box(Modifier.width(80.dp).height(2.dp).background(accentGradient(c.primary)))

            Spacer(Modifier.height(4.dp))
            Text("Ghost-кнопки — бордер+иконка+текст одним градиентом", color = c.text2, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                GhostButton(null, c.primary, AccentGradientStrength) { StarGlyph(it) }
                GhostButton(null, TesseraDanger, AccentGradientStrength) { TrashGlyph(it) }
                GhostButton("В архив", c.primary, AccentGradientStrength) { StarGlyph(it) }
                GhostButton("Удалить", TesseraDanger, AccentGradientStrength) { TrashGlyph(it) }
            }

            Spacer(Modifier.height(4.dp))
            Text("Табы с текстом — активная как одно целое", color = c.text2, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
            TabsDemo(AccentGradientStrength)
        }
    }
}

@Preview(name = "Accent gradient — light", showBackground = true, heightDp = 1140)
@Composable
private fun AccentGradientPreviewLight() = Workbench(dark = false)

@Preview(name = "Accent gradient — dark", showBackground = true, heightDp = 1140)
@Composable
private fun AccentGradientPreviewDark() = Workbench(dark = true)

/** Focused, guaranteed-uncut view of just the ghost buttons + active-tab demo. */
@Composable
private fun GhostAndTabs(dark: Boolean) {
    TesseraTheme(isDark = dark) {
        val c = Tessera.colors
        Column(
            Modifier.width(340.dp).background(c.bg).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text("Ghost-кнопки — бордер+иконка+текст одним градиентом", color = c.text2, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                GhostButton(null, c.primary, AccentGradientStrength) { StarGlyph(it) }
                GhostButton(null, TesseraDanger, AccentGradientStrength) { TrashGlyph(it) }
                GhostButton("В архив", c.primary, AccentGradientStrength) { StarGlyph(it) }
                GhostButton("Удалить", TesseraDanger, AccentGradientStrength) { TrashGlyph(it) }
            }
            Text("Табы с текстом — активная как одно целое", color = c.text2, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
            TabsDemo(AccentGradientStrength)
        }
    }
}

@Preview(name = "Ghost + tabs — light", showBackground = true)
@Composable
private fun GhostAndTabsLight() = GhostAndTabs(dark = false)

@Preview(name = "Ghost + tabs — dark", showBackground = true)
@Composable
private fun GhostAndTabsDark() = GhostAndTabs(dark = true)
