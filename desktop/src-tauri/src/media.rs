// Media (camera / microphone / screen share) support for the desktop webview — #2929.
//
// Conferences are a *webview* capability, not a Tauri IPC one, so nothing here goes
// through capabilities/default.json. Two separate things have to be fixed, in order:
//
//  1. Linux: WebKitGTK ships `enable-media-stream` OFF and wry never touches it, so
//     `navigator.mediaDevices` does not exist at all in the desktop build. The frontend
//     detects that and disables conferences outright (see
//     frontend/src/composables/useConfTransport.js). Turning the settings on is the
//     actual fix — without it there is nothing to grant, because no permission request
//     is ever raised.
//  2. Both platforms: once the API exists, the webview asks for permission. WebKitGTK
//     denies by default; WebView2 denies anything it has no UI for. We auto-grant
//     camera/mic for our own origin — this is our app talking to the user's own server,
//     and a modal on every call is pure friction. Screen capture still goes through the
//     OS picker (xdg-desktop-portal on Linux, the WebView2 picker on Windows), so the
//     user always confirms *what* is shared.
//
// Everything is best-effort and logs to stderr on failure: a desktop that cannot do
// video calls must still start.

use tauri::{Manager, Runtime};

/// Origins the webview legitimately serves our own frontend from. `tauri://localhost`
/// is the production custom scheme on Linux, `http://tauri.localhost` its Windows
/// equivalent, and the localhost pair is `tauri dev` against the Vite server (:5174).
/// No third-party content is ever loaded in this window, but the check costs nothing
/// and keeps a future in-app browser from inheriting the camera.
const TRUSTED_ORIGIN_PREFIXES: &[&str] = &[
    "tauri://localhost",
    "http://tauri.localhost",
    "https://tauri.localhost",
    "http://localhost:5174",
    "http://127.0.0.1:5174",
];

fn is_trusted_origin(uri: &str) -> bool {
    TRUSTED_ORIGIN_PREFIXES.iter().any(|prefix| {
        // A bare `starts_with` would also accept `https://tauri.localhost.evil.tld/`,
        // so the origin has to end exactly where the prefix does.
        uri.strip_prefix(prefix)
            .is_some_and(|rest| rest.is_empty() || rest.starts_with(['/', '?', '#']))
    })
}

/// Wire media settings and permission handling into the app's main webview.
///
/// Called from `setup(...)`. `with_webview` hands the native handle over on the UI
/// thread, so the closure must be `Send` and do its work there.
pub fn setup<R: Runtime>(app: &tauri::AppHandle<R>) {
    let Some(window) = app.get_webview_window("main") else {
        eprintln!("tessera: no main webview window — media permissions not wired");
        return;
    };
    if let Err(e) = window.with_webview(|webview| {
        configure(&webview);
    }) {
        eprintln!("tessera: could not reach the native webview for media setup: {e}");
    }
}

#[cfg(any(
    target_os = "linux",
    target_os = "dragonfly",
    target_os = "freebsd",
    target_os = "netbsd",
    target_os = "openbsd"
))]
fn configure(webview: &tauri::webview::PlatformWebview) {
    use webkit2gtk::glib::object::Cast;
    use webkit2gtk::{
        PermissionRequestExt, SettingsExt, UserMediaPermissionRequestExt, WebViewExt,
    };

    let view = webview.inner();

    // The real fix. Without `enable-media-stream` the whole `navigator.mediaDevices`
    // namespace is absent; `enable-webrtc` gates RTCPeerConnection (WebKitGTK 2.38+),
    // and media-capabilities is what lets LiveKit negotiate codecs instead of guessing.
    if let Some(settings) = WebViewExt::settings(&view) {
        settings.set_enable_media_stream(true);
        settings.set_enable_webrtc(true);
        settings.set_enable_media_capabilities(true);
    } else {
        eprintln!("tessera: WebKit settings unavailable — conferences will stay disabled");
    }

    view.connect_permission_request(|view, request| {
        let uri = view.uri().map(|u| u.to_string()).unwrap_or_default();

        if let Some(media) = request.downcast_ref::<webkit2gtk::UserMediaPermissionRequest>() {
            // Audio/video are the properties WebKitGTK exposes; a request that is
            // neither is a display-capture request (getDisplayMedia). We grant all
            // three for our own origin — screen capture is still gated by the
            // xdg-desktop-portal picker, which is the dialog that actually matters.
            let (audio, video) = (media.is_for_audio_device(), media.is_for_video_device());
            let allowed = is_trusted_origin(&uri);
            if allowed {
                media.allow();
            } else {
                media.deny();
            }
            // Logged unconditionally: when a call comes up silent this line is the
            // difference between "the grant never happened" and "the portal said no".
            eprintln!(
                "tessera: user-media request (audio={audio}, video={video}) from {uri} -> {}",
                if allowed { "allowed" } else { "denied" }
            );
            return true;
        }

        // enumerateDevices() returns nameless entries until this is granted, which
        // leaves the device pickers in the call UI empty even when capture works.
        if let Some(info) = request.downcast_ref::<webkit2gtk::DeviceInfoPermissionRequest>() {
            if is_trusted_origin(&uri) {
                info.allow();
            } else {
                info.deny();
            }
            return true;
        }

        // Anything else (geolocation, notifications, pointer lock) keeps WebKit's
        // default handling — we deliberately do not widen the grant.
        false
    });
}

#[cfg(windows)]
fn configure(webview: &tauri::webview::PlatformWebview) {
    use webview2_com::Microsoft::Web::WebView2::Win32::{
        COREWEBVIEW2_PERMISSION_KIND, COREWEBVIEW2_PERMISSION_KIND_CAMERA,
        COREWEBVIEW2_PERMISSION_KIND_MICROPHONE, COREWEBVIEW2_PERMISSION_STATE_ALLOW,
    };
    use webview2_com::{take_pwstr, PermissionRequestedEventHandler};
    use windows::core::PWSTR;

    let controller = webview.controller();
    let core = match unsafe { controller.CoreWebView2() } {
        Ok(core) => core,
        Err(e) => {
            eprintln!("tessera: no ICoreWebView2 — media permissions not wired: {e}");
            return;
        }
    };

    // wry already registers its own PermissionRequested handler (clipboard-read only);
    // WebView2 supports several subscribers, so this one is added alongside it rather
    // than replacing it. Screen capture has no PermissionKind of its own — WebView2
    // shows its native picker instead — so there is nothing to grant for it here.
    let mut token = Default::default();
    let result = unsafe {
        core.add_PermissionRequested(
            &PermissionRequestedEventHandler::create(Box::new(|_, args| {
                let Some(args) = args else { return Ok(()) };

                let mut kind = COREWEBVIEW2_PERMISSION_KIND::default();
                args.PermissionKind(&mut kind)?;
                if kind != COREWEBVIEW2_PERMISSION_KIND_CAMERA
                    && kind != COREWEBVIEW2_PERMISSION_KIND_MICROPHONE
                {
                    return Ok(());
                }

                let mut uri = PWSTR::null();
                args.Uri(&mut uri)?;
                let uri = take_pwstr(uri);
                if is_trusted_origin(&uri) {
                    args.SetState(COREWEBVIEW2_PERMISSION_STATE_ALLOW)?;
                } else {
                    eprintln!("tessera: media request left to default for origin {uri}");
                }

                Ok(())
            })),
            &mut token,
        )
    };
    if let Err(e) = result {
        eprintln!("tessera: could not hook PermissionRequested: {e}");
    }
}

// macOS/iOS have no bundled target yet (#2895 tracks that); WKWebView needs Info.plist
// usage descriptions rather than a runtime hook, so there is nothing to do here.
#[cfg(any(target_os = "macos", target_os = "ios"))]
fn configure(_webview: &tauri::webview::PlatformWebview) {}

#[cfg(test)]
mod tests {
    use super::is_trusted_origin;

    #[test]
    fn own_origins_are_trusted() {
        assert!(is_trusted_origin("tauri://localhost/"));
        assert!(is_trusted_origin("http://tauri.localhost/index.html"));
        assert!(is_trusted_origin("http://localhost:5174/"));
    }

    #[test]
    fn foreign_origins_are_not() {
        assert!(!is_trusted_origin("https://example.com/"));
        assert!(!is_trusted_origin("http://localhost:5175/"));
        // A lookalike host must not pass the prefix check.
        assert!(!is_trusted_origin("https://tauri.localhost.evil.tld/"));
    }
}
