package website.msdnna.tessera.util

import java.io.File
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Every [Ion] constant is an `assets/ionicons/<name>.svg` file name, resolved at
 * runtime by Coil — a typo or a missing asset renders *nothing*, silently, with no
 * compile error. This walks the constants against the asset directory instead.
 *
 * Unit tests run with the module directory as their working dir (Gradle default).
 */
class IonAssetsTest {
    private val assetsDir = File("src/main/assets/ionicons")

    @Test
    fun `every Ion constant has a bundled svg`() {
        assertTrue("assets/ionicons not found from ${File(".").absolutePath}", assetsDir.isDirectory)
        val missing = Ion::class.java.declaredFields
            .filter { it.type == String::class.java }
            .mapNotNull { field ->
                field.isAccessible = true
                (field.get(Ion) as? String)?.takeIf { !File(assetsDir, "$it.svg").isFile }
            }
        assertTrue("missing ionicons assets: $missing", missing.isEmpty())
    }

    /** The chips of the composer bar carry these two (added with the web-parity
     *  icon chips); they had no Android asset before. */
    @Test
    fun `composer chip icons person and ribbon are bundled`() {
        assertTrue(File(assetsDir, "${Ion.PERSON}.svg").isFile)
        assertTrue(File(assetsDir, "${Ion.RIBBON}.svg").isFile)
    }
}
