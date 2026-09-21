---
description: Persiste una instrucción/artefacto al PERFIL ACTIVO (overlay del perfil de esta terminal)
argument-hint: <lo que quieres que Claude recuerde para este perfil>
---

El usuario quiere recordar algo a nivel del **perfil activo** (el `CCP_PROFILE` de esta terminal; se escribe a su overlay, en `~/.config/ccp/profiles/<perfil>/overlay/`).

Input del usuario: $ARGUMENTS

Pasos:
1. **Clasifica** el tipo (igual que remember-global). A nivel perfil valen los seis: `rule`, `hook`, `mcp`, `agent`, `command` y `skill`. Desde la Fase B el perfil tiene sus propias capas y la regeneración las proyecta a lo que leen la CLI, la pestaña Code y el chat de su ventana.
2. **Redacta** el texto.
3. **Confirma** tipo + destino + texto. No escribas sin confirmación.
4. **Escribe**:
   - Para `rule`: `ccp instruct add profile rule "<texto>"` → `overlay/CLAUDE.md`.
   - Para `hook`: construye el objeto de hook en formato oficial
     `{"hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"..."}]}]}}`
     y llama `ccp instruct add profile hook 'id={...}'`. El hook se escribe en el overlay del perfil y se regenera al cc-home automáticamente. Avísale al usuario que el borrado de hooks es manual.
   - Para `mcp`: `ccp instruct add profile mcp 'nombre={"command":"...","args":[...]}'` → `overlay/mcp.json`. Avisa de dos cosas: al **chat** de la ventana de Desktop solo llegan servidores `stdio`, y si esa ventana está abierta el cambio queda pendiente hasta que se reinicie. Para acotarlo a un destino se edita `mcp.targets.<nombre>` en `ccp.yaml` (`[cli]`, `[desktop]` o los dos).
   - Para `agent`/`command`/`skill`: `add` no los escribe. Pide el directorio con `ccp instruct dest profile <tipo>` (sale `overlay/{agents,commands,skills}/`), escribe ahí el archivo y regístralo con `ccp instruct record profile <tipo> <ref> "<desc>"`. La regeneración lo une con los del global en el cc-home (si chocan, gana el perfil); lánzala con `ccp profile sync`.
5. Si ccp responde que el perfil activo es `default` (sin overlay), explícale al usuario que `default` = config global, y ofrécele `/ccp:remember-global` o activar un perfil con `ccp use <n>`.
6. Reporta la ruta destino. Si quiere comprobar que llegó a su sitio: `ccp profile sync --check`.
