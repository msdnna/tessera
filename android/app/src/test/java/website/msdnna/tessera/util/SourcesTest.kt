package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Provenance dictionary (web `utils/sources.js` parity). */
class SourcesTest {
    @Test
    fun `known sources carry a friendly label`() {
        assertEquals("Tessera", sourceMeta("user").label)
        assertEquals("GitLab", sourceMeta("gitlab").label)
        assertEquals(Ion.GITLAB, sourceMeta("gitlab").icon)
        // "Tessera" is not a provider — no icon, so the badge stays text-only.
        assertEquals(null, sourceMeta("user").icon)
    }

    @Test
    fun `an unknown source falls back to its raw name, blank to a dash`() {
        assertEquals("jira", sourceMeta("jira").label)
        assertEquals(null, sourceMeta("jira").icon)
        assertEquals("—", sourceMeta("").label)
        assertEquals("—", sourceMeta(null).label)
    }

    @Test
    fun `only a non-user source counts as external`() {
        assertTrue(isExternalSource("gitlab"))
        assertTrue(isExternalSource("jira"))
        assertFalse(isExternalSource("user"))
        // Legacy rows predate the column: absent/blank means the user made it.
        assertFalse(isExternalSource(null))
        assertFalse(isExternalSource(""))
    }
}
