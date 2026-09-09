package website.msdnna.tessera.ui.screens.documents

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmDialog
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.DOC_LOCAL_EXTENSIONS
import website.msdnna.tessera.util.DOC_OFFICE_EXTENSIONS
import website.msdnna.tessera.util.DOC_PDF_EXTENSION
import website.msdnna.tessera.util.DocTemplateCard
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.filterDocTemplates

/**
 * The template gallery, and the way a file becomes a document (#2734, #2733 —
 * §8 of #2894).
 *
 * A full-height panel rather than the web's modal grid of tiles: on a phone a
 * two-column grid of cards with a description in each is three words per line,
 * so the tiles are rows. What they carry is the same — where the template came
 * from, and the one button that uses it.
 *
 * Importing lives here rather than beside «Новый документ» because it is the
 * same question asked twice: the gallery is "start from something that already
 * exists", and a file on the phone is exactly that.
 */
@Composable
fun DocTemplatesSheet(
    cards: List<DocTemplateCard>,
    loading: Boolean,
    importing: Boolean,
    busy: String,
    error: UiText?,
    /** Whether the office half of the picker is offered — see [importHint]. */
    converterAvailable: Boolean,
    converterReason: String,
    onDismiss: () -> Unit,
    onUse: (DocTemplateCard) -> Unit,
    onRemove: (DocTemplateCard) -> Unit,
    onPickFile: () -> Unit,
) {
    val c = Tessera.colors
    var query by remember { mutableStateOf("") }
    var confirmRemove by remember { mutableStateOf<DocTemplateCard?>(null) }
    val shown = remember(cards, query) { filterDocTemplates(cards, query) }

    BackHandler(enabled = true) { onDismiss() }

    Column(Modifier.fillMaxSize().background(c.surface).imePadding().testTag(TestTags.DOCUMENT_TEMPLATES)) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                stringResource(R.string.docs_templates_title),
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.padding(start = 8.dp),
            )
            if (loading) {
                Spacer(Modifier.width(6.dp))
                Text("…", color = c.text3, fontSize = 13.sp)
            }
            Spacer(Modifier.weight(1f))
            IonIconButton(
                Ion.CLOSE,
                onClick = onDismiss,
                boxSize = 40.dp,
                modifier = Modifier.testTag(TestTags.DOCUMENT_TEMPLATES_CLOSE),
            )
        }
        HorizontalDivider(color = c.border)

        Column(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 10.dp)) {
            TTextField(
                value = query,
                onValueChange = { query = it },
                placeholder = stringResource(R.string.docs_templates_search),
                modifier = Modifier.fillMaxWidth(),
                fieldTag = TestTags.DOCUMENT_TEMPLATES_SEARCH,
            )
            Spacer(Modifier.height(8.dp))
            TButton(
                stringResource(R.string.docs_templates_upload),
                onClick = onPickFile,
                kind = TButtonKind.Secondary,
                icon = Ion.ATTACH,
                enabled = !importing,
                loading = importing,
                modifier = Modifier.testTag(TestTags.DOCUMENT_TEMPLATES_UPLOAD),
            )
            Spacer(Modifier.height(4.dp))
            Text(
                importHint(converterAvailable, converterReason),
                color = c.text3,
                fontSize = 11.sp,
                modifier = Modifier.testTag(TestTags.DOCUMENT_IMPORT_HINT),
            )
            error?.let {
                Spacer(Modifier.height(6.dp))
                TFormError(it)
            }
        }
        HorizontalDivider(color = c.border)

        if (shown.isEmpty()) {
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text(
                    stringResource(R.string.docs_templates_empty),
                    color = c.text3,
                    fontSize = 13.sp,
                    modifier = Modifier.padding(24.dp).testTag(TestTags.DOCUMENT_TEMPLATES_EMPTY),
                )
            }
        } else {
            LazyColumn(
                Modifier.fillMaxSize(),
                contentPadding = PaddingValues(12.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                items(shown, key = { it.id }) { card ->
                    TemplateRow(
                        card = card,
                        // Any card being used disables all of them: a second tap
                        // while the first create is in flight makes two documents,
                        // and the gallery closes on the first one.
                        busy = busy,
                        onUse = { onUse(card) },
                        onRemove = { confirmRemove = card },
                    )
                }
            }
        }
    }

    confirmRemove?.let { card ->
        TConfirmDialog(
            title = stringResource(R.string.docs_templates_remove_title),
            message = stringResource(R.string.docs_templates_remove_confirm),
            confirmTag = TestTags.DOCUMENT_TEMPLATE_REMOVE_CONFIRM,
            onConfirm = {
                confirmRemove = null
                onRemove(card)
            },
            onDismiss = { confirmRemove = null },
        )
    }
}

/**
 * What the picker will accept, in one line.
 *
 * PDF and the two locally-parsed formats are named in *both* branches on
 * purpose: they need no sidecar, and leaving them out of the "no converter"
 * hint would hide a feature that works.
 */
@Composable
private fun importHint(converterAvailable: Boolean, reason: String): String {
    val local = (DOC_LOCAL_EXTENSIONS + DOC_PDF_EXTENSION).joinToString(", ")
    return if (converterAvailable) {
        stringResource(
            R.string.docs_import_hint,
            (DOC_OFFICE_EXTENSIONS + DOC_PDF_EXTENSION + DOC_LOCAL_EXTENSIONS).joinToString(", "),
        )
    } else {
        stringResource(
            R.string.docs_import_hint_limited,
            local,
            reason.ifBlank { stringResource(R.string.docs_import_no_converter) },
        )
    }
}

/** One gallery row: emoji or glyph, name, description, where it came from. */
@Composable
private fun TemplateRow(
    card: DocTemplateCard,
    busy: String,
    onUse: () -> Unit,
    onRemove: () -> Unit,
) {
    val c = Tessera.colors
    Column(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(RadiusMd))
            // A built-in is marked by a flat neutral tint, not by an accent: it
            // is not more important than the team's own templates, only
            // differently sourced.
            .background(if (card.builtin) c.surfaceAlt else c.cardSurface)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
            .padding(12.dp)
            .testTag(TestTags.documentTemplateRow(card.id)),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (card.icon.isNotBlank()) {
                Text(card.icon, fontSize = 16.sp)
            } else {
                IonIcon(Ion.DOCUMENT_TEXT, size = 16.dp, tint = c.text3)
            }
            Spacer(Modifier.width(8.dp))
            Text(
                card.title.ifBlank { stringResource(R.string.docs_untitled) },
                color = c.text1,
                fontSize = 14.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
        }
        Spacer(Modifier.height(6.dp))
        Text(
            card.description.ifBlank { stringResource(R.string.docs_templates_no_description) },
            color = c.text3,
            fontSize = 12.sp,
            maxLines = 3,
            overflow = TextOverflow.Ellipsis,
        )
        Spacer(Modifier.height(8.dp))
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(
                if (card.builtin) {
                    stringResource(R.string.docs_templates_builtin)
                } else {
                    card.authorName
                },
                color = c.placeholder,
                fontSize = 11.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            if (!card.builtin) {
                IonIconButton(
                    Ion.TRASH,
                    onClick = onRemove,
                    boxSize = 36.dp,
                    modifier = Modifier.testTag(TestTags.documentTemplateRemove(card.id)),
                )
                Spacer(Modifier.width(4.dp))
            }
            TButton(
                stringResource(R.string.docs_templates_use),
                onClick = onUse,
                enabled = busy.isEmpty(),
                loading = busy == card.id,
                modifier = Modifier.testTag(TestTags.documentTemplateUse(card.id)),
            )
        }
    }
}
