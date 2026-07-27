# Changelog

## [2.11.2] — la barra propia enseña las dos ventanas

### Changed

- La statusLine mínima que ccp pinta cuando **no** tienes una propia pasa de
  `emco-cc · 31%` a `emco-cc · 5h 14% · 7d 31%`. Antes enseñaba solo el máximo
  entre las dos ventanas, que es el número que decide (la más gastada corta
  primero) pero como porcentaje suelto era ambiguo: no dice si te quedan horas
  o días, y saltaba de una ventana a otra en cuanto la otra la adelantaba, sin
  indicarlo. El sensor vigila las dos por separado —`ExhaustedAt` compara cada
  una contra el umbral—, así que la barra ahora enseña las dos.
  Una ventana **sin dato se omite** en vez de pintarse como `0%`: el bug conocido
  de CC 2.1.220 (`five_hour` a 0 con `seven_day` poblado) haría que ese `0%`
  afirmara justo lo contrario de lo que sabemos. Las etiquetas `5h`/`7d` son las
  que ya usaba `ccp auto status` y las claves del propio Claude Code.
  Si tienes statusLine propia no cambia nada: se sigue envolviendo y se sigue
  pintando la tuya intacta.

## [2.11.1] — CI verde en el commit tageado

Sin cambios de producto: los binarios son idénticos a los de 2.11.0. Lo único
que cambia es un test.

### Fixed

- `TestRunContextoCanceladoMataAlHijo` pasaba en macOS y fallaba **siempre** en
  CI, clavado en los 10s de `termGrace`. La causa era el falso claude del test,
  no el supervisor: `#!/bin/sh` + `sleep 30` deja al shell como hijo directo, y
  ahí manda qué sea `/bin/sh`. El bash de macOS hace `exec` implícito del último
  comando (el hijo ES `sleep` y muere con el SIGTERM), pero dash —el `/bin/sh`
  de Ubuntu— aplaza la señal mientras espera a un hijo en foreground, así que el
  shell sobrevivía la gracia entera y solo moría con el SIGKILL. Con `exec` el
  hijo es un único proceso, que es como corre el `claude` real (directo, sin
  shell en medio).

## [2.11.0] — auto-handoff

### Added

- **`ccp session`** — supervisor que corre `claude` como proceso hijo, detecta
  el límite de uso, presta la sesión a otro perfil y la relanza ahí. Un `claude`
  vivo no puede cambiar de perfil (`ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`
  y `CLAUDE_CONFIG_DIR` se leen una sola vez al arrancar), así que lo único
  limpio es un proceso de fuera que pueda matarlo y volver a arrancarlo.
  Interactivo y headless (`-p`), con `--policy`, `--max-hops`, `--yolo`,
  `--session`, `--dry-run`, `--no-return` y `--claude-bin`.
  Códigos de salida: `0` ok · `1` uso/config · `2` E/S de handoff · `75` todos
  los perfiles agotados (`EX_TEMPFAIL`) · cualquier otro es el de claude.
- **Rotación en péndulo, no round-robin.** El primario es `ccp resolve $PWD`;
  todo lo demás son préstamos. Se vuelve a casa en cuanto la ventana del
  primario se reabre, aunque queden préstamos frescos. `max_hops` cuenta
  préstamos, no movimientos: la vuelta a casa cierra un préstamo, así que ni
  gasta presupuesto ni la bloquea uno agotado (tope real: `2·max_hops + 1`
  lanzamientos).
- **Temporizador `return_check` + guard `return_idle`.** Ningún sensor dispara
  cuando se libera *otra* cuenta, así que un temporizador pregunta solo si el
  primario ya se liberó. Como esa es la única jugada que mata a un hijo *sano*,
  exige cuatro condiciones a la vez: préstamo vivo, cooldown del primario
  vencido, `min_dwell` cumplido y transcript en silencio `return_idle` (90s por
  defecto; `0s` es el opt-out).
- **Cuatro sensores, tres a la vez.** `ccp _statusline` (proactivo, dispara al
  `threshold` % antes de que falle un turno), el `api_retry` del stream-json en
  headless, la cola del transcript, y el hook `StopFailure` (`ccp _limit-hook`).
  Los dos internos siempre salen con 0: una statusLine rota dejaría a Claude
  Code sin barra de estado y un hook que falla molesta en cada turno.
- **`ccp auto init | install | uninstall | status [--json] | test`** — siembra
  la política, instala la capa de sensores en el `settings.json` generado de
  cada perfil (envolviendo tu propia statusLine, sin reemplazarla) y verifica el
  cableado. Reversible: la fuente de verdad es `auto_handoff.hooks` en
  `ccp.yaml`, no el archivo generado.
- **Verja `allow_from`** (compliance, default deny). Rotar solo, de madrugada,
  puede acabar mandando la conversación de un cliente a una cuenta personal o a
  una API de terceros; las reglas de ruta son geográficas, no una declaración de
  confianza. Tres estados: ausente ⇒ sin verja · declarada con entrada para el
  primario ⇒ solo eso pasa · declarada **sin** entrada ⇒ deny total.
- **`ccp handoff prune [--keep N]`** y **`ccp handoff sessions [--json]`**. El
  historial archivado crecía una entrada por handoff cerrado y nada lo limpiaba.
- Documentación: sección de auto-handoff en ambos READMEs, capítulo runbook
  nuevo en los dos manuales SPA, e `index.html` convertido de redirect a hub
  bilingüe con tabla situación → comando.

### Changed

- `handoffs.yaml` gana `auto: true` y `hops: [...]` en los marcadores (ambos
  `omitempty`; la versión del schema no cambia). `ccp.yaml` gana el bloque
  `auto_handoff` y **sigue en schema `version: 2`**, así que un binario viejo
  conserva las claves nuevas en vez de rechazar el archivo.
- El supervisor **sí encadena** handoffs, mutando el marcador vivo en sitio
  (`To`/`Hops`) en vez de apilar un nivel, así que `handoff end` sigue volviendo
  a casa en un solo paso. Encadenar a mano sigue prohibido, igual que prestar
  una sesión a dos perfiles a la vez.
- El bloque de shell añade `prune|sessions` a la lista de passthrough de
  `handoff` — un rc instalado antes los reenviaba como si fueran un perfil
  destino. `ccp session` y `ccp auto` **no** necesitan reinstalarlo: llegan por
  la rama `*) command ccp "$@"` que ya existía.

### Fixed

- El watcher del transcript ya no rebobina con un archivo de tamaño cero. Una
  reescritura no atómica (truncar y escribir) exponía un instante vacío en el
  que el offset caía por debajo de la línea base, y la historia ya descartada se
  releía como un límite de ahora.

## [2.10.0] — renombrar perfiles

### Added

- **`ccp profile rename <viejo> <nuevo>`** (alias `mv`, y tecla `r` en el panel
  de perfiles del TUI). El nombre de un perfil vive en cuatro sitios —la clave
  de `profiles` y el destino de cada regla en `ccp.yaml`, el directorio con la
  api_key y el login, y el `from`/`to` de cada marcador de handoff—, así que el
  rename los mueve todos bajo el mismo criterio y regenera el overlay (el
  `CLAUDE.md` del cc-home lleva la ruta absoluta del perfil dentro).
  Renombrar a mano solo uno de los cuatro fallaba en silencio: una regla
  huérfana no da error, resuelve a `default`.
  El directorio se mueve **antes** de tocar `ccp.yaml` y vuelve a su sitio si la
  escritura falla; los nombres se validan como componente de ruta (nada de `/`
  ni `..`). Las completions bash/zsh completan `rename` y sus perfiles.

## [2.9.0] — handoff multi-activo

### Added

- **Varios handoffs en vuelo a la vez.** `handoffs.yaml` sube a `version: 2` con
  `active` como **lista** de marcadores (migración transparente desde el mapping
  de v1). `end`, `resume` y `discard` resuelven **por el directorio actual**: si
  hay uno solo aquí, ese; si hay varios, picker TUI (o `--session <uuid>` sin
  TTY); si no hay ninguno aquí, el error dice en qué repos sí los hay.
- **`ccp handoff resume [<uuid>]`**: vuelve a entrar a un handoff vivo sin
  cerrarlo — no copia transcripts ni toca el marcador. Es lo que hace útil tener
  varios abiertos.
- **`ccp handoff discard [<uuid>]`**: suelta un marcador zombi (el jsonl del
  destino desapareció) sin back-sync, el caso donde `end` falla siempre.
- **`ccp handoff` sin argumentos y con TTY**: panel gestor con los activos (los
  de este repo primero) — `enter` reanudar · `e` terminar (con confirmación) ·
  `n` nuevo · `y` toggle skip-permissions · `q` salir. Sin activos entra directo
  al wizard de handoff nuevo.
- **`--dangerously-skip-permissions` (alias `--yolo`)** en las tres operaciones
  que lanzan `claude`. No se recuerda entre invocaciones: ni en el marcador ni en
  `ccp.yaml`.
- **`ccp handoff status --all`** agrupado por repo; `list` marca los activos de
  este proyecto.
- **`ccp upgrade --from-source`** (y `CCP_FROM_SOURCE=1` en `install.sh`):
  reinstala compilando el repo registrado en vez de bajar el último release —
  para probar un cambio antes de tagearlo.

### Changed

- La función shell gana las ramas `resume` y `discard` y el `if` de
  `CCP_RESUME_YOLO`; las completions bash/zsh completan los subcomandos de
  handoff. Todo byte-idéntico con el oráculo bash y con el golden regenerado.
- Aviso no bloqueante a partir de 5 handoffs sin cerrar, y recordatorio de una
  línea al entrar (`cd`) a un repo con handoff activo.
- Exit codes de las operaciones de handoff: **0** ok, **1** pre-chequeo, **2**
  fallo de I/O al persistir `handoffs.yaml` (el 2 no se emitía nunca).

### Fixed

- `--session` se valida como uuid antes de tocar disco: un valor con `../`
  copiaba cualquier `.jsonl` legible al perfil destino y dejaba un marcador
  basura.
- Un `handoffs.yaml` escrito por un ccp más nuevo se detecta **antes** de copiar
  nada, y `status`/`list`/panel/hook lo dicen en vez de reportar «no hay handoff
  activo».
- `--force` llega hasta `CopyTranscript` (antes se parseaba y se ignoraba, así
  que el remedio que sugería el error de colisión no funcionaba).
- El picker de sesiones **marca** las que ya están en vuelo en vez de ocultarlas.
- `--no-marker` y `--force` valen también cuando el forward sale del panel.
- Un argumento posicional sobrante ya no se descarta en silencio.

Spec y plan: `docs/superpowers/{specs,plans}/2026-07-25-handoff-multi*`.

## [2.8.3] — parches de handoff e instalación

### Fixed

- `SlugForCwd` aplana **todo** carácter no alfanumérico, igual que Claude Code:
  con un cwd con `.` o `_` el slug no coincidía y ccp no encontraba las sesiones.
- `ccp install` sobre un rc que ya tiene el bloque pero **desfasado** lo reescribe
  en sitio en vez de ser un no-op: antes, una versión que cambiara la función de
  shell dejaba al usuario con la vieja para siempre.

## [2.8.0] — proveedores Kimi/GLM

### Added

- **Presets de proveedor para Kimi (Moonshot) y GLM (Z.ai)**: `ccp profile add
  <n> --kimi` / `--glm`. Cada preset rellena el `ANTHROPIC_BASE_URL` correcto,
  los modelos por defecto y las vars de tuning recomendadas por el proveedor
  (Kimi: `ENABLE_TOOL_SEARCH`, `CLAUDE_CODE_AUTO_COMPACT_WINDOW`; GLM:
  `API_TIMEOUT_MS`, `CLAUDE_CODE_AUTO_COMPACT_WINDOW`). Cualquier campo se
  sobreescribe con `--base-url --pro --flash --effort`.

## [2.7.0] — ccp handoff

### Added

- **`ccp handoff`**: continúa una conversación de Claude Code bajo otro perfil
  sin perder contexto, con tokens frescos del perfil destino. Persiste el
  transcript → cambia de perfil → reanuda la misma sesión en un proceso nuevo
  (`claude --resume`). TUI con pickers de perfil destino y de sesión; equivalente
  por flags (`ccp handoff <to> --session <uuid>`) para no-TTY/scripting.
- **`ccp handoff end`**: back-sync del contexto actualizado destino→origen como
  **sesión nueva** (no destructivo) y relanza en el origen; la sesión de vuelta
  muestra `[de <perfil>]` en su título.
- **`ccp handoff status` / `list`**: handoff en vuelo + historial archivado
  (solo lectura, funcionan sin la función shell).
- Marcador atómico (`handoffs.yaml`), warning no-bloqueante en handoff
  cross-provider, y el `case handoff)` añadido al shell-init byte-idéntico (gate
  de paridad). Ver spec/plan en `docs/superpowers/{specs,plans}/2026-06-19-ccp-handoff*`.

## [2.0.0] — rewrite a Go + cutover

### Changed (breaking en distribución, NO en el contrato observable)

- **Reescritura completa de Bash a Go** (`cmd/ccp` + `internal/{core,cli,tui}`). El
  contrato observable (`_env`, `_hook`, `resolve`, `path test`, `status --json`,
  `completion bash|zsh`, `completion-shellinit`) es **byte-idéntico** al bash,
  garantizado por el gate golden-diff Go↔bash (`internal/golden/parity_test.go`).
- **El bash quedó archivado en `legacy/`** como oráculo del contrato (binario,
  libs y suite de tests). El bloque rc no cambia (sigue llamando `command ccp`),
  así que actualizar NO requiere reinstalar el shell-init.
- **`ccp.yaml` es la fuente de verdad única** (reemplaza `profiles.tsv` +
  `rules.tsv` + `config` + los `meta` + `authored.tsv` global/profile). Schema
  `version: 2`, escritura atómica bajo `flock`, preserva comentarios y claves
  desconocidas. Secretos (`api_key`, OAuth) quedan **fuera** del YAML.
- **Migración automática y respaldada** dsctl→ccp(TSV)→`ccp.yaml` en el primer
  arranque Go (`.backup-pre-go-*`), idempotente y reversible.

### Added

- **`install.sh` Go-aware**: descarga el binario prebuilt por OS/arch del GitHub
  Release y verifica su `sha256` contra `checksums.txt`; fallback `go build` si
  hay toolchain. Limpia las libs bash viejas y re-apunta `install-source`.
- **Pipeline de release** (`.github/workflows/release.yml`): publica binarios
  darwin/linux × amd64/arm64 + `checksums.txt` en cada tag `v*`.
- **`ccp upgrade`** (Go): re-ejecuta `install.sh` + `profile sync` con el binario nuevo.
- **TUI** bubbletea+huh (3 paneles: Perfiles | Reglas | Estado) al correr `ccp`
  sin args con TTY; sin TTY cae a la CLI.
- **`ccp backup export|restore`** (`.tar.gz` + `manifest.yaml`, checksum por
  miembro, restore no-destructivo con snapshot previo).
- Comandos `/ccp:remember-{global,profile,project}`, `/ccp:recall`, `/ccp:forget`
  y la superficie CLI `ccp instruct add|list|rm|dest|record`: capturan artefactos
  (rule/agent/command/hook/mcp/skill) en la estructura oficial de Claude Code.
  Ver docs/adr/0004, 0005, 0006, 0007.

## [2.1.0]
- Autocompletado de shell (`dsctl completion bash|zsh`): subcomandos, llaves de
  config, rutas y verbos de `ds`. Se auto-carga vía el shell init.
- Cache en el hook `_ds_autocheck`: evita el fork de `dsctl _resolve` cuando el
  `$PWD` no cambió desde el último prompt.
- Salida machine-readable: `dsctl status --json`, exit codes en `dsctl path test`
  (0=deepseek, 1=official) y comando público `dsctl resolve`.
- `doctor` reporta si el autocompletado está cargado.
- Fix: `path list` no mostraba reglas en macOS (BSD grep no soporta `grep -P`).
  Reemplazado por filtrado con `read` (portable). El ruteo nunca estuvo afectado.

## [2.0.0]
- Gestión de paths con reglas `include` / `exclude` y precedencia por especificidad.
- Motor de resolución aislado en `lib/paths.sh` (testeable).
- Hook por carpeta basado en `dsctl _resolve "$PWD"`.
- Submenú interactivo de paths; `path list/test/edit/clear`.
- Soporte bash y zsh.

## [1.0.0]
- Versión inicial: función `ds on/off/run`, lista plana de auto-repos,
  key segura, config, doctor, menú.
