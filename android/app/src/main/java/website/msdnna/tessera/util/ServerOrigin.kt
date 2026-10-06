package website.msdnna.tessera.util

import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

/**
 * Is [url] really a resource on *our* server — the one the session belongs to?
 *
 * Compared as an origin (scheme, host, port) plus the root's path subtree, not
 * as a string prefix. `url.startsWith(serverRoot)` — what the two callers used
 * to do — also says yes to `https://<root>.attacker.example/x` and to
 * `https://<root>@attacker.example/x`: both begin with the root's text and both
 * resolve to somebody else's host. The same parser the request itself will go
 * through (OkHttp's [okhttp3.HttpUrl]) reads the host, so the answer cannot
 * drift from where the bytes actually go.
 *
 * The answer gates things the session pays for: the `Authorization` header put
 * on inline images in [website.msdnna.tessera.ui.components.RichContent], and
 * whether a link is allowed to open *inside* the document editor's WebView,
 * which carries the `AndroidDoc` bridge and its `requestToken`. A URL we cannot
 * parse — or one on a scheme HttpUrl does not accept, such as `file:` — is not
 * ours: the callers then skip the header and send the link to the browser.
 *
 * [serverRoot] is the server root, with or without a trailing slash.
 */
fun isOwnServerUrl(url: String, serverRoot: String): Boolean {
    val root = serverRoot.toHttpUrlOrNull() ?: return false
    val target = url.toHttpUrlOrNull() ?: return false
    if (target.scheme != root.scheme || target.host != root.host || target.port != root.port) return false
    // A root may sit under a path (`https://host/tessera`); then only that
    // subtree is ours, and `/tessera-other` is not inside it.
    val rootPath = root.encodedPath.trimEnd('/')
    if (rootPath.isEmpty()) return true
    val path = target.encodedPath
    return path == rootPath || path.startsWith("$rootPath/")
}
