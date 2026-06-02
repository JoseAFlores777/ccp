---
description: Persiste una instrucción/artefacto de Claude a nivel GLOBAL (~/.claude, todos los perfiles)
argument-hint: <lo que quieres que Claude recuerde>
---

El usuario quiere que recuerdes algo a nivel **global** (`~/.claude`, aplica a todos los perfiles de ccp).

Input del usuario: $ARGUMENTS

Pasos:
1. **Clasifica** el tipo de artefacto según lo que pide:
   - `rule` — una directiva de comportamiento ("siempre X", "nunca Y", preferencias). Es el caso por defecto.
   - `agent`, `command`, `skill` — si pide crear un subagente, un slash command, o una skill (ver el paso 4 para cómo escribirlos).
   - `hook`, `mcp` — si pide un hook o un servidor MCP.
2. **Redacta** el texto de la instrucción en imperativo, claro y conciso (una sola línea para `rule`).
3. **Confirma** con el usuario: muestra `tipo`, `destino` y el `texto` exacto que vas a escribir. No escribas sin confirmación.
4. **Escribe** llamando al CLI (él es el dueño de la mecánica):
   - Para `rule`: `ccp instruct add global rule "<texto>"`.
   - Para `mcp`: construye el objeto de configuración del server y llama
     `ccp instruct add global mcp 'nombre={"command":"...","args":[...]}'`.
   - Para `hook`: construye el objeto de hook en formato oficial
     `{"hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"..."}]}]}}`
     y llama `ccp instruct add global hook 'id={...}'`. Avísale al usuario que el borrado de hooks es manual.
   - Para `agent`/`command`/`skill`:
     1. Pide la ruta destino: `ccp instruct dest global <type>` (devuelve el directorio oficial).
     2. Escribe el archivo del artefacto ahí con un slug claro (p.ej. `<dir>/auditor-seguridad.md`), en el formato oficial de Claude Code (frontmatter `name`/`description` + cuerpo para agent/command; estructura de skill para skill).
     3. Regístralo: `ccp instruct record global <type> "<ruta-escrita>" "<descripción corta>"`.
5. Reporta la ruta destino que devolvió ccp.

Reglas:
- NO edites `~/.claude/CLAUDE.md` a mano: usa `ccp instruct add` (mantiene el bloque gestionado con marcadores).
- Si ccp responde "ya existía", díselo al usuario; no reintentes.
