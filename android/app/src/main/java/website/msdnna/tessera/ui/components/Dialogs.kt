package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.Tessera

/** Themed modal shell — a centred surface card used by the dialogs below. */
@Composable
private fun DialogShell(onDismiss: () -> Unit, content: @Composable () -> Unit) {
    val c = Tessera.colors
    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(RadiusLg))
                .background(c.surface)
                .padding(20.dp),
        ) { content() }
    }
}

/** Single-line text-input dialog (create / rename). */
@Composable
fun TInputDialog(
    title: String,
    initial: String = "",
    confirmText: String = "Сохранить",
    placeholder: String = "",
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    var text by remember { mutableStateOf(initial) }
    DialogShell(onDismiss) {
        Text(title, color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(14.dp))
        TTextField(value = text, onValueChange = { text = it }, placeholder = placeholder)
        Spacer(Modifier.height(18.dp))
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            TButton("Отмена", kind = TButtonKind.Ghost, onClick = onDismiss)
            Spacer(Modifier.width(8.dp))
            TButton(
                confirmText,
                enabled = text.isNotBlank(),
                onClick = { if (text.isNotBlank()) onConfirm(text.trim()) },
            )
        }
    }
}

/** Confirm / cancel dialog (destructive actions). */
@Composable
fun TConfirmDialog(
    title: String,
    message: String,
    confirmText: String = "Удалить",
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    DialogShell(onDismiss) {
        Text(title, color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(10.dp))
        Text(message, color = c.text2, fontSize = 14.sp)
        Spacer(Modifier.height(18.dp))
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            TButton("Отмена", kind = TButtonKind.Ghost, onClick = onDismiss)
            Spacer(Modifier.width(8.dp))
            TButton(confirmText, onClick = onConfirm)
        }
    }
}
