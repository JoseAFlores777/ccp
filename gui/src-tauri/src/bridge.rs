//! bridge.rs — el puente entre la interfaz y el motor de ccp.
//!
//! La app no reimplementa nada de ccp: mantiene vivo un `ccp serve --stdio` y
//! le pasa las peticiones de la interfaz, una línea JSON por petición. Este
//! archivo decide QUÉ binario arrancar y con qué entorno, y reparte las
//! respuestas a quien las pidió.
//!
//! Qué binario, en este orden:
//!   1. `CCP_GUI_BIN`, si está (desarrollo, pruebas).
//!   2. `CCP_SANDBOX`: el binario y el HOME de pruebas de `npm run sandbox`.
//!   3. El ccp instalado (el de la shell del usuario), si entiende `serve`.
//!   4. El que viaja dentro de la app (sidecar), si el instalado no existe o es
//!      anterior a `serve`. En ese caso los sensores siguen apuntando al
//!      instalado: es el que seguirá ahí cuando la app se cierre.
//!
//! El entorno del hijo no hereda el perfil de ninguna terminal: la app decide
//! el contexto (carpeta, cuenta) en cada petición, y un `CCP_PROFILE` o un
//! `ANTHROPIC_BASE_URL` heredados cambiarían las respuestas en silencio.

use serde::Serialize;
use serde_json::{json, Value};
use std::collections::HashMap;
use std::io::{BufRead, BufReader, Write};
use std::path::{Path, PathBuf};
use std::process::{Child, ChildStdin, Command, Stdio};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{mpsc, Arc, Mutex, OnceLock};
use std::time::Duration;
use tokio::sync::oneshot;

/// Variables que ccp gestiona: nunca se heredan de la terminal que lanzó la app.
const MANAGED_VARS: &[&str] = &[
    "CCP_PROFILE",
    "CLAUDE_CONFIG_DIR",
    "ANTHROPIC_BASE_URL",
    "ANTHROPIC_AUTH_TOKEN",
    "ANTHROPIC_MODEL",
    "ANTHROPIC_DEFAULT_OPUS_MODEL",
    "ANTHROPIC_DEFAULT_SONNET_MODEL",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL",
    "CLAUDE_CODE_SUBAGENT_MODEL",
    "CLAUDE_CODE_EFFORT_LEVEL",
    "ENABLE_TOOL_SEARCH",
    "API_TIMEOUT_MS",
    "CLAUDE_CODE_AUTO_COMPACT_WINDOW",
];

#[derive(Serialize, Clone, Debug)]
pub struct BridgeInfo {
    pub binary: String,
    /// installed | bundled | env | dev
    pub source: String,
    pub version: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub fallback_reason: Option<String>,
}

type Pending = Arc<Mutex<HashMap<u64, oneshot::Sender<Value>>>>;

struct Live {
    child: Child,
    stdin: ChildStdin,
    alive: Arc<AtomicBool>,
    info: BridgeInfo,
}

pub struct Bridge {
    live: Mutex<Option<Live>>,
    pending: Pending,
    next: AtomicU64,
}

impl Default for Bridge {
    fn default() -> Self {
        Self {
            live: Mutex::new(None),
            pending: Arc::new(Mutex::new(HashMap::new())),
            next: AtomicU64::new(1),
        }
    }
}

impl Drop for Bridge {
    fn drop(&mut self) {
        if let Ok(mut g) = self.live.lock() {
            if let Some(mut l) = g.take() {
                let _ = l.child.kill();
            }
        }
    }
}

fn err_json(code: &str, message: impl Into<String>) -> String {
    json!({ "code": code, "message": message.into() }).to_string()
}

/// El PATH de la shell de login del usuario. Una app abierta desde Finder
/// arranca con /usr/bin:/bin y poco más, y ccp necesita encontrar `claude`,
/// `git` y compañía igual que en la terminal.
fn login_path() -> &'static str {
    static PATH: OnceLock<String> = OnceLock::new();
    PATH.get_or_init(|| {
        let inherited = std::env::var("PATH").unwrap_or_default();
        let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/zsh".into());
        let (tx, rx) = mpsc::channel();
        let child = Command::new(&shell)
            .args(["-ilc", "printf '__CCP_PATH__%s' \"$PATH\""])
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::null())
            .spawn();
        if let Ok(child) = child {
            std::thread::spawn(move || {
                let out = child.wait_with_output().ok();
                let _ = tx.send(out);
            });
            if let Ok(Some(out)) = rx.recv_timeout(Duration::from_secs(4)) {
                let s = String::from_utf8_lossy(&out.stdout);
                if let Some(p) = s.rsplit("__CCP_PATH__").next() {
                    let p = p.trim();
                    if !p.is_empty() && s.contains("__CCP_PATH__") {
                        return p.to_string();
                    }
                }
            }
        }
        // Sin shell que preguntar: lo heredado más los sitios de siempre.
        let home = std::env::var("HOME").unwrap_or_default();
        let mut parts: Vec<String> = inherited
            .split(':')
            .filter(|s| !s.is_empty())
            .map(String::from)
            .collect();
        for extra in [
            format!("{home}/.local/bin"),
            "/opt/homebrew/bin".into(),
            "/usr/local/bin".into(),
            "/usr/bin".into(),
            "/bin".into(),
            "/usr/sbin".into(),
            "/sbin".into(),
        ] {
            if !parts.contains(&extra) {
                parts.push(extra);
            }
        }
        parts.join(":")
    })
}

fn is_exec(p: &Path) -> bool {
    p.is_file()
}

/// El ccp instalado: el primero que encuentre el PATH de la shell de login, o
/// los sitios donde lo deja install.sh.
fn installed_ccp() -> Option<PathBuf> {
    for dir in login_path().split(':') {
        let p = Path::new(dir).join("ccp");
        if is_exec(&p) {
            return Some(p);
        }
    }
    let home = std::env::var("HOME").unwrap_or_default();
    [
        format!("{home}/.local/bin/ccp"),
        "/opt/homebrew/bin/ccp".into(),
        "/usr/local/bin/ccp".into(),
    ]
    .into_iter()
    .map(PathBuf::from)
    .find(|p| is_exec(p))
}

/// El ccp que viaja dentro de la app, junto al ejecutable (externalBin).
fn bundled_ccp() -> Option<PathBuf> {
    let exe = std::env::current_exe().ok()?;
    let p = exe.parent()?.join("ccp");
    is_exec(&p).then_some(p)
}

struct Candidate {
    bin: PathBuf,
    source: &'static str,
    env: Vec<(String, String)>,
}

fn candidates() -> (Vec<Candidate>, Option<PathBuf>) {
    let mut out = Vec::new();
    let installed = installed_ccp();
    if let Ok(bin) = std::env::var("CCP_GUI_BIN") {
        if !bin.is_empty() {
            let mut env = Vec::new();
            if let Ok(sb) = std::env::var("CCP_SANDBOX") {
                env.push(("HOME".into(), format!("{sb}/home")));
                env.push(("CCP_HOME".into(), format!("{sb}/ccp")));
            }
            out.push(Candidate {
                bin: bin.into(),
                source: "env",
                env,
            });
            return (out, installed);
        }
    }
    if let Ok(sb) = std::env::var("CCP_SANDBOX") {
        if !sb.is_empty() {
            out.push(Candidate {
                bin: PathBuf::from(format!("{sb}/bin/ccp")),
                source: "dev",
                env: vec![
                    ("HOME".into(), format!("{sb}/home")),
                    ("CCP_HOME".into(), format!("{sb}/ccp")),
                ],
            });
            return (out, installed);
        }
    }
    if let Some(p) = &installed {
        out.push(Candidate {
            bin: p.clone(),
            source: "installed",
            env: vec![],
        });
    }
    if let Some(p) = bundled_ccp() {
        let mut env = Vec::new();
        if let Some(inst) = &installed {
            env.push(("CCP_SENSOR_BIN".into(), inst.to_string_lossy().into_owned()));
        }
        out.push(Candidate {
            bin: p,
            source: "bundled",
            env,
        });
    }
    (out, installed)
}

/// Arranca un candidato y espera su `{"event":"ready"}`. Devuelve el proceso,
/// su stdin, el lector de stdout ya posicionado tras el ready y la versión.
fn start(
    c: &Candidate,
) -> Result<
    (
        Child,
        ChildStdin,
        BufReader<std::process::ChildStdout>,
        String,
    ),
    String,
> {
    let mut cmd = Command::new(&c.bin);
    cmd.args(["serve", "--stdio"])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    for v in MANAGED_VARS {
        cmd.env_remove(v);
    }
    cmd.env("PATH", login_path());
    for (k, v) in &c.env {
        cmd.env(k, v);
    }
    let mut child = cmd
        .spawn()
        .map_err(|e| format!("no se pudo arrancar {}: {e}", c.bin.display()))?;
    let stdin = child.stdin.take().ok_or("sin stdin")?;
    let stdout = child.stdout.take().ok_or("sin stdout")?;
    if let Some(stderr) = child.stderr.take() {
        // Lo que ccp diga por stderr va al registro de la app, línea a línea.
        std::thread::spawn(move || {
            for line in BufReader::new(stderr).lines().map_while(Result::ok) {
                eprintln!("[ccp serve] {line}");
            }
        });
    }
    let (tx, rx) = mpsc::channel();
    std::thread::spawn(move || {
        let mut reader = BufReader::new(stdout);
        let mut line = String::new();
        let res = match reader.read_line(&mut line) {
            Ok(0) | Err(_) => Err("terminó sin responder".to_string()),
            Ok(_) => Ok(line),
        };
        let _ = tx.send((res, reader));
    });
    match rx.recv_timeout(Duration::from_secs(8)) {
        Ok((Ok(line), reader)) => {
            let v: Value = serde_json::from_str(line.trim())
                .map_err(|_| format!("respuesta inesperada: {}", line.trim()))?;
            if v.get("event").and_then(Value::as_str) != Some("ready") {
                let _ = child.kill();
                return Err(format!("respuesta inesperada: {}", line.trim()));
            }
            let version = v
                .get("version")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_string();
            Ok((child, stdin, reader, version))
        }
        Ok((Err(e), _)) => {
            let _ = child.kill();
            let _ = child.wait();
            Err(format!(
                "{} {e} (¿es anterior a «ccp serve»?)",
                c.bin.display()
            ))
        }
        Err(_) => {
            let _ = child.kill();
            Err(format!("{} no respondió a tiempo", c.bin.display()))
        }
    }
}

impl Bridge {
    /// Garantiza un `ccp serve` vivo y devuelve su información.
    fn ensure(&self) -> Result<BridgeInfo, String> {
        let mut guard = self
            .live
            .lock()
            .map_err(|_| err_json("bridge_down", "puente bloqueado"))?;
        if let Some(l) = guard.as_mut() {
            if l.alive.load(Ordering::SeqCst) {
                return Ok(l.info.clone());
            }
            let _ = l.child.kill();
            let _ = l.child.wait();
            *guard = None;
        }
        let (cands, _) = candidates();
        if cands.is_empty() {
            return Err(err_json(
                "no_ccp",
                "No encuentro ccp: instálalo con el instalador del README (curl … install.sh | bash) y vuelve a abrir la app.",
            ));
        }
        let mut reasons = Vec::new();
        for c in &cands {
            match start(c) {
                Ok((child, stdin, reader, version)) => {
                    let alive = Arc::new(AtomicBool::new(true));
                    let pending = self.pending.clone();
                    let flag = alive.clone();
                    std::thread::spawn(move || {
                        for line in reader.lines().map_while(Result::ok) {
                            let Ok(msg) = serde_json::from_str::<Value>(&line) else {
                                continue;
                            };
                            let Some(id) = msg.get("id").and_then(Value::as_u64) else {
                                continue;
                            };
                            if let Some(tx) = pending.lock().ok().and_then(|mut p| p.remove(&id)) {
                                let _ = tx.send(msg);
                            }
                        }
                        // EOF: el proceso murió. Lo pendiente se contesta con
                        // error en vez de quedarse esperando para siempre.
                        flag.store(false, Ordering::SeqCst);
                        if let Ok(mut p) = pending.lock() {
                            for (_, tx) in p.drain() {
                                let _ = tx.send(json!({ "error": { "code": "bridge_down", "message": "ccp serve terminó" } }));
                            }
                        }
                    });
                    let info = BridgeInfo {
                        binary: c.bin.to_string_lossy().into_owned(),
                        source: c.source.into(),
                        version,
                        fallback_reason: if reasons.is_empty() {
                            None
                        } else {
                            Some(reasons.join(" · "))
                        },
                    };
                    *guard = Some(Live {
                        child,
                        stdin,
                        alive,
                        info: info.clone(),
                    });
                    return Ok(info);
                }
                Err(e) => reasons.push(e),
            }
        }
        Err(err_json("bridge_down", reasons.join(" · ")))
    }

    fn send(&self, method: &str, params: Value) -> Result<oneshot::Receiver<Value>, String> {
        self.ensure()?;
        let id = self.next.fetch_add(1, Ordering::SeqCst);
        let (tx, rx) = oneshot::channel();
        self.pending
            .lock()
            .map_err(|_| err_json("bridge_down", "puente bloqueado"))?
            .insert(id, tx);
        let line = json!({ "id": id, "method": method, "params": params }).to_string() + "\n";
        let mut guard = self
            .live
            .lock()
            .map_err(|_| err_json("bridge_down", "puente bloqueado"))?;
        let live = guard
            .as_mut()
            .ok_or_else(|| err_json("bridge_down", "ccp serve no está vivo"))?;
        if let Err(e) = live
            .stdin
            .write_all(line.as_bytes())
            .and_then(|_| live.stdin.flush())
        {
            live.alive.store(false, Ordering::SeqCst);
            if let Ok(mut p) = self.pending.lock() {
                p.remove(&id);
            }
            return Err(err_json(
                "bridge_down",
                format!("no se pudo escribir a ccp serve: {e}"),
            ));
        }
        Ok(rx)
    }
}

/// Cuánto se espera a una respuesta. Actualizar ccp o importar una sesión en
/// Desktop tardan más que leer ccp.yaml.
fn timeout_for(method: &str) -> Duration {
    match method {
        "system.run" => Duration::from_secs(900),
        "conversations.copy" | "desktop.run" | "auto.test" | "backup.restore" => {
            Duration::from_secs(300)
        }
        _ => Duration::from_secs(120),
    }
}

#[tauri::command]
pub async fn ccp_call(
    bridge: tauri::State<'_, Bridge>,
    method: String,
    params: Option<Value>,
) -> Result<Value, String> {
    let rx = bridge.send(&method, params.unwrap_or_else(|| json!({})))?;
    let wait = timeout_for(&method);
    match tokio::time::timeout(wait, rx).await {
        Ok(Ok(msg)) => {
            if let Some(err) = msg.get("error") {
                Err(err.to_string())
            } else {
                Ok(msg.get("result").cloned().unwrap_or(Value::Null))
            }
        }
        Ok(Err(_)) => Err(err_json("bridge_down", "ccp serve terminó")),
        Err(_) => Err(err_json(
            "timeout",
            format!("{method} no respondió en {} s", wait.as_secs()),
        )),
    }
}

#[tauri::command]
pub fn ccp_bridge_info(bridge: tauri::State<'_, Bridge>) -> Result<BridgeInfo, String> {
    bridge.ensure()
}
