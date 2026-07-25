# Changelog

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
