// Sin consola aparte en Windows en los builds de release.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    ccp_gui_lib::run()
}
