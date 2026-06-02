---
description: Borra una instrucción/artefacto gestionado por ccp, por scope
argument-hint: "[global|profile|project]"
---

Borra algo que ccp gestiona. Argumento (scope): $ARGUMENTS

Pasos:
1. Determina el scope. Si está vacío, pregunta al usuario cuál (global/profile/project).
2. Lista numerado:
   ```
   ccp instruct list <scope>
   ```
3. Muéstrale la lista y pregunta **qué número** borrar. No borres sin confirmación.
4. Borra:
   ```
   ccp instruct rm <scope> <n>
   ```
5. Confirma el resultado y vuelve a listar si el usuario quiere seguir.

Nunca borres artefactos hechos a mano: `ccp instruct` solo ve lo que ccp creó (bloque gestionado + manifest).
