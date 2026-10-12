package website.msdnna.tessera.util

/**
 * Column colour swatches, in the web's order (ColumnHeader.vue `swatches`).
 *
 * The first entry is the EMPTY string — «no colour», the state every column is
 * created in. Without it the palette is a one-way door: `board_columns.color` is
 * an optional field (handlers/boards.go UpdateColumn binds `color` without
 * `required`, so "" clears it) and the whole phone UI already draws a colourless
 * column — [parseHexColor] falls back on a blank, `laneHasColor` turns the
 * gradients off — but nothing on the phone can ever SEND that blank. Tap a
 * colour once and the only way back is the web client.
 */
val ColumnPalette: List<String> = listOf(
    "",
    "#9aa0aa",
    "#7c5cff",
    "#2f80ed",
    "#0eb0a9",
    "#18a058",
    "#f0a020",
    "#e0533d",
    "#eb2f96",
)

/**
 * Does [swatch] show the colour the column currently wears?
 *
 * A column with no colour stores it as NULL on Android (`color` is nullable in
 * the model) and as "" on the web, so the «no colour» swatch has to answer to
 * both — a plain string compare would leave it permanently unselected.
 */
fun columnSwatchSelected(current: String?, swatch: String): Boolean =
    if (swatch.isBlank()) current.isNullOrBlank() else swatch.equals(current?.trim(), ignoreCase = true)
