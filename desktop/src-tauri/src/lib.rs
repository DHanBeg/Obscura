// Obscura Desktop — Tauri 2.x Ana Uygulama
use tauri::{Manager, WindowEvent};

mod commands;
mod tray;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        // ─── Eklentiler ──────────────────────────────────────────────────────
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_store::Builder::default().build())
        .plugin(tauri_plugin_window_state::Builder::default().build())
        .plugin(tauri_plugin_autostart::init(
            tauri_plugin_autostart::MacosLauncher::LaunchAgent,
            Some(vec!["--minimized"]),
        ))
        // ─── Komutlar ────────────────────────────────────────────────────────
        .invoke_handler(tauri::generate_handler![
            commands::get_token,
            commands::set_token,
            commands::delete_token,
            commands::show_notification,
            commands::get_app_version,
        ])
        // ─── Pencere Olayları ─────────────────────────────────────────────────
        .on_window_event(|window, event| {
            if let WindowEvent::CloseRequested { api, .. } = event {
                // Kapat → system tray'e küçült (uygulamayı kapatma)
                window.hide().unwrap_or_default();
                api.prevent_close();
            }
        })
        // ─── Kurulum ──────────────────────────────────────────────────────────
        .setup(|app| {
            // System tray kur
            tray::setup_tray(&app.handle())?;

            // Ana pencereyi merkeze al ve göster
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.center();
                let _ = window.show();
                let _ = window.set_focus();
            }

            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("Tauri uygulaması başlatılamadı");
}
