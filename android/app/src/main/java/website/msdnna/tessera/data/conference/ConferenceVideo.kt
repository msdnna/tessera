package website.msdnna.tessera.data.conference

import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.viewinterop.AndroidView
import io.livekit.android.renderer.TextureViewRenderer
import io.livekit.android.room.track.VideoTrack

/**
 * A participant's live picture (#2896 §5).
 *
 * The one composable that lives down here with the media core rather than in
 * `ui/`, and for the same reason [ConferenceEngine] exists at all: rendering
 * needs the SDK's own renderer and the room's EGL context, and keeping that
 * here is what lets the room screen be written — and mounted in a spec —
 * without `io.livekit` anywhere near it.
 *
 * Callers must not mount this for a peer with no picture: the tile decides
 * between video and an avatar, and a renderer created for a null track is an
 * EGL surface allocated to show nothing.
 */
@Composable
fun ConfVideo(peer: ConfPeer, screen: Boolean, modifier: Modifier = Modifier) {
    val track = if (screen) peer.screenTrack else peer.videoTrack

    // What this particular view is currently bound to. Held in a box rather than
    // derived from `track` because unbinding needs the *previous* value, and the
    // same camera track is legitimately on two views at once — a presenter's face
    // in the strip while their screen holds the stage — so detaching by track
    // alone would blank the other tile.
    val bound = remember { arrayOfNulls<VideoTrack>(1) }

    AndroidView(
        modifier = modifier,
        factory = { ctx ->
            TextureViewRenderer(ctx).also { view ->
                ConferenceEngine.attachRenderer(view)
                // Our own camera, seen the way a mirror shows it. Everyone else
                // is left alone: mirroring a remote speaker reverses the text on
                // whatever they are holding up.
                view.setMirror(peer.local && !screen)
            }
        },
        update = { view ->
            val current = bound[0]
            if (current !== track) {
                current?.removeRenderer(view)
                track?.addRenderer(view)
                bound[0] = track
            }
            view.setMirror(peer.local && !screen)
        },
        onRelease = { view ->
            // A view that goes away still attached leaves the SDK believing
            // somebody is watching, and adaptiveStream keeps paying for a stream
            // nobody can see — on somebody's mobile data.
            bound[0]?.removeRenderer(view)
            bound[0] = null
            view.release()
        },
    )
}
