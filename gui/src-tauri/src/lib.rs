//! ccp — interfaz de escritorio. Rust solo hace de puente: la lógica vive en el
//! binario de ccp (`ccp serve --stdio`) y la presentación en la interfaz web.

mod bridge;
mod native;

pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .manage(bridge::Bridge::default())
        .invoke_handler(tauri::generate_handler![
            bridge::ccp_call,
            bridge::ccp_bridge_info,
            native::open_terminal,
            native::reveal_path,
            native::restart_app
        ])
        .run(tauri::generate_context!())
        .expect("no se pudo arrancar la app de ccp");
}
