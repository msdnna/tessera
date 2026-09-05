package website.msdnna.tessera.util

import android.net.Uri
import org.json.JSONObject

// The app's half of the contract with the embedded document editor (#2894 §4) —
// the URL that opens it and the signals it sends back.
//
// Kept here, free of Compose and of the WebView, because this is the part that
// can be wrong in a way nothing on screen would show: a mistyped parameter name
// gives a blank editor, and a status word the app does not recognise silently
// reads as «сохранено» over text that was never saved.

/** What the editor is doing with the user's text right now. */
enum class DocSaveStatus {
    /** Everything typed is on the server. */
    SAVED,

    /** Edits are queued behind the debounce. */
    DIRTY,

    /** A write is in flight. */
    SAVING,

    /** The document changed elsewhere; saving has stopped (409). */
    CONFLICT,

    /** The last write failed for another reason; it stays queued. */
    ERROR,
}

/** One status report from the page: the state and, for [DocSaveStatus.ERROR], why. */
data class DocEditorSignal(
    val status: DocSaveStatus = DocSaveStatus.SAVED,
    val message: String = "",
)

/**
 * Builds the URL of the embedded editor.
 *
 * The access token travels in the query because a WebView's first request is a
 * navigation — there is no header to put it in. It is the *access* token alone:
 * the refresh token stays in the app (see `RetrofitClient`), so the page and the
 * app never rotate each other's session away. When the short-lived one dies the
 * page asks for a new one over the bridge.
 *
 * [serverRoot] is the server root, not the API base: the SPA is served from `/`
 * and the API from `/api`.
 */
fun docEmbedUrl(
    serverRoot: String,
    slug: String,
    workspaceId: String,
    token: String,
    dark: Boolean,
    language: String,
): String {
    val root = serverRoot.trimEnd('/')
    val theme = if (dark) "dark" else "light"
    return root + "/embed/document/" + Uri.encode(slug) +
        "?ws=" + Uri.encode(workspaceId) +
        "&token=" + Uri.encode(token) +
        "&theme=" + theme +
        "&lang=" + Uri.encode(language)
}

/**
 * Reads a status report.
 *
 * Anything unparseable or unknown becomes [DocSaveStatus.ERROR] rather than the
 * enum's first entry: a report we cannot read is precisely the case where
 * claiming "сохранено" would be a lie.
 */
fun parseDocEditorSignal(payload: String): DocEditorSignal {
    val obj = runCatching { JSONObject(payload) }.getOrNull()
        ?: return DocEditorSignal(DocSaveStatus.ERROR)
    val status = when (obj.optString("status")) {
        "saved" -> DocSaveStatus.SAVED
        "dirty" -> DocSaveStatus.DIRTY
        "saving" -> DocSaveStatus.SAVING
        "conflict" -> DocSaveStatus.CONFLICT
        else -> DocSaveStatus.ERROR
    }
    return DocEditorSignal(status, obj.optString("error"))
}

/** The document the page opened: its id and the title it actually has on the
 *  server (a rename made elsewhere arrives with it). */
data class DocEditorReady(val id: String = "", val title: String = "")

fun parseDocEditorReady(payload: String): DocEditorReady {
    val obj = runCatching { JSONObject(payload) }.getOrNull() ?: return DocEditorReady()
    return DocEditorReady(obj.optString("id"), obj.optString("title"))
}
