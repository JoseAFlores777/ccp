---
description: Persiste una instrucción/artefacto al PERFIL ACTIVO (overlay del perfil de esta terminal)
argument-hint: <lo que quieres que Claude recuerde para este perfil>
---

El usuario quiere recordar algo a nivel del **perfil activo** (el `CCP_PROFILE` de esta terminal; se escribe a su `overlay/CLAUDE.md`).

Input del usuario: $ARGUMENTS

Pasos:
1. **Clasifica** el tipo (igual que remember-global). A nivel perfil solo se permiten `rule`, `hook`, `mcp`; si pide `agent`/`command`/`skill`, ccp lo rechazará — sugiere usar `/ccp:remember-global` o `/ccp:remember-project`.
2. **Redacta** el texto.
3. **Confirma** tipo + destino + texto. No escribas sin confirmación.
4. **Escribe**:
   - Para `rule`/`hook`/`mcp`: `ccp instruct add profile <type> "<texto>"` (hook/mcp llegan en una versión próxima).
   - `agent`/`command`/`skill` NO se permiten a nivel perfil (ccp los rechaza); usa `/ccp:remember-global` o `/ccp:remember-project`.
5. Si ccp responde que el perfil activo es `default` (sin overlay), explícale al usuario que `default` = config global, y ofrécele `/ccp:remember-global` o activar un perfil con `ccp use <n>`.
6. Reporta la ruta destino.
