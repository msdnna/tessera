package website.msdnna.tessera.ui.components

import android.annotation.SuppressLint
import android.content.Intent
import android.graphics.Color as AColor
import android.net.Uri
import android.webkit.JavascriptInterface
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.util.DocEditorReady
import website.msdnna.tessera.util.DocEditorSignal
import website.msdnna.tessera.util.DocSaveStatus
import website.msdnna.tessera.util.parseDocEditorReady
import website.msdnna.tessera.util.parseDocEditorSignal

/**
 * Commands the app sends *into* the open editor. The other direction (status,
 * conflicts, taps) arrives through the bridge callbacks below.
 *
 * Held by the screen and handed to the view, rather than the view exposing the
 * WebView: everything the app is allowed to ask of the page is on this class,
 * and nothing else can reach the DOM.
 */
class DocEditorController {
    private var web: WebView? = null

    internal fun attach(view: WebView?) {
        web = view
    }

    private fun call(js: String) {
        val view = web ?: return
        view.post { view.evaluateJavascript(js, null) }
    }

    /** Writes whatever is queued right now. Called before the editor closes, so
     *  the last keystrokes are never the ones that get lost. */
    fun save() = call("window.tesseraEmbed && window.tesseraEmbed.save()")

    /** Drops local text for the server's version — how a conflict ends. */
    fun reload() = call("window.tesseraEmbed && window.tesseraEmbed.reload()")

    /** Answers the page's `requestToken`, or pushes a token refreshed meanwhile. */
    fun pushToken(token: String) =
        call("window.tesseraEmbed && window.tesseraEmbed.setToken(${JSONObject.quote(token)})")
}

/**
 * The document editor itself: the web editor, in a WebView, with the app's
 * session and theme (#2894 §4).
 *
 * The decision behind this is in `DocEmbedView.vue` — Compose has no rich-text
 * model of ProseMirror's shape, and a partial native editor would drop the
 * attributes it does not understand the first time a typo is fixed on a phone.
 * Everything around the text (list, reader, comments, history, the bar above
 * this view) is native.
 *
 * Unlike [RichContent] this view *does* scroll: it is a whole editing surface,
 * not a block of rendered markdown sized to its content.
 */
@SuppressLint("SetJavaScriptEnabled")
@Composable
fun DocEditorWebView(
    url: String,
    controller: DocEditorController,
    scope: CoroutineScope,
    modifier: Modifier = Modifier,
    onReady: (DocEditorReady) -> Unit = {},
    onStatus: (DocEditorSignal) -> Unit = {},
    onBlocked: (String) -> Unit = {},
    onRemoteChange: () -> Unit = {},
    onLoadFailed: () -> Unit = {},
) {
    val ctx = LocalContext.current
    // The long-lived bridge always calls the current callbacks, not the ones
    // that happened to be in scope when the WebView was created.
    val readyCb by rememberUpdatedState(onReady)
    val statusCb by rememberUpdatedState(onStatus)
    val blockedCb by rememberUpdatedState(onBlocked)
    val remoteCb by rememberUpdatedState(onRemoteChange)
    val failedCb by rememberUpdatedState(onLoadFailed)
    val held = rememberUpdatedState(controller)

    AndroidView(
        modifier = modifier,
        factory = { context ->
            WebView(context).apply {
                setBackgroundColor(AColor.TRANSPARENT)
                settings.javaScriptEnabled = true
                // The editor stores images through our API and reads them back
                // from the same origin, so the page needs its own storage for
                // ProseMirror's clipboard/undo scratch space.
                settings.domStorageEnabled = true
                settings.mediaPlaybackRequiresUserGesture = false
                addJavascriptInterface(
                    object {
                        @JavascriptInterface
                        fun onReady(payload: String) {
                            post { readyCb(parseDocEditorReady(payload)) }
                        }

                        @JavascriptInterface
                        fun onStatus(payload: String) {
                            post { statusCb(parseDocEditorSignal(payload)) }
                        }

                        @JavascriptInterface
                        fun onError(message: String) {
                            post { statusCb(DocEditorSignal(DocSaveStatus.ERROR, message)) }
                        }

                        /** A keystroke refused because someone else holds the
                         *  block. The page has no toast of its own — a web toast
                         *  inside a native screen reads as borrowed. */
                        @JavascriptInterface
                        fun onBlocked(name: String) {
                            post { blockedCb(name) }
                        }

                        @JavascriptInterface
                        fun onRemoteChange() {
                            post { remoteCb() }
                        }

                        /** The page's access token expired. The refresh token is
                         *  ours alone, so only we can answer — off the main
                         *  thread, since the exchange is a blocking call. */
                        @JavascriptInterface
                        fun requestToken() {
                            scope.launch {
                                val token = withContext(Dispatchers.IO) {
                                    RetrofitClient.refreshAccess() ?: RetrofitClient.authToken
                                }
                                held.value.pushToken(token)
                            }
                        }
                    },
                    "AndroidDoc",
                )
                webViewClient = object : WebViewClient() {
                    override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                        // Our own page navigating (the editor never does, but a
                        // redirect through the login guard would) stays inside;
                        // a link in the text opens in the browser, as it does in
                        // the reader.
                        val link = request.url.toString()
                        if (link.startsWith(RetrofitClient.serverRoot)) return false
                        runCatching { ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(link))) }
                        return true
                    }

                    override fun onReceivedError(
                        view: WebView,
                        request: WebResourceRequest,
                        error: WebResourceError,
                    ) {
                        // Only the document itself. A failed image inside the
                        // text is not a reason to replace the editor with an
                        // error screen.
                        if (request.isForMainFrame) view.post { failedCb() }
                    }
                }
                controller.attach(this)
            }
        },
        update = { view ->
            controller.attach(view)
            // Loaded once per document: the URL carries the access token, and
            // reloading on a recomposition would drop the caret and the undo
            // stack on the floor.
            if (view.url != url) view.loadUrl(url)
        },
        onRelease = { view ->
            controller.attach(null)
            view.destroy()
        },
    )
}
