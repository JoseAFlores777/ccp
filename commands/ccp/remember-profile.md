---
description: Persiste una instrucción/artefacto al PERFIL ACTIVO (overlay del perfil de esta terminal)
argument-hint: <lo que quieres que Claude recuerde para este perfil>
---

El usuario quiere recordar algo a nivel del **perfil activo** (el `CCP_PROFILE` de esta terminal; se escribe a su `overlay/CLAUDE.md`).

Input del usuario: $ARGUMENTS

Pasos:
1. **Clasifica** el tipo (igual que remember-global). A nivel perfil solo se permiten `rule` y `hook`; `mcp` por-perfil no está soportado (ccp lo rechazará con rc 5); si pide `agent`/`command`/`skill`, ccp lo rechazará — sugiere usar `/ccp:remember-global` o `/ccp:remember-project`.
2. **Redacta** el texto.
3. **Confirma** tipo + destino + texto. No escribas sin confirmación.
4. **Escribe**:
   - Para `rule`: `ccp instruct add profile rule "<texto>"`.
   - Para `hook`: construye el objeto de hook en formato oficial
     `{"hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"..."}]}]}}`
     y llama `ccp instruct add profile hook 'id={...}'`. El hook se escribe en el overlay del perfil y se regenera al cc-home automáticamente. Avísale al usuario que el borrado de hooks es manual.
   - Para `mcp`: MCP por-perfil **no está soportado**. Usa `ccp instruct add global mcp ...` (aplica a todos los perfiles) o `ccp instruct add project mcp ...` (acota al repo).
   - `agent`/`command`/`skill` NO se permiten a nivel perfil (ccp los rechaza); usa `/ccp:remember-global` o `/ccp:remember-project`.
5. Si ccp responde que el perfil activo es `default` (sin overlay), explícale al usuario que `default` = config global, y ofrécele `/ccp:remember-global` o activar un perfil con `ccp use <n>`.
6. Reporta la ruta destino.
