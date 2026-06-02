---
description: Persiste una instrucción/artefacto al PROYECTO (.claude/ del repo git actual, versionado)
argument-hint: <lo que quieres que Claude recuerde para este repo>
---

El usuario quiere recordar algo a nivel del **proyecto** (el `.claude/` de la raíz del repo git actual; se versiona con el código).

Input del usuario: $ARGUMENTS

Pasos:
1. **Clasifica** el tipo (igual que remember-global; project soporta los 6 tipos).
2. **Redacta** el texto.
3. **Confirma** tipo + destino + texto. No escribas sin confirmación.
4. **Escribe** llamando al CLI (él es el dueño de la mecánica):
   - Para `rule`: `ccp instruct add project rule "<texto>"`.
   - Para `mcp`: construye el objeto de configuración del server y llama
     `ccp instruct add project mcp 'nombre={"command":"...","args":[...]}'`.
   - Para `hook`: construye el objeto de hook en formato oficial
     `{"hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"..."}]}]}}`
     y llama `ccp instruct add project hook 'id={...}'`. Avísale al usuario que el borrado de hooks es manual.
   - Para `agent`/`command`/`skill`:
     1. Pide la ruta destino: `ccp instruct dest project <type>` (devuelve el directorio oficial).
     2. Escribe el archivo del artefacto ahí con un slug claro (p.ej. `<dir>/auditor-seguridad.md`), en el formato oficial de Claude Code (frontmatter `name`/`description` + cuerpo para agent/command; estructura de skill para skill).
     3. Regístralo: `ccp instruct record project <type> "<ruta-escrita>" "<descripción corta>"`.
5. Si no estás dentro de un repo git, ccp usa el directorio actual (`$PWD/.claude/`) como fallback — avísale al usuario dónde quedó.
6. Reporta la ruta destino. Recuérdale que el cambio queda en `.claude/` y conviene comitearlo.
