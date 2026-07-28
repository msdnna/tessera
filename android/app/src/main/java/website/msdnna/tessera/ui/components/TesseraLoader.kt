package website.msdnna.tessera.ui.components

import android.graphics.Matrix
import android.graphics.RectF
import android.provider.Settings
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.LinearGradientShader
import androidx.compose.ui.graphics.ShaderBrush
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.asAndroidPath
import androidx.compose.ui.graphics.drawscope.rotate
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import kotlin.math.max
import kotlin.math.min
import kotlin.math.pow
import website.msdnna.tessera.ui.theme.AccentGradientStrength
import website.msdnna.tessera.ui.theme.Tessera

// ─────────────────────────────────────────────────────────────────────────────
// Firmennyy tessera loader — the exact port of the web engine
// (frontend/src/utils/tesseraLoader.js). The "t" mark's corner tile grows to
// cover the glyph, then the tessera endlessly spins, unfolding into an app view
// (kanban / list / timeline / gantt / matrix) on each turn, collapsing back and
// rotating 90°. Geometry + timings are the SAME NUMBERS as the web loader
// (loader-states.json) — keep the two in sync so the platforms don't diverge.
// ─────────────────────────────────────────────────────────────────────────────

private class Tile(
    val x: Float,
    val y: Float,
    val w: Float,
    val h: Float,
    val rx: Float,
    val op: Float,
)

private class St(val t: Float, val p: List<Tile>)

private val FOLD = Tile(246f, 246f, 20f, 20f, 10f, 0f)

/** Build a state from its active tiles, padding the rest with the folded tile. */
private fun st(t: Float, vararg tiles: Tile): St {
    val list = tiles.toMutableList()
    while (list.size < 7) list.add(FOLD)
    return St(t, list)
}

private val STATES: Map<String, St> = mapOf(
    "corner" to st(1f, Tile(304.6f, 127f, 54.6f, 54.6f, 16.7f, 1f)),
    "cover" to st(0f, Tile(132f, 132f, 248f, 248f, 58f, 1f)),
    "kanban" to st(
        0f,
        Tile(96f, 104f, 140f, 304f, 24f, 1f),
        Tile(276f, 104f, 140f, 200f, 24f, 1f),
    ),
    "list" to st(
        0f,
        Tile(156f, 132f, 276f, 32f, 16f, 1f),
        Tile(156f, 240f, 276f, 32f, 16f, 1f),
        Tile(156f, 348f, 276f, 32f, 16f, 1f),
        Tile(80f, 130f, 36f, 36f, 10f, 1f),
        Tile(80f, 238f, 36f, 36f, 10f, 1f),
        Tile(80f, 346f, 36f, 36f, 10f, 1f),
    ),
    "timeline" to st(
        0f,
        Tile(200f, 124f, 224f, 32f, 16f, 1f),
        Tile(200f, 240f, 172f, 32f, 16f, 1f),
        Tile(200f, 356f, 200f, 32f, 16f, 1f),
        Tile(112f, 112f, 56f, 56f, 28f, 1f),
        Tile(112f, 228f, 56f, 56f, 28f, 1f),
        Tile(112f, 344f, 56f, 56f, 28f, 1f),
        Tile(124f, 120f, 32f, 272f, 16f, 1f),
    ),
    "gantt" to st(
        0f,
        Tile(80f, 112f, 200f, 48f, 26f, 1f),
        Tile(168f, 196f, 210f, 48f, 26f, 1f),
        Tile(120f, 280f, 170f, 48f, 26f, 1f),
        Tile(236f, 364f, 184f, 48f, 26f, 1f),
    ),
    "matrix" to st(
        0f,
        Tile(96f, 96f, 140f, 140f, 20f, 1f),
        Tile(276f, 96f, 140f, 140f, 20f, 1f),
        Tile(96f, 276f, 140f, 140f, 20f, 1f),
        Tile(276f, 276f, 140f, 140f, 20f, 1f),
    ),
)

// Glyph "t" (Fredoka 600 outline) — identical path + transform as the web engine
// (translate(-86.89, 382) scale(0.34) in the 512×512 scene). One long literal:
// it's the same curve baked into mark-white.svg / ic_launcher_foreground.xml.
@Suppress("MaxLineLength")
private const val GLYPH_D =
    "M1101 9L1101 9Q1050 9 1013.500-2Q977-13 953-36Q929-59 917.500-94.500Q906-130 906-179L906-179L906-339L871-339Q842-340 830.500-356.500Q819-373 819-412L819-412Q819-448 833-465.500Q847-483 875-483L875-483L906-483L906-585Q906-605 909.500-621Q913-637 929-647.500Q945-658 981-658Q1017-658 1033-647Q1049-636 1052.500-619Q1056-602 1056-582L1056-582L1056-485L1123-487Q1142-487 1159-483.500Q1176-480 1187-464Q1198-448 1198-411L1198-411Q1198-377 1187.500-361Q1177-345 1160-340.500Q1143-336 1123-336L1123-336L1056-338L1056-185Q1056-169 1058.500-158.500Q1061-148 1066-143Q1071-138 1080-136Q1089-134 1102-134L1102-134Q1124-134 1141-130Q1158-126 1167.500-112Q1177-98 1177-65L1177-65Q1177-29 1166-13Q1155 3 1137.500 6Q1120 9 1101 9Z"
private const val GLYPH_SCALE = 0.34f
private const val GLYPH_TX = -86.89f
private const val GLYPH_TY = 382f

private class Seg(val a: St, val b: St, val ra: Float, val rb: Float, val dur: Float)

private fun seg(a: St, b: St, ra: Float, rb: Float, dur: Float) = Seg(a, b, ra, rb, dur)

// NB: the `corner` state (the "t" logo with the tile) is deliberately NOT a view
// and there is no intro — the loader is purely the tile unfolding into app layouts,
// so the brand logo no longer "plays" before it (parity with the web engine).
private val VIEWS = listOf("kanban", "list", "timeline", "gantt", "matrix")

private val LOOP: List<Seg> = buildList {
    val cover = STATES.getValue("cover")
    for (name in VIEWS) {
        val v = STATES.getValue(name)
        add(seg(cover, v, 0f, 0f, 540f)) // unfold
        add(seg(v, v, 0f, 0f, 620f)) // hold
        add(seg(v, cover, 0f, 0f, 480f)) // collapse
        add(seg(cover, cover, 0f, 90f, 780f)) // turn
    }
}

private val LTOT: Float = LOOP.sumOf { it.dur.toDouble() }.toFloat()

private class Frame(val t: Float, val rot: Float, val p: List<Tile>)

private fun ease(t: Float): Float =
    if (t < 0.5f) 4f * t * t * t else 1f - (-2f * t + 2f).pow(3) / 2f

private fun lerp(a: Float, b: Float, k: Float): Float = a + (b - a) * k

private fun frameAt(segs: List<Seg>, time: Float): Frame {
    var acc = 0f
    var i = 0
    while (i < segs.size - 1 && time > acc + segs[i].dur) {
        acc += segs[i].dur
        i++
    }
    val s = segs[i]
    val k = ease(min(1f, (time - acc) / s.dur))
    val tiles = s.a.p.indices.map { j ->
        val pa = s.a.p[j]
        val pb = s.b.p[j]
        Tile(
            lerp(pa.x, pb.x, k),
            lerp(pa.y, pb.y, k),
            lerp(pa.w, pb.w, k),
            lerp(pa.h, pb.h, k),
            lerp(pa.rx, pb.rx, k),
            lerp(pa.op, pb.op, k),
        )
    }
    return Frame(lerp(s.a.t, s.b.t, k), lerp(s.ra, s.rb, k), tiles)
}

/** Same-hue diagonal accent gradient over the given box (bottom-left dark →
 *  top-right light, base at centre) — the per-shape sibling of `accentGradient`,
 *  matching the web loader's per-tile `url(#t-accent-grad-svg)` fill. */
private fun boxBrush(color: Color, gradient: Boolean, l: Float, t: Float, r: Float, b: Float): Brush {
    if (!gradient) return SolidColor(color)
    val darker = lerp(color, Color.Black, AccentGradientStrength).copy(alpha = color.alpha)
    val lighter = lerp(color, Color.White, AccentGradientStrength).copy(alpha = color.alpha)
    return ShaderBrush(
        LinearGradientShader(
            from = Offset(l, b),
            to = Offset(r, t),
            colors = listOf(darker, color, lighter),
            colorStops = listOf(0f, 0.5f, 1f),
        ),
    )
}

/**
 * The brand loader. On the launch splash it's the white mark on the purple
 * gradient (`gradient = false`); in-app it's the accent-tinted mark on the app
 * background. Replaces the generic CircularProgressIndicator.
 *
 * @param color    the mark colour (defaults to the active accent)
 * @param gradient when true, each tile/glyph carries the soft accent gradient
 *                 (parity with the web loader); pass `false` for a flat fill
 *                 (the white splash mark).
 */
@Composable
fun TesseraLoader(
    modifier: Modifier = Modifier,
    size: Dp = 66.dp,
    color: Color = Tessera.colors.primary,
    gradient: Boolean = true,
) {
    // Glyph path, transformed into the 512-scene once.
    val glyphPath = remember {
        val p = PathParser().parsePathString(GLYPH_D).toPath()
        val m = Matrix().apply {
            postScale(GLYPH_SCALE, GLYPH_SCALE)
            postTranslate(GLYPH_TX, GLYPH_TY)
        }
        p.asAndroidPath().transform(m)
        p
    }
    val glyphBounds = remember(glyphPath) {
        RectF().also { glyphPath.asAndroidPath().computeBounds(it, true) }
    }

    // Honour "remove animations" (developer options / accessibility): show the
    // static kanban frame, like the web loader's prefers-reduced-motion path.
    val ctx = LocalContext.current
    val reduced = remember {
        Settings.Global.getFloat(ctx.contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f
    }

    var elapsed by remember { mutableStateOf(0f) }
    LaunchedEffect(reduced) {
        if (reduced) return@LaunchedEffect
        var start = 0L
        while (true) {
            withFrameNanos { now ->
                if (start == 0L) start = now
                elapsed = (now - start) / 1_000_000f
            }
        }
    }

    val frame = when {
        reduced -> Frame(0f, 0f, STATES.getValue("kanban").p)
        else -> frameAt(LOOP, elapsed % LTOT)
    }

    Canvas(modifier.size(size)) {
        val s = this.size.minDimension / 512f
        scale(s, pivot = Offset.Zero) {
            // Glyph: only its opacity animates; it is NOT part of the spinning group.
            if (frame.t > 0.001f) {
                drawPath(
                    path = glyphPath,
                    brush = boxBrush(color, gradient, glyphBounds.left, glyphBounds.top, glyphBounds.right, glyphBounds.bottom),
                    alpha = frame.t.coerceIn(0f, 1f),
                )
            }
            // Tiles spin as a group around the scene centre.
            rotate(degrees = frame.rot, pivot = Offset(256f, 256f)) {
                frame.p.forEach { tile ->
                    if (tile.op > 0.001f && tile.w > 0f && tile.h > 0f) {
                        drawRoundRect(
                            brush = boxBrush(color, gradient, tile.x, tile.y, tile.x + tile.w, tile.y + tile.h),
                            topLeft = Offset(tile.x, tile.y),
                            size = Size(max(0f, tile.w), max(0f, tile.h)),
                            cornerRadius = CornerRadius(tile.rx, tile.rx),
                            alpha = tile.op.coerceIn(0f, 1f),
                        )
                    }
                }
            }
        }
    }
}
