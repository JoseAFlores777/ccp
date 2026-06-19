# ccp handoff — continuar una sesión en otro perfil sin perder contexto

**Versión** v0.1 · **Fecha** 2026-06-19 · **Status** brainstorming cerrado · listo para plan · **Scope** comando `handoff` + back-sync con marcador

## 01 · Overview

Cuando trabajas en una sesión de Claude Code con un perfil `ccp` y ese perfil se queda sin tokens/cuota, hoy no hay forma limpia de seguir con otro perfil **sin perder el contexto** de la conversación. La restricción dura: un proceso `claude` vivo congeló sus credenciales al arrancar; `ccp use` solo reescribe el env del shell y un proceso vivo nunca lo re-lee. No existe hot-swap de credenciales.

`ccp handoff` automatiza el único camino real: **persistir el contexto → cambiar de perfil → reanudar la misma conversación en un proceso nuevo**, con tokens frescos del perfil destino. El contexto vive en disco como un transcript JSONL; el comando lo copia al `cc-home` del perfil destino, lanza `claude --resume`, y deja un marcador para el viaje de vuelta. Al terminar, `ccp handoff end` trae el contexto actualizado de regreso al perfil origen como una sesión nueva (no destructiva) y reanuda ahí.

El modelo mental: **pides prestados los tokens de otro perfil para una sesión, y devuelves el trabajo al volver.**

## 02 · Goals / Non-goals

### Goals

- Continuar una conversación existente bajo otro perfil sin perder contexto, con tokens del perfil destino.
- Flujo interactivo (TUI): elegir perfil destino (excluyendo el activo) → elegir sesión a continuar (orden nuevo→viejo, con fecha/título/uuid) → confirmar marcador.
- Equivalente CLI por flags (`ccp handoff <to> --session <uuid>`) para no-TTY/scripting (regla de arquitectura ccp).
- Marcador de referencia en el perfil origen para recordar el handoff en vuelo.
- `ccp handoff end`: back-sync del contexto actualizado destino→origen como **sesión nueva** (uuid nuevo), no destructivo, y relanzar en origen.
- Visibilidad del origen: la sesión de vuelta muestra `[de <perfil>]` en su título, tanto en la TUI de ccp como en el menú nativo `claude --resume`.
- No confundir a Claude Code: la sesión copiada/reescrita reanuda limpia (validado contra el formato en disco, ver §08).

### Non-goals

- **No** hot-swap de credenciales en un proceso vivo (imposible; el feature evita prometerlo).
- **No** handoff encadenado multi-nivel (`personal→work→cliente-x`): v1 permite **1 nivel** y bloquea un segundo handoff hasta hacer `end`. Cadena real = v2.
- **No** copiar los blobs de `file-history/` (rewind/restore de versiones de archivos): la conversación reanuda perfecta, solo ese feature degrada. Copia opcional = v2.
- **No** soportar handoff entre máquinas o cwd distintos: asume misma máquina, mismo repo.
- **No** auto-detección de "volvieron los tokens" para auto-`end`: no hay señal confiable; `end` es manual.
- **No** funcionar en cloud/headless sin ccp: el feature es para máquina local con ccp instalado, shell function activa y TTY (el equivalente por flags existe pero la cara primaria es la TUI).

## 03 · Arquitectura y superficie de comandos

Encaja en el split central de ccp **sin cambiarlo**: lógica en el binario, mutación de env/launch en la shell function emitida por `core.WriteShellInit`.

### Comandos de cara al usuario

```
ccp handoff                          # TUI: pick perfil destino → pick sesión → confirma marcador
ccp handoff <to>                     # salta el picker de perfil
ccp handoff <to> --session <uuid>    # salta ambos pickers (scriptable)
ccp handoff <to> --no-marker         # forward sin marcador (sin end automático; warning)
ccp handoff <to> --force             # fuerza sobre colisión de uuid con contenido distinto
ccp handoff end                      # back-sync destino→origen, relanza en origen
ccp handoff status                   # muestra el handoff activo (o "sin handoff activo")
ccp handoff list                     # historial (archivados) + activo
```

### Comandos internos (emiten env, estilo `_env`/`_hook`)

```
command ccp _handoff <to> <pwd>      # forward: copia + escribe marcador + emite env(destino) + CCP_RESUME_ID
command ccp _handoff-end <pwd>       # return: back-sync + archiva marcador + emite env(origen) + CCP_RESUME_ID
```

### El split en la shell function

```bash
handoff)
  case "$2" in
    end)          out=$(command ccp _handoff-end "$PWD") || return ;;
    status|list)  command ccp handoff "$2"; return ;;        # solo lectura, sin env
    *)            out=$(command ccp _handoff "${2:-}" "$PWD") || return ;;
  esac
  ( eval "$out"; claude --resume "$CCP_RESUME_ID" )   # subshell → env efímero, como `run`
  ;;
```

El `eval` corre en subshell para que el env del destino **no persista** en el shell del usuario tras salir de `claude` (decisión: efímero, como `run`). El binario hace toda la lógica (pickers TUI, copia, reescritura, marcador); solo emite el env delta + `CCP_RESUME_ID`. La shell es el único lugar que puede lanzar `claude` con ese env.

### Componentes nuevos

- `internal/core/handoff.go` — copia forward, reescritura del jsonl (sessionId + aiTitle), marcador CRUD, listado de sesiones, resolución de origen.
- `internal/tui` — gana dos pickers (perfil, sesión). Llaman solo a funciones de `core` ya testeadas; la TUI queda fina.
- `internal/cli` — dispatch de `handoff` / `_handoff` / `_handoff-end` + formato text/JSON.
- `core/shellinit.go` — el `case handoff)` se añade al string byte-idéntico (ver §08, gate de paridad).

## 04 · Modelo de datos — el marcador

Estado de **runtime**, no config. Archivo nuevo `~/.config/ccp/handoffs.yaml`, escritura atómica tmp+rename bajo `flock` (mismo patrón que `store.go`). **No** se mezcla con `ccp.yaml` (config declarativa versionada): mezclarlos ensuciaría el diff de config y arriesgaría el `version: 2` del store. Si `handoffs.yaml` se corrompe o se borra, no rompe perfiles ni reglas — solo se pierde el rastro del round-trip (degradación suave; los jsonl siguen en disco).

```yaml
version: 1
active:                              # 0 o 1 entradas (regla "1 nivel v1")
  session: bbc1ed61-ada1-408f-...    # uuid copiado (mismo en origen y destino)
  slug: -Volumes-...-dsctl-v2        # proyecto (cwd → guiones)
  cwd: /Volumes/.../dsctl-v2         # ruta absoluta, para validar
  from: personal-cc                  # perfil padre (de donde saliste)
  to: emco-cc                        # perfil destino (tokens prestados)
  title: "Refactor handoff en ccp"   # aiTitle al momento del forward (display)
  since: 2026-06-19T14:30:00Z
archived:                            # historial (para `handoff list`)
  - session: 9c2e0d4f-...
    from: personal-cc
    to: emco-cc
    returned_as: a1b2f0d3-...        # uuid nuevo creado en el back-sync
    since: 2026-06-18T10:00:00Z
    ended: 2026-06-18T15:20:00Z
```

**Reglas:**
- `active` presente → hay handoff en vuelo. Un segundo `ccp handoff <to>` → error 1-nivel.
- `ccp handoff end` → toma `active`, back-sync, mueve a `archived` con `returned_as`+`ended`, deja `active` vacío.
- `status` imprime `active`; `list` imprime `archived` + activo.
- **Migración:** ninguna. Archivo nuevo, nace en `version: 1`. No toca el migrador encadenado.

## 05 · Flujo forward (`ccp handoff`)

1. **Pre-chequeos**: ¿hay `active`? → error 1-nivel. Origen = perfil activo (de `CLAUDE_CONFIG_DIR` o `Resolve(cwd)`); `default` permitido (cc-home = `~/.claude`).
2. **Picker de perfil destino** (TUI) o `<to>` por flag: lista perfiles **menos el activo**. destino==origen o inexistente → error.
3. **Picker de sesión** (TUI) o `--session <uuid>`: escanea `origen-cc-home/projects/<slug>/*.jsonl`; lee `mtime`, `aiTitle`, uuid; ordena **nuevo→viejo**; muestra `fecha · título · uuid`. Carpeta vacía → error.
4. **Confirmar marcador** (`[Y/n]` TUI; `--no-marker` para saltar): default sí.
5. **Copia forward** (mismo uuid, copia cruda): `mkdir -p destino/projects/<slug>/`; copia `<uuid>.jsonl`. `sessionId` interno ya == uuid, `cwd` mismo repo → **sin reescritura**. Colisión mismo-contenido → overwrite+warn; distinto → error sin `--force`.
6. **Warning cross-provider**: si origen/destino difieren en tipo o modelo → aviso no-bloqueante.
7. **Escribe marcador `active` + emite env**: env delta del destino (`EnvDelta`) + `CCP_RESUME_ID=<uuid>` a stdout.
8. **Shell**: `eval "$out"` (subshell efímero) + `claude --resume $CCP_RESUME_ID`.

**Orden transaccional:** copia (5) → marcador (7) → emit env. Si la copia falla, no hay marcador huérfano. Si el marcador falla, no se emite env → no se lanza claude → estado consistente (jsonl copiado pero no marcado; recuperable).

## 06 · Flujo return (`ccp handoff end`) y reescritura del jsonl

1. **Pre-chequeos**: ¿hay `active`? No → error. `cwd` ≠ `active.cwd` → warning (permite). ¿Existe `destino/projects/<slug>/<uuid>.jsonl`? No → error (marcador queda activo, no se pierde rastro).
2. **Genera uuid nuevo `N`** con `crypto/rand` (UUID v4).
3. **Reescritura** (lee `destino/.../<uuid_viejo>.jsonl` línea por línea, JSONL):
   - campo `sessionId` → reemplaza `uuid_viejo` por `N` (todas las líneas).
   - `type:"ai-title"` → prepende `[de <to>] ` al `aiTitle` (idempotente: no duplica si ya lo tiene).
   - **NO toca:** `cwd`, árbol `uuid`/`parentUuid`/`leafUuid`, `messageId`, timestamps, contenido de mensajes.
   - escribe en `origen/.../N.jsonl`.
4. **No-destructivo**: el `<uuid_viejo>.jsonl` original en origen (congelado en el punto del handoff) queda intacto; el de destino (que creció) tampoco se borra (solo se lee). Origen termina con dos sesiones: la vieja + `N` con todo el contexto.
5. **Validación**: re-parsea `N.jsonl`; assert cero `sessionId` con `uuid_viejo` y JSONL válido. Falla → borra `N.jsonl`, no archiva marcador, error (estado = pre-`end`).
6. **Archiva marcador** (`active` → `archived` con `returned_as=N`, `ended`) **+ emite env** del origen + `CCP_RESUME_ID=N`.
7. **Shell**: `eval` (env origen) + `claude --resume N`. Vuelves con el contexto completo (lo de antes + lo hecho en destino).

## 07 · Manejo de errores y casos borde

Principio: toda operación es **transaccional o no pasa nada**.

| Caso | Comportamiento |
|---|---|
| Handoff activo, intentas otro | Error 1-nivel: termina el activo con `ccp handoff end`. |
| `end` sin handoff activo | Error: sin handoff activo. |
| destino == origen | Error no-op. |
| destino no existe como perfil | Error + lista perfiles válidos. |
| `--session <uuid>` inexistente | Error + lista sesiones del cwd. |
| Carpeta `projects/` vacía en origen | Error: sin sesiones para este proyecto. |
| Colisión uuid en destino, mismo contenido | Overwrite + warning. |
| Colisión uuid en destino, contenido distinto | Error, pide `--force`. |
| Cross-provider (Anthropic→DeepSeek) | Warning no-bloqueante. |
| jsonl de destino borrado antes de `end` | Error; marcador queda activo. |
| Reescritura produce JSONL inválido | Borra `N.jsonl`, no archiva marcador, error (estado pre-`end`). |
| `cwd` en `end` ≠ marcador | Warning, permite. |
| `handoffs.yaml` corrupto/ausente | Degradación suave: `status`/`list` "sin datos"; forward arranca limpio. |
| Usuario nunca corre `end` | Marcador sigue activo; `status` lo recuerda; aviso opcional en el hook autocheck (v1 opcional). |
| `default` como origen o destino | Permitido (cc-home = `~/.claude`). |
| Concurrencia (dos handoff a la vez) | `flock` serializa; el segundo ve el marcador → error 1-nivel. |

**Mensajes:** bilingües (ES default), vía `present.go`, respetan `NO_COLOR`/no-TTY.

**Exit codes** (superficie máquina-legible, estable): forward/end OK → 0; error de pre-chequeo → 1; error de I/O (copia/reescritura) → 2; `handoff status` → 0 si hay activo, 1 si no (paridad con `resolve`).

## 08 · Formato en disco de Claude Code (validado)

Investigado en disco (CC v2.1.183). La sesión vive en `<CLAUDE_CONFIG_DIR>/projects/<slug>/<uuid>.jsonl`, slug = cwd con cada `/` → `-`. Una línea JSON por evento.

**Identidad de sesión (mantener consistente al copiar):**
- El **uuid del nombre de archivo** ES el id para `claude --resume <uuid>`.
- El campo `sessionId` (en ~90% de líneas) debe igualar el uuid del archivo. Copiar como sesión nueva exige reescribir cada `sessionId` (no es `cp` crudo).
- El campo `cwd` (ruta absoluta) → misma máquina/repo, sin cambios.
- Título = campo `aiTitle` en líneas `type:"ai-title"` (esta versión no usa `type:"summary"`).
- Árbol de mensajes `uuid`/`parentUuid`/`leafUuid` (líneas `last-prompt` guardan el leaf de reanudación) — interno, intacto.

**NO es identidad de sesión — no tocar, sin riesgo de confusión:**
- `.claude.json` — llaveado por cwd(proyecto), solo guarda `lastSessionId`+métricas, **no enumera** sesiones; resume-by-uuid escanea la carpeta directo, así que un jsonl nuevo aparece solo.
- `session-env/<sid>/` — dir scratch vacío, **sin credenciales** (cero leak).
- `sessions/<pid>.json` — registro de proceso vivo por PID, efímero (CC lo recrea).

**Degradación residual:** `file-history/` (rewind/restore) llaveado aparte y **no se copia** → la conversación reanuda perfecta, solo el rewind de archivos puede no hallar blobs en el destino. Documentado, aceptable; copia opcional en v2.

## 09 · Estrategia de pruebas

El feature es **nuevo** (no está en el contrato bash congelado), pero la **shell function** gana el `case handoff)`, y ese texto vive byte-idéntico en `core/shellinit.go` ⟷ oráculo bash.

**Gates existentes (obligatorio):**
- Añadir `handoff` a la shell function → actualizar el oráculo bash en `legacy/` + regenerar golden con `testdata/golden/capture.sh` + mantener `parity_test.go` verde.
- `_handoff`/`_handoff-end` emiten env → se suman al fixture golden como comandos de scripting con salida estable.
- `gofmt`/`vet`/`golangci-lint` limpios.

**Unitarios (`internal/core/handoff_test.go`):**
- **Reescritura jsonl** (corazón): sobre un fixture jsonl real, assertar (a) `sessionId` viejo→`N` en todas las líneas, cero residuos; (b) `aiTitle` prepende `[de <to>]` e idempotente; (c) `cwd`/árbol/`messageId`/timestamps/contenido intactos; (d) salida JSONL válida.
- **Marcador CRUD**: write→read→archive; `active` único; escritura atómica bajo `flock`.
- **Copia forward**: mismo uuid; colisión mismo-contenido vs distinto.
- **Listado de sesiones**: orden nuevo→viejo, extracción de `aiTitle`, carpeta vacía → error.
- **Resolución de origen**: desde `CLAUDE_CONFIG_DIR` y desde `Resolve(cwd)`.

**Casos borde**: uno por fila de §07.

**Eval-effect (zsh + bash)**, como el test de `env.go`: `eval "$(ccp _handoff ...)"` exporta `CLAUDE_CONFIG_DIR`(destino)+`CCP_RESUME_ID`; `_handoff-end` exporta el del origen + `N`.

**E2E binario** (temp `CCP_HOME`, dir existente para no disparar auto-migración): 2 perfiles con cc-homes + fixture jsonl en origen; forward → assert copiado + marcador + env emitido; end → assert `N.jsonl` reescrito, original intacto, marcador archivado, env emitido. **Nunca lanza `claude`** (el binario solo emite env; lanzar es de la shell).

**TUI**: la lógica vive en `core`; se testea el core, no bubbletea.

## 10 · Resumen del ciclo

```
personal-cc (sin tokens)                    emco-cc (tokens frescos)
   bbc1ed61.jsonl                                │
   │ ccp handoff → pick emco-cc → pick bbc1ed61  │
   │ marcador active: personal-cc → emco-cc      │
   ├──── copia bbc1ed61 (mismo uuid) ───────────►│ claude --resume bbc1ed61, +N msgs
   │                                              │
   │ ccp handoff end                              │
   │◄─── reescribe sessionId→a1b2, aiTitle ───────┤ (lee bbc1ed61 actualizado)
   │     escribe a1b2.jsonl "[de emco-cc] ..."    │
   bbc1ed61 intacto · a1b2 = contexto completo    │
   marcador → archived (returned_as a1b2)         │
   claude --resume a1b2  (ya en personal-cc)      │
```

---

v0.1 — 2026-06-19
