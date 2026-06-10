package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.parseHexColor

/** Small coloured tag chip, matching the web kanban card chips. */
@Composable
fun TagChip(name: String, color: String, modifier: Modifier = Modifier) {
    val base = parseHexColor(color, Tessera.colors.text3)
    Box(
        modifier
            .clip(RoundedCornerShape(4.dp))
            .background(accentGradient(base.copy(alpha = 0.18f)))
            .padding(horizontal = 6.dp, vertical = 2.dp),
    ) {
        Text(name, fontSize = 11.sp, fontWeight = FontWeight.Medium, style = TextStyle(brush = accentGradient(base)))
    }
}

/** Circular initials avatar for an assignee. */
@Composable
fun AvatarChip(initials: String, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    Box(
        modifier
            .clip(CircleShape)
            .background(c.surfaceAlt)
            .padding(horizontal = 5.dp, vertical = 3.dp),
    ) {
        Text(initials, color = c.text2, fontSize = 10.sp, fontWeight = FontWeight.SemiBold)
    }
}

/** Solid colour dot (column header swatch). */
@Composable
fun ColorDot(color: Color, sizeDp: Int = 9, modifier: Modifier = Modifier) {
    Box(modifier.size(sizeDp.dp).clip(CircleShape).background(accentGradient(color)))
}
