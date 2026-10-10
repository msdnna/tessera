package website.msdnna.tessera.ui.screens.documents

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.DocPins
import website.msdnna.tessera.util.Ion

/**
 * The document head's chips (#3044, #3046) — the stage the document belongs to
 * and the meeting it is the agenda of, as the web draws them under the title.
 * Read-only here: the pickers live on the web (see util/DocPins.kt).
 */
@Composable
internal fun DocPinsRow(pins: DocPins) {
    Row(
        Modifier
            .fillMaxWidth()
            .horizontalScroll(rememberScrollState())
            .padding(start = 16.dp, end = 16.dp, top = 10.dp),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        pins.stage?.let { PinChip(Ion.ROCKET, it, Modifier.testTag(TestTags.DOCUMENT_PIN_STAGE)) }
        pins.meeting?.let { PinChip(Ion.VIDEOCAM, it, Modifier.testTag(TestTags.DOCUMENT_PIN_MEETING)) }
    }
}

@Composable
private fun PinChip(icon: String, label: String, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    Row(
        modifier
            .height(26.dp)
            .clip(RoundedCornerShape(50))
            .background(c.surfaceAlt)
            .padding(start = 8.dp, end = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(icon, size = 12.dp, tint = c.text2)
        Spacer(Modifier.width(6.dp))
        Text(
            label,
            color = c.text2,
            fontSize = 12.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.widthIn(max = 220.dp),
        )
    }
}
