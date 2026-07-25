# Handoff multi-activo — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Commits:** este repo prohíbe commits autónomos. Los pasos `git commit` de este plan **solo se ejecutan si el usuario lo autoriza explícitamente en ese turno**. Si no hay autorización, deja los cambios en el working tree y reporta qué quedó modificado. Nunca añadas trailers `Co-Authored-By`.

**Goal:** Permitir N handoffs de sesión en vuelo a la vez en `ccp`, resolviendo por `cwd` (con picker cuando empata) para que ninguna operación actúe sobre el handoff equivocado, más `handoff resume`, panel TUI gestor y `--dangerously-skip-permissions`.

**Architecture:** `~/.config/ccp/handoffs.yaml` sube a `version: 2` con `active` como **lista** de marcadores (migración transparente desde el mapping de v1). Una función pura nueva `ResolveActive(h, cwd, sessionFlag)` decide sobre cuál marcador actúan `end` y `resume`; devuelve candidatos en vez de preguntar, para que la desambiguación viva en `internal/cli`/`internal/tui` y `core` siga sin I/O de presentación. Las tres operaciones (`forward`, `end`, `resume`) emiten `EnvDelta(perfil) + CCP_RESUME_ID + (CCP_RESUME_YOLO|unset)`, y la función shell ramifica sobre `CCP_RESUME_YOLO` para añadir `--dangerously-skip-permissions` al `claude --resume`.

**Tech Stack:** Go 1.22+, `github.com/goccy/go-yaml`, bubbletea + huh + lipgloss (TUI), `golang.org/x/sys/unix` (flock). Gates: `go test ./...`, `gofmt -l`, `go vet`, `golangci-lint run`, `bash legacy/tests/run.sh`, `bash testdata/golden/capture.sh --check`.

**Spec:** `docs/superpowers/specs/2026-07-25-handoff-multi-design.md`

---

## Contexto que el implementador necesita antes de empezar

Lee, en este orden:

1. `CLAUDE.md` (raíz) — el split binario / función shell, y por qué el texto de la shell function es contrato congelado.
2. `docs/superpowers/specs/2026-06-19-ccp-handoff-design.md` — la spec v1 del feature (formato en disco de Claude Code, reescritura del jsonl).
3. `docs/superpowers/specs/2026-07-25-handoff-multi-design.md` — la spec de este plan.
4. `internal/core/handoff.go`, `internal/core/handoff_store.go`, `internal/core/transcript.go` — el código que se modifica.

Reglas del repo que aplican a **todas** las tareas:

- **Nunca** lanzar `claude` desde Go. El binario solo **emite** texto eval-able; la función shell es la única que ejecuta.
- `core` no escribe a `os.Stdout`/`os.Stderr`: los warnings viajan **dentro** del emit como `echo "…" >&2`.
- Los tests que ejecutan el binario usan un `CCP_HOME` temporal **que ya existe** (`t.TempDir()`), para no disparar la auto-migración contra `~/.config/dsctl` real.
- Strings de cara al usuario: español por defecto, con su par inglés en `internal/core/i18n/catalog_cli.go`.
- Tras cada tarea: `gofmt -l internal cmd` (debe imprimir nada), `go vet ./...`, `go test ./...`.

Comando de test único usado en todo el plan:

```bash
go test ./internal/core/ -run '<TestName>' -v
```

---

## File Structure

| Archivo | Responsabilidad | Acción |
|---|---|---|
| `internal/core/handoff_store.go` | Persistencia de `handoffs.yaml`: struct, load con migración v1→v2, save atómico | Modificar |
| `internal/core/handoff_resolve.go` | **Nuevo.** Selección pura del marcador objetivo (`ResolveActive`, `ActiveForCwd`, `FindActiveSession`) | Crear |
| `internal/core/handoff.go` | Orquestación de forward / end / resume + emit | Modificar |
| `internal/core/shellinit.go` | Texto byte-idéntico de la función shell (rama `resume`, rama yolo) | Modificar |
| `internal/cli/handoff.go` | Dispatch legible + parsing de flags + desambiguación vía TUI | Modificar |
| `internal/cli/cli.go` | `case "_handoff-resume"` | Modificar |
| `internal/cli/env.go` | `cmdHook`: aviso de handoff activo en el repo | Modificar |
| `internal/tui/handoff.go` | Pickers: marcador (desambiguar) + sesiones marcando las que están en vuelo | Modificar |
| `internal/tui/handoff_panel.go` | **Nuevo.** Panel gestor bubbletea de `ccp handoff` sin argumentos | Crear |
| `internal/core/i18n/catalog_cli.go` | Strings ES/EN nuevos | Modificar |
| `legacy/bin/ccp` | Oráculo bash: misma función shell | Modificar |
| `testdata/golden/basic/expected/*` | Golden regenerado desde el oráculo | Regenerar |
| `internal/core/handoff_store_test.go`, `handoff_test.go`, `handoff_eval_test.go`, `handoff_resolve_test.go` (nuevo), `internal/cli/handoff_test.go` | Tests | Modificar / Crear |
| `commands/ccp/handoff.md`, `README.md`, `README.es.md` | Documentación de usuario | Modificar |

**Decisión de decomposición:** `ResolveActive` va a un archivo propio (`handoff_resolve.go`) y no a `handoff.go`, porque es pura (sin I/O, sin `Config`) y es la pieza con más casos borde: aislada se testea con tablas y `handoff.go` no crece.

---

### Task 1: `handoffs.yaml` v2 — `active` como lista + migración v1→v2

**Files:**
- Modify: `internal/core/handoff_store.go`
- Test: `internal/core/handoff_store_test.go`

- [ ] **Step 1: Escribe los tests que fallan**

Reemplaza el contenido completo de `internal/core/handoff_store_test.go` por:

```go
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHandoffsRoundTrip(t *testing.T) {
	home := t.TempDir()
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if h.Version != HandoffsVersion || len(h.Active) != 0 || len(h.Archived) != 0 {
		t.Fatalf("vacío esperado, got %+v", h)
	}
	h.Active = []Marker{
		{Session: "abc", Slug: "-r", Cwd: "/r", From: "personal-cc", To: "emco-cc", Title: "T", Since: "2026-07-25T00:00:00Z"},
		{Session: "def", Slug: "-s", Cwd: "/s", From: "personal-cc", To: "kimi", Title: "U", Since: "2026-07-25T01:00:00Z"},
	}
	if err := SaveHandoffs(home, h); err != nil {
		t.Fatal(err)
	}
	h2, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h2.Active) != 2 {
		t.Fatalf("esperaba 2 activos, got %+v", h2.Active)
	}
	if h2.Active[0].To != "emco-cc" || h2.Active[1].To != "kimi" {
		t.Fatalf("no round-tripeó en orden: %+v", h2.Active)
	}
	if h2.Version != HandoffsVersion {
		t.Fatalf("version esperada %d, got %d", HandoffsVersion, h2.Version)
	}
}

func TestHandoffsMissingIsEmpty(t *testing.T) {
	h, err := LoadHandoffs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 0 {
		t.Fatal("archivo ausente debe dar Active vacío")
	}
}

// v1 escribía `active:` como un mapping único. LoadHandoffs debe elevarlo a
// lista de uno sin perder el archivado.
func TestHandoffsMigratesV1Mapping(t *testing.T) {
	home := t.TempDir()
	v1 := `version: 1
active:
  session: bbc1ed61
  slug: -repo
  cwd: /repo
  from: personal-cc
  to: emco-cc
  title: Refactor
  since: 2026-06-19T14:30:00Z
archived:
  - session: 9c2e0d4f
    from: personal-cc
    to: emco-cc
    slug: -repo
    returned_as: a1b2f0d3
    since: 2026-06-18T10:00:00Z
    ended: 2026-06-18T15:20:00Z
`
	if err := os.WriteFile(filepath.Join(home, "handoffs.yaml"), []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 1 || h.Active[0].Session != "bbc1ed61" || h.Active[0].To != "emco-cc" {
		t.Fatalf("no elevó el marcador v1: %+v", h.Active)
	}
	if len(h.Archived) != 1 || h.Archived[0].ReturnedAs != "a1b2f0d3" {
		t.Fatalf("perdió el archivado: %+v", h.Archived)
	}
	if h.Version != HandoffsVersion {
		t.Fatalf("debe reportar v%d en memoria, got %d", HandoffsVersion, h.Version)
	}
	// Persistir deja el archivo ya en v2.
	if err := SaveHandoffs(home, h); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, "handoffs.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	h3, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h3.Active) != 1 {
		t.Fatalf("tras Save no round-tripea v2: %s", data)
	}
}

// Un handoffs.yaml de una versión futura degrada suave (runtime, no config).
func TestHandoffsFutureVersionDegrades(t *testing.T) {
	home := t.TempDir()
	future := "version: 99\nactive: []\n"
	if err := os.WriteFile(filepath.Join(home, "handoffs.yaml"), []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 0 || h.Version != HandoffsVersion {
		t.Fatalf("versión futura debe degradar a vacío v%d: %+v", HandoffsVersion, h)
	}
}
```

- [ ] **Step 2: Corre los tests y verifica que fallan**

Run: `go test ./internal/core/ -run 'TestHandoffs' -v`
Expected: FAIL de compilación — `undefined: HandoffsVersion` y `cannot use []Marker literal as *Marker`.

- [ ] **Step 3: Implementa el cambio de esquema**

En `internal/core/handoff_store.go`, sustituye el bloque del struct `Handoffs` y la función `LoadHandoffs` por:

```go
// HandoffsVersion es el esquema actual de handoffs.yaml. v1 guardaba `active`
// como un mapping único (un solo handoff en vuelo); v2 lo guarda como lista.
const HandoffsVersion = 2

// Handoffs es el modelo en memoria de handoffs.yaml.
type Handoffs struct {
	Version  int              `yaml:"version"`
	Active   []Marker         `yaml:"active,omitempty"`
	Archived []ArchivedMarker `yaml:"archived,omitempty"`
}

// handoffsV1 es el esquema viejo, solo para migrar al leer.
type handoffsV1 struct {
	Version  int              `yaml:"version"`
	Active   *Marker          `yaml:"active"`
	Archived []ArchivedMarker `yaml:"archived"`
}

// LoadHandoffs lee handoffs.yaml. Ausente, corrupto, o de una versión futura =>
// Handoffs vacío (sin error: degradación suave; el rastro se pierde pero los
// jsonl siguen en disco). Un archivo v1 se eleva a v2 en memoria; se persiste
// como v2 en el siguiente SaveHandoffs.
func LoadHandoffs(home string) (*Handoffs, error) {
	empty := &Handoffs{Version: HandoffsVersion}
	data, err := os.ReadFile(handoffsPath(home))
	if err != nil {
		return empty, nil
	}
	h := &Handoffs{}
	if err := yaml.Unmarshal(data, h); err == nil {
		if h.Version > HandoffsVersion {
			return empty, nil
		}
		h.Version = HandoffsVersion
		return h, nil
	}
	// `active` como mapping => esquema v1.
	old := &handoffsV1{}
	if err := yaml.Unmarshal(data, old); err != nil {
		return empty, nil
	}
	out := &Handoffs{Version: HandoffsVersion, Archived: old.Archived}
	if old.Active != nil {
		out.Active = []Marker{*old.Active}
	}
	return out, nil
}
```

En `SaveHandoffs`, cambia el default de versión:

```go
	if h.Version == 0 {
		h.Version = HandoffsVersion
	}
```

- [ ] **Step 4: Corre los tests y verifica que pasan**

Run: `go test ./internal/core/ -run 'TestHandoffs' -v`
Expected: PASS los 4. `go build ./...` fallará todavía en `handoff.go` (usa `h.Active` como puntero) — eso lo arregla la Task 3; para que este paso compile, aplica **solo** este parche mínimo temporal en `internal/core/handoff.go` (se reescribe en Task 3):

```go
	// en HandoffForward, sustituye el bloque de la regla 1-nivel:
	if len(h.Active) > 0 {
		return "", fmt.Errorf("ya hay un handoff activo (%s → %s); termínalo con `ccp handoff end`", h.Active[0].From, h.Active[0].To)
	}
	// ...
	// donde escribía el marcador:
	if writeMarker {
		h.Active = []Marker{{
			Session: sessionUUID, Slug: slug, Cwd: cwd,
			From: from, To: to,
			Title: readAITitle(srcPath),
			Since: now.UTC().Format(time.RFC3339),
		}}
		if err := SaveHandoffs(home, h); err != nil {
			return "", err
		}
	}
	// en HandoffEnd:
	if len(h.Active) == 0 {
		return "", fmt.Errorf("no hay handoff activo que terminar")
	}
	m := h.Active[0]
	// ... y al archivar:
	h.Active = nil
```

Ajusta también, en `internal/core/handoff_test.go`, las 3 líneas que construyen `&Handoffs{Version: 1, Active: &Marker{...}}` a `&Handoffs{Version: HandoffsVersion, Active: []Marker{{...}}}`, y los asserts `h.Active == nil || h.Active.To != …` a `len(h.Active) != 1 || h.Active[0].To != …` (líneas ~49, ~57, ~108, ~143).

Run: `go test ./internal/core/ -v` → PASS.

- [ ] **Step 5: Commit** (solo con autorización explícita del usuario en el turno)

```bash
git add internal/core/handoff_store.go internal/core/handoff_store_test.go internal/core/handoff.go internal/core/handoff_test.go
git commit -m "feat(handoff): handoffs.yaml v2 con lista de activos + migración v1"
```

---

### Task 2: `ResolveActive` — elegir el marcador correcto

**Files:**
- Create: `internal/core/handoff_resolve.go`
- Test: `internal/core/handoff_resolve_test.go`

- [ ] **Step 1: Escribe los tests que fallan**

Crea `internal/core/handoff_resolve_test.go`:

```go
package core

import (
	"errors"
	"strings"
	"testing"
)

func mk(session, cwd, from, to string) Marker {
	return Marker{Session: session, Slug: SlugForCwd(cwd), Cwd: cwd, From: from, To: to, Since: "2026-07-25T00:00:00Z"}
}

func TestResolveActiveByCwdUnico(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{
		mk("aaa", "/repo/uno", "personal-cc", "emco-cc"),
		mk("bbb", "/repo/dos", "personal-cc", "kimi"),
	}}
	idx, cands, err := ResolveActive(h, "/repo/uno", "")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if idx != 0 || len(cands) != 0 {
		t.Fatalf("esperaba idx 0 sin candidatos, got idx=%d cands=%v", idx, cands)
	}
}

func TestResolveActivePorSessionFlag(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{
		mk("aaa", "/repo/uno", "personal-cc", "emco-cc"),
		mk("bbb", "/repo/dos", "personal-cc", "kimi"),
	}}
	// El uuid gana aunque el cwd sea otro.
	idx, _, err := ResolveActive(h, "/repo/uno", "bbb")
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Fatalf("esperaba idx 1, got %d", idx)
	}
}

func TestResolveActiveSessionFlagDesconocida(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{mk("aaa", "/repo/uno", "personal-cc", "emco-cc")}}
	_, _, err := ResolveActive(h, "/repo/uno", "zzz")
	if err == nil {
		t.Fatal("esperaba error con uuid desconocido")
	}
	if !strings.Contains(err.Error(), "aaa") {
		t.Fatalf("el error debe listar los activos: %v", err)
	}
}

func TestResolveActiveSinActivosEnEsteRepo(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{mk("aaa", "/repo/uno", "personal-cc", "emco-cc")}}
	_, _, err := ResolveActive(h, "/otro/repo", "")
	if err == nil {
		t.Fatal("esperaba error: no hay activo para este cwd")
	}
	if !strings.Contains(err.Error(), "/repo/uno") {
		t.Fatalf("el error debe nombrar dónde sí hay activos: %v", err)
	}
}

func TestResolveActiveSinActivos(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion}
	_, _, err := ResolveActive(h, "/repo/uno", "")
	if err == nil {
		t.Fatal("esperaba error sin activos")
	}
	if !strings.Contains(err.Error(), "no hay handoff activo") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
}

func TestResolveActiveAmbiguo(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{
		mk("aaa", "/repo/uno", "personal-cc", "emco-cc"),
		mk("bbb", "/repo/uno", "personal-cc", "kimi"),
	}}
	idx, cands, err := ResolveActive(h, "/repo/uno", "")
	if !errors.Is(err, ErrAmbiguousHandoff) {
		t.Fatalf("esperaba ErrAmbiguousHandoff, got %v", err)
	}
	if idx != -1 || len(cands) != 2 {
		t.Fatalf("esperaba idx -1 y 2 candidatos, got idx=%d cands=%v", idx, cands)
	}
}

func TestFindActiveSession(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{mk("aaa", "/r", "p", "e")}}
	if FindActiveSession(h, "aaa") != 0 {
		t.Fatal("debe hallar la sesión activa")
	}
	if FindActiveSession(h, "zzz") != -1 {
		t.Fatal("uuid ausente debe dar -1")
	}
}

func TestActiveForCwd(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{
		mk("aaa", "/repo/uno", "p", "e"),
		mk("bbb", "/repo/dos", "p", "k"),
		mk("ccc", "/repo/uno", "p", "g"),
	}}
	got := ActiveForCwd(h, "/repo/uno")
	if len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("esperaba índices [0 2], got %v", got)
	}
}
```

- [ ] **Step 2: Corre los tests y verifica que fallan**

Run: `go test ./internal/core/ -run 'TestResolveActive|TestFindActiveSession|TestActiveForCwd' -v`
Expected: FAIL de compilación — `undefined: ResolveActive`, `undefined: ErrAmbiguousHandoff`, `undefined: ActiveForCwd`, `undefined: FindActiveSession`.

- [ ] **Step 3: Implementa `handoff_resolve.go`**

Crea `internal/core/handoff_resolve.go`:

```go
package core

import (
	"errors"
	"fmt"
	"strings"
)

// handoff_resolve.go — selección PURA del marcador sobre el que actúan `end` y
// `resume`. No hace I/O ni pregunta nada: cuando hay empate devuelve los
// candidatos y ErrAmbiguousHandoff, y la capa CLI decide (picker TUI con TTY,
// error pidiendo --session sin TTY). Ese reparto mantiene core sin presentación.

// ErrAmbiguousHandoff señala 2+ handoffs activos para el mismo cwd.
var ErrAmbiguousHandoff = errors.New("varios handoffs activos para este proyecto")

// ActiveForCwd devuelve los índices de h.Active cuyo slug coincide con cwd.
func ActiveForCwd(h *Handoffs, cwd string) []int {
	slug := SlugForCwd(cwd)
	var out []int
	for i, m := range h.Active {
		if m.Slug == slug {
			out = append(out, i)
		}
	}
	return out
}

// FindActiveSession devuelve el índice del activo con ese uuid de sesión, o -1.
func FindActiveSession(h *Handoffs, session string) int {
	for i, m := range h.Active {
		if m.Session == session {
			return i
		}
	}
	return -1
}

// ResolveActive elige el marcador objetivo.
//
//	sessionFlag != ""  => exacto por uuid (error si no está activo).
//	1 activo con slug(cwd) => ese.
//	0 con slug(cwd)        => error, nombrando dónde sí hay activos.
//	2+ con slug(cwd)       => (-1, candidatos, ErrAmbiguousHandoff).
func ResolveActive(h *Handoffs, cwd, sessionFlag string) (int, []Marker, error) {
	if sessionFlag != "" {
		if i := FindActiveSession(h, sessionFlag); i >= 0 {
			return i, nil, nil
		}
		return -1, nil, fmt.Errorf("no hay handoff activo con la sesión %s%s", sessionFlag, activeHint(h))
	}
	if len(h.Active) == 0 {
		return -1, nil, errors.New("no hay handoff activo que terminar")
	}
	idxs := ActiveForCwd(h, cwd)
	switch len(idxs) {
	case 1:
		return idxs[0], nil, nil
	case 0:
		return -1, nil, fmt.Errorf("sin handoff activo para este proyecto%s", activeHint(h))
	default:
		cands := make([]Marker, 0, len(idxs))
		for _, i := range idxs {
			cands = append(cands, h.Active[i])
		}
		return -1, cands, ErrAmbiguousHandoff
	}
}

// activeHint arma el sufijo "; activos: <cwd> <sesión> (from → to), …" que
// acompaña a los errores de resolución, para que el usuario vea qué sí hay.
func activeHint(h *Handoffs) string {
	if len(h.Active) == 0 {
		return ""
	}
	parts := make([]string, 0, len(h.Active))
	for _, m := range h.Active {
		parts = append(parts, fmt.Sprintf("%s %s (%s → %s)", m.Cwd, ShortUUID(m.Session), m.From, m.To))
	}
	return "; activos: " + strings.Join(parts, ", ")
}

// ShortUUID recorta un uuid a sus primeros 8 caracteres para display.
func ShortUUID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
```

- [ ] **Step 4: Corre los tests y verifica que pasan**

Run: `go test ./internal/core/ -run 'TestResolveActive|TestFindActiveSession|TestActiveForCwd' -v`
Expected: PASS los 8.

- [ ] **Step 5: Commit** (solo con autorización explícita)

```bash
git add internal/core/handoff_resolve.go internal/core/handoff_resolve_test.go
git commit -m "feat(handoff): ResolveActive elige marcador por cwd/uuid"
```

---

### Task 3: Forward multi-activo + invariantes + emit yolo

**Files:**
- Modify: `internal/core/handoff.go`
- Test: `internal/core/handoff_test.go`

Cambia la firma: `HandoffForward(home, from, to, cwd, sessionUUID string, writeMarker, yolo bool, now time.Time)`.

- [ ] **Step 1: Escribe los tests que fallan**

Sustituye `TestHandoffForwardBlocksWhenActive` (línea ~54 de `internal/core/handoff_test.go`) por estos tests, y añade los demás al final del archivo:

```go
// Con v2 ya NO se bloquea un segundo handoff: se bloquea repetir la MISMA sesión.
func TestHandoffForwardPermiteSegundoActivo(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwdA, cwdB := "/repo/uno", "/repo/dos"
	uuidA := "11111111-1111-4111-8111-111111111111"
	uuidB := "22222222-2222-4222-8222-222222222222"
	cc := home + "/profiles/personal-cc/cc-home"
	writeJSONL(t, ProjectDir(cc, SlugForCwd(cwdA)), uuidA, "A", time.Now())
	writeJSONL(t, ProjectDir(cc, SlugForCwd(cwdB)), uuidB, "B", time.Now())

	if _, err := HandoffForward(home, "personal-cc", "emco-cc", cwdA, uuidA, true, false, time.Now()); err != nil {
		t.Fatalf("primer forward: %v", err)
	}
	if _, err := HandoffForward(home, "personal-cc", "emco-cc", cwdB, uuidB, true, false, time.Now()); err != nil {
		t.Fatalf("segundo forward debe permitirse: %v", err)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 2 {
		t.Fatalf("esperaba 2 marcadores activos, got %+v", h.Active)
	}
}

func TestHandoffForwardBloqueaSesionEnVuelo(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	uuid := "33333333-3333-4333-8333-333333333333"
	writeJSONL(t, ProjectDir(home+"/profiles/personal-cc/cc-home", SlugForCwd(cwd)), uuid, "A", time.Now())

	if _, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, true, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, true, false, time.Now())
	if err == nil {
		t.Fatal("esperaba error: la sesión ya está en vuelo")
	}
	if !strings.Contains(err.Error(), "ya está en vuelo") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
}

func TestHandoffForwardBloqueaCadena(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	slug := SlugForCwd(cwd)
	uuidViejo := "44444444-4444-4444-8444-444444444444"
	uuidNuevo := "55555555-5555-4555-8555-555555555555"
	// Marcador activo personal-cc → emco-cc en este repo.
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: uuidViejo, Slug: slug, Cwd: cwd, From: "personal-cc", To: "emco-cc",
		Since: "2026-07-25T00:00:00Z",
	}}})
	// Estando en emco-cc (el destino), intentar prestar otra sesión del mismo repo.
	writeJSONL(t, ProjectDir(home+"/profiles/emco-cc/cc-home", slug), uuidNuevo, "N", time.Now())
	_, err := HandoffForward(home, "emco-cc", "personal-cc", cwd, uuidNuevo, true, false, time.Now())
	if err == nil {
		t.Fatal("esperaba error de cadena multi-nivel")
	}
	if !strings.Contains(err.Error(), "encadenado") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
}

func TestHandoffForwardAvisaMuchosActivos(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cc := home + "/profiles/personal-cc/cc-home"
	// 4 marcadores previos en repos distintos.
	var pre []Marker
	for i := 0; i < ActiveWarnThreshold-1; i++ {
		cwd := fmt.Sprintf("/repo/viejo%d", i)
		pre = append(pre, Marker{
			Session: fmt.Sprintf("old-%d", i), Slug: SlugForCwd(cwd), Cwd: cwd,
			From: "personal-cc", To: "emco-cc", Since: "2026-07-01T00:00:00Z",
		})
	}
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: pre})

	cwd := "/repo/nuevo"
	uuid := "66666666-6666-4666-8666-666666666666"
	writeJSONL(t, ProjectDir(cc, SlugForCwd(cwd)), uuid, "N", time.Now())
	emit, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, true, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emit, "handoffs sin cerrar") || !strings.Contains(emit, ">&2") {
		t.Fatalf("esperaba aviso de acumulación en el emit: %s", emit)
	}
}

func TestHandoffForwardEmiteYolo(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	uuid := "77777777-7777-4777-8777-777777777777"
	writeJSONL(t, ProjectDir(home+"/profiles/personal-cc/cc-home", SlugForCwd(cwd)), uuid, "A", time.Now())

	emit, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, false, true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emit, "CCP_RESUME_YOLO=1") {
		t.Fatalf("con yolo debe emitir CCP_RESUME_YOLO=1: %s", emit)
	}

	emit2, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, false, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emit2, "unset CCP_RESUME_YOLO") {
		t.Fatalf("sin yolo debe hacer unset: %s", emit2)
	}
}
```

Añade `"fmt"` a los imports de `handoff_test.go` si no está.

Actualiza además las llamadas ya existentes de `HandoffForward` en `handoff_test.go` (líneas ~31, ~66, ~74, ~218, ~238) y en `handoff_eval_test.go` (línea ~30) añadiendo el parámetro `yolo` en penúltima posición: `HandoffForward(home, from, to, cwd, uuid, true, false, time.Now())`.

- [ ] **Step 2: Corre los tests y verifica que fallan**

Run: `go test ./internal/core/ -run 'TestHandoffForward' -v`
Expected: FAIL de compilación — `too many arguments in call to HandoffForward` y `undefined: ActiveWarnThreshold`.

- [ ] **Step 3: Implementa**

En `internal/core/handoff.go`, añade tras los imports:

```go
// ActiveWarnThreshold es el número de handoffs sin cerrar a partir del cual el
// forward avisa (no bloquea: el usuario decide cuántos préstamos lleva vivos).
const ActiveWarnThreshold = 5

// resumeSuffix arma el par de líneas que la función shell necesita: el uuid a
// reanudar y el modo skip-permissions. El `unset` es deliberado: sin él, un
// lanzamiento previo con --yolo en el mismo shell contaminaría el siguiente.
func resumeSuffix(sessionID string, yolo bool) string {
	s := "CCP_RESUME_ID=" + shellQuote(sessionID) + "\n"
	if yolo {
		return s + "CCP_RESUME_YOLO=1\n"
	}
	return s + "unset CCP_RESUME_YOLO\n"
}

// warnLine formatea un echo a stderr eval-able. core nunca escribe a os.Stderr
// directo: los avisos viajan dentro del emit.
func warnLine(format string, args ...any) string {
	return "echo \"⚠️  ccp: " + fmt.Sprintf(format, args...) + "\" >&2\n"
}
```

Sustituye `HandoffForward` completa por:

```go
// HandoffForward valida, copia la sesión origen→destino (mismo uuid), añade el
// marcador a la lista de activos y devuelve el emit con el env del DESTINO.
// v2: se permiten N handoffs en vuelo; lo que se bloquea es repetir la MISMA
// sesión (fan-out) y encadenar niveles sobre el mismo repo.
func HandoffForward(home, from, to, cwd, sessionUUID string, writeMarker, yolo bool, now time.Time) (string, error) {
	if from == to {
		return "", fmt.Errorf("el perfil destino es el mismo que el origen (%s)", to)
	}
	cfg, err := Load(home)
	if err != nil {
		return "", err
	}
	if to != "default" {
		if _, ok := cfg.Profiles[to]; !ok {
			return "", fmt.Errorf("perfil destino desconocido: %s", to)
		}
	}
	if from != "default" {
		if _, ok := cfg.Profiles[from]; !ok {
			return "", fmt.Errorf("perfil origen desconocido: %s", from)
		}
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	slug := SlugForCwd(cwd)

	// Invariante 1: la sesión no puede estar ya prestada (sin fan-out).
	if i := FindActiveSession(h, sessionUUID); i >= 0 {
		a := h.Active[i]
		return "", fmt.Errorf("esa sesión ya está en vuelo: %s → %s (desde %s); termínala con `ccp handoff end` o elige otra", a.From, a.To, a.Since)
	}
	// Invariante 2: sin cadena multi-nivel. Si el perfil actual es el destino de
	// un handoff vivo en este mismo repo, re-prestar desde aquí encadenaría.
	for _, a := range h.Active {
		if a.To == from && a.Slug == slug {
			return "", fmt.Errorf("handoff encadenado no soportado: %s ya presta este proyecto a %s; haz `ccp handoff end` primero", a.From, a.To)
		}
	}

	fromCC, err := CCHome(home, from)
	if err != nil {
		return "", err
	}
	toCC, err := CCHome(home, to)
	if err != nil {
		return "", err
	}
	srcPath := ProjectDir(fromCC, slug) + "/" + sessionUUID + ".jsonl"
	if _, err := os.Stat(srcPath); err != nil {
		return "", fmt.Errorf("no encuentro la sesión %s en %s", sessionUUID, from)
	}
	if _, err := CopyTranscript(srcPath, ProjectDir(toCC, slug), false); err != nil {
		return "", err
	}

	if writeMarker {
		h.Active = append(h.Active, Marker{
			Session: sessionUUID, Slug: slug, Cwd: cwd,
			From: from, To: to,
			Title: readAITitle(srcPath),
			Since: now.UTC().Format(time.RFC3339),
		})
		if err := SaveHandoffs(home, h); err != nil {
			return "", err
		}
	}

	// CCP_RESUME_ID se emite SIEMPRE (incluso con writeMarker=false): la shell
	// function lo necesita para `claude --resume "$CCP_RESUME_ID"`.
	emit := EnvDelta(home, to, cfg) + resumeSuffix(sessionUUID, yolo)
	// Spec §handoff: si origen y destino son de proveedores distintos, advierte
	// pero no bloquea — misma semántica que el warning de cwd-mismatch en HandoffEnd.
	if profileKind(from, cfg) != profileKind(to, cfg) {
		emit = warnLine("handoff entre proveedores distintos (%s → %s); el modelo y el formato de tools pueden diferir", from, to) + emit
	}
	if writeMarker && len(h.Active) >= ActiveWarnThreshold {
		o := oldestActive(h)
		emit = warnLine("%d handoffs sin cerrar (más viejo: %s, desde %s); revísalos con \\`ccp handoff list\\`", len(h.Active), o.Cwd, o.Since) + emit
	}
	return emit, nil
}

// oldestActive devuelve el marcador activo con `since` más antiguo. Since es
// RFC3339 en UTC, así que el orden lexicográfico ES el cronológico.
func oldestActive(h *Handoffs) Marker {
	out := h.Active[0]
	for _, m := range h.Active[1:] {
		if m.Since < out.Since {
			out = m
		}
	}
	return out
}
```

- [ ] **Step 4: Corre los tests y verifica que pasan**

Run: `go test ./internal/core/ -run 'TestHandoffForward' -v`
Expected: PASS. `HandoffEnd` sigue compilando con `h.Active[0]` del parche de Task 1.

- [ ] **Step 5: Commit** (solo con autorización explícita)

```bash
git add internal/core/handoff.go internal/core/handoff_test.go internal/core/handoff_eval_test.go
git commit -m "feat(handoff): forward multi-activo con invariantes y modo yolo"
```

---

### Task 4: `HandoffEnd` sobre la lista

**Files:**
- Modify: `internal/core/handoff.go`
- Test: `internal/core/handoff_test.go`

Firma nueva: `HandoffEnd(home, cwd, sessionFlag string, yolo bool, now time.Time)`.

- [ ] **Step 1: Escribe los tests que fallan**

Actualiza las llamadas existentes en `handoff_test.go` (líneas ~98, ~152, ~166, ~179): `HandoffEnd(home, cwd, "", false, time.Now())`. En el subtest "mismatch advierte y reanuda" (línea ~152) el cwd no coincide con ningún marcador, así que ahora **hay que pasar el uuid**: `HandoffEnd(home, "/otro/repo", uuid, false, time.Now())` — captura el `uuid` que devuelve `setup(t)`.

Añade al final del archivo:

```go
func TestHandoffEndArchivaSoloElElegido(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwdA, cwdB := "/repo/uno", "/repo/dos"
	uuidA := "88888888-8888-4888-8888-888888888888"
	uuidB := "99999999-9999-4999-8999-999999999999"
	dst := home + "/profiles/emco-cc/cc-home"
	writeJSONL(t, ProjectDir(dst, SlugForCwd(cwdA)), uuidA, "A", time.Now())
	writeJSONL(t, ProjectDir(dst, SlugForCwd(cwdB)), uuidB, "B", time.Now())
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{
		{Session: uuidA, Slug: SlugForCwd(cwdA), Cwd: cwdA, From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z"},
		{Session: uuidB, Slug: SlugForCwd(cwdB), Cwd: cwdB, From: "personal-cc", To: "emco-cc", Since: "2026-07-25T01:00:00Z"},
	}})

	if _, err := HandoffEnd(home, cwdA, "", false, time.Now()); err != nil {
		t.Fatal(err)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 || h.Active[0].Session != uuidB {
		t.Fatalf("debía quedar solo el handoff de %s activo: %+v", cwdB, h.Active)
	}
	if len(h.Archived) != 1 || h.Archived[0].Session != uuidA {
		t.Fatalf("archivó el equivocado: %+v", h.Archived)
	}
}

func TestHandoffEndAmbiguoDevuelveError(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	slug := SlugForCwd(cwd)
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{
		{Session: "aaa", Slug: slug, Cwd: cwd, From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z"},
		{Session: "bbb", Slug: slug, Cwd: cwd, From: "personal-cc", To: "emco-cc", Since: "2026-07-25T01:00:00Z"},
	}})
	_, err := HandoffEnd(home, cwd, "", false, time.Now())
	if !errors.Is(err, ErrAmbiguousHandoff) {
		t.Fatalf("esperaba ErrAmbiguousHandoff, got %v", err)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 2 {
		t.Fatal("un end ambiguo no debe tocar el estado")
	}
}

func TestHandoffEndEmiteYolo(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	uuid := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	writeJSONL(t, ProjectDir(home+"/profiles/emco-cc/cc-home", SlugForCwd(cwd)), uuid, "A", time.Now())
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: uuid, Slug: SlugForCwd(cwd), Cwd: cwd, From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z",
	}}})
	emit, err := HandoffEnd(home, cwd, "", true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emit, "CCP_RESUME_YOLO=1") {
		t.Fatalf("end con yolo debe emitirlo: %s", emit)
	}
}
```

Añade `"errors"` a los imports de `handoff_test.go`.

- [ ] **Step 2: Corre los tests y verifica que fallan**

Run: `go test ./internal/core/ -run 'TestHandoffEnd' -v`
Expected: FAIL de compilación — `too many arguments in call to HandoffEnd`.

- [ ] **Step 3: Implementa**

Sustituye `HandoffEnd` completa en `internal/core/handoff.go` por:

```go
// HandoffEnd toma el marcador resuelto (por cwd o por uuid explícito), hace
// back-sync del transcript (que creció en el destino) hacia el origen como una
// sesión NUEVA (uuid nuevo, sessionId reescrito, aiTitle prefijado con el
// origen), lo archiva y devuelve el emit con el env del ORIGEN + el uuid nuevo.
// No destructivo: ni el original del origen ni el del destino se borran.
// Si hay 2+ activos para el cwd devuelve ErrAmbiguousHandoff SIN tocar nada;
// el caller desambigua y vuelve a llamar con sessionFlag.
func HandoffEnd(home, cwd, sessionFlag string, yolo bool, now time.Time) (string, error) {
	cfg, err := Load(home)
	if err != nil {
		return "", err
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	idx, _, err := ResolveActive(h, cwd, sessionFlag)
	if err != nil {
		return "", err
	}
	m := h.Active[idx]

	toCC, err := CCHome(home, m.To)
	if err != nil {
		return "", err
	}
	fromCC, err := CCHome(home, m.From)
	if err != nil {
		return "", err
	}
	srcPath := ProjectDir(toCC, m.Slug) + "/" + m.Session + ".jsonl"
	if _, err := os.Stat(srcPath); err != nil {
		return "", fmt.Errorf("no encuentro la sesión %s en %s; el marcador queda activo", m.Session, m.To)
	}
	newID, err := NewUUID()
	if err != nil {
		return "", err
	}
	dstPath := ProjectDir(fromCC, m.Slug) + "/" + newID + ".jsonl"
	if err := RewriteSession(srcPath, dstPath, m.Session, newID, m.To); err != nil {
		return "", err // RewriteSession ya validó; no se archiva el marcador
	}

	h.Archived = append(h.Archived, ArchivedMarker{
		Session: m.Session, From: m.From, To: m.To, Slug: m.Slug,
		ReturnedAs: newID, Since: m.Since, Ended: now.UTC().Format(time.RFC3339),
	})
	h.Active = append(h.Active[:idx], h.Active[idx+1:]...)
	if err := SaveHandoffs(home, h); err != nil {
		return "", err
	}
	emit := EnvDelta(home, m.From, cfg) + resumeSuffix(newID, yolo)
	// Spec §07: si el cwd actual difiere del del marcador, advierte pero permite.
	// Con resolución por cwd esto solo ocurre vía --session explícito.
	if cwd != "" && cwd != m.Cwd {
		emit = warnLine("el cwd actual difiere del marcador del handoff") + emit
	}
	return emit, nil
}
```

Nota: el warning de cwd-mismatch usaba texto sin el prefijo `⚠️  ccp:`; `warnLine` lo añade. El test existente busca el substring `"el cwd actual difiere"`, así que sigue pasando.

- [ ] **Step 4: Corre los tests y verifica que pasan**

Run: `go test ./internal/core/ -v`
Expected: PASS todo el paquete.

- [ ] **Step 5: Commit** (solo con autorización explícita)

```bash
git add internal/core/handoff.go internal/core/handoff_test.go
git commit -m "feat(handoff): end resuelve el marcador por cwd o uuid"
```

---

### Task 5: `HandoffResume` — re-entrar a un handoff vivo

**Files:**
- Modify: `internal/core/handoff.go`
- Test: `internal/core/handoff_test.go`

- [ ] **Step 1: Escribe los tests que fallan**

Añade a `internal/core/handoff_test.go`:

```go
func TestHandoffResumeEmiteDestinoSinMutar(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	uuid := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	writeJSONL(t, ProjectDir(home+"/profiles/emco-cc/cc-home", SlugForCwd(cwd)), uuid, "A", time.Now())
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: uuid, Slug: SlugForCwd(cwd), Cwd: cwd, From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z",
	}}})

	emit, err := HandoffResume(home, cwd, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emit, "emco-cc/cc-home") {
		t.Fatalf("resume debe emitir el env del DESTINO: %s", emit)
	}
	if !strings.Contains(emit, "CCP_RESUME_ID='"+uuid+"'") {
		t.Fatalf("resume debe reanudar la sesión prestada: %s", emit)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 || len(h.Archived) != 0 {
		t.Fatalf("resume no debe mutar handoffs.yaml: %+v", h)
	}
}

func TestHandoffResumeSinTranscriptFalla(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Slug: SlugForCwd(cwd), Cwd: cwd,
		From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z",
	}}})
	if _, err := HandoffResume(home, cwd, "", false); err == nil {
		t.Fatal("esperaba error: el jsonl no existe en el destino")
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 {
		t.Fatal("un resume fallido no debe tocar el marcador")
	}
}
```

- [ ] **Step 2: Corre los tests y verifica que fallan**

Run: `go test ./internal/core/ -run 'TestHandoffResume' -v`
Expected: FAIL — `undefined: HandoffResume`.

- [ ] **Step 3: Implementa**

Añade al final de `internal/core/handoff.go`:

```go
// HandoffResume re-entra a un handoff en vuelo SIN cerrarlo: no copia
// transcripts, no escribe ni archiva marcador. Solo resuelve el marcador,
// verifica que el jsonl sigue en el destino y emite el env del DESTINO con el
// uuid ya prestado. Es lo que hace posible tener N handoffs vivos: sin esto,
// volver a uno exigiría terminarlo.
func HandoffResume(home, cwd, sessionFlag string, yolo bool) (string, error) {
	cfg, err := Load(home)
	if err != nil {
		return "", err
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	idx, _, err := ResolveActive(h, cwd, sessionFlag)
	if err != nil {
		return "", err
	}
	m := h.Active[idx]
	toCC, err := CCHome(home, m.To)
	if err != nil {
		return "", err
	}
	srcPath := ProjectDir(toCC, m.Slug) + "/" + m.Session + ".jsonl"
	if _, err := os.Stat(srcPath); err != nil {
		return "", fmt.Errorf("no encuentro la sesión %s en %s; el marcador queda activo", m.Session, m.To)
	}
	emit := EnvDelta(home, m.To, cfg) + resumeSuffix(m.Session, yolo)
	if cwd != "" && cwd != m.Cwd {
		emit = warnLine("el cwd actual difiere del marcador del handoff") + emit
	}
	return emit, nil
}
```

- [ ] **Step 4: Corre los tests y verifica que pasan**

Run: `go test ./internal/core/ -run 'TestHandoffResume' -v`
Expected: PASS los 2.

- [ ] **Step 5: Commit** (solo con autorización explícita)

```bash
git add internal/core/handoff.go internal/core/handoff_test.go
git commit -m "feat(handoff): HandoffResume re-entra a un handoff en vuelo"
```

---

### Task 6: Eval-effect de `CCP_RESUME_YOLO` en bash y zsh

**Files:**
- Modify: `internal/core/handoff_eval_test.go`

Por qué: el emit se **evalúa** en dos shells distintos. Un test Go que solo busca substrings no detecta un quoting roto; este sí.

- [ ] **Step 1: Escribe el test que falla**

Añade a `internal/core/handoff_eval_test.go`:

```go
// TestHandoffYoloEvalEffect verifica en bash y zsh reales que el emit define
// CCP_RESUME_YOLO con --yolo y lo deja SIN definir en el caso normal, incluso
// si la variable venía seteada del entorno (por eso el `export` previo).
func TestHandoffYoloEvalEffect(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		sh := sh
		t.Run(sh, func(t *testing.T) {
			shPath, ok := lookShell(sh)
			if !ok {
				t.Skipf("%s no disponible en PATH", sh)
			}
			home := t.TempDir()
			seedHandoffEnv(t, home)
			cwd := "/repo"
			uuid := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
			srcDir := ProjectDir(home+"/profiles/personal-cc/cc-home", SlugForCwd(cwd))
			writeJSONL(t, srcDir, uuid, "T", time.Now())

			for _, tc := range []struct {
				name string
				yolo bool
				want string
			}{
				{"con yolo", true, "YOLO=1"},
				{"sin yolo", false, "YOLO="},
			} {
				emit, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, false, tc.yolo, time.Now())
				if err != nil {
					t.Fatalf("%s: %v", tc.name, err)
				}
				script := "export CCP_RESUME_YOLO=heredado\n" + emit + "\necho \"YOLO=$CCP_RESUME_YOLO\"\n"
				out, err := exec.Command(shPath, "-c", script).CombinedOutput()
				if err != nil {
					t.Fatalf("%s/%s eval falló: %v\nsalida:\n%s", sh, tc.name, err, out)
				}
				got := strings.TrimSpace(string(out))
				if !strings.Contains(got, tc.want) {
					t.Errorf("%s/%s: esperaba %q en la salida, got %q", sh, tc.name, tc.want, got)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Corre el test y verifica que pasa** (la implementación ya está — este test **confirma** el quoting de Task 3)

Run: `go test ./internal/core/ -run 'TestHandoffYoloEvalEffect' -v`
Expected: PASS en bash y zsh. Si zsh no está instalado, SKIP de ese subtest.

- [ ] **Step 3: Corre todo el paquete**

Run: `go test ./internal/core/ && gofmt -l internal cmd && go vet ./...`
Expected: PASS, `gofmt` sin salida.

- [ ] **Step 4: Commit** (solo con autorización explícita)

```bash
git add internal/core/handoff_eval_test.go
git commit -m "test(handoff): eval-effect de CCP_RESUME_YOLO en bash y zsh"
```

---

### Task 7: Función shell — `resume` + rama yolo (contrato congelado)

**Files:**
- Modify: `internal/core/shellinit.go:21-28`
- Modify: `legacy/bin/ccp:552-559`
- Regenerate: `testdata/golden/basic/expected/*`
- Test: `internal/core/shellinit_test.go`, `internal/golden/parity_test.go`

⚠️ **Este es el paso de mayor riesgo del plan.** El texto de la función shell vive **byte-idéntico** en dos sitios (`internal/core/shellinit.go` y el oráculo bash `legacy/bin/ccp`) y el golden lo compara. Cambia los dos con el mismo texto, luego regenera.

- [ ] **Step 1: Escribe el test que falla**

Añade a `internal/core/shellinit_test.go`:

```go
func TestShellInitTieneResumeYYolo(t *testing.T) {
	var b strings.Builder
	if _, err := WriteShellInit(&b); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	for _, want := range []string{
		"resume)",
		"_handoff-resume",
		"CCP_RESUME_YOLO",
		"--dangerously-skip-permissions",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("shell init sin %q:\n%s", want, s)
		}
	}
}
```

- [ ] **Step 2: Corre el test y verifica que falla**

Run: `go test ./internal/core/ -run 'TestShellInitTieneResumeYYolo' -v`
Expected: FAIL — faltan los 4 substrings.

- [ ] **Step 3: Cambia el bloque en `internal/core/shellinit.go`**

Sustituye las líneas 21-28 (el `handoff)` completo) por:

```
    handoff)
      shift
      case "$1" in
        end)          shift; out=$(command ccp _handoff-end "$PWD" "$@") || return ;;
        resume)       shift; out=$(command ccp _handoff-resume "$PWD" "$@") || return ;;
        status|list)  command ccp handoff "$@"; return ;;
        *)            out=$(command ccp _handoff "$PWD" "$@") || return ;;
      esac
      ( eval "$out" || exit
        if [[ -n "${CCP_RESUME_YOLO:-}" ]]; then
          claude --resume "$CCP_RESUME_ID" --dangerously-skip-permissions
        else
          claude --resume "$CCP_RESUME_ID"
        fi ) ;;
```

Por qué la rama y no interpolar args: zsh **no** hace word-splitting de `$var` sin comillas y bash sí. Interpolar una lista de flags se comportaría distinto en cada shell; el `if` se comporta igual en ambos.

⚠️ **No quites el `|| exit`** (ya está en el bloque actual, con test de regresión en `internal/core/handoff_hardening_test.go:TestShellInitNoLanzaClaudeSiElEvalFalla`). bash y zsh parsean TODO el input del `eval` antes de ejecutar nada: un emit con error de sintaxis no aplica ni el `unset` ni el `export`, así que sin la guarda el subshell continuaría y lanzaría `claude` en el perfil **origen** con `--resume ""` — con el marcador ya persistido como activo. `exit` sale solo del subshell; la shell interactiva sobrevive.

- [ ] **Step 4: Copia el MISMO texto al oráculo bash**

En `legacy/bin/ccp`, sustituye las líneas 552-559 por el bloque idéntico del paso 3 (mismo indentado: el bloque vive dentro de un heredoc en `_print_shell_init()`).

Verifica que los dos textos coinciden:

```bash
go build -o /tmp/ccp-check ./cmd/ccp && /tmp/ccp-check completion-shellinit > /tmp/go-init.txt
bash legacy/bin/ccp completion-shellinit > /tmp/bash-init.txt
diff /tmp/go-init.txt /tmp/bash-init.txt && echo "IDENTICOS"
```
Expected: `IDENTICOS` sin diff.

- [ ] **Step 5: Regenera el golden y valida la paridad**

```bash
bash testdata/golden/capture.sh
git diff --stat testdata/golden/
go test ./internal/golden/ -v
```
Expected: cambian solo `completion-shellinit.out`, `completion-bash.out`, `completion-zsh.out`; `parity_test.go` PASS.

- [ ] **Step 6: Corre la suite bash y los tests Go**

```bash
bash legacy/tests/run.sh
go test ./...
```
Expected: ambas verdes.

- [ ] **Step 7: Commit** (solo con autorización explícita)

```bash
git add internal/core/shellinit.go internal/core/shellinit_test.go legacy/bin/ccp testdata/golden
git commit -m "feat(handoff): shell function gana resume y rama skip-permissions"
```

---

### Task 8: Strings i18n

**Files:**
- Modify: `internal/core/i18n/catalog_cli.go:499-527`

- [ ] **Step 1: Añade las entradas**

En el bloque `// --- handoff.go ---` de `internal/core/i18n/catalog_cli.go`, **después** de `"cli.handoff.list_empty"`, añade:

```go
	"cli.handoff.status_none_here": {
		En: "No active handoff for this project.",
		Es: "Sin handoff activo para este proyecto.",
	},
	"cli.handoff.status_row": {
		En: "  %s → %s · session %s · %s · since %s",
		Es: "  %s → %s · sesión %s · %s · desde %s",
	},
	"cli.handoff.active_header": {
		En: "Active handoffs (%d):",
		Es: "Handoffs activos (%d):",
	},
	"cli.handoff.elsewhere_header": {
		En: "Active in other projects:",
		Es: "Activos en otros proyectos:",
	},
	"cli.handoff.need_session": {
		En: "Several active handoffs for this project and no TTY: pass --session <uuid>.",
		Es: "Varios handoffs activos para este proyecto y no hay TTY: usa --session <uuid>.",
	},
	"cli.handoff.pick_marker": {
		En: "Which handoff?",
		Es: "¿Cuál handoff?",
	},
	"cli.handoff.in_flight": {
		En: "in flight → %s",
		Es: "en vuelo → %s",
	},
	"cli.handoff.hook_notice": {
		En: "ccp: active handoff here — %s → %s (since %s); `ccp handoff end` to return",
		Es: "ccp: handoff activo aquí — %s → %s (desde %s); `ccp handoff end` para volver",
	},
	"cli.handoff.hook_notice_many": {
		En: "ccp: %d active handoffs here; see `ccp handoff`",
		Es: "ccp: %d handoffs activos aquí; revisa `ccp handoff`",
	},
```

- [ ] **Step 2: Verifica que el catálogo compila y no hay claves duplicadas**

Run: `go test ./internal/core/i18n/ -v`
Expected: PASS (el paquete ya tiene un test que valida que toda clave tiene `En` y `Es`).

- [ ] **Step 3: Commit** (solo con autorización explícita)

```bash
git add internal/core/i18n/catalog_cli.go
git commit -m "i18n(handoff): strings de multi-activo, resume y aviso del hook"
```

---

### Task 9: CLI — flags, `_handoff-resume`, `status`/`list` multi

**Files:**
- Modify: `internal/cli/handoff.go`
- Modify: `internal/cli/cli.go:68-71`
- Test: `internal/cli/handoff_test.go`

- [ ] **Step 1: Escribe los tests que fallan**

Añade a `internal/cli/handoff_test.go`:

```go
func TestParseHandoffFlags(t *testing.T) {
	cases := []struct {
		args    []string
		wantTo  string
		wantSes string
		wantYol bool
		wantMk  bool
	}{
		{[]string{"emco-cc"}, "emco-cc", "", false, true},
		{[]string{"emco-cc", "--session", "abc"}, "emco-cc", "abc", false, true},
		{[]string{"emco-cc", "--yolo"}, "emco-cc", "", true, true},
		{[]string{"emco-cc", "--dangerously-skip-permissions"}, "emco-cc", "", true, true},
		{[]string{"emco-cc", "--no-marker", "--yolo"}, "emco-cc", "", true, false},
		{[]string{"--session", "abc", "emco-cc"}, "emco-cc", "abc", false, true},
	}
	for _, c := range cases {
		f, err := parseHandoffFlags(c.args)
		if err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		if f.to != c.wantTo || f.session != c.wantSes || f.yolo != c.wantYol || f.marker != c.wantMk {
			t.Errorf("%v → %+v", c.args, f)
		}
	}
}

func TestParseHandoffFlagsSessionSinValor(t *testing.T) {
	if _, err := parseHandoffFlags([]string{"emco-cc", "--session"}); err == nil {
		t.Fatal("esperaba error: --session sin valor")
	}
}

func TestHandoffStatusSinActivosExit1(t *testing.T) {
	var out, errb bytes.Buffer
	t.Setenv("CCP_HOME", t.TempDir())
	code := Dispatch([]string{"handoff", "status"}, &out, &errb)
	if code != 1 {
		t.Fatalf("esperaba exit 1 sin activos, got %d", code)
	}
}
```

Asegúrate de que el archivo importa `"bytes"` y `"testing"`.

- [ ] **Step 2: Corre los tests y verifica que fallan**

Run: `go test ./internal/cli/ -run 'TestParseHandoffFlags|TestHandoffStatus' -v`
Expected: FAIL — `undefined: parseHandoffFlags`.

- [ ] **Step 3: Implementa el parser y los subcomandos**

En `internal/cli/handoff.go`, añade el parser y sustituye `cmdHandoffEmit` / `cmdHandoffEndEmit`:

```go
// handoffFlags es el resultado de parsear la cola de argumentos de handoff.
type handoffFlags struct {
	to      string
	session string
	yolo    bool
	marker  bool
	force   bool
}

// parseHandoffFlags acepta, en cualquier orden: [<to>|<uuid>] --session <uuid>
// --yolo|--dangerously-skip-permissions --no-marker --force.
// El primer argumento posicional es el destino (forward) o el uuid (end/resume);
// el caller decide cómo interpretarlo.
func parseHandoffFlags(args []string) (handoffFlags, error) {
	f := handoffFlags{marker: true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--session":
			if i+1 >= len(args) {
				return f, fmt.Errorf("--session requiere un valor")
			}
			f.session = args[i+1]
			i++
		case a == "--yolo" || a == "--dangerously-skip-permissions":
			f.yolo = true
		case a == "--no-marker":
			f.marker = false
		case a == "--force":
			f.force = true
		case strings.HasPrefix(a, "-"):
			return f, fmt.Errorf("flag desconocido: %s", a)
		case f.to == "":
			f.to = a
		}
	}
	return f, nil
}

// cmdHandoffEmit implementa `_handoff <pwd> [to] [flags]`. Sin `to` y con TTY
// abre el panel gestor (que puede terminar en forward, resume o end); sin TTY
// es error. Emite a stdout el delta eval-able; la TUI se renderiza en /dev/tty.
func cmdHandoffEmit(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if len(args) < 1 {
		fmt.Fprintln(stderr, "[error] _handoff requiere <pwd>")
		return 1
	}
	cwd := args[0]
	f, err := parseHandoffFlags(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	from := activeProfile(home, cwd)

	// Sin destino: panel gestor (Task 11). Devuelve el emit ya resuelto.
	if f.to == "" && f.session == "" {
		emit, err := runHandoffPanel(home, from, cwd, f.yolo, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, emit)
		return 0
	}

	if f.to == "" {
		picked, err := pickHandoffProfile(home, from, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		f.to = picked
	}
	if f.session == "" {
		picked, err := pickHandoffSession(home, from, cwd, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		f.session = picked
	}

	emit, err := core.HandoffForward(home, from, f.to, cwd, f.session, f.marker, f.yolo, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, emit)
	return 0
}

// cmdHandoffEndEmit implementa `_handoff-end <pwd> [uuid] [flags]`.
func cmdHandoffEndEmit(args []string, stdout, stderr io.Writer) int {
	return handoffTargeted(args, stdout, stderr, "end")
}

// cmdHandoffResumeEmit implementa `_handoff-resume <pwd> [uuid] [flags]`.
func cmdHandoffResumeEmit(args []string, stdout, stderr io.Writer) int {
	return handoffTargeted(args, stdout, stderr, "resume")
}

// handoffTargeted comparte el flujo de end/resume: parsear, resolver (con
// desambiguación TUI si hace falta) y emitir. El uuid puede venir posicional
// (`ccp handoff end <uuid>`) o por --session.
func handoffTargeted(args []string, stdout, stderr io.Writer, op string) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if len(args) < 1 {
		fmt.Fprintf(stderr, "[error] _handoff-%s requiere <pwd>\n", op)
		return 1
	}
	cwd := args[0]
	f, err := parseHandoffFlags(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	session := f.session
	if session == "" {
		session = f.to // posicional: `ccp handoff end <uuid>`
	}

	// Desambiguar ANTES de mutar: si hay 2+ activos para este cwd, elegir uno.
	if session == "" {
		h, err := core.LoadHandoffs(home)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		_, cands, rerr := core.ResolveActive(h, cwd, "")
		if errors.Is(rerr, core.ErrAmbiguousHandoff) {
			picked, perr := tui.RunHandoffMarkerPicker(cands, stderr)
			if perr != nil {
				fmt.Fprintf(stderr, "[error] %v: %v\n", i18n.T(currentLang(), "cli.handoff.need_session"), perr)
				return 1
			}
			session = picked
		}
	}

	var emit string
	if op == "end" {
		emit, err = core.HandoffEnd(home, cwd, session, f.yolo, time.Now())
	} else {
		emit, err = core.HandoffResume(home, cwd, session, f.yolo)
	}
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, emit)
	return 0
}
```

Sustituye los `case "status"` / `case "list"` de `cmdHandoff` por:

```go
	case "status":
		h, _ := core.LoadHandoffs(home)
		all := len(args) > 1 && args[1] == "--all"
		cwd := currentDir()
		here := core.ActiveForCwd(h, cwd)
		if all {
			if len(h.Active) == 0 {
				fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.no_active"))
				return 1
			}
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.active_header", len(h.Active)))
			for _, m := range h.Active {
				printMarker(stdout, lang, m)
			}
			return 0
		}
		if len(here) == 0 {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.status_none_here"))
			if len(h.Active) > 0 {
				fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.elsewhere_header"))
				for _, m := range h.Active {
					printMarker(stdout, lang, m)
				}
			}
			return 1
		}
		fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.active_header", len(here)))
		for _, i := range here {
			printMarker(stdout, lang, h.Active[i])
		}
		return 0
	case "list":
		h, _ := core.LoadHandoffs(home)
		if len(h.Archived) == 0 && len(h.Active) == 0 {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_empty"))
			return 0
		}
		fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_header"))
		for _, m := range h.Active {
			printMarker(stdout, lang, m)
		}
		for _, a := range h.Archived {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_row", a.From, a.To, a.Session, a.ReturnedAs, a.Ended))
		}
		return 0
```

Y añade el helper al final del archivo:

```go
// printMarker imprime una fila de handoff activo: origen → destino, uuid corto,
// proyecto y antigüedad.
func printMarker(w io.Writer, lang i18n.Lang, m core.Marker) {
	fmt.Fprintln(w, i18n.T(lang, "cli.handoff.status_row", m.From, m.To, core.ShortUUID(m.Session), m.Cwd, m.Since))
}
```

Ajusta los imports de `internal/cli/handoff.go` a: `errors`, `fmt`, `io`, `os`, `strings`, `time`, `core`, `i18n`, `tui`.

En `internal/cli/cli.go`, tras `case "_handoff-end":`, añade:

```go
	case "_handoff-resume":
		return cmdHandoffResumeEmit(rest, stdout, stderr)
```

- [ ] **Step 4: Corre los tests y verifica que pasan**

Run: `go test ./internal/cli/ -v`
Expected: PASS. (Compilará solo tras la Task 10, que añade `RunHandoffMarkerPicker`, y la Task 11, que añade `runHandoffPanel`. Si prefieres compilar ya, crea stubs temporales que devuelvan `("", fmt.Errorf("no implementado"))` y bórralos en esas tareas.)

- [ ] **Step 5: Commit** (solo con autorización explícita)

```bash
git add internal/cli/handoff.go internal/cli/cli.go internal/cli/handoff_test.go
git commit -m "feat(handoff): CLI multi-activo, resume y flags de skip-permissions"
```

---

### Task 10: TUI — picker de marcador + sesiones en vuelo marcadas

**Files:**
- Modify: `internal/tui/handoff.go`
- Test: `internal/tui/handoff_test.go`

- [ ] **Step 1: Escribe el test que falla**

Añade a `internal/tui/handoff_test.go` (créalo si no existe, con `package tui`):

```go
package tui

import (
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

func TestMarkerLabel(t *testing.T) {
	m := core.Marker{
		Session: "bbc1ed61-ada1-408f-0000-000000000000",
		Cwd:     "/repo/uno", From: "personal-cc", To: "emco-cc",
		Title: "Refactor handoff", Since: "2026-07-25T14:30:00Z",
	}
	got := markerLabel(m)
	for _, want := range []string{"personal-cc", "emco-cc", "bbc1ed61", "Refactor handoff"} {
		if !strings.Contains(got, want) {
			t.Errorf("label sin %q: %s", want, got)
		}
	}
}

func TestSessionLabelMarcaEnVuelo(t *testing.T) {
	inFlight := map[string]string{"aaa": "emco-cc"}
	got := sessionLabel(core.SessionInfo{UUID: "aaa", Title: "T"}, inFlight)
	if !strings.Contains(got, "en vuelo") {
		t.Errorf("la sesión prestada debe marcarse: %s", got)
	}
	got2 := sessionLabel(core.SessionInfo{UUID: "bbb", Title: "T"}, inFlight)
	if strings.Contains(got2, "en vuelo") {
		t.Errorf("la sesión libre no debe marcarse: %s", got2)
	}
}
```

- [ ] **Step 2: Corre el test y verifica que falla**

Run: `go test ./internal/tui/ -run 'TestMarkerLabel|TestSessionLabel' -v`
Expected: FAIL — `undefined: markerLabel`, `undefined: sessionLabel`.

- [ ] **Step 3: Implementa**

En `internal/tui/handoff.go` añade:

```go
// markerLabel formatea un marcador activo para los selects.
func markerLabel(m core.Marker) string {
	title := m.Title
	if title == "" {
		title = "(sin título)"
	}
	return fmt.Sprintf("%s · %s → %s · %s · %s", m.Cwd, m.From, m.To, core.ShortUUID(m.Session), title)
}

// sessionLabel formatea una sesión del picker. inFlight mapea uuid → perfil
// destino de los handoffs vivos, para marcar las que ya están prestadas.
func sessionLabel(s core.SessionInfo, inFlight map[string]string) string {
	title := s.Title
	if title == "" {
		title = "(sin título)"
	}
	label := fmt.Sprintf("%s · %s · %s", s.ModTime.Format("2006-01-02 15:04"), title, core.ShortUUID(s.UUID))
	if to, ok := inFlight[s.UUID]; ok {
		label += fmt.Sprintf("  ⟳ en vuelo → %s", to)
	}
	return label
}

// RunHandoffMarkerPicker desambigua entre 2+ handoffs activos del mismo
// proyecto. Devuelve el uuid de sesión elegido. Sin TTY devuelve error (el
// caller pide --session).
func RunHandoffMarkerPicker(cands []core.Marker, w io.Writer) (string, error) {
	if len(cands) == 0 {
		return "", fmt.Errorf("no hay handoffs entre los que elegir")
	}
	tty, err := openTTY()
	if err != nil {
		return "", err
	}
	defer tty.Close()

	opts := make([]huh.Option[string], len(cands))
	for i, m := range cands {
		opts[i] = huh.NewOption(markerLabel(m), m.Session)
	}
	var chosen string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("¿Cuál handoff?").
				Options(opts...).
				Value(&chosen),
		),
	).WithOutput(tty).WithInput(tty)
	if err := form.Run(); err != nil {
		return "", fmt.Errorf("picker de handoff cancelado: %w", err)
	}
	return chosen, nil
}
```

En `RunHandoffSessionPicker`, sustituye el bloque que arma `huhOpts` (el `for i, s := range sess { … }`) por:

```go
	h, _ := core.LoadHandoffs(home)
	inFlight := make(map[string]string, len(h.Active))
	for _, m := range h.Active {
		inFlight[m.Session] = m.To
	}

	huhOpts := make([]huh.Option[string], 0, len(sess))
	for _, s := range sess {
		if _, prestada := inFlight[s.UUID]; prestada {
			// Las sesiones ya en vuelo se muestran pero no se pueden elegir:
			// HandoffForward las rechazaría igual, mejor no ofrecerlas.
			continue
		}
		huhOpts = append(huhOpts, huh.NewOption(sessionLabel(s, inFlight), s.UUID))
	}
	if len(huhOpts) == 0 {
		return "", fmt.Errorf("todas las sesiones de este proyecto ya están en vuelo; usa `ccp handoff resume` o termina una con `ccp handoff end`")
	}
```

(La función `sessionLabel` con `inFlight` sigue siendo útil para el panel de la Task 11, que sí las lista.)

- [ ] **Step 4: Corre los tests y verifica que pasan**

Run: `go test ./internal/tui/ -v`
Expected: PASS.

- [ ] **Step 5: Commit** (solo con autorización explícita)

```bash
git add internal/tui/handoff.go internal/tui/handoff_test.go
git commit -m "feat(handoff): picker de desambiguación y marcado de sesiones en vuelo"
```

---

### Task 11: TUI — panel gestor de `ccp handoff`

**Files:**
- Create: `internal/tui/handoff_panel.go`
- Modify: `internal/cli/handoff.go` (quita el stub de `runHandoffPanel`)
- Test: `internal/tui/handoff_panel_test.go`

- [ ] **Step 1: Escribe el test que falla**

Crea `internal/tui/handoff_panel_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

func TestPanelRowsOrdenaEsteRepoPrimero(t *testing.T) {
	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "aaa", Slug: core.SlugForCwd("/otro"), Cwd: "/otro", From: "p", To: "k", Since: "2026-07-20T00:00:00Z"},
		{Session: "bbb", Slug: core.SlugForCwd("/repo"), Cwd: "/repo", From: "p", To: "e", Since: "2026-07-25T00:00:00Z"},
	}}
	rows := panelRows(h, "/repo")
	if len(rows) != 2 {
		t.Fatalf("esperaba 2 filas, got %d", len(rows))
	}
	if rows[0].marker.Session != "bbb" || !rows[0].here {
		t.Fatalf("el handoff de este repo debe ir primero y marcado: %+v", rows[0])
	}
	if rows[1].here {
		t.Fatalf("el de otro repo no debe marcarse: %+v", rows[1])
	}
}

func TestPanelViewMuestraAccionesYModo(t *testing.T) {
	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "bbb", Slug: core.SlugForCwd("/repo"), Cwd: "/repo", From: "p", To: "e", Title: "T", Since: "2026-07-25T00:00:00Z"},
	}}
	m := newPanelModel(h, "/repo", true)
	v := m.View()
	for _, want := range []string{"ACTIVOS", "reanudar", "terminar", "nuevo", "skip-permissions: on"} {
		if !strings.Contains(v, want) {
			t.Errorf("la vista no muestra %q:\n%s", want, v)
		}
	}
}

func TestPanelSinActivosPideNuevo(t *testing.T) {
	m := newPanelModel(&core.Handoffs{Version: core.HandoffsVersion}, "/repo", false)
	if m.action() != panelActionNew {
		t.Fatal("sin activos, la acción por defecto es un handoff nuevo")
	}
}
```

- [ ] **Step 2: Corre el test y verifica que falla**

Run: `go test ./internal/tui/ -run 'TestPanel' -v`
Expected: FAIL — `undefined: panelRows`, `undefined: newPanelModel`.

- [ ] **Step 3: Implementa el panel**

Crea `internal/tui/handoff_panel.go`:

```go
// handoff_panel.go — panel gestor de `ccp handoff` sin argumentos. Muestra los
// handoffs activos (los de este repo primero y marcados) y ofrece las cuatro
// acciones: reanudar, terminar, nuevo y toggle de skip-permissions. El panel NO
// ejecuta nada: devuelve la acción elegida y el caller (internal/cli) llama al
// core, que emite el env. Así la TUI queda fina y todo lo testeable vive fuera.
package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// panelAction es lo que el usuario decidió hacer al salir del panel.
type panelAction int

const (
	panelActionNone panelAction = iota
	panelActionResume
	panelActionEnd
	panelActionNew
)

// PanelResult es la decisión del panel, ya lista para que el CLI actúe.
type PanelResult struct {
	Action  panelAction
	Session string // uuid del marcador elegido (vacío para "nuevo")
	Yolo    bool
}

// panelRow es una fila de la lista: un marcador + si pertenece al cwd actual.
type panelRow struct {
	marker core.Marker
	here   bool
}

// panelRows ordena los activos: primero los del cwd (marcados), luego el resto.
// Dentro de cada grupo conserva el orden de handoffs.yaml (cronológico de alta).
func panelRows(h *core.Handoffs, cwd string) []panelRow {
	slug := core.SlugForCwd(cwd)
	var here, others []panelRow
	for _, m := range h.Active {
		r := panelRow{marker: m, here: m.Slug == slug}
		if r.here {
			here = append(here, r)
		} else {
			others = append(others, r)
		}
	}
	return append(here, others...)
}

type panelModel struct {
	rows   []panelRow
	idx    int
	yolo   bool
	result PanelResult
	done   bool
}

func newPanelModel(h *core.Handoffs, cwd string, yolo bool) panelModel {
	return panelModel{rows: panelRows(h, cwd), yolo: yolo}
}

// action devuelve la acción por defecto según el estado del panel: sin activos
// la única acción sensata es crear uno nuevo.
func (m panelModel) action() panelAction {
	if len(m.rows) == 0 {
		return panelActionNew
	}
	return panelActionNone
}

func (m panelModel) Init() tea.Cmd { return nil }

func (m panelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "q", "esc", "ctrl+c":
		m.done = true
		m.result = PanelResult{Action: panelActionNone}
		return m, tea.Quit
	case "up", "k":
		if m.idx > 0 {
			m.idx--
		}
	case "down", "j":
		if m.idx < len(m.rows)-1 {
			m.idx++
		}
	case "y":
		m.yolo = !m.yolo
	case "n":
		m.done = true
		m.result = PanelResult{Action: panelActionNew, Yolo: m.yolo}
		return m, tea.Quit
	case "enter":
		if len(m.rows) == 0 {
			m.result = PanelResult{Action: panelActionNew, Yolo: m.yolo}
		} else {
			m.result = PanelResult{Action: panelActionResume, Session: m.rows[m.idx].marker.Session, Yolo: m.yolo}
		}
		m.done = true
		return m, tea.Quit
	case "e":
		if len(m.rows) > 0 {
			m.done = true
			m.result = PanelResult{Action: panelActionEnd, Session: m.rows[m.idx].marker.Session, Yolo: m.yolo}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m panelModel) View() string {
	var b strings.Builder
	modo := "off"
	if m.yolo {
		modo = "on"
	}
	title := lipgloss.NewStyle().Bold(true)
	fmt.Fprintf(&b, "%s        skip-permissions: %s\n\n",
		title.Render(fmt.Sprintf("ACTIVOS (%d)", len(m.rows))), modo)

	if len(m.rows) == 0 {
		b.WriteString("  (ninguno)\n\n")
	}
	for i, r := range m.rows {
		cursor := "  "
		if i == m.idx {
			cursor = "▸ "
		}
		mark := " "
		if r.here {
			mark = "•"
		}
		title := r.marker.Title
		if title == "" {
			title = "(sin título)"
		}
		fmt.Fprintf(&b, "%s%s %s · %s → %s · %s · %s\n",
			cursor, mark, r.marker.Cwd, r.marker.From, r.marker.To,
			core.ShortUUID(r.marker.Session), title)
	}
	b.WriteString("\nenter reanudar · e terminar · n nuevo · y skip-permissions · q salir\n")
	return b.String()
}

// RunHandoffPanel muestra el panel en /dev/tty y devuelve la decisión. Sin TTY
// devuelve error: el caller debe caer al equivalente por flags.
func RunHandoffPanel(h *core.Handoffs, cwd string, yolo bool, w io.Writer) (PanelResult, error) {
	tty, err := openTTY()
	if err != nil {
		return PanelResult{}, err
	}
	defer tty.Close()

	p := tea.NewProgram(newPanelModel(h, cwd, yolo), tea.WithInput(tty), tea.WithOutput(tty))
	out, err := p.Run()
	if err != nil {
		return PanelResult{}, fmt.Errorf("panel de handoff cancelado: %w", err)
	}
	fm, ok := out.(panelModel)
	if !ok || !fm.done || fm.result.Action == panelActionNone {
		return PanelResult{}, fmt.Errorf("no se eligió ninguna acción")
	}
	return fm.result, nil
}

// PanelActionResume/End/New se exportan para que internal/cli ramifique sin
// duplicar las constantes.
var (
	PanelActionResume = panelActionResume
	PanelActionEnd    = panelActionEnd
	PanelActionNew    = panelActionNew
)
```

En `internal/cli/handoff.go`, implementa `runHandoffPanel` (sustituyendo el stub de la Task 9):

```go
// runHandoffPanel abre el panel gestor y traduce la decisión a una llamada al
// core. El binario emite; la shell function lanza claude. El panel puede
// terminar en forward (nuevo), resume o end — los tres emiten el mismo par
// CCP_RESUME_ID/CCP_RESUME_YOLO, así que la shell no necesita distinguirlos.
func runHandoffPanel(home, from, cwd string, yolo bool, w io.Writer) (string, error) {
	h, err := core.LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	res, err := tui.RunHandoffPanel(h, cwd, yolo, w)
	if err != nil {
		return "", err
	}
	switch res.Action {
	case tui.PanelActionResume:
		return core.HandoffResume(home, cwd, res.Session, res.Yolo)
	case tui.PanelActionEnd:
		return core.HandoffEnd(home, cwd, res.Session, res.Yolo, time.Now())
	case tui.PanelActionNew:
		to, err := pickHandoffProfile(home, from, w)
		if err != nil {
			return "", err
		}
		session, err := pickHandoffSession(home, from, cwd, w)
		if err != nil {
			return "", err
		}
		return core.HandoffForward(home, from, to, cwd, session, true, res.Yolo, time.Now())
	default:
		return "", fmt.Errorf("no se eligió ninguna acción")
	}
}
```

- [ ] **Step 4: Corre los tests y verifica que pasan**

Run: `go test ./internal/tui/ ./internal/cli/ -v`
Expected: PASS.

- [ ] **Step 5: Verifica el binario completo**

```bash
go build ./... && gofmt -l internal cmd && go vet ./... && go test ./...
```
Expected: build OK, `gofmt` sin salida, todo verde.

- [ ] **Step 6: Commit** (solo con autorización explícita)

```bash
git add internal/tui/handoff_panel.go internal/tui/handoff_panel_test.go internal/cli/handoff.go
git commit -m "feat(handoff): panel gestor TUI de handoffs activos"
```

---

### Task 12: Aviso de handoff activo en el hook

**Files:**
- Modify: `internal/cli/env.go:62-77`
- Test: `internal/cli/hook_test.go`

- [ ] **Step 1: Escribe el test que falla**

Añade a `internal/cli/hook_test.go` (créalo con `package cli` si no existe):

```go
func TestHookAvisaHandoffActivo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	cwd := "/repo/uno"
	if err := core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{{
			Session: "aaa", Slug: core.SlugForCwd(cwd), Cwd: cwd,
			From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"_hook", cwd}, &out, &errb); code != 0 {
		t.Fatalf("_hook debe salir 0, got %d", code)
	}
	s := out.String()
	if !strings.Contains(s, "handoff activo") || !strings.Contains(s, ">&2") {
		t.Fatalf("el emit del hook debe llevar el aviso a stderr: %s", s)
	}
	if !strings.Contains(s, "CLAUDE_CONFIG_DIR") && !strings.Contains(s, "unset") {
		t.Fatalf("el hook debe seguir emitiendo el delta de env: %s", s)
	}
}

func TestHookSinHandoffNoAvisa(t *testing.T) {
	t.Setenv("CCP_HOME", t.TempDir())
	var out, errb bytes.Buffer
	Dispatch([]string{"_hook", "/repo/uno"}, &out, &errb)
	if strings.Contains(out.String(), "handoff activo") {
		t.Fatalf("sin marcadores no debe avisar: %s", out.String())
	}
}
```

- [ ] **Step 2: Corre los tests y verifica que fallan**

Run: `go test ./internal/cli/ -run 'TestHook' -v`
Expected: FAIL — el emit no contiene el aviso.

- [ ] **Step 3: Implementa**

En `internal/cli/env.go`, sustituye el final de `cmdHook` (la línea `io.WriteString(stdout, core.EnvDelta(home, prof, cfg))`) por:

```go
	io.WriteString(stdout, core.EnvDelta(home, prof, cfg))
	io.WriteString(stdout, handoffHookNotice(home, query))
	return 0
}

// handoffHookNotice devuelve la línea `echo … >&2` que recuerda los handoffs
// vivos de este proyecto, o "" si no hay. Va en el emit (no a os.Stderr) porque
// el hook se evalúa: es la shell la que imprime. _ccp_autocheck cachea por
// $PWD, así que aparece una vez por `cd`, no en cada prompt.
func handoffHookNotice(home, cwd string) string {
	h, err := core.LoadHandoffs(home)
	if err != nil || len(h.Active) == 0 {
		return ""
	}
	idxs := core.ActiveForCwd(h, cwd)
	lang := currentLang()
	switch len(idxs) {
	case 0:
		return ""
	case 1:
		m := h.Active[idxs[0]]
		return "echo \"↳ " + i18n.T(lang, "cli.handoff.hook_notice", m.From, m.To, m.Since) + "\" >&2\n"
	default:
		return "echo \"↳ " + i18n.T(lang, "cli.handoff.hook_notice_many", len(idxs)) + "\" >&2\n"
	}
}
```

Añade `i18n` a los imports de `internal/cli/env.go` si falta.

⚠️ El string de i18n `cli.handoff.hook_notice` contiene backticks (`` `ccp handoff end` ``). Dentro de `echo "…"` los backticks harían **command substitution**. Escápalos en el catálogo (Task 8) como ``\` `` — es decir, en Go: `"\\`ccp handoff end\\`"`. Verifica con el test de eval del paso siguiente.

- [ ] **Step 4: Verifica que el aviso es seguro al evaluarse**

Añade a `internal/cli/hook_test.go`:

```go
func TestHookAvisoEsEvalSeguro(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	cwd := "/repo/uno"
	_ = core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{{
			Session: "aaa", Slug: core.SlugForCwd(cwd), Cwd: cwd,
			From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z",
		}},
	})
	var out, errb bytes.Buffer
	Dispatch([]string{"_hook", cwd}, &out, &errb)

	shOut, err := exec.Command("bash", "-c", out.String()).CombinedOutput()
	if err != nil {
		t.Fatalf("el emit del hook no evalúa limpio: %v\n%s", err, shOut)
	}
	if !strings.Contains(string(shOut), "handoff activo") {
		t.Fatalf("el aviso no llegó a stderr: %s", shOut)
	}
}
```

Run: `go test ./internal/cli/ -run 'TestHook' -v`
Expected: PASS los 3.

- [ ] **Step 5: Confirma que el golden NO cambia**

Los fixtures de `_hook` no tienen `handoffs.yaml`, así que el aviso no aparece.

Run: `bash testdata/golden/capture.sh --check && go test ./internal/golden/ -v`
Expected: sin diferencias, PASS.

- [ ] **Step 6: Commit** (solo con autorización explícita)

```bash
git add internal/cli/env.go internal/cli/hook_test.go
git commit -m "feat(handoff): el hook avisa de handoffs activos en el repo"
```

---

### Task 13: E2E del binario — 2 activos, resume y end selectivo

**Files:**
- Test: `internal/cli/handoff_e2e_test.go` (crear)

- [ ] **Step 1: Escribe el test que falla**

Crea `internal/cli/handoff_e2e_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// seedE2E deja un CCP_HOME con 2 perfiles official y una sesión en cada repo,
// dentro del cc-home de personal-cc.
func seedE2E(t *testing.T) (home string, repoA, repoB, uuidA, uuidB string) {
	t.Helper()
	home = t.TempDir()
	cfg := &core.Config{
		Version:  core.SchemaVersion,
		Profiles: map[string]core.Profile{"personal-cc": {Type: "official"}, "emco-cc": {Type: "official"}},
	}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	repoA, repoB = "/repo/uno", "/repo/dos"
	uuidA = "11111111-1111-4111-8111-111111111111"
	uuidB = "22222222-2222-4222-8222-222222222222"
	cc := filepath.Join(home, "profiles", "personal-cc", "cc-home")
	for _, x := range []struct{ cwd, uuid string }{{repoA, uuidA}, {repoB, uuidB}} {
		dir := core.ProjectDir(cc, core.SlugForCwd(x.cwd))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		line := `{"sessionId":"` + x.uuid + `","cwd":"` + x.cwd + `","type":"user"}` + "\n" +
			`{"sessionId":"` + x.uuid + `","type":"ai-title","aiTitle":"Tarea"}` + "\n"
		if err := os.WriteFile(filepath.Join(dir, x.uuid+".jsonl"), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home, repoA, repoB, uuidA, uuidB
}

func TestE2EDosHandoffsResumeYEndSelectivo(t *testing.T) {
	home, repoA, repoB, uuidA, uuidB := seedE2E(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_PROFILE", "personal-cc")

	run := func(args ...string) (string, string, int) {
		var out, errb bytes.Buffer
		code := Dispatch(args, &out, &errb)
		return out.String(), errb.String(), code
	}

	// Forward A
	out, errs, code := run("_handoff", repoA, "emco-cc", "--session", uuidA)
	if code != 0 {
		t.Fatalf("forward A falló: %s", errs)
	}
	if !strings.Contains(out, "CCP_RESUME_ID='"+uuidA+"'") {
		t.Fatalf("forward A no emitió el uuid: %s", out)
	}

	// Forward B en otro repo: v2 lo permite.
	if _, errs, code = run("_handoff", repoB, "emco-cc", "--session", uuidB); code != 0 {
		t.Fatalf("forward B debía permitirse: %s", errs)
	}
	h, _ := core.LoadHandoffs(home)
	if len(h.Active) != 2 {
		t.Fatalf("esperaba 2 activos: %+v", h.Active)
	}

	// Resume de A: emite el env del destino, no muta nada.
	out, errs, code = run("_handoff-resume", repoA)
	if code != 0 {
		t.Fatalf("resume A falló: %s", errs)
	}
	if !strings.Contains(out, "CCP_RESUME_ID='"+uuidA+"'") || !strings.Contains(out, "emco-cc") {
		t.Fatalf("resume A emitió mal: %s", out)
	}
	h, _ = core.LoadHandoffs(home)
	if len(h.Active) != 2 {
		t.Fatal("resume no debe archivar nada")
	}

	// End de B: solo B se archiva; A sigue vivo.
	if _, errs, code = run("_handoff-end", repoB); code != 0 {
		t.Fatalf("end B falló: %s", errs)
	}
	h, _ = core.LoadHandoffs(home)
	if len(h.Active) != 1 || h.Active[0].Session != uuidA {
		t.Fatalf("debía quedar A activo: %+v", h.Active)
	}
	if len(h.Archived) != 1 || h.Archived[0].Session != uuidB {
		t.Fatalf("archivó el equivocado: %+v", h.Archived)
	}

	// El back-sync dejó la sesión nueva en el origen.
	origen := core.ProjectDir(filepath.Join(home, "profiles", "personal-cc", "cc-home"), core.SlugForCwd(repoB))
	entries, err := os.ReadDir(origen)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("esperaba original + sesión de vuelta en %s: %v", origen, entries)
	}

	// End desde un repo sin handoff: error, sin tocar estado.
	if _, _, code = run("_handoff-end", "/repo/sin-handoff"); code == 0 {
		t.Fatal("end en repo sin handoff debe fallar")
	}
	h, _ = core.LoadHandoffs(home)
	if len(h.Active) != 1 {
		t.Fatal("un end fallido no debe mutar estado")
	}
}
```

- [ ] **Step 2: Corre el test y verifica que pasa**

Run: `go test ./internal/cli/ -run 'TestE2EDosHandoffs' -v`
Expected: PASS. Si falla en el forward por "no encuentro la sesión", revisa que `CCP_PROFILE` esté seteado (determina el perfil origen en `activeProfile`).

- [ ] **Step 3: Corre TODOS los gates**

```bash
gofmt -l internal cmd
go vet ./...
go test ./...
bash legacy/tests/run.sh
bash testdata/golden/capture.sh --check
```
Expected: `gofmt` sin salida; el resto verde.

- [ ] **Step 4: Commit** (solo con autorización explícita)

```bash
git add internal/cli/handoff_e2e_test.go
git commit -m "test(handoff): e2e de 2 handoffs activos con resume y end selectivo"
```

---

### Task 14: Documentación + companion HTML

**Files:**
- Modify: `commands/ccp/handoff.md` (si no existe, créalo)
- Modify: `README.md`, `README.es.md`
- Create: `docs/superpowers/plans/2026-07-25-handoff-multi.html`

- [ ] **Step 1: Documenta la superficie nueva**

En `commands/ccp/handoff.md` (y el bloque equivalente de ambos README), sustituye la sección de comandos por:

```markdown
## ccp handoff

Continúa una sesión de Claude Code en otro perfil sin perder contexto. Desde v2
puedes tener **varios handoffs en vuelo a la vez** (uno por sesión).

```
ccp handoff                       # panel: ves los activos y eliges qué hacer
ccp handoff <perfil>              # nuevo handoff hacia <perfil>
ccp handoff <perfil> --session <uuid> [--no-marker] [--force]
ccp handoff resume [<uuid>]       # vuelve a entrar a un handoff vivo (no lo cierra)
ccp handoff end    [<uuid>]       # trae el contexto de vuelta al perfil origen
ccp handoff status [--all]        # activos de este proyecto (o todos)
ccp handoff list                  # activos + historial
```

Cualquiera de los tres lanzamientos acepta `--dangerously-skip-permissions`
(alias `--yolo`): arranca la sesión reanudada saltándose los prompts de permiso
de Claude Code. No se recuerda entre invocaciones — se pide cada vez.

**Cómo elige `end`/`resume` cuál handoff:** por el directorio actual. Si hay
exactamente uno activo en este repo, ese; si hay varios, pregunta; si no hay
ninguno aquí, falla y te dice en qué repos sí los hay. Con `<uuid>` explícito
se salta la resolución.
```

- [ ] **Step 2: Genera el companion HTML**

Invoca el skill `ui-ux-pro-max` para construir `docs/superpowers/plans/2026-07-25-handoff-multi.html` (no lo escribas a mano). Debe cubrir, en este orden: Hero · Resumen ejecutivo · Diagrama de arquitectura (el ciclo forward/resume/end con N marcadores) · Decisiones y trade-offs (lista vs mapping en `handoffs.yaml`; resolución por cwd vs picker siempre; rama `if` en la shell vs interpolar args; no persistir el modo yolo) · Plan de ejecución por fases (las 14 tareas) · Flujos clave · Riesgos y rollback (gate de paridad, migración v1→v2) · Snippets con Copy · Footer. Tema día/noche con toggle, un solo `.html` autocontenido que abra con doble clic.

- [ ] **Step 3: Verifica el HTML**

```bash
open docs/superpowers/plans/2026-07-25-handoff-multi.html
```
Expected: abre sin servidor, el toggle de tema funciona, los diagramas renderizan.

- [ ] **Step 4: Commit** (solo con autorización explícita)

```bash
git add commands/ccp/handoff.md README.md README.es.md docs/superpowers/plans/2026-07-25-handoff-multi.html
git commit -m "docs(handoff): documenta multi-activo, resume y skip-permissions"
```

---

## Verificación final

Antes de dar el feature por hecho, corre y **pega la salida**:

```bash
gofmt -l internal cmd          # debe imprimir NADA
go vet ./...
golangci-lint run
go test ./...
bash legacy/tests/run.sh
bash testdata/golden/capture.sh --check
```

Prueba manual mínima (requiere `ccp install && source ~/.zshrc` para refrescar la función shell):

1. `cd` a dos repos distintos, `ccp handoff <otro-perfil>` en cada uno → `ccp handoff list` muestra 2 activos.
2. `ccp handoff resume` en el primero → entra a Claude con el perfil destino, sin cerrar nada.
3. `ccp handoff end` en el segundo → vuelve al origen; `ccp handoff status` en el primero sigue mostrando su handoff.
4. `cd` de nuevo al primero → aparece el aviso `↳ ccp: handoff activo aquí`.
5. `ccp handoff <perfil> --yolo` → la sesión arranca sin prompts de permiso.

## Self-review (hecho)

- **Cobertura de la spec:** §03 modelo → Task 1 · §04 resolución → Task 2 · §05 comandos → Tasks 5, 9 · §06 invariantes → Task 3 · §07 yolo → Tasks 3, 6, 7 · §08 panel TUI → Tasks 10, 11 · §09 hook → Task 12 · §10 casos borde → repartidos en los tests de Tasks 2-5, 12, 13 · §11 gates → Tasks 7, 13 · §12 archivos → File Structure.
- **Refinamiento sobre la spec:** el warning de "cwd distinto al del marcador" (heredado de v1) ahora solo se alcanza vía `--session` explícito, porque sin uuid la resolución por cwd falla antes. Documentado en Task 4 Step 3.
- **Consistencia de tipos:** `Handoffs.Active []Marker`, `HandoffsVersion`, `ActiveWarnThreshold`, `ResolveActive(h, cwd, sessionFlag) (int, []Marker, error)`, `ErrAmbiguousHandoff`, `ActiveForCwd(h, cwd) []int`, `FindActiveSession(h, session) int`, `ShortUUID(s) string`, `HandoffForward(…, writeMarker, yolo bool, now)`, `HandoffEnd(home, cwd, sessionFlag string, yolo bool, now)`, `HandoffResume(home, cwd, sessionFlag string, yolo bool)`, `resumeSuffix(sessionID, yolo)`, `warnLine(format, args…)`, `oldestActive(h) Marker`, `parseHandoffFlags(args) (handoffFlags, error)`, `RunHandoffMarkerPicker(cands, w)`, `RunHandoffPanel(h, cwd, yolo, w) (PanelResult, error)`, `runHandoffPanel(home, from, cwd, yolo, w) (string, error)`, `handoffHookNotice(home, cwd) string` — usados con la misma firma en todas las tareas.
