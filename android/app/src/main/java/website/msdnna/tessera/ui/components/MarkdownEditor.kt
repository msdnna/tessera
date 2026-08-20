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
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.relocation.BringIntoViewRequester
import androidx.compose.foundation.relocation.bringIntoViewRequester
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.selection.LocalTextSelectionColors
import androidx.compose.foundation.text.selection.TextSelectionColors
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import kotlin.math.roundToInt
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.CommandItem
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.MdEdit
import website.msdnna.tessera.util.MentionItem
import website.msdnna.tessera.util.applyTyping
import website.msdnna.tessera.util.commandInsertText
import website.msdnna.tessera.util.detectSlashQuery
import website.msdnna.tessera.util.indentLines
import website.msdnna.tessera.util.linePrefixLines
import website.msdnna.tessera.util.matchCommands
import website.msdnna.tessera.util.orderedListPrefix
import website.msdnna.tessera.util.outdentLines
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
    // @-mention candidates (Tessera members + GitLab users); empty → mentions off.
    mentions: List<MentionItem> = emptyList(),
    // Quick-action rows for the `/`-popup; empty → commands off. Only passed where
    // commands actually run (the new-comment composer) — see the web editor.
    commands: List<CommandItem> = emptyList(),
    // Opt-in: "#123" in the preview becomes a link, reported here on tap (the
    // web editor previews with task-refs on too).
    onTaskRef: ((Int) -> Unit)? = null,
    /** e2e anchor placed on the text area itself — `modifier` lands on the column
     *  that also holds the toolbar and the preview, which carries no text-input
     *  semantics to type into. Null on the editors no spec drives. */
    fieldTag: String? = null,
    // Offer the fullscreen split editor (web parity). False inside that dialog
    // itself, which reuses this composable and must not offer to reopen itself.
    allowFullscreen: Boolean = true,
    // Controlled preview mode: when non-null the host owns the Написать/Просмотр
    // flag and renders the eye/pencil toggle itself (the comment composers place it
    // next to Send/Cancel). Null → the editor keeps its own state and shows the
    // toggle in its top row (the task description). Also used with `false` inside the
    // fullscreen dialog, whose separate preview pane makes an in-editor toggle moot.
    previewOverride: Boolean? = null,
) {
    val c = Tessera.colors
    // Saveable, not plain remember: inside the task modal the editor lives in a tab
    // that leaves the composition when another tab is shown (#2754) — the chosen
    // Написать/Просмотр mode has to survive coming back, and a rotation with it.
    var previewInternal by rememberSaveable { mutableStateOf(startInPreview) }
    val controlled = previewOverride != null
    val preview = previewOverride ?: previewInternal
    var fullscreen by rememberSaveable { mutableStateOf(false) }
    var uploading by remember { mutableStateOf(false) }
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()

    // Scroll anchor + the live IME inset, used to keep the editor's bottom visible
    // once the keyboard is up (see the tail Spacer below).
    val tail = remember { BringIntoViewRequester() }
    val imeBottom = WindowInsets.ime.getBottom(LocalDensity.current)

    // Internal selection-aware state; resynced when [value] changes externally
    // (e.g. an image insert appends to the parent's string).
    var tfv by remember { mutableStateOf(TextFieldValue(value)) }
    if (tfv.text != value) tfv = TextFieldValue(value)
    /** Programmatic edit (toolbar, suggestion pick) — applied verbatim. */
    fun set(next: TextFieldValue) {
        tfv = next
        if (next.text != value) onValueChange(next.text)
    }

    /** An edit the IME just made — smart-typing rules get a say first. */
    fun update(next: TextFieldValue) = set(smartTyping(tfv, next))

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
        // A single right-aligned control row (the old Написать/Просмотр tabs are gone,
        // replaced by the eye/pencil toggle): insert actions in write mode, then the
        // mode toggle next to the fullscreen button — matching the web editor.
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
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
                    onClick = { set(insert(tfv, MERMAID_SNIPPET)) },
                    boxSize = 30.dp,
                    iconSize = 17.dp,
                    tint = c.text2,
                )
            }
            // Controlled editors (comments) hide the toggle — the host renders it by
            // its Send/Cancel buttons; the fullscreen dialog has its own preview pane.
            if (!controlled) {
                Spacer(Modifier.width(2.dp))
                MarkdownModeToggle(preview = preview, onToggle = { previewInternal = !preview })
            }
            if (allowFullscreen) {
                Spacer(Modifier.width(2.dp))
                IonIconButton(
                    Ion.EXPAND,
                    onClick = { fullscreen = true },
                    boxSize = 30.dp,
                    iconSize = 16.dp,
                    tint = c.text2,
                )
            }
        }

        if (!preview) {
            Spacer(Modifier.height(6.dp))
            FormatToolbar(onApply = { set(it(tfv)) })
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
                        taskRefs = onTaskRef != null,
                        onTaskRef = onTaskRef,
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
            var focused by remember { mutableStateOf(false) }
            // Autocomplete: "@query" anywhere before the caret, or a `/`-command at
            // the start of a line. Mentions win — a mention query can't start a line
            // with a slash, so the two never compete for the same caret.
            val caret = tfv.selection.min
            val upto = tfv.text.substring(0, caret)
            val mq = if (mentions.isNotEmpty()) MENTION_RE.find(upto) else null
            val query = mq?.groupValues?.get(2)
            val matches = if (query != null) {
                mentions.filter {
                    it.display.contains(query, ignoreCase = true) || it.insert.contains(query, ignoreCase = true)
                }.take(8)
            } else {
                emptyList()
            }
            val slash = if (query == null && commands.isNotEmpty()) detectSlashQuery(upto) else null
            val cmdMatches = if (slash != null) matchCommands(commands, slash.query) else emptyList()
            fun pickMention(item: MentionItem) {
                val start = caret - (query?.length ?: 0) - 1
                val ins = "@${item.insert} "
                val text = tfv.text.substring(0, start) + ins + tfv.text.substring(caret)
                set(TextFieldValue(text, androidx.compose.ui.text.TextRange(start + ins.length)))
            }
            fun pickCommand(item: CommandItem) {
                val start = slash?.start ?: return
                val ins = commandInsertText(item)
                val text = tfv.text.substring(0, start) + ins + tfv.text.substring(caret)
                set(TextFieldValue(text, androidx.compose.ui.text.TextRange(start + ins.length)))
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
                        .then(if (fieldTag != null) Modifier.testTag(fieldTag) else Modifier)
                        .heightIn(min = minHeight)
                        .clip(RoundedCornerShape(RadiusMd))
                        .background(c.surface)
                        .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
                        .padding(12.dp)
                        .onFocusChanged {
                            focused = it.isFocused
                            if (it.isFocused) hadFocus = true else if (hadFocus) onBlur?.invoke()
                        },
                    decorationBox = { inner ->
                        if (tfv.text.isEmpty()) Text(placeholder, color = c.placeholder, fontSize = 14.sp)
                        inner()
                    },
                )
            }
            if (matches.isNotEmpty()) MentionSuggestions(matches, onPick = ::pickMention)
            if (cmdMatches.isNotEmpty()) CommandSuggestions(cmdMatches, onPick = ::pickCommand)
            // Keep the editor's bottom edge — the field and, once typing starts, the
            // suggestion list — inside the scroll viewport. The anchor is a zero-height
            // marker under the list, so bringing it into view scrolls to the END of the
            // block instead of its top. Re-run while the IME animates in (imeBottom
            // changes) and whenever the list grows: right after focus the keyboard has
            // not yet taken its space, so a single scroll would stop short.
            Spacer(Modifier.fillMaxWidth().height(1.dp).bringIntoViewRequester(tail))
            LaunchedEffect(focused, imeBottom, matches.size, cmdMatches.size) {
                if (focused) runCatching { tail.bringIntoView() }
            }
        }
    }

    if (fullscreen) {
        MarkdownFullscreenDialog(
            value = value,
            onValueChange = onValueChange,
            placeholder = placeholder,
            mentions = mentions,
            commands = commands,
            onTaskRef = onTaskRef,
            uploadImage = uploadImage,
            onDismiss = {
                fullscreen = false
                // The inline field never saw focus while the dialog was up, so its
                // blur never fires — persist what came back from the dialog here.
                onBlur?.invoke()
            },
        )
    }
}

/**
 * The web editor's fullscreen mode: the same editor over a live preview, so long
 * descriptions can be written without the surrounding modal's cramped height.
 * Landscape / tablet (the short-height case) puts the two side by side instead —
 * stacked, each pane would be a couple of lines tall.
 */
@Composable
private fun MarkdownFullscreenDialog(
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    mentions: List<MentionItem>,
    commands: List<CommandItem>,
    onTaskRef: ((Int) -> Unit)?,
    uploadImage: (suspend (ByteArray, String, String?) -> String?)?,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    val wide = LocalConfiguration.current.screenWidthDp >= 600
    // The two panes carry their own scroll, kept lined up as either is dragged (web
    // parity for long descriptions). Proportional — the panes differ in height — and
    // cross-guarded on isScrollInProgress so the follower's programmatic scroll can't
    // feed back into a loop.
    val editorScroll = rememberScrollState()
    val previewScroll = rememberScrollState()
    LaunchedEffect(editorScroll, previewScroll) {
        snapshotFlow { editorScroll.value }.collect { v ->
            if (editorScroll.isScrollInProgress && !previewScroll.isScrollInProgress) {
                val room = editorScroll.maxValue
                if (room > 0) previewScroll.scrollTo((v.toFloat() / room * previewScroll.maxValue).roundToInt())
            }
        }
    }
    LaunchedEffect(editorScroll, previewScroll) {
        snapshotFlow { previewScroll.value }.collect { v ->
            if (previewScroll.isScrollInProgress && !editorScroll.isScrollInProgress) {
                val room = previewScroll.maxValue
                if (room > 0) editorScroll.scrollTo((v.toFloat() / room * editorScroll.maxValue).roundToInt())
            }
        }
    }
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(Modifier.fillMaxSize().background(c.bg).systemBarsPadding().imePadding()) {
            Row(
                Modifier.fillMaxWidth().padding(start = 16.dp, end = 8.dp, top = 8.dp, bottom = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("Редактор", color = c.text1, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                IonIconButton(Ion.CLOSE, onClick = onDismiss, boxSize = 34.dp, iconSize = 18.dp, tint = c.text2)
            }
            val panes: @Composable (Modifier, Modifier) -> Unit = { editorMod, previewMod ->
                Box(editorMod.padding(horizontal = 12.dp).verticalScroll(editorScroll)) {
                    MarkdownEditor(
                        value = value,
                        onValueChange = onValueChange,
                        placeholder = placeholder,
                        mentions = mentions,
                        commands = commands,
                        onTaskRef = onTaskRef,
                        uploadImage = uploadImage,
                        allowFullscreen = false,
                        // Always the editing pane — the preview lives in its own box
                        // beside/below it, so no in-editor toggle.
                        previewOverride = false,
                    )
                }
                Box(
                    previewMod
                        .padding(12.dp)
                        .clip(RoundedCornerShape(RadiusMd))
                        .background(c.surfaceAlt)
                        .padding(12.dp)
                        .verticalScroll(previewScroll),
                ) {
                    if (value.isBlank()) {
                        Text("Предпросмотр", color = c.placeholder, fontSize = 13.sp)
                    } else {
                        RichContent(value, mentions = mentions, taskRefs = onTaskRef != null, onTaskRef = onTaskRef)
                    }
                }
            }
            if (wide) {
                Row(Modifier.fillMaxWidth().weight(1f)) {
                    panes(Modifier.weight(1f).fillMaxHeight(), Modifier.weight(1f).fillMaxHeight())
                }
            } else {
                panes(Modifier.fillMaxWidth().weight(1f), Modifier.fillMaxWidth().weight(1f))
            }
        }
    }
}

/**
 * Re-applies the smart-typing rules to the edit the IME already made ([prev] →
 * [next]), returning the state it should have been. A soft keyboard delivers no
 * key events worth intercepting, so the rules run on the diff instead. A live
 * composing region is left strictly alone: rewriting text under predictive input
 * corrupts what the IME thinks it owns.
 */
private fun smartTyping(prev: TextFieldValue, next: TextFieldValue): TextFieldValue {
    if (next.composition != null || !next.selection.collapsed) return next
    val edit = applyTyping(
        prev.text,
        prev.selection.start,
        prev.selection.end,
        next.text,
        next.selection.start,
    ) ?: return next
    return TextFieldValue(edit.text, androidx.compose.ui.text.TextRange(edit.start, edit.end))
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
        FmtButton({ IonIcon(Ion.LIST, size = 15.dp, tint = it) }) { onApply { v -> orderedList(v) } }
        // The literal marker, not a glyph: no bundled ionicon draws a checkbox, and
        // a text ☑ is not in every system font.
        FmtButton({ Text("[ ]", fontFamily = FontFamily.Monospace, fontSize = 12.sp, color = it) }) { onApply { v -> linePrefix(v, "- [ ] ") } }
        FmtButton({ Text("❝", fontSize = 14.sp, color = it) }) { onApply { v -> linePrefix(v, "> ") } }
        FmtButton({ IonIcon(Ion.CHEVRON_DOWN, size = 15.dp, tint = it) }) { onApply { v -> spoiler(v) } }
        FmtButton({ IonIcon(Ion.LINK, size = 15.dp, tint = it) }) { onApply { v -> link(v) } }
        // Indent / outdent are buttons because Android has no Tab: the soft keyboard
        // has no such key, and a hardware one is not something we can count on.
        FmtButton({ IonIcon(Ion.CHEVRON_FORWARD, Modifier.rotate(180f), size = 15.dp, tint = it) }) { onApply { v -> outdent(v) } }
        FmtButton({ IonIcon(Ion.CHEVRON_FORWARD, size = 15.dp, tint = it) }) { onApply { v -> indent(v) } }
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

/** @-mention suggestion list shown under the textarea while typing "@…". Each row
 *  shows the member/GitLab-user avatar + display name (web parity). */
@Composable
private fun MentionSuggestions(items: List<MentionItem>, onPick: (MentionItem) -> Unit) {
    val c = Tessera.colors
    Column(
        Modifier
            .fillMaxWidth()
            .padding(top = 4.dp)
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.surfaceAlt)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd)),
    ) {
        items.forEach { item ->
            Row(
                Modifier
                    .fillMaxWidth()
                    .clickableNoRipple { onPick(item) }
                    .padding(horizontal = 12.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                MemberAvatar(22.dp, item.display, userId = item.avatarUserId, avatarUrl = item.avatarSrc, muted = item.gitlab)
                Spacer(Modifier.width(10.dp))
                Text(item.display, color = c.text1, fontSize = 14.sp, modifier = Modifier.weight(1f))
                if (item.gitlab) {
                    Spacer(Modifier.width(8.dp))
                    Text("GitLab", color = c.text3, fontSize = 11.sp)
                }
            }
        }
    }
}

/**
 * `/`-command suggestions (web parity): the mono `/key` plus its description.
 * Custom dictionary entries carry a neutral «свои» badge — they are a hint for a
 * human reader, the backend never executes them, so they must not read as an
 * action the app will take.
 */
@Composable
private fun CommandSuggestions(items: List<CommandItem>, onPick: (CommandItem) -> Unit) {
    val c = Tessera.colors
    Column(
        Modifier
            .fillMaxWidth()
            .padding(top = 4.dp)
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.surfaceAlt)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd)),
    ) {
        items.forEach { item ->
            Row(
                Modifier
                    .fillMaxWidth()
                    .clickableNoRipple { onPick(item) }
                    .padding(horizontal = 12.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    "/${item.key}",
                    color = c.text1,
                    fontSize = 13.sp,
                    fontFamily = FontFamily.Monospace,
                    fontWeight = FontWeight.Medium,
                )
                Spacer(Modifier.width(10.dp))
                Text(
                    item.description,
                    color = c.text3,
                    fontSize = 12.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                if (!item.builtin) {
                    Spacer(Modifier.width(8.dp))
                    Text("свои", color = c.text3, fontSize = 11.sp)
                }
            }
        }
    }
}

/**
 * The eye/pencil preview toggle (web parity — it replaces the old Написать/Просмотр
 * tabs). An eye while editing (tap → preview), a pencil while previewing (tap →
 * edit). Hosted in the editor's top row for the description, and next to the
 * Send/Cancel buttons for the comment composers.
 */
@Composable
fun MarkdownModeToggle(preview: Boolean, onToggle: () -> Unit, modifier: Modifier = Modifier) {
    IonIconButton(
        if (preview) Ion.PENCIL else Ion.EYE,
        onClick = onToggle,
        boxSize = 30.dp,
        iconSize = 17.dp,
        tint = Tessera.colors.text2,
        modifier = modifier,
    )
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

private fun MdEdit.toValue(): TextFieldValue =
    TextFieldValue(text, androidx.compose.ui.text.TextRange(start, end))

/** Prepends [prefix] to every line the selection touches, keeping the block selected. */
private fun linePrefix(v: TextFieldValue, prefix: String): TextFieldValue =
    linePrefixLines(v.text, v.selection.min, v.selection.max, prefix).toValue()

/** Same, numbering the lines 1., 2., 3. */
private fun orderedList(v: TextFieldValue): TextFieldValue =
    linePrefixLines(v.text, v.selection.min, v.selection.max, orderedListPrefix).toValue()

private fun indent(v: TextFieldValue): TextFieldValue =
    indentLines(v.text, v.selection.min, v.selection.max).toValue()

/** Nothing to strip (already at column 0) leaves the value untouched. */
private fun outdent(v: TextFieldValue): TextFieldValue =
    outdentLines(v.text, v.selection.min, v.selection.max)?.toValue() ?: v

/** Blank lines around the body are load-bearing: without them the markdown inside
 *  `<details>` is kept as raw HTML and never renders (same as on web). */
private fun spoiler(v: TextFieldValue): TextFieldValue {
    val sel = v.text.substring(v.selection.min, v.selection.max).ifEmpty { "Скрытый текст" }
    return insert(v, "\n<details><summary>Подробнее</summary>\n\n$sel\n\n</details>\n")
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
