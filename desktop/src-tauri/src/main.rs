// Obscura Desktop — Tauri entry point
// Windows'ta konsol penceresi açılmasın
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    obscura_desktop_lib::run();
}
