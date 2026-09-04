package website.msdnna.tessera.ui.screens.documents

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
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
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import website.msdnna.tessera.R
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.DocBlock
import website.msdnna.tessera.util.DocCode
import website.msdnna.tessera.util.DocDivider
import website.msdnna.tessera.util.DocFontKind
import website.msdnna.tessera.util.DocHeading
import website.msdnna.tessera.util.DocImage
import website.msdnna.tessera.util.DocListRow
import website.msdnna.tessera.util.DocPage
import website.msdnna.tessera.util.DocParagraph
import website.msdnna.tessera.util.DocPdf
import website.msdnna.tessera.util.DocQuote
import website.msdnna.tessera.util.DocSectionBreak
import website.msdnna.tessera.util.DocSpan
import website.msdnna.tessera.util.DocTable
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.docFontKind
import website.msdnna.tessera.util.docFontSizeSp
import website.msdnna.tessera.util.docSizeKey
import website.msdnna.tessera.util.isDocLandscape

// One row of a document body (#2735, §3 of #2894). Split out of DocumentsScreen
// when the reader grew the blocks it used to drop on the floor: a PDF, a section
// break, and the paragraph styling (spacing, font, size) the toolbar can set.

/** Body text size the reader draws at — what a span's own size is measured against. */
private val BODY_SIZE = 15.sp

@Composable
internal fun DocBlockView(block: DocBlock) {
    val c = Tessera.colors
    when (block) {
        is DocParagraph -> Text(
            annotate(block.spans),
            color = c.text1,
            fontSize = BODY_SIZE,
            lineHeight = lineHeightOf(block.lineHeight, BODY_SIZE),
            textAlign = alignOf(block.align),
            modifier = Modifier.fillMaxWidth().padding(start = indentDp(block.indent)),
        )

        is DocHeading -> {
            val size = headingSize(block.level)
            Text(
                annotate(block.spans),
                color = c.text1,
                fontSize = size,
                lineHeight = lineHeightOf(block.lineHeight, size),
                fontWeight = FontWeight.Bold,
                textAlign = alignOf(block.align),
                modifier = Modifier.fillMaxWidth()
                    .padding(top = 6.dp, start = indentDp(block.indent)),
            )
        }

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
                        fontSize = BODY_SIZE,
                        modifier = Modifier.widthIn(min = 18.dp),
                    )
                    Spacer(Modifier.width(4.dp))
                }
            }
            Text(annotate(block.spans), color = c.text1, fontSize = BODY_SIZE, modifier = Modifier.weight(1f))
        }

        is DocQuote -> Row(Modifier.fillMaxWidth().padding(start = indentDp(block.indent))) {
            Box(Modifier.width(3.dp).height(20.dp).background(c.primary))
            Spacer(Modifier.width(10.dp))
            Text(
                annotate(block.spans),
                color = c.text2,
                fontSize = BODY_SIZE,
                lineHeight = lineHeightOf(block.lineHeight, BODY_SIZE),
                fontStyle = FontStyle.Italic,
            )
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

        is DocPdf -> DocPdfBlockView(block)

        is DocSectionBreak -> DocSectionBreakView(block)
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

/**
 * A section boundary, drawn as the caption the web editor puts on it: what the
 * *next* section looks like. The reader has one column and cannot show a
 * landscape band as landscape, so the caption is the whole of it — without it a
 * document that changes geometry mid-way looks like one that does not, and the
 * wide table below the break has no explanation.
 */
@Composable
private fun DocSectionBreakView(block: DocSectionBreak) {
    val c = Tessera.colors
    val orientation = stringResource(
        if (isDocLandscape(block.page)) R.string.docs_page_landscape else R.string.docs_page_portrait,
    )
    val size = docSizeKey(block.page).let { key ->
        when (key) {
            "a3" -> "A3"
            "a4" -> "A4"
            "a5" -> "A5"
            "letter" -> "Letter"
            "legal" -> "Legal"
            else -> stringResource(R.string.docs_page_size_custom)
        }
    }
    Row(
        Modifier.fillMaxWidth().padding(vertical = 8.dp).testTag(TestTags.DOCUMENT_SECTION_BREAK),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        HorizontalDivider(color = c.border, modifier = Modifier.weight(1f))
        Text(
            stringResource(R.string.docs_section_caption, orientation.lowercase(), size),
            color = c.text3,
            fontSize = 11.sp,
        )
        HorizontalDivider(color = c.border, modifier = Modifier.weight(1f))
    }
}

/** Renders inline runs with their marks; links stay tappable. */
@Composable
internal fun annotate(spans: List<DocSpan>) = buildAnnotatedString {
    val c = Tessera.colors
    val uriHandler = LocalUriHandler.current
    for (span in spans) {
        val style = SpanStyle(
            fontSize = docFontSizeSp(span.fontSize)?.sp ?: TextUnit.Unspecified,
            fontWeight = if (span.bold) FontWeight.Bold else null,
            fontStyle = if (span.italic) FontStyle.Italic else null,
            // A run that is code is monospace whatever family it also carries:
            // the mark means «this is code», and the picker's font is decoration.
            fontFamily = if (span.code) FontFamily.Monospace else familyOf(span.fontFamily),
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

private fun familyOf(stack: String?): FontFamily? = when (docFontKind(stack)) {
    DocFontKind.SERIF -> FontFamily.Serif
    DocFontKind.MONO -> FontFamily.Monospace
    DocFontKind.SANS -> FontFamily.SansSerif
    DocFontKind.DEFAULT -> null
}

/**
 * The block's line spacing as a size. Unspecified when the block sets none, so
 * the theme's own line height keeps applying — a hard-coded 1.0 would tighten
 * every paragraph that never asked for spacing.
 */
private fun lineHeightOf(multiplier: Float?, fontSize: androidx.compose.ui.unit.TextUnit): TextUnit =
    multiplier?.let { (fontSize.value * it).sp } ?: TextUnit.Unspecified

private fun indentDp(indent: Int) = (indent.coerceIn(0, MAX_INDENT) * 16).dp

// Matches MAX_INDENT in blockStyle.js. An imported document can carry more, and
// eight steps is already the whole width of a phone.
private const val MAX_INDENT = 8

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
