package website.msdnna.tessera.ui.screens

import android.content.Context
import android.content.Intent
import androidx.core.content.FileProvider
import java.io.File

/**
 * Hands a file we have already downloaded to whatever on the phone can open it.
 *
 * Its own file since #2896 §8, when the third caller appeared: the task modal's
 * attachments, the in-call chat's, and now the lobby's recordings all go out
 * through the one `FileProvider` authority, and three copies of that string is
 * three places for it to drift out of step with the manifest.
 *
 * [chooserTitle] is resolved by the caller in composition on purpose — the
 * chooser is shown from outside it, where `ctx.getString` would pick the
 * system's language over the profile's.
 *
 * [mime] is what the row said it was, and the wildcard type when it said
 * nothing: an empty type offers no apps at all, which reads as a download that
 * failed rather than as one nothing on the phone can open.
 */
fun openDownloadedFile(ctx: Context, file: File, mime: String?, chooserTitle: String) {
    val uri = FileProvider.getUriForFile(ctx, "${ctx.packageName}.fileprovider", file)
    val view = Intent(Intent.ACTION_VIEW).apply {
        setDataAndType(uri, mime?.takeIf { it.isNotBlank() } ?: "*/*")
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
    }
    val chooser = Intent.createChooser(view, chooserTitle)
        .apply { addFlags(Intent.FLAG_ACTIVITY_NEW_TASK) }
    // A phone with nothing that opens mp4 throws rather than returning; the
    // download is on disk either way, so this is not worth a crash.
    runCatching { ctx.startActivity(chooser) }
}
