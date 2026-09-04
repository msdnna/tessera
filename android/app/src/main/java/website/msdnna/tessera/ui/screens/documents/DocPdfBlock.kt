package website.msdnna.tessera.ui.screens.documents

import android.graphics.Bitmap
import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import website.msdnna.tessera.R
import website.msdnna.tessera.data.repository.DocumentRepository
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.DocPdf
import website.msdnna.tessera.util.DocSizeUnit
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.clampPdfPage
import website.msdnna.tessera.util.docFileSize
import website.msdnna.tessera.util.pdfBitmapSize

// A PDF read in place (#2733, §3 of #2894).
//
// The web client renders it with pdf.js; here it is `android.graphics.pdf`,
// which needs a real file — PdfRenderer reads through a file descriptor and does
// random access over it, so the asset is pulled into the cache first (see
// DocumentRepository.downloadAsset) and rendered one page at a time.
//
// One page at a time on purpose: pdf.js keeps a window of pages because the web
// viewer scrolls continuously, while this one has a phone's worth of width and
// two arrows. Rendering ahead would allocate several screen-sized bitmaps to
// show one.

private sealed interface PdfState {
    data object Loading : PdfState

    data class Ready(val file: java.io.File) : PdfState

    data object Failed : PdfState
}

@Composable
internal fun DocPdfBlockView(block: DocPdf, repo: DocumentRepository = remember { DocumentRepository() }) {
    val c = Tessera.colors
    val context = LocalContext.current
    var state by remember(block.src) { mutableStateOf<PdfState>(PdfState.Loading) }

    LaunchedEffect(block.src) {
        if (block.src.isBlank()) {
            state = PdfState.Failed
            return@LaunchedEffect
        }
        state = PdfState.Loading
        val file = runCatching {
            withContext(Dispatchers.IO) { repo.downloadAsset(context.cacheDir, block.src, cacheKey(block)) }
        }.getOrNull()
        state = if (file == null) PdfState.Failed else PdfState.Ready(file)
    }

    Column(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.surfaceAlt)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
            .testTag(TestTags.DOCUMENT_PDF),
    ) {
        PdfHeader(block)
        when (val current = state) {
            is PdfState.Loading -> Box(
                Modifier.fillMaxWidth().height(160.dp),
                contentAlignment = Alignment.Center,
            ) { TesseraLoader() }

            is PdfState.Failed -> Box(
                Modifier.fillMaxWidth().height(96.dp).padding(12.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    stringResource(R.string.docs_pdf_failed),
                    color = c.text3,
                    fontSize = 12.sp,
                )
            }

            is PdfState.Ready -> PdfPages(current.file)
        }
    }
}

/** File name and size — what the reader knows before a byte has been fetched. */
@Composable
private fun PdfHeader(block: DocPdf) {
    val c = Tessera.colors
    val size = remember(block.size) { docFileSize(block.size) }
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text("PDF", color = c.text3, fontSize = 11.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.width(8.dp))
        Text(
            block.name.ifBlank { stringResource(R.string.docs_pdf_untitled) },
            color = c.text1,
            fontSize = 13.sp,
            maxLines = 1,
            modifier = Modifier.weight(1f),
        )
        if (size != null) {
            Spacer(Modifier.width(8.dp))
            Text(
                "${size.text} ${stringResource(unitLabel(size.unit))}",
                color = c.text3,
                fontSize = 11.sp,
            )
        }
    }
}

@Composable
private fun PdfPages(file: java.io.File) {
    val c = Tessera.colors
    var page by remember(file) { mutableIntStateOf(1) }
    var total by remember(file) { mutableIntStateOf(0) }
    var bitmap by remember(file) { mutableStateOf<Bitmap?>(null) }
    var failed by remember(file) { mutableStateOf(false) }
    val density = LocalDensity.current

    BoxWithConstraints(Modifier.fillMaxWidth()) {
        val widthPx = with(density) { maxWidth.roundToPx() }
        LaunchedEffect(file, page, widthPx) {
            if (widthPx <= 0) return@LaunchedEffect
            val rendered = renderPdfPage(file, page, widthPx)
            if (rendered == null) {
                failed = true
            } else {
                bitmap = rendered.bitmap
                total = rendered.pageCount
                failed = false
            }
        }
        val image = bitmap
        when {
            failed -> Box(
                Modifier.fillMaxWidth().height(96.dp).padding(12.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    stringResource(R.string.docs_pdf_failed),
                    color = c.text3,
                    fontSize = 12.sp,
                )
            }

            image == null -> Box(
                // Reserves roughly a portrait page so the reader does not jump
                // when the first render lands.
                Modifier.fillMaxWidth().aspectRatio(A4_RATIO),
                contentAlignment = Alignment.Center,
            ) { TesseraLoader() }

            else -> Image(
                image.asImageBitmap(),
                contentDescription = null,
                modifier = Modifier.fillMaxWidth().testTag(TestTags.DOCUMENT_PDF_PAGE),
            )
        }
    }

    if (total > 1) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.Center,
        ) {
            IonIconButton(
                Ion.CHEVRON_FORWARD,
                onClick = { page = clampPdfPage(page - 1, total) },
                boxSize = 36.dp,
                tint = if (page > 1) c.text2 else c.text3.copy(alpha = DISABLED_ALPHA),
                modifier = Modifier.graphicsLayer { scaleX = -1f }.testTag(TestTags.DOCUMENT_PDF_PREV),
            )
            Text(
                stringResource(R.string.docs_pdf_page, page, total),
                color = c.text3,
                fontSize = 12.sp,
                modifier = Modifier.padding(horizontal = 8.dp),
            )
            IonIconButton(
                Ion.CHEVRON_FORWARD,
                onClick = { page = clampPdfPage(page + 1, total) },
                boxSize = 36.dp,
                tint = if (page < total) c.text2 else c.text3.copy(alpha = DISABLED_ALPHA),
                modifier = Modifier.testTag(TestTags.DOCUMENT_PDF_NEXT),
            )
        }
    }
}

private class RenderedPage(val bitmap: Bitmap, val pageCount: Int)

/**
 * Renders one page into a bitmap the width it is drawn at.
 *
 * The renderer is opened and closed around every page rather than held open:
 * holding it would keep a file descriptor per PDF block in a scrolling list, and
 * opening one costs a header parse, not a re-read of the file.
 */
private suspend fun renderPdfPage(file: java.io.File, page: Int, widthPx: Int): RenderedPage? =
    withContext(Dispatchers.IO) {
        runCatching {
            ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY).use { fd ->
                PdfRenderer(fd).use { renderer ->
                    val count = renderer.pageCount
                    if (count <= 0) return@runCatching null
                    renderer.openPage(clampPdfPage(page, count) - 1).use { open ->
                        val size = pdfBitmapSize(open.width, open.height, widthPx)
                            ?: return@runCatching null
                        val bitmap = Bitmap.createBitmap(size.first, size.second, Bitmap.Config.ARGB_8888)
                        // A PDF page is transparent where it is blank; without a
                        // sheet under it the text would be drawn onto the dark
                        // theme's background and read as damaged.
                        bitmap.eraseColor(android.graphics.Color.WHITE)
                        open.render(bitmap, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                        RenderedPage(bitmap, count)
                    }
                }
            }
        }.getOrNull()
    }

/**
 * The name the asset is cached under. Taken from the URL, not from the block's
 * caption: the caption is the author's title and repeats across documents, while
 * the asset path is unique and immutable.
 */
private fun cacheKey(block: DocPdf): String {
    val path = block.src.substringBefore('?').substringAfterLast('/')
    val name = path.ifBlank { block.id }
    return if (name.endsWith(".pdf", ignoreCase = true)) name else "$name.pdf"
}

private fun unitLabel(unit: DocSizeUnit) = when (unit) {
    DocSizeUnit.BYTES -> R.string.docs_size_b
    DocSizeUnit.KB -> R.string.docs_size_kb
    DocSizeUnit.MB -> R.string.docs_size_mb
    DocSizeUnit.GB -> R.string.docs_size_gb
}

private const val A4_RATIO = 210f / 297f

// An arrow at the end of the document stays put rather than disappearing — the
// row would otherwise re-centre under the reader's thumb between pages.
// clampPdfPage makes the tap itself a no-op, so this is the whole of «disabled».
private const val DISABLED_ALPHA = 0.4f
