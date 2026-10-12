package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * The phone's column palette has to be able to UNDO itself: without a «no colour»
 * entry a tap on any swatch is permanent on the phone (#2932).
 */
class ColumnColorsTest {
    @Test
    fun `palette opens with the no-colour swatch`() {
        assertThat(ColumnPalette.first()).isEmpty()
        assertThat(ColumnPalette.count { it.isBlank() }).isEqualTo(1)
    }

    @Test
    fun `palette matches the web swatches`() {
        // ColumnHeader.vue `swatches`, same order — the two clients show one board.
        assertThat(ColumnPalette).containsExactly(
            "", "#9aa0aa", "#7c5cff", "#2f80ed", "#0eb0a9", "#18a058", "#f0a020", "#e0533d", "#eb2f96",
        ).inOrder()
    }

    @Test
    fun `no-colour swatch is the selected one for a colourless column`() {
        assertThat(columnSwatchSelected(null, "")).isTrue()
        assertThat(columnSwatchSelected("", "")).isTrue()
        assertThat(columnSwatchSelected("   ", "")).isTrue()
        assertThat(columnSwatchSelected("#18a058", "")).isFalse()
    }

    @Test
    fun `a coloured column selects its own swatch only`() {
        assertThat(columnSwatchSelected("#18a058", "#18a058")).isTrue()
        assertThat(columnSwatchSelected("#18A058", "#18a058")).isTrue()
        assertThat(columnSwatchSelected(" #18a058 ", "#18a058")).isTrue()
        assertThat(columnSwatchSelected("#18a058", "#7c5cff")).isFalse()
        assertThat(columnSwatchSelected(null, "#7c5cff")).isFalse()
    }
}
