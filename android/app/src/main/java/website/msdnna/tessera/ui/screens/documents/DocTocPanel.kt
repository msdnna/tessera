package website.msdnna.tessera.ui.screens.documents

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.DocTocRow
import website.msdnna.tessera.util.Ion

/**
 * The document's outline (#2733, §3 of #2894).
 *
 * A panel over the reader rather than beside it: the web sidebar has a column to
 * spare and a phone does not, and an outline is read once — to jump — not kept
 * open alongside the text.
 *
 * A heading with no text still gets a row. A heading is created empty and typed
 * into, and dropping it until its first character would make the list jump; the
 * row is labelled «Без заголовка» and jumps to the same place.
 */
@Composable
internal fun DocTocPanel(rows: List<DocTocRow>, onDismiss: () -> Unit, onJump: (DocTocRow) -> Unit) {
    val c = Tessera.colors
    BackHandler(enabled = true) { onDismiss() }
    Column(
        Modifier.fillMaxSize().background(c.surface).testTag(TestTags.DOCUMENT_TOC),
    ) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                stringResource(R.string.docs_toc_title),
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.padding(start = 8.dp).weight(1f),
            )
            IonIconButton(
                Ion.CLOSE,
                onClick = onDismiss,
                boxSize = 40.dp,
                modifier = Modifier.testTag(TestTags.DOCUMENT_TOC_CLOSE),
            )
        }
        HorizontalDivider(color = c.border)

        if (rows.isEmpty()) {
            Box(Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
                Text(stringResource(R.string.docs_toc_empty), color = c.text3, fontSize = 14.sp)
            }
            return@Column
        }

        LazyColumn(Modifier.fillMaxSize()) {
            items(rows, key = { it.id }) { row ->
                Row(
                    Modifier.fillMaxWidth()
                        .clickableNoRipple { onJump(row) }
                        .padding(
                            // The nesting is a left offset rather than a nested
                            // list, so a deeply nested heading is not squeezed to
                            // nothing on a narrow panel.
                            start = (16 + row.depth * 14).dp,
                            end = 16.dp,
                            top = 10.dp,
                            bottom = 10.dp,
                        )
                        .testTag(TestTags.documentTocRow(row.id)),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        "H${row.level}",
                        color = c.text3,
                        fontSize = 10.sp,
                        fontWeight = FontWeight.SemiBold,
                    )
                    Spacer(Modifier.width(10.dp))
                    Text(
                        row.text.ifBlank { stringResource(R.string.docs_toc_untitled) },
                        color = if (row.depth == 0) c.text1 else c.text2,
                        fontSize = if (row.depth == 0) 15.sp else 14.sp,
                        fontWeight = if (row.depth == 0) FontWeight.Medium else FontWeight.Normal,
                        maxLines = 2,
                    )
                }
            }
        }
    }
}
