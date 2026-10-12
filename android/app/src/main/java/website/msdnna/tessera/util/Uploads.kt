package website.msdnna.tessera.util

import android.content.res.Resources
import java.text.NumberFormat
import website.msdnna.tessera.R

/*
 * The rules the two upload paths of a task run on (#2932, web `utils/upload.js`).
 *
 * They are the server's, restated: `handlers/attachments.go` refuses an
 * attachment over 25 MiB ([ATTACHMENT_MAX_BYTES]) and an inline image over 8 MiB
 * ([MEDIA_MAX_BYTES]) or outside the PNG/JPEG/GIF/WebP/BMP sniff allowlist
 * ([MEDIA_TYPES]). Neither picker on the phone had a rule of its own, and the
 * cost of that is higher here than in a browser:
 *
 *  - both pickers read the file into a `ByteArray` before sending it, so a video
 *    from the gallery is spent twice — once on the heap, once on the wire — for a
 *    refusal that was knowable from the file's size alone;
 *  - the refusal, when it arrives, is the handler's hardcoded *Russian* sentence
 *    ("файл больше 25 МБ"), shown verbatim to a reader who chose English;
 *  - the inline-image path drops it entirely (`runCatching { … }.getOrNull()`
 *    twice over), so an over-cap image just never appears, with nothing said.
 *
 * The same answer the conference chat already gives its own picker
 * ([confChatAccept]) and the document import gives its own ([docImportLimit]) —
 * this is those two rules for the third and fourth pickers.
 *
 * Kept out of the composables because a rule that is only ever true inside one is
 * a rule nobody can pin with a test.
 */

/** The server's `maxAttachmentBytes`. */
const val ATTACHMENT_MAX_BYTES = 25L * 1024 * 1024

/** The server's `maxMediaBytes`, for images embedded in a description/comment. */
const val MEDIA_MAX_BYTES = 8L * 1024 * 1024

/** The types the server's `mediaExts` sniff allowlist accepts, in its order. */
val MEDIA_TYPES = listOf("image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp")

/** Why the server would refuse a picked file, decided before it is sent. */
enum class PickRefusal { NONE, TOO_BIG, BAD_TYPE }

/**
 * Whether a file is over a cap the server compares the same bytes against.
 *
 * A size the picker could not read (`null`, or the `-1` a provider answers with
 * when it does not know) passes: the server compares what actually arrives, and a
 * phone that refuses a file on a number it does not have is stricter than the
 * server for no reason the user can act on. The conference chat drops such a file
 * instead, because there the unknown one is picked alongside four others and the
 * whole message fails with it; a single-file picker has no such batch to protect.
 */
private fun overCap(size: Long?, cap: Long): Boolean = size != null && size > cap

/**
 * Why the server would refuse this as a task attachment. Attachments are
 * arbitrary files — the handler stores whatever arrives — so size is the only
 * rule there is.
 */
fun attachmentPickError(size: Long?): PickRefusal =
    if (overCap(size, ATTACHMENT_MAX_BYTES)) PickRefusal.TOO_BIG else PickRefusal.NONE

/**
 * Why the server would refuse this as an inline image.
 *
 * Size is checked first because the handler does, so a 12 MB SVG reads as "too
 * large" on both sides rather than inviting a smaller SVG that would be refused
 * anyway. The type check is deliberately one-sided: it only fires on a type the
 * resolver actually reported and the server certainly rejects — SVG above all,
 * which the picker's `image` wildcard filter lets through and which is absent from
 * the allowlist on purpose (it is a script-bearing document, and /uploads is served
 * on the app's own origin). A URI the resolver types as null is handed over
 * untouched, because the
 * server sniffs the *bytes*; refusing it here would make the picker stricter than
 * the server.
 */
fun mediaPickError(size: Long?, mime: String?): PickRefusal {
    if (overCap(size, MEDIA_MAX_BYTES)) return PickRefusal.TOO_BIG
    // `substringBefore(';')`: a resolver may answer with parameters attached
    // ("image/jpeg; charset=binary"), and the allowlist is of bare types — one-sided
    // means one-sided, so a type we merely failed to normalise must not be refused.
    val type = mime.orEmpty().substringBefore(';').trim().lowercase()
    if (type.isNotEmpty() && type !in MEDIA_TYPES) return PickRefusal.BAD_TYPE
    return PickRefusal.NONE
}

/**
 * How big an uploaded attachment is, for the row that lists it in the task's
 * «Файлы» tab (#2932, web `attachmentSizeText`).
 *
 * The row used to format the number itself, with `String.format(Locale.US, …)`,
 * which is exactly half of the rule — and the half it got right was documented:
 * the unit comes from the catalogue so that «МБ»/«MB» follows the profile. The
 * separator is catalogue business for the same reason, and every other place the
 * product weighs a file says so ([docFileSize], [confFileSize], web
 * `formatFileSize`): one attachment read «1.4 МБ» here and «1,4 МБ» in a document
 * or a call's chat, inside one Russian interface. Kilobytes disagreed about the
 * number as well — `%.0f` rounded 14.6 KB to a flat «15 КБ» — so anything scaled
 * keeps one fraction digit, as it does everywhere else.
 *
 * The locale comes from [res], not `Locale.getDefault()`: `AppLocale` hands the
 * composition resources in the *profile's* language, while the device may well be
 * in another one — taking the separator from the phone and the unit from the
 * profile is the same split-brain in a new place.
 */
fun attachmentSizeText(res: Resources, bytes: Long): String {
    val size = bytes.coerceAtLeast(0)
    val (scale, unit) = when {
        size >= 1L shl 20 -> (1L shl 20) to R.string.task_file_size_mb
        size >= 1L shl 10 -> (1L shl 10) to R.string.task_file_size_kb
        else -> 1L to R.string.task_file_size_b
    }
    val format = NumberFormat.getInstance(res.configuration.locales[0]).apply {
        maximumFractionDigits = if (scale == 1L) 0 else 1
        minimumFractionDigits = 0
    }
    return res.getString(unit, format.format(size / scale.toDouble()))
}
