package website.msdnna.tessera.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * The app's own design system. Material3 is wrapped only as a thin host
 * (ripple, text selection, base typography). All colours come from
 * [LocalTessera] — read it via the [Tessera] accessor, never MaterialTheme.
 */
val LocalTessera = staticCompositionLocalOf { LightPalette }

object Tessera {
    val colors: TesseraColors
        @Composable get() = LocalTessera.current
}

/** Shared corner radius — matches the web's `borderRadius: 8px`. */
val RadiusSm = 6.dp
val RadiusMd = 8.dp
val RadiusLg = 12.dp

private val TesseraTypography = Typography(
    headlineSmall = TextStyle(fontSize = 20.sp, fontWeight = FontWeight.SemiBold),
    titleLarge = TextStyle(fontSize = 18.sp, fontWeight = FontWeight.SemiBold),
    titleMedium = TextStyle(fontSize = 15.sp, fontWeight = FontWeight.Medium),
    bodyLarge = TextStyle(fontSize = 14.sp),
    bodyMedium = TextStyle(fontSize = 13.sp),
    bodySmall = TextStyle(fontSize = 12.sp),
    labelSmall = TextStyle(fontSize = 11.sp),
)

@Composable
fun TesseraTheme(
    accent: AccentTheme = AccentThemes[0],
    isDark: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    val base = if (isDark) DarkPalette else LightPalette
    val colors = base.copy(primary = accent.primary, onPrimary = accent.onPrimary)

    // Keep a Material colorScheme in sync. Crucially, override EVERY surface*
    // token to our own neutral surfaces — otherwise M3 derives tonally-tinted
    // surfaceContainer* values from `primary`, giving menus/dialogs/date-pickers
    // a coloured (lavender/pink) cast.
    val m3Base = if (isDark) darkColorScheme() else lightColorScheme()
    val m3 = m3Base.copy(
        primary = accent.primary,
        onPrimary = accent.onPrimary,
        background = colors.bg,
        onBackground = colors.text1,
        surface = colors.surface,
        onSurface = colors.text1,
        surfaceVariant = colors.surfaceAlt,
        onSurfaceVariant = colors.text2,
        surfaceTint = colors.surface,
        surfaceBright = colors.surface,
        surfaceDim = colors.surface,
        surfaceContainerLowest = colors.surface,
        surfaceContainerLow = colors.surface,
        surfaceContainer = colors.surface,
        surfaceContainerHigh = colors.surface,
        surfaceContainerHighest = colors.surface,
        outline = colors.border,
        outlineVariant = colors.border,
    )

    CompositionLocalProvider(LocalTessera provides colors) {
        MaterialTheme(colorScheme = m3, typography = TesseraTypography, content = content)
    }
}
