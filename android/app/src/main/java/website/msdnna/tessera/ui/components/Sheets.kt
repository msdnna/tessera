package website.msdnna.tessera.ui.components

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.BottomSheetDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import website.msdnna.tessera.ui.theme.Tessera

/**
 * Themed Material3 modal bottom sheet — the mobile equivalent of the web's
 * popover toolbars. Carries the app surface colour and a muted drag handle; the
 * [content] runs in the sheet's [ColumnScope] with comfortable side padding.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TBottomSheet(
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    val c = Tessera.colors
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = c.surface,
        contentColor = c.text1,
        scrimColor = c.text1.copy(alpha = 0.32f),
        dragHandle = { BottomSheetDefaults.DragHandle(color = c.text3) },
    ) {
        Column(
            modifier.fillMaxWidth().padding(horizontal = 18.dp).padding(bottom = 28.dp),
            content = content,
        )
    }
}
