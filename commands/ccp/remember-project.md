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
4. **Escribe**:
   ```
   ccp instruct add project rule "<texto redactado>"
   ```
5. Si ccp responde que no estás en un repo git, avísale al usuario (escribirá en el cwd como fallback).
6. Reporta la ruta destino. Recuérdale que el cambio queda en `.claude/` y conviene comitearlo.
