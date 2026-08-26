package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Screenshot resolution for help articles (#2795) — the Android side of the web
 * `tests/cx-help-assets.spec.js`.
 *
 * Worth its own test because the failure is silent: the renderer bases its
 * document at `file:///android_asset/richcontent/`, so an un-rewritten
 * `../assets/board-light.png` resolves to a path that doesn't exist and the
 * article simply shows a gap where the picture was.
 */
class HelpAssetsTest {
    private val names = setOf("board-light.png", "board-dark.png", "diagram.png")

    @Test
    fun `a relative src becomes an android_asset url`() {
        assertThat(helpAssetUrl("../assets/board-light.png", dark = false, names = names))
            .isEqualTo(HELP_ASSET_BASE + "board-light.png")
    }

    @Test
    fun `depth of the article does not matter — only the file name`() {
        assertThat(helpAssetUrl("../../assets/board-light.png", dark = false, names = names))
            .isEqualTo(HELP_ASSET_BASE + "board-light.png")
    }

    @Test
    fun `the dark theme gets the dark twin`() {
        assertThat(helpAssetUrl("../assets/board-light.png", dark = true, names = names))
            .isEqualTo(HELP_ASSET_BASE + "board-dark.png")
    }

    @Test
    fun `a shot with no dark twin stays as it is`() {
        assertThat(helpAssetUrl("../assets/diagram.png", dark = true, names = names))
            .isEqualTo(HELP_ASSET_BASE + "diagram.png")
    }

    @Test
    fun `an unbundled file resolves to nothing`() {
        assertThat(helpAssetUrl("../assets/missing.png", dark = false, names = names)).isEmpty()
    }

    @Test
    fun `markdown images are rewritten in place`() {
        val md = "текст\n\n![Доска](../assets/board-light.png)\n\nещё"
        assertThat(resolveHelpImages(md, dark = false, names = names))
            .isEqualTo("текст\n\n![Доска](${HELP_ASSET_BASE}board-light.png)\n\nещё")
    }

    @Test
    fun `the dark twin is substituted in the markdown too`() {
        val md = "![Доска](../assets/board-light.png)"
        assertThat(resolveHelpImages(md, dark = true, names = names))
            .contains("board-dark.png")
    }

    @Test
    fun `external and data images are left alone`() {
        val md = "![a](https://example.com/x.png) ![b](//cdn/x.png) ![c](data:image/png;base64,AA)"
        assertThat(resolveHelpImages(md, dark = true, names = names)).isEqualTo(md)
    }

    @Test
    fun `an unknown local file keeps its original src rather than blanking`() {
        val md = "![Нет такой](../assets/missing.png)"
        assertThat(resolveHelpImages(md, dark = false, names = names)).isEqualTo(md)
    }

    /** The English shots (#2816) live beside the Russian ones as
     *  `<name>-<light|dark>.en.png`; a name with no English twin keeps the
     *  Russian file, which is why they can land in waves. */
    private val withEn = names + setOf("board-light.en.png", "board-dark.en.png")

    @Test
    fun `an english reader gets the english shot`() {
        assertThat(helpAssetUrl("../assets/board-light.png", dark = false, names = withEn, lang = "en"))
            .isEqualTo(HELP_ASSET_BASE + "board-light.en.png")
        assertThat(helpAssetUrl("../assets/board-light.png", dark = true, names = withEn, lang = "en"))
            .isEqualTo(HELP_ASSET_BASE + "board-dark.en.png")
    }

    @Test
    fun `without an english shot the russian one of the same theme is used`() {
        assertThat(helpAssetUrl("../assets/board-light.png", dark = false, names = names, lang = "en"))
            .isEqualTo(HELP_ASSET_BASE + "board-light.png")
        assertThat(helpAssetUrl("../assets/board-light.png", dark = true, names = names, lang = "en"))
            .isEqualTo(HELP_ASSET_BASE + "board-dark.png")
    }

    @Test
    fun `theme beats language`() {
        // A white shot on a dark page hurts to look at; a Russian shot in an
        // English article merely reads as untranslated. So when only the light
        // English twin exists, the dark Russian one still wins.
        val lightEnOnly = names + "board-light.en.png"
        assertThat(helpAssetUrl("../assets/board-light.png", dark = true, names = lightEnOnly, lang = "en"))
            .isEqualTo(HELP_ASSET_BASE + "board-dark.png")
    }

    @Test
    fun `russian resolves exactly as before the language axis`() {
        val md = "![Доска](../assets/board-light.png)"
        assertThat(resolveHelpImages(md, dark = true, names = withEn, lang = "ru"))
            .isEqualTo(resolveHelpImages(md, dark = true, names = withEn))
        assertThat(resolveHelpImages(md, dark = true, names = withEn, lang = "ru"))
            .contains("board-dark.png)")
    }

    @Test
    fun `an ordinary link is not touched — only images`() {
        val md = "[Доски](/help/boards-and-tasks) и ![Доска](../assets/board-light.png)"
        val out = resolveHelpImages(md, dark = false, names = names)
        assertThat(out).contains("[Доски](/help/boards-and-tasks)")
        assertThat(out).contains(HELP_ASSET_BASE + "board-light.png")
    }
}
