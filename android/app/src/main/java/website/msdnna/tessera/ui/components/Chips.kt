package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
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
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.layout.Layout
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.SourceMeta
import website.msdnna.tessera.util.onColor
import website.msdnna.tessera.util.parseHexColor
import website.msdnna.tessera.util.readableHue
import website.msdnna.tessera.util.tagParts

/** Tag pills stand the same height as the other card pills (`Pill` in TaskCard) —
 *  a shorter chip read as a different, misaligned control in the same row. */
val TagPillHeight = 24.dp
private val TagPillHeightBig = 28.dp

/**
 * Small coloured tag chip, matching the web kanban card chips ([TagPill.vue] parity).
 *
 * A **scoped** tag ("type::feature") renders as a GitLab-EE two-segment pill —
 * «scope value» — where the scope side is a filled accent segment with contrast
 * text and the value side a **flat** soft tint with accent (gradient) text, the
 * whole pill bordered in the tag's hue. An unscoped tag keeps the plain
 * single-segment chip.
 * The raw "type::feature" is never shown: the scope segment carries the configured
 * friendly prefix name (or the bare prefix under the «короткие префиксы» device
 * preference, or when no name is configured).
 *
 * The fill keeps the raw tag colour (subtle) but is **opaque** — blended with
 * [surface] rather than alpha-tinted, so neither the card behind it nor the stack
 * cascade peeking out to the right shows through the pill. The pale fill is also
 * **flat**: the accent gradient belongs to the accent-coloured parts (the scope
 * segment's fill, the value text), and over a 18%-tint it just reads as a grey
 * wash across the pill (web parity — `tagPillBg` keeps a flat padding-box interior
 * and puts the gradient on the border). Text is clamped to a
 * legible lightness for the active theme so dark/light tag colours stay readable.
 * The chip is the same height as the other card pills ([TagPillHeight]); its
 * padding only sets the width.
 * Long names truncate; [big] renders a roomier badge (tag manager).
 * [showScope] false drops the scope segment — for a picker already grouped by
 * scope, where it would just repeat the section header.
 */
@Composable
fun TagChip(
    name: String,
    color: String,
    modifier: Modifier = Modifier,
    big: Boolean = false,
    prefixNames: Map<String, String> = emptyMap(),
    showScope: Boolean = true,
    surface: Color = Tessera.colors.cardSurface,
) {
    val base = parseHexColor(color, Tessera.colors.text3)
    val text = readableHue(base, Tessera.colors.isDark)
    val parts = tagParts(name, prefixNames, Tessera.rawTagPrefix)
    val shape = RoundedCornerShape(if (big) 6.dp else 4.dp)
    val fontSize = if (big) 13.sp else 11.sp
    val height = if (big) TagPillHeightBig else TagPillHeight
    val fill = lerp(surface, base, 0.18f)
    if (parts.hasScope && showScope) {
        // Two segments inside one clipped, bordered box: the colour change *is* the
        // divider, so there's no doubled hairline where the segments meet (the web
        // drops the shared border edge for the same reason).
        Row(
            modifier.height(height).clip(shape).border(1.dp, text, shape),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            TagSegment(accentGradient(base), if (big) 8.dp else 6.dp) {
                Text(
                    parts.scope,
                    fontSize = fontSize,
                    fontWeight = FontWeight.SemiBold,
                    color = onColor(base),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            TagSegment(SolidColor(fill), if (big) 9.dp else 7.dp) {
                Text(
                    parts.label,
                    fontSize = fontSize,
                    fontWeight = FontWeight.Medium,
                    style = TextStyle(brush = accentGradient(text)),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
        return
    }
    Box(
        modifier
            .height(height)
            .clip(shape)
            .background(fill)
            .padding(horizontal = if (big) 9.dp else 7.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            parts.label,
            fontSize = fontSize,
            fontWeight = FontWeight.Medium,
            style = TextStyle(brush = accentGradient(text)),
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

/** One side of a two-segment scoped pill: fills the pill's height so both segments
 *  meet edge to edge (a padding-sized segment would leave the border showing through
 *  above and below the shorter one). */
@Composable
private fun TagSegment(fill: Brush, horizontal: Dp, content: @Composable () -> Unit) {
    Box(
        Modifier.fillMaxHeight().background(fill).padding(horizontal = horizontal),
        contentAlignment = Alignment.Center,
    ) { content() }
}

/**
 * A tag name as flat text for a call site that owns the box and the colour — the
 * tag pickers, whose chips flip fill/text on selection (web `TagPill variant="inherit"`).
 * A scoped tag reads «scope │ value» with the scope muted behind a hairline
 * divider in the current colour; [showScope] false leaves the value alone.
 */
@Composable
fun TagLabel(
    name: String,
    color: Color,
    modifier: Modifier = Modifier,
    prefixNames: Map<String, String> = emptyMap(),
    showScope: Boolean = true,
    fontSize: TextUnit = 12.sp,
) {
    val parts = tagParts(name, prefixNames, Tessera.rawTagPrefix)
    if (!parts.hasScope || !showScope) {
        Text(parts.label, color = color, fontSize = fontSize, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = modifier)
        return
    }
    Row(modifier, verticalAlignment = Alignment.CenterVertically) {
        Text(
            parts.scope,
            color = color.copy(alpha = 0.78f),
            fontSize = fontSize,
            fontWeight = FontWeight.Medium,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Box(
            Modifier.padding(horizontal = 5.dp).width(1.dp).height(fontSize.value.dp)
                .background(color.copy(alpha = 0.42f)),
        )
        Text(parts.label, color = color, fontSize = fontSize, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

/**
 * Coloured tag chips ([TagChip]) filling the available width, with a "+N" chip for
 * the overflow (web tag-fit; used on cards and in the task modal). All chips + every
 * possible "+n" are composed; only the fitting subset is placed (unplaced children
 * aren't drawn). Fills its slot exactly and places chips flush-left.
 */
@Composable
fun TagChipsFit(tags: List<Tag>, modifier: Modifier = Modifier, prefixNames: Map<String, String> = emptyMap()) {
    // The "+N" takes the leading tag's hue (web parity) — a neutral grey chip read
    // as a separate, unrelated pill next to the coloured ones.
    val tint = tags.firstOrNull()?.color
    Layout(
        modifier = modifier,
        content = {
            tags.forEach { TagChip(it.name, it.color, prefixNames = prefixNames) }
            for (n in 1..tags.size) OverflowChip(n, tint)
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

/** The "+N" overflow chip for tags that didn't fit. Takes the leading tag's hue
 *  ([tint], a hex colour) so it reads as part of the same tag run; falls back to
 *  the neutral surface when there is no tag to borrow a colour from. */
@Composable
private fun OverflowChip(n: Int, tint: String? = null) {
    val c = Tessera.colors
    val base = tint?.let { parseHexColor(it, c.text3) }
    // Flat fill, like the tag chips it stands next to: a gradient over the pale
    // tint reads as a grey wash, not as the tag's hue. Opaque (blended with the
    // card) for the same reason as [TagChip]: the stack cascade must not show through.
    val fill = if (base != null) {
        Modifier.background(lerp(c.cardSurface, base, 0.18f))
    } else {
        Modifier.background(c.surfaceAlt)
    }
    Box(
        Modifier.height(TagPillHeight).clip(RoundedCornerShape(4.dp)).then(fill).padding(horizontal = 7.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            "+$n",
            fontSize = 11.sp,
            color = if (base != null) readableHue(base, c.isDark) else c.text3,
            fontWeight = FontWeight.Medium,
            maxLines = 1,
        )
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

/**
 * Provenance badge for a record owned by an integration (relations today, whatever
 * gets a `source` column next) — web `.rel-src` parity.
 *
 * Deliberately flat neutral grey: it marks where the row came from, not an accent,
 * so it must not compete with the gradient chips around it.
 */
@Composable
fun SourceBadge(meta: SourceMeta, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    Row(
        modifier
            .clip(RoundedCornerShape(9.dp))
            .border(1.dp, c.border, RoundedCornerShape(9.dp))
            .background(c.surfaceAlt)
            .padding(horizontal = 6.dp, vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        meta.icon?.let {
            IonIcon(it, size = 11.dp, tint = c.text3)
            Spacer(Modifier.width(3.dp))
        }
        Text(meta.label, color = c.text3, fontSize = 11.sp, maxLines = 1)
    }
}

/** Solid colour dot (column header swatch). */
@Composable
fun ColorDot(color: Color, sizeDp: Int = 9, modifier: Modifier = Modifier) {
    Box(modifier.size(sizeDp.dp).clip(CircleShape).background(accentGradient(color)))
}
