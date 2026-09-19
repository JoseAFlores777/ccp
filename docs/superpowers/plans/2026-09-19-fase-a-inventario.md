# Fase A: inventario y adopción («detectar la máquina») — plan de implementación

**Goal:** que ccp vea toda la configuración de Claude de la máquina (la suya y la que no gestiona), la
clasifique con las clases del spec §2 y diga dónde aplica cada cosa (CLI · Code · Chat, ADR 0016), y
que proponga y aplique, en orden y con red, la adopción de lo que está fuera. Criterio de salida (§12):
en esta máquina aparecen los MCP de la ventana `default` de Desktop y se proponen como global.

**Spec:** `docs/superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md` §2 (clases),
§5 (A), §6.1 (qué es «global» para un MCP), §11 (identidad portable de proyectos). ADR 0016 para
«dónde aplica». D5: adoptar un `~/.claude-x` = copiar + un `/login`.

**Architecture:**
- `internal/core/inventory.go` — `Inventory(r InventoryRoots) Inventory`: solo lectura, raíces
  inyectadas (no lee `os.Getenv` ni `os.UserHomeDir`), `LookPath` inyectado. Hereda la regla del
  doctor: **una fuente que no se puede leer produce `unknown`, jamás vacío** (`Inventory.Probes`).
- `internal/core/adopt.go` — `AdoptPlan(inv Inventory, cfg *Config) []AdoptStep` puro, y
  `AdoptApply(home string, r InventoryRoots, steps []AdoptStep, o AdoptApplyOpts) (*AdoptReport, error)`.
  El snapshot de seguridad entra como `o.Before func() error` para que core no dependa de la CLI
  (la CLI y serve pasan `autoSnapshot(home, "pre-adopt")`).
- Superficie: `ccp scan`, `ccp adopt`; serve `inventory.scan`, `adopt.plan`, `adopt.apply`; GUI P-19.

**Global constraints:** las del resto de planes (commits por tarea, TDD, bilingüe, estado real
intocable, golden intacto, `go 1.24.0`). Además:
- **Nunca un valor secreto en el inventario**: de `env.*` y `headers.*` de un MCP, y de `env` de un
  settings, solo las rutas (`Item.Secrets`), nunca los valores. Un test lo comprueba serializando el
  inventario de un árbol con un token falso y buscándolo.
- `scan` y `adopt` **no** entran en la completion (como `snapshot`, `backup`, `serve`).
- `adopt` escribe en `~/.claude.json`, que Claude Code reescribe a menudo: solo por
  `ClaudeJSONApplyConfig`-style read-modify-write con tmp+rename, conservando todas las demás claves y
  el modo del archivo (M6 mide que Claude Code conserva lo escrito desde fuera).

---

### Task 1: tipos y raíces; el recorrido de ccp y del global de Claude Code

**Files:** Create `internal/core/inventory.go`, `internal/core/inventory_test.go`.

- Tipos, con etiquetas JSON en snake_case (son el contrato de `scan --json` y de serve):
  - `InventoryRoots{Home, CCPHome, ClaudeSrc, DesktopDefaultDataDir, ManagedDir string; RCFiles []string; LookPath func(string) (string, error)}`.
    `ClaudeSrc` es `~/.claude` (o `CCP_CLAUDE_SRC`); el `~/.claude.json` es `ClaudeSrc + ".json"`
    (igual que `InstructDest`). `DesktopDefaultDataDir` es el data dir de la ventana `default`.
  - `InvScope{Level string /* global|profile|project|desktop|managed|plugin|account */; Name string}`.
  - `InvItem` con los campos del spec §5.1: `Kind, Scope, Name, Source, Class, Managed, Editable, Why,
    AppliesTo []string, Hash, Secrets []string` (+ `Key string`: la clave JSON dentro de `Source`
    cuando el elemento es una clave de un archivo).
  - `InvProbe{Source string; Status string /* ok|missing|unknown */; Err string}`.
  - `Inventory{Items []InvItem; Probes []InvProbe}` — siempre slices no nil.
- Recorrido de esta tarea:
  - **ccp**: `Load(CCPHome)` → un `profile` por perfil (+ `default`), un `rule-path` por regla; cada
    perfil con su overlay (`CLAUDE.md` → `rule-instr` managed; `settings.overlay.json` clave a clave →
    `settings-key`/`env`/`permission`/`hook`/`statusline`/`output-style`) con `Scope{profile, n}`.
  - **Global** (`ClaudeSrc`): `settings.json` clave a clave (misma clasificación), `CLAUDE.md`,
    `agents/`, `commands/`, `skills/<n>/SKILL.md`, `output-styles/`, `hooks/` (archivos),
    `keybindings.json`, `plugins/installed_plugins.json` (un `plugin` por entrada, marcando los de
    `enabledPlugins`).
- Clases (§2): lo escrito por el usuario `authored`; los valores de `env` → el item es `authored` pero
  lleva `Secrets: ["env.X"]` y `Hash` de la forma, no del valor.
- `AppliesTo`: lo del global y del cc-home → `cli` + `desktop-code` (ADR 0016 M1); nada de settings
  aplica a `desktop-chat`.
- `Hash`: sha256 corto del contenido normalizado (para detectar duplicados entre capas en la Task 4).
- Un archivo que existe y no se puede leer o no es JSON → `InvProbe{Status: unknown}` y ningún item de
  él; uno que no existe → `missing` (no es error).

**Tests:** árbol falso con global + un perfil official con overlay; se comprueban kinds, scopes,
clases, `AppliesTo`, que `unknown` aparece con un `settings.json` roto, y que un `env` con
`sk-FAKE-123` no aparece en `json.Marshal(inv)`.

### Task 2: MCP en todas sus fuentes, con «dónde aplica»

**Files:** `internal/core/inventory.go` (+ test).

- Fuentes y `AppliesTo` (ADR 0016):
  - `~/.claude.json` `mcpServers` → `Scope{global}`, aplica a `cli` (perfil `default`) y a
    `desktop-code` de la ventana `default`. Por proyecto (`projects.<ruta>.mcpServers`) →
    `Scope{project, <ruta>}`.
  - `cc-home/.claude.json` de cada perfil → `Scope{profile, n}`, `cli` + `desktop-code` de su ventana.
  - `claude_desktop_config.json` de la ventana `default` (`DesktopDefaultDataDir`) y de cada perfil
    (`DesktopDataDir(home, n)`) → `Scope{desktop, n}` (`default` para la principal), aplica a
    `desktop-chat` y `desktop-code` de esa ventana (el pool compartido, M2). Solo las entradas stdio son
    válidas ahí: una `http`/`sse` sale con `Why: "Desktop solo carga stdio en este archivo (M3)"`.
  - `.mcp.json` de cada proyecto conocido → `Scope{project}`, `cli` + `desktop-code`.
  - Managed (`ManagedDir/managed-mcp.json`) → `Editable: false`, `Why: "managed-settings"`.
  - Plugins activos: los `.mcp.json` que traiga cada plugin instalado y activo → `Scope{plugin, id}`,
    `Editable: false`, `Why: "lo trae el plugin <id>"` (si el plugin no está en disco, se omite).
- `Name` = nombre del servidor; `Key` = `mcpServers.<name>`; `Secrets` = rutas `env.*` y `headers.*`
  presentes; `Hash` = sha256 de `{type,command,args,url}` (sin secretos): dos entradas con la misma
  forma tienen el mismo hash aunque cambien los tokens.
- `InvItem` gana `Missing string` para un MCP stdio cuyo `command` no resuelve con `LookPath` (absoluto
  inexistente o no en PATH): alimenta los pendientes de la Task 4.

**Tests:** árbol con `claude_desktop_config.json` de `default` con dos MCP (uno con `env.TOKEN`) y una
entrada http; `~/.claude.json` sin `mcpServers`; un perfil con un MCP en su cc-home. Se comprueban
scopes, `AppliesTo`, el `Why` de la http, `Secrets` sin valores y `Missing` con `LookPath` falso.

### Task 3: proyectos y directorios de config sin gestionar

**Files:** `internal/core/inventory.go` (+ test).

- Proyectos conocidos: los de las reglas de carpeta + las claves `projects` de `~/.claude.json` y de
  cada `cc-home/.claude.json` que existan en disco. De cada uno: `.claude/settings.json`,
  `.claude/settings.local.json`, `.mcp.json`, `CLAUDE.md`, `CLAUDE.local.md`, `.claude/{agents,commands,skills}`.
  Identidad portable: reutiliza `projectKey`/`gitOriginURL` de `snapshot_layout.go` y guárdala en un
  campo `Project` del item (`{path, key, remote}`).
- Directorios de config sin gestionar (`Kind: config-dir`, `Scope{global}`): hijos de `Home` que
  empiezan por `.claude` (no `.claude` ni `.claude.json` mismos) y que parecen un `CLAUDE_CONFIG_DIR`
  (tienen `settings.json`, `.claude.json` o `projects/`), y que no son el cc-home de ningún perfil. Más
  las líneas `export CLAUDE_CONFIG_DIR=...` de `RCFiles` (con `~`/`$HOME` expandidos contra `Home`).
  Cada uno con `Why` = lo que contiene (resumen) y la cuenta de MCP/skills/agents que traería.
- Reglas huérfanas: un `rule-path` cuya ruta no existe → `Why: "la carpeta no existe"`.

**Tests:** árbol con `~/.claude-work/{settings.json,.claude.json,agents/a.md}`, un rc con
`export CLAUDE_CONFIG_DIR="$HOME/.claude-alt"`, un proyecto con `.mcp.json` y una regla huérfana.

### Task 4: el plan de adopción (puro)

**Files:** Create `internal/core/adopt.go`, `internal/core/adopt_test.go`.

- `AdoptStep{ID, Order int, Kind, Title, Detail, From, To, Key string; Items []string; Default bool; Pending bool}`
  con `Kind` en: `adopt-config-dir`, `lift-mcp-global`, `rule-orphan`, `desktop-launcher`, `login`,
  `api-key`, `mcp-command-missing`. `Key` es la clave JSON dentro de `From`/`To` que el paso mueve
  (`mcpServers.<name>` en `lift-mcp-global` y `mcp-command-missing`; vacía si el paso es el archivo o
  el directorio entero), igual que `InvItem` separa `Source` de `Key`.
- `ID` estable para seleccionar pasos: hash corto de kind+from+to+key+`Items` ordenados, cada campo
  separado por un byte `\x00` (así `a`+`bc` y `ab`+`c` no dan el mismo hash). Kind+from+to **no
  basta**: todos los `lift-mcp-global` salidos del mismo `claude_desktop_config.json` comparten los
  tres (mismo archivo, mismo `~/.claude.json`), y con un ID repetido `ccp adopt --only <id>` aplicaría
  todos — p. ej. subiría `github` con su `env.GITHUB_TOKEN` al pedir solo `filesystem` — y las casillas
  de la GUI, indexadas por ID, quedarían enlazadas. Si aun así dos pasos coinciden en todo, son el mismo
  paso: `AdoptPlan` los deduplica en vez de emitir dos con el mismo ID.
- Orden del spec §5.2: perfiles (config dirs) → reglas → capas → Desktop → pendientes. La proyección
  a perfiles es la Fase B: aquí no hay pasos de proyección.
- **Capas (lo que pide el criterio de salida):** un MCP que está en uno o más `claude_desktop_config.json`
  y en **ningún** destino de CLI (`~/.claude.json` ni `cc-home/.claude.json`) → paso
  `lift-mcp-global` («subir a global»: a `~/.claude.json mcpServers`) si está en la ventana `default`
  o en todas las ventanas; si solo está en la ventana de un perfil, paso `Pending` «subir a perfil:
  llega con la Fase B». Mismo nombre con distinta forma (`Hash`) en dos ventanas → no se sube: paso
  pendiente «conflicto de nombre».
- `adopt-config-dir`: nombre de perfil derivado del sufijo (`~/.claude-work` → `work`), único frente a
  los existentes (`work-2`…), validado con `validProfileName`; `Detail` lista qué se copia (config, no
  tokens) y que hará falta un `/login`.
- Pendientes (`Pending: true`, no se aplican): perfiles official sin `HasLogin`, providers sin
  `api_key`, MCP con `Missing`, lanzadores de Desktop que faltan (`DesktopAppList`/manifest; se
  ofrecen, no se construyen).
- `Default`: `true` para `lift-mcp-global` y `adopt-config-dir`; los pendientes no son seleccionables.

**Tests:** el árbol del criterio de salida (MCP en `default` de Desktop, `~/.claude.json` vacío) da
exactamente un `lift-mcp-global` por MCP y ninguno para uno que ya está en `~/.claude.json`;
conflicto de forma; nombre único para `.claude-work` cuando `work` ya existe; orden estable; **dos MCP
del mismo `claude_desktop_config.json`** (`filesystem` y `github`) dan dos `lift-mcp-global` con
`Key` distinta e **IDs distintos**, y los IDs de todo el plan son únicos (se comprueba recorriéndolo).

### Task 5: aplicar la adopción

**Files:** `internal/core/adopt.go` (+ test).

- `AdoptApplyOpts{Only []string /* IDs; vacío = los Default */; Before func() error; Now time.Time}`.
- Primero `Before()` (el snapshot de seguridad): si falla, no se aplica nada.
- `lift-mcp-global`: lee `~/.claude.json` (puede no existir: se crea `0600`), añade las entradas que
  falten con su forma exacta (secretos incluidos: se copian tal cual de donde ya estaban en claro), sin
  pisar un nombre que ya exista, y escribe con tmp+rename conservando el modo. Nunca toca otras claves.
- `adopt-config-dir`: `ProfileAddOfficial(nuevo)`; copia `settings.json` del directorio como
  `overlay/settings.overlay.json` (sin `env`: va a `Report.Skipped` con aviso, mismo motivo que B6);
  su `CLAUDE.md` como `overlay/CLAUDE.md` (fuera del bloque gestionado); `agents/`, `commands/`,
  `skills/`, `output-styles/` a `overlay/<dir>/` (la capa de perfil de §6.2, que proyecta la Fase B);
  de su `.claude.json` solo la parte de configuración (`ClaudeJSONConfig`) aplicada al
  `cc-home/.claude.json` nuevo con `ClaudeJSONApplyConfig`. Nunca tokens ni `projects` con historial.
  El original no se toca. Al final `CfgRegenerate` del perfil nuevo y pendiente `login`.
- `AdoptReport{Applied, Skipped []string; Pending []AdoptStep; Snapshot string}`.

**Tests:** aplicar el plan del árbol del criterio de salida deja los MCP en `~/.claude.json` con las
demás claves intactas y el modo conservado; aplicarlo dos veces no duplica; `Before` que falla no
escribe nada; con dos MCP del mismo archivo, `Only: []string{<ID de filesystem>}` sube
**exactamente** `filesystem` y deja `github` (y su token) fuera de `~/.claude.json`; adoptar `~/.claude-work` crea el perfil con overlay y cc-home, sin tokens y con el
original intacto.

### Task 6: CLI `ccp scan` y `ccp adopt`

**Files:** Create `internal/cli/scan.go`, `internal/cli/scan_test.go`,
`internal/core/i18n/catalog_inventory.go`; modify `internal/cli/cli.go` (dispatch + `dailySnapshotCmds`
gana `adopt`), `internal/core/i18n/catalog_cli.go` (sección en `cli.help.body`, En y Es).

- `inventoryRoots()` en la CLI: `HOME`, `CCP_HOME`/`ccpHome()`, `claudeSrc()`,
  `CCP_DESKTOP_DEFAULT_DATA_DIR` o el data dir real de `default`, `CCP_MANAGED_DIR` o
  `/Library/Application Support/ClaudeCode`, los rc habituales (`.zshrc`, `.zprofile`, `.bashrc`,
  `.bash_profile`) y `exec.LookPath`.
- `ccp scan [--json]`: texto agrupado perfil → tipo → elemento, con procedencia y dónde aplica; al
  final las sondas `unknown`. `--json` = `Inventory` (siempre arrays).
- `ccp adopt [--dry-run] [--only <id|kind>]... [--yes] [--json]`: como `snapshot restore`: sin `--yes`
  enseña el plan (con los IDs) y sale 1; `--dry-run` sale 0; con `--yes` aplica con
  `withSafetySnapshot(home, "pre-adopt", …)` como `Before` y cuenta el informe.

**Tests:** `scan --json` sobre un árbol falso (roots por variables de entorno temporales) es JSON
válido con arrays; `adopt` sin `--yes` sale 1 y no escribe; con `--yes` escribe y toma el snapshot
(con `CCP_NO_AUTO_SNAPSHOT=""`).

### Task 7: serve `inventory.scan`, `adopt.plan`, `adopt.apply`

**Files:** Create `internal/cli/serve_inventory.go`; modify `serve_methods.go` (registro:
`inventory.scan` y `adopt.plan` lectura, `adopt.apply` escritura), `serve_test.go`.

- `adopt.apply {only?: string[]}` usa `autoSnapshot(home, "pre-adopt")` como `Before` y devuelve el
  informe. Mismas formas JSON que la CLI.

### Task 8: GUI P-19 «Detectar esta máquina»

**Files:** Create `gui/src/screens/Detectar.tsx`; modify `gui/src/App.tsx` (`bienvenida: Detectar`),
`gui/src/lib/api.ts` (tipos + `inventoryScan`, `adoptPlan`, `adoptApply`), `gui/src/lib/i18n_en.ts`,
y un botón «Detectar esta máquina» en `gui/src/screens/Ajustes.tsx`.

- Tres bloques: lo encontrado (perfil → tipo → elemento, con procedencia y distintivos CLI · Code ·
  Chat, y los `unknown` destacados), el plan con casillas (los `Default` marcados; los pendientes sin
  casilla, con qué hacer) y el resultado. Cada bloque enseña su CLI equivalente (`ccp scan`,
  `ccp adopt --dry-run`, `ccp adopt --only … --yes`).
- Sigue el estilo de las pantallas que ya existen (componentes de `gui/src/components`), sin librerías
  nuevas. `npm run typecheck` en verde.

### Task 9: documentación

- README.md y README.es.md: sección «Detectar la máquina» breve (scan, adopt, qué se adopta y qué no).
- CHANGELOG (Unreleased → Added). CLAUDE.md: `inventory.go` y `adopt.go` (reglas: raíces inyectadas,
  `unknown` jamás vacío, secretos solo por ruta, adopción copia y no mueve, snapshot antes).
- Spec §5 y §12: nota «Implementado (plan 2026-09-19-fase-a-inventario)».
