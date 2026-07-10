package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.Layout
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.parseHexColor
import website.msdnna.tessera.util.readableHue

/** Small coloured tag chip, matching the web kanban card chips. The fill keeps
 *  the raw tag colour (subtle), but the text is clamped to a legible lightness
 *  for the active theme so dark/light tag colours stay readable (web parity).
 *  Long names truncate; [big] renders a roomier badge (tag manager). */
@Composable
fun TagChip(name: String, color: String, modifier: Modifier = Modifier, big: Boolean = false) {
    val base = parseHexColor(color, Tessera.colors.text3)
    val text = readableHue(base, Tessera.colors.isDark)
    Box(
        modifier
            .clip(RoundedCornerShape(if (big) 6.dp else 4.dp))
            .background(accentGradient(base.copy(alpha = 0.18f)))
            .padding(horizontal = if (big) 9.dp else 6.dp, vertical = if (big) 4.dp else 2.dp),
    ) {
        Text(
            name,
            fontSize = if (big) 13.sp else 11.sp,
            fontWeight = FontWeight.Medium,
            style = TextStyle(brush = accentGradient(text)),
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

/**
 * Coloured tag chips ([TagChip]) filling the available width, with a "+N" chip for
 * the overflow (web tag-fit; used on cards and in the task modal). All chips + every
 * possible "+n" are composed; only the fitting subset is placed (unplaced children
 * aren't drawn). Fills its slot exactly and places chips flush-left.
 */
@Composable
fun TagChipsFit(tags: List<Tag>, modifier: Modifier = Modifier) {
    Layout(
        modifier = modifier,
        content = {
            tags.forEach { TagChip(it.name, it.color) }
            for (n in 1..tags.size) OverflowChip(n)
        },
    ) { measurables, constraints ->
        val n = tags.size
        val chip = measurables.take(n).map { it.measure(Constraints()) }
        val over = measurables.drop(n).map { it.measure(Constraints()) } // over[k-1] = "+k"
        val gap = 5.dp.roundToPx()
        val maxW = constraints.maxWidth
        fun width(k: Int) = if (k <= 0) 0 else chip.take(k).sumOf { it.width } + (k - 1) * gap
        var k = n
        if (width(n) > maxW) {
            k = 0
            for (cand in n downTo 0) {
                val rem = n - cand
                val ovW = if (rem > 0) over[rem - 1].width + (if (cand > 0) gap else 0) else 0
                if (width(cand) + ovW <= maxW) {
                    k = cand
                    break
                }
            }
        }
        val ov = if (k < n) over[n - k - 1] else null
        val placed = chip.take(k)
        val h = (placed.map { it.height } + listOfNotNull(ov?.height)).maxOrNull() ?: 0
        val outW = if (constraints.hasBoundedWidth) maxW else width(k) + (if (ov != null) (if (k > 0) gap else 0) + ov.width else 0)
        layout(outW, h) {
            var x = 0
            placed.forEach { p ->
                p.place(x, (h - p.height) / 2)
                x += p.width + gap
            }
            ov?.let { p -> p.place(x, (h - p.height) / 2) }
        }
    }
}

/** The "+N" overflow chip for tags that didn't fit (neutral fill). */
@Composable
private fun OverflowChip(n: Int) {
    val c = Tessera.colors
    Box(
        Modifier.clip(RoundedCornerShape(4.dp)).background(c.surfaceAlt).padding(horizontal = 7.dp, vertical = 2.dp),
    ) {
        Text("+$n", fontSize = 11.sp, color = c.text3, fontWeight = FontWeight.Medium, maxLines = 1)
    }
}

/** Circular initials avatar for an assignee. */
@Composable
fun AvatarChip(initials: String, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    Box(
        modifier
            .clip(CircleShape)
            .background(c.surfaceAlt)
            .padding(horizontal = 5.dp, vertical = 3.dp),
    ) {
        Text(initials, color = c.text2, fontSize = 10.sp, fontWeight = FontWeight.SemiBold)
    }
}

/** Solid colour dot (column header swatch). */
@Composable
fun ColorDot(color: Color, sizeDp: Int = 9, modifier: Modifier = Modifier) {
    Box(modifier.size(sizeDp.dp).clip(CircleShape).background(accentGradient(color)))
}
