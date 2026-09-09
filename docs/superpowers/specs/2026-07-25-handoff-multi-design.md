# ccp handoff multi — varios handoffs en vuelo, sin equivocarse

**Versión** v0.1 · **Fecha** 2026-07-25 · **Status** implementado · revisado tras la implementación (2026-07-25, ver pie) · **Scope** `handoffs.yaml` v2 (lista de activos) + resolución por cwd + `handoff resume` + panel TUI + `--dangerously-skip-permissions`

Extiende la spec base [`2026-06-19-ccp-handoff-design.md`](2026-06-19-ccp-handoff-design.md). Todo lo que esa spec define y esta no contradice sigue vigente: formato en disco de Claude Code (§08), reescritura del jsonl en `end` (§06), copia forward no destructiva (§05), split binario/shell function.

## 01 · Problema

v1 permite **un** handoff en vuelo: `HandoffForward` falla con «ya hay un handoff activo» y `handoffs.yaml` guarda `active` como un marcador único. En uso real hay varias sesiones prestadas a la vez — dsctl-v2 hacia `work-1`, ibc-tools hacia `kimi`, overtime de vuelta hacia `personal-1`. Hoy eso obliga a cerrar uno para abrir otro, y cerrar el equivocado significa traer de vuelta contexto que no querías mover.

El feature resuelve dos cosas a la vez: **permitir N activos** y **que ninguna operación actúe sobre el handoff equivocado**.

## 02 · Goals / Non-goals

### Goals

- N handoffs activos simultáneos, cada uno una sesión distinta.
- Toda operación destructiva-ish (`end`) resuelve su objetivo por `cwd` y, si empata, pregunta. Nunca adivina entre dos.
- `ccp handoff` sin argumentos abre un panel gestor: ves los activos antes de actuar.
- Re-entrar a un handoff en vuelo (`handoff resume`) sin cerrarlo ni copiar nada.
- Lanzar cualquiera de los tres flujos con `--dangerously-skip-permissions`.
- Aviso al entrar a un repo con handoff activo, para que no se olviden abiertos.
- Migración transparente del `handoffs.yaml` v1 existente.

### Non-goals

- **No** cadena multi-nivel (`personal-1→work-1→kimi` sobre la misma sesión): sigue bloqueada, con mensaje propio.
- **No** fan-out: una sesión no se presta a dos perfiles a la vez.
- **No** tope duro de activos: solo aviso.
- **No** persistir el modo skip-permissions en el marcador ni en `ccp.yaml`: se pide en cada lanzamiento.
- **No** cambia nada del formato en disco de Claude Code ni de la reescritura del jsonl.

## 03 · Modelo de datos — `handoffs.yaml` v2

`active` pasa de marcador único a lista. `archived` no cambia. Sigue siendo estado de runtime, fuera de `ccp.yaml`, escritura atómica tmp+rename bajo `flock`.

```yaml
version: 2
active:
  - session: bbc1ed61-ada1-408f-...
    slug: -Volumes-...-dsctl-v2
    cwd: /Volumes/.../dsctl-v2
    from: personal-1
    to: work-1
    title: "Refactor handoff en ccp"
    since: 2026-07-25T14:30:00Z
  - session: 9c2e0d4f-...
    slug: -Volumes-...-ibc-tools
    cwd: /Volumes/.../ibc-tools
    from: personal-1
    to: kimi
    title: "Himnos API"
    since: 2026-07-19T09:10:00Z
archived:
  - session: 7f10c2ab-...
    from: personal-1
    to: work-1
    slug: -Volumes-...-overtime
    returned_as: a1b2f0d3-...
    since: 2026-07-18T10:00:00Z
    ended: 2026-07-18T15:20:00Z
```

El struct `Marker` no cambia de campos; cambia el contenedor:

```go
type Handoffs struct {
    Version  int              `yaml:"version"`
    Active   []Marker         `yaml:"active,omitempty"`
    Archived []ArchivedMarker `yaml:"archived,omitempty"`
}
```

### Migración v1 → v2

`LoadHandoffs` decodifica primero en v2 (`active` como secuencia). Si el unmarshal falla porque `active` es un mapping (v1), reintenta con un struct auxiliar `{Active *Marker}` y eleva ese marcador a lista de uno. Se persiste `version: 2` en el siguiente `SaveHandoffs`; no hay paso de migración explícito ni backup (el archivo es runtime, reconstruible).

Degradación suave intacta: archivo ausente o ilegible bajo ambos esquemas ⇒ `Handoffs{Version: 2}` vacío, sin error.

## 04 · Resolución por cwd — el núcleo del "sin equivocarse"

Función nueva pura en `core`:

```go
// ResolveActive elige el marcador objetivo entre los activos.
// sessionFlag != "" => exacto por uuid. Si no, filtra por slug(cwd).
func ResolveActive(h *Handoffs, cwd, sessionFlag string) (idx int, candidates []Marker, err error)
```

| Situación | Resultado |
|---|---|
| `--session <uuid>` y está en `active[]` | ese marcador |
| `--session <uuid>` y no está | error; lista los uuid activos |
| Exactamente 1 activo con `slug(cwd)` | ese, sin preguntar (comportamiento idéntico a v1) |
| 0 activos con `slug(cwd)`, hay otros | error: «sin handoff activo para este proyecto» + lista de los otros con su repo |
| 0 activos en total | error: «no hay ningún handoff activo» — sin nombrar operación, porque `ResolveActive` la comparten `end`, `resume` y `discard` |
| 2+ activos con `slug(cwd)` | devuelve `candidates`; la capa CLI abre el picker TUI |
| 2+ candidatos y no hay TTY | error pidiendo `--session <uuid>` |

`end` y `resume` comparten esta resolución. Devolver `candidates` en vez de decidir mantiene `core` sin I/O de presentación: la TUI vive en `internal/tui`, invocada desde `internal/cli`.

## 05 · Superficie de comandos

```
ccp handoff                                    # panel gestor TUI
ccp handoff <to> [--session <uuid>] [--yolo] [--no-marker] [--force]
ccp handoff end     [<uuid>] [--yolo]
ccp handoff resume  [<uuid>] [--yolo]          # re-entra a uno en vuelo
ccp handoff discard [<uuid>]                   # suelta un marcador, sin back-sync
ccp handoff status  [--all]
ccp handoff list
```

`--dangerously-skip-permissions` es la forma larga de `--yolo`; ambas aceptadas en `handoff`, `end` y `resume`.

Internos (emiten env, estilo `_env`/`_hook`):

```
command ccp _handoff        <pwd> [to] [flags]
command ccp _handoff-end    <pwd> [uuid] [flags]
command ccp _handoff-resume <pwd> [uuid] [flags]     # NUEVO
```

`discard` **no** necesita interno propio: no emite env, así que la función shell lo
enruta como `status`/`list` (`command ccp handoff "$@"`, sin `eval` ni lanzamiento de
`claude`). Sí necesita el `<pwd>` para el desambiguado por cwd; lo toma del proceso.

### `handoff resume`

Reanuda un handoff **sin cerrarlo**: no copia transcripts, no escribe ni archiva marcador. Solo resuelve el marcador (§04), valida que el jsonl siga existiendo en `to`, y emite `EnvDelta(to) + CCP_RESUME_ID=<session>`. Es el mecanismo para saltar entre varios handoffs vivos; sin él, tener N activos no serviría de nada porque solo podrías volver a uno cerrándolo.

### `handoff discard`

Suelta un marcador activo **sin back-sync**: lo saca de `active[]` y lo archiva con
`returned_as` vacío. No copia ni reescribe transcripts, no toca el jsonl del destino
(lo que hubiera sigue en disco y se puede reanudar a mano con `claude --resume <uuid>`
desde ese perfil) y no cambia el perfil de la shell — a diferencia de `end`/`resume`,
`discard` **no emite env ni lanza `claude`**: archiva, informa y devuelve el prompt.

Resuelve su objetivo con **exactamente el mismo desambiguado por cwd de §04** que
`end`/`resume`: uuid explícito (posicional o `--session`) → exacto; si no, activos con
`slug(cwd)`: uno → ese, sin preguntar; varios → picker TUI (sin TTY, error pidiendo
`--session <uuid>`); ninguno → error que lista los otros repos.

El caso de uso es el **marcador zombi**: si el jsonl del destino desapareció (limpieza
de `~/.claude`, rotación, perfil destino borrado), `end` y `resume` fallan siempre — no
encuentran la sesión que reescribir/reanudar — y el marcador se queda activo para
siempre. Un marcador así no es inocuo: secuestra la resolución por cwd de ese repo
(cualquier `end`/`resume` allí resuelve a él o se vuelve ambiguo), cuenta para el aviso
de ≥5 activos y, desde el perfil destino, bloquea el forward por la invariante de
cadena. Sin `discard` la única salida es editar `handoffs.yaml` a mano; por eso los
mensajes de error de `end`/`resume` lo nombran («si el transcript ya no existe,
descártalo»).

Exit codes: **0** si archivó el marcador; **1** en cualquier fallo de pre-chequeo
(sin activo para este cwd, uuid inexistente en `active[]`, ambigüedad sin TTY);
**2** I/O al persistir `handoffs.yaml`. Mismo esquema que forward/end/resume.

### `handoff status` / `list`

- `status` (sin flags): los activos con `slug(cwd)`. Exit **0** si hay ≥1 para este cwd, **1** si no — conserva la paridad con `resolve` que ya documentaba la spec v1.
- `status --all`: todos los activos, agrupados por repo. Exit 0 si existe cualquiera.
- `list`: activos (marcando cuáles son de este repo) + archivados, como hoy.

Exit codes de forward/end/resume sin cambios: 0 OK, 1 pre-chequeo, 2 I/O.

## 06 · Invariantes del forward

| Invariante | Comportamiento |
|---|---|
| La sesión elegida ya está en `active[]` | Error: «esa sesión ya está en vuelo: `<from>` → `<to>` (hace 2h)». El picker de sesiones **la muestra marcada** `⟳ en vuelo → <perfil>` y no la deja elegir. `huh` no sabe deshabilitar una opción concreta, así que "no dejar elegir" se implementa rechazando la elección (`checkSessionFree`, cableado como `Validate` del select y revalidado al salir del form) con un error que nombra el perfil que la tiene y los dos remedios (`resume` / `end`). Filtrarla de la lista **no** vale: el usuario no vería por qué falta su sesión, que es justo lo que la marca existe para explicar. |
| Cadena multi-nivel | Bloqueo **por sesión, no por repo**: solo hay error si el perfil origen actual es el `to` de un activo **y la sesión elegida es exactamente la sesión de ese activo** (`marker.Session == session`) → «handoff encadenado no soportado; haz `end` primero». Que el repo esté prestado a este perfil **no** basta: prestar otra sesión distinta del mismo repo (p. ej. una nacida ya en el perfil destino) hacia un tercer perfil es legal y no debe bloquearse — el activo existente no es una cadena de esa sesión. Comparar por `slug`/`cwd` en vez de por `session` convertiría la invariante en «un handoff por repo», que es justo lo que este feature elimina. |
| `to == from` / perfil inexistente | Error, como en v1. |
| ≥5 activos | **Aviso no bloqueante** al terminar el forward: «5 handoffs sin cerrar (más viejo: `<repo>`, hace 6 días); revísalos con `ccp handoff list`». El umbral es una constante en `core`, no configuración. |
| Cross-provider | Warning no bloqueante (v1, sin cambios). |
| Concurrencia | `flock` serializa; el segundo proceso relee la lista antes de añadir, así que no se pierde un marcador por carrera. |

La regla «un handoff activo global» **desaparece**; el resto de la tabla de errores de la spec v1 §07 sigue vigente.

## 07 · `--dangerously-skip-permissions`

El binario añade al emit una línea más:

```
CCP_RESUME_YOLO=1        # con el flag
unset CCP_RESUME_YOLO    # sin él (evita heredar el modo de un lanzamiento previo)
```

La shell function ramifica en vez de interpolar args — así no hay word-splitting (zsh no divide `$var` sin comillas; bash sí: interpolar sería un bug de portabilidad):

```bash
    handoff)
      shift
      case "$1" in
        end)          shift; out=$(command ccp _handoff-end "$PWD" "$@") || return ;;
        resume)       shift; out=$(command ccp _handoff-resume "$PWD" "$@") || return ;;
        status|list)  command ccp handoff "$@"; return ;;
        *)            out=$(command ccp _handoff "$PWD" "$@") || return ;;
      esac
      ( eval "$out"
        if [[ -n "${CCP_RESUME_YOLO:-}" ]]; then
          claude --resume "$CCP_RESUME_ID" --dangerously-skip-permissions
        else
          claude --resume "$CCP_RESUME_ID"
        fi ) ;;
```

El `eval` sigue en subshell ⇒ ni el env del destino ni `CCP_RESUME_YOLO` sobreviven al salir de `claude`.

**No se persiste**: el modo no va al marcador ni a `ccp.yaml`. Un `end` hecho semanas después no debe heredar salto de permisos de un forward que ya nadie recuerda; se pide explícito cada vez, en el comando o con el toggle de la TUI.

## 08 · TUI — panel gestor

`ccp handoff` sin argumentos (con TTY) abre `internal/tui/handoff_panel.go`:

```
┌ ccp handoff ─────────────────────────────────────────────┐
│ ACTIVOS (3)                        skip-permissions: off │
│ ▸ dsctl-v2   personal-1 → work-1         2h   "Refactor…"│  ← este repo
│   ibc-tools  personal-1 → kimi           6d   "Himnos…"  │
│   overtime   work-1 → personal-1        20m   "Cierre…"  │
│                                                          │
│ enter reanudar · e terminar · n nuevo · y yolo · q salir │
└──────────────────────────────────────────────────────────┘
```

- Los marcadores de **este** proyecto van primero y marcados; el resto abajo. Qué cuenta como "este proyecto" lo decide `core.ActiveForCwd`, **no** una comparación de slugs propia del panel: `SlugForCwd` colisiona (`/repo/foo-bar` y `/repo/foo/bar` comparten slug), y reimplementar el criterio hacía que el panel pusiera bajo el cursor un handoff que `ccp handoff status` ni lista.
- `enter` → `resume` del seleccionado · `e` → `end` del seleccionado · `n` → wizard existente (perfil → sesión → confirmar marcador) · `y` → toggle del modo skip-permissions, visible en el header.
- `e` **no** actúa con una sola pulsación: abre una pantalla de confirmación que repite el marcador afectado. Solo `y`/`s` confirman; cualquier otra tecla cancela y vuelve a la lista. `end` reescribe el transcript en el origen y archiva el marcador, y la tecla está pegada a `j`/`k`.
- Sin activos el panel **ni se abre**: `RunHandoffPanel` devuelve "nuevo" antes de tocar `/dev/tty`, así que el flujo de estreno entra directo al wizard (nada de una pantalla vacía que exija una tecla, donde una `q` haría fallar el comando).
- El panel decide la acción y el **binario emite el env correspondiente por el mismo `_handoff`**; la shell function no necesita saber qué eligió el usuario. Por eso `_handoff` puede emitir un delta de forward, de resume o de end.
- Sin TTY, `ccp handoff` sin argumentos sigue siendo error shell-only con la ayuda de flags.
- El panel solo llama a `core`; la lógica testeable (resolución, invariantes, listados) no vive en bubbletea.

## 09 · Aviso en el hook

`_hook <pwd>` añade, cuando hay ≥1 activo con `slug(pwd)`:

```
echo "↳ ccp: handoff activo aquí — personal-1 → work-1 (hace 2h); \`ccp handoff end\` para volver" >&2
```

Va a stderr dentro del emit (core nunca escribe a `os.Stderr` directo, misma convención que los warnings de `HandoffEnd`). `_ccp_autocheck` ya cachea por `$PWD`, así que aparece una vez por `cd`, no en cada prompt. Con 2+ activos en el repo, una línea que dice cuántos y remite a `ccp handoff`.

## 10 · Errores y casos borde nuevos

| Caso | Comportamiento |
|---|---|
| `end` con 2+ activos del mismo repo, con TTY | Picker; nada se toca hasta elegir. |
| `end` con 2+ activos del mismo repo, sin TTY | Error pidiendo `--session <uuid>`; ningún marcador se archiva. |
| `end`/`resume` con `--session` de un handoff **archivado** | Error: «ese handoff ya terminó (volvió como `<returned_as>`)». |
| `resume` y el jsonl ya no está en `to` | Error; el marcador **no** se toca (a diferencia de `end`, `resume` no muta estado). Si el transcript desapareció de verdad, la salida es `ccp handoff discard` (§05). |
| `end` y el jsonl ya no está en `to` | Error; el marcador queda activo (no se archiva a medias). Salida: `ccp handoff discard`. |
| `handoffs.yaml` v1 en disco | Se eleva a v2 en memoria; se persiste v2 al primer `Save`. |
| `handoffs.yaml` v2 con `version` mayor al que conoce el binario | **La operación aborta sin efectos secundarios.** La versión se valida **al inicio**, antes de copiar/reescribir nada: exit 1, mensaje «`handoffs.yaml` declara una versión más nueva que este ccp; actualiza ccp o mueve el archivo — no se tocará», y en disco no queda **ningún** transcript copiado, ningún marcador escrito, ningún archivo reescrito. |
| Dos `end` concurrentes sobre el mismo marcador | `flock` serializa; el segundo relee, no lo encuentra activo y falla limpio. |
| `--yolo` en un perfil `default` | Permitido; el flag es del lanzamiento de `claude`, no del perfil. |

### Por qué abortar y no degradar suave ante una `version` futura

v0.1 de esta spec decía lo contrario («degradación suave a vacío; **no** aborta como
`store.go` — es runtime, no config»). Se cambió tras la implementación, y esta es la
razón: degradar suave significa leer el archivo como `Handoffs{Version: 2}` vacío y, en
la primera escritura, **reescribirlo** — es decir, un ccp viejo borraría los activos que
un ccp más nuevo tiene en vuelo, con sus transcripts ya copiados en los perfiles
destino. Reconstruir eso a mano exige saber qué sesión estaba en qué perfil, que es
exactamente el dato que el archivo destruido guardaba. El estado sea runtime no lo hace
desechable: es reconstruible **por el usuario**, no por el binario.

Abortar es la elección conservadora — el fallo es ruidoso, reversible y con una salida
obvia (actualizar ccp, o mover el archivo si ya no importa), mientras que la
degradación es silenciosa e irreversible. Misma política que `store.go` para `ccp.yaml`,
por consistencia: ningún ccp destruye el estado de un ccp más nuevo.

La contrapartida es dónde se valida. La validación vive **al inicio de cada operación**
que vaya a tocar `handoffs.yaml`, no solo dentro de la escritura: si la comprobación
solo estuviera en `writeHandoffs`, un forward fallaría **después** de haber copiado el
jsonl al perfil destino, dejando una sesión huérfana sin marcador que la referencie —
visible para siempre en el picker de sesiones de ese perfil y en su `claude --resume`.
La regla es: **si la operación va a abortar por versión, aborta antes del primer efecto
secundario.**

## 11 · Impacto en los gates

**Contrato congelado.** La shell function cambia (`resume`, rama yolo) ⇒ obligatorio:

1. Actualizar el oráculo bash `legacy/bin/ccp` con el mismo texto.
2. Regenerar `testdata/golden/` con `bash testdata/golden/capture.sh`.
3. `internal/golden/parity_test.go` verde (`completion-shellinit`, `completion-bash|zsh` incluyen el bloque).

Los fixtures de `_hook` no tienen `handoffs.yaml` ⇒ no emiten el aviso ⇒ esos golden no cambian. Si se añade un fixture con handoffs activos, va como caso nuevo, no como edición de los existentes.

**Tests nuevos:**

- `handoff_store_test.go` — lista con N marcadores, round-trip, migración v1→v2 (fixture con `active` mapping), archivo con `version` futura.
- `handoff_test.go` — `ResolveActive` con una prueba por fila de §04; invariante sesión-en-vuelo; invariante cadena; aviso ≥5; `HandoffResume` (emite env correcto, no muta `handoffs.yaml`); `HandoffEnd` sobre lista (archiva solo el elegido, deja los demás intactos).
- `handoff_eval_test.go` — eval-effect en **bash y zsh**: `CCP_RESUME_YOLO=1` presente con `--yolo`, y `unset` sin él.
- `internal/cli/handoff_test.go` — parsing de `--dangerously-skip-permissions`/`--yolo` en los tres subcomandos; `status` exit 0/1 por cwd; `status --all`; sin TTY y ambiguo → error con `--session`.
- E2E binario con `CCP_HOME` temporal (dir existente, para no disparar auto-migración): 3 perfiles, 2 forwards a repos distintos, `resume` del primero, `end` del segundo, assert que el primero sigue activo.
- `gofmt` / `go vet` / `golangci-lint` limpios; strings nuevos en `i18n` (ES default + EN).

## 12 · Archivos tocados

| Archivo | Cambio |
|---|---|
| `internal/core/handoff_store.go` | `Active []Marker`, `version: 2`, migración v1→v2 en `LoadHandoffs` |
| `internal/core/handoff.go` | `ResolveActive`, `HandoffResume`, `HandoffDiscard`, invariantes de forward, aviso ≥5, `end` sobre lista |
| `internal/core/shellinit.go` | rama `resume`, `discard` enrutado junto a `status`/`list` (sin `eval`), rama yolo (byte-idéntico al oráculo) |
| `internal/cli/handoff.go` | dispatch de `resume`/`_handoff-resume` y de `discard`, flags yolo, `status --all`, picker de desambiguación |
| `internal/cli/cli.go` | `case "_handoff-resume"` |
| `internal/tui/handoff_panel.go` | nuevo panel gestor |
| `internal/tui/handoff.go` | picker de sesiones marca `⟳ en vuelo`; picker de marcadores para `end`/`resume` |
| `internal/core/i18n/catalog_cli.go` | strings nuevos ES/EN |
| `legacy/bin/ccp` | shell function actualizada (oráculo) |
| `testdata/golden/basic/expected/*` | regenerados con `capture.sh` |
| `commands/ccp/*.md`, `README*.md` | documentar `resume`, `--yolo`, multi-activo |

---

**Revisada tras la implementación — 2026-07-25.** Dos revisiones adversariales sobre el
código ya escrito obligaron a corregir la spec, no solo el código. Cambios de esta
revisión:

- **§05** — se añade `ccp handoff discard [<uuid>]` a la superficie (existía en `core`
  como `HandoffDiscard` sin ningún caller): archiva un marcador sin back-sync, con el
  mismo desambiguado por cwd que `end`/`resume`. Es la salida del marcador zombi.
- **§06** — la invariante de cadena multi-nivel se explicita como bloqueo **por sesión**
  (solo si la sesión elegida es la del activo), nunca por repo.
- **§10** — la `version` futura de `handoffs.yaml` **aborta la operación sin efectos
  secundarios** (validación al inicio) en vez de degradar suave, con el porqué
  documentado: un ccp viejo no debe destruir el estado de uno nuevo.

v0.1 — 2026-07-25 · rev. 2026-07-25 (post-implementación)
