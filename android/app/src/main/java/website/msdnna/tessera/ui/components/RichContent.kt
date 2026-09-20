package website.msdnna.tessera.ui.components

import android.annotation.SuppressLint
import android.content.Context
import android.content.Intent
import android.content.res.Resources
import android.graphics.Color as AColor
import android.net.Uri
import android.webkit.JavascriptInterface
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import java.io.File
import java.security.SecureRandom
import java.util.Locale
import kotlin.math.roundToInt
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.Request
import org.json.JSONObject
import website.msdnna.tessera.R
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.api.TlsTrust
import website.msdnna.tessera.ui.screens.openDownloadedFile
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraColors
import website.msdnna.tessera.util.MentionItem
import website.msdnna.tessera.util.attachmentIdFromHref
import website.msdnna.tessera.util.filenameFromContentDisposition
import website.msdnna.tessera.util.isOwnServerUrl
import website.msdnna.tessera.util.resolveMention
import website.msdnna.tessera.util.safeDownloadName

/**
 * Renders stored Markdown exactly like the web `RichContent.vue`: marked → HTML,
 * highlight.js for fenced code, and lazily-loaded mermaid for ```mermaid blocks.
 * A WebView is the faithful (and only sane) way to reproduce mermaid diagrams +
 * syntax highlighting; everything around it (tabs, editor, controls) is native.
 *
 * Разметка из задачи недоверенная, поэтому — как и на web — между marked и
 * `innerHTML` стоит DOMPurify: markdown допускает сырой HTML, а этот WebView
 * куда опаснее вкладки браузера (JS включён, есть мост `AndroidRich`, а
 * `shouldInterceptRequest` подставляет Bearer к запросам на свой сервер).
 *
 * marked, DOMPurify, highlight.js и его тема лежат в `assets/richcontent` — сеть не участвует
 * в отрисовке вообще (#2897: внешний `<link>` на CDN блокировал первый кадр, и на
 * телефоне без выхода наружу описание появлялось через десяток секунд). Из сети
 * тянется только mermaid — он нужен единицам, весит полмегабайта и грузится лениво,
 * а его отсутствие уже обработано `onerror`. The view reports its content height
 * back so it sizes to its content inside the scrolling modal.
 */
@SuppressLint("SetJavaScriptEnabled")
@Composable
fun RichContent(
    source: String,
    modifier: Modifier = Modifier,
    mentions: List<MentionItem> = emptyList(),
    // When true, GFM task checkboxes become clickable; a click reports the box
    // index via [onToggleCheck] so the caller can rewrite the stored markdown.
    interactive: Boolean = false,
    onToggleCheck: ((Int) -> Unit)? = null,
    // Opt-in: tapping an @-mention opens a card naming the person. Off by default
    // because RichContent also draws the description preview on board cards, where
    // a floating card would cover neighbours and fight drag-and-drop.
    mentionCards: Boolean = false,
    // Opt-in: render "#123" as a link and report a tap on it. Off on board cards,
    // where the link would swallow clicks meant for the card.
    taskRefs: Boolean = false,
    onTaskRef: ((Int) -> Unit)? = null,
    // Opt-in: `/help/<slug>` links between help articles stay inside the app and
    // report the slug instead of opening the site in a browser (#2795). Off
    // everywhere else, where such a path is an ordinary server link.
    helpLinks: Boolean = false,
    // The slugs this build actually bundles. A cross-link to an article the app
    // does not ship (the admin topics are web-only) is left as an ordinary server
    // link, so a tap opens it in the manual on the site instead of doing nothing.
    helpSlugs: List<String> = emptyList(),
    onHelpLink: ((String) -> Unit)? = null,
) {
    val c = Tessera.colors
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    // Ресурсы берём в композиции, а не у контекста в момент клика: диалог «чем
    // открыть» показывается уже вне её, где `ctx.getString` возьмёт язык
    // системы, а не профиля (тот же приём, что в «Файлах» задачи).
    val res = LocalResources.current
    val serverRoot = RetrofitClient.serverRoot
    var heightDp by remember { mutableStateOf(1) }
    // Latest callback, so the long-lived JS bridge always calls the current one.
    val toggleCb by rememberUpdatedState(onToggleCheck)
    val taskRefCb by rememberUpdatedState(onTaskRef)
    val helpLinkCb by rememberUpdatedState(onHelpLink)
    val roster by rememberUpdatedState(mentions)
    // The mention card and where its chip sits (CSS px inside the view — with the
    // viewport at initial-scale=1 those are dp, as the height report already assumes).
    var card by remember { mutableStateOf<MentionItem?>(null) }
    var cardX by remember { mutableStateOf(0) }
    var cardY by remember { mutableStateOf(0) }
    // A self-originated checkbox tap already toggled (and animated) the box in the
    // WebView, so skip the reload the resulting markdown change would trigger —
    // reloading would wipe the tick animation. `lastLoaded` makes `update` act only
    // on a genuine html change (not every recomposition).
    val skipNextLoad = remember { mutableStateOf(false) }
    val lastLoaded = remember { mutableStateOf<String?>(null) }

    // Rebuild the document whenever the source, theme, mentions or mode changes.
    val html = remember(source, c.isDark, mentions, interactive, mentionCards, taskRefs, helpLinks, helpSlugs) {
        // Both handles a person can be written by are matched, longer first.
        val handles = mentions.flatMap { listOf(it.insert, it.display) }.filter { it.isNotBlank() }.distinct()
        buildRichHtml(source, c, serverRoot, handles, interactive, mentionCards, taskRefs, helpLinks, helpSlugs)
    }

    Box(modifier.fillMaxWidth()) {
        AndroidView(
            modifier = Modifier.fillMaxWidth().height(heightDp.dp),
            factory = {
                NonScrollingWebView(it).apply {
                    setBackgroundColor(AColor.TRANSPARENT)
                    settings.javaScriptEnabled = true
                    settings.mixedContentMode = android.webkit.WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
                    settings.allowFileAccess = true
                    addJavascriptInterface(
                        object {
                            @JavascriptInterface
                            fun onHeight(px: Int) {
                                post { heightDp = px.coerceIn(1, 6000) }
                            }

                            @JavascriptInterface
                            fun onCheckToggle(index: Int) {
                                post {
                                    skipNextLoad.value = true
                                    toggleCb?.invoke(index)
                                }
                            }

                            /** A tapped @-mention: the card opens by the chip, and
                             *  only for a handle somebody in the roster owns. */
                            @JavascriptInterface
                            fun onMention(handle: String, x: Int, y: Int) {
                                post {
                                    val item = resolveMention(roster, handle) ?: return@post
                                    cardX = x
                                    cardY = y
                                    card = item
                                }
                            }

                            @JavascriptInterface
                            fun onTaskRef(number: Int) {
                                post { taskRefCb?.invoke(number) }
                            }

                            @JavascriptInterface
                            fun onHelpLink(slug: String) {
                                post { helpLinkCb?.invoke(slug) }
                            }
                        },
                        "AndroidRich",
                    )
                    webViewClient = object : WebViewClient() {
                        override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                            val url = request.url
                            return when (val action = linkAction(url.toString(), url.scheme, RetrofitClient.serverRoot)) {
                                is LinkAction.Attachment -> {
                                    saveAttachmentLink(ctx, scope, res, action.url)
                                    true
                                }

                                is LinkAction.External -> {
                                    runCatching { ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(action.url))) }
                                    true
                                }

                                LinkAction.None -> false
                            }
                        }

                        // Inline images live on our backend (`/api/uploads/…`). The
                        // WebView's own network stack can't load them (its TLS/policy
                        // differs from the app's working OkHttp — and any future auth
                        // wouldn't be sent), so fetch our-server resources through the
                        // same OkHttp path Coil uses and hand the bytes back.
                        override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse? {
                            val url = request.url.toString()
                            // By origin, not by text: a string prefix would hand the
                            // session's Bearer to `https://<root>.attacker.example/x`,
                            // which any `![](…)` in a task can point at.
                            if (request.method != "GET" || !isOwnServerUrl(url, RetrofitClient.serverRoot)) return null
                            return runCatching { fetchResource(url) }.getOrNull()
                        }
                    }
                }
            },
            update = { web ->
                // Only (re)load on a genuine html change. A self-toggle changed the html
                // (markdown rewrite) but the WebView DOM already reflects + animated it,
                // so adopt the new html without reloading.
                if (html != lastLoaded.value) {
                    if (skipNextLoad.value) {
                        skipNextLoad.value = false
                    } else {
                        web.loadDataWithBaseURL("file:///android_asset/richcontent/", html, "text/html", "utf-8", null)
                    }
                    lastLoaded.value = html
                }
            },
        )
        // Anchored to the chip: a zero-size box at the reported position, with the
        // card hanging off it (the popup positions against its parent's bounds).
        val shown = card
        if (shown != null) {
            Box(Modifier.offset(cardX.dp, cardY.dp).size(1.dp)) {
                MentionCardPopup(shown) { card = null }
            }
        }
    }
}

/**
 * A WebView that must never scroll its own content. The document is sized to fit
 * (the page reports its height back), so internal scrolling can only mean the
 * height report lagged a frame — and then the view pans its own content inside a
 * too-short window: text gets clipped mid-line and the drag never reaches the
 * scrolling modal around it (#2781).
 *
 * Reporting the right height is the actual fix; this is the guard that keeps the
 * symptom from returning. Telling the scrolling machinery the vertical range
 * equals the visible extent is what does the work — with nothing to scroll,
 * Chromium neither moves the content nor asks the parent to stop intercepting
 * the drag. [onScrollChanged] pins the offset for anything that scrolls the view
 * programmatically (focus/anchor jumps).
 */
internal class NonScrollingWebView(context: Context) : WebView(context) {
    init {
        overScrollMode = OVER_SCROLL_NEVER
        isVerticalScrollBarEnabled = false
        isHorizontalScrollBarEnabled = false
    }

    override fun computeVerticalScrollRange(): Int = computeVerticalScrollExtent()

    override fun onOverScrolled(scrollX: Int, scrollY: Int, clampedX: Boolean, clampedY: Boolean) {
        super.onOverScrolled(scrollX, 0, clampedX, clampedY)
    }

    override fun onScrollChanged(l: Int, t: Int, oldl: Int, oldt: Int) {
        super.onScrollChanged(l, 0, oldl, oldt)
        if (scrollY != 0) scrollTo(l, 0)
    }
}

/**
 * Что делать с нажатой ссылкой. Решение вынесено из `shouldOverrideUrlLoading`
 * отдельно, потому что проверяется оно без WebView — а WebView здесь и есть то
 * место, где проглядели [LinkAction.Attachment].
 */
internal sealed interface LinkAction {
    /**
     * Ссылка на файл задачи (`/api/attachments/<uuid>/download`) на нашем же
     * сервере. Такую нельзя отдавать браузеру: маршрут в защищённой группе, а
     * токен живёт только в памяти приложения — браузер получит 401, и человек
     * вместо файла увидит страницу с ошибкой. Качаем сами, с `Authorization`.
     */
    data class Attachment(val url: String) : LinkAction

    /** Обычная ссылка — уходит в браузер, как и раньше. */
    data class External(val url: String) : LinkAction

    /** Не http(s) — пусть WebView разбирается сам. */
    object None : LinkAction
}

/** Куда ведёт ссылка [url] со схемой [scheme] при сервере [serverRoot]. */
internal fun linkAction(url: String, scheme: String?, serverRoot: String): LinkAction {
    if (scheme != "http" && scheme != "https") return LinkAction.None
    // Своим считаем только по origin (см. isOwnServerUrl): путь вида
    // `/api/attachments/<uuid>/download` на чужом хосте — чужая ссылка, и
    // заголовок с токеном ей не полагается.
    if (attachmentIdFromHref(url) != null && isOwnServerUrl(url, serverRoot)) return LinkAction.Attachment(url)
    return LinkAction.External(url)
}

/**
 * Скачивает файл по ссылке из текста и отдаёт его системе — как кнопка в
 * «Файлах» задачи, только имя файла берётся из ответа сервера: в самой ссылке
 * его нет, там uuid строки.
 */
private fun saveAttachmentLink(ctx: Context, scope: CoroutineScope, res: Resources, url: String) {
    scope.launch {
        val saved = withContext(Dispatchers.IO) { runCatching { fetchAttachmentFile(ctx, url) }.getOrNull() }
        if (saved == null) {
            Toast.makeText(ctx, res.getString(R.string.task_file_download_failed), Toast.LENGTH_LONG).show()
            return@launch
        }
        openDownloadedFile(ctx, saved.file, saved.mime, res.getString(R.string.task_file_open_chooser, saved.file.name))
    }
}

/** Скачанный файл и тип, которым его объявил сервер. */
private class SavedAttachment(val file: File, val mime: String?)

/** Тянет вложение с Bearer'ом в тот же кэш, куда его кладёт «Файлы» задачи. */
private fun fetchAttachmentFile(ctx: Context, url: String): SavedAttachment? {
    val builder = Request.Builder().url(url).get()
    val token = RetrofitClient.authToken
    if (token.isNotBlank()) builder.header("Authorization", "Bearer $token")
    richHttp.get().newCall(builder.build()).execute().use { resp ->
        if (!resp.isSuccessful) return null
        val fallback = attachmentIdFromHref(url) ?: "file"
        val name = safeDownloadName(filenameFromContentDisposition(resp.header("Content-Disposition")), fallback)
        // Тот же каталог, что у TaskRepository.downloadAttachment: FileProvider
        // раздаёт наружу ровно его (res/xml/file_paths.xml).
        val dir = File(ctx.cacheDir, "attachments").apply { mkdirs() }
        val out = File(dir, name)
        resp.body.byteStream().use { input -> out.outputStream().use { input.copyTo(it) } }
        val mime = resp.header("Content-Type")?.substringBefore(";")?.trim()
        return SavedAttachment(out, mime?.takeIf { it.isNotBlank() })
    }
}

private val richHttp = TlsTrust.Holder()

/** Fetches a backend resource (inline image) with the Bearer token, as a
 *  [WebResourceResponse] for the WebView. Runs on the WebView's IO thread. */
private fun fetchResource(url: String): WebResourceResponse? {
    val builder = Request.Builder().url(url).get()
    val token = RetrofitClient.authToken
    if (token.isNotBlank()) builder.header("Authorization", "Bearer $token")
    val resp = richHttp.get().newCall(builder.build()).execute()
    val body = resp.body
    val contentType = resp.header("Content-Type") ?: "application/octet-stream"
    val mime = contentType.substringBefore(";").trim().ifBlank { "application/octet-stream" }
    val reason = resp.message.ifBlank { "OK" }
    return WebResourceResponse(mime, null, resp.code, reason, emptyMap(), body.byteStream())
}

/**
 * Единственный удалённый скрипт документа — mermaid. Версия прибита точно (не
 * `@11`), а [MERMAID_SRI] — sha384 ровно того файла, который лежит в npm-пакете
 * `mermaid@11.17.2`, то есть в бандле web-клиента: одна и та же диаграмма
 * рисуется одним и тем же кодом на обоих клиентах.
 */
internal const val MERMAID_VERSION = "11.17.2"
internal const val MERMAID_URL = "https://cdn.jsdelivr.net/npm/mermaid@$MERMAID_VERSION/dist/mermaid.min.js"
internal const val MERMAID_SRI = "sha384-EOXBFmc3gx5mb+vn0vPvvGqACToJD24hhacX5Yx+8NUUQrHIle/Qi5Bg9o3zKwW2"

/** Разовый nonce для единственного инлайнового скрипта документа. */
private fun scriptNonce(): String {
    val bytes = ByteArray(16)
    SecureRandom().nextBytes(bytes)
    return bytes.joinToString("") { String.format(Locale.US, "%02x", it) }
}

/**
 * Политика документа (#2932).
 *
 * DOMPurify стоит перед `innerHTML`, но он — единственный рубеж, а за ним
 * включённый JS, мост `AndroidRich` и `shouldInterceptRequest`, подставляющий
 * Bearer. CSP — второй рубеж: и обход санитайзера, и дыра в самом mermaid
 * (его SVG кладётся в `innerHTML` уже без DOMPurify) упираются в то, что
 * выполнить получится только код из APK и один прибитый файл с CDN.
 *
 * Что где:
 * - `script-src` — ассеты APK (`file:`), инлайновый скрипт по nonce и mermaid;
 *   без `'unsafe-inline'`, поэтому `onerror=` и `javascript:` мертвы;
 * - `style-src` — с `'unsafe-inline'`: стили документа инлайновые, и mermaid
 *   кладёт свои внутрь SVG. Nonce тут не ставим намеренно — он бы отменил
 *   `'unsafe-inline'` и диаграммы поехали бы;
 * - `img-src` — широкий: картинку в задачу можно вставить с любого адреса;
 * - `connect-src 'none'` — странице незачем самой ходить в сеть (UMD-сборка
 *   mermaid самодостаточна, чанков не подгружает);
 * - `base-uri`/`form-action`/`frame-src`/`object-src` закрыты: это пути,
 *   которые обходят `script-src`.
 */
private fun contentSecurityPolicy(nonce: String): String = listOf(
    "default-src 'none'",
    "script-src 'nonce-$nonce' file: $MERMAID_URL",
    "style-src 'unsafe-inline' file:",
    "img-src file: data: blob: http: https:",
    "font-src file: data:",
    "connect-src 'none'",
    "base-uri 'none'",
    "form-action 'none'",
    "frame-src 'none'",
    "object-src 'none'",
).joinToString("; ")

private fun hex(c: Color): String =
    String.format(Locale.US, "#%02X%02X%02X", (c.red * 255).roundToInt(), (c.green * 255).roundToInt(), (c.blue * 255).roundToInt())

/** Builds a self-contained HTML document themed to the current Tessera colours. */
internal fun buildRichHtml(
    source: String,
    c: TesseraColors,
    serverRoot: String,
    mentions: List<String>,
    interactive: Boolean,
    mentionCards: Boolean,
    taskRefs: Boolean,
    helpLinks: Boolean,
    helpSlugs: List<String> = emptyList(),
): String {
    val hljsTheme = if (c.isDark) "github-dark" else "github"
    val nonce = scriptNonce()
    val src = JSONObject.quote(source)
    val root = JSONObject.quote(serverRoot)
    val mentionsJson = org.json.JSONArray(mentions).toString()
    val helpSlugsJson = org.json.JSONArray(helpSlugs).toString()
    return """
<!DOCTYPE html><html><head>
<meta http-equiv="Content-Security-Policy" content="${contentSecurityPolicy(nonce)}">
<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
<link rel="stylesheet" href="file:///android_asset/richcontent/hljs-$hljsTheme.min.css">
${richStyles(c, interactive)}
</head><body>
<div id="content"></div>
<script src="file:///android_asset/richcontent/marked.umd.js"></script>
<script src="file:///android_asset/richcontent/purify.min.js"></script>
<script nonce="$nonce">
  var SRC = $src, ROOT = $root, MENTIONS = $mentionsJson, INTERACTIVE = $interactive;
  var MENTION_CARDS = $mentionCards, TASK_REFS = $taskRefs, HELP_LINKS = $helpLinks;
  var HELP_SLUGS = $helpSlugsJson;
  marked.setOptions({breaks:true, gfm:true});
  // Wrap "@Name" tokens for known members in a .mention span (mirrors the web
  // utils/markdown.js highlightMentions: text boundaries only, longer names first).
  // data-handle carries who was written, so a tap can be resolved to the person.
  function escapeRe(s){ return s.replace(/[-.*+?^()|[\]\\{}$]/g, '\\$&'); }
  function attr(s){ return s.replace(/"/g, '&quot;'); }
  // A handle needs a right-hand boundary too, or the longest-first scan chips a
  // member found inside someone else's handle: "@ivanov" would draw "@ivan"
  // stamped as that member (web utils/markdown.js handleEndsAt). '\w' is ASCII
  // here while the names are usually not, so the letter test is written as
  // "changes under case folding". A dot ends a handle only when no word follows,
  // keeping a mention that closes a sentence a chip.
  function wordChar(c){
    if (!c) return false;
    if ((c >= '0' && c <= '9') || c === '_' || c === '-') return true;
    return c.toLowerCase() !== c.toUpperCase();
  }
  function handleEndsAt(s, i){
    var c = s.charAt(i);
    if (wordChar(c)) return false;
    return !(c === '.' && wordChar(s.charAt(i + 1)));
  }
  function highlightMentions(html){
    var labels = MENTIONS.filter(Boolean).sort(function(a,b){ return b.length - a.length; }).map(escapeRe);
    if (!labels.length) return html;
    var re = new RegExp('(^|[\\s>(])@(' + labels.join('|') + ')(?![\\w-])(?!\\.[\\w-])', 'g');
    return html.replace(re, function(whole, lead, handle, offset){
      if (!handleEndsAt(html, offset + whole.length)) return whole;
      return lead + '<span class="mention" data-handle="' + attr(handle) + '">@' + handle + '</span>';
    });
  }
  // "#123" → a chip the app resolves and opens. Capped at 7 digits so a long
  // numeric string doesn't become a link (web linkTaskRefs).
  function linkTaskRefs(html){
    return html.replace(/(^|[\s>([])#(\d{1,7})\b(?!\.\d)/g, function(_, lead, n){
      return lead + '<a class="task-ref" data-task-ref="' + n + '" href="#">#' + n + '</a>';
    });
  }
  // Both decorations rewrite already-rendered HTML, so `fn` must only ever see
  // the DISPLAYED TEXT — everything that is neither a code sample nor markup
  // (port of the web replaceOutsideMarkup).
  //
  //   • <code>/<pre> blocks, because their contents are a sample and not prose:
  //     "#2550" in a diff is not a task link and "@root" in a shell snippet is
  //     not a mention.
  //   • tags themselves, because an attribute is not text. marked writes an
  //     image's alt and a link's title into one, and a decoration splices an
  //     element in place of what it matched - so an image captioned "#123" put an
  //     <a ...> inside its own alt="...", which ends at the chip's first quote:
  //     the image lost its caption, grew junk attributes, and the tail of the
  //     chip was drawn as visible text. A link title took the anchor's own text
  //     down with it (#2932).
  function replaceOutsideMarkup(html, fn){
    var re = /<(code|pre)\b[^>]*>[\s\S]*?<\/\1>|<[^>]*>/gi, out = '', last = 0, m;
    while ((m = re.exec(html)) !== null) {
      out += fn(html.slice(last, m.index)) + m[0];
      last = m.index + m[0].length;
    }
    return out + fn(html.slice(last));
  }
  // Order mirrors the web utils/markdown.js: decorate first, sanitise the whole
  // thing afterwards, so the chips we inject are inspected by the same pass.
  // ADD_ATTR lists what the decorations and marked emit themselves and DOMPurify
  // would otherwise drop along with the markup: without target/rel links lose
  // their safe opening, without class the mention and task-ref chips lose theirs.
  var SANITIZE_OPTS = {ADD_ATTR: ['target', 'rel', 'class', 'data-handle', 'data-task-ref']};
  var el = document.getElementById('content');
  el.innerHTML = DOMPurify.sanitize(replaceOutsideMarkup(marked.parse(SRC || ''), function(part){
    var withMentions = highlightMentions(part);
    return TASK_REFS ? linkTaskRefs(withMentions) : withMentions;
  }), SANITIZE_OPTS);
  // Expand root-relative upload/asset URLs to the active server (inline images
  // are intercepted + fetched with auth; links open in the browser).
  el.querySelectorAll('img').forEach(function(im){
    var s = im.getAttribute('src')||'';
    if (s.charAt(0) === '/') im.src = ROOT + s;
  });
  // Help articles cross-link as "/help/<slug>". Claim those before the
  // root-relative rewrite below, or they would become site URLs and a tap would
  // leave the app for a browser. A slug this build does not bundle is left
  // unclaimed on purpose: the app has no page to open, so the rewrite below
  // turns it into the site's own manual rather than a link that does nothing.
  if (HELP_LINKS) {
    el.querySelectorAll('a').forEach(function(an){
      var h = an.getAttribute('href')||'';
      if (h.indexOf('/help/') !== 0) return;
      var slug = h.slice(6).split('#')[0];
      if (HELP_SLUGS.indexOf(slug) < 0) return;
      an.setAttribute('data-help-slug', slug);
      an.setAttribute('href', '#');
    });
  }
  el.querySelectorAll('a').forEach(function(an){
    var h = an.getAttribute('href')||'';
    if (h.charAt(0) === '/') an.href = ROOT + h;
  });
  // Interactive GFM checkboxes: enable them, and delegate clicks so a tap on the
  // checkbox OR on the item's text toggles it. The native checkbox toggle animates
  // (no preventDefault); a text tap flips it manually so it animates too. Kotlin
  // skips the reload the markdown rewrite triggers (the DOM already matches it).
  var allBoxes = [];
  if (INTERACTIVE) {
    allBoxes = [].slice.call(el.querySelectorAll('input[type=checkbox]'));
    allBoxes.forEach(function(box){ box.disabled = false; });
  }
  if (INTERACTIVE || MENTION_CARDS || TASK_REFS || HELP_LINKS) {
    el.addEventListener('click', function(e){
      var t = e.target;
      if (HELP_LINKS && t.closest) {
        var help = t.closest('[data-help-slug]');
        if (help) {
          e.preventDefault(); // href="#" would reload the document
          if (window.AndroidRich) AndroidRich.onHelpLink(help.getAttribute('data-help-slug'));
          return;
        }
      }
      // Mention chips and "#N" links come first — they work whether or not the
      // content is interactive. A mention highlighted inside a code sample is
      // not a person, so it gets no card.
      if (MENTION_CARDS && t.closest) {
        var chip = t.closest('.mention');
        if (chip && !chip.closest('code,pre')) {
          var r = chip.getBoundingClientRect();
          var handle = (chip.getAttribute('data-handle') || chip.textContent || '').replace(/^@/, '');
          if (window.AndroidRich) AndroidRich.onMention(handle, Math.round(r.left), Math.round(r.bottom));
          return;
        }
      }
      if (TASK_REFS && t.closest) {
        var ref = t.closest('[data-task-ref]');
        if (ref) {
          e.preventDefault(); // href="#" would reload the document
          if (window.AndroidRich) AndroidRich.onTaskRef(parseInt(ref.getAttribute('data-task-ref'), 10));
          return;
        }
      }
      if (!INTERACTIVE) return;
      var box = null;
      if (t.tagName === 'INPUT' && t.type === 'checkbox') {
        box = t;
      } else {
        if (t.closest('a,code,pre,.mention')) return;
        var li = t.closest('li');
        box = li && li.querySelector(':scope > input[type=checkbox]');
        if (box) box.checked = !box.checked;
      }
      if (!box) return;
      var i = allBoxes.indexOf(box);
      if (i >= 0 && window.AndroidRich) AndroidRich.onCheckToggle(i);
    });
  }
  // The height has to track the document continuously, not once at the end of
  // this script: the hljs stylesheet arrives from a CDN, font metrics settle
  // late, a width change (rotation, the modal shrinking under the keyboard)
  // reflows everything, and ticking a checkbox rewrites content without a
  // reload. Any of those leaves the reported height short — and a short WebView
  // scrolls its own content instead of the modal (#2781).
  // Measured off #content (display:flow-root, so child margins count) rather
  // than body/documentElement: those never report less than the viewport, so a
  // document that got shorter could never shrink back.
  var lastH = -1, frame = 0;
  function push(){
    frame = 0;
    var h = Math.ceil(Math.max(el.scrollHeight, el.getBoundingClientRect().height)) + 4;
    if (h !== lastH && window.AndroidRich) { lastH = h; AndroidRich.onHeight(h); }
  }
  function report(){ if (!frame) frame = requestAnimationFrame(push); }
  if (window.ResizeObserver) new ResizeObserver(report).observe(el);
  // Width changes (rotation, keyboard) reflow without changing #content's box
  // height in every case — measure again regardless.
  window.addEventListener('resize', report);
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(report);
  // The hljs theme is a separate <link>: it restyles `pre` once it lands.
  var themeCss = document.querySelector('link[rel=stylesheet]');
  if (themeCss) { themeCss.addEventListener('load', report); themeCss.addEventListener('error', report); }
  // Image loads change height — re-report once they settle.
  el.querySelectorAll('img').forEach(function(im){ im.addEventListener('load', report); });
  var mer = el.querySelectorAll('code.language-mermaid');
  function highlightCode(){
    el.querySelectorAll('pre code').forEach(function(code){
      if (!code.classList.contains('language-mermaid') && window.hljs) hljs.highlightElement(code);
    });
  }
  // The 122 KB highlighter is pulled in only when the text actually has code —
  // the vast majority of descriptions and comments are plain prose.
  function withHljs(cb){
    if (window.hljs) return cb();
    if (!el.querySelector('pre code:not(.language-mermaid)')) return cb();
    var s = document.createElement('script');
    s.src = 'file:///android_asset/richcontent/highlight.min.js';
    s.onload = cb; s.onerror = cb; document.head.appendChild(s);
  }
  if (mer.length) {
    var m = document.createElement('script');
    m.src = '$MERMAID_URL';
    // Pinned version + SRI: this is the only code in the document that does not
    // come from the APK, and it is untrusted task text that asks for it (a
    // ```mermaid block). A floating `@11` meant "run whatever the CDN serves
    // today"; `integrity` checks the bytes instead, so a swapped mermaid — at
    // the CDN or on the way — simply does not execute. The hash is the one of
    // the npm package the web client bundles.
    m.integrity = '$MERMAID_SRI';
    m.crossOrigin = 'anonymous';
    m.onload = function(){
      mermaid.initialize({startOnLoad:false, securityLevel:'strict', theme: ${if (c.isDark) "'dark'" else "'default'"}});
      var i = 0, jobs = [];
      mer.forEach(function(code){
        var pre = code.closest('pre') || code;
        var id = 'mmd-' + (i++);
        jobs.push(mermaid.render(id, code.textContent || '').then(function(r){
          var w = document.createElement('div'); w.className = 'mermaid-diagram'; w.innerHTML = r.svg; pre.replaceWith(w);
        }).catch(function(){ pre.style.borderLeft = '3px solid #e0533d'; }));
      });
      Promise.all(jobs).then(function(){ withHljs(function(){ highlightCode(); report(); }); });
    };
    m.onerror = function(){ withHljs(function(){ highlightCode(); report(); }); };
    document.head.appendChild(m);
  } else {
    withHljs(function(){ highlightCode(); report(); });
  }
  report();
</script>
</body></html>
"""
}

/**
 * Стили документа вынесены отдельно, чтобы buildRichHtml оставался в пределах
 * detekt-лимита на длину функции: CSS занимал в нём больше трети строк.
 */
private fun richStyles(c: TesseraColors, interactive: Boolean): String {
    // Accent gradient colours for links and mentions (mirrors AccentGradient.kt)
    val strength = 0.14f
    val accentDarker = lerp(c.primary, Color.Black, strength)
    val accentLighter = lerp(c.primary, Color.White, strength)
    val accentBoxGrad = "linear-gradient(135deg,${hex(accentDarker)},${hex(c.primary)},${hex(accentLighter)})"
    val accentGradCss = "background:$accentBoxGrad;" +
        "-webkit-background-clip:text;-webkit-text-fill-color:transparent;background-clip:text;"
    val checkCursor = if (interactive) "pointer" else "default"
    // Muted box border (not the bright text3) so checkboxes read softly on dark.
    val checkBorder = hex(lerp(c.text3, c.border, 0.55f))
    return """
<style>
  html,body{margin:0;padding:0;background:transparent;}
  body{color:${hex(c.text1)};font-family:-apple-system,Roboto,sans-serif;font-size:14px;line-height:1.55;
       word-wrap:break-word;overflow-wrap:anywhere;}
  /* flow-root so child margins stay inside the box we measure for the height. */
  #content{padding:0;display:flow-root;}
  a{text-decoration:none;$accentGradCss}
  p{margin:0 0 8px;} p:last-child{margin-bottom:0;}
  h1,h2,h3,h4{margin:12px 0 6px;line-height:1.3;font-weight:600;}
  h1{font-size:20px;} h2{font-size:18px;} h3{font-size:16px;} h4{font-size:14px;}
  ul,ol{margin:0 0 8px;padding-left:20px;} li{margin:2px 0;}
  li:has(> input[type=checkbox]){list-style:none;}
  input[type=checkbox]{appearance:none;-webkit-appearance:none;width:15px;height:15px;margin:0 7px 0 0;
       vertical-align:-2px;border:1.5px solid $checkBorder;border-radius:4px;background:${hex(c.surface)};
       position:relative;flex:none;cursor:$checkCursor;
       transition:border-color .15s ease,background-color .15s ease;}
  input[type=checkbox]:checked{border-color:transparent;background-color:transparent;
       background-image:$accentBoxGrad;background-origin:border-box;}
  /* Checkmark always present; pops in/out (scale+fade) — the light tick animation. */
  input[type=checkbox]::after{content:'';position:absolute;left:50%;top:50%;width:4px;height:8px;
       border:solid ${hex(c.onPrimary)};border-width:0 2px 2px 0;
       transform:translate(-50%,-60%) rotate(45deg) scale(.4);opacity:0;
       transition:opacity .12s ease,transform .18s cubic-bezier(.2,.7,.3,1.55);}
  input[type=checkbox]:checked::after{opacity:1;transform:translate(-50%,-60%) rotate(45deg) scale(1);}
  @media (prefers-reduced-motion:reduce){input[type=checkbox],input[type=checkbox]::after{transition:none;}}
  img{max-width:100%;height:auto;border-radius:8px;}
  blockquote{margin:8px 0;padding:2px 12px;border-left:3px solid ${hex(c.border)};color:${hex(c.text2)};}
  hr{border:none;border-top:1px solid ${hex(c.border)};margin:12px 0;}
  code{font-family:ui-monospace,Menlo,monospace;font-size:12.5px;background:${hex(c.surfaceAlt)};
       padding:1px 5px;border-radius:5px;}
  pre{margin:8px 0;padding:10px 12px;background:${hex(c.surfaceAlt)};border:1px solid ${hex(c.border)};
      border-radius:8px;overflow-x:auto;}
  pre code{background:transparent;padding:0;font-size:12.5px;}
  table{border-collapse:collapse;margin:8px 0;} th,td{border:1px solid ${hex(c.border)};padding:4px 8px;}
  .mention{font-weight:600;$accentGradCss}
  .task-ref{font-weight:600;cursor:pointer;$accentGradCss}
  .mermaid-diagram{display:flex;justify-content:center;margin:10px 0;}
  .mermaid-diagram svg{max-width:100%;height:auto;}
</style>
"""
}
