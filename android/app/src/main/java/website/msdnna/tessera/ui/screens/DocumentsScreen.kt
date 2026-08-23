package website.msdnna.tessera.ui.screens

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.withLink
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import coil.compose.AsyncImage
import website.msdnna.tessera.R
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.resolve
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.DocumentsViewModel
import website.msdnna.tessera.util.DocBlock
import website.msdnna.tessera.util.DocCode
import website.msdnna.tessera.util.DocDivider
import website.msdnna.tessera.util.DocHeading
import website.msdnna.tessera.util.DocImage
import website.msdnna.tessera.util.DocListRow
import website.msdnna.tessera.util.DocParagraph
import website.msdnna.tessera.util.DocQuote
import website.msdnna.tessera.util.DocSpan
import website.msdnna.tessera.util.DocTable
import website.msdnna.tessera.util.DocTreeRow
import website.msdnna.tessera.util.Ion

/**
 * Documents module (web `DocumentsView`), **read-only** — see #2735. A tree of
 * the workspace's documents; tapping one slides a reader over it, the same
 * master/detail shape [NotesScreen] uses. Editing is deliberately out of scope:
 * a block editor with drag handles and per-block locks is its own project.
 */
@Composable
fun DocumentsScreen(workspaceId: String) {
    val c = Tessera.colors
    val vm: DocumentsViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()

    LaunchedEffect(workspaceId) {
        if (workspaceId.isNotBlank()) vm.load(workspaceId)
    }

    // The reader is an inline overlay, not a Dialog, so Back would otherwise
    // fall through to the nav back-stack.
    BackHandler(enabled = state.openId != null) { vm.close() }

    Box(Modifier.fillMaxSize().background(c.bg).testTag(TestTags.DOCUMENTS_SCREEN)) {
        when {
            state.loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                TesseraLoader()
            }

            state.rows.isEmpty() -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    IonIcon(Ion.BOOK, size = 40.dp, tint = c.text3)
                    Spacer(Modifier.height(10.dp))
                    Text(stringResource(R.string.docs_empty), color = c.text3, fontSize = 14.sp)
                    Spacer(Modifier.height(4.dp))
                    Text(stringResource(R.string.docs_empty_hint), color = c.placeholder, fontSize = 12.sp)
                }
            }

            else -> LazyColumn(
                Modifier.fillMaxSize().padding(horizontal = 12.dp, vertical = 12.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                items(state.rows, key = { it.doc.id }) { row ->
                    DocumentRow(row, onClick = { vm.open(row.doc) })
                }
                item { Spacer(Modifier.height(16.dp)) }
            }
        }

        state.error?.let { message ->
            Box(Modifier.fillMaxSize().padding(16.dp), contentAlignment = Alignment.BottomCenter) {
                Text(message.resolve(), color = c.text3, fontSize = 12.sp)
            }
        }

        if (state.openId != null) {
            DocumentReader(
                title = state.open?.title.orEmpty(),
                icon = state.open?.icon.orEmpty(),
                blocks = state.blocks,
                loading = state.opening,
                onBack = { vm.close() },
            )
        }
    }
}

@Composable
private fun DocumentRow(row: DocTreeRow, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().padding(start = (row.depth * 16).dp)
            .clip(RoundedCornerShape(RadiusMd)).background(c.cardSurface)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
            .clickableNoRipple(onClick = onClick).padding(12.dp)
            .testTag(TestTags.documentRow(row.doc.id)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // `icon` is an emoji on the web; fall back to the section's own glyph.
        if (row.doc.icon.isNotBlank()) {
            Text(row.doc.icon, fontSize = 16.sp)
        } else {
            IonIcon(Ion.BOOK, size = 18.dp, tint = c.text3)
        }
        Spacer(Modifier.width(10.dp))
        Column(Modifier.weight(1f)) {
            Text(
                row.doc.title.ifBlank { stringResource(R.string.docs_untitled) },
                color = c.text1,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
            )
            val snippet = row.doc.preview.replace("\n", " ").take(80)
            if (snippet.isNotBlank()) {
                Spacer(Modifier.height(3.dp))
                Text(snippet, color = c.text3, fontSize = 12.sp, maxLines = 1)
            }
        }
    }
}

@Composable
private fun DocumentReader(
    title: String,
    icon: String,
    blocks: List<DocBlock>,
    loading: Boolean,
    onBack: () -> Unit,
) {
    val c = Tessera.colors
    Column(Modifier.fillMaxSize().background(c.surface).testTag(TestTags.DOCUMENT_READER)) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIconButton(
                Ion.CHEVRON_FORWARD,
                onClick = onBack,
                boxSize = 40.dp,
                modifier = Modifier.graphicsLayer { scaleX = -1f },
            )
            Spacer(Modifier.width(4.dp))
            if (icon.isNotBlank()) {
                Text(icon, fontSize = 16.sp)
                Spacer(Modifier.width(6.dp))
            }
            Text(
                title.ifBlank { stringResource(R.string.docs_reader_untitled) },
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                modifier = Modifier.weight(1f),
            )
        }
        HorizontalDivider(color = c.border)

        when {
            loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { TesseraLoader() }

            blocks.isEmpty() -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text(stringResource(R.string.docs_reader_empty), color = c.text3, fontSize = 14.sp)
            }

            else -> LazyColumn(
                Modifier.fillMaxSize().padding(horizontal = 16.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                item { Spacer(Modifier.height(8.dp)) }
                items(blocks, key = { it.id }) { block -> DocBlockView(block) }
                item { Spacer(Modifier.height(24.dp)) }
            }
        }
    }
}

@Composable
private fun DocBlockView(block: DocBlock) {
    val c = Tessera.colors
    when (block) {
        is DocParagraph -> Text(
            annotate(block.spans),
            color = c.text1,
            fontSize = 15.sp,
            textAlign = alignOf(block.align),
            modifier = Modifier.fillMaxWidth().padding(start = (block.indent * 16).dp),
        )

        is DocHeading -> Text(
            annotate(block.spans),
            color = c.text1,
            fontSize = headingSize(block.level),
            fontWeight = FontWeight.Bold,
            textAlign = alignOf(block.align),
            modifier = Modifier.fillMaxWidth().padding(top = 6.dp),
        )

        is DocListRow -> Row(Modifier.fillMaxWidth().padding(start = (12 + block.depth * 16).dp)) {
            when {
                block.checked != null -> {
                    IonIcon(
                        if (block.checked == true) Ion.CHECK_CIRCLE else Ion.ELLIPSE,
                        size = 16.dp,
                        tint = if (block.checked == true) c.primary else c.text3,
                        modifier = Modifier.padding(top = 3.dp),
                    )
                    Spacer(Modifier.width(8.dp))
                }

                else -> {
                    Text(
                        block.marker,
                        color = c.text3,
                        fontSize = 15.sp,
                        modifier = Modifier.widthIn(min = 18.dp),
                    )
                    Spacer(Modifier.width(4.dp))
                }
            }
            Text(annotate(block.spans), color = c.text1, fontSize = 15.sp, modifier = Modifier.weight(1f))
        }

        is DocQuote -> Row(Modifier.fillMaxWidth()) {
            Box(Modifier.width(3.dp).height(20.dp).background(c.primary))
            Spacer(Modifier.width(10.dp))
            Text(annotate(block.spans), color = c.text2, fontSize = 15.sp, fontStyle = FontStyle.Italic)
        }

        is DocCode -> Column(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.surfaceAlt)
                .border(1.dp, c.border, RoundedCornerShape(RadiusMd)).padding(12.dp),
        ) {
            if (block.language.isNotBlank()) {
                Text(block.language, color = c.text3, fontSize = 11.sp)
                Spacer(Modifier.height(4.dp))
            }
            // Code must not wrap silently — an indented block reads as different
            // code once it reflows, so it scrolls instead.
            Box(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState())) {
                Text(block.text, color = c.text1, fontSize = 13.sp, fontFamily = FontFamily.Monospace, softWrap = false)
            }
        }

        is DocDivider -> HorizontalDivider(color = c.border, modifier = Modifier.padding(vertical = 4.dp))

        is DocImage -> DocImageView(block)

        is DocTable -> DocTableView(block)
    }
}

@Composable
private fun DocImageView(block: DocImage) {
    val c = Tessera.colors
    // Document assets are served from our own origin behind a signature, and the
    // stored src is the path only — Coil needs it absolute.
    val model = remember(block.src) {
        when {
            block.src.isBlank() -> null
            block.src.startsWith("http") || block.src.startsWith("data:") -> block.src
            else -> RetrofitClient.serverRoot + block.src
        }
    }
    if (model == null) return
    Column(Modifier.fillMaxWidth()) {
        AsyncImage(
            model = model,
            contentDescription = block.alt.ifBlank { stringResource(R.string.docs_image_alt) },
            modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)),
        )
        if (block.alt.isNotBlank()) {
            Spacer(Modifier.height(4.dp))
            Text(block.alt, color = c.text3, fontSize = 12.sp)
        }
    }
}

@Composable
private fun DocTableView(block: DocTable) {
    val c = Tessera.colors
    // Tables are authored for a desktop width; scrolling sideways keeps cells
    // readable instead of squeezing every column into the phone's width.
    Box(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState())) {
        Column(
            Modifier.clip(RoundedCornerShape(RadiusMd)).border(1.dp, c.border, RoundedCornerShape(RadiusMd)),
        ) {
            block.rows.forEachIndexed { index, row ->
                if (index > 0) HorizontalDivider(color = c.border)
                Row {
                    row.cells.forEachIndexed { cellIndex, cell ->
                        if (cellIndex > 0) {
                            Box(Modifier.width(1.dp).height(36.dp).background(c.border))
                        }
                        Text(
                            annotate(cell.spans),
                            color = if (cell.header) c.text1 else c.text2,
                            fontSize = 13.sp,
                            fontWeight = if (cell.header) FontWeight.SemiBold else FontWeight.Normal,
                            modifier = Modifier.widthIn(min = 96.dp, max = 220.dp)
                                .background(if (cell.header) c.surfaceAlt else Color.Transparent)
                                .padding(horizontal = 10.dp, vertical = 8.dp),
                        )
                    }
                }
            }
        }
    }
}

/** Renders inline runs with their marks; links stay tappable. */
@Composable
private fun annotate(spans: List<DocSpan>) = buildAnnotatedString {
    val c = Tessera.colors
    val uriHandler = LocalUriHandler.current
    for (span in spans) {
        val style = SpanStyle(
            fontWeight = if (span.bold) FontWeight.Bold else null,
            fontStyle = if (span.italic) FontStyle.Italic else null,
            fontFamily = if (span.code) FontFamily.Monospace else null,
            color = parseHex(span.color) ?: if (span.code) c.primary else Color.Unspecified,
            textDecoration = decorationOf(span),
        )
        val href = span.href
        if (href.isNullOrBlank()) {
            withStyle(style) { append(span.text) }
        } else {
            val link = LinkAnnotation.Url(
                href,
                TextLinkStyles(style.copy(color = c.primary, textDecoration = TextDecoration.Underline)),
            ) { runCatching { uriHandler.openUri(href) } }
            withLink(link) { append(span.text) }
        }
    }
}

private fun decorationOf(span: DocSpan): TextDecoration? = when {
    span.underline && span.strike -> TextDecoration.combine(
        listOf(TextDecoration.Underline, TextDecoration.LineThrough),
    )

    span.underline -> TextDecoration.Underline

    span.strike -> TextDecoration.LineThrough

    else -> null
}

/** `#rgb` / `#rrggbb` from the editor's colour picker; anything else is ignored. */
private fun parseHex(value: String?): Color? {
    val hex = value?.trim()?.removePrefix("#") ?: return null
    val full = when (hex.length) {
        3 -> hex.map { "$it$it" }.joinToString("")
        6 -> hex
        else -> return null
    }
    val rgb = full.toLongOrNull(16) ?: return null
    return Color(0xFF000000L or rgb)
}

private fun headingSize(level: Int) = when (level) {
    1 -> 24.sp
    2 -> 20.sp
    3 -> 18.sp
    else -> 16.sp
}

private fun alignOf(value: String?) = when (value) {
    "center" -> TextAlign.Center
    "right" -> TextAlign.End
    "justify" -> TextAlign.Justify
    else -> TextAlign.Start
}
