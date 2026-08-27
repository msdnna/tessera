package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

class MiscUtilTest {
    // ── diffSegments ─────────────────────────────────────────────────────────
    @Test
    fun `diffSegments equal strings single unchanged segment`() {
        val segs = diffSegments("abc", "abc")
        assertThat(segs).containsExactly(DiffSegment("abc", false))
    }

    @Test
    fun `diffSegments null handled as empty`() {
        assertThat(diffSegments(null, null)).containsExactly(DiffSegment("", false))
        assertThat(diffSegments(null, "x")).containsExactly(DiffSegment("x", true))
    }

    @Test
    fun `diffSegments common prefix and suffix around changed middle`() {
        val segs = diffSegments("hello world", "hello brave world")
        assertThat(segs).containsExactly(
            DiffSegment("hello ", false),
            DiffSegment("brave ", true),
            DiffSegment("world", false),
        ).inOrder()
    }

    @Test
    fun `diffSegments pure append`() {
        val segs = diffSegments("foo", "foobar")
        assertThat(segs).containsExactly(
            DiffSegment("foo", false),
            DiffSegment("bar", true),
        ).inOrder()
    }

    @Test
    fun `diffSegments cyrillic`() {
        val segs = diffSegments("привет мир", "привет добрый мир")
        assertThat(segs.first { it.changed }.text).isEqualTo("добрый ")
    }

    // ── toggleTaskMarker ─────────────────────────────────────────────────────
    @Test
    fun `toggleTaskMarker empty source unchanged`() {
        assertThat(toggleTaskMarker("", 0)).isEmpty()
    }

    @Test
    fun `toggleTaskMarker flips unchecked to checked`() {
        assertThat(toggleTaskMarker("- [ ] task", 0)).isEqualTo("- [x] task")
    }

    @Test
    fun `toggleTaskMarker flips checked to unchecked`() {
        assertThat(toggleTaskMarker("- [x] done", 0)).isEqualTo("- [ ] done")
        assertThat(toggleTaskMarker("- [X] done", 0)).isEqualTo("- [ ] done")
    }

    @Test
    fun `toggleTaskMarker targets nth marker only`() {
        val src = "- [ ] a\n- [ ] b\n- [ ] c"
        assertThat(toggleTaskMarker(src, 1)).isEqualTo("- [ ] a\n- [x] b\n- [ ] c")
    }

    @Test
    fun `toggleTaskMarker supports numbered and star markers`() {
        assertThat(toggleTaskMarker("1. [ ] x", 0)).isEqualTo("1. [x] x")
        assertThat(toggleTaskMarker("* [ ] y", 0)).isEqualTo("* [x] y")
    }

    @Test
    fun `toggleTaskMarker out-of-range index leaves source intact`() {
        val src = "- [ ] only"
        assertThat(toggleTaskMarker(src, 5)).isEqualTo(src)
    }

    @Test
    fun `toggleTaskMarker ignores non-marker lines`() {
        val src = "plain text\nno checkbox"
        assertThat(toggleTaskMarker(src, 0)).isEqualTo(src)
    }

    // ── LocaleOptions ────────────────────────────────────────────────────────
    @Test
    fun `timezoneOptions are sorted value equals label and include known zone`() {
        val tzs = timezoneOptions()
        assertThat(tzs).isNotEmpty()
        assertThat(tzs.all { it.first == it.second }).isTrue()
        assertThat(tzs.map { it.first }).contains("UTC")
        val labels = tzs.map { it.first }
        assertThat(labels).isEqualTo(labels.sorted())
    }

    @Test
    fun `countryOptions localized and sorted`() {
        val ru = countryOptions("ru")
        assertThat(ru).isNotEmpty()
        // every country has a non-blank display name
        assertThat(ru.all { it.second.isNotBlank() }).isTrue()
        // sorted by localized name (lowercased)
        val names = ru.map { it.second.lowercase() }
        assertThat(names).isEqualTo(names.sorted())
    }
}
