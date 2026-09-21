# 11. Una fuente declarada y varias proyecciones

Fecha: 2026-09-19

## Estado

Aceptada. Implementa el §6 del spec
[2026-09-18-config-unificada-snapshots-nube](../superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md)
y decide con él D7 y D8. Se apoya entera en las mediciones del
[ADR 0016](0016-what-desktop-reads-from-a-profile.md) — qué lee de verdad cada destino — y no cambia el
merge de settings del [ADR 0002](0002-settings-overlay-jq-deep-merge.md): lo amplía con una unión que hay
que pedir por escrito.

## Contexto

Hasta la Fase B, un MCP solo podía declararse en dos sitios y ninguno servía para lo que la gente quería:

- `~/.claude.json` (el scope *user*) lo lee **solo** `default`, porque cada perfil tiene su propio
  `CLAUDE_CONFIG_DIR` y, con él, su propio `.claude.json`;
- `<repo>/.mcp.json` acota al repo, no a la cuenta.

Así que «quiero este MCP en la cuenta *work*, en su terminal y en su ventana» no tenía respuesta:
`ccp instruct add profile mcp` devolvía el código 5 («no está soportado todavía»), y lo mismo pasaba con
agents, commands y skills, que el código 3 mandaba al global. El usuario acababa editando a mano tres
archivos que Claude Code y Claude Desktop reescriben por su cuenta.

Lo que el 0016 midió cambia el planteamiento: el `.claude.json` de un perfil **conserva** lo que se le
escribe desde fuera (M6), el chat de Desktop lee otro archivo distinto y solo admite `stdio` (M3), y no lo
relee en caliente (M3, M5). Es decir: hay una fuente que el usuario declara y **varios archivos ajenos**
donde eso tiene que aparecer. Ninguno de ellos es propiedad de ccp.

## Decisión

**Se declara una vez, en capas, y ccp proyecta a cada destino. Lo proyectado es derivado y se puede
reconstruir; lo declarado es lo único que el usuario edita.**

### 1. Las capas de MCP

| Capa | Archivo | Quién la edita |
|---|---|---|
| Global | `~/.claude.json:mcpServers` | el usuario, o `ccp instruct add global mcp` |
| Perfil | `profiles/<n>/overlay/mcp.json`, con la forma `{"mcpServers": {…}}` | `ccp instruct add profile mcp` |
| Proyecto | `<repo>/.mcp.json` | el repo; ccp lo lista, **no lo proyecta** (Claude Code ya lo lee por cwd) |
| Metadatos | el bloque `mcp:` de `ccp.yaml` | `targets` por servidor, `disabled` por perfil, `desktop_default` |

**Efectivo de un perfil = global ⊕ overlay − `disabled`**, y si un nombre choca gana el perfil
(`MCPEffective`, `core/mcp_layers.go`). El overlay tiene la forma de `.mcp.json` a propósito: se lee sin
ccp, y una herramienta ajena lo entiende.

El bloque `mcp:` es **aditivo**: `version` sigue en `2`, así que un binario viejo lo preserva por
`Config.Extra` en vez de rechazar el archivo. Como en `auto_handoff`, `MCPConfig` lleva su propio `Extra`
con `yaml:",inline"`, y `stripMCPKnownKeys` quita de ahí las claves tipadas para que no se serialicen dos
veces.

### 2. La proyección: ccp solo toca los nombres que registró como suyos

Los destinos son archivos que otras aplicaciones escriben. La regla que hace esto seguro es una sola:

> ccp añade, actualiza y retira **únicamente** las entradas cuyo nombre figura en su registro de
> gestionados. Lo que el usuario puso a mano se queda. Si un nombre declarado ya existía a mano en el
> destino, **no se pisa**: se informa como conflicto.

El registro es `cc-home/.ccp-managed.json` (CLI) y `<data-dir>/.ccp-managed-mcp.json` (chat). Es estado
**derivado**: si se pierde, `reconcileManaged` lo reconstruye dando por gestionado todo nombre declarado
cuyo valor en el destino ya sea idéntico al declarado — si coinciden, da igual quién lo escribió. Nunca se
captura en snapshots ni en backups.

### 3. Al CLI y a la pestaña Code: `cc-home/.claude.json` (técnica A, D7)

Se lee, se fusiona en `mcpServers` y se escribe con tmp+rename conservando **todas** las demás claves.
Es la técnica A que M6 validó: Claude Code conserva lo escrito desde fuera mientras corre. La alternativa
—un plugin local— renombraba los servidores a `mcp__plugin_…` y eso sí lo nota el usuario.

`default` no se proyecta aquí: su destino **es** la capa global (`~/.claude.json`). Proyectarlo sería
copiar un archivo sobre sí mismo.

### 4. Al chat de Desktop: `profiles/<n>/desktop/claude_desktop_config.json`, y con la ventana cerrada

Dos límites salen medidos del 0016 y los dos están en el código, no en la documentación:

- **Solo `stdio`** (M3). Una entrada `http`/`sse` la descarta Desktop al arrancar («Skipped invalid MCP
  server config entries») y la deja en el archivo, así que ni se escribe: sale en `remote_skipped` para
  que la UI diga que en el chat eso va como conector de la cuenta o envuelto en un puente.
- **No se escribe con la ventana corriendo** (M3 + M5). Desktop no relee el archivo en caliente y lo
  reescribe desde su copia en memoria, así que una escritura en caliente se pierde o pelea. Con la
  ventana viva la proyección queda **aplazada** (`profiles/<n>/state/desktop-pending.json`) y se aplica al
  siguiente arranque. Nunca se mata una ventana por sorpresa.

La sonda de «¿está corriendo?» **no** vive en `core`: `internal/cli` la inyecta con
`SetDesktopRunningProbe`, igual que `SetAutoHooksBin`. Sin inyectar se responde «no corre», que es lo que
vale en los tests.

**La ventana `default` no recibe proyección** (D8). Es el Claude del usuario, el mismo motivo por el que
solo a ella se le respetan las actualizaciones (0009). Se inventaría siempre; `desktop_default: true` está
reservado para activarlo y hoy es una declaración sin efecto: `projectProfileMCP` sale antes para
`default`.

### 5. Un servidor con destino `[cli, desktop]` va a los dos archivos

M2 midió que la pestaña Code **hereda** los MCP del chat y que un nombre repetido arranca dos procesos,
aunque solo se use el de Desktop. Aun así se escribe en los dos sitios: el `cc-home` es el mismo para la
terminal y para la pestaña Code, así que quitarlo del `.claude.json` dejaría sin él a la CLI, que es el
caso principal. Se acepta el proceso ocioso — es la misma definición y gana la de Desktop.

### 6. Skills, agents, commands y output-styles por perfil

`overlay/{agents,commands,skills,output-styles}/` pasan a existir, y con ellos desaparecen los códigos 3 y
5 de `InstructDest`. La regeneración los proyecta al `cc-home` con `mirrorTree`: **directorios reales y
symlinks solo en las hojas**, que es la forma que Desktop exige (0008, «symlink at a non-leaf component»).
Se espeja primero el overlay y luego el global, y como `mirrorTree` nunca pisa una entrada que ya existe,
la precedencia sale gratis: si chocan, gana el perfil.

`seedCCHome` **no** cambia. El oráculo bash exige que justo después de `profile add` el `cc-home` tenga
symlinks de directorio, y eso sigue siendo cierto mientras el perfil no declare nada propio: la conversión
ocurre la primera vez que hay algo en `overlay/<dir>`.

### 7. Permisos: la unión se pide por escrito

El merge de ccp reemplaza arrays (0002), así que un `permissions.allow` en el overlay tapa la lista global
entera. Eso **no cambia**: hacerlo en silencio rompería perfiles que cuentan con ello. Quien quiera sumar
lo dice en su overlay:

```json
"permissions": { "$merge": "union", "allow": ["Bash(make:*)"] }
```

y entonces `allow`, `deny` y `ask` salen como global ⊎ overlay, sin duplicados y con el global primero. La
marca es instrucción para ccp, no configuración de Claude Code: nunca llega al `settings.json` generado.

Los **hooks** se editan como el array entero de su evento (`SettingsLayerSet(home, src, layer, ["hooks",
"PreToolUse"], …)`). Eso es lo que resuelve «se añaden pero no se borran»: para quitar uno se escribe la
lista sin él.

### 8. La proyección entra en TODA regeneración, y se puede preguntar sin escribir

`CfgRegenerateReport` proyecta MCP y artefactos después de fusionar los settings, por el mismo motivo por
el que la adopción de la deriva `/config` está ahí y no solo en `profile sync`: si un único camino
proyectara, cualquier otro (un `instruct add`, un restore, un rename) dejaría el destino desfasado.

El modo *check* es **el mismo motor con `dry=true`** (`CheckMCPToCLI`, `CheckMCPToDesktop`,
`ProfileProjectionCheck`), no un cálculo paralelo. Un comparador escrito aparte acaba respondiendo otra
cosa que la escritura — el mismo bug que evita compartir `traceMove` en el supervisor.

`ProjectionCheck.Stale()` cuenta lo que un `sync` **arreglaría** (escrituras, retiradas, artefactos sin
espejar, lo aplazado, y los dos archivos generados: `cc-home/settings.json` y `cc-home/CLAUDE.md`, que se
construyen con los mismos `cfgBuildSettings`/`cfgBuildClaudeMD` de la escritura y se comparan sin adoptar
ni escribir nada — el settings **por valor**, porque `/config` reescribe el archivo con `JSON.stringify` y
`30.0` → `30` no es algo que un sync arregle). Dejarlos fuera era el agujero que hacía decir «todo lo
declarado está donde lo leen las apps» con el perfil desfasado por el caso más común de todos: cambiar el
global o el overlay y olvidar el sync. **No** cuentan los conflictos ni lo que el chat no puede cargar: son estados
permanentes que decide el usuario, y meterlos en el `Stale` dejaría `ccp profile sync --check` en 1 para
siempre por algo que ningún sync cambia.

## Consecuencias

**A favor**

- Un MCP se declara una vez en el perfil y aparece en `claude`, en la pestaña Code y en el chat de esa
  ventana, sin editar a mano tres archivos que sus dueños reescriben.
- Lo que el usuario puso a mano en cualquiera de esos archivos sobrevive a todas las regeneraciones, y un
  choque de nombres se cuenta en vez de resolverse a espaldas de nadie.
- `ccp profile sync --check` (y `profiles.drift` en serve) contesta «¿está donde las apps lo leen?» sin
  tocar nada, con la misma regla que decide el código de salida, así que la GUI no la recalcula sumando
  listas por su cuenta.
- El doctor gana cinco hallazgos que antes solo se descubrían usando Claude: `projection_stale`,
  `desktop_restart_pending`, `mcp_command_missing`, `mcp_unmanaged_only_desktop` y
  `cc_home_symlink_nonleaf` (un symlink de directorio bajo un `cc-home` rompe la pestaña Code).

**En contra, y aceptado**

- Los **secretos de un MCP** (`env.*`, `headers.*`) se proyectan **en claro**, porque así los leen las
  apps. No es un cambio —hoy ya están en claro en esos archivos— pero ahora también viajan en
  `overlay/mcp.json`, que por eso se captura con clase `secret` en los snapshots (sellado), a diferencia
  del resto del overlay.
- Un servidor con los dos destinos deja un proceso ocioso en la pestaña Code (§5).
- El registro de gestionados puede desincronizarse si alguien edita el destino con ccp parado. Se
  reconstruye por contenido, pero un nombre declarado cuyo valor se editó a mano pasa a contar como
  conflicto — que es lo correcto, aunque sorprenda.
- `desktop_default` existe en el esquema y no hace nada todavía. Se documenta como reservado en vez de
  quitarlo, para no volver a cambiar la forma de `ccp.yaml` cuando se active.

## Alternativas descartadas

- **Un plugin local con los MCP** (D7, alternativa). Sin carreras con Claude Code, pero los nombres pasan
  a `mcp__plugin_…` y eso rompe cualquier referencia escrita por el usuario.
- **Matar la ventana para escribir su config.** Es la única forma de que el chat vea un MCP al instante;
  también es tirar el trabajo de alguien sin avisar. Se aplaza y se avisa.
- **Cambiar el merge de arrays a unión por defecto.** Enmendar el 0002 en silencio; un `permissions.deny`
  que el perfil creía que reemplazaba pasaría a sumarse al global. La unión se pide por escrito.
- **Calcular la deriva con un comparador propio.** Dos implementaciones de la misma pregunta se separan;
  el check es la proyección con la escritura apagada.
