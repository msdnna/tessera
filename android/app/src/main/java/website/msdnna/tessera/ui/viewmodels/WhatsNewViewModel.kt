package website.msdnna.tessera.ui.viewmodels

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import website.msdnna.tessera.BuildConfig
import website.msdnna.tessera.data.model.User
import website.msdnna.tessera.update.WhatsNewEntries
import website.msdnna.tessera.update.WhatsNewRepository
import website.msdnna.tessera.util.SPOTLIGHT_PREFIX
import website.msdnna.tessera.util.WHATSNEW_PREFIX
import website.msdnna.tessera.util.WhatsNewEntry
import website.msdnna.tessera.util.WhatsNewSpotlight
import website.msdnna.tessera.util.planWhatsNew

/**
 * Drives the post-update "What's New" card, the sidebar spotlight hints and the
 * API-version label (#2766). The decision of *what* to show is pure
 * ([planWhatsNew]); this holds the session state and writes the acknowledgements.
 *
 * Order is deliberate: the card first, hints one at a time only once it is gone —
 * an arrow drawn behind a modal points at nothing.
 */
class WhatsNewViewModel(app: Application) : AndroidViewModel(app) {
    private val repo = WhatsNewRepository()

    /** Everything acknowledged this session (server keys + optimistic writes). */
    private val acked = mutableSetOf<String>()

    private val _releases = MutableStateFlow<List<WhatsNewEntry>>(emptyList())
    val releases: StateFlow<List<WhatsNewEntry>> = _releases.asStateFlow()

    private val _queue = MutableStateFlow<List<WhatsNewSpotlight>>(emptyList())

    private val _apiVersion = MutableStateFlow("")
    val apiVersion: StateFlow<String> = _apiVersion.asStateFlow()

    /** The hint to draw right now, or null while the card is still up. */
    val spotlight: StateFlow<WhatsNewSpotlight?> =
        combine(_releases, _queue) { releases, queue -> if (releases.isEmpty()) queue.firstOrNull() else null }
            .stateIn(viewModelScope, SharingStarted.Eagerly, null)

    /** Guards against re-running on every recomposition / user-flow emission. */
    private var loadedFor: String? = null

    /** Loads the acks for [user] and decides what to surface. Safe to call often. */
    fun load(user: User?) {
        val id = user?.id?.takeIf { it.isNotBlank() } ?: return
        if (loadedFor == id) return
        loadedFor = id
        viewModelScope.launch {
            _apiVersion.value = runCatching { repo.apiVersion() }.getOrDefault("")
            // Offline / unauthorised: surface nothing rather than risk showing a
            // release the user already dismissed elsewhere.
            val keys = runCatching { repo.acknowledged() }.getOrNull() ?: return@launch
            acked += keys
            val plan = planWhatsNew(
                entries = WhatsNewEntries,
                acked = keys,
                currentVersion = BuildConfig.VERSION_NAME,
                accountCreatedAt = user.createdAt,
                buildAtMillis = buildArrivedAt(),
            )
            plan.baseline?.let {
                ack(it) // brand-new account: nothing to catch up on, just set the mark
                return@launch
            }
            _releases.value = plan.releases
            _queue.value = plan.spotlights
        }
    }

    /** "Понятно" on the card: every shown release is seen, and the baseline moves
     *  to the running build so a later downgrade doesn't replay the backlog. */
    fun dismissCard() {
        val keys = _releases.value.map { WHATSNEW_PREFIX + it.version } + (WHATSNEW_PREFIX + BuildConfig.VERSION_NAME)
        _releases.value = emptyList()
        viewModelScope.launch { keys.forEach { ack(it) } }
    }

    /** "Понятно" on a hint: drop it and advance to the next queued one. */
    fun dismissSpotlight(navKey: String) {
        _queue.value = _queue.value.filterNot { it.navKey == navKey }
        viewModelScope.launch { ack(SPOTLIGHT_PREFIX + navKey) }
    }

    /** Optimistic: the local mark hides it for this session even if the write
     *  fails; a lost write only means it may reappear next launch. */
    private suspend fun ack(key: String) {
        if (!acked.add(key)) return
        runCatching { repo.ack(key) }
    }

    /**
     * When this build landed on the device — the Android stand-in for the web
     * bundle's build date (there is no build timestamp in `BuildConfig`, and one
     * would change on every compile). `lastUpdateTime` is exactly the moment the
     * user updated *into* the running version.
     */
    private fun buildArrivedAt(): Long = runCatching {
        val app = getApplication<Application>()
        app.packageManager.getPackageInfo(app.packageName, 0).lastUpdateTime
    }.getOrDefault(0L)
}
