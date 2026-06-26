package website.msdnna.tessera.ui.components

import android.annotation.SuppressLint
import android.content.Intent
import android.graphics.Color as AColor
import android.net.Uri
import android.webkit.JavascriptInterface
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import java.util.Locale
import kotlin.math.roundToInt
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraColors

/**
 * Renders stored Markdown exactly like the web `RichContent.vue`: marked → HTML,
 * highlight.js for fenced code, and lazily-loaded mermaid for ```mermaid blocks.
 * A WebView is the faithful (and only sane) way to reproduce mermaid diagrams +
 * syntax highlighting; everything around it (tabs, editor, controls) is native.
 *
 * marked is bundled (assets/richcontent); highlight.js + mermaid load from a CDN
 * on demand (online-first app), so diagrams need connectivity — basic Markdown
 * still renders offline. The view reports its content height back so it sizes to
 * its content inside the scrolling modal.
 */
@SuppressLint("SetJavaScriptEnabled")
@Composable
fun RichContent(
    source: String,
    modifier: Modifier = Modifier,
    mentions: List<String> = emptyList(),
    // When true, GFM task checkboxes become clickable; a click reports the box
    // index via [onToggleCheck] so the caller can rewrite the stored markdown.
    interactive: Boolean = false,
    onToggleCheck: ((Int) -> Unit)? = null,
) {
    val c = Tessera.colors
    val ctx = LocalContext.current
    val serverRoot = RetrofitClient.serverRoot
    var heightDp by remember { mutableStateOf(1) }
    // Latest callback, so the long-lived JS bridge always calls the current one.
    val toggleCb by rememberUpdatedState(onToggleCheck)

    // Rebuild the document whenever the source, theme, mentions or mode changes.
    val html = remember(source, c.isDark, mentions, interactive) {
        buildRichHtml(source, c, serverRoot, mentions, interactive)
    }

    AndroidView(
        modifier = modifier.fillMaxWidth().height(heightDp.dp),
        factory = {
            WebView(it).apply {
                setBackgroundColor(AColor.TRANSPARENT)
                isVerticalScrollBarEnabled = false
                isHorizontalScrollBarEnabled = false
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
                            post { toggleCb?.invoke(index) }
                        }
                    },
                    "AndroidRich",
                )
                webViewClient = object : WebViewClient() {
                    override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                        val url = request.url
                        if (url.scheme == "http" || url.scheme == "https") {
                            runCatching { ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url.toString()))) }
                            return true
                        }
                        return false
                    }

                    // Inline images live on our backend (`/api/uploads/…`). The
                    // WebView's own network stack can't load them (its TLS/policy
                    // differs from the app's working OkHttp — and any future auth
                    // wouldn't be sent), so fetch our-server resources through the
                    // same OkHttp path Coil uses and hand the bytes back.
                    override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse? {
                        val url = request.url.toString()
                        val root = RetrofitClient.serverRoot
                        if (root.isBlank() || !url.startsWith(root) || request.method != "GET") return null
                        return runCatching { fetchResource(url) }.getOrNull()
                    }
                }
            }
        },
        update = { web ->
            web.loadDataWithBaseURL("file:///android_asset/richcontent/", html, "text/html", "utf-8", null)
        },
    )
}

private val richHttp by lazy { OkHttpClient() }

/** Fetches a backend resource (inline image) with the Bearer token, as a
 *  [WebResourceResponse] for the WebView. Runs on the WebView's IO thread. */
private fun fetchResource(url: String): WebResourceResponse? {
    val builder = Request.Builder().url(url).get()
    val token = RetrofitClient.authToken
    if (token.isNotBlank()) builder.header("Authorization", "Bearer $token")
    val resp = richHttp.newCall(builder.build()).execute()
    val body = resp.body
    val contentType = resp.header("Content-Type") ?: "application/octet-stream"
    val mime = contentType.substringBefore(";").trim().ifBlank { "application/octet-stream" }
    val reason = resp.message.ifBlank { "OK" }
    return WebResourceResponse(mime, null, resp.code, reason, emptyMap(), body.byteStream())
}

private fun hex(c: Color): String =
    String.format(Locale.US, "#%02X%02X%02X", (c.red * 255).roundToInt(), (c.green * 255).roundToInt(), (c.blue * 255).roundToInt())

/** Builds a self-contained HTML document themed to the current Tessera colours. */
private fun buildRichHtml(
    source: String,
    c: TesseraColors,
    serverRoot: String,
    mentions: List<String>,
    interactive: Boolean,
): String {
    val hljsTheme = if (c.isDark) "github-dark" else "github"
    val src = JSONObject.quote(source)
    val root = JSONObject.quote(serverRoot)
    val mentionsJson = org.json.JSONArray(mentions).toString()
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
<!DOCTYPE html><html><head>
<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
<link rel="stylesheet" href="https://cdn.jsdelivr.net/gh/highlightjs/cdn-release@11.9.0/build/styles/$hljsTheme.min.css">
<style>
  html,body{margin:0;padding:0;background:transparent;}
  body{color:${hex(c.text1)};font-family:-apple-system,Roboto,sans-serif;font-size:14px;line-height:1.55;
       word-wrap:break-word;overflow-wrap:anywhere;}
  #content{padding:0;}
  a{text-decoration:none;$accentGradCss}
  p{margin:0 0 8px;} p:last-child{margin-bottom:0;}
  h1,h2,h3,h4{margin:12px 0 6px;line-height:1.3;font-weight:600;}
  h1{font-size:20px;} h2{font-size:18px;} h3{font-size:16px;} h4{font-size:14px;}
  ul,ol{margin:0 0 8px;padding-left:20px;} li{margin:2px 0;}
  li:has(> input[type=checkbox]){list-style:none;}
  input[type=checkbox]{appearance:none;-webkit-appearance:none;width:15px;height:15px;margin:0 7px 0 0;
       vertical-align:-2px;border:1.5px solid $checkBorder;border-radius:4px;background:${hex(c.surface)};
       position:relative;flex:none;cursor:$checkCursor;}
  input[type=checkbox]:checked{border-color:transparent;background:$accentBoxGrad;}
  input[type=checkbox]:checked::after{content:'';position:absolute;left:50%;top:50%;width:4px;height:8px;
       border:solid ${hex(c.onPrimary)};border-width:0 2px 2px 0;transform:translate(-50%,-55%) rotate(45deg);}
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
  .mermaid-diagram{display:flex;justify-content:center;margin:10px 0;}
  .mermaid-diagram svg{max-width:100%;height:auto;}
</style>
</head><body>
<div id="content"></div>
<script src="file:///android_asset/richcontent/marked.umd.js"></script>
<script>
  var SRC = $src, ROOT = $root, MENTIONS = $mentionsJson, INTERACTIVE = $interactive;
  marked.setOptions({breaks:true, gfm:true});
  // Wrap "@Name" tokens for known members in a .mention span (mirrors the web
  // utils/markdown.js highlightMentions: text boundaries only, longer names first).
  function escapeRe(s){ return s.replace(/[-.*+?^()|[\]\\{}$]/g, '\\$&'); }
  function highlightMentions(html){
    var labels = MENTIONS.filter(Boolean).sort(function(a,b){ return b.length - a.length; }).map(escapeRe);
    if (!labels.length) return html;
    var re = new RegExp('(^|[\\s>(])@(' + labels.join('|') + ')', 'g');
    return html.replace(re, '$1<span class="mention">@$2</span>');
  }
  var el = document.getElementById('content');
  el.innerHTML = highlightMentions(marked.parse(SRC || ''));
  // Expand root-relative upload/asset URLs to the active server (inline images
  // are intercepted + fetched with auth; links open in the browser).
  el.querySelectorAll('img').forEach(function(im){
    var s = im.getAttribute('src')||'';
    if (s.charAt(0) === '/') im.src = ROOT + s;
  });
  el.querySelectorAll('a').forEach(function(an){
    var h = an.getAttribute('href')||'';
    if (h.charAt(0) === '/') an.href = ROOT + h;
  });
  // Interactive GFM checkboxes: enable + report the box index on click so Kotlin
  // can rewrite the markdown marker (the re-render reflects the new state).
  if (INTERACTIVE) {
    el.querySelectorAll('input[type=checkbox]').forEach(function(box, i){
      box.disabled = false;
      box.addEventListener('click', function(e){
        e.preventDefault();
        if (window.AndroidRich) AndroidRich.onCheckToggle(i);
      });
    });
  }
  function report(){ if (window.AndroidRich) AndroidRich.onHeight(document.body.scrollHeight + 4); }
  // Image loads change height — re-report once they settle.
  el.querySelectorAll('img').forEach(function(im){ im.addEventListener('load', report); });
  var mer = el.querySelectorAll('code.language-mermaid');
  function highlightCode(){
    el.querySelectorAll('pre code').forEach(function(code){
      if (!code.classList.contains('language-mermaid') && window.hljs) hljs.highlightElement(code);
    });
  }
  function withHljs(cb){
    if (window.hljs) return cb();
    var s = document.createElement('script');
    s.src = 'https://cdn.jsdelivr.net/gh/highlightjs/cdn-release@11.9.0/build/highlight.min.js';
    s.onload = cb; s.onerror = cb; document.head.appendChild(s);
  }
  if (mer.length) {
    var m = document.createElement('script');
    m.src = 'https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js';
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
