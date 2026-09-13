// Tessera desktop shell (Tauri v2). Hosts the shared Vue frontend and wires the
// OS-integration plugins. The frontend detects Tauri at runtime and points the
// API/WS at the login-configured server (see frontend/src/utils/serverBase.js).
//
// Phase 1: native notifications, self-update, file dialogs + fs, clipboard.
// Phase 2: single-instance, system tray, close-to-tray, autostart.
// Phase 3: GitLab OAuth via the system browser + tessera:// deep link (#2696).
// Phase 4: media permissions for conferences — camera/mic/screen share (#2929).

mod media;

use tauri::{
    menu::{Menu, MenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    Manager, WindowEvent,
};
use tauri_plugin_deep_link::DeepLinkExt;

// Bring the main window to the foreground (from the tray, a second launch, or a
// notification click).
fn show_main<R: tauri::Runtime>(app: &tauri::AppHandle<R>) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.show();
        let _ = w.unminimize();
        let _ = w.set_focus();
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        // Single-instance must be registered first: a second launch focuses the
        // running window instead of spawning a duplicate.
        //
        // It is also how deep links reach a *running* app on Linux/Windows: the OS
        // starts a second process with the URL as its only argument, and this callback
        // is the only place that argv can be recovered. The deep-link plugin does not
        // hook single-instance itself — forwarding argv here is required, or
        // tessera:// login would work only when the app was closed.
        .plugin(tauri_plugin_single_instance::init(|app, argv, _cwd| {
            app.deep_link().handle_cli_arguments(argv.iter());
            show_main(app);
        }))
        .plugin(tauri_plugin_autostart::init(
            tauri_plugin_autostart::MacosLauncher::LaunchAgent,
            Some(vec!["--minimized"]),
        ))
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_deep_link::init())
        .plugin(tauri_plugin_opener::init())
        .setup(|app| {
            // Installers (.deb/.rpm/NSIS) register tessera:// system-wide, but an
            // AppImage installs nothing and `tauri dev` never runs an installer — so
            // register at runtime too. Best-effort by design: the handler points at the
            // current executable path, so a moved AppImage needs another launch to
            // refresh it, and the login screen already times out with that hint.
            if let Err(e) = app.deep_link().register_all() {
                eprintln!("tessera: could not register the tessera:// scheme: {e}");
            }

            // Conferences: turn media on in the native webview and grant camera/mic to
            // our own origin. Must happen before the frontend boots — it probes
            // navigator.mediaDevices once and disables calls if the API is missing.
            media::setup(app.handle());

            // System tray: menu (open / quit) + left-click to show the window.
            let show = MenuItem::with_id(app, "show", "Открыть Tessera", true, None::<&str>)?;
            let quit = MenuItem::with_id(app, "quit", "Выход", true, None::<&str>)?;
            let menu = Menu::with_items(app, &[&show, &quit])?;
            TrayIconBuilder::new()
                .icon(app.default_window_icon().unwrap().clone())
                .tooltip("Tessera")
                .menu(&menu)
                .show_menu_on_left_click(false)
                .on_menu_event(|app, event| match event.id.as_ref() {
                    "show" => show_main(app),
                    "quit" => app.exit(0),
                    _ => {}
                })
                .on_tray_icon_event(|tray, event| {
                    if let TrayIconEvent::Click {
                        button: MouseButton::Left,
                        button_state: MouseButtonState::Up,
                        ..
                    } = event
                    {
                        show_main(tray.app_handle());
                    }
                })
                .build(app)?;

            // Launched via autostart with --minimized: start hidden in the tray.
            if std::env::args().any(|a| a == "--minimized") {
                if let Some(w) = app.get_webview_window("main") {
                    let _ = w.hide();
                }
            }
            Ok(())
        })
        // Close-to-tray: hide the window instead of exiting so the WS stays alive
        // and reminders keep arriving. A real quit goes through the tray menu.
        .on_window_event(|window, event| {
            if let WindowEvent::CloseRequested { api, .. } = event {
                let _ = window.hide();
                api.prevent_close();
            }
        })
        .run(tauri::generate_context!())
        .expect("error while running the Tessera desktop app");
}

// The shell's security posture lives in two JSON files that nothing else checks:
// `capabilities/default.json` (what the webview may ask the Rust side to do) and the
// `app.security.csp` of `tauri.conf.json` (what the page may load/run). Both are easy to
// loosen "just for a moment" while debugging, and the app boots fine either way — so the
// regression is silent. These tests are the ratchet (#2932).
#[cfg(test)]
mod hardening {
    use serde_json::Value;

    const CAPABILITIES: &str = include_str!("../capabilities/default.json");
    const CONFIG: &str = include_str!("../tauri.conf.json");

    // csp_directive pulls one directive's source list out of the configured policy.
    fn csp_directive(csp: &str, name: &str) -> Option<Vec<String>> {
        csp.split(';')
            .map(str::trim)
            .find(|d| d == &name || d.starts_with(&format!("{name} ")))
            .map(|d| {
                d[name.len()..]
                    .split_whitespace()
                    .map(str::to_string)
                    .collect()
            })
    }

    fn csp() -> String {
        let cfg: Value = serde_json::from_str(CONFIG).expect("tauri.conf.json must be valid JSON");
        cfg["app"]["security"]["csp"]
            .as_str()
            .expect("app.security.csp must be a policy string, not null")
            .to_string()
    }

    // A blanket fs scope turns any script that reaches the page (a malicious task
    // description, a compromised dependency) into write-anywhere on the user's disk.
    // The Save-as dialog grants the single path the user picked, so nothing static is needed.
    #[test]
    fn the_filesystem_scope_is_not_blanket() {
        let caps: Value =
            serde_json::from_str(CAPABILITIES).expect("capabilities must be valid JSON");
        let perms = caps["permissions"]
            .as_array()
            .expect("capabilities must list permissions");
        for perm in perms {
            let Some(obj) = perm.as_object() else {
                continue;
            };
            if obj.get("identifier").and_then(Value::as_str) != Some("fs:scope") {
                continue;
            }
            for entry in obj
                .get("allow")
                .and_then(Value::as_array)
                .map(Vec::as_slice)
                .unwrap_or_default()
            {
                let path = entry
                    .get("path")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .trim_start_matches('/');
                assert!(
                    !matches!(path, "**" | "*" | "**/*"),
                    "fs:scope allows the whole filesystem ({path:?})"
                );
            }
        }
    }

    // Content injected into task text is the realistic attack: it runs as inline script or
    // pulls one in from a remote host. 'self' (plus the build-time hashes Tauri adds for the
    // inline script in index.html) stops both; 'unsafe-inline'/'unsafe-eval' would hand it back.
    #[test]
    fn scripts_may_only_come_from_the_bundle() {
        let csp = csp();
        let sources = csp_directive(&csp, "script-src")
            .expect("script-src must be set explicitly — default-src alone is too permissive");
        assert!(
            sources.contains(&"'self'".to_string()),
            "script-src: {sources:?}"
        );
        for banned in ["'unsafe-inline'", "'unsafe-eval'", "http:", "https:", "*"] {
            assert!(
                !sources.iter().any(|s| s == banned),
                "script-src must not contain {banned}: {sources:?}"
            );
        }
    }

    // withGlobalTauri publishes the whole plugin JS API on `window.__TAURI__`. Nothing in
    // the frontend reads it — the plugins are imported as npm modules
    // (`await import('@tauri-apps/plugin-dialog')` in utils/download.js), and isTauri()
    // only ever *probes* the global as a secondary signal next to __TAURI_INTERNALS__ and
    // the tauri:// protocol (frontend/src/utils/servers.js). So it buys nothing and hands
    // any script that reaches the page a ready-made `__TAURI__.fs.writeFile` /
    // `.dialog.save` / `.opener.openUrl` / `.process.relaunch` / `.clipboardManager`.
    //
    // Not a complete seal: __TAURI_INTERNALS__.invoke is injected regardless, so a
    // determined payload can still call a command by name. What goes away is the
    // documented, ready-to-hand surface — and a bundle the app does not use.
    #[test]
    fn the_global_tauri_api_is_not_published() {
        let cfg: Value = serde_json::from_str(CONFIG).expect("tauri.conf.json must be valid JSON");
        let global = &cfg["app"]["withGlobalTauri"];
        assert!(
            global.is_null() || global == &Value::Bool(false),
            "app.withGlobalTauri must stay off: {global}"
        );
    }

    // Plugins are another script-execution path, and a rewritten <base> relocates every
    // relative asset URL — both sit outside script-src.
    #[test]
    fn plugins_and_base_uri_are_pinned() {
        let csp = csp();
        assert_eq!(
            csp_directive(&csp, "object-src"),
            Some(vec!["'none'".to_string()])
        );
        assert_eq!(
            csp_directive(&csp, "base-uri"),
            Some(vec!["'self'".to_string()])
        );
    }
}
