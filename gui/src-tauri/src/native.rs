//! native.rs — lo único que la interfaz hace fuera de ccp: abrir una terminal
//! con un comando ya escrito y enseñar un archivo en Finder.
//!
//! Hay cosas que solo pueden pasar en una terminal (un /login, un préstamo, una
//! sesión supervisada): claude es interactivo y el perfil de una terminal solo
//! lo cambia la función shell. La app no las finge; abre una ventana nueva en
//! la carpeta correcta con el comando puesto.
//!
//! Los argumentos llegan separados y aquí se citan uno a uno; la línea que ve
//! la shell nunca se arma pegando texto sin escapar. Y solo se aceptan
//! comandos `ccp`: la interfaz no tiene por qué pedir otra cosa.
//!
//! Con `CCP_SANDBOX` (desarrollo) la terminal que se abre es la de verdad, con
//! la shell y el rc del usuario: la línea empieza exportando el HOME, el
//! CCP_HOME y el `ccp` del sandbox para que el comando actúe sobre él y no
//! sobre la configuración real.

use std::path::Path;
use std::process::Command;

/// Cita un argumento para POSIX sh: tal cual si es inocuo, entre comillas
/// simples si no (con la comilla simple escapada como '\'').
fn sh_quote(a: &str) -> String {
    if !a.is_empty()
        && a.chars()
            .all(|c| c.is_ascii_alphanumeric() || "_@%+=:,./-".contains(c))
    {
        return a.to_string();
    }
    format!("'{}'", a.replace('\'', r"'\''"))
}

/// El prefijo que lleva la terminal al sandbox, si la app corre en uno.
fn sandbox_prefix(sandbox: Option<&str>) -> Option<String> {
    let sb = sandbox.filter(|s| !s.is_empty())?;
    Some(format!(
        "export HOME={} CCP_HOME={} PATH={}:\"$PATH\"",
        sh_quote(&format!("{sb}/home")),
        sh_quote(&format!("{sb}/ccp")),
        sh_quote(&format!("{sb}/bin")),
    ))
}

fn command_line(
    prefix: Option<&str>,
    cwd: Option<&str>,
    cmds: &[Vec<String>],
) -> Result<String, String> {
    if cmds.is_empty() {
        return Err("no hay nada que ejecutar".into());
    }
    let mut parts = Vec::new();
    if let Some(p) = prefix {
        parts.push(p.to_string());
    }
    if let Some(dir) = cwd {
        let p = Path::new(dir);
        if p.is_absolute() && p.is_dir() {
            parts.push(format!("cd {}", sh_quote(dir)));
        }
    }
    for argv in cmds {
        match argv.first().map(String::as_str) {
            Some("ccp") => {}
            _ => return Err("solo se abren comandos de ccp".into()),
        }
        parts.push(
            argv.iter()
                .map(|a| sh_quote(a))
                .collect::<Vec<_>>()
                .join(" "),
        );
    }
    Ok(parts.join(" && "))
}

/// Escapa una cadena para meterla entre comillas dobles en AppleScript.
#[cfg(target_os = "macos")]
fn applescript_string(s: &str) -> String {
    format!("\"{}\"", s.replace('\\', "\\\\").replace('"', "\\\""))
}

#[tauri::command]
pub fn open_terminal(cwd: Option<String>, cmds: Vec<Vec<String>>) -> Result<(), String> {
    let sandbox = std::env::var("CCP_SANDBOX").ok();
    let prefix = sandbox_prefix(sandbox.as_deref());
    let line = command_line(prefix.as_deref(), cwd.as_deref(), &cmds)?;
    #[cfg(target_os = "macos")]
    {
        let script = format!(
            "tell application \"Terminal\"\n  activate\n  do script {}\nend tell",
            applescript_string(&line)
        );
        let out = Command::new("osascript")
            .arg("-e")
            .arg(script)
            .output()
            .map_err(|e| e.to_string())?;
        if !out.status.success() {
            return Err(String::from_utf8_lossy(&out.stderr).trim().to_string());
        }
        Ok(())
    }
    #[cfg(not(target_os = "macos"))]
    {
        let keep = format!("{line}; exec \"${{SHELL:-bash}}\" -i");
        Command::new("x-terminal-emulator")
            .args(["-e", "bash", "-lc", &keep])
            .spawn()
            .map(|_| ())
            .map_err(|e| format!("no se pudo abrir una terminal: {e}"))
    }
}

#[tauri::command]
pub fn reveal_path(path: String) -> Result<(), String> {
    let p = Path::new(&path);
    if !p.is_absolute() || !p.exists() {
        return Err(format!("no existe: {path}"));
    }
    #[cfg(target_os = "macos")]
    let st = Command::new("open").arg("-R").arg(p).status();
    #[cfg(not(target_os = "macos"))]
    let st = Command::new("xdg-open")
        .arg(p.parent().unwrap_or(p))
        .status();
    match st {
        Ok(s) if s.success() => Ok(()),
        Ok(s) => Err(format!("terminó con {s}")),
        Err(e) => Err(e.to_string()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cita_como_la_shell() {
        assert_eq!(sh_quote("ccp"), "ccp");
        assert_eq!(sh_quote("~/x"), "'~/x'");
        assert_eq!(sh_quote("a b"), "'a b'");
        assert_eq!(sh_quote("it's"), r"'it'\''s'");
        assert_eq!(sh_quote(""), "''");
        assert_eq!(sh_quote("$(rm -rf /)"), "'$(rm -rf /)'");
    }

    #[test]
    fn solo_comandos_de_ccp() {
        let ok = command_line(
            None,
            None,
            &[
                vec!["ccp".into(), "use".into(), "trabajo".into()],
                vec!["ccp".into(), "handoff".into(), "end".into()],
            ],
        );
        assert_eq!(ok.unwrap(), "ccp use trabajo && ccp handoff end");
        assert!(command_line(None, None, &[vec!["rm".into(), "-rf".into()]]).is_err());
        assert!(command_line(None, None, &[]).is_err());
    }

    #[test]
    fn ignora_carpetas_que_no_existen() {
        let line = command_line(
            None,
            Some("/no/existe/seguro"),
            &[vec!["ccp".into(), "status".into()]],
        )
        .unwrap();
        assert_eq!(line, "ccp status");
        let line =
            command_line(None, Some("/tmp"), &[vec!["ccp".into(), "status".into()]]).unwrap();
        assert_eq!(line, "cd /tmp && ccp status");
    }

    #[test]
    fn en_el_sandbox_la_terminal_no_toca_la_config_real() {
        assert_eq!(sandbox_prefix(None), None);
        assert_eq!(sandbox_prefix(Some("")), None);
        let p = sandbox_prefix(Some("/x/.sandbox")).unwrap();
        assert_eq!(
            p,
            "export HOME=/x/.sandbox/home CCP_HOME=/x/.sandbox/ccp PATH=/x/.sandbox/bin:\"$PATH\""
        );
        let line = command_line(Some(&p), None, &[vec!["ccp".into(), "status".into()]]).unwrap();
        assert!(line.starts_with("export HOME=/x/.sandbox/home "));
        assert!(line.ends_with(" && ccp status"));
        let raro = sandbox_prefix(Some("/mi carpeta/.sandbox")).unwrap();
        assert!(raro.contains("HOME='/mi carpeta/.sandbox/home'"));
    }

    #[cfg(target_os = "macos")]
    #[test]
    fn escapa_para_applescript() {
        assert_eq!(
            applescript_string(r#"echo "a" \ b"#),
            r#""echo \"a\" \\ b""#
        );
    }
}
