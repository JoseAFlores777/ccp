---
description: Lista las instrucciones/artefactos que ccp gestiona para el contexto actual
argument-hint: "[global|profile|project]  (vacío = los tres)"
---

Muestra lo que ccp tiene registrado.

Argumento (scope, opcional): $ARGUMENTS

- Si el argumento es `global`, `profile` o `project`: corre `ccp instruct list <scope>` y muestra el resultado.
- Si está **vacío**: muestra los tres, cada uno con su encabezado, corriendo:
  ```
  ccp instruct list global
  ccp instruct list profile   # omítelo si el perfil activo es 'default'
  ccp instruct list project   # omítelo si no estás en un repo git
  ```
- Es solo lectura. Para borrar, dirige al usuario a `/ccp:forget`.
