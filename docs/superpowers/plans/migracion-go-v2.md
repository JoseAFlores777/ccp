# Plan de migración de `ccp` a Go — v2.0

**Fecha:** 2026-06-02 · **Estado:** propuesto · **Scope:** reescritura completa del binario y libs a Go, conservando el contrato shell; YAML canónico; backup/restore; TUI bubbletea+huh. Todo en un único release v2.0.

---

## 0. Objetivo e invariantes

Migrar `ccp` de Bash a Go **sin que el usuario pierda data ni configuración** al actualizar. Un `ccp upgrade` (todavía bash en ese momento) deja instalado el binario Go; el siguiente `command ccp` ya es Go y migra los datos a YAML de forma respaldada. El bloque rc no se toca.

**Invariantes que NO pueden romperse (contrato congelado):**

1. La salida de `ccp _env <perfil>` y `ccp _hook <path>` se hace `eval` en el shell. Debe ser equivalente byte-a-byte a la del bash actual (mismo `unset`/`export`, mismo quoting, mismo orden).
2. `ccp completion-shellinit` produce el bloque rc (función `ccp` + hook `_ccp_autocheck` + completions) entre los marcadores `# >>> ccp shell init >>>` / `# <<<`.
3. `ccp completion zsh|bash` produce scripts de completion válidos (el rc los `eval` a al arrancar el shell).
4. Superficie scriptable estable: `ccp resolve [path]`, `ccp path test [path]` (exit 0=no-default, 1=default), `ccp status --json` (objeto con `active`/`profile`/`profile_type`/`cwd`/`repo`).
5. La frontera binario↔shell: el binario EMITE entorno, el shell lo EVALúa. Nunca el binario muta el entorno (corre en proceso hijo).

---

## 1. Decisiones resueltas (grilling 2026-06-02)

| # | Decisión | Elegido |
|---|----------|---------|
| 1 | Alcance Go | Solo el binario; cola shell sigue bash, emitida por Go |
| 2 | Gate de compat | Golden diff bash-vs-go sobre fixtures (bash = oráculo) |
| 3 | Adopción YAML | Auto-migrar a YAML canónico con backup; secretos fuera |
| 4 | Layout YAML | Un solo `ccp.yaml` como fuente de verdad |
| 5 | Alcance backup | Dos niveles: config (default) y full (`--with-secrets`) |
| 6 | Formato backup | `.tar.gz` con `manifest.yaml` + checksum |
| 7 | Restore | No-destructivo: auto-snapshot + skip colisiones; `--overwrite`/`--force` |
| 8 | Alcance TUI | App bubbletea completa con huh embebido |
| 9 | Vistas TUI | 3 paneles: Perfiles \| Reglas \| Estado, acciones vía huh |
| 10 | Distribución | Release prebuilt por OS/arch + checksum, fallback `go build` |
| 11 | Cutover | Parity-first big-bang; bash = oráculo hasta v2.0, luego `legacy/` |
| 12 | Tests | `go test` (core) + golden-diff de integración vs bash |
| 13 | Migración dsctl | Go es migrador universal en cadena (dsctl→ccp→YAML) |
| 14 | Lib YAML | `goccy/go-yaml` (preserva comentarios) |
| 15 | Secuencia | v2.0 incluye todo (core+YAML+backup+TUI) |

ADRs relacionados: [0006](../../adr/0006-rewrite-to-go-keeping-bash-shell-tail.md), [0007](../../adr/0007-yaml-canonical-with-one-way-migration-and-backup.md).

---

## 2. Arquitectura objetivo

**Core en un paquete Go; dos front-ends (CLI y TUI) sobre el mismo core.** La TUI nunca duplica lógica.

Module path: `github.com/JoseAFlores777/ccp` · binario: `ccp` · Go mínimo: **1.23**. El repo de GitHub se **renombra** `dsctl` → `ccp` (alinea repo=binario=module); requiere re-apuntar `install-source` y avisar a clones (GitHub deja redirect).

```
ccp/
├── cmd/ccp/main.go              # entrypoint: parse args -> dispatch (a mano, sin cobra)
├── internal/
│   ├── core/                    # motor (sin I/O de presentación)
│   │   ├── profile.go           # CRUD perfiles (official/deepseek/default)
│   │   ├── rules.go             # path rules, ccp_resolve (deepest wins)
│   │   ├── env.go               # EnvDelta: emite unset/export %q-equivalente
│   │   ├── cfg.go               # overlay engine (CLAUDE.md @import, jq-like merge)
│   │   ├── store.go             # lectura/escritura ccp.yaml (atómica)
│   │   ├── secrets.go           # api_key 600 + credenciales cc-home
│   │   ├── backup.go            # export/restore tar.gz + manifest
│   │   ├── migrate.go           # migrador universal dsctl->ccp->YAML
│   │   └── shellinit.go         # texto bash de la cola shell (heredoc -> string)
│   ├── cli/                     # dispatch de subcomandos, salida texto/JSON
│   └── tui/                     # bubbletea app + formularios huh
│       ├── app.go               # modelo raíz, 3 paneles, navegación
│       ├── profiles.go rules.go status.go
│       └── forms.go             # asistentes huh (add/edit/backup/restore)
├── testdata/golden/             # fixtures CCP_HOME + salidas esperadas
├── install.sh                   # detecta OS/arch, descarga release o go build
├── legacy/                      # bash archivado tras paridad (oráculo hasta v2.0)
└── .github/workflows/           # build matrix + release + golden gate
```

- **`internal/core` no imprime nada de presentación.** Devuelve datos/strings; los front-ends formatean. `env.go` y `shellinit.go` sí producen strings exactos (son el contrato).
- **`internal/core/shellinit.go`** contiene la cola shell bash como string constante (el heredoc actual portado verbatim). `ccp completion-shellinit` lo imprime sin cambios.

---

## 3. Contrato congelado — detalle de equivalencia

El punto fino es el quoting. Bash `printf '%q'` quotea de forma específica (p.ej. `$'...'` para caracteres de control, escapes de espacios/comillas). Go debe producir algo que zsh **y** bash evalúen a la misma cadena.

- Implementar `core.shellQuote(s string) string` que replique `%q` de bash para el rango de valores reales (paths con espacios, base_url, tokens, modelos). No usar `strconv.Quote` (semántica Go ≠ shell).
- Verificación: para cada fixture, `eval` la salida de Go en un zsh y un bash reales y comparar las variables resultantes contra las del bash oráculo (no solo comparar el texto — comparar el **efecto** del eval).
- Orden de emisión idéntico: primero `unset <todas las managed vars>`, luego los `export` del perfil objetivo. Mantener el orden de `CCP_MANAGED_VARS`.

---

## 4. Modelo de datos — `ccp.yaml`

Fuente de verdad única. Reemplaza `profiles.tsv` + `rules.tsv` + `config` + los `meta` + el `authored.tsv` global/profile. Secretos **fuera**.

### 4.1 Schema canónico

```yaml
version: 2                      # entero. Si el binario conoce < version -> ABORTA (no escribe).

defaults:                       # ex-archivo `config`. SOLO siembra perfiles nuevos;
  base_url: https://api.deepseek.com/anthropic   # editar esto NO muta perfiles existentes.
  model_pro: deepseek-chat
  model_flash: deepseek-chat
  effort: high
  editor: nano                  # "" => usa $EDITOR -> nano

profiles:                       # mapa name -> perfil. 'default' es IMPLÍCITO (nunca aquí).
  work:
    type: official              # official: solo type. Login vive en cc-home/.claude.json.
  personal:
    type: official
  deepseek:                     # literal 'deepseek' es load-bearing (env.go switchea sobre él).
    type: deepseek
    base_url: https://api.deepseek.com/anthropic  # los 4 campos van EXPLÍCITOS (no heredan).
    model_pro: deepseek-chat
    model_flash: deepseek-chat
    effort: high
    # api_key NO va aquí -> profiles/deepseek/api_key (chmod 600). Presencia = implícita.

rules:                          # lista de {path, profile}. path absoluto normalizado, ÚNICO.
  - path: /Users/me/work        # 'path set' reemplaza si ya existe. deepest-wins en runtime.
    profile: work
  - path: /Users/me/labs
    profile: deepseek

authored:                       # ex-authored.tsv (SOLO global+profile, machine-local).
  - scope: global               # scope: global | profile
    type: rule                  # rule|agent|command|skill|hook|mcp
    ref: "líneas o nombre de entrada"
    desc: "descripción"
  - scope: profile
    profile: work               # presente solo cuando scope=profile
    type: hook
    ref: PreToolUse
    desc: "..."
```

### 4.2 Reglas del schema

- **Versión:** entero monótono. Binario con `version` conocida **menor** a la del archivo → **aborta con error**, nunca escribe (no trunca data de un binario más nuevo).
- **Claves desconocidas:** se **preservan** en round-trip (catch-all `inline` en el struct) — un ccp viejo no borra campos de uno nuevo.
- **Comentarios:** preservados vía `CommentMap` de goccy (decode captura comentarios por-path; encode los re-adjunta). Permite editar `ccp.yaml` a mano con comentarios que sobreviven a writes de ccp.
- **Escritura atómica:** `ccp.yaml.tmp` + `rename`.
- **`default` implícito:** el perfil reservado nunca se serializa.
- **Inheritance: ninguna.** Perfiles deepseek guardan sus 4 campos completos; `defaults` solo aplica al crear.

### 4.3 Lo que NO entra al YAML (queda igual en disco)

- `cc-home/`, `overlay/` — estructuras nativas de Claude Code.
- `profiles/<n>/api_key` (chmod 600) y credenciales OAuth en `cc-home/.claude.json` — secretos.
- `install-source` — puntero al repo para `upgrade`.
- `.claude/ccp-authored.tsv` (authored **project**) — versionado per-repo; ccp **no reescribe archivos del repo del usuario** (cero churn de git). Asimetría a propósito vs el authored global/profile.

---

## 5. Migración de datos (migrador universal)

`internal/core/migrate.go`, disparado en el primer arranque Go cuando detecta estado legacy.

```
detectar estado:
  ~/.config/dsctl existe && ~/.config/ccp no  -> migrar dsctl -> ccp (TSV)  -> seguir
  ~/.config/ccp con *.tsv / meta (sin ccp.yaml) -> migrar TSV/meta -> ccp.yaml
  ccp.yaml ya existe -> no-op
antes de tocar nada:
  copiar estado legacy -> ~/.config/ccp/.backup-pre-go-<YYYYMMDD-HHMMSS>/
migrar:
  leer profiles.tsv + cada meta -> profiles map
  leer rules.tsv -> rules list
  leer config -> defaults
  leer authored.tsv -> authored
  escribir ccp.yaml (atómico)
  NO borrar api_key ni cc-home (intactos)
post:
  dejar los .tsv/meta legacy en su sitio O moverlos al backup (idempotencia: el lector legacy solo corre si no hay ccp.yaml)
```

- Atómico e idempotente. Re-correr no duplica.
- La cadena `dsctl→ccp→YAML` permite que un usuario muy viejo salte directo a Go sin pasar por bash ccp (decisión 13).
- Tras migrar, el bash viejo no lee YAML — puerta de una vía, recuperable vía el backup (ver ADR 0007).

---

## 6. Backup / restore

**Export** — `ccp backup export [archivo.tar.gz] [--with-secrets]`

- Contenido `.tar.gz` (preserva permisos 600):
  - `manifest.yaml`.
  - `ccp.yaml`.
  - `overlay/` de cada perfil (CLAUDE.md, settings.overlay.json) — siempre.
  - Si `--with-secrets`: `api_key` de cada deepseek + credenciales del `cc-home` de cada official.
- **Excluye** los symlinks re-seedables (plugins/commands/agents/skills) — se reconstruyen al restaurar.
- Sin `--with-secrets` el archivo es seguro de compartir/versionar. Con secretos: `chmod 600` + warning explícito.

`manifest.yaml`:
```yaml
ccp_version: 2.0.0
schema_version: 2               # la de ccp.yaml; restore aborta si es mayor a la conocida
created: 2026-06-02T12:00:00Z   # timestamp pasado por el caller (no Date.now en lógica pura)
with_secrets: false
profiles:
  - {name: work, type: official}
  - {name: deepseek, type: deepseek}
checksums:                      # sha256 POR archivo del payload
  ccp.yaml: "sha256:..."
  profiles/work/overlay/CLAUDE.md: "sha256:..."
```

- **Checksum por miembro:** restore verifica cada archivo antes de aplicar y reporta cuál falló (corrupción/tampering parcial). El manifest no se auto-incluye; se valida que el tar abra y el manifest parsee.
- **Restore cross-version:** si `schema_version` > la conocida por el binario → **aborta** con guía (espeja la política de lectura de `ccp.yaml`). Igual o menor se migra como en el arranque normal.

**Restore** — `ccp backup restore archivo.tar.gz [--overwrite | --force]`

```
verificar checksum del manifest
auto-snapshot del estado actual -> .backup-pre-restore-<fecha>/   (reversible)
para cada perfil en el backup:
  si NO existe -> crear (re-seed cc-home symlinks)
  si existe:
    default            -> skip + reportar
    --overwrite        -> reemplazar ese perfil
    --force            -> (a nivel global) wipe + restore total
reglas de path: merge; las del backup que no chocan se añaden
secretos (si el backup es full) -> escribir con chmod 600
reportar: creados / skipped / overwritten
```

- Nunca se pierde data sin querer: el snapshot previo permite deshacer.

---

## 7. TUI (bubbletea + huh)

`ccp` sin args **y con TTY** lanza la TUI. Sin TTY (pipe/script) imprime ayuda/estado (no bloquea scripting).

- **3 paneles** navegables (tab para cambiar foco, `j/k` dentro):
  - **Perfiles**: lista; `enter` → detalle; acciones vía formularios huh: añadir (official/deepseek con sus campos), editar config (abre `$EDITOR`), login (official), borrar (confirm huh), set key (deepseek).
  - **Reglas**: lista de path rules; añadir (huh: path + select de perfil), borrar (confirm).
  - **Estado**: perfil del `cwd`, perfil activo de la terminal, repo. Recomputa **al ganar foco** o con tecla `r`; **sin ticker** (cero trabajo en idle).
- **Barra de comandos** (`:` o teclas) para: backup export/restore (huh wizard), doctor, sync, install.
- **Formularios huh embebidos** como `tea.Model` subvista: el dashboard cede el foco al form y lo recupera al completar/cancelar (sin salir ni limpiar pantalla).
- **Acciones destructivas** (borrar perfil, `path clear`, `--force` restore): `huh.Confirm` en TUI; en CLI confirmación interactiva salvo `--yes`/`-y`.
- La TUI **solo llama `internal/core`**; cada acción tiene su subcomando CLI equivalente.
- Degradación: detectar `isatty`; `NO_COLOR` y ancho de terminal respetados.

---

## 8. Distribución, install y upgrade

**CI (GitHub Actions):** matriz de build `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`. Publica binarios + `checksums.txt` en GitHub Releases por tag.

**`install.sh` (Go-aware):**
```
detectar OS/arch
si hay release para esa plataforma -> descargar binario + verificar checksum -> ~/.local/bin/ccp
si no hay red/release pero hay toolchain Go -> go build ./cmd/ccp -> ~/.local/bin/ccp
limpiar libs viejas: rm -rf ~/.local/lib/ccp   (ya no hay libs bash)
registrar install-source (re-apuntar tras el rename del repo dsctl->ccp)
copiar commands/ccp/*.md a ~/.claude/commands/ccp  (igual que hoy)
```

> **Nota operativa:** en la máquina actual del usuario **Go no está instalado** → el fallback `go build` no aplica; el camino prebuilt-release es el efectivo. El binario debe poder instalarse sin toolchain Go.

**`ccp upgrade`:** el flujo de transición clave.
- El usuario aún en bash corre `ccp upgrade` → re-ejecuta el `install.sh` nuevo → instala el binario Go, borra libs bash.
- El bloque rc no cambia (sigue llamando `command ccp`), así que el siguiente `command ccp` ya es Go.
- La migración-a-YAML dispara en ese primer arranque Go.
- `ccp upgrade` corre `profile sync` con el binario nuevo (Go).
- Una vez en Go, `ccp upgrade` es Go y hace lo mismo (descarga release nuevo o `go build`).

---

## 9. Estrategia de tests

- **`go test` (core):** tablas de casos para `profile`, `rules` (deepest-wins, norm-path), `env` (EnvDelta por tipo de perfil + quoting), `cfg` (overlay merge), `store` (round-trip YAML), `backup` (export/restore + colisiones), `migrate` (dsctl→ccp→YAML, idempotencia).
- **Golden-diff de integración (gate de CI):** fixtures `testdata/golden/<caso>/CCP_HOME`. Para cada uno, correr el **binario bash (oráculo)** y el **binario Go** y exigir:
  - `_env <perfil>`, `_hook <path>`, `completion zsh`, `completion bash`, `completion-shellinit`, `status --json`, `resolve`, `path test`: salida idéntica.
  - Además, **efecto del eval**: `eval` la salida de cada uno en zsh y bash reales, comparar el set de variables resultante.
- `tests/run.sh` (harness bash actual) se mantiene mientras el bash es oráculo; se retira al lograr paridad.
- Lint: `go vet`, `golangci-lint`, `gofmt`. (shellcheck solo para `install.sh` + cola shell.)

---

## 10. Plan de ejecución por fases (tracer bullets)

Cada fase deja algo verificable. El bash permanece intacto y funcional hasta la Fase 7.

| Fase | Entregable | Verificación |
|------|-----------|--------------|
| 0 | Scaffold módulo Go, CI build matrix, fixtures golden capturados del bash | `go build` OK; fixtures generados |
| 1 | `core.store` (lee TSV/meta legacy + lee/escribe ccp.yaml) + `core.migrate` | `go test` migración idempotente |
| 2 | `core.profile` + `core.rules` + `core.env` (EnvDelta + shellQuote) | golden-diff `_env`/`_hook`/`resolve` = bash |
| 3 | `core.cfg` (overlay merge) + `core.secrets` + `shellinit` | golden-diff `completion*`/`status --json` = bash |
| 4 | CLI dispatch completo (todos los subcomandos), paridad de superficie | golden-diff de toda la superficie = bash; eval real en zsh+bash |
| 5 | `core.backup` export/restore + subcomandos | `go test` round-trip + colisiones |
| 6 | TUI bubbletea+huh (3 paneles + wizards) | smoke manual; acciones = subcomandos |
| 7 | `install.sh` Go-aware + `ccp upgrade` + release pipeline; archivar bash en `legacy/` | upgrade real bash→Go en VM limpia; migración YAML respaldada |

---

## 11. Riesgos

| Riesgo | Mitigación |
|--------|-----------|
| Quoting Go ≠ bash `%q` → eval roto/inyección | `shellQuote` dedicado + test de **efecto del eval** en zsh y bash reales, no solo diff de texto |
| Migración YAML corrompe data (puerta de una vía) | Backup pre-migración atómico + idempotencia + golden-diff sobre estado migrado; rollback documentado |
| Acoplar TUI (mucho código) al corte de datos en un solo v2.0 | TUI solo llama al core ya probado (Fases 1–5 verdes antes de Fase 6); la TUI no toca el contrato |
| Credenciales OAuth no portables entre máquinas en restore full | Documentar que restore full restaura sesión solo en la misma máquina/cuenta; config-restore + re-login como camino portable |
| Usuario en dsctl salta directo a Go | Migrador universal en cadena (dsctl→ccp→YAML) |
| rc viejo incompatible | El rc solo llama `command ccp`; Go emite shell-init idéntico → no requiere reinstall; `_upgrade_check_rc` avisa si difiere |

---

## 12. Criterios de aceptación (v2.0)

1. Golden-diff verde: salida y **efecto del eval** de toda la superficie observable idénticos al bash, en zsh y bash.
2. `ccp upgrade` desde una instalación bash real deja el binario Go, migra a `ccp.yaml` con backup, y `cd`/`claude` siguen funcionando sin reinstalar el rc.
3. Migración idempotente y reversible (el `.backup-pre-go-*` permite volver a bash).
4. `ccp backup export`/`restore` round-trip completo (config y full), restore no-destructivo con snapshot previo.
5. TUI lanza con `ccp` (TTY), cubre CRUD de perfiles/reglas + backup/doctor, y no rompe el modo no-interactivo.
6. `install.sh` instala por release (con fallback `go build`) en darwin/linux × arm64/amd64.
7. Secretos nunca en `ccp.yaml`; backups sin `--with-secrets` libres de secretos.

---

---

## 13. Decisiones de implementación (cabos atados)

Resueltas tras el schema, antes de codear. Todas con el bash como oráculo del contrato.

| Tema | Decisión | Por qué |
|------|----------|---------|
| **shellQuote** | Replicar `printf %q` de bash a mano; gate **byte-idéntico** + eval-equivalencia en zsh+bash de respaldo | Libs como `mvdan.cc/sh` quotean con otro estilo (comillas simples) → romperían el diff byte-a-byte. Es el riesgo nº1. |
| **Dispatch CLI** | A mano (sin cobra); emitir la completion bash/zsh **verbatim** del bash actual | La completion generada por cobra difiere del texto que el rc espera → rompería el golden-diff. Cero deps de framework. |
| **Merge JSON** | Reimplementar el deep-merge `. * $x` de jq en **Go puro**, sin jq | Elimina la dependencia de jq (y el fallback snapshot). Mismo merge para overlay settings y para hooks/mcp de `instruct`. Debe replicar: objetos merge recursivo, arrays/escalares se reemplazan. |
| **Locking ccp.yaml** | `flock` advisory en cada write (además de tmp+rename) | Ahora es un solo archivo → más contención que los `meta` per-perfil. Evita lost-update entre terminales. |
| **Backup checksum** | `sha256` **por miembro** listado en el manifest; restore verifica c/u | Detecta corrupción/tampering parcial y reporta qué archivo falló. |
| **Restore cross-version** | Aborta si `schema_version` del backup > la conocida | Espeja la política de lectura de `ccp.yaml`; no restaura a medias formatos futuros. |
| **Forms TUI** | `huh.Form` como subvista `tea.Model` embebida; foco vuelve al panel | Mantiene el contexto de los 3 paneles; sin salir ni limpiar pantalla. |
| **Confirmaciones** | Siempre en destructivas (TUI `huh.Confirm`; CLI interactivo salvo `--yes`) | Red de seguridad para `rm` de perfil / `--force` restore / `path clear`. |
| **Refresh Estado** | On-focus + tecla `r`; sin ticker | El estado casi nunca cambia solo; cero polling en idle. |
| **Naming** | Module `github.com/JoseAFlores777/ccp`; repo se **renombra** dsctl→ccp; binario `ccp`; Go **1.23** | Alinea repo=binario=module. Coste: re-apuntar `install-source`, redirect de GitHub. |

### Cosas que el core Go debe portar (parity, sin re-grillar)

- **`_seed_cc_home`**: symlinks `plugins/ commands/ agents/ skills/` desde `~/.claude` (override `CCP_CLAUDE_SRC`).
- **`cfg`**: `cc-home/CLAUDE.md` = header + `@import` global + `@import` overlay; `settings.json` = merge global ⊕ overlay; migración legacy (settings copiado→overlay, CLAUDE.md symlink→@import).
- **`instruct`**: 6 tipos de artefacto a estructura nativa Claude Code; bloque con marcadores `<!-- >>> ccp instructions >>> -->`; merge JSON para hook/mcp; manifest (global/profile→ccp.yaml, project→`.claude/ccp-authored.tsv`).
- **`migrate` dsctl**: `DS_*`→defaults, `include`→deepseek, `exclude`→default, backup `rules.dsctl.bak`.
- **Helpers de salida** `ok/warn/err/info/say/hr` respetando `NO_COLOR`/no-TTY; strings en **español**.
- **Exit codes**: `resolve`/`path test` 0=no-default, 1=default; `status --json` con `active/profile/profile_type/cwd/repo`.
- **macOS/BSD**: nada de coreutils GNU-only; Go es portable, pero `install.sh` sigue siendo POSIX sh.

---

*v0.2 — 2026-06-02*
