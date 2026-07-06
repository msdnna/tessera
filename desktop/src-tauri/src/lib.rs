// Tessera desktop shell (Tauri v2). Hosts the shared Vue frontend and wires the
// OS-integration plugins. The frontend detects Tauri at runtime and points the
// API/WS at the login-configured server (see frontend/src/utils/serverBase.js).
//
// Phase 1 (MVP): native notifications, self-update, native file dialogs + fs
// (save downloaded attachments), clipboard (image-paste fallback on Linux).
// Phase 2 adds tray / close-to-tray / autostart / single-instance / deep-link.

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_process::init())
        .run(tauri::generate_context!())
        .expect("error while running the Tessera desktop app");
}
