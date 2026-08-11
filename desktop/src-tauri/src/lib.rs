// Tessera desktop shell (Tauri v2). Hosts the shared Vue frontend and wires the
// OS-integration plugins. The frontend detects Tauri at runtime and points the
// API/WS at the login-configured server (see frontend/src/utils/serverBase.js).
//
// Phase 1: native notifications, self-update, file dialogs + fs, clipboard.
// Phase 2: single-instance, system tray, close-to-tray, autostart.
// Phase 3: GitLab OAuth via the system browser + tessera:// deep link (#2696).

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
