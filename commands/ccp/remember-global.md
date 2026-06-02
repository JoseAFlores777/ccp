---
description: Persiste una instrucción/artefacto de Claude a nivel GLOBAL (~/.claude, todos los perfiles)
argument-hint: <lo que quieres que Claude recuerde>
---

El usuario quiere que recuerdes algo a nivel **global** (`~/.claude`, aplica a todos los perfiles de ccp).

Input del usuario: $ARGUMENTS

Pasos:
1. **Clasifica** el tipo de artefacto según lo que pide:
   - `rule` — una directiva de comportamiento ("siempre X", "nunca Y", preferencias). Es el caso por defecto.
   - `agent`, `command`, `skill`, `hook`, `mcp` — si claramente pide crear uno de estos. (Aún no implementados en `ccp instruct add`; si es el caso, avísale al usuario que por ahora solo se soportan instrucciones de comportamiento.)
2. **Redacta** el texto de la instrucción en imperativo, claro y conciso (una sola línea para `rule`).
3. **Confirma** con el usuario: muestra `tipo`, `destino` y el `texto` exacto que vas a escribir. No escribas sin confirmación.
4. **Escribe** llamando al CLI (él es el dueño de la mecánica):
   ```
   ccp instruct add global rule "<texto redactado>"
   ```
5. Reporta la ruta destino que devolvió ccp.

Reglas:
- NO edites `~/.claude/CLAUDE.md` a mano: usa `ccp instruct add` (mantiene el bloque gestionado con marcadores).
- Si ccp responde "ya existía", díselo al usuario; no reintentes.
