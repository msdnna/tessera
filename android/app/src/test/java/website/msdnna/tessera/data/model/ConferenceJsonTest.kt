package website.msdnna.tessera.data.model

import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import org.junit.Test

/**
 * What the conference models make of real server payloads (#2896 §1).
 *
 * The bodies below are the shapes the Go handlers actually emit — the list row
 * with its joined counters, the detail wrapper, the join envelope, a chat page
 * and a recording. A `@SerializedName` that does not match arrives as the field
 * default instead of failing, so a typo here is invisible until a screen shows
 * an empty title, which is why these assert on parsed values rather than on the
 * models compiling.
 */
class ConferenceJsonTest {
    private val gson = Gson()

    @Test
    fun `a list row keeps its joined author and counters`() {
        val json = """
            {"id":"c-1","workspace_id":"ws-1","task_id":null,"created_by":"u-1",
             "title":"Планёрка","description":"Каждый день","scheduled_at":"2026-09-06T10:00:00+03:00",
             "started_at":null,"ended_at":null,"status":"live","recording_ttl_days":30,
             "created_at":"2026-09-05T09:00:00+03:00","updated_at":"2026-09-05T09:00:00+03:00",
             "created_by_name":"Мария","participant_count":7,"active_count":3}
        """.trimIndent()

        val conf = gson.fromJson(json, Conference::class.java)

        assertThat(conf.title).isEqualTo("Планёрка")
        assertThat(conf.createdByName).isEqualTo("Мария")
        // The two numbers the list shows next to every row; a name mismatch would
        // silently render "0 участников" for a full meeting.
        assertThat(conf.participantCount).isEqualTo(7)
        assertThat(conf.activeCount).isEqualTo(3)
        assertThat(conf.isLive).isTrue()
        assertThat(conf.isScheduled).isFalse()
    }

    @Test
    fun `a paused room reads as scheduled, not ended`() {
        // #2879: the last person leaving pauses the call back to 'scheduled' and
        // keeps started_at. A client that inferred "has run ⇒ over" would file a
        // daily standup into the archive after its first morning.
        val json = """
            {"id":"c-1","status":"scheduled","started_at":"2026-09-05T10:00:00+03:00","ended_at":null}
        """.trimIndent()

        val conf = gson.fromJson(json, Conference::class.java)

        assertThat(conf.isScheduled).isTrue()
        assertThat(conf.isEnded).isFalse()
        assertThat(conf.isLive).isFalse()
    }

    @Test
    fun `presence is the pair of stamps, not just a join`() {
        val detail = gson.fromJson(
            """
            {"conference":{"id":"c-1","title":"Ретро","status":"live"},
             "participants":[
               {"conference_id":"c-1","user_id":"u-1","role":"host","invited_at":"2026-09-05T09:00:00+03:00",
                "joined_at":"2026-09-05T10:00:00+03:00","left_at":null,"force_muted":false,
                "user_name":"Мария","user_email":"m@example.com"},
               {"conference_id":"c-1","user_id":"u-2","role":"member","invited_at":"2026-09-05T09:00:00+03:00",
                "joined_at":"2026-09-04T10:00:00+03:00","left_at":"2026-09-04T11:00:00+03:00","force_muted":true,
                "user_name":"Пётр","user_email":"p@example.com"}]}
            """.trimIndent(),
            ConferenceDetail::class.java,
        )

        assertThat(detail.conference.title).isEqualTo("Ретро")
        val (host, past) = detail.participants
        assertThat(host.isHost).isTrue()
        assertThat(host.isPresent).isTrue()
        // Yesterday's attendee has both stamps: reading joined_at alone would draw
        // a room full of people who left hours ago.
        assertThat(past.isPresent).isFalse()
        assertThat(past.isHost).isFalse()
        assertThat(past.forceMuted).isTrue()
    }

    @Test
    fun `join returns the restarted call together with our own row`() {
        val membership = gson.fromJson(
            """
            {"conference":{"id":"c-1","status":"live","started_at":"2026-09-05T10:00:00+03:00"},
             "participant":{"conference_id":"c-1","user_id":"u-3","role":"member",
                            "joined_at":"2026-09-05T10:00:01+03:00","left_at":null}}
            """.trimIndent(),
            ConferenceMembership::class.java,
        )

        assertThat(membership.conference.isLive).isTrue()
        assertThat(membership.participant.userId).isEqualTo("u-3")
        assertThat(membership.participant.isPresent).isTrue()
    }

    @Test
    fun `a chat page carries attachments and the older-text flag`() {
        val page = gson.fromJson(
            """
            {"messages":[
               {"id":"m-1","user_id":"u-1","user_name":"Мария","body":"Смотрите смету",
                "created_at":"2026-09-05T10:05:00+03:00",
                "attachments":[{"id":"a-1","message_id":"m-1","filename":"смета.pdf",
                                "type":"application/pdf","size":40960,"is_image":false,
                                "created_at":"2026-09-05T10:05:00+03:00"}]},
               {"id":"m-2","user_id":"u-2","user_name":"Пётр","body":"Ок",
                "created_at":"2026-09-05T10:06:00+03:00","attachments":[]}],
             "has_more":true}
            """.trimIndent(),
            ConferenceMessagePage::class.java,
        )

        assertThat(page.hasMore).isTrue()
        assertThat(page.messages).hasSize(2)
        val file = page.messages.first().attachments.single()
        assertThat(file.filename).isEqualTo("смета.pdf")
        assertThat(file.size).isEqualTo(40_960L)
        // is_image is the server's call — the client must not re-derive it from
        // the extension, or a .pdf named "снимок.png.pdf" renders as a broken img.
        assertThat(file.isImage).isFalse()
    }

    @Test
    fun `recording statuses match the column's CHECK`() {
        val recs = gson.fromJson(
            """
            [{"id":"r-1","conference_id":"c-1","status":"active","error":"","file_name":"rec-1.mp4",
              "size_bytes":0,"duration_sec":0,"started_at":"2026-09-05T10:00:00+03:00",
              "ended_at":null,"started_by":"u-1","started_by_name":"Мария"},
             {"id":"r-2","conference_id":"c-1","status":"completed","error":"","file_name":"rec-2.mp4",
              "size_bytes":1048576,"duration_sec":900,"started_at":"2026-09-04T10:00:00+03:00",
              "ended_at":"2026-09-04T10:15:00+03:00"},
             {"id":"r-3","conference_id":"c-1","status":"failed","error":"egress unreachable",
              "file_name":"rec-3.mp4","started_at":"2026-09-03T10:00:00+03:00"}]
            """.trimIndent(),
            Array<ConferenceRecording>::class.java,
        ).toList()

        val (running, done, failed) = recs
        assertThat(running.isRunning).isTrue()
        assertThat(running.isReady).isFalse()
        assertThat(done.isReady).isTrue()
        assertThat(done.sizeBytes).isEqualTo(1_048_576L)
        assertThat(done.durationSec).isEqualTo(900)
        // A failed row is listed on purpose — it is what people come asking about,
        // and the reason has to survive the parse to be shown.
        assertThat(failed.isFailed).isTrue()
        assertThat(failed.error).isEqualTo("egress unreachable")
    }

    @Test
    fun `a token arrives with the SFU url and its lifetime`() {
        val token = gson.fromJson(
            """{"url":"wss://sfu.example.com","token":"ey.jwt","room":"conf-c-1",
                "identity":"u-1","expires_in":3600}""",
            ConferenceToken::class.java,
        )

        assertThat(token.url).isEqualTo("wss://sfu.example.com")
        assertThat(token.room).isEqualTo("conf-c-1")
        // Seconds, not a deadline: §4 re-requests on reconnect rather than
        // caching for the meeting.
        assertThat(token.expiresIn).isEqualTo(3600)
    }
}
