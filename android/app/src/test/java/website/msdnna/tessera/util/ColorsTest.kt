package website.msdnna.tessera.util

import androidx.compose.ui.graphics.Color
import com.google.common.truth.Truth.assertThat
import org.junit.Test

class ColorsTest {
    // ── parseHexColor ────────────────────────────────────────────────────────
    @Test
    fun `parseHexColor null blank returns fallback`() {
        assertThat(parseHexColor(null, Color.Red)).isEqualTo(Color.Red)
        assertThat(parseHexColor("", Color.Red)).isEqualTo(Color.Red)
        assertThat(parseHexColor("   ", Color.Red)).isEqualTo(Color.Red)
    }

    @Test
    fun `parseHexColor six-digit adds opaque alpha`() {
        val c = parseHexColor("#7c5cff", Color.Black)
        assertThat(c.alpha).isEqualTo(1f)
        assertThat(c.red).isWithin(0.005f).of(0x7c / 255f)
        assertThat(c.green).isWithin(0.005f).of(0x5c / 255f)
        assertThat(c.blue).isWithin(0.005f).of(1f)
    }

    @Test
    fun `parseHexColor works without leading hash`() {
        assertThat(parseHexColor("ffffff", Color.Black)).isEqualTo(Color.White)
    }

    @Test
    fun `parseHexColor eight-digit keeps given alpha`() {
        val c = parseHexColor("#80ffffff", Color.Black)
        assertThat(c.alpha).isWithin(0.005f).of(0x80 / 255f)
    }

    @Test
    fun `parseHexColor invalid length or chars returns fallback`() {
        assertThat(parseHexColor("#abc", Color.Red)).isEqualTo(Color.Red) // 3 digits unsupported
        assertThat(parseHexColor("#gggggg", Color.Red)).isEqualTo(Color.Red) // non-hex
    }

    // ── onColor ──────────────────────────────────────────────────────────────
    @Test
    fun `onColor picks dark text on light fill`() {
        // yellow is bright → dark text
        assertThat(onColor(Color(0xFFFFEB3B))).isEqualTo(Color(0xFF1F1F1F))
        assertThat(onColor(Color.White)).isEqualTo(Color(0xFF1F1F1F))
    }

    @Test
    fun `onColor picks white text on dark fill`() {
        assertThat(onColor(Color(0xFF2b0a52))).isEqualTo(Color.White)
        assertThat(onColor(Color.Black)).isEqualTo(Color.White)
    }

    // ── readableHue ──────────────────────────────────────────────────────────
    @Test
    fun `readableHue lightens dark colour for dark theme`() {
        val dark = Color(0xFF101010)
        val out = readableHue(dark, isDark = true)
        assertThat(luminanceApprox(out)).isGreaterThan(luminanceApprox(dark))
    }

    @Test
    fun `readableHue darkens light colour for light theme`() {
        val light = Color(0xFFEFEFEF)
        val out = readableHue(light, isDark = false)
        assertThat(luminanceApprox(out)).isLessThan(luminanceApprox(light))
    }

    @Test
    fun `readableHue preserves alpha`() {
        val c = Color(red = 0.2f, green = 0.4f, blue = 0.6f, alpha = 0.5f)
        assertThat(readableHue(c, isDark = true).alpha).isWithin(0.01f).of(0.5f)
    }

    @Test
    fun `readableHue keeps grey achromatic`() {
        // pure grey has no hue/sat; result stays grey (r==g==b)
        val out = readableHue(Color(0xFF808080), isDark = true)
        assertThat(out.red).isWithin(0.01f).of(out.green)
        assertThat(out.green).isWithin(0.01f).of(out.blue)
    }

    private fun luminanceApprox(c: Color): Float = (c.red + c.green + c.blue) / 3f
}
