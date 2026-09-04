package website.msdnna.tessera.data.repository

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * The 409 branch of a content save (#2894).
 *
 * A lost race is the one moment the editor is holding text nobody else has, so
 * the parse of the server's error body must never throw: a crash there loses the
 * edit the merge exists to save.
 */
class DocConflictParseTest {
    @Test
    fun `the server timestamp is read out of a real 409 body`() {
        // Verbatim from the backend on :8094 — the second save of the same stale
        // updated_at, captured with the fixtures for DocumentJsonTest.
        val body = """
            {"error":"document changed elsewhere","updated_at":"2026-09-05T00:28:25.716274+03:00"}
        """.trimIndent()
        assertThat(parseDocConflictTimestamp(body)).isEqualTo("2026-09-05T00:28:25.716274+03:00")
    }

    @Test
    fun `a body we cannot read yields an empty string, not an exception`() {
        // Each of these is a shape a proxy, a timeout page or an older server can
        // put in front of us. The caller treats "" as "refetch the document".
        assertThat(parseDocConflictTimestamp(null)).isEmpty()
        assertThat(parseDocConflictTimestamp("")).isEmpty()
        assertThat(parseDocConflictTimestamp("<html>502 Bad Gateway</html>")).isEmpty()
        assertThat(parseDocConflictTimestamp("""{"error":"document changed elsewhere"}""")).isEmpty()
        assertThat(parseDocConflictTimestamp("""["not","an","object"]""")).isEmpty()
    }
}
