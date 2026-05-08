use keyring::Entry;
use tauri::AppHandle;
use tauri_plugin_notification::NotificationExt;

const SERVICE: &str = "obscura-desktop";
const TOKEN_KEY: &str = "auth_token";

/// JWT token'ı OS keychain'den al
#[tauri::command]
pub fn get_token() -> Result<Option<String>, String> {
    let entry = Entry::new(SERVICE, TOKEN_KEY).map_err(|e| e.to_string())?;
    match entry.get_password() {
        Ok(token) => Ok(Some(token)),
        Err(keyring::Error::NoEntry) => Ok(None),
        Err(e) => Err(e.to_string()),
    }
}

/// JWT token'ı OS keychain'e kaydet
#[tauri::command]
pub fn set_token(token: String) -> Result<(), String> {
    let entry = Entry::new(SERVICE, TOKEN_KEY).map_err(|e| e.to_string())?;
    entry.set_password(&token).map_err(|e| e.to_string())
}

/// Token'ı sil (logout)
#[tauri::command]
pub fn delete_token() -> Result<(), String> {
    let entry = Entry::new(SERVICE, TOKEN_KEY).map_err(|e| e.to_string())?;
    entry.delete_password().map_err(|e| e.to_string())
}

/// Native bildirim göster
#[tauri::command]
pub fn show_notification(
    app: AppHandle,
    title: String,
    body: String,
) -> Result<(), String> {
    app.notification()
        .builder()
        .title(&title)
        .body(&body)
        .show()
        .map_err(|e| e.to_string())
}

/// Uygulama sürümünü döndür
#[tauri::command]
pub fn get_app_version(app: AppHandle) -> String {
    // Tauri 2: package.version → app.config().version
    app.package_info().version.to_string()
}
