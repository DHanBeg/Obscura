// Tauri 2.x System Tray — Obscura Desktop
use tauri::{
    menu::{Menu, MenuItem, PredefinedMenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    AppHandle, Manager, Runtime,
};

/// Sistem tepsisi ikonunu ve menüsünü oluştur
pub fn setup_tray<R: Runtime>(app: &AppHandle<R>) -> tauri::Result<()> {
    let show = MenuItem::with_id(app, "show", "Obscura'yı Göster", true, None::<&str>)?;
    let new_chat = MenuItem::with_id(app, "new_chat", "Yeni Sohbet", true, None::<&str>)?;
    let separator = PredefinedMenuItem::separator(app)?;
    let quit = MenuItem::with_id(app, "quit", "Çıkış", true, None::<&str>)?;

    let menu = Menu::with_items(app, &[&show, &new_chat, &separator, &quit])?;

    TrayIconBuilder::new()
        .icon(app.default_window_icon().unwrap().clone())
        .tooltip("Obscura — Şifreli Mesajlaşma")
        .menu(&menu)
        .menu_on_left_click(false) // Sol tık → pencere toggle
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                let app = tray.app_handle();
                toggle_window(app);
            }
        })
        .on_menu_event(|app, event| match event.id.as_ref() {
            "show" => show_window(app),
            "new_chat" => {
                show_window(app);
                // Frontend'e yeni sohbet aç eventi gönder
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.emit("open-new-chat", ());
                }
            }
            "quit" => app.exit(0),
            _ => {}
        })
        .build(app)?;

    Ok(())
}

pub fn toggle_window<R: Runtime>(app: &AppHandle<R>) {
    if let Some(window) = app.get_webview_window("main") {
        if window.is_visible().unwrap_or(false) {
            let _ = window.hide();
        } else {
            show_window(app);
        }
    }
}

pub fn show_window<R: Runtime>(app: &AppHandle<R>) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.set_focus();
        let _ = window.unminimize();
    }
}
