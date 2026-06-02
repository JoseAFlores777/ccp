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
   - Para `rule`/`hook`/`mcp`: `ccp instruct add project <type> "<texto>"` (hook/mcp llegan en una versión próxima).
   - Para `agent`/`command`/`skill`:
     1. Pide la ruta destino: `ccp instruct dest project <type>` (devuelve el directorio oficial).
     2. Escribe el archivo del artefacto ahí con un slug claro (p.ej. `<dir>/auditor-seguridad.md`), en el formato oficial de Claude Code (frontmatter `name`/`description` + cuerpo para agent/command; estructura de skill para skill).
     3. Regístralo: `ccp instruct record project <type> "<ruta-escrita>" "<descripción corta>"`.
5. Si ccp responde que no estás en un repo git, avísale al usuario (escribirá en el cwd como fallback).
6. Reporta la ruta destino. Recuérdale que el cambio queda en `.claude/` y conviene comitearlo.
