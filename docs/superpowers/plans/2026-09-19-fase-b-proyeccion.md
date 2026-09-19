# Fase B: capas unificadas y proyección (incluido Desktop) — plan de implementación

**Goal:** que la configuración se declare una vez por capa (global · perfil · proyecto) y ccp la
proyecte a lo que leen de verdad la CLI, la pestaña Code y el chat de Desktop. Criterio de salida (§12):
un MCP añadido al perfil `work` aparece en `claude` (CLI con ese perfil) y en la ventana de `work` tras
`ccp profile sync`.

**Spec:** §2 (principio y reglas), §6.1–§6.4, §15. **ADR 0016** manda en todo lo que toca Desktop:
- la pestaña Code lee todo el cc-home y hereda los MCP del chat (M1, M2);
- `claude_desktop_config.json` solo admite stdio y no se relee en caliente (M3);
- Desktop reescribe ese archivo mientras corre (M5);
- Claude Code conserva lo escrito desde fuera en `.claude.json` (M6, D7 = técnica A).

**Architecture (decisiones tomadas; no se reabren en las tareas):**
- Capas de MCP:
  - global = `mcpServers` de `~/.claude.json` (`src + ".json"`, el scope user oficial de `default`);
  - perfil = `profiles/<n>/overlay/mcp.json` con la forma de `.mcp.json` (`{"mcpServers": {…}}`);
  - proyecto = `<repo>/.mcp.json`, que ccp lista pero NO proyecta: Claude Code lo lee solo, por cwd.
- Metadatos en `ccp.yaml`, bloque aditivo `mcp:` (version sigue en 2; viaja por `Config.Extra`, con
  su propio `Extra` inline y `stripMCPKnownKeys`, igual que `auto_handoff`):
  `targets: {<server>: [cli, desktop]}` (por defecto `[cli, desktop]`),
  `disabled: {<perfil>: [<server>…]}` y `desktop_default: false`.
- Efectivo por perfil = global ⊕ perfil − disabled; si el nombre choca, gana el perfil.
- **Proyección al CLI (técnica A):** se fusionan en `cc-home/.claude.json:mcpServers` solo los
  nombres gestionados, con read-modify-write atómico y bajo el flock de ccp, conservando el resto de
  claves y el modo. Los nombres gestionados van en `cc-home/.ccp-managed.json` (derivado: si se
  pierde, se reconstruye como «nombres cuyo valor actual es idéntico al declarado»).
  - Un nombre que ya existe a mano en el destino con otra forma **no se pisa**: se informa como
    conflicto `mcp_unmanaged_conflict`.
  - `default` no se proyecta: su destino es la propia capa global.
- **Proyección al chat de Desktop:** misma regla de nombres gestionados, en
  `profiles/<n>/desktop/claude_desktop_config.json`, conservando `preferences`, `coworkUserFilesPath` y
  toda clave desconocida. Gestionados en `profiles/<n>/desktop/.ccp-managed-mcp.json`. Solo stdio: un
  `http`/`sse` con destino `desktop` NO se escribe y se informa «en el chat solo como conector de la
  cuenta». El puente `mcp-remote` queda fuera: metería cabeceras con tokens en los argumentos del
  proceso.
  - **Con la ventana corriendo no se escribe:** se marca `profiles/<n>/state/desktop-pending.json` y
    se aplica en el siguiente arranque. `RunDesktopLauncher`, el Dock, y `ccp desktop open` proyectan
    justo antes de lanzar, donde ya espejan el cc-home. Es la lectura prudente de M5.
  - `default` solo con `mcp.desktop_default: true`.
- Un servidor con destino `[cli, desktop]` va a los dos archivos. En la pestaña Code gana la copia de
  Desktop y la otra queda ociosa (ADR 0016, decisión 2).
- Snapshots y backups:
  - `overlay/mcp.json` es clase `secret` en los snapshots (lleva `env`/`headers`), sellado.
  - En `ccp backup export` solo entra con `--with-secrets`, como `api_key`.
  - `overlay/{skills,agents,commands,output-styles}/` entra siempre en backups (authored) y ya lo
    capturan los snapshots por el árbol de overlay.

**Global constraints:** las de siempre (commits por tarea sin trailers, TDD, bilingüe, estado real
intocable, golden intacto, `go 1.24.0`, cargo no disponible). Toda escritura sobre un archivo que
también escriben Claude Code o Desktop va por tmp+rename y conserva el modo. Nada de lo que el usuario
escribió a mano en un destino se borra ni se pisa.

---

### Task 1: el bloque `mcp:` de `ccp.yaml`
**Files:** `internal/core/config.go` (o donde viva `Config`), `internal/core/store.go`
(`knownTopKeys` + `stripMCPKnownKeys`), tests en `store_test.go`.
- `MCPConfig{Targets map[string][]string; Disabled map[string][]string; DesktopDefault bool; Extra map[string]any `yaml:",inline"`}`.
- Round-trip: una clave desconocida dentro de `mcp:` y otra al lado sobreviven a Load/Save; sin bloque
  no se serializa nada (un ccp.yaml viejo queda byte a byte igual tras Save).
- `MCPTargets(cfg, server) []string` con el valor por defecto `[cli, desktop]` y validación
  (solo `cli`/`desktop`).

### Task 2: capas y efectivo (puro)
**Files:** Create `internal/core/mcp_layers.go` + test.
- `MCPServer` = el objeto JSON tal cual (`map[string]any`, con `UseNumber`), más helpers `mcpKind`
  (stdio si hay `command`; `http`/`sse` por `type` o `url`).
- `ReadMCPLayers(home, src, name)`: global de `src+".json"` (solo `mcpServers`), perfil de
  `overlay/mcp.json`. Un archivo inválido → error con su ruta (el que regenera no puede proyectar a
  ciegas).
- `MCPEffective(cfg, global, profile, name) []EffMCP{Name, Def, Origin (global|overlay), Targets, Shadowed}`
  ordenado por nombre, aplicando `disabled`.
- `snapshot_layout.go`: `overlay/mcp.json` → `ClassSecret`, y test que lo compruebe.
  `backup.go`: `overlay/mcp.json` solo con `--with-secrets`, y `overlay/{skills,agents,commands,output-styles}`
  siempre, con su ruta permitida en el restore (`dacba0f` endureció las rutas: añade los prefijos
  nuevos a la lista cerrada, sin abrirla).

### Task 3: proyección al CLI
**Files:** Create `internal/core/mcp_project.go` + test.
- `ProjectMCPToCLI(home, name, eff) (MCPProjection, error)`, según la arquitectura: nombres
  gestionados, conflictos con entradas a mano, borrado de los gestionados que ya no son efectivos, tmp+rename
  conservando modo, `.ccp-managed.json` y reconstrucción si falta.
- `MCPProjection{Written, Removed, Conflicts, RemoteSkipped []string; DesktopPending bool}` (se
  reutiliza en la Task 4).
- Tests: añade, actualiza, quita, respeta la entrada a mano con el mismo nombre (conflicto), conserva
  `oauthAccount`/`projects`, idempotente (segunda proyección no reescribe si nada cambia: compara bytes).

### Task 4: proyección al chat de Desktop
**Files:** `internal/core/mcp_project.go` (+ test), `internal/core/desktop_launch.go` o
`internal/cli/desktop_launcher.go` (aplicar lo pendiente al arrancar), `internal/cli/desktop.go`
(`desktop open`).
- `ProjectMCPToDesktop(home, name, eff, running bool) (MCPProjection, error)`: si `running`, no escribe y
  deja `state/desktop-pending.json`. Si no, escribe los stdio gestionados y borra el pendiente.
- `running` lo decide el llamador con la sonda que ya existe (`desktopInstanceRunning`/preflight). Core
  no ejecuta `ps`.
- Al lanzar (Dock y `desktop open`), antes del `exec`/`open`, se proyecta con `running=false`.
- Tests: conserva `preferences` y claves desconocidas; http no se escribe y sale en `RemoteSkipped`;
  corriendo → pendiente y archivo intacto; lanzar aplica el pendiente.

### Task 5: la proyección entra en la regeneración
**Files:** `internal/core/cfg.go` (`CfgRegenerateReport`), `internal/core/cfg_drift.go`
(`SettingsDrift` gana `MCP *MCPProjection` y `Empty` lo tiene en cuenta), CLI `printSettingsDrift`,
serve `profiles.sync` (campos aditivos), `catalog_cli.go`.
- Tras settings y CLAUDE.md: `ReadMCPLayers` → `MCPEffective` → CLI y, si el perfil tiene data dir
  de Desktop, Desktop con `running` inyectado. La sonda de procesos vive en `internal/cli`: core recibe
  un `func(profile string) bool` como variable de paquete ajustable (patrón `SetAutoHooksBin`), con
  valor por defecto «no corriendo».
- Con esto, B1 de fondo: «global» significa «proyectado a todos los perfiles».
- Test del **criterio de salida**: un MCP en `overlay/mcp.json` de `work` → tras `ProfileSyncReport`
  está en `cc-home/.claude.json` y en `desktop/claude_desktop_config.json` de `work`, y no en
  `~/.claude.json`.

### Task 6: `instruct` escribe MCP y artefactos de perfil
**Files:** `internal/core/instruct.go` (+ test), `legacy/bin/ccp` (texto de los códigos que
desaparecen: el oráculo no conoce la capa nueva, así que ahí solo se cambia la pista para que no
diverjan los mensajes compartidos).
- `InstructDest("profile","mcp")` → `overlay/mcp.json` (el código 5 desaparece).
- `profile agent|command|skill` → `overlay/<dir>` (el código 3 desaparece).
- `InstructAdd` de un MCP de perfil escribe en `overlay/mcp.json` y regenera.
- Los tests que afirmaban los códigos 3 y 5 se actualizan, incluido `TestInstructDestProfileMCPHintIsHonest`.

### Task 7: skills, agents, commands y output-styles por perfil (§6.2)
**Files:** `internal/core/desktop.go` (`mirrorTree` con dos fuentes), `internal/core/cfg.go`
(regenerar), tests.
- Si `overlay/<dir>` tiene contenido, `cc-home/<dir>` pasa a ser espejo de global ∪ overlay: directorios
  reales, symlinks solo en las hojas y, si chocan, gana el overlay. Si `overlay/<dir>` está vacío o no
  existe, no se toca nada. Así `seedCCHome` y el contrato del oráculo (`[[ -L "$cch/plugins" ]]` tras
  `profile add`) siguen iguales.
- Idempotente, respeta lo que el usuario puso a mano en `cc-home/<dir>` (misma regla que
  `MirrorForDesktop`) y poda solo enlaces colgados que apunten dentro de las fuentes.

### Task 8: permisos con unión opcional y hooks editables por capa (§6.3)
**Files:** `internal/core/cfg.go` (`MergeJSON` o una capa previa), `internal/core/effective.go`,
`internal/core/settings_layers.go` (nuevo) + tests; nota en `docs/adr/0002-*.md` o ADR nuevo.
- `"$merge": "union"` dentro de `permissions` del overlay: `allow`/`deny`/`ask` se unen
  (global primero, sin duplicados) y la clave `$merge` no llega al generado. Sin la marca, el
  comportamiento de ADR 0002 no cambia; test que lo fije.
- La vista efectiva distingue «reemplaza la lista global entera» de «se une».
- `SettingsLayerGet/Set(home, layer /* global|profile:<n> */, path []string, value any)`: lectura y
  escritura atómica de una clave de settings de una capa (global = `~/.claude/settings.json`; perfil =
  overlay). Los hooks se editan como el array completo de un evento, lo que resuelve «se añaden pero no
  se borran». La usarán la Fase C y la TUI.

### Task 9: deriva y diagnóstico (§6.4)
**Files:** `internal/core/mcp_project.go` (modo `check`), `internal/core/doctor.go`,
`internal/core/effective.go`, CLI `profile sync --check`, serve `profiles.drift`, catálogos.
- `ProfileProjectionCheck(home, name)`: lo que la proyección cambiaría, sin escribir.
  `ccp profile sync --check [n]` sale 1 si hay algo desfasado.
- Doctor: `mcp_unmanaged_only_desktop`, `mcp_command_missing`, `projection_stale`,
  `desktop_restart_pending` y `cc_home_symlink_nonleaf` (un symlink de directorio bajo un cc-home
  que usa Desktop).
- `EffRow` gana `AppliesTo []string` (cli, desktop-code, desktop-chat) según ADR 0016; serve lo expone.

### Task 10: documentación
- ADR 0011 «Una fuente declarada y varias proyecciones»: MCP por capas, nombres gestionados,
  Desktop con la ventana cerrada y stdio solo. Cita ADR 0016 y 0002.
- README y README.es: MCP por perfil (`instruct add profile mcp`), qué llega a CLI, Code y chat.
- CHANGELOG, CLAUDE.md (arquitectura: `mcp_layers.go`, `mcp_project.go`, reglas) y spec §6 y §12 con
  la nota «Implementado».
