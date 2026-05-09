---
name: tauri-2x-patterns
description: Tauri 2.x patterns for Obscura desktop — TrayIconBuilder, MenuBuilder, get_webview_window, commands, capabilities. Use when writing any desktop/src-tauri/ Rust code.
---

# Tauri 2.x Patterns (Obscura Desktop)

## ⚠️ Tauri 1.x APIs DO NOT WORK

Forbidden 1.x APIs (will not compile in 2.x):
- `SystemTrayBuilder` → use `TrayIconBuilder`
- `CustomMenuItem` → use `MenuItem::with_id`
- `app.system_tray()` → use `TrayIconBuilder::new().build(app)`
- `on_system_tray_event` → use `.on_tray_icon_event()`
- `app.get_window("main")` → use `app.get_webview_window("main")`
- `app.config().package.version` → use `app.package_info().version.to_string()`

## Project structure

```
desktop/
├── package.json
├── src/                    (frontend, often imports from frontend/ via path)
└── src-tauri/
    ├── Cargo.toml
    ├── tauri.conf.json
    ├── capabilities/
    │   └── default.json
    └── src/
        ├── main.rs
        ├── lib.rs
        ├── commands.rs
        └── tray.rs
```

## main.rs

```rust
fn main() {
    obscura_lib::run();
}
```

## lib.rs (app builder)

```rust
mod commands;
mod tray;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_single_instance::init(|app, _argv, _cwd| {
            tray::show_window(app);
        }))
        .setup(|app| {
            tray::setup_tray(&app.handle())?;
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            commands::get_app_version,
            commands::open_external,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
```

## commands.rs (IPC)

```rust
use tauri::{AppHandle, Manager, Runtime};

#[tauri::command]
pub fn get_app_version<R: Runtime>(app: AppHandle<R>) -> Result<String, String> {
    Ok(app.package_info().version.to_string())
}

#[tauri::command]
pub async fn open_external(url: String) -> Result<(), String> {
    // For Tauri 2.x with shell plugin
    Ok(())
}
```

## tray.rs (system tray, 2.x API)

```rust
use tauri::{
    menu::{Menu, MenuItem, PredefinedMenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    AppHandle, Manager, Runtime,
};

pub fn setup_tray<R: Runtime>(app: &AppHandle<R>) -> tauri::Result<()> {
    let show = MenuItem::with_id(app, "show", "Göster", true, None::<&str>)?;
    let hide = MenuItem::with_id(app, "hide", "Gizle", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, "quit", "Çık", true, None::<&str>)?;
    let separator = PredefinedMenuItem::separator(app)?;

    let menu = Menu::with_items(app, &[&show, &hide, &separator, &quit])?;

    let _tray = TrayIconBuilder::new()
        .icon(app.default_window_icon().unwrap().clone())
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(move |app, event| match event.id.as_ref() {
            "show" => show_window(app),
            "hide" => hide_window(app),
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
                let app = tray.app_handle();
                toggle_window(app);
            }
        })
        .build(app)?;
    Ok(())
}

pub fn show_window<R: Runtime>(app: &AppHandle<R>) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.set_focus();
        let _ = window.unminimize();
    }
}

pub fn hide_window<R: Runtime>(app: &AppHandle<R>) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.hide();
    }
}

pub fn toggle_window<R: Runtime>(app: &AppHandle<R>) {
    if let Some(window) = app.get_webview_window("main") {
        if window.is_visible().unwrap_or(false) {
            let _ = window.hide();
        } else {
            let _ = window.show();
            let _ = window.set_focus();
        }
    }
}
```

## Cargo.toml essentials

```toml
[package]
name = "obscura-desktop"
version = "1.0.0"
edition = "2021"

[lib]
name = "obscura_lib"
crate-type = ["staticlib", "cdylib", "rlib"]

[build-dependencies]
tauri-build = { version = "2.0", features = [] }

[dependencies]
tauri = { version = "2.0", features = ["tray-icon"] }
tauri-plugin-shell = "2.0"
tauri-plugin-dialog = "2.0"
tauri-plugin-notification = "2.0"
tauri-plugin-single-instance = "2.0"
serde = { version = "1", features = ["derive"] }
serde_json = "1"
```

## tauri.conf.json essentials (2.x format)

```json
{
  "$schema": "https://schema.tauri.app/config/2",
  "productName": "Obscura",
  "version": "1.0.0",
  "identifier": "network.obscura.desktop",
  "build": {
    "beforeDevCommand": "npm run dev",
    "beforeBuildCommand": "npm run build",
    "frontendDist": "../frontend/out",
    "devUrl": "http://localhost:3000"
  },
  "app": {
    "windows": [
      {
        "title": "Obscura",
        "width": 1200,
        "height": 800,
        "minWidth": 800,
        "minHeight": 600,
        "resizable": true,
        "fullscreen": false
      }
    ],
    "security": {
      "csp": "default-src 'self'; connect-src 'self' http://localhost:8080 ws://localhost:8080"
    },
    "trayIcon": {
      "iconPath": "icons/icon.png",
      "iconAsTemplate": true
    }
  },
  "bundle": {
    "active": true,
    "targets": "all",
    "icon": ["icons/icon.png", "icons/icon.ico", "icons/icon.icns"]
  }
}
```

## capabilities/default.json (security)

```json
{
  "$schema": "../gen/schemas/desktop-schema.json",
  "identifier": "default",
  "description": "Default capability",
  "windows": ["main"],
  "permissions": [
    "core:default",
    "shell:allow-open",
    "dialog:default",
    "notification:default"
  ]
}
```

## Build commands

```bash
cd D:/obscura/desktop
npm install
npm run tauri dev      # development
npm run tauri build    # production bundle
cargo check --manifest-path src-tauri/Cargo.toml  # quick syntax check
```

## Common pitfalls

- Forgetting `[lib]` section → `cargo build` fails
- Missing `tray-icon` feature → `TrayIconBuilder` not found
- Using `tauri::generate_handler!` without commands listed → IPC fails silently
- CSP too strict → frontend can't reach backend
- Missing capability → permission denied at runtime
