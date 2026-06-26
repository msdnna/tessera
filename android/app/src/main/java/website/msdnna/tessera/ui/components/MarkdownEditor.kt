package website.msdnna.tessera.ui.components

import android.net.Uri
import android.provider.OpenableColumns
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.selection.LocalTextSelectionColors
import androidx.compose.foundation.text.selection.TextSelectionColors
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import website.msdnna.tessera.ui.theme.AccentGradientStrengthSubtle
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.toggleTaskMarker

/** Snippet inserted by the mermaid toolbar button (matches the web editor). */
private const val MERMAID_SNIPPET = "\n```mermaid\ngraph TD\n  A[Старт] --> B[Готово]\n```\n"

/** "@query" right before the caret (mirrors the web editor's mention trigger). */
private val MENTION_RE = Regex("(^|\\s)@([^\\s@]*)$")

/**
 * The web `MarkdownEditor`, native: a Написать / Просмотр tab pair, a borderless
 * textarea storing Markdown, a formatting toolbar (bold / italic / strike /
 * code / list / quote / heading) that wraps the current selection, plus buttons
 * to upload an inline image and to insert a mermaid block. Preview renders
 * through [RichContent]. Edits save on focus loss via [onBlur].
 */
@Composable
fun MarkdownEditor(
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    modifier: Modifier = Modifier,
    minHeight: Dp = 84.dp,
    startInPreview: Boolean = false,
    onBlur: (() -> Unit)? = null,
    uploadImage: (suspend (ByteArray, String, String?) -> String?)? = null,
    // Workspace member names for @-mentions; empty → mentions off.
    mentions: List<String> = emptyList(),
) {
    val c = Tessera.colors
    var preview by remember { mutableStateOf(startInPreview) }
    var uploading by remember { mutableStateOf(false) }
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()

    // Internal selection-aware state; resynced when [value] changes externally
    // (e.g. an image insert appends to the parent's string).
    var tfv by remember { mutableStateOf(TextFieldValue(value)) }
    if (tfv.text != value) tfv = TextFieldValue(value)
    fun update(next: TextFieldValue) {
        tfv = next
        if (next.text != value) onValueChange(next.text)
    }

    val picker = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri: Uri? ->
        if (uri == null || uploadImage == null) return@rememberLauncherForActivityResult
        uploading = true
        scope.launch {
            val url = runCatching {
                val (bytes, name) = withContext(Dispatchers.IO) {
                    val b = ctx.contentResolver.openInputStream(uri)?.use { it.readBytes() } ?: ByteArray(0)
                    b to (displayName(ctx, uri) ?: "image")
                }
                val mime = ctx.contentResolver.getType(uri)
                if (bytes.isEmpty()) null else uploadImage(bytes, name, mime)
            }.getOrNull()
            uploading = false
            if (!url.isNullOrBlank()) {
                val sep = if (value.isEmpty() || value.endsWith("\n")) "" else "\n"
                onValueChange(value + sep + "![]($url)\n")
            }
        }
    }

    Column(modifier.fillMaxWidth()) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            EditorTab("Написать", active = !preview) { preview = false }
            Spacer(Modifier.width(16.dp))
            EditorTab("Просмотр", active = preview) { preview = true }
            Spacer(Modifier.weight(1f))
            if (!preview) {
                if (uploadImage != null) {
                    IonIconButton(
                        Ion.IMAGE,
                        onClick = { if (!uploading) picker.launch("image/*") },
                        boxSize = 30.dp,
                        iconSize = 17.dp,
                        tint = if (uploading) c.text3 else c.text2,
                    )
                    Spacer(Modifier.width(2.dp))
                }
                IonIconButton(
                    Ion.BRANCH,
                    onClick = { update(insert(tfv, MERMAID_SNIPPET)) },
                    boxSize = 30.dp,
                    iconSize = 17.dp,
                    tint = c.text2,
                )
            }
        }

        if (!preview) {
            Spacer(Modifier.height(6.dp))
            FormatToolbar(onApply = { update(it(tfv)) })
        }
        Spacer(Modifier.height(6.dp))

        if (preview) {
            Box(Modifier.fillMaxWidth().heightIn(min = minHeight).padding(vertical = 4.dp)) {
                if (value.isBlank()) {
                    Text(placeholder, color = c.placeholder, fontSize = 14.sp)
                } else {
                    // Ticking a checkbox in preview rewrites the markdown marker and
                    // persists via onBlur (the description/comment save path).
                    RichContent(
                        value,
                        mentions = mentions,
                        interactive = true,
                        onToggleCheck = { i ->
                            onValueChange(toggleTaskMarker(value, i))
                            onBlur?.invoke()
                        },
                    )
                }
            }
        } else {
            var hadFocus by remember { mutableStateOf(false) }
            // @-mention autocomplete: match "@query" right before the caret.
            val caret = tfv.selection.min
            val mq = if (mentions.isNotEmpty()) MENTION_RE.find(tfv.text.substring(0, caret)) else null
            val query = mq?.groupValues?.get(2)
            val matches = if (query != null) {
                mentions.filter { it.contains(query, ignoreCase = true) }.take(8)
            } else {
                emptyList()
            }
            fun pickMention(name: String) {
                val start = caret - (query?.length ?: 0) - 1
                val ins = "@$name "
                val text = tfv.text.substring(0, start) + ins + tfv.text.substring(caret)
                update(TextFieldValue(text, androidx.compose.ui.text.TextRange(start + ins.length)))
            }
            val selectionColors = TextSelectionColors(c.primary, c.primary.copy(alpha = 0.3f))
            CompositionLocalProvider(LocalTextSelectionColors provides selectionColors) {
                BasicTextField(
                    value = tfv,
                    onValueChange = ::update,
                    // Monospace so indentation (nested lists) lines up while editing;
                    // the field auto-grows to its content (no fixed height/scroll).
                    textStyle = TextStyle(color = c.text1, fontSize = 13.sp, lineHeight = 20.sp, fontFamily = FontFamily.Monospace),
                    cursorBrush = SolidColor(c.primary),
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(min = minHeight)
                        .clip(RoundedCornerShape(RadiusMd))
                        .background(c.surface)
                        .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
                        .padding(12.dp)
                        .onFocusChanged { if (it.isFocused) hadFocus = true else if (hadFocus) onBlur?.invoke() },
                    decorationBox = { inner ->
                        if (tfv.text.isEmpty()) Text(placeholder, color = c.placeholder, fontSize = 14.sp)
                        inner()
                    },
                )
            }
            if (matches.isNotEmpty()) MentionSuggestions(matches, onPick = ::pickMention)
        }
    }
}

/** The formatting buttons. Each applies a transform to the current value. */
@Composable
private fun FormatToolbar(onApply: ((TextFieldValue) -> TextFieldValue) -> Unit) {
    Row(
        Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        FmtButton({ Text("B", fontWeight = FontWeight.Bold, fontSize = 14.sp, color = it) }) { onApply { v -> wrap(v, "**") } }
        FmtButton({ Text("I", fontStyle = FontStyle.Italic, fontSize = 14.sp, color = it) }) { onApply { v -> wrap(v, "*") } }
        FmtButton({ Text("S", textDecoration = TextDecoration.LineThrough, fontSize = 14.sp, color = it) }) { onApply { v -> wrap(v, "~~") } }
        FmtButton({ Text("</>", fontFamily = FontFamily.Monospace, fontSize = 12.sp, color = it) }) { onApply { v -> wrap(v, "`") } }
        FmtButton({ Text("H", fontWeight = FontWeight.SemiBold, fontSize = 14.sp, color = it) }) { onApply { v -> linePrefix(v, "## ") } }
        FmtButton({ Text("•", fontSize = 16.sp, color = it) }) { onApply { v -> linePrefix(v, "- ") } }
        FmtButton({ Text("❝", fontSize = 14.sp, color = it) }) { onApply { v -> linePrefix(v, "> ") } }
        FmtButton({ IonIcon(Ion.LINK, size = 15.dp, tint = it) }) { onApply { v -> link(v) } }
    }
}

@Composable
private fun FmtButton(content: @Composable (androidx.compose.ui.graphics.Color) -> Unit, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier
            .padding(end = 6.dp)
            .heightIn(min = 28.dp)
            .width(34.dp)
            .clip(RoundedCornerShape(RadiusSm))
            .background(c.surfaceAlt)
            .clickableNoRipple(onClick = onClick)
            .padding(vertical = 5.dp),
        contentAlignment = Alignment.Center,
    ) { content(c.text2) }
}

/** @-mention suggestion list shown under the textarea while typing "@…". */
@Composable
private fun MentionSuggestions(names: List<String>, onPick: (String) -> Unit) {
    val c = Tessera.colors
    Column(
        Modifier
            .fillMaxWidth()
            .padding(top = 4.dp)
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.surfaceAlt)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd)),
    ) {
        names.forEach { name ->
            Text(
                "@$name",
                color = c.text1,
                fontSize = 14.sp,
                modifier = Modifier
                    .fillMaxWidth()
                    .clickableNoRipple { onPick(name) }
                    .padding(horizontal = 12.dp, vertical = 9.dp),
            )
        }
    }
}

@Composable
private fun EditorTab(label: String, active: Boolean, onClick: () -> Unit) {
    val c = Tessera.colors
    // IntrinsicSize.Max sizes the column to the label width, so the fillMaxWidth
    // underline spans exactly the text (not the whole row, and not zero).
    Column(Modifier.width(IntrinsicSize.Max).clickableNoRipple(onClick = onClick)) {
        Text(
            label,
            color = if (active) c.primary else c.text2,
            fontSize = 13.sp,
            fontWeight = if (active) FontWeight.SemiBold else FontWeight.Medium,
            style = if (active) TextStyle(brush = accentGradient(c.primary, AccentGradientStrengthSubtle)) else TextStyle.Default,
            modifier = Modifier.padding(vertical = 4.dp),
        )
        Box(
            Modifier
                .fillMaxWidth()
                .height(2.dp)
                .then(
                    if (active) {
                        Modifier.background(accentGradient(c.primary, AccentGradientStrengthSubtle))
                    } else {
                        Modifier
                    },
                ),
        )
    }
}

// ── Markdown text transforms (selection-aware) ───────────────────────────────

/** Wraps the selection with [token] on both sides; empty selection drops the
 *  caret between the tokens. */
private fun wrap(v: TextFieldValue, token: String): TextFieldValue = wrap(v, token, token)

private fun wrap(v: TextFieldValue, pre: String, post: String): TextFieldValue {
    val start = v.selection.min
    val end = v.selection.max
    val t = v.text
    val selected = t.substring(start, end)
    val text = t.substring(0, start) + pre + selected + post + t.substring(end)
    val sel = if (selected.isEmpty()) {
        androidx.compose.ui.text.TextRange(start + pre.length)
    } else {
        androidx.compose.ui.text.TextRange(start + pre.length, end + pre.length)
    }
    return TextFieldValue(text, sel)
}

/** Prepends [prefix] to the start of the caret's line. */
private fun linePrefix(v: TextFieldValue, prefix: String): TextFieldValue {
    val t = v.text
    val caret = v.selection.min
    val lineStart = t.lastIndexOf('\n', (caret - 1).coerceAtLeast(0)).let { if (it < 0) 0 else it + 1 }
    val text = t.substring(0, lineStart) + prefix + t.substring(lineStart)
    return TextFieldValue(text, androidx.compose.ui.text.TextRange(v.selection.max + prefix.length))
}

/** Turns the selection (or empty caret) into `[text](url)`, selecting `url`. */
private fun link(v: TextFieldValue): TextFieldValue {
    val start = v.selection.min
    val end = v.selection.max
    val t = v.text
    val label = t.substring(start, end).ifEmpty { "текст" }
    val urlAt = start + 1 + label.length + 2 // after "[" + label + "]("
    val text = t.substring(0, start) + "[" + label + "](url)" + t.substring(end)
    return TextFieldValue(text, androidx.compose.ui.text.TextRange(urlAt, urlAt + 3))
}

/** Inserts [snippet] at the caret (replacing any selection). */
private fun insert(v: TextFieldValue, snippet: String): TextFieldValue {
    val start = v.selection.min
    val end = v.selection.max
    val text = v.text.substring(0, start) + snippet + v.text.substring(end)
    return TextFieldValue(text, androidx.compose.ui.text.TextRange(start + snippet.length))
}

private fun displayName(ctx: android.content.Context, uri: Uri): String? = runCatching {
    ctx.contentResolver.query(uri, null, null, null, null)?.use { cursor ->
        val idx = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
        if (idx >= 0 && cursor.moveToFirst()) cursor.getString(idx) else null
    }
}.getOrNull()
