package website.msdnna.tessera.ui.components

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.ui.theme.Tessera

/**
 * A board counter with its explanation (web parity for the `title=` tooltips on the
 * column header and the composer bar, #2850/#2851).
 *
 * The web counter explains itself on hover; a touch screen has no hover, so the
 * explanation opens on a **long press** — a plain tap stays free (the header's tap
 * targets around it, e.g. rename, keep working). The popover is the app's own
 * [TDropdown] rather than a Material tooltip so it carries the same surface/border
 * as every other popover here.
 */
@Composable
fun CountWithHint(
    label: String,
    hint: String,
    modifier: Modifier = Modifier,
    color: Color = Tessera.colors.text3,
    fontSize: TextUnit = 13.sp,
) {
    var hintOpen by remember { mutableStateOf(false) }
    // The long press sits on this Box, not on the Text, and that placement is load-
    // bearing for more than the tap target: `combinedClickable` merges semantics, so
    // a clickable Text would become its own merged node and the caller's testTag —
    // which lands on the Box — would end up on a node carrying no text at all. With
    // the gesture here, tag and number are one node: a spec anchors on the tag and
    // reads the label off it. The hint lives in a Popup, i.e. its own composition,
    // so it never leaks into that text.
    Box(
        modifier
            .semantics(mergeDescendants = true) {}
            .combinedClickableNoRipple(onClick = {}, onLongClick = { hintOpen = true }),
    ) {
        Text(label, color = color, fontSize = fontSize, maxLines = 1)
        TDropdown(hintOpen, { hintOpen = false }) {
            Text(
                hint,
                color = Tessera.colors.text2,
                fontSize = 12.sp,
                modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp),
            )
        }
    }
}
