# 16. Qué lee Claude Desktop de un perfil, medido

Fecha: 2026-09-18

## Estado

Aceptada. Cierra las mediciones M1–M6 de la Fase 0 del spec
[2026-09-18-config-unificada-snapshots-nube](../superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md)
(§3, §4) y decide D7. No contradice a [0008](0008-desktop-launcher-two-layer-bundle.md) ni a
[0009](0009-desktop-identity-is-not-durable.md): confirma lo que 0008 daba por hecho (la pestaña Code lee
`CLAUDE_CONFIG_DIR`) y añade lo que ninguno de los dos miraba, el MCP.

## Contexto

El subproyecto B quiere declarar la configuración una vez (global, perfil, proyecto) y proyectarla a la CLI y a
Claude Desktop. Antes de escribir una línea hacía falta saber qué lee de verdad cada destino. La lección de 0009
es que en Desktop las suposiciones salen caras: aquel día se dio por cierta una identidad que no era durable.

## Método

- Un perfil desechable, `probe-desktop` (official), creado con el ccp instalado (v2.18.0) y preparado con
  `ccp desktop prepare`. Su ventana es un lanzador de dos capas (0008). `ccp desktop doctor` no encontró
  problemas antes de iniciar sesión.
- Marcadores únicos (`CCP-PROBE-*`) en cada archivo del cc-home y de la ventana, y un servidor MCP mínimo propio
  (`/tmp/ccp-probe/mcp_probe.py`: stdio, una herramienta `probe_origin` que devuelve la etiqueta con la que se
  lanzó y deja constancia de quién lo arrancó). Con `/bin/sleep` como servidor, el resultado de M2 **engañaba**
  (ver M2), así que las conclusiones de MCP salen del servidor real.
- Evidencia leída por fuera, nunca inferida de la respuesta del modelo: transcripts, `ps`, los logs de MCP de
  Claude Code (`~/Library/Caches/claude-cli-nodejs/<proyecto>/mcp-logs-<servidor>/`), los de Desktop
  (`~/Library/Logs/Claude/`) y los atributos del Llavero (`security`, nunca el secreto).
- Versiones: Claude.app **2.2553.1**. La CLI del usuario es Claude Code **2.1.236**. La pestaña Code **no la
  usa**: cada ventana descarga su propio Claude Code en `<data-dir>/claude-code/<ver>/claude.app` (la de `e-cc`
  corría 2.1.271 y la recién creada 2.1.275).

Cómo lanza Desktop ese Claude Code (`ps`, idéntico en dos ventanas):
`--input-format stream-json --output-format stream-json --verbose --permission-prompt-tool stdio
--setting-sources=user,project,local --permission-mode <el de la UI> --await-initialize --settings {"deniedMcpServers":[…sus internos…]}`.
Sin `--mcp-config` ni `--plugin-dir`: lo que Desktop añade llega por el `initialize` del stream-json.

## Mediciones

### M1 — qué lee la pestaña Code del cc-home

| Elemento | Resultado | Observado |
|---|---|---|
| `CLAUDE.md` con `@import` fuera del config root | ✓ | la respuesta termina en `CCP-PROBE-CLAUDEMD` |
| `env` de `settings.json` | ✓ | `echo $CCP_PROBE_ENV` → `CCP-PROBE-ENV` |
| hook `SessionStart` | ✓ | aparece `/tmp/ccp-probe/hook-ran` |
| `permissions.allow` | ✓ | `echo CCP-PROBE-PERM` con `permissionMode: default`: tool_use → tool_result en 0,40 s, sin diálogo |
| skill en `cc-home/skills` | ✓ | lista `ccp-probe` |
| agente en `cc-home/agents` | ✓ | lista `ccp-probe-agent` |
| `mcpServers` de `cc-home/.claude.json` | ✓ | lo lanza el `claude` de la pestaña con `CLAUDE_CONFIG_DIR=<cc-home>`; `probe_origin` → `origen=cli` |
| `enabledPlugins` (el global, vía el `settings.json` generado) | ✓ | lista las skills de `figma@claude-plugins-official` |

Además, Desktop inyecta en la pestaña lo que es suyo o de la cuenta: sus MCP internos (`ccd_*`, navegador,
simulador, terminal…), los **conectores de la cuenta de claude.ai** (Figma, Vercel, calendario…) y los **plugins
de la cuenta u organización** desde `<data-dir>/local-agent-mode-sessions/skills-plugin` (anthropic-skills,
design, legal…).

### M2 — ¿Code hereda los MCP del chat? ¿Qué pasa con un nombre repetido?

- **Sí los hereda.** `ccp-probe-desktop`, definido solo en `claude_desktop_config.json`, responde en la
  pestaña Code (`origen=desktop`). No lo lanza el `claude` de la pestaña: lo sirve el proceso de Desktop desde
  un *shared pool* «for Cowork and Code sessions».
- Con un servidor que no habla MCP (`sleep`), el pool falla («Couldn't start for Cowork and Code sessions …
  the sessions waiting for it started without it») y la sesión arranca **sin** él. Por eso la primera lectura
  con `sleep` decía, en falso, que no se heredaba.
- **Nombre repetido** (`ccp-probe-both` en los dos archivos): aparece una sola vez y **gana Desktop**
  (`origen=both-desktop`). Pero el `claude` de la pestaña **también lanza y conecta** su copia
  (`ccp-probe-both-cli`, «Connection established»). Es un proceso de más que nadie usa.
- Desktop arranca cada MCP local tres veces al abrirse, todas colgando de su helper `disclaimer`.

### M3 — el chat y las entradas remotas; ¿se relee en caliente?

- Una entrada `{"type":"http","url":…}` en `claude_desktop_config.json` se **rechaza**. En `main.log`:
  «Skipped invalid MCP server config entries: { invalidServers: [ 'ccp-probe-http' ] }». La deja en el
  archivo, pero no la carga. El archivo local del chat solo admite stdio.
- **No se relee en caliente**: una entrada stdio nueva escrita con la ventana abierta no arrancó en 40 s y no
  dejó log. Al reiniciar la ventana, sí arrancó.

### M4 — con qué nombre guarda Claude Code las credenciales

Atributos del Llavero, sin leer ningún secreto:

- Sin `CLAUDE_CONFIG_DIR`, el servicio es `Claude Code-credentials`.
- Con `CLAUDE_CONFIG_DIR=<dir>`, el servicio es `Claude Code-credentials-<sha256(<dir>)[:8]>`. Cuadran los cuatro
  cc-home que había: `a-cc` → `58cc11f4`, `e-cc` → `caec6132`, `personal-cc` → `a98f4e7a` y `probe-desktop`
  → `ca7ccbe6`. **Depende de la ruta literal.**
- La entrada de `probe-desktop` la creó el Claude Code de la pestaña a las 18:44:04, al arrancar la primera
  sesión; el login de Desktop además escribe `oauthAccount` en `cc-home/.claude.json`. Aun así, la CLI del
  usuario (2.1.236) con el mismo `CLAUDE_CONFIG_DIR` **pidió `/login`**, y al hacerlo reescribió esa misma
  entrada (mdat 19:21:31). La causa no se estableció, porque exigía leer el secreto. **El login de Desktop no
  deja iniciada la CLI de ese perfil.**

### M5 — ¿Desktop reescribe `claude_desktop_config.json`?

- Es su almacén de `preferences` (Cowork, permisos por carpeta, estado de paneles…). Lo **reescribió mientras
  corría** (19:03:27, con la ventana abierta desde las 18:51) y **conservó los `mcpServers` escritos a mano**
  antes de arrancar.
- Con entradas escritas desde fuera y la ventana abierta, no tocó el archivo en 40 s ni al cerrar con ⌘Q.
- **Sin medir:** si una reescritura de Desktop *posterior* a una escritura en caliente la conserva (si relee el
  archivo antes de escribir o vuelca su copia en memoria). No se produjo ninguna en la ventana observada.

### M6 — ¿Claude Code conserva un `mcpServers` escrito desde fuera mientras corre?

- **Sí.** Con una sesión interactiva abierta (CLI 2.1.236, `CLAUDE_CONFIG_DIR=<cc-home>`), se añadió
  `ccp-probe-external` a `.claude.json` a las 19:21:52 con escritura atómica. Claude Code reescribió el archivo
  a las 19:22:49 (tras un mensaje y un `/config`) y a las 19:23:30 (`/exit`), y la entrada **siguió** ahí las
  dos veces.

## Decisión

1. **D7 = técnica A.** El MCP de un perfil se proyecta a la CLI fusionando solo los nombres gestionados en
   `cc-home/.claude.json:mcpServers`, con lectura, modificación y escritura atómica bajo flock, sin cachear
   nada. Claude Code reescribe ese archivo a menudo (M6), así que ccp nunca escribe una copia vieja. El plugin
   local (técnica B) queda descartado: habría renombrado los servidores a `mcp__plugin_…`.
2. **Un servidor que va a la CLI y a Desktop se escribe en los dos archivos.** El cc-home lo comparten la CLI
   y la pestaña Code, así que no hay forma de dárselo a la terminal sin que la pestaña lo vea también. En la
   pestaña gana la copia de Desktop (M2), que es la misma definición, y la otra queda como un proceso ocioso. Se
   acepta ese coste. Se descarta escribirlo solo en Desktop, porque la terminal se quedaría sin él.
3. **Al chat de Desktop solo van servidores stdio** (M3). Un servidor remoto con destino `desktop` se proyecta
   envuelto en el puente `mcp-remote`, o la UI dice que en el chat solo puede ir como conector de la cuenta.
4. **Proyectar a Desktop exige reiniciar la ventana** (M3). Si la instancia está corriendo, el perfil queda
   «pendiente de reiniciar la ventana» y la GUI lo ofrece; nunca se mata una ventana por sorpresa. Y como M5 no
   descarta que Desktop vuelque una copia en memoria sobre una escritura en caliente, ccp **escribe
   `claude_desktop_config.json` con la ventana cerrada** o, si está abierta, relee el archivo después del
   reinicio y vuelve a proyectar lo que falte.
5. **La pestaña Code recibe todo lo del cc-home** (M1), así que la tabla §3 del spec pasa sus «?» de la columna
   Code a ✓. El chat sigue recibiendo solo MCP.

## Consecuencias

Defectos nuevos, para arreglar como B1–B5:

- **B6 — `/config` dentro de un perfil se pierde.** `/config` escribe en `cc-home/settings.json`
  (`autoCompactEnabled: false`, 19:22:29), que es un archivo generado por ccp. El siguiente `ccp profile sync`,
  que también ejecuta `ccp upgrade`, lo quitó. ccp tiene que detectar esa deriva y adoptarla en el overlay
  antes de regenerar, o avisar de que se va a perder.
- **B7 — `ccp profile rename` deja el perfil sin login.** El nombre de la credencial depende de la ruta del
  cc-home (M4), y el rename lo mueve. El rename tiene que avisar y pedir un `/login`. Mover la credencial de un
  nombre a otro tocaría secretos del Llavero, y eso ccp no lo hace.
- **B8 — `ccp profile add` no genera la config del perfil.** Dice «config generated», pero el perfil nace sin
  `overlay/`, `cc-home/CLAUDE.md` ni `settings.json` hasta el primer sync. El oráculo bash (`_seed_cc_home`) sí
  los genera; la versión Go se quedó en la siembra.

Otras:

- **D5 se confirma.** Adoptar un `~/.claude-x` copiándolo necesita un `/login`, porque cambia la ruta y con ella
  el nombre de la credencial. Adoptarlo por referencia conservaría el login.
- Restaurar un snapshot en otra máquina, o con otro `HOME`, siempre pide un `/login` por perfil. Es lo correcto:
  las credenciales no viajan (ADR 0012).
- La pestaña Code corre **su propia** versión de Claude Code, distinta por ventana y de la CLI. Cualquier
  compatibilidad que ccp dé por hecha en la CLI hay que comprobarla también ahí.
- `ccp desktop open` pasa a la ventana el `PATH` de quien lo invoca. Abierta desde el Dock no lo hace. Menor.
- Los logs de todas las ventanas, perfiles incluidos, comparten `~/Library/Logs/Claude`. No se aíslan.
