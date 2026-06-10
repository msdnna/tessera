# Project-specific R8 / ProGuard rules. Library rules ship inside their AARs
# (Compose, Retrofit 3, OkHttp 5, Coil 2, Coroutines) and aren't repeated here.

# ── Gson model classes ──────────────────────────────────────────────────────
# API request/response types are (de)serialized via reflection by Gson — keep
# names + fields so @SerializedName mappings resolve and default-arg
# constructors stay reachable.
-keep class website.msdnna.tessera.data.model.** { *; }
-keepclassmembers class website.msdnna.tessera.data.model.** { <init>(...); }

-keepattributes Signature, InnerClasses, EnclosingMethod
-keepattributes RuntimeVisibleAnnotations, AnnotationDefault
-keepclassmembers class * {
    @com.google.gson.annotations.SerializedName <fields>;
}

# ── Retrofit ApiService ─────────────────────────────────────────────────────
-keep class website.msdnna.tessera.data.api.ApiService { *; }
-keepclassmembers,allowshrinking,allowobfuscation interface website.msdnna.tessera.data.api.ApiService {
    <methods>;
}
