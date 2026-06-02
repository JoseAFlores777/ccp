# 7. YAML canónico con migración one-way respaldada

Fecha: 2026-06-02

## Estado

Aceptada

## Contexto

El estado de ccp vive hoy en `~/.config/ccp` como formatos heterogéneos: `profiles.tsv`/`rules.tsv`/`authored.tsv` (TSV), `meta` por perfil (`key=value`, parseado no sourced), `config` (defaults), y secretos en archivos aparte (`api_key`, `chmod 600`, más credenciales OAuth dentro de cada `cc-home`). El usuario quiere "configuraciones tipo YAML" y que el update "no afecte la data ya guardada".

Como la cola shell nunca lee estos archivos (solo llama `command ccp`), **el formato interno es libre de cambiar** mientras Go sea el único que lee/escribe.

Alternativas:

1. **Auto-migrar a un `ccp.yaml` canónico** en el primer arranque Go, con backup del estado legacy; un solo formato hacia adelante.
2. **Dual-read indefinido**: leer TSV/meta o YAML, escribir YAML solo en data nueva.
3. **Mantener TSV/meta internos**; YAML solo como formato de export/import.

## Decisión

Tomamos la **1**. El primer arranque del binario Go (el "migrador universal") detecta el estado y migra en cadena — `dsctl`-only → ccp → YAML, o ccp-TSV → YAML — escribiendo un único `ccp.yaml` (mapa de perfiles, reglas de path, defaults). Antes de migrar copia el estado legacy a `~/.config/ccp/.backup-pre-go-<fecha>/` (backup de pre-migración). El lector legacy se conserva **solo** para esa conversión única.

Los **secretos quedan fuera del YAML**: `api_key` sigue como archivo `chmod 600`; las credenciales OAuth siguen dentro del `cc-home`. `cc-home/` y `overlay/` son estructuras nativas de Claude Code y no se "YAML-ifican".

La librería es **goccy/go-yaml** (preserva comentarios y orden en round-trip, mejores errores) porque `ccp.yaml` es editable por humanos.

El dual-read (alternativa 2) deja estado mixto y código de lectura legacy permanente. Mantener TSV (alternativa 3) no cumple "editar config en YAML".

## Consecuencias

- Es una **puerta de una sola vía**: tras la migración, el binario **bash viejo ya no lee YAML**. Un downgrade a bash post-migración rompe.
- Esa puerta es **recuperable**: el `.backup-pre-go-<fecha>/` conserva el TSV/meta originales; el rollback es reinstalar bash + restaurar ese backup.
- La migración debe ser **atómica** (temp + rename) e **idempotente** (re-correrla no duplica ni corrompe).
- Go debe garantizar conversión lossless; el gate golden-diff (estado migrado → mismas salidas `_env`/`_hook` que el bash sobre el estado original) lo verifica.
- Escribir `ccp.yaml` en cada cambio preserva comentarios del usuario gracias a goccy/go-yaml.

## Nota de implementación (schema, 2026-06-02)

Resuelto el detalle del schema (ver el plan §4):

- **Round-trip:** struct + `yaml.CommentMap` (decode captura comentarios por-path, encode los re-adjunta) + un catch-all `inline` (`map[string]any`) que conserva claves desconocidas. Así se honran a la vez la preservación de comentarios y la preservación de campos de versiones futuras, sin edición manual de AST.
- **Forward-compat:** `version` entero; binario con versión conocida **menor** a la del archivo → aborta y no escribe.
- **Sin herencia:** perfiles deepseek guardan sus 4 campos explícitos; `defaults` solo siembra al crear.
- **Asimetría del authored:** el authored global+profile se pliega a `ccp.yaml`; el authored **project** se queda como `.claude/ccp-authored.tsv` (versionado per-repo) — ccp no reescribe archivos del repo del usuario.
- **Secretos:** nunca en `ccp.yaml` (api_key 600 + credenciales en cc-home); su presencia es implícita.
