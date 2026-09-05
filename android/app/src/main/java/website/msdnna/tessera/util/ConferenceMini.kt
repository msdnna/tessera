package website.msdnna.tessera.util

/*
 * The rules the minimised call runs on (#2896 §9, web `ConferenceMiniWindow`).
 *
 * Up to §8 the call and the room screen were the same thing: leaving the screen
 * ended the call, because there was nowhere else for it to be. The bar is that
 * somewhere — so these are the rules for a call the user cannot see, which is
 * exactly when getting them wrong is invisible.
 */

/** What the bar says about a call nobody is looking at. */
enum class ConfMiniLine {
    /** Token or SFU connection in flight. */
    CONNECTING,

    /** The SDK is re-establishing the connection; the call is not over. */
    RECONNECTING,

    LIVE,

    /**
     * The connection failed while the call was off-screen.
     *
     * Kept as a state of the bar rather than dropping it: a bar that vanished
     * would end the call from the user's side without ever saying so, and the
     * one control that can fix it — «Повторить» — lives on the room screen the
     * bar is the way back to.
     */
    LOST,
}

/**
 * What the minimised bar shows, or null for no bar at all.
 *
 * [roomOnScreen] is the room composable's own presence, not a destination
 * check: the section, the lobby and the call are three layers of one screen, and
 * a bar keyed to «is the user in the conferences section» would draw itself over
 * the very call it is a shortcut to.
 *
 * The two statuses without a bar are the two with nothing behind it:
 * [ConfMediaStatus.IDLE] is where hanging up and an ended call both land, and
 * [ConfMediaStatus.UNAVAILABLE] is an install without an SFU — tapping either
 * would return the user to a room that is not there.
 */
fun confMiniLine(
    status: ConfMediaStatus,
    reconnecting: Boolean,
    roomOnScreen: Boolean,
): ConfMiniLine? = when {
    roomOnScreen -> null

    status == ConfMediaStatus.ERROR -> ConfMiniLine.LOST

    // Before the status check, not after: a reconnect keeps the session LIVE and
    // a bar that read it off the status alone would promise a call that is
    // currently not carrying a word.
    reconnecting && status != ConfMediaStatus.IDLE -> ConfMiniLine.RECONNECTING

    status == ConfMediaStatus.CONNECTING -> ConfMiniLine.CONNECTING

    status == ConfMediaStatus.LIVE -> ConfMiniLine.LIVE

    else -> null
}

/**
 * How many other people are in the call.
 *
 * Ourselves excluded: «3» on a bar that means «you and two others» is the kind
 * of off-by-one nobody reports and everybody notices. Counted off the peers
 * rather than off the roster because this is the media call — somebody invited
 * and still ringing (#2875) is not in it yet.
 */
fun confMiniOthers(peers: List<ConfStagePerson>): Int = peers.count { !it.local }

/**
 * Whether the bar's own microphone button may be pressed.
 *
 * Deliberately [confControls]'s answer rather than a second one: the toolbar and
 * the bar are two surfaces onto one microphone, and a force-mute (#2878) that
 * reached only one of them would leave the button that is currently on screen
 * silently doing nothing. The whole reason §6 replaced its second copy of the
 * queue rule was that two descriptions of one rule drift apart quietly.
 */
fun confMiniMicEnabled(status: ConfMediaStatus, forceMuted: Boolean): Boolean =
    confControls(status, micOn = false, camOn = false, forceMuted = forceMuted).micEnabled

/**
 * Whether tapping the bar can put the user back in the call.
 *
 * Every line the bar has is worth returning to — including [ConfMiniLine.LOST],
 * which is the one that needs the room most: the retry is there.
 */
fun confMiniReturnable(line: ConfMiniLine?): Boolean = line != null
