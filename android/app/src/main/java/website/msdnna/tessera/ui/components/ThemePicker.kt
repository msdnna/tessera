package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.theme.AccentThemes
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.Ion

/** Accent-colour + dark-mode picker, mirroring the web's palette popover. */
@Composable
fun ThemePicker(
    currentAccent: String,
    isDark: Boolean,
    onSelectAccent: (String) -> Unit,
    onToggleDark: () -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.popupAppear(TransformOrigin.Center).fillMaxWidth().clip(RoundedCornerShape(RadiusLg)).background(c.surface).padding(20.dp),
        ) {
            Text(stringResource(R.string.theme_picker_title), color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
            Spacer(Modifier.height(16.dp))
            Text(stringResource(R.string.theme_picker_accent), color = c.text3, fontSize = 12.sp)
            Spacer(Modifier.height(10.dp))
            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(12.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                AccentThemes.forEach { accent ->
                    val selected = accent.key == currentAccent
                    Box(
                        Modifier
                            .size(34.dp)
                            .clip(CircleShape)
                            .background(accentGradient(accent.primary))
                            .then(if (selected) Modifier.border(2.dp, c.text1, CircleShape) else Modifier)
                            .clickableNoRipple { onSelectAccent(accent.key) },
                        contentAlignment = Alignment.Center,
                    ) {
                        if (selected) IonIcon(Ion.CHECK, size = 18.dp, tint = accent.onPrimary)
                    }
                }
            }
            Spacer(Modifier.height(20.dp))
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text(stringResource(R.string.theme_picker_dark), color = c.text1, fontSize = 14.sp, modifier = Modifier.weight(1f))
                Switch(
                    checked = isDark,
                    onCheckedChange = { onToggleDark() },
                    colors = SwitchDefaults.colors(
                        checkedThumbColor = Color.White,
                        checkedTrackColor = c.primary,
                        uncheckedTrackColor = c.surfaceAlt,
                        uncheckedBorderColor = c.border,
                    ),
                )
            }
        }
    }
}
