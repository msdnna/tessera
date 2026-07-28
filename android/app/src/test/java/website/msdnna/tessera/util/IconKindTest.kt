package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

class IconKindTest {
    @Test
    fun `classifies svg with xml prologue and doctype`() {
        val icon = """<?xml version="1.0"?><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "x.dtd"><svg><text>Pa</text></svg>"""
        val kind = classifyIcon(icon)
        assertThat(kind).isInstanceOf(IconKind.Svg::class.java)
        assertThat((kind as IconKind.Svg).markup).startsWith("<svg")
    }

    @Test
    fun `classifies plain svg`() {
        assertThat(classifyIcon("  <svg></svg>")).isInstanceOf(IconKind.Svg::class.java)
    }

    @Test
    fun `classifies data url image`() {
        assertThat(classifyIcon("data:image/png;base64,AAAA")).isInstanceOf(IconKind.Image::class.java)
    }

    @Test
    fun `classifies curated key and blank`() {
        assertThat(classifyIcon("rocket")).isInstanceOf(IconKind.Curated::class.java)
        assertThat(classifyIcon("")).isEqualTo(IconKind.None)
        assertThat(classifyIcon(null)).isEqualTo(IconKind.None)
    }

    @Test
    fun `light-dark with hex keeps light variant`() {
        val out = sanitizeSvgForAndroid("""<text fill="light-dark(#FFFFFF,#000000)">Pa</text>""")
        assertThat(out).contains("""fill="#FFFFFF"""")
        assertThat(out).doesNotContain("light-dark")
    }

    @Test
    fun `light-dark with nested rgb is paren-aware`() {
        val out = sanitizeSvgForAndroid("""style="fill: light-dark(rgb(244, 91, 105), rgb(233, 101, 113))"""")
        assertThat(out).contains("fill: rgb(244, 91, 105)")
        assertThat(out).doesNotContain("light-dark")
    }

    @Test
    fun `helvetica mapped to sans-serif`() {
        assertThat(sanitizeSvgForAndroid("""<text font-family="Helvetica">Pa</text>"""))
            .contains("""font-family="sans-serif"""")
    }

    @Test
    fun `drops drawio help switch`() {
        val svg = """<g/><switch><g requiredFeatures="x"/><a><text>Text is not SVG - cannot display</text></a></switch>"""
        val out = sanitizeSvgForAndroid(svg)
        assertThat(out).doesNotContain("Text is not SVG")
    }

    @Test
    fun `sanitize is idempotent on clean markup`() {
        val clean = """<svg><rect fill="#f45b69"/><text fill="#FFFFFF" font-family="sans-serif">Pa</text></svg>"""
        assertThat(sanitizeSvgForAndroid(clean)).isEqualTo(clean)
    }
}
