package website.msdnna.tessera.util

import retrofit2.HttpException
import website.msdnna.tessera.data.model.ConferenceRecording

/*
 * The rules the recordings run on (#2896 §8, web `ConferenceRecordings.vue` plus
 * the room's own red dot).
 *
 * Two surfaces, one subject, and they are split on purpose: *that* a call is
 * being recorded belongs in the room, where the people who did not press the
 * button are, and the *files* belong in the lobby, where they are still there
 * tomorrow. Neither reads the other's state.
 *
 * The recording is server-side (an egress worker joins the call and writes the
 * file), so nothing here captures anything. What this side owes the user is
 * honesty about a service that may not be installed at all — see
 * [confRecordingFailure].
 */

/** Why a start or stop was refused, as far as it changes what we say. */
enum class ConfRecordingFailure {
    /** No egress on this install, or it is not answering. Not a retry. */
    UNAVAILABLE,

    /** The server does not accept this from us — moderators only. */
    FORBIDDEN,

    /** Already running, or already stopped: the button raced another moderator. */
    CONFLICT,

    /** Anything else; the server's own sentence is shown instead. */
    OTHER,
}

/**
 * Classifies a failed recording call.
 *
 * A 502/503 is the honest «сервис недоступен» this feature was asked to give:
 * an install without the egress worker answers every start that way, and the
 * one thing the user must not be told is «попробуйте ещё раз». It is a fact
 * about the server, not a failure to retry — same shape as the media banner's
 * [ConfMediaStatus.UNAVAILABLE], and for the same reason.
 */
fun confRecordingFailure(t: Throwable): ConfRecordingFailure = when {
    t !is HttpException -> ConfRecordingFailure.OTHER
    t.code() == HTTP_BAD_GATEWAY || t.code() == HTTP_UNAVAILABLE -> ConfRecordingFailure.UNAVAILABLE
    t.code() == HTTP_FORBIDDEN -> ConfRecordingFailure.FORBIDDEN
    t.code() == HTTP_CONFLICT -> ConfRecordingFailure.CONFLICT
    else -> ConfRecordingFailure.OTHER
}

/** What the recording button does when pressed. */
enum class ConfRecordingPress {
    START,
    STOP,

    /** Not ours to press — the control is not shown at all. */
    NONE,
}

/**
 * Who may press it, and to what effect.
 *
 * Hidden rather than disabled for a member, unlike the microphone under a
 * force-mute: a greyed-out record button on every participant's toolbar
 * advertises a capability nobody but a moderator has, and the toolbar is already
 * two rows deep on a phone.
 *
 * [running] is the room snapshot's, never a local flag flipped on the press:
 * the egress worker takes a second or two to actually join, and a button that
 * said «идёт запись» before it did would be lying to the one person who most
 * needs to be right about it.
 */
fun confRecordingPress(canModerate: Boolean, running: Boolean): ConfRecordingPress = when {
    !canModerate -> ConfRecordingPress.NONE
    running -> ConfRecordingPress.STOP
    else -> ConfRecordingPress.START
}

/**
 * Orders the list: newest first, and a running recording always at the top.
 *
 * Sorted here rather than trusted from the server because the two sources
 * disagree by a moment — a row that has just started arrives through a different
 * path than the list refetch — and a recording that jumped into the middle of
 * the list is one nobody finds.
 */
fun confRecordingOrder(rows: List<ConferenceRecording>): List<ConferenceRecording> =
    rows.sortedWith(
        compareBy(
            { !it.isRunning },
            { -(parseInstantMillis(it.startedAt) ?: 0L) },
        ),
    )

/** Whether a row can be handed to the system viewer: a file has to exist first. */
fun confRecordingDownloadable(rec: ConferenceRecording): Boolean =
    rec.isReady && rec.id.isNotBlank()

/**
 * The name a downloaded file is saved under.
 *
 * The server's name when it sent one, the id otherwise — never blank, because
 * that would write a directory. Path separators are stripped for the same reason
 * chat attachments strip them: the name comes from the other side.
 */
fun confRecordingFileName(rec: ConferenceRecording): String {
    val raw = rec.fileName.ifBlank { "$RECORDING_PREFIX${rec.id}$MP4_SUFFIX" }
    return raw.replace(Regex("[/\\\\]"), "_")
}

/** Hours/minutes/seconds of a finished recording; null while it is still running. */
data class ConfRecordingLength(val hours: Int, val minutes: Int, val seconds: Int)

/**
 * Splits a duration for display.
 *
 * Returns null for a running or zero-length row rather than «00:00»: the length
 * of a recording being written is not known yet, and a zero printed next to a
 * live red dot reads as a broken one.
 */
fun confRecordingLength(rec: ConferenceRecording): ConfRecordingLength? {
    if (rec.isRunning || rec.durationSec <= 0) return null
    val total = rec.durationSec
    return ConfRecordingLength(
        hours = total / SECONDS_PER_HOUR,
        minutes = (total % SECONDS_PER_HOUR) / SECONDS_PER_MINUTE,
        seconds = total % SECONDS_PER_MINUTE,
    )
}

private const val HTTP_FORBIDDEN = 403
private const val HTTP_CONFLICT = 409
private const val HTTP_BAD_GATEWAY = 502
private const val HTTP_UNAVAILABLE = 503

private const val SECONDS_PER_MINUTE = 60
private const val SECONDS_PER_HOUR = 3600

private const val RECORDING_PREFIX = "recording-"
private const val MP4_SUFFIX = ".mp4"
