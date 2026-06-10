package website.msdnna.tessera.data.model

/**
 * The `apks/latest.json` manifest the release build publishes (see
 * `tools/build-android-release.sh`). [versionCode] uses the same
 * `major*10000 + minor*100 + patch` formula as `app/build.gradle`.
 *
 * Lives in `data.model` so the R8 keep rule
 * (`-keep class …data.model.** { *; }`) preserves its field names — otherwise a
 * release build renames them and Gson deserialises every field to its default
 * (versionCode 0), and no update is ever detected.
 */
data class LatestRelease(
    val version: String = "",
    val versionCode: Int = 0,
    val apk: String = "",
    val notes: String = "",
)
