package website.msdnna.tessera.ui.screens.documents

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Document
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.DocCrumb
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.shortDate

/**
 * One nesting level of the documents section: the breadcrumb trail of
 * containers walked into, «Новый документ», and the tiles themselves.
 *
 * A grid rather than an indented tree, matching the web after the review of
 * #2726 — a tile opens its document, and nesting is walked with the trail. On a
 * phone the tiles are wide, so the grid is one or two columns depending on the
 * screen rather than the web's four.
 */
@Composable
fun DocumentsList(
    tiles: List<Document>,
    trail: List<DocCrumb>,
    childCount: (String) -> Int,
    busy: Boolean,
    onCrumb: (Int) -> Unit,
    onOpen: (Document) -> Unit,
    onCreate: () -> Unit,
    onTemplates: () -> Unit,
) {
    Column(Modifier.fillMaxSize()) {
        DocumentsHead(
            trail = trail,
            busy = busy,
            onCrumb = onCrumb,
            onCreate = onCreate,
            onTemplates = onTemplates,
        )
        if (tiles.isEmpty()) {
            DocumentsEmpty(nested = trail.isNotEmpty())
        } else {
            LazyVerticalGrid(
                columns = GridCells.Adaptive(minSize = TILE_MIN_WIDTH),
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(start = 12.dp, end = 12.dp, bottom = 24.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                items(tiles, key = { it.id }) { doc ->
                    DocumentTile(doc = doc, nested = childCount(doc.id), onClick = { onOpen(doc) })
                }
            }
        }
    }
}

/** Breadcrumbs on the left, the two ways to start a document on the right. */
@Composable
private fun DocumentsHead(
    trail: List<DocCrumb>,
    busy: Boolean,
    onCrumb: (Int) -> Unit,
    onCreate: () -> Unit,
    onTemplates: () -> Unit,
) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // A deep trail is longer than a phone is wide; it scrolls sideways rather
        // than wrapping, so the create button keeps its place.
        Row(
            Modifier.weight(1f).horizontalScroll(rememberScrollState()),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Crumb(
                title = stringResource(R.string.docs_crumb_root),
                active = trail.isEmpty(),
                tag = TestTags.DOCUMENTS_CRUMB_ROOT,
                onClick = { onCrumb(-1) },
            )
            trail.forEachIndexed { index, crumb ->
                Text("/", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(horizontal = 4.dp))
                Crumb(
                    title = crumb.title.ifBlank { stringResource(R.string.docs_untitled) },
                    active = index == trail.lastIndex,
                    tag = TestTags.documentCrumb(crumb.id),
                    onClick = { onCrumb(index) },
                )
            }
        }
        Spacer(Modifier.width(8.dp))
        // The gallery is an icon rather than a second labelled button: two of
        // those side by side on a phone leave the breadcrumb trail no room, and
        // starting from a blank page is the commoner of the two.
        IonIconButton(
            Ion.ALBUMS,
            onClick = onTemplates,
            boxSize = 40.dp,
            modifier = Modifier.testTag(TestTags.DOCUMENTS_TEMPLATES_OPEN),
        )
        TButton(
            stringResource(R.string.docs_create),
            onClick = onCreate,
            icon = Ion.ADD,
            enabled = !busy,
            modifier = Modifier.testTag(TestTags.DOCUMENTS_CREATE),
        )
    }
}

@Composable
private fun Crumb(title: String, active: Boolean, tag: String, onClick: () -> Unit) {
    val c = Tessera.colors
    Text(
        title,
        color = if (active) c.text1 else c.primary,
        fontSize = 13.sp,
        fontWeight = if (active) FontWeight.SemiBold else FontWeight.Normal,
        maxLines = 1,
        modifier = Modifier
            .testTag(tag)
            .clip(RoundedCornerShape(RadiusMd))
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 4.dp, vertical = 4.dp),
    )
}

/**
 * The empty state. Inside a container it says the level is empty rather than
 * that the workspace has no documents — the hint to create one is the same, but
 * «документов пока нет» under a document that plainly exists reads as a bug.
 */
@Composable
private fun DocumentsEmpty(nested: Boolean) {
    val c = Tessera.colors
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            IonIcon(Ion.BOOK, size = 40.dp, tint = c.text3)
            Spacer(Modifier.height(10.dp))
            Text(
                stringResource(if (nested) R.string.docs_empty_nested else R.string.docs_empty),
                color = c.text3,
                fontSize = 14.sp,
            )
            Spacer(Modifier.height(4.dp))
            Text(stringResource(R.string.docs_empty_hint), color = c.placeholder, fontSize = 12.sp)
        }
    }
}

/** Emoji or fallback glyph + title, the server's preview, date and nesting. */
@Composable
private fun DocumentTile(doc: Document, nested: Int, onClick: () -> Unit) {
    val c = Tessera.colors
    Column(
        Modifier
            .fillMaxWidth()
            .heightIn(min = TILE_MIN_HEIGHT)
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.cardSurface)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
            .clickableNoRipple(onClick = onClick)
            .padding(12.dp)
            .testTag(TestTags.documentRow(doc.id)),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            // `icon` is an emoji on the web; fall back to the section's own glyph.
            if (doc.icon.isNotBlank()) {
                Text(doc.icon, fontSize = 16.sp)
            } else {
                IonIcon(Ion.BOOK, size = 16.dp, tint = c.text3)
            }
            Spacer(Modifier.width(8.dp))
            Text(
                doc.title.ifBlank { stringResource(R.string.docs_untitled) },
                color = c.text1,
                fontSize = 14.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Spacer(Modifier.height(6.dp))
        Text(
            doc.preview.replace("\n", " ").ifBlank { stringResource(R.string.docs_empty_preview) },
            color = c.text3,
            fontSize = 12.sp,
            maxLines = 3,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f, fill = false),
        )
        Spacer(Modifier.height(8.dp))
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(shortDate(LocalResources.current, doc.updatedAt), color = c.placeholder, fontSize = 11.sp)
            if (nested > 0) {
                Spacer(Modifier.weight(1f))
                Text(
                    stringResource(R.string.docs_nested_count, nested),
                    color = c.placeholder,
                    fontSize = 11.sp,
                )
            }
        }
    }
}

private val TILE_MIN_WIDTH = 150.dp
private val TILE_MIN_HEIGHT = 110.dp
