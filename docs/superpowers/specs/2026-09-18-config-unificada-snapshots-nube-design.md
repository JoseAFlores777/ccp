# Configuración unificada, snapshots y nube — diseño

Fecha: 2026-09-18 · Estado: **aceptado** 2026-09-18 (paraguas; cada subproyecto tendrá su spec y su plan) ·
ADRs a escribir: 0011–0016 (§14)

Planes de implementación:
- [Fase 0: mediciones de Desktop y defectos B1-B5](../plans/2026-09-18-fase0-mediciones-y-defectos.md)
- [D: snapshots locales](../plans/2026-09-18-snapshots-locales.md)
- [F1: bóveda, dispositivos y sincronización](../plans/2026-09-18-nube-f1-boveda-y-sync.md) (depende de D)
- Infraestructura (I): desplegada el 2026-09-18 desde `deploy/ccp-cloud/`

## 0. Qué se pide

1. **Detectar la máquina.** Al arrancar, la app encuentra toda la configuración de Claude que hay:
   perfiles de ccp ya existentes, `~/.claude`, instancias de Desktop, MCP, skills, agents, commands,
   hooks, permisos, reglas. Lo importa **en orden** y lo presenta ordenado en la UI.
2. **Leer y editar todo desde la UI**, en sus tres niveles: global, perfil y proyecto.
3. **Que lo configurado para el CLI llegue también a Desktop.** Ejemplo: un MCP definido en el
   perfil `work` lo ven tanto `claude` en la terminal como la ventana de Desktop de `work`.
4. **Snapshots de toda la configuración**, guardados en local como copia de seguridad.
5. **Nube**: un backend con usuarios y credenciales para:
   - subir esos snapshots;
   - traerlos a otra máquina y replicarlos;
   - gobernar todas las máquinas desde un portal con una sola cuenta.

> Terminología: en el pedido «RPCs»/«MCT» se lee como **MCP** y «CIELAI» como **CLI**.

## 1. Punto de partida (verificado en el código, 2026-09-18)

| Capacidad | Hoy | Hueco |
|---|---|---|
| Inventario de la máquina | `Doctor` y `diag.run` solo miran ccp: PATH, login, reglas huérfanas, préstamos | Nada enumera la config de Claude Code. Ningún `ReadDir` sobre `skills/`, `agents/` ni `commands/`, y nadie lee `mcpServers` para listarlo |
| Onboarding | Tarjeta «Para empezar» en Inicio (`gui/src/screens/Inicio.tsx:129`). La ruta `bienvenida` existe pero nada navega a ella (`store.tsx:18`) | Falta el flujo de detección e importación |
| Vista efectiva | `ProfileEffective` (`core/effective.go:91`) con 6 secciones | No cubre MCP, `permissions.deny/ask/defaultMode`, `statusLine`, `outputStyle` ni `model` |
| MCP | `instruct add` escribe en global (`~/.claude.json`) y en proyecto (`.mcp.json`). En perfil está prohibido (`instruct.go:91`, código 5) | Ver B1: el «global» no llega a ningún perfil `official` |
| Skills, agents, commands | Compartidos desde global por symlinks (`seedCCHome`) o por el espejo de Desktop (`MirrorForDesktop`). En perfil están prohibidos (código 3) | No hay skills ni agents propios de un perfil |
| Hooks | Se pueden añadir. No se pueden borrar: viven en arrays sin id | Editar el array entero de cada evento |
| Desktop, pestaña Code | Recibe `CLAUDE_CONFIG_DIR=<cc-home>`, el mismo que el CLI | Solo está **medido** que lee `projects/` y `sessions/`. Lo demás se supone |
| Desktop, chat | `<data-dir>/claude_desktop_config.json` por instancia. ccp **nunca** lo lee ni lo escribe | Hoy los MCP no pasan de un perfil a su ventana |
| Backup | `.tar.gz` con manifiesto y sha256. Incluye `ccp.yaml` y `overlay/`, más `api_key` y `.claude.json` con `--with-secrets`. El restore no destruye nada (`core/backup.go`) | Nada de `~/.claude`, ni del resto del cc-home, ni de Desktop, ni de los repos. Ver B2 |
| Nube | — | Todo |

Estado real de esta máquina, para calibrar:
- `~/.claude.json` no tiene MCP de nivel usuario.
- El `claude_desktop_config.json` de `default` tiene seis: `MCP_DOCKER`, `dokploy-mcp`,
  `seminario`, `seminario-local`, `obsidian-vault` y `finance-os`.
- `e-cc` tiene uno; `a-cc` y `personal-cc`, ninguno.
- Es decir: hoy los MCP viven **en Desktop, no en el CLI**, y cada ventana tiene los suyos.

### Defectos encontrados de camino (se arreglan en la Fase 0)

> **B1–B5 arreglados** en la rama `fix/fase0-defectos` (plan 2026-09-18-fase0-mediciones-y-defectos). B1 solo
> en su pista: el arreglo de fondo es el subproyecto B. B4 llega a los perfiles ya creados por `profile sync`.
> B5 enseña además `permissions.ask`, y en la TUI `e` sobre los MCP explica dónde viven en vez de abrir el overlay.

- **B1 — El MCP «global» no es global.** `InstructDest("global","mcp")` escribe `src+".json"`,
  es decir `~/.claude.json` (`instruct.go:64`). Ese archivo solo lo lee `default`: cada perfil
  `official` lee **su** `cc-home/.claude.json`. La pista del error de código 5 dice «Usa global
  (todos los perfiles)», y no es verdad.
- **B2 — El restore no regenera.** `BackupRestore` → `applyProfile` llama a `seedCCHome` pero nunca a
  `CfgRegenerate` (`backup.go:405`). Tras restaurar, `cc-home/settings.json` y `CLAUDE.md` quedan
  viejos hasta que alguien ejecuta `ccp profile sync`.
- **B3 — `HasLogin` es débil.** Se decide porque existe `.claude.json` (`doctor.go:25`), y Claude Code
  crea ese archivo aunque no se haya iniciado sesión.
- **B4 — Faltan cosas por sembrar.** `seedCCHome` no siembra `output-styles/`, `hooks/` (los scripts)
  ni `keybindings.json`. Los perfiles no heredan estilos de salida.
- **B5 — `ProfileEffective` ciega** a MCP y al resto de claves de la tabla de arriba.

## 2. Principio de arquitectura: una fuente declarada, varias proyecciones

ccp ya hace esto con `settings.json`: global ⊕ overlay ⊕ capa auto → `cc-home/settings.json`
generado. El diseño **generaliza ese patrón** a todo lo demás y añade un destino nuevo, Desktop:

```
 CAPAS DECLARADAS (lo que edita el usuario)          DESTINOS (lo que leen las apps)
 ─────────────────────────────────────────           ────────────────────────────────────────
 global    ~/.claude/…, ~/.claude.json        ┐       CLI + pestaña Code de Desktop
 perfil    profiles/<n>/overlay/…             ├─► ─►  profiles/<n>/cc-home/{settings.json,
 auto      (sensores de ccp)                  ┘          CLAUDE.md, .claude.json:mcpServers,
                                                         skills/ agents/ commands/ …}
 proyecto  <repo>/.claude/…, <repo>/.mcp.json  ──►    lo lee Claude Code solo, por cwd (ccp no proyecta)
                                                  ─►  chat de Desktop
                                                       profiles/<n>/desktop/claude_desktop_config.json
```

Reglas que no se negocian:

1. **Precedencia**, de menor a mayor: global < perfil < proyecto. Gana el más cercano, igual que las
   reglas de carpeta y que ADR 0001, donde el overlay es base y el repo gana.
   - Un perfil puede **apagar** algo heredado del global (`enabled: false`) sin borrarlo del global.
2. **Las capas declaradas conservan la forma oficial siempre que exista.**
   - El MCP de perfil vive en `overlay/mcp.json` con la misma forma que `.mcp.json`.
   - Las skills de perfil viven en `overlay/skills/<n>/SKILL.md`.
   - Lo que solo le importa a ccp (destinos, apagados) va en `ccp.yaml`, en un bloque aditivo.
   - Es la línea de ADR 0005: el usuario y otras herramientas pueden leer esos archivos sin ccp.
3. **ccp solo toca lo que ha escrito él.**
   - En un destino compartido (`cc-home/.claude.json`, `claude_desktop_config.json`), ccp escribe
     únicamente las entradas cuyo nombre registró como suyas.
   - Lo que el usuario añadió a mano se respeta y aparece en el inventario como «solo aquí», con la
     acción «subir a perfil» o «subir a global».
   - Es el mismo espíritu que el bloque `<!-- >>> ccp instructions >>> -->` y que `.authored`.
4. **Cada elemento tiene una clase**, y la misma clasificación la usan el inventario, la proyección,
   los snapshots y la sincronización:

| Clase | Ejemplos | Snapshot | Otra máquina |
|---|---|---|---|
| `authored` | `ccp.yaml`, `overlay/*`, `~/.claude/settings.json`, `CLAUDE.md`, skills, agents, commands, output-styles, scripts de hooks, `keybindings.json`, `enabledPlugins`, MCP | Sí | Se replica |
| `secret` | `api_key`, valores de `env` y `headers` de los MCP | Sí, **cifrado** y opcional | Se replica cifrado, si se elige |
| `derived` | `cc-home/settings.json` y `CLAUDE.md`, espejos, capa auto, lanzadores `.app` | No | Se regenera |
| `state` | transcripts `projects/`, `handoffs.yaml`, índice de sesiones de Desktop | Opcional (desactivado por defecto) | Se copia si se pide |
| `machine` | `machineID`, tokens OAuth (Keychain), Cookies y Local Storage de Desktop, rutas absolutas al binario | **Nunca** | Se recrea (login y reconstrucción) |
| `cache` | `plugins/cache`, `marketplaces/`, `rate-limits/`, cachés de `.claude.json` | Nunca | — |

## 3. Qué puede llegar a cada destino

Leyenda: ✓ ya llega · ◐ llega con este diseño · ✗ imposible desde local. Los «?» de la Fase 0 ya
están medidos: [ADR 0016](../../adr/0016-what-desktop-reads-from-a-profile.md) (Claude.app 2.2553.1; la
pestaña Code corre su propio Claude Code, 2.1.27x).

| Elemento | CLI | Desktop · Code | Desktop · chat | Notas |
|---|---|---|---|---|
| CLAUDE.md / reglas (global, perfil) | ✓ | ✓ (M1, `@import` fuera del root incluido) | ✗ | El chat usa instrucciones de la cuenta en claude.ai, en el servidor |
| CLAUDE.md / reglas (proyecto) | ✓ | ✓ (por cwd) | ✗ | El chat no tiene cwd |
| hooks, permisos, `env`, `statusLine`, `outputStyle` | ✓ | ✓ (M1: hook, `allow` y `env` medidos) | ✗ | Solo existen en Claude Code |
| MCP stdio (global, perfil) | ◐ (B1) | ✓ por los dos lados (M1, M2) | ◐ | Se proyecta a `.claude.json` **y** a `claude_desktop_config.json`. En Code, con nombre repetido gana Desktop (M2) |
| MCP http/sse (global, perfil) | ◐ | ◐ (M1) | ✗ en local (M3) | El archivo del chat solo admite stdio: el remoto pasa por el puente stdio `mcp-remote`, o se avisa de que en el chat solo va como conector de la cuenta |
| MCP (proyecto, `.mcp.json`) | ✓ | ✓ (por cwd) | ✗ | Sin cwd en el chat |
| skills, agents, commands (global) | ✓ | ✓ (M1) | ✗ | Las skills del chat son de la cuenta (servidor) |
| skills, agents, commands (perfil) | ◐ | ✓ (M1: `cc-home/{skills,agents}` tras el espejo) | ✗ | `overlay/{skills,agents,commands}` se une al espejo |
| plugins (`enabledPlugins`) | ✓ | ✓ (M1) | ✗ | Desktop añade en Code los plugins y conectores de la cuenta |

Conclusión honesta:
- La **pestaña Code** puede recibir todo, porque comparte el cc-home con el CLI.
- El **chat** de Desktop solo puede recibir **MCP**: es lo único que lee de disco.
- Las skills, las instrucciones y los conectores del chat son de la cuenta de claude.ai. La UI lo
  dice en cada elemento con un distintivo «Dónde aplica: CLI · Code · Chat» en lugar de prometerlo.

M2 lo confirmó en una instancia de perfil: **la pestaña Code hereda los MCP del chat**, servidos por el
proceso de Desktop desde un *shared pool*. Cuando el mismo nombre llega por los dos lados, se ve una vez y
gana Desktop, pero el `claude` de la pestaña también lanza su copia: dos procesos, uno ocioso.

## 4. Fase 0: mediciones y arreglos previos

Nada de lo demás se construye sobre suposiciones; es la lección de ADR 0009. Cada medición deja
escrito el método, la versión de Claude.app y de Claude Code, y el resultado. Las conclusiones de
Desktop van en un ADR, igual que 0008 y 0009.

| # | Pregunta | Método | Resultado ([ADR 0016](../../adr/0016-what-desktop-reads-from-a-profile.md)) |
|---|---|---|---|
| M1 | ¿La pestaña Code de una instancia de perfil respeta `cc-home/{settings.json, CLAUDE.md (con @import fuera del root), skills/, agents/, commands/, .claude.json:mcpServers, enabledPlugins}`? | Poner marcadores únicos en cada archivo, abrir una sesión Code y preguntar o ejecutar. Además, grep en `app.asar` de cómo lanza su `claude-code/<ver>/` | **Sí, todo.** Desktop lanza su propio Claude Code con `--setting-sources=user,project,local` y sin `--mcp-config` |
| M2 | ¿Code hereda los `mcpServers` de `claude_desktop_config.json`? ¿Qué ocurre si el mismo nombre llega también por `.claude.json`? | Un servidor con nombre repetido y otro con nombre único; mirar `/mcp` y `ps` | **Sí**, por un pool compartido de Desktop. Nombre repetido: se ve una vez y gana Desktop, pero también arranca la copia de `.claude.json` |
| M3 | ¿El `claude_desktop_config.json` del chat admite entradas remotas? ¿Se relee en caliente o hace falta reiniciar? | Entrada http de prueba; editar con la instancia abierta | **Solo stdio**: la http se descarta («Skipped invalid MCP server config entries»). **Hay que reiniciar** |
| M4 | ¿Con qué nombre guarda Claude Code las credenciales en el Keychain cuando `CLAUDE_CONFIG_DIR` no es el de siempre? ¿Depende de la ruta? | `security find-generic-password` antes y después de `/login` en un cc-home temporal | `Claude Code-credentials-<sha256(dir)[:8]>`, sin sufijo para `~/.claude`. **Depende de la ruta**. El login de Desktop no le sirvió a la CLI |
| M5 | ¿Desktop reescribe `claude_desktop_config.json` mientras corre (por `preferences`)? | fswatch con la instancia abierta y cambiando preferencias | **Sí**, y conservó los `mcpServers` escritos antes de arrancar. Sin medir: una escritura en caliente seguida de una reescritura suya |
| M6 | ¿Claude Code conserva un `mcpServers` escrito desde fuera en `.claude.json` mientras hay un `claude` vivo, o lo pisa con su copia en memoria? | Escribir con un `claude` interactivo abierto, esperar a que guarde estado y releer | **Lo conserva**: dos reescrituras suyas después, la entrada seguía ahí |

M6 decidía la técnica de proyección del MCP al CLI, y la decidió: **técnica A**, fusionar en
`.claude.json` (D7). El plan B era un plugin local por perfil (`ccp-<perfil>@ccp-local`, activado en el
`settings.json` que ya se genera), que habría renombrado los servidores a `mcp__plugin_…` y roto las
reglas de permisos escritas con el nombre corto. Queda descartado.

Arreglos: **B1–B5**, más **B6–B8**, que salieron de las mediciones (ADR 0016): `/config` dentro de un
perfil se pierde en el siguiente sync, `profile rename` deja el perfil sin login y `profile add` no
genera la config. B1 se resuelve de fondo con el subproyecto B: «global» pasa a significar «proyectado a
todos los perfiles». Tamaño: **S**.

## 5. Subproyecto A: inventario y adopción («detectar la máquina»)

### 5.1 Motor: `core/inventory.go` (puro, raíces inyectadas)

`Inventory(roots) → []Item`. Solo lectura, sin efectos, con raíces inyectadas para montar una
máquina falsa en un test, igual que `desktop_audit`. Hereda la regla del doctor: **una fuente que no
se puede leer produce `unknown`, jamás «vacío»**.

```go
type Item struct {
    Kind      string   // mcp | skill | agent | command | hook | permission | env | rule-instr
                       // | rule-path | plugin | output-style | statusline | settings-key | profile
                       // | desktop-instance | config-dir
    Scope     Scope    // {Level: global|profile|project|desktop|managed|plugin|account, Name}
    Name      string
    Source    string   // ruta real (o "claude.ai" para lo que vive en la cuenta)
    Class     string   // authored | secret | derived | state | machine | cache (§2)
    Managed   bool     // lo escribió ccp (authored/manifest)
    Editable  bool
    Why       string   // por qué no es editable: "lo trae el plugin X", "managed-settings", …
    AppliesTo []string // cli | desktop-code | desktop-chat
    Hash      string
    Secrets   []string // rutas JSON de los campos secretos (env.*, headers.*)
}
```

Fuentes que recorre, cada una con su sonda:

- **ccp**: `ccp.yaml` (perfiles, reglas, `auto_handoff`, `authored`) y `profiles/*/{overlay,cc-home,desktop}`.
- **Global de Claude Code**:
  - `~/.claude/`: `settings.json` clave a clave, `CLAUDE.md`, `agents/`, `commands/`, `skills/`,
    `output-styles/`, `hooks/`, `keybindings.json`, `plugins/installed_plugins.json` y
    `known_marketplaces.json`.
  - De `~/.claude.json` **solo las claves de configuración**: `mcpServers`, más de cada proyecto su
    `mcpServers`, `enabledMcpjsonServers` y `disabledMcpjsonServers`, y `allowedTools`. El resto son
    ~60 claves volátiles: `machineID`, cachés y contadores.
- **Cada cc-home** de perfil, con el mismo recorrido.
- **Desktop**: cada data dir (`default` y los perfiles), con `claude_desktop_config.json` (`mcpServers`,
  `preferences`) y, si existen, las extensiones instaladas (M3 dirá dónde).
- **Managed**: `/Library/Application Support/ClaudeCode/managed-settings.json` y `managed-mcp.json`,
  solo lectura.
- **Plugins**: los MCP, skills y commands que aporta cada plugin activo, solo lectura («lo trae el plugin X»).
- **Proyectos conocidos**, sacados de las reglas de carpeta, de las claves `projects` de cada
  `.claude.json` y de `githubRepoPaths`. De cada uno: `.claude/`, `.mcp.json`,
  `.claude/settings.local.json` y `CLAUDE.local.md`. Para cada uno se guarda también su **identidad
  portable**: la URL normalizada del remoto de git (§11).
- **Directorios de config sin gestionar**: otros `~/.claude*` con `projects/` y `.claude.json`, y los
  `export CLAUDE_CONFIG_DIR=` que haya en el rc. Son candidatos a adoptar.

### 5.2 Adopción: un plan, luego aplicarlo, en orden

`AdoptPlan(inv) → []Step` es puro y se muestra entero. `AdoptApply(steps)` hace primero un snapshot
automático (§8) y luego aplica en este orden, porque cada paso depende del anterior:

1. **Migración de estado**: `ensureMigrated` (dsctl→TSV→yaml), que ya existe.
2. **Perfiles**:
   - Los que ya tiene ccp se cargan tal cual, en el orden de `ccp.yaml`.
   - Los directorios de config sin gestionar se ofrecen como perfil nuevo. Por defecto **se copia** la
     config (no los tokens) al cc-home de ccp, se deja intacto el original y se hace **un `/login`**
     por perfil adoptado.
   - La adopción **por referencia** (un `cc_home:` que apunte al directorio original y conserve el
     login) depende de M4, y cambia la salida de `_env`: ver §15.
3. **Reglas de carpeta**: cada ruta se valida contra el disco; las huérfanas se marcan.
4. **Capas**:
   - Se detectan los duplicados: un mismo MCP en tres ventanas de Desktop y en ningún CLI, que es
     justo el caso de esta máquina.
   - Se propone **subirlos**: si está en todas las instancias, a global; si solo en una, a su perfil.
   - Es opcional por elemento, con vista previa.
5. **Proyección** (subproyecto B) a cada destino.
6. **Desktop**: lanzadores que faltan o están desfasados; se ofrecen, no se construyen solos.
7. **Pendientes**, que no se pueden automatizar: logins, `api_key`s y comandos de MCP que no existen
   en esta máquina (`npx`, `docker`, `/opt/homebrew/bin/node`, …).

### 5.3 Superficie

- CLI: `ccp scan [--json]` y `ccp adopt [--dry-run] [--only <tipo>]`.
- serve: `inventory.scan`, `adopt.plan` y `adopt.apply`.
- GUI: la ruta `bienvenida` pasa a ser la pantalla **P-19 · Detectar esta máquina**. Se abre en el
  primer arranque, cuando `ccp.yaml` es mínimo o hay candidatos sin adoptar, y a mano desde Ajustes.
  Sus tres bloques:
  1. Lo encontrado, agrupado por perfil → tipo → elemento, con su procedencia.
  2. El plan, con casillas por elemento.
  3. El resultado, con la lista de pendientes.

Tamaño: **M**.

## 6. Subproyecto B: capas unificadas y proyección (incluido Desktop)

### 6.1 MCP

- **Global**: los `mcpServers` de `~/.claude.json`, que es el scope user oficial de `default`.
- **Perfil**: `profiles/<n>/overlay/mcp.json`, con la forma `{"mcpServers": {…}}`.
- **Proyecto**: `<repo>/.mcp.json`. Claude Code lo lee solo; ccp lo lista y lo edita, pero no lo proyecta.
- **Metadatos de ccp**: un bloque aditivo en `ccp.yaml`. `version` sigue en `2` y viaja por
  `Config.Extra`, igual que `auto_handoff`:

```yaml
mcp:
  targets:            # por servidor; por defecto [cli, desktop]
    obsidian-vault: [cli, desktop]
    jira: [cli]
  disabled:           # un perfil apaga lo heredado sin borrarlo del global
    work: [finance-os]
  desktop_default: false   # ¿proyectar también a la ventana `default`? (ver abajo)
```

**Proyección** (`CfgRegenerate` la hace suya; `ccp profile sync` la ejecuta):

- **Efectivo por perfil** = global ⊕ overlay − disabled. Si hay colisión de nombres, gana el perfil.
- **Al CLI**:
  - **Técnica A** (D7, decidida por M6): fusionar en `cc-home/.claude.json:mcpServers` solo los
    nombres gestionados. Se lee, se modifica y se escribe de forma atómica, bajo flock, conservando
    todas las demás claves y sin cachear nada: Claude Code reescribe ese archivo a menudo, pero conserva
    lo que se escribe desde fuera.
  - La lista de nombres gestionados se guarda en `cc-home/.ccp-managed.json`. Es `derived`: si se
    pierde, se reconstruye comparando con la capa declarada.
- **Al chat de Desktop**:
  - Se escriben solo los nombres gestionados en `profiles/<n>/desktop/claude_desktop_config.json`,
    conservando `preferences`, `coworkUserFilesPath` y cualquier clave desconocida.
  - No se relee en caliente (M3). Si la instancia está corriendo, el perfil queda marcado como
    **«pendiente de reiniciar la ventana»** y la GUI ofrece reiniciarla. Nunca se mata una ventana por
    sorpresa. Como M5 no descarta que Desktop vuelque su copia en memoria sobre una escritura en
    caliente, se escribe con la ventana cerrada o se relee y se reproyecta tras el reinicio.
  - Solo stdio (M3). Un servidor http/sse con destino `desktop` va envuelto en el puente `mcp-remote`,
    o la UI dice que en el chat solo puede ir como conector de la cuenta.
- **Ventana `default`** (`~/Library/Application Support/Claude/`):
  - Es el Claude del usuario, igual que en la barrera del updater: ccp no escribe en ella salvo que
    el usuario active `desktop_default: true`.
  - Aun así la inventaría siempre. Ahí están hoy los 6 MCP.
- **Duplicados en la pestaña Code**: M2 confirmó que Code hereda los MCP del chat y que un nombre
  repetido arranca dos procesos, aunque solo se usa el de Desktop. Aun así, un servidor con destino
  `[cli, desktop]` va a **los dos** archivos. El cc-home es el mismo para la terminal y para la pestaña
  Code, así que quitarlo de `.claude.json` dejaría sin él a la CLI. Se acepta el proceso ocioso en la
  pestaña: es la misma definición y gana la de Desktop (ADR 0016).
- Los **secretos** (`env.*`, `headers.*`) se proyectan en claro porque así los leen las apps, igual
  que hoy. Solo cambia su clase en los snapshots (§8).

### 6.2 Skills, agents, commands y output-styles por perfil

- `overlay/{skills,agents,commands,output-styles}/` pasan a existir; los códigos 3 y 5 de `InstructDest`
  desaparecen.
- En cuanto un perfil tiene algo propio en uno de esos directorios, el suyo en `cc-home` pasa a ser
  el espejo que ya existe (`MirrorForDesktop`: directorios reales con symlinks solo en las hojas),
  ampliado a **global ∪ overlay**. Si hay colisión, gana el overlay.
- Es la forma que Desktop ya exige («symlink at a non-leaf component»), así que el CLI y la pestaña
  Code reciben lo mismo.
- `seedCCHome` no cambia: el oráculo exige `[[ -L "$cch/plugins" ]]` justo después de
  `profile add`, y eso sigue siendo cierto.
- B4: `output-styles/`, `hooks/` y `keybindings.json` entran al conjunto que se siembra y se espeja.

### 6.3 Hooks y permisos

- **Hooks**: se editan como el array completo de cada evento, en global o en overlay. Eso resuelve
  «se añaden pero no se borran». En la UI, cada entrada se identifica por su posición y su
  `matcher`+`command`, y el guardado reescribe el array entero.
- **Permisos**: se editan `allow`, `deny`, `ask` y `defaultMode` por capa.
- **Arrays**: la fusión actual *reemplaza* los arrays, igual que `jq *`. La vista efectiva tiene que
  enseñarlo: «el perfil reemplaza la lista global entera».
  - Se propone añadir `permissions.*` con **unión**, detrás de una clave explícita del overlay
    (`"$merge": "union"`), para no cambiar ADR 0002 en silencio.

### 6.4 Deriva y diagnóstico

- `ccp profile sync --check` (y `profiles.drift` en serve) compara lo declarado con lo proyectado
  sin escribir nada.
- Hallazgos nuevos del doctor:
  - `mcp_unmanaged_only_desktop`
  - `mcp_command_missing`
  - `projection_stale`
  - `desktop_restart_pending`
  - `cc_home_symlink_nonleaf`, que es lo que rompe la pestaña Code.
- `ProfileEffective` gana las secciones MCP, permisos completos, `statusLine`, `outputStyle` y `model`.
  Cada fila lleva su `AppliesTo`.

Tamaño: **L**.

## 7. Subproyecto C: editor en la GUI

Las pantallas siguen la convención de siempre: cada una muestra su equivalente de CLI, y todo lo que
hace pasa por `core`.

- **P-19 · Detectar esta máquina** (§5).
- **P-20 · Configuración**. Sustituye a la vista de solo-gestionado de P-15 Memoria y absorbe P-05.
  - Arriba, un selector de capa: Global · Perfil ▾ · Proyecto ▾ · Desktop ▾.
  - A la izquierda, los tipos: Instrucciones · MCP · Skills · Agents · Commands · Hooks · Permisos ·
    Env · Plugins · Estilos · Barra de estado.
  - En el centro, los elementos con su **procedencia** y sus distintivos **Dónde aplica** (CLI ·
    Code · Chat).
  - Los elementos no editables dicen por qué: «lo trae el plugin figma», «managed-settings», «vive
    en tu cuenta de claude.ai».
  - Un conmutador **Efectivo** muestra el resultado fusionado de un perfil concreto, con `Shadowed`.
- **Editores**:
  - MCP: stdio/http/sse/JSON (`lib/mcp.ts` ya existe), con los campos secretos enmascarados, el
    destino por servidor y «apagar en este perfil».
  - Skill: frontmatter + cuerpo de `SKILL.md`, y los archivos anexos.
  - Agent y command: frontmatter + cuerpo.
  - Hooks: por evento, con matcher, command y timeout.
  - Permisos: tres listas y `defaultMode`.
  - CLAUDE.md: bloque gestionado y texto libre, con vista previa de `@import`.
- **Acciones de capa**: «subir a global», «bajar a perfil», «copiar a proyecto» y «apagar aquí».
- **P-17 Copias** evoluciona a **Snapshots** (§8). Aparece **P-21 · Nube** (§10).
- Tras cada escritura, la GUI llama a la proyección y enseña el aviso «pendiente de reiniciar
  ventana» cuando toca.

Métodos de serve (un único registro, sin subir el protocolo porque son altas):
`config.items`, `config.item.get|put|delete`, `config.effective`, `mcp.list|put|delete|setTargets|disable`
y `profiles.drift`.

CLI equivalente:
- `ccp mcp list|add|rm|enable|disable|targets [--scope global|profile <n>|project]`.
- `ccp instruct` sigue existiendo y usa los mismos destinos.

Tamaño: **L**.

## 8. Subproyecto D: snapshots locales

> **Implementado (plan 2026-09-18-snapshots-locales).** La GUI (P-17 → Snapshots) queda para el plan de la GUI.

### 8.1 Qué captura: todo lo `authored`, y lo demás a elección

- Todo lo de clase `authored` del inventario:
  - ccp: `ccp.yaml` y `overlay/*`.
  - global: `~/.claude/…` y la parte de configuración de `.claude.json`.
  - cada cc-home: su parte de configuración.
  - cada instancia de Desktop: `claude_desktop_config.json`.
  - proyectos: **solo lo que no está en git**, es decir `settings.local.json` y `CLAUDE.local.md`.
    Lo versionado ya tiene su historia; el snapshot guarda la identidad del repo y el commit en el
    que se vio.
- `secret`: opcional y siempre cifrado.
- `state` (conversaciones, préstamos): opcional, desactivado por defecto por tamaño y privacidad.
  Con deduplicación, cada snapshot incremental solo añade lo nuevo.
- Nunca `machine` ni `cache`.

### 8.2 Formato: almacén direccionado por contenido y rutas lógicas

```
~/.config/ccp/snapshots/
  objects/ab/cdef…          blobs comprimidos, id = sha256 del contenido en claro
                            (con clave HMAC en la nube, §10)
  snaps/<id>.json           manifiesto: {id, parent, created, machine, ccp_version, label, trigger,
                            items: [{lpath, hash, mode, class, meta}]}
  keys/                     clave local de cifrado de secretos (envuelta por el Keychain de macOS)
```

- Las **rutas lógicas** (`lpath`) no dependen de la máquina: `{ccp}/ccp.yaml`,
  `{claude}/skills/foo/SKILL.md`, `{profile:work}/overlay/mcp.json`, `{desktop:work}/config`,
  `{repo:github.com/org/app}/.claude/settings.local.json`.
- Dentro del contenido, las rutas absolutas del home se normalizan a `~` al capturar. Aplican a
  reglas de carpeta y a comandos de hooks.
- Deduplicación entre snapshots, snapshots baratos y diffs triviales (comparar dos listas de
  `lpath→hash`).
- Es lo mismo que en §10 se sube a la nube: **un solo formato para local y remoto**.
- Compatibilidad:
  - `ccp backup export` sigue existiendo: saca un snapshot a un `.tar.gz` autocontenido (manifiesto +
    blobs), válido para mover a mano.
  - `ccp backup restore` acepta el formato viejo y el nuevo.

### 8.3 Comandos, disparadores y retención

- `ccp snapshot create [-m <label>] [--with-secrets] [--with-state]`
- `ccp snapshot list [--json]`
- `ccp snapshot show <id>`
- `ccp snapshot diff <a> [<b>|--live]`
- `ccp snapshot restore <id> [--only <lpath|tipo>…] [--dry-run]`
- `ccp snapshot prune`
- `ccp snapshot export <id> <archivo>` / `import <archivo>`

**Automáticos**, antes de todo lo que destruye o sustituye:
- restore
- `profile rm`
- `adopt apply`
- aplicar un cambio llegado de la nube
- `desktop rm`

Y uno **diario** si hubo cambios (un LaunchAgent opcional, o el primer `ccp` del día).

**Retención** al estilo restic: 7 diarios, 4 semanales y 6 mensuales, más los que tienen etiqueta,
que nunca se podan.

**Restore** = plan (diff contra el estado vivo) → selección por elemento → snapshot automático →
aplicar → **regenerar la proyección**. Esto último es el arreglo de B2.

### 8.4 Cifrado local

- Los blobs `secret` se cifran con `age` (X25519, `filippo.io/age`).
- La identidad se guarda en el Keychain de macOS, o en un archivo `0600` en Linux, y se puede
  exportar con frase de recuperación.
- El resto del almacén local va en claro, `0700`: es tu propia máquina.

Tamaño: **M**.

## 9. Subproyecto E: sincronización sin servidor

> **Aplazado (2026-09-18).** Se decidió ir directo al backend (D10), así que F1 sustituye a este
> subproyecto. La abstracción de «remoto» se conserva en el cliente: una carpeta o un bucket puede
> añadirse más tarde como segundo remoto sin tocar el formato.

Antes del backend conviene un paso que da el 80 % de «replicar en varias máquinas» sin operar ningún
servicio:

- **Remoto = carpeta o bucket.** `ccp sync remote add <nombre> file:///…/iCloud Drive/ccp` o
  `s3://…`. Sirven iCloud Drive, Dropbox, Syncthing, un NAS o R2.
- En el remoto va el almacén de §8.2 **cifrado entero** con una clave de cuenta (§10.2): los blobs
  con ids HMAC, los manifiestos cifrados y un `remote.json` con los parámetros KDF.
- `ccp sync push`, `ccp sync pull` y `ccp sync apply <snap> [--plan]`.
- Valida en condiciones reales el formato, el cifrado, el mapeo de rutas (§11) y la reconciliación
  **antes** de construir cuentas y portal. Si el backend no llega nunca, esto ya resuelve la necesidad
  principal.

Tamaño: **M**.

## 10. Subproyecto F: backend, cuentas y portal

### 10.1 Principio: el servidor no lee tu configuración (cifrado de extremo a extremo)

Los snapshots llevan claves de API, tokens de MCP, y hooks y comandos que **se ejecutan** en tus
máquinas. Un servidor que pueda leerlos y escribirlos es, si lo comprometen, **ejecución remota de
código en todas tus máquinas**. Por eso:

- El servidor guarda **solo texto cifrado**: blobs con id HMAC, para que deduplique sin saber qué
  contienen, y manifiestos cifrados. Además, los metadatos mínimos: dispositivos, fechas y tamaños.
- El **portal descifra en el navegador**, como hacen Bitwarden y 1Password. La lógica de fusión y
  validación de `core` puede compilarse a **WASM** (`GOOS=js GOARCH=wasm`) para no reescribirla en TS.
- **Precio**: si se olvidan la frase de bóveda y el código de recuperación, los datos no se
  recuperan. La UI lo dice al crear la bóveda.

### 10.2 Identidad con Keycloak, cifrado con la bóveda (D9)

Hay dos preguntas distintas, y cada una tiene su dueño:
- «¿Quién eres?» → **Keycloak**.
- «¿Puedes leer esto?» → **la clave de la bóveda**, que solo existe en tus dispositivos.

Separarlas es obligatorio con Keycloak. La contraseña se escribe en la página de Keycloak, así que
`ccp` y el portal nunca la ven y no pueden derivar de ella la clave de cifrado. Además, un reset de
contraseña en Keycloak destruiría todos los datos.

**Identidad: Keycloak propio del stack `ccp-cloud`, en `https://ccp-auth.joseiz.com`, realm `ccp`**
(§10.6). No comparte instancia con el Keycloak de `Personal`: la nube de ccp se despliega, respalda
y restaura como una unidad.
- El realm es código: `deploy/ccp-cloud/files/realm-ccp.json`, que Keycloak importa al arrancar con
  `--import-realm` si el realm no existe.

- **Clientes**:
  - `ccp-cli`: público, con la concesión de dispositivo (OAuth 2.0 Device Authorization Grant,
    RFC 8628, ya expuesta por tu Keycloak) y PKCE. `ccp cloud login` enseña un código y abre el
    navegador.
  - `ccp-portal`: público, código de autorización + PKCE, redirección a `https://ccp.joseiz.com/*`.
  - `ccp-api`: el *audience*. Un mapper añade `aud: ccp-api` a los tokens de los otros dos.
- **Tokens**: acceso de 5 min. Cada dispositivo pide `offline_access`: su sesión offline es su
  credencial de larga vida, visible y revocable en Keycloak.
- **Usuarios y credenciales**: se gestionan en la consola de Keycloak, que cubre lo de «mantenimiento
  de usuarios».
  - Registro público **apagado**: alta por invitación, con acciones requeridas «verificar email» y
    «configurar OTP».
  - Passkeys (WebAuthn) opcionales y política de contraseñas.
  - **Detección de fuerza bruta** encendida y SMTP para verificar y restablecer.
  - Login con GitHub o Google opcional, como *identity brokering*.
- **Roles de realm**: `ccp-user` (usar la nube) y `ccp-admin` (ver auditoría global, invitar).
- **El backend** valida cada JWT contra el JWKS del realm: `iss`, `aud`, `exp` y firma, con caché de
  claves y rotación.
  - El `sub` es la identidad: la fila de `users` se crea en el primer acceso.
  - Además, en **cada petición** comprueba que el dispositivo no esté revocado en su propia tabla.
    Es defensa en profundidad: revocar en ccp surte efecto aunque el token siga vivo 5 minutos.

**Cifrado: la bóveda.**

- **Clave de cuenta (AK)**: 256 bits aleatorios, creada en el primer dispositivo. **Nunca** sale en
  claro de un cliente. De ella salen, por HKDF:
  - la clave de datos (AEAD de blobs y manifiestos);
  - la clave HMAC de los ids de blob;
  - la clave de firma Ed25519 de manifiestos y revisiones.
- La AK se guarda **envuelta** tres veces, y el servidor guarda esas tres envolturas sin poder
  abrirlas:
  1. **por dispositivo**, con su par X25519, cuya parte privada vive en el Keychain;
  2. con la **frase de bóveda** (Argon2id → KEK). Es distinta de la contraseña de Keycloak y es la
     que se escribe en el portal o para dar de alta un dispositivo sin tener otro a mano;
  3. con el **código de recuperación**, que se muestra una vez al crear la bóveda para imprimirlo o
     guardarlo en 1Password.
- **Alta de un dispositivo nuevo**: `ccp cloud login` (Keycloak) → el backend sabe quién eres, pero
  el dispositivo aún no puede leer nada. Se desbloquea de una de dos formas:
  - escribiendo la frase de bóveda;
  - **aprobándolo desde otro dispositivo**: ese dispositivo envuelve la AK para la clave pública del
    nuevo, previa comparación de un código corto en ambas pantallas.
- **Portal**: login con Keycloak → pide la frase de bóveda → desenvuelve la AK **en el navegador**
  (WebCrypto + argon2 en WASM).
  - La AK vive solo en memoria de la pestaña y se olvida al cerrarla o tras 15 min de inactividad.
  - El portal lleva una CSP estricta y ningún script de terceros, porque maneja la clave.
- **Revocar un dispositivo**: se cierra su sesión offline en Keycloak y se marca en ccp. Si se teme
  que la clave se filtró, **rotar la AK** es una acción explícita: nueva AK, re-cifrado de manifiestos
  y re-envoltura para los dispositivos que quedan; los blobs se re-cifran perezosamente.
- **Usuarios**: el esquema es multiusuario desde el día uno (D3). Cada usuario tiene su propia bóveda;
  no hay datos compartidos entre usuarios.

### 10.3 Control desde el portal: «el portal propone, la máquina aplica»

- **Sin puertos entrantes.** Cada máquina tira: `ccp cloud agent` como LaunchAgent, con long-poll o
  consulta cada pocos minutos. La GUI también lo hace mientras está abierta.
- **Estado deseado.** Desde el portal se fija, para un dispositivo o un **grupo** («todas mis Macs»),
  una **revisión deseada**: un snapshot, o un conjunto de cambios sobre el último aplicado. La
  revisión va **firmada** con la clave de cuenta, que el servidor no tiene, así que no puede
  falsificar órdenes.
- **Reconciliación** con merge a tres bandas por `lpath`: base = última revisión aplicada, la local
  y la deseada.
  - Lo que no choca se aplica.
  - Lo que choca aparece en la GUI como conflicto a resolver.
  - Antes de aplicar, snapshot automático.
- **Política por dispositivo**:
  - `auto`: aplica sin preguntar lo **no ejecutable** (reglas de instrucciones, env, permisos que
    restringen).
  - **confirmar siempre** lo ejecutable: hooks, `command`/`args` de MCP, `statusLine`, plugins,
    skills con scripts, y permisos que **amplían** (`allow`, `defaultMode`).
  - La confirmación se hace en la GUI o con `ccp cloud review`. Una cuenta robada no basta para
    ejecutar código en tus máquinas.
- **Portal**:
  - Dispositivos: último contacto, versión de ccp, perfiles y **deriva** respecto al deseado.
  - Línea de tiempo de snapshots de cada dispositivo, con diff entre dos cualesquiera.
  - Editor de configuración, el mismo modelo que P-20.
  - «Aplicar a…», grupos, auditoría y revocación.
- **El portal no hace login en Anthropic.** Los tokens OAuth son `machine`: tras replicar, el
  dispositivo muestra «login pendiente» por perfil y abre Terminal con `ccp profile login <n>`, igual
  que hoy.

### 10.3.1 Historial, descarga y restauración

**Historial.**
- Cada dispositivo sube un snapshot cuando cambia algo y con los automáticos de §8.3. El servidor
  guarda **todos** por defecto: con deduplicación, un snapshot de configuración pesa kilobytes.
- La retención es configurable, pero los **fijados** (etiqueta o «conservar») nunca se podan.
- Cada manifiesto lleva el hash de su padre, firmado: el historial es una **cadena**, y ni el servidor
  ni nadie puede reescribirlo, reordenarlo o quitarle un eslabón sin que el cliente lo detecte.

**Descargar**, desde el portal (botón «Descargar») o desde el CLI:
- **Cifrado** (`.ccpsnap`), por defecto: manifiesto + blobs, tal como están en el servidor. Se
  importa con `ccp snapshot import <archivo>` y la frase de bóveda. Es seguro guardarlo en cualquier
  sitio.
- **Descifrado** (`.tar.gz` legible): el navegador lo arma en memoria. Si el snapshot trae secretos,
  hay que confirmarlo explícitamente, con el aviso «contiene tus claves en claro».
- CLI: `ccp cloud pull <snap> [-o archivo] [--decrypted]`.

**Restaurar**, por tres caminos que terminan en el mismo motor: el restore de §8.3, que planifica,
hace un snapshot previo, aplica selectivamente y regenera la proyección.

1. **Desde la app de tu máquina** (GUI, pantalla Nube → Historial): ves los snapshots de **todas** tus
   máquinas, eliges uno, ves el diff contra tu estado vivo, marcas «todo» o elementos sueltos y
   restauras. Es inmediato, porque la app corre en la máquina.
2. **Desde el portal web**: eliges máquina → snapshot → diff → «Restaurar en <máquina>».
   - Se publica una **revisión deseada firmada**; la máquina la aplica en cuanto su agente contacta.
     Si está apagada, al encenderse.
   - Lo ejecutable pide confirmación local (D6).
   - El portal muestra el estado: `pendiente` → `aplicada` / `parcial` (con qué quedó sin aplicar y
     por qué) / `conflicto` / `fallida`.
3. **En una máquina nueva**: `ccp cloud login` → desbloquear la bóveda → elegir un snapshot de otra
   máquina → el asistente mapea rutas y repos (§11) **en la máquina**, porque necesita su disco →
   aplicar → lista de pendientes (logins, comandos que faltan).

El portal **nunca** restaura por sí mismo: no hay conexión entrante a tus máquinas. Siempre propone,
y la máquina ejecuta (ADR 0014).

### 10.4 Pila recomendada

- **Backend en Go** en el mismo repo: `cmd/ccp-cloud`, `internal/cloud/{server,store,auth}`.
  Comparte con el cliente el paquete `internal/snapshot` (formato, manifiesto y cifrado): un solo
  código para ambos extremos.
- **Keycloak** (propio del stack, realm `ccp`) para identidad, usuarios, MFA y sesiones de
  dispositivo (§10.2). El backend **no guarda contraseñas**.
- **Postgres** propio del backend, no el de Keycloak. Tablas:
  - `users`, con el `sub` de Keycloak;
  - `vaults` y `vault_wraps` (las envolturas de la AK: por dispositivo, frase y recuperación);
  - `devices`;
  - `snapshots` (manifiesto cifrado, padre y firma);
  - `blobs` (id HMAC, tamaño, referencias);
  - `revisions` (estado deseado firmado);
  - `applies` (resultado de cada aplicación en cada dispositivo);
  - `groups` y `audit_log` (solo inserción).
  - Las migraciones son versionadas (`goose`), hacia delante, y se prueban en CI contra una base vacía
    y contra el volcado anterior.
- **Almacenamiento: [Alarik](https://alarik.io)** para los blobs, desplegado en el mismo servidor.
  - Es S3-compatible, está escrito en Swift y tiene licencia Apache 2.0. Se despliega con Docker
    Compose y admite desde un nodo hasta un clúster con erasure coding.
  - Las subidas y bajadas van con **URLs prefirmadas** (SigV4, hasta 7 días): los blobs no pasan por
    el proceso Go.
  - Otras dos piezas de Alarik que se aprovechan:
    - el **lifecycle**, para limpiar multiparts abandonados;
    - el **versionado**, como red contra un borrado accidental del recolector de blobs.
  - El backend habla **solo S3 genérico** (`aws-sdk-go-v2/service/s3`, endpoint propio, path-style) y
    usa este subconjunto: `PutObject`, `GetObject` y `HeadObject` prefirmados, multipart,
    `DeleteObject` y `ListObjectsV2`.
  - F1 incluye una **prueba de contrato del almacenamiento** que levanta Alarik con Docker Compose y
    ejercita exactamente ese subconjunto. Cambiar a Garage, SeaweedFS o R2 sería cambiar el endpoint.
  - **CORS**: el README de Alarik no lo lista. Solo afecta al portal, porque el navegador baja blobs
    de otro origen; `ccp` no pasa por CORS.
    - Si la prueba de contrato confirma que no existe `PutBucketCors`, las cabeceras se ponen con un
      middleware `headers` de Traefik en el dominio de Alarik, que Dokploy ya usa.
    - Si no, el portal baja los blobs a través del backend.
- **Portal**: React + Vite como `gui/`, reutilizando sus componentes con un tercer transporte en
  `lib/bridge.ts`. Hoy tiene `invoke` (Tauri) y `fetch('/__ccp')` (Vite); el nuevo sería «nube»:
  trabaja sobre un snapshot descifrado en memoria y publica revisiones. El servidor Go lo sirve como
  estático: un solo despliegue.
- **Despliegue en Dokploy**: proyecto `ccp-cloud` con **un solo compose** que contiene toda la nube.
  Hoy lleva Postgres, Keycloak y Alarik; la app Go se añade en F1. Fuente: `deploy/ccp-cloud/`.
  - El Alarik de `Lab` se queda como laboratorio (§10.6).
  - Postgres es una instancia con dos bases, `keycloak` y `ccp`, cada una con su usuario.
  - El compose va con aislamiento de red (Isolated Deployment).
  - Dominios **de un solo nivel**:
    - `ccp.joseiz.com`: portal + API, mismo origen (F1);
    - `ccp-auth.joseiz.com`: Keycloak;
    - `ccp-s3.joseiz.com`: Alarik (`s3.joseiz.com` ya estaba en uso).
    - Tus dominios pasan por Cloudflare, y su certificado universal solo cubre `*.joseiz.com`. Nombres
      como `alarik.api.joseiz.com` fallan en el handshake TLS, y eso es exactamente lo que le pasa
      hoy al Alarik de `Lab`.
  - Límites de Cloudflare que el diseño respeta:
    - cuerpo de petición ≤ 100 MB: las partes de multipart son de ≤ 64 MiB;
    - 100 s por petición: el long-poll del agente dura ≤ 60 s.
  - Los backups del Postgres se programan desde Dokploy **a un destino que no sea Alarik**. El bucket
    se replica a un segundo destino con rclone, porque Alarik está en beta (§16).
- **API** (JSON sobre HTTPS, versionada `/v1`, todo con `Authorization: Bearer <JWT de Keycloak>`):
  - `me` (alta perezosa del usuario)
  - `vault` (crear, leer envolturas, rotar)
  - `devices` (alta con clave pública, aprobar desde otro dispositivo, revocar)
  - `blobs/{id}` (HEAD / URL prefirmada de subida y bajada)
  - `snapshots` (commit, listar, leer manifiesto cifrado, fijar)
  - `devices/{id}/desired` y `devices/{id}/applies`
  - `groups`
  - `audit`
  - `events` (SSE para el portal; long-poll para el agente)

Tamaño: **XL**. Se parte en:
- **F1**: realm + vault + dispositivos + push/pull de snapshots desde el CLI y la GUI.
- **F2**: portal de solo lectura (dispositivos, historial, diff, **descarga**).
- **F3**: **restaurar desde el portal** (revisiones firmadas, agente, confirmación local, estado de
  aplicación).
- **F4**: grupos, auditoría en el portal y rotación de AK.

### 10.5 Robustez

Qué significa «robusto» aquí, en requisitos que se pueden probar:

1. **Integridad de punta a punta**.
   - Id de blob = HMAC(AK, sha256 del claro). El cliente verifica el hash al descifrar.
   - Manifiestos y revisiones firmados con Ed25519, encadenados por el hash del padre.
   - Un servidor comprometido puede **negar** servicio, pero no alterar, colar ni reordenar
     configuración sin que el cliente lo rechace.
2. **Commit atómico de snapshots**.
   - Primero los blobs: `PUT` idempotente, porque el id es el contenido y reintentar es seguro;
     `HEAD` salta los que ya existen.
   - Después, `POST /snapshots` con el manifiesto: en una transacción de Postgres, el servidor
     comprueba que existen todos los blobs referenciados.
   - Un snapshot es visible **solo** si está completo; no existen snapshots a medias.
3. **Subidas reanudables y cola offline**.
   - Multipart por partes, con reintentos y retroceso exponencial.
   - Sin red, el snapshot local se hace igual y el agente lo sube al volver.
   - La nube nunca bloquea a `ccp`: todo lo local funciona con el backend caído.
4. **Recolección de basura segura**.
   - Referencias contadas en Postgres.
   - Un blob sin referencias se borra solo tras un **periodo de gracia** de 7 días y nunca durante
     un commit en curso, que tiene un *lease*.
   - El versionado de Alarik es la segunda red.
5. **Los datos del servidor también tienen backup**.
   - Postgres a diario (Dokploy) a un destino que no es Alarik.
   - Réplica del bucket con rclone.
   - Un **simulacro de restauración** automático semanal: restaura un snapshot fijado en un
     `CCP_HOME` desechable y verifica hashes. Un backup que nunca se ha restaurado no cuenta.
6. **Observabilidad**, con lo que ya tienes desplegado:
   - `/healthz` (vivo) y `/readyz` (Postgres, Alarik y JWKS de Keycloak alcanzables).
   - Métricas Prometheus para tu Prometheus + Grafana de `Lab`.
   - Errores a tu GlitchTip y trazas OpenTelemetry a tu SigNoz.
   - Logs estructurados con id de petición.
   - **Ningún secreto en logs**: el servidor no los tiene, y los ids se truncan.
7. **Seguridad operativa**.
   - Límites de tasa por usuario y dispositivo, y límites de tamaño.
   - Validación estricta de JWT, revocación de dispositivo comprobada en cada petición.
   - Auditoría de solo inserción.
   - Cabeceras de seguridad y CSP estricta en el portal.
   - Dependencias fijadas y escaneadas; tienes SonarQube en `Lab`.
8. **Degradación clara**.
   - Si Keycloak está caído, los dispositivos no renuevan token: la sincronización se pausa y avisa,
     y `ccp` sigue funcionando.
   - Si Alarik está caído, `/readyz` falla y los commits se reintentan desde la cola.
9. **Pruebas**.
   - e2e en CI con Docker Compose: Keycloak con el realm importado, Postgres y Alarik.
   - Prueba de contrato S3 contra Alarik.
   - Inyección de fallos: matar el proceso a mitad de commit o de GC y comprobar que no queda nada
     inconsistente.
   - Tests de propiedad del merge a tres bandas.

### 10.6 Infraestructura existente (inspeccionada el 2026-09-18, solo lectura)

Dokploy v0.30.4, un solo servidor.
- **Keycloak** `Personal/keycloak` en `auth.joseiz.com`: **no se usa**. Se decidió (2026-09-18) que la
  nube de ccp lleve el suyo dentro de su propio stack (§10.4). `IBC/keycloak`, parado, tampoco.
- **Alarik** `Lab/alarik` tiene **dos problemas independientes**:
  1. El último despliegue (2026-09-17) falló antes de arrancar: `HashiCorp Vault: failed to read
     secret at "alarik" (status 403)`. La integración de Vault de Dokploy no tiene permiso sobre ese
     secreto; hay que revisar la política o el token de Vault.
  2. `alarik.api.joseiz.com` y `alarik.console.joseiz.com` no responden: el handshake TLS falla en
     Cloudflare por ser subdominios de segundo nivel (ver §10.4).
  - Se deja como laboratorio; la nube usa uno limpio en `ccp-cloud` con `ccp-s3.joseiz.com`.
- **MinIO** `Personal/minio` corriendo: queda como destino posible del backup de Postgres o de la
  réplica del bucket, no como almacenamiento principal (D2).

## 11. Portabilidad entre máquinas

- **Rutas**:
  - Las reglas de carpeta y los proyectos se identifican por el **remoto de git normalizado**, más
    la ruta relativa dentro del repo.
  - Al importar, el asistente propone para cada repo: encontrado (busca en las raíces habituales y en
    los `projects` de `.claude.json`), clonarlo, o elegir la carpeta a mano.
  - Las carpetas sin git se mapean a mano o se descartan.
- **Home distinto**: todo va normalizado a `~` en el snapshot (§8.2).
- **Comandos de MCP y hooks**: antes de aplicar se comprueba que existan. Si falta alguno, queda como
  «instalar X» en la lista de pendientes; no se aplica a medias.
- **SO**: lo de Desktop es solo macOS. En Linux se ignora y se avisa.
- **Lo que no viaja**: tokens OAuth, `machineID`, sesión de Desktop, lanzadores. Los lanzadores se
  reconstruyen con `ccp desktop app`.
- **Excepciones por máquina**:
  - Un elemento puede llevar un selector `machines: [<id>…]`, por ejemplo un perfil que solo existe
    en el portátil del trabajo.
  - Los ajustes puramente locales viven en `ccp.local.yaml`, que **no** se sincroniza.

## 12. Orden de entrega

| Fase | Entrega | Depende de | Tamaño | Sale cuando… |
|---|---|---|---|---|
| 0 | M1–M6 medidos + ADR de Desktop; B1–B5 | — | S | Cada «?» de §3 tiene respuesta |
| A | `ccp scan`, `ccp adopt`, P-19 | 0 | M | En esta máquina aparecen los 6 MCP de Desktop y se proponen como global |
| B | Proyección de MCP a CLI y a Desktop; skills y agents por perfil; hooks y permisos editables; deriva | 0, A | L | Un MCP añadido al perfil `work` aparece en `claude` y en la ventana de `work` tras `profile sync` |
| C | P-20 + editores + serve y CLI `ccp mcp` | B | L | Todo lo de §3 se puede leer desde la GUI, y editar lo que es editable |
| D | `ccp snapshot *`, retención, restore selectivo, P-17 → Snapshots | A (clasificación) | M | Un restore selectivo de un solo MCP deja la proyección al día |
| I | Infra: stack `ccp-cloud` en Dokploy (Postgres + Keycloak en `ccp-auth.joseiz.com` con realm `ccp` + Alarik en `ccp-s3.joseiz.com`), desde `deploy/ccp-cloud/` | — | S | Un login de prueba por flujo de dispositivo obtiene un token con `aud: ccp-api` |
| F1 | Vault, dispositivos, push/pull de snapshots | D, I | L | Una segunda Mac se desbloquea con la frase de bóveda y trae el historial de la primera |
| F2 | Portal: dispositivos, historial, diff, descarga | F1 | M | Desde `ccp.joseiz.com` se descarga un `.ccpsnap` y se importa en otra máquina |
| F3 | Restaurar desde el portal | F2 | L | «Restaurar en <máquina>» en el portal deja la máquina en ese snapshot, con lo ejecutable confirmado en local |
| F4 | Grupos, auditoría, rotación de AK | F3 | M | Un cambio aplicado a un grupo aparece como `aplicada` en cada máquina |
| E | (aplazado, D10) `ccp sync` sobre carpeta o S3 | D | M | — |

**Dos vías en paralelo tras la Fase 0 (D10):**
- **nube**: A (solo la clasificación) → D → I → F1 → F2 → F3 → F4;
- **configuración**: A completo → B → C.

La vía nube puede empezar en cuanto D tenga el formato, y la I (infra) no depende de código.

## 13. Decisiones (aceptadas el 2026-09-18: todas con la recomendación; D2 con Alarik en lugar de MinIO)

| # | Decisión | Elegido | Alternativa y coste |
|---|---|---|---|
| D1 | Cifrado de la nube | **E2E**: el servidor no lee nada | Legible por el servidor: portal más simple y diff en servidor, pero un servidor comprometido = RCE en todas las máquinas y tus claves en claro |
| D2 | Alojamiento | **Go + Postgres + Alarik en Dokploy** (decidido 2026-09-18) | Supabase o Vercel: menos operación, pero la autenticación E2E es propia de todos modos y se pierde compartir código Go |
| D3 | Alcance de usuarios | **Multiusuario en el esquema, alta por invitación** | Producto abierto: facturación, abuso y términos legales |
| D4 | Conversaciones en snapshots | **Desactivado por defecto**, opcional | Siempre: GB por máquina y datos sensibles en la nube |
| D5 | Adoptar un `~/.claude-x` existente | **Copiar + un `/login`** | Por referencia (`cc_home:`): conserva el login (si M4 lo confirma), pero toca `_env` (§15) |
| D6 | Aplicar cambios remotos | **Auto para lo no ejecutable; confirmar lo ejecutable** | Todo automático: cómodo, pero una cuenta robada ejecuta código |
| D7 | Proyección de MCP al CLI | **`.claude.json`** (técnica A): M6 confirmó que Claude Code conserva lo escrito desde fuera (ADR 0016) | Plugin local: sin carreras, pero los nombres cambian a `mcp__plugin_…` |
| D8 | Ventana `default` | **Solo inventariar**, proyectar con opt-in | Proyectar siempre: ccp gestionaría tu Claude principal |
| D9 | Autenticación | **Keycloak propio del stack `ccp-cloud` (realm `ccp`) para la identidad + frase de bóveda aparte para el cifrado** | Derivar la clave de la contraseña: imposible con Keycloak, porque la contraseña no pasa por `ccp`, y un reset borraría los datos. Quitar el E2E para usar solo Keycloak: el servidor leería tus claves y podría ordenar comandos |
| D10 | Orden | **La nube antes que el editor**: dos vías en paralelo; E aplazado | El orden original (A→B→C→D→E→F) retrasaba el backend hasta el final |

## 14. ADRs que salen de aquí

- **0011** — Una fuente declarada y varias proyecciones. MCP por capas; ccp solo toca los nombres que
  gestiona; destino chat de Desktop.
- **0012** — Snapshots direccionados por contenido con rutas lógicas y clases de elemento.
- **0013** — Nube con cifrado de extremo a extremo: el servidor no lee la configuración.
- **0014** — El portal propone y la máquina aplica: pull, revisiones firmadas y confirmación local de
  lo ejecutable.
- **0015** — Identidad en Keycloak y cifrado en la bóveda: dos secretos con dos dueños.
- **0016** — Qué lee Claude Desktop de un perfil: las mediciones M1–M6 de la Fase 0. Enmienda a 0008
  y 0009 donde toque.

## 15. Contrato y compatibilidad

- **Contrato congelado** (`_env`, `_hook`, `resolve`, `path test`, `status --json`, shellinit):
  ninguno de los subproyectos A–F lo cambia, **salvo** dos puntos:
  - La adopción por referencia (D5, no recomendada de entrada) cambiaría el valor de
    `CLAUDE_CONFIG_DIR` que emite `_env`. Obligaría a enseñárselo al oráculo bash y a regenerar el
    golden.
  - Los subcomandos nuevos (`scan`, `adopt`, `mcp`, `snapshot`, `sync`, `cloud`) cambian
    `completion bash|zsh`, que **sí** está en el golden. Hay que actualizar `legacy/` y ejecutar
    `capture.sh`, como se hizo con `desktop`. Llegan a través de la ruta genérica `*) command ccp`,
    así que no hace falta reinstalar el rc; basta `ccp install` para refrescar el completado.
- **`ccp.yaml`**: los bloques `mcp:`, `sync:` y `cloud:` son aditivos. `version` sigue en `2`, viajan
  por `Config.Extra`, y cada bloque lleva su `Extra` inline con su `strip*KnownKeys`, igual que
  `auto_handoff`.
- **serve**: todo son métodos nuevos, así que el protocolo sigue en `1`.
- **Bilingüe**: cada cadena nueva, en español y en inglés (catálogo CLI y `i18n_en.ts`).
- **Gates**: gofmt, vet, `go test`, golangci-lint v2.12.2, `legacy/tests/run.sh`, `capture.sh --check`,
  y en `gui/` typecheck + `cargo test/clippy/fmt`.
- **Pruebas**:
  - Todo motor nuevo (`inventory`, `adopt`, proyección, `snapshot`, reconciliación) es puro, con
    raíces y sondas inyectadas, para montar máquinas falsas en tests Linux.
  - Nunca contra el `~/.config/ccp` real: `CCP_HOME` temporal y el sandbox de la GUI.

## 16. Riesgos

| Riesgo | Mitigación |
|---|---|
| Claude Code o Desktop cambian formatos internos (`.claude.json`, `claude_desktop_config.json`, lectura de la pestaña Code) | Se escriben solo los nombres gestionados; `projection_stale` y M1–M6 van como tests de humo reproducibles tras cada actualización de Claude.app |
| Carrera al escribir `.claude.json` con un `claude` vivo | M6 decide; plan B = plugin local |
| Proyectar a una ventana abierta sin que se entere | Marca «pendiente de reiniciar», nunca un reinicio implícito |
| Snapshot con secretos que se escapa | Secretos siempre cifrados con `age`; manifiesto sin valores; `0700` |
| Cuenta de nube robada | E2E + MFA + aprobar dispositivos desde otro dispositivo + confirmación local de lo ejecutable |
| Alarik está en beta: durabilidad o compatibilidad S3 por demostrar | El servidor solo guarda texto cifrado y los snapshots locales siguen siendo la fuente primaria; réplica del bucket a un segundo destino; prueba de contrato S3 en CI; el backend solo usa S3 genérico, así que cambiar de almacenamiento es cambiar el endpoint |
| Keycloak caído o mal configurado | La sincronización se pausa y avisa, y `ccp` sigue funcionando en local. El realm es código versionado (`realm-ccp.json`), reimportable. Keycloak se incluye en el simulacro de restauración |
| Reimportar la plantilla de Dokploy regenera todos los secretos y deja sin acceso a Postgres | Solo se importa en el primer despliegue; los cambios posteriores editan el compose y los file mounts (`deploy/ccp-cloud/README.md`) |
| Pérdida de la frase de bóveda | Código de recuperación al crear la bóveda, y alta desde otro dispositivo; los snapshots locales siguen siendo la fuente primaria |
| Alcance: F es un producto en sí mismo | E entrega la replicación sin servidor; F se parte en F1–F4 y cada uno aporta algo por sí solo |
