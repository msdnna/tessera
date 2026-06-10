package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera

/**
 * Inline single-line creator used for new cards / subtasks. Auto-focuses; a
 * non-blank value commits on Enter or on focus loss (tap elsewhere); a blank
 * value just dismisses. No buttons — mirrors the web's inline inputs.
 */
@Composable
fun InlineCreateField(
    placeholder: String,
    onCommit: (String) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
    autoFocus: Boolean = true,
) {
    val c = Tessera.colors
    val focus = remember { FocusRequester() }
    var text by remember { mutableStateOf("") }
    var finished by remember { mutableStateOf(false) }
    // Only treat a focus loss as "commit/cancel" once the field has actually
    // been focused — otherwise the initial unfocused state fires finish()
    // immediately and the field vanishes before the user can type.
    var hadFocus by remember { mutableStateOf(false) }

    fun finish() {
        if (finished) return
        finished = true
        val t = text.trim()
        if (t.isEmpty()) onDismiss() else onCommit(t)
    }

    LaunchedEffect(Unit) { if (autoFocus) focus.requestFocus() }

    BasicTextField(
        value = text,
        onValueChange = { text = it },
        singleLine = true,
        textStyle = TextStyle(color = c.text1, fontSize = 14.sp),
        cursorBrush = SolidColor(c.text2),
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
        keyboardActions = KeyboardActions(onDone = { finish() }),
        modifier = modifier
            .fillMaxWidth()
            .focusRequester(focus)
            .onFocusChanged {
                if (it.isFocused) hadFocus = true else if (hadFocus) finish()
            }
            .clip(RoundedCornerShape(RadiusSm))
            .background(c.surface)
            .border(1.dp, c.border, RoundedCornerShape(RadiusSm))
            .padding(horizontal = 11.dp, vertical = 10.dp),
        decorationBox = { inner ->
            Box(contentAlignment = Alignment.CenterStart) {
                if (text.isEmpty()) Text(placeholder, color = c.placeholder, fontSize = 14.sp)
                inner()
            }
        },
    )
}
