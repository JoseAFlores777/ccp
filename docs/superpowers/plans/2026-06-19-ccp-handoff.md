# ccp handoff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `ccp handoff` — continuar una sesión de Claude Code en otro perfil (tokens prestados) sin perder contexto, y traer el trabajo de vuelta como sesión nueva con `ccp handoff end`.

**Architecture:** El binario hace toda la lógica (descubrir/copiar/reescribir el transcript JSONL, marcador en `handoffs.yaml`) y EMITE un delta de env eval-able + `CCP_RESUME_ID`; la shell function lo evalúa en subshell y lanza `claude --resume`. Mismo split que `_env`/`_hook`/`run`. El back-sync copia como uuid nuevo (no destructivo), reescribiendo `sessionId` en cada línea.

**Tech Stack:** Go 1.24 (módulo `github.com/JoseAFlores777/ccp`), `goccy/go-yaml`, `golang.org/x/sys/unix` (flock), `crypto/rand` (uuid v4), `encoding/json` (JSONL), bubbletea+huh (TUI, último). Spec: `docs/superpowers/specs/2026-06-19-ccp-handoff-design.md`.

---

## Convenciones de este plan (LEER ANTES)

- **Commits requieren autorización explícita del usuario en el turno.** Los pasos "Commit" significan: stagear (`git add`) y **PEDIR autorización**, mostrando el comando propuesto. NUNCA ejecutar `git commit` autónomamente. **NUNCA** añadir trailer `Co-Authored-By`. Autor = `Jose Izaguirre <joseadolfoizaguirreflores@gmail.com>`.
- **Tests con `CCP_HOME` temporal** (un dir existente) para que la auto-migración no dispare contra `~/.config/dsctl` real. Patrón: `t.Setenv("CCP_HOME", t.TempDir())`.
- Texto de cara al usuario en **español** vía `i18n.T(lang, "key", args...)`. Las superficies internas (`_handoff` emit) son idioma-neutrales.
- Compilar: `go build ./...`. Tests: `go test ./...`. Gates: `gofmt -l internal cmd` (vacío), `go vet ./...`, `golangci-lint run`.

## Estructura de archivos

| Archivo | Responsabilidad |
|---|---|
| `internal/core/transcript.go` (crear) | Ops puras sobre transcripts JSONL: slug, listar sesiones, copiar, reescribir `sessionId`/`aiTitle`, uuid v4. Sin estado de ccp. |
| `internal/core/transcript_test.go` (crear) | Tests de transcript.go. |
| `internal/core/handoff_store.go` (crear) | Modelo + persistencia de `handoffs.yaml` (marcador). Atómico bajo flock. |
| `internal/core/handoff_store_test.go` (crear) | Tests del store del marcador. |
| `internal/core/handoff.go` (crear) | Orquestación: `HandoffForward`/`HandoffEnd` → devuelven string emit (env + `CCP_RESUME_ID`). |
| `internal/core/handoff_test.go` (crear) | Tests de orquestación (e2e a nivel core, sin lanzar claude). |
| `internal/cli/handoff.go` (crear) | Dispatch `handoff` (status/list/shell-only) + `_handoff`/`_handoff-end` (imprimen emit). |
| `internal/cli/handoff_test.go` (crear) | Tests de dispatch/formato. |
| `internal/cli/cli.go` (modificar) | Añadir casos `handoff`, `_handoff`, `_handoff-end` al switch de `Dispatch`. |
| `internal/core/shellinit.go` (modificar) | Añadir `case handoff)` a `ShellInit` + `handoff` a los `top` de completion (bash+zsh). |
| `legacy/bin/ccp` (modificar) | Espejo bash del shell init + completion (oráculo del contrato). |
| `internal/core/i18n/catalog_cli.go`, `catalog_core.go` (modificar) | Strings nuevas (errores, status, list). |
| `internal/tui/handoff.go` (crear) | Pickers TUI (perfil, sesión) + confirm. Capa fina sobre core. |
| `internal/tui/tui.go` (modificar, opcional) | Wire del flujo handoff si aplica. |
| `testdata/golden/basic/expected/*` (regenerar) | Golden de shell-init/completion vía `capture.sh`. |

**Nota sobre golden:** solo cambian las salidas de `completion-shellinit`, `completion bash`, `completion zsh` (texto del contrato congelado). Los comandos internos `_handoff`/`_handoff-end` NO se añaden al fixture golden (requerirían un transcript + dos perfiles falsos); su comportamiento se cubre con tests Go (unit + eval-effect + e2e). Esto es deliberado: el golden protege el contrato bash↔Go; los emit nuevos no existen en el oráculo bash original salvo el `case handoff)` del shell init.

---

## Task 1: Slug + paths + uuid (transcript.go base)

**Files:**
- Create: `internal/core/transcript.go`
- Test: `internal/core/transcript_test.go`

- [ ] **Step 1: Write the failing test**

```go
package core

import (
	"strings"
	"testing"
)

func TestSlugForCwd(t *testing.T) {
	got := SlugForCwd("/Volumes/X/Personal/Projects/dsctl-v2")
	want := "-Volumes-X-Personal-Projects-dsctl-v2"
	if got != want {
		t.Fatalf("SlugForCwd = %q, want %q", got, want)
	}
}

func TestProjectDir(t *testing.T) {
	got := ProjectDir("/home/u/.claude", "-a-b")
	want := "/home/u/.claude/projects/-a-b"
	if got != want {
		t.Fatalf("ProjectDir = %q, want %q", got, want)
	}
}

func TestNewUUIDFormat(t *testing.T) {
	u, err := NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	// 8-4-4-4-12 hex, version 4.
	if len(u) != 36 || strings.Count(u, "-") != 4 || u[14] != '4' {
		t.Fatalf("NewUUID = %q, no parece uuid v4", u)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run 'TestSlugForCwd|TestProjectDir|TestNewUUIDFormat' -v`
Expected: FAIL (`undefined: SlugForCwd` etc.)

- [ ] **Step 3: Write minimal implementation**

```go
package core

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
)

// transcript.go — operaciones puras sobre los transcripts JSONL de Claude Code.
// Una sesión vive en <cc-home>/projects/<slug>/<uuid>.jsonl. La identidad de la
// sesión es el uuid del nombre de archivo, que DEBE igualar el campo sessionId
// dentro de cada línea. Validado contra CC v2.1.183 (ver spec §08).

// SlugForCwd convierte un cwd absoluto al slug de proyecto de CC: cada '/' -> '-'.
func SlugForCwd(cwd string) string {
	return strings.ReplaceAll(cwd, "/", "-")
}

// ProjectDir devuelve <ccHome>/projects/<slug>.
func ProjectDir(ccHome, slug string) string {
	return filepath.Join(ccHome, "projects", slug)
}

// NewUUID genera un UUID v4 (RFC 4122) usando crypto/rand.
func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("no se pudo generar uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // versión 4
	b[8] = (b[8] & 0x3f) | 0x80 // variante 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run 'TestSlugForCwd|TestProjectDir|TestNewUUIDFormat' -v`
Expected: PASS

- [ ] **Step 5: Commit (requiere autorización)**

```bash
git add internal/core/transcript.go internal/core/transcript_test.go
# PEDIR autorización antes de:
# git commit -m "feat(handoff): slug/paths/uuid base para transcripts"
```

---

## Task 2: Resolver el cc-home de un perfil (CCHome)

**Files:**
- Modify: `internal/core/transcript.go`
- Test: `internal/core/transcript_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestCCHomeProfile(t *testing.T) {
	got, err := CCHome("/cfg/ccp", "emco-cc")
	if err != nil {
		t.Fatal(err)
	}
	want := "/cfg/ccp/profiles/emco-cc/cc-home"
	if got != want {
		t.Fatalf("CCHome(profile) = %q, want %q", got, want)
	}
}

func TestCCHomeDefault(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	got, err := CCHome("/cfg/ccp", "default")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/home/tester/.claude" {
		t.Fatalf("CCHome(default) = %q, want /home/tester/.claude", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run 'TestCCHome' -v`
Expected: FAIL (`undefined: CCHome`)

- [ ] **Step 3: Write minimal implementation** (añadir a `transcript.go`)

```go
import "os" // añadir al import block existente

// CCHome devuelve el CLAUDE_CONFIG_DIR de un perfil. Para 'default' es
// ~/.claude; para el resto, <home>/profiles/<perfil>/cc-home. Espeja la lógica
// de EnvDelta (env.go).
func CCHome(home, profile string) (string, error) {
	if profile == "default" {
		uh, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("no se pudo determinar HOME: %w", err)
		}
		return filepath.Join(uh, ".claude"), nil
	}
	return filepath.Join(home, "profiles", profile, "cc-home"), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run 'TestCCHome' -v`
Expected: PASS

- [ ] **Step 5: Commit (requiere autorización)**

```bash
git add internal/core/transcript.go internal/core/transcript_test.go
# git commit -m "feat(handoff): CCHome resuelve cc-home por perfil"
```

---

## Task 3: Listar sesiones (descubrimiento + título + orden)

**Files:**
- Modify: `internal/core/transcript.go`
- Test: `internal/core/transcript_test.go`

- [ ] **Step 1: Write the failing test**

```go
import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeJSONL crea un transcript mínimo con el uuid y un aiTitle dados.
func writeJSONL(t *testing.T, dir, uuid, title string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, uuid+".jsonl")
	lines := `{"type":"last-prompt","sessionId":"` + uuid + `","leafUuid":"L1"}` + "\n" +
		`{"type":"ai-title","aiTitle":"` + title + `","sessionId":"` + uuid + `"}` + "\n" +
		`{"type":"user","sessionId":"` + uuid + `","cwd":"/repo","uuid":"m1","parentUuid":null}` + "\n"
	if err := os.WriteFile(p, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestListSessionsOrderAndTitle(t *testing.T) {
	cc := t.TempDir()
	slug := "-repo"
	dir := ProjectDir(cc, slug)
	old := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	newr := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	writeJSONL(t, dir, "11111111-1111-4111-8111-111111111111", "Vieja", old)
	writeJSONL(t, dir, "22222222-2222-4222-8222-222222222222", "Nueva", newr)

	got, err := ListSessions(cc, slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Title != "Nueva" || got[1].Title != "Vieja" {
		t.Fatalf("orden incorrecto: %q, %q (debe ser nuevo→viejo)", got[0].Title, got[1].Title)
	}
	if got[0].UUID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("uuid[0] = %q", got[0].UUID)
	}
}

func TestListSessionsEmpty(t *testing.T) {
	cc := t.TempDir()
	got, err := ListSessions(cc, "-noexiste")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0 (carpeta inexistente = lista vacía)", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run 'TestListSessions' -v`
Expected: FAIL (`undefined: ListSessions`, `undefined: SessionInfo`)

- [ ] **Step 3: Write minimal implementation** (añadir a `transcript.go`)

```go
import (
	"bufio"
	"encoding/json"
	"sort"
	"time"
)

// SessionInfo describe un transcript en disco para el picker.
type SessionInfo struct {
	UUID    string
	Path    string
	Title   string // aiTitle (último visto); "" si no hay
	ModTime time.Time
}

// ListSessions escanea <ccHome>/projects/<slug>/*.jsonl y devuelve las sesiones
// ordenadas de más nueva a más vieja (por mtime). Carpeta inexistente => lista
// vacía sin error.
func ListSessions(ccHome, slug string) ([]SessionInfo, error) {
	dir := ProjectDir(ccHome, slug)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("no se pudo leer %s: %w", dir, err)
	}
	var out []SessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		uuid := strings.TrimSuffix(e.Name(), ".jsonl")
		full := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, SessionInfo{
			UUID:    uuid,
			Path:    full,
			Title:   readAITitle(full),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// readAITitle devuelve el último aiTitle del transcript, o "" si no hay.
func readAITitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	title := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // líneas grandes
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		if m["type"] == "ai-title" {
			if t, ok := m["aiTitle"].(string); ok {
				title = t
			}
		}
	}
	return title
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run 'TestListSessions' -v`
Expected: PASS

- [ ] **Step 5: Commit (requiere autorización)**

```bash
git add internal/core/transcript.go internal/core/transcript_test.go
# git commit -m "feat(handoff): ListSessions con título y orden nuevo→viejo"
```

---

## Task 4: Copia forward (mismo uuid, copia cruda + colisión)

**Files:**
- Modify: `internal/core/transcript.go`
- Test: `internal/core/transcript_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestCopyTranscriptSameUUID(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	uuid := "33333333-3333-4333-8333-333333333333"
	writeJSONL(t, src, uuid, "T", time.Now())
	srcPath := filepath.Join(src, uuid+".jsonl")

	dstPath, err := CopyTranscript(srcPath, dst, false)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dstPath) != uuid+".jsonl" {
		t.Fatalf("dst conserva uuid? got %q", dstPath)
	}
	a, _ := os.ReadFile(srcPath)
	b, _ := os.ReadFile(dstPath)
	if string(a) != string(b) {
		t.Fatal("copia no idéntica")
	}
}

func TestCopyTranscriptCollisionDifferent(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	uuid := "44444444-4444-4444-8444-444444444444"
	writeJSONL(t, src, uuid, "nuevo", time.Now())
	// dst ya tiene ese uuid con contenido distinto.
	if err := os.WriteFile(filepath.Join(dst, uuid+".jsonl"), []byte("distinto\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcPath := filepath.Join(src, uuid+".jsonl")
	if _, err := CopyTranscript(srcPath, dst, false); err == nil {
		t.Fatal("esperaba error de colisión sin force")
	}
	if _, err := CopyTranscript(srcPath, dst, true); err != nil {
		t.Fatalf("con force no debería fallar: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run 'TestCopyTranscript' -v`
Expected: FAIL (`undefined: CopyTranscript`)

- [ ] **Step 3: Write minimal implementation** (añadir a `transcript.go`)

```go
import "bytes"

// CopyTranscript copia srcPath al directorio dstDir conservando el nombre
// (mismo uuid). El sessionId interno ya == uuid y el cwd es el mismo repo, así
// que NO se reescribe nada. Si dstDir ya tiene ese archivo: si el contenido es
// idéntico, sobre-escribe; si difiere, error salvo force. Devuelve el path destino.
func CopyTranscript(srcPath, dstDir string, force bool) (string, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("no se pudo leer %s: %w", srcPath, err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", fmt.Errorf("no se pudo crear %s: %w", dstDir, err)
	}
	dstPath := filepath.Join(dstDir, filepath.Base(srcPath))
	if existing, err := os.ReadFile(dstPath); err == nil {
		if !bytes.Equal(existing, data) && !force {
			return "", fmt.Errorf("colisión: %s ya existe con contenido distinto (usa --force)", dstPath)
		}
	}
	if err := os.WriteFile(dstPath, data, 0o644); err != nil {
		return "", fmt.Errorf("no se pudo escribir %s: %w", dstPath, err)
	}
	return dstPath, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run 'TestCopyTranscript' -v`
Expected: PASS

- [ ] **Step 5: Commit (requiere autorización)**

```bash
git add internal/core/transcript.go internal/core/transcript_test.go
# git commit -m "feat(handoff): CopyTranscript con manejo de colisión"
```

---

## Task 5: Reescritura para sesión nueva (sessionId + aiTitle, idempotente, validada)

**Files:**
- Modify: `internal/core/transcript.go`
- Test: `internal/core/transcript_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestRewriteSession(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	old := "55555555-5555-4555-8555-555555555555"
	writeJSONL(t, src, old, "Refactor", time.Now())
	srcPath := filepath.Join(src, old+".jsonl")
	newID := "66666666-6666-4666-8666-666666666666"
	dstPath := filepath.Join(dst, newID+".jsonl")

	if err := RewriteSession(srcPath, dstPath, old, newID, "emco-cc"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dstPath)
	s := string(data)
	if strings.Contains(s, old) {
		t.Fatal("quedó el sessionId viejo en la salida")
	}
	if !strings.Contains(s, newID) {
		t.Fatal("no aparece el sessionId nuevo")
	}
	if !strings.Contains(s, `[de emco-cc] Refactor`) {
		t.Fatal("aiTitle no quedó prefijado con el origen")
	}
	// cwd intacto, árbol de mensajes intacto.
	if !strings.Contains(s, `"cwd":"/repo"`) || !strings.Contains(s, `"uuid":"m1"`) {
		t.Fatal("se alteró cwd o el árbol de mensajes")
	}
	// JSONL válido: cada línea parsea.
	for _, ln := range strings.Split(strings.TrimSpace(s), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(ln), &m) != nil {
			t.Fatalf("línea no es JSON válido: %s", ln)
		}
	}
}

func TestRewriteSessionTitleIdempotent(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	old := "77777777-7777-4777-8777-777777777777"
	// título ya prefijado.
	dir := src
	_ = os.MkdirAll(dir, 0o755)
	line := `{"type":"ai-title","aiTitle":"[de x] Ya","sessionId":"` + old + `"}` + "\n"
	_ = os.WriteFile(filepath.Join(dir, old+".jsonl"), []byte(line), 0o644)
	srcPath := filepath.Join(dir, old+".jsonl")
	dstPath := filepath.Join(dst, "n.jsonl")

	if err := RewriteSession(srcPath, dstPath, old, "n", "emco-cc"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dstPath)
	if strings.Count(string(data), "[de ") != 1 {
		t.Fatalf("prefijo duplicado: %s", data)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run 'TestRewriteSession' -v`
Expected: FAIL (`undefined: RewriteSession`)

- [ ] **Step 3: Write minimal implementation** (añadir a `transcript.go`)

```go
// RewriteSession lee srcPath (JSONL), reescribe cada campo sessionId de oldID a
// newID, y prepende "[de <fromLabel>] " al aiTitle (idempotente: no duplica si
// ya empieza con "[de "). NO toca cwd, el árbol uuid/parentUuid/leafUuid,
// messageId, timestamps ni el contenido de los mensajes. Escribe en dstPath y
// valida que el resultado no contenga oldID en sessionId y sea JSONL válido.
func RewriteSession(srcPath, dstPath, oldID, newID, fromLabel string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("no se pudo leer %s: %w", srcPath, err)
	}
	defer f.Close()

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // no escapar <,>,& : mantener el JSON natural

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		raw := sc.Bytes()
		if len(strings.TrimSpace(string(raw))) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("línea JSONL inválida en %s: %w", srcPath, err)
		}
		if sid, ok := m["sessionId"].(string); ok && sid == oldID {
			m["sessionId"] = newID
		}
		if m["type"] == "ai-title" {
			if t, ok := m["aiTitle"].(string); ok && !strings.HasPrefix(t, "[de ") {
				m["aiTitle"] = "[de " + fromLabel + "] " + t
			}
		}
		if err := enc.Encode(m); err != nil { // Encode añade '\n'
			return fmt.Errorf("no se pudo serializar línea: %w", err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("error leyendo %s: %w", srcPath, err)
	}

	// Validación: cero oldID en sessionId, JSONL válido.
	out := buf.Bytes()
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			return fmt.Errorf("salida JSONL inválida: %w", err)
		}
		if sid, ok := m["sessionId"].(string); ok && sid == oldID {
			return fmt.Errorf("reescritura incompleta: quedó sessionId viejo")
		}
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", filepath.Dir(dstPath), err)
	}
	if err := os.WriteFile(dstPath, out, 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", dstPath, err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run 'TestRewriteSession' -v`
Expected: PASS

- [ ] **Step 5: Commit (requiere autorización)**

```bash
git add internal/core/transcript.go internal/core/transcript_test.go
# git commit -m "feat(handoff): RewriteSession reescribe sessionId + aiTitle, validado"
```

---

## Task 6: Store del marcador (handoffs.yaml)

**Files:**
- Create: `internal/core/handoff_store.go`
- Test: `internal/core/handoff_store_test.go`

- [ ] **Step 1: Write the failing test**

```go
package core

import (
	"testing"
)

func TestHandoffsRoundTrip(t *testing.T) {
	home := t.TempDir()
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if h.Version != 1 || h.Active != nil || len(h.Archived) != 0 {
		t.Fatalf("vacío esperado, got %+v", h)
	}
	h.Active = &Marker{
		Session: "abc", Slug: "-r", Cwd: "/r",
		From: "personal-cc", To: "emco-cc", Title: "T", Since: "2026-06-19T00:00:00Z",
	}
	if err := SaveHandoffs(home, h); err != nil {
		t.Fatal(err)
	}
	h2, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if h2.Active == nil || h2.Active.To != "emco-cc" || h2.Active.Session != "abc" {
		t.Fatalf("no round-tripeó: %+v", h2.Active)
	}
}

func TestHandoffsMissingIsEmpty(t *testing.T) {
	h, err := LoadHandoffs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if h.Active != nil {
		t.Fatal("archivo ausente debe dar Active nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run 'TestHandoffs' -v`
Expected: FAIL (`undefined: LoadHandoffs`)

- [ ] **Step 3: Write minimal implementation**

```go
package core

import (
	"fmt"
	"os"
	"path/filepath"

	yaml "github.com/goccy/go-yaml"
)

// handoff_store.go — persistencia de ~/.config/ccp/handoffs.yaml. Estado de
// runtime (NO config): vive aparte de ccp.yaml para no ensuciar su diff ni
// arriesgar su version. Escritura atómica tmp+rename bajo el mismo flock que
// store.go (acquireLock). Si el archivo falta o está corrupto, degradación
// suave: LoadHandoffs devuelve un Handoffs vacío.

// Marker es el handoff en vuelo (0 o 1 a la vez en v1).
type Marker struct {
	Session string `yaml:"session"`
	Slug    string `yaml:"slug"`
	Cwd     string `yaml:"cwd"`
	From    string `yaml:"from"`
	To      string `yaml:"to"`
	Title   string `yaml:"title,omitempty"`
	Since   string `yaml:"since"`
}

// ArchivedMarker es un handoff terminado (historial para `handoff list`).
type ArchivedMarker struct {
	Session    string `yaml:"session"`
	From       string `yaml:"from"`
	To         string `yaml:"to"`
	ReturnedAs string `yaml:"returned_as"`
	Since      string `yaml:"since"`
	Ended      string `yaml:"ended"`
}

// Handoffs es el modelo en memoria de handoffs.yaml.
type Handoffs struct {
	Version  int              `yaml:"version"`
	Active   *Marker          `yaml:"active,omitempty"`
	Archived []ArchivedMarker `yaml:"archived,omitempty"`
}

func handoffsPath(home string) string { return filepath.Join(home, "handoffs.yaml") }

// LoadHandoffs lee handoffs.yaml. Ausente o corrupto => Handoffs vacío v1 (sin
// error: degradación suave; el rastro se pierde pero los jsonl siguen en disco).
func LoadHandoffs(home string) (*Handoffs, error) {
	data, err := os.ReadFile(handoffsPath(home))
	if err != nil {
		return &Handoffs{Version: 1}, nil
	}
	h := &Handoffs{}
	if err := yaml.Unmarshal(data, h); err != nil {
		return &Handoffs{Version: 1}, nil
	}
	if h.Version == 0 {
		h.Version = 1
	}
	return h, nil
}

// SaveHandoffs escribe handoffs.yaml atómicamente bajo flock (reusa acquireLock).
func SaveHandoffs(home string, h *Handoffs) error {
	if h.Version == 0 {
		h.Version = 1
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", home, err)
	}
	out, err := yaml.Marshal(h)
	if err != nil {
		return fmt.Errorf("no se pudo serializar handoffs.yaml: %w", err)
	}
	unlock, err := acquireLock(home)
	if err != nil {
		return err
	}
	defer unlock()

	path := handoffsPath(home)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("no se pudo renombrar %s -> %s: %w", tmp, path, err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run 'TestHandoffs' -v`
Expected: PASS

- [ ] **Step 5: Commit (requiere autorización)**

```bash
git add internal/core/handoff_store.go internal/core/handoff_store_test.go
# git commit -m "feat(handoff): store atómico de handoffs.yaml (marcador)"
```

---

## Task 7: Orquestación forward (HandoffForward → emit)

**Files:**
- Create: `internal/core/handoff.go`
- Test: `internal/core/handoff_test.go`

- [ ] **Step 1: Write the failing test**

```go
package core

import (
	"strings"
	"testing"
	"time"
)

// seedProfiles crea un ccp.yaml con dos perfiles official y sus cc-home.
func seedHandoffEnv(t *testing.T, home string) {
	t.Helper()
	cfg := &Config{
		Version:  SchemaVersion,
		Profiles: map[string]Profile{"personal-cc": {Type: "official"}, "emco-cc": {Type: "official"}},
	}
	if err := Save(home, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestHandoffForwardCopiesAndMarks(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	slug := SlugForCwd(cwd)
	uuid := "88888888-8888-4888-8888-888888888888"
	srcDir := ProjectDir(home+"/profiles/personal-cc/cc-home", slug)
	writeJSONL(t, srcDir, uuid, "Trabajo", time.Now())

	emit, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Emit debe ser eval-able: env del destino + CCP_RESUME_ID.
	if !strings.Contains(emit, "CLAUDE_CONFIG_DIR=") || !strings.Contains(emit, "emco-cc/cc-home") {
		t.Fatalf("emit sin env del destino: %s", emit)
	}
	if !strings.Contains(emit, "CCP_RESUME_ID="+uuid) {
		t.Fatalf("emit sin CCP_RESUME_ID correcto: %s", emit)
	}
	// Copió el jsonl al destino con el mismo uuid.
	dstDir := ProjectDir(home+"/profiles/emco-cc/cc-home", slug)
	if _, err := os.Stat(dstDir + "/" + uuid + ".jsonl"); err != nil {
		t.Fatalf("no copió al destino: %v", err)
	}
	// Escribió el marcador activo.
	h, _ := LoadHandoffs(home)
	if h.Active == nil || h.Active.To != "emco-cc" || h.Active.Session != uuid {
		t.Fatalf("marcador activo incorrecto: %+v", h.Active)
	}
}

func TestHandoffForwardBlocksWhenActive(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	_ = SaveHandoffs(home, &Handoffs{Version: 1, Active: &Marker{Session: "x", From: "a", To: "b"}})
	if _, err := HandoffForward(home, "personal-cc", "emco-cc", "/repo", "u", true, time.Now()); err == nil {
		t.Fatal("esperaba error 1-nivel con handoff activo")
	}
}

func TestHandoffForwardSameProfile(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	if _, err := HandoffForward(home, "emco-cc", "emco-cc", "/repo", "u", true, time.Now()); err == nil {
		t.Fatal("esperaba error destino==origen")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run 'TestHandoffForward' -v`
Expected: FAIL (`undefined: HandoffForward`)

- [ ] **Step 3: Write minimal implementation**

```go
package core

import (
	"fmt"
	"time"
)

// handoff.go — orquesta el round-trip. Devuelve el string EMIT (eval-able):
// el delta de env del perfil objetivo + una línea CCP_RESUME_ID=<uuid>. La
// shell function hace `( eval "$emit"; claude --resume "$CCP_RESUME_ID" )`.

// HandoffForward valida, copia la sesión origen→destino (mismo uuid), escribe el
// marcador activo y devuelve el emit con el env del DESTINO. Regla 1-nivel:
// falla si ya hay un handoff activo.
func HandoffForward(home, from, to, cwd, sessionUUID string, writeMarker bool, now time.Time) (string, error) {
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
	h, err := LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	if h.Active != nil {
		return "", fmt.Errorf("ya hay un handoff activo (%s → %s); termínalo con `ccp handoff end`", h.Active.From, h.Active.To)
	}

	slug := SlugForCwd(cwd)
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
		h.Active = &Marker{
			Session: sessionUUID, Slug: slug, Cwd: cwd,
			From: from, To: to,
			Title: readAITitle(srcPath),
			Since: now.UTC().Format(time.RFC3339),
		}
		if err := SaveHandoffs(home, h); err != nil {
			return "", err
		}
	}
	return EnvDelta(home, to, cfg) + "CCP_RESUME_ID=" + sessionUUID + "\n", nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run 'TestHandoffForward' -v`
Expected: PASS

- [ ] **Step 5: Commit (requiere autorización)**

```bash
git add internal/core/handoff.go internal/core/handoff_test.go
# git commit -m "feat(handoff): HandoffForward copia + marca + emite env destino"
```

---

## Task 8: Orquestación return (HandoffEnd → back-sync + emit)

**Files:**
- Modify: `internal/core/handoff.go`
- Test: `internal/core/handoff_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestHandoffEndBackSyncsAsNewSession(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	slug := SlugForCwd(cwd)
	uuid := "99999999-9999-4999-8999-999999999999"

	// Estado tras un forward: marcador activo + jsonl (crecido) en el destino.
	dstDir := ProjectDir(home+"/profiles/emco-cc/cc-home", slug)
	writeJSONL(t, dstDir, uuid, "Refactor", time.Now())
	_ = SaveHandoffs(home, &Handoffs{Version: 1, Active: &Marker{
		Session: uuid, Slug: slug, Cwd: cwd, From: "personal-cc", To: "emco-cc",
		Title: "Refactor", Since: "2026-06-19T00:00:00Z",
	}})

	emit, err := HandoffEnd(home, cwd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Emit lleva el env del ORIGEN + el uuid NUEVO.
	if !strings.Contains(emit, "personal-cc/cc-home") {
		t.Fatalf("emit sin env del origen: %s", emit)
	}
	// Marcador archivado, active vacío.
	h, _ := LoadHandoffs(home)
	if h.Active != nil || len(h.Archived) != 1 {
		t.Fatalf("marcador no archivado: %+v", h)
	}
	newID := h.Archived[0].ReturnedAs
	if newID == "" || newID == uuid {
		t.Fatalf("returned_as inválido: %q", newID)
	}
	if !strings.Contains(emit, "CCP_RESUME_ID="+newID) {
		t.Fatalf("emit no resume el uuid nuevo: %s", emit)
	}
	// La sesión nueva existe en el ORIGEN; la vieja del origen no se tocó.
	srcNew := ProjectDir(home+"/profiles/personal-cc/cc-home", slug) + "/" + newID + ".jsonl"
	if _, err := os.Stat(srcNew); err != nil {
		t.Fatalf("no creó la sesión nueva en origen: %v", err)
	}
	data, _ := os.ReadFile(srcNew)
	if !strings.Contains(string(data), "[de emco-cc] Refactor") {
		t.Fatal("título no marca el origen")
	}
}

func TestHandoffEndNoActive(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	if _, err := HandoffEnd(home, "/repo", time.Now()); err == nil {
		t.Fatal("esperaba error sin handoff activo")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run 'TestHandoffEnd' -v`
Expected: FAIL (`undefined: HandoffEnd`)

- [ ] **Step 3: Write minimal implementation** (añadir a `handoff.go`)

```go
// HandoffEnd toma el marcador activo, hace back-sync del transcript (que creció
// en el destino) hacia el origen como una sesión NUEVA (uuid nuevo, sessionId
// reescrito, aiTitle prefijado con el origen), archiva el marcador y devuelve el
// emit con el env del ORIGEN + el uuid nuevo. No destructivo: ni el original del
// origen ni el del destino se borran.
func HandoffEnd(home, cwd string, now time.Time) (string, error) {
	cfg, err := Load(home)
	if err != nil {
		return "", err
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	if h.Active == nil {
		return "", fmt.Errorf("no hay handoff activo que terminar")
	}
	m := h.Active

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
		Session: m.Session, From: m.From, To: m.To,
		ReturnedAs: newID, Since: m.Since, Ended: now.UTC().Format(time.RFC3339),
	})
	h.Active = nil
	if err := SaveHandoffs(home, h); err != nil {
		return "", err
	}
	return EnvDelta(home, m.From, cfg) + "CCP_RESUME_ID=" + newID + "\n", nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run 'TestHandoffEnd' -v`
Expected: PASS

- [ ] **Step 5: Commit (requiere autorización)**

```bash
git add internal/core/handoff.go internal/core/handoff_test.go
# git commit -m "feat(handoff): HandoffEnd back-sync como sesión nueva + archiva marcador"
```

---

## Task 9: i18n — strings de status/list/errores

**Files:**
- Modify: `internal/core/i18n/catalog_cli.go`
- Test: (cubierto por `internal/core/i18n/catalog_test.go` que valida paridad en/es)

- [ ] **Step 1: Verificar el formato del catálogo**

Run: `grep -n 'cli.backup.unknown_sub' internal/core/i18n/catalog_cli.go`
Expected: ver una entrada con claves `en:` y `es:` — replicar ese formato exacto.

- [ ] **Step 2: Añadir las claves nuevas** (en `catalog_cli.go`, junto a las demás `cli.*`)

Añadir (adaptar la sintaxis al map exacto del archivo — pares en/es):

```go
"cli.handoff.shell_only":      {en: "`ccp handoff` solo funciona vía la función shell de ccp (reinstala con `ccp install`).", es: "`ccp handoff` solo funciona vía la función shell de ccp (reinstala con `ccp install`)."},
"cli.handoff.no_active":       {en: "No active handoff.", es: "Sin handoff activo."},
"cli.handoff.status_active":   {en: "Active handoff: %s → %s · session %s · since %s", es: "Handoff activo: %s → %s · sesión %s · desde %s"},
"cli.handoff.list_header":     {en: "Handoff history:", es: "Historial de handoffs:"},
"cli.handoff.list_row":        {en: "  %s → %s · %s → %s · ended %s", es: "  %s → %s · %s → %s · terminó %s"},
"cli.handoff.list_empty":      {en: "No handoffs recorded.", es: "Sin handoffs registrados."},
"cli.handoff.unknown_sub":     {en: "unknown handoff subcommand: %s", es: "subcomando de handoff desconocido: %s"},
```

> **Nota:** revisar la estructura real del `var` en `catalog_cli.go` (puede ser `map[string]entry{en,es string}`). Usar EXACTAMENTE esa forma. Si `catalog_test.go` exige que toda clave exista en ambos idiomas, estas ya lo cumplen.

- [ ] **Step 3: Run test to verify catalog parity**

Run: `go test ./internal/core/i18n/ -v`
Expected: PASS (paridad en/es completa)

- [ ] **Step 4: Commit (requiere autorización)**

```bash
git add internal/core/i18n/catalog_cli.go
# git commit -m "feat(handoff): strings i18n de handoff status/list/errores"
```

---

## Task 10: CLI dispatch (`handoff` status/list/shell-only + `_handoff`/`_handoff-end`)

**Files:**
- Create: `internal/cli/handoff.go`
- Modify: `internal/cli/cli.go` (switch de `Dispatch`)
- Test: `internal/cli/handoff_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

func TestHandoffStatusEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	var out, errb bytes.Buffer
	code := Dispatch([]string{"handoff", "status"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (sin activo)", code)
	}
	if !strings.Contains(out.String()+errb.String(), "activo") {
		t.Fatalf("salida inesperada: %q / %q", out.String(), errb.String())
	}
}

func TestHandoffForwardShellOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	var out, errb bytes.Buffer
	// `ccp handoff <perfil>` directo al binario (sin función shell) = shell-only.
	code := Dispatch([]string{"handoff", "emco-cc"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestHandoffEmitForward(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	cfg := &core.Config{Version: core.SchemaVersion,
		Profiles: map[string]core.Profile{"personal-cc": {Type: "official"}, "emco-cc": {Type: "official"}}}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	cwd := "/repo"
	slug := core.SlugForCwd(cwd)
	uuid := "abababab-abab-4bab-8bab-abababababab"
	dir := core.ProjectDir(home+"/profiles/personal-cc/cc-home", slug)
	_ = writeJSONLForTest(t, dir, uuid) // helper local (ver Step 3)

	t.Setenv("CCP_PROFILE", "personal-cc")
	var out, errb bytes.Buffer
	// _handoff <pwd> <to> --session <uuid>
	code := Dispatch([]string{"_handoff", cwd, "emco-cc", "--session", uuid}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "CCP_RESUME_ID="+uuid) {
		t.Fatalf("emit sin resume id: %s", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run 'TestHandoff' -v`
Expected: FAIL (`undefined: writeJSONLForTest`, dispatch cases ausentes)

- [ ] **Step 3: Write the implementation**

Crear `internal/cli/handoff.go`:

```go
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// cmdHandoff maneja la cara LEÍBLE de `ccp handoff`: status, list, y el resto
// (forward/end) que son shell-only (necesitan la función shell para lanzar
// claude con el env aplicado).
func cmdHandoff(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := currentLang()
	var sub string
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "status":
		h, _ := core.LoadHandoffs(home)
		if h.Active == nil {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.no_active"))
			return 1
		}
		a := h.Active
		fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.status_active", a.From, a.To, a.Session, a.Since))
		return 0
	case "list":
		h, _ := core.LoadHandoffs(home)
		if len(h.Archived) == 0 && h.Active == nil {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_empty"))
			return 0
		}
		fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_header"))
		if h.Active != nil {
			a := h.Active
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.status_active", a.From, a.To, a.Session, a.Since))
		}
		for _, a := range h.Archived {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_row", a.From, a.To, a.Session, a.ReturnedAs, a.Ended))
		}
		return 0
	default:
		// forward / end / no-arg: shell-only.
		fmt.Fprintln(stderr, i18n.T(lang, "cli.handoff.shell_only"))
		return 1
	}
}

// activeProfile devuelve el perfil activo: CCP_PROFILE si está, si no resuelto
// por reglas desde cwd.
func activeProfile(home, cwd string) string {
	if p := os.Getenv("CCP_PROFILE"); p != "" {
		return p
	}
	cfg, err := core.Load(home)
	if err != nil {
		return "default"
	}
	return core.Resolve(cwd, cfg.Rules)
}

// cmdHandoffEmit implementa `_handoff <pwd> [to] [--session uuid] [--no-marker]
// [--force]`. Hace la lógica (incluyendo pickers TUI si falta to/session y hay
// TTY) y emite a stdout el delta eval-able. La TUI se renderiza en stderr/tty
// para no contaminar stdout.
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
	var to, session string
	marker := true
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--session" && i+1 < len(rest):
			session = rest[i+1]
			i++
		case a == "--no-marker":
			marker = false
		case a == "--force":
			// reservado: la colisión se maneja en CopyTranscript; flag se
			// propaga en una iteración futura si se requiere.
		case !strings.HasPrefix(a, "-") && to == "":
			to = a
		}
	}
	from := activeProfile(home, cwd)

	// Pickers TUI cuando falta info y hay TTY (ver Task 12). Sin TTY: error.
	if to == "" {
		picked, err := pickHandoffProfile(home, from, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		to = picked
	}
	if session == "" {
		picked, err := pickHandoffSession(home, from, cwd, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		session = picked
	}

	emit, err := core.HandoffForward(home, from, to, cwd, session, marker, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, emit)
	return 0
}

// cmdHandoffEndEmit implementa `_handoff-end <pwd>`.
func cmdHandoffEndEmit(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if len(args) < 1 {
		fmt.Fprintln(stderr, "[error] _handoff-end requiere <pwd>")
		return 1
	}
	emit, err := core.HandoffEnd(home, args[0], time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, emit)
	return 0
}

// readLineFallback lee una línea de stdin (fallback no-TTY si los pickers no
// aplican). No usado en el happy-path TUI; presente para tests deterministas.
func readLineFallback(r io.Reader) string {
	sc := bufio.NewScanner(r)
	if sc.Scan() {
		return strings.TrimSpace(sc.Text())
	}
	return ""
}
```

> **Pickers (Task 12):** declarar por ahora stubs en `internal/cli/handoff.go` para que compile y los tests pasen vía flags (que saltan los pickers):
>
> ```go
> // pickHandoffProfile / pickHandoffSession: cuando no hay TTY o no se
> // implementan aún, devuelven error pidiendo los flags. Task 12 los reemplaza
> // por la TUI.
> func pickHandoffProfile(home, from string, w io.Writer) (string, error) {
> 	return "", fmt.Errorf("falta el perfil destino (usa `ccp handoff <perfil>`)")
> }
> func pickHandoffSession(home, from, cwd string, w io.Writer) (string, error) {
> 	return "", fmt.Errorf("falta la sesión (usa `--session <uuid>`)")
> }
> ```

Añadir el helper de test en `internal/cli/handoff_test.go`:

```go
func writeJSONLForTest(t *testing.T, dir, uuid string) error {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	line := `{"type":"ai-title","aiTitle":"T","sessionId":"` + uuid + `"}` + "\n" +
		`{"type":"user","sessionId":"` + uuid + `","cwd":"/repo"}` + "\n"
	return os.WriteFile(dir+"/"+uuid+".jsonl", []byte(line), 0o644)
}
```
(añadir `import "os"` al test).

Modificar `internal/cli/cli.go` — en el `switch cmd` de `Dispatch`, añadir:

```go
	case "handoff":
		return cmdHandoff(rest, stdout, stderr)
	case "_handoff":
		return cmdHandoffEmit(rest, stdout, stderr)
	case "_handoff-end":
		return cmdHandoffEndEmit(rest, stdout, stderr)
```
(colocar junto a la frontera binario↔shell, antes del bloque `use/default/off/on/run`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run 'TestHandoff' -v`
Expected: PASS

- [ ] **Step 5: Build + vet**

Run: `go build ./... && go vet ./...`
Expected: sin errores.

- [ ] **Step 6: Commit (requiere autorización)**

```bash
git add internal/cli/handoff.go internal/cli/handoff_test.go internal/cli/cli.go
# git commit -m "feat(handoff): dispatch CLI handoff/_handoff/_handoff-end"
```

---

## Task 11: Shell function + completion (shellinit.go + oráculo bash + golden)

**Files:**
- Modify: `internal/core/shellinit.go`
- Modify: `legacy/bin/ccp` (oráculo)
- Regenerate: `testdata/golden/basic/expected/*`
- Test: `internal/golden/parity_test.go`, `internal/core/shellinit_test.go`

- [ ] **Step 1: Editar el oráculo bash** `legacy/bin/ccp`

En `_print_shell_init()` (alrededor de la línea 541), dentro del `case "$1" in` de la función `ccp()`, añadir el caso `handoff)` ANTES de `*) command ccp "$@" ;;`:

```bash
    handoff)
      shift
      case "$1" in
        end)          shift; out=$(command ccp _handoff-end "$PWD" "$@") || return ;;
        status|list)  command ccp handoff "$@"; return ;;
        *)            out=$(command ccp _handoff "$PWD" "$@") || return ;;
      esac
      ( eval "$out"; claude --resume "$CCP_RESUME_ID" ) ;;
```

En las DOS listas `top=` de completion del oráculo (bash ~línea 615 y zsh ~línea 641), añadir `handoff` al final de la lista de palabras.

- [ ] **Step 2: Editar `internal/core/shellinit.go` para que sea BYTE-IDÉNTICO**

En la constante `ShellInit`, añadir el mismo `case handoff)` antes de `*) command ccp "$@" ;;`:

```go
    handoff)
      shift
      case "$1" in
        end)          shift; out=$(command ccp _handoff-end "$PWD" "$@") || return ;;
        status|list)  command ccp handoff "$@"; return ;;
        *)            out=$(command ccp _handoff "$PWD" "$@") || return ;;
      esac
      ( eval "$out"; claude --resume "$CCP_RESUME_ID" ) ;;
```

En `CompletionBash` y `CompletionZsh`, añadir ` handoff` al final de la lista `top` (string en bash, array en zsh). Mantener el formato exacto (espaciado).

- [ ] **Step 3: Regenerar el golden desde el oráculo**

Run:
```bash
bash testdata/golden/capture.sh
```
Expected: actualiza `testdata/golden/basic/expected/completion-shellinit.out`, `completion-bash.out`, `completion-zsh.out`.

- [ ] **Step 4: Verificar paridad Go == bash**

Run: `go test ./internal/golden/ -v`
Expected: PASS (el binario Go emite exactamente lo capturado del oráculo).

- [ ] **Step 5: Verificar el oráculo bash sigue verde**

Run: `bash legacy/tests/run.sh && bash testdata/golden/capture.sh --check`
Expected: suite del oráculo PASA; `--check` confirma que el golden reproduce el oráculo.

- [ ] **Step 6: shellinit_test.go**

Run: `go test ./internal/core/ -run 'ShellInit|Completion' -v`
Expected: PASS. Si hay un test que asserta substrings del shell init, añadir un assert de que contiene `handoff)` y `_handoff-end`.

- [ ] **Step 7: Commit (requiere autorización)**

```bash
git add internal/core/shellinit.go legacy/bin/ccp testdata/golden/basic/expected internal/core/shellinit_test.go
# git commit -m "feat(handoff): shell function handoff + completion + golden regen"
```

---

## Task 12: TUI pickers (perfil + sesión + confirm)

**Files:**
- Create: `internal/tui/handoff.go`
- Modify: `internal/cli/handoff.go` (reemplazar los stubs `pickHandoffProfile`/`pickHandoffSession` por llamadas a la TUI)
- Test: `internal/tui/handoff_test.go` (lógica de armado de opciones, no el render)

- [ ] **Step 1: Inspeccionar el patrón huh existente**

Run: `sed -n '1,60p' internal/tui/forms.go`
Expected: ver cómo se construye un `huh.NewSelect[...]` y se corre (`.Run()` / programa bubbletea). Replicar ese patrón y el manejo de TTY.

- [ ] **Step 2: Write the failing test** (`internal/tui/handoff_test.go`)

Testear la función PURA que arma las opciones (sin render):

```go
package tui

import (
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

func TestHandoffProfileOptionsExcludesActive(t *testing.T) {
	cfg := &core.Config{Profiles: map[string]core.Profile{
		"personal-cc": {Type: "official"}, "emco-cc": {Type: "official"},
	}}
	opts := HandoffProfileOptions(cfg, "personal-cc")
	for _, o := range opts {
		if o == "personal-cc" {
			t.Fatal("el perfil activo no debe aparecer como destino")
		}
	}
	if len(opts) != 1 || opts[0] != "emco-cc" {
		t.Fatalf("opciones = %v, want [emco-cc]", opts)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestHandoffProfileOptions' -v`
Expected: FAIL (`undefined: HandoffProfileOptions`)

- [ ] **Step 4: Write implementation** (`internal/tui/handoff.go`)

```go
package tui

import (
	"fmt"
	"io"
	"sort"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// HandoffProfileOptions devuelve los nombres de perfil candidatos a destino
// (todos menos el activo, incluyendo 'default'), ordenados.
func HandoffProfileOptions(cfg *core.Config, active string) []string {
	var out []string
	for name := range cfg.Profiles {
		if name != active {
			out = append(out, name)
		}
	}
	if active != "default" {
		out = append(out, "default")
	}
	sort.Strings(out)
	return out
}

// RunHandoffProfilePicker muestra un select huh (en w/tty) y devuelve el perfil
// elegido. Sin TTY devuelve error (el caller debe exigir el flag).
func RunHandoffProfilePicker(cfg *core.Config, active string, w io.Writer) (string, error) {
	opts := HandoffProfileOptions(cfg, active)
	if len(opts) == 0 {
		return "", fmt.Errorf("no hay otros perfiles a los que hacer handoff")
	}
	// ... construir huh.NewSelect[string] con opts, render en w/tty, devolver
	// la selección. Seguir el patrón de forms.go. Si no hay TTY -> error.
	return runSelect("Perfil destino (tokens prestados de):", opts, w)
}

// RunHandoffSessionPicker lista las sesiones del cwd en el perfil origen y
// muestra un select con "fecha · título · uuid". Devuelve el uuid elegido.
func RunHandoffSessionPicker(home, from, cwd string, w io.Writer) (string, error) {
	cc, err := core.CCHome(home, from)
	if err != nil {
		return "", err
	}
	sess, err := core.ListSessions(cc, core.SlugForCwd(cwd))
	if err != nil {
		return "", err
	}
	if len(sess) == 0 {
		return "", fmt.Errorf("no hay sesiones para este proyecto en %s", from)
	}
	labels := make([]string, len(sess))
	ids := make([]string, len(sess))
	for i, s := range sess {
		labels[i] = fmt.Sprintf("%s · %s · %s", s.ModTime.Format("2006-01-02 15:04"), s.Title, s.UUID[:8])
		ids[i] = s.UUID
	}
	choice, err := runSelectIndexed("Sesión a continuar:", labels, w)
	if err != nil {
		return "", err
	}
	return ids[choice], nil
}
```

> `runSelect`/`runSelectIndexed` son helpers a implementar siguiendo `forms.go` (huh select sobre /dev/tty; error si no hay TTY). Si `forms.go` ya tiene un helper de select reutilizable, usar ese en lugar de crear nuevos.

- [ ] **Step 5: Reemplazar los stubs en `internal/cli/handoff.go`**

```go
func pickHandoffProfile(home, from string, w io.Writer) (string, error) {
	cfg, err := core.Load(home)
	if err != nil {
		return "", err
	}
	return tui.RunHandoffProfilePicker(cfg, from, w)
}
func pickHandoffSession(home, from, cwd string, w io.Writer) (string, error) {
	return tui.RunHandoffSessionPicker(home, from, cwd, w)
}
```
(añadir `import "github.com/JoseAFlores777/ccp/internal/tui"` a `internal/cli/handoff.go`.)

- [ ] **Step 6: Run test + build**

Run: `go test ./internal/tui/ -run 'TestHandoff' -v && go build ./...`
Expected: PASS, compila.

- [ ] **Step 7: Commit (requiere autorización)**

```bash
git add internal/tui/handoff.go internal/tui/handoff_test.go internal/cli/handoff.go
# git commit -m "feat(handoff): pickers TUI de perfil y sesión"
```

---

## Task 13: Test eval-effect (zsh + bash) del emit

**Files:**
- Create/Modify: `internal/core/handoff_eval_test.go`

- [ ] **Step 1: Inspeccionar el test eval-effect existente de env**

Run: `grep -n 'bash\|zsh\|exec.Command\|eval' internal/core/env_test.go`
Expected: ver cómo `env_test.go` corre `eval "$(...)"` en bash y zsh y verifica las variables resultantes. Replicar ese arnés.

- [ ] **Step 2: Write the test**

```go
// Verifica que el emit de HandoffForward, al ser evaluado por bash/zsh, exporta
// CLAUDE_CONFIG_DIR del destino y define CCP_RESUME_ID. Skip si no hay el shell.
func TestHandoffEmitEvalEffect(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		sh := sh
		t.Run(sh, func(t *testing.T) {
			if _, err := exec.LookPath(sh); err != nil {
				t.Skipf("%s no disponible", sh)
			}
			home := t.TempDir()
			seedHandoffEnv(t, home)
			cwd := "/repo"
			uuid := "cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd"
			writeJSONL(t, ProjectDir(home+"/profiles/personal-cc/cc-home", SlugForCwd(cwd)), uuid, "T", time.Now())
			emit, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, true, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			script := emit + "\necho \"CCD=$CLAUDE_CONFIG_DIR\"\necho \"RID=$CCP_RESUME_ID\"\n"
			out, err := exec.Command(sh, "-c", script).CombinedOutput()
			if err != nil {
				t.Fatalf("%s: %v\n%s", sh, err, out)
			}
			s := string(out)
			if !strings.Contains(s, "emco-cc/cc-home") || !strings.Contains(s, "RID="+uuid) {
				t.Fatalf("%s eval no exportó lo esperado:\n%s", sh, s)
			}
		})
	}
}
```
(añadir imports `os/exec`.)

- [ ] **Step 3: Run test**

Run: `go test ./internal/core/ -run 'TestHandoffEmitEvalEffect' -v`
Expected: PASS (o SKIP si falta el shell).

- [ ] **Step 4: Commit (requiere autorización)**

```bash
git add internal/core/handoff_eval_test.go
# git commit -m "test(handoff): eval-effect del emit en bash y zsh"
```

---

## Task 14: Verificación final (gates completos)

**Files:** ninguno (solo verificación)

- [ ] **Step 1: Suite completa**

Run: `go test ./...`
Expected: TODO PASS.

- [ ] **Step 2: Gates de formato/lint/vet**

Run:
```bash
gofmt -l internal cmd        # debe imprimir NADA
go vet ./...
golangci-lint run
```
Expected: limpio.

- [ ] **Step 3: Gate del oráculo + golden**

Run:
```bash
bash legacy/tests/run.sh
bash testdata/golden/capture.sh --check
```
Expected: PASS.

- [ ] **Step 4: Smoke manual (opcional, requiere ccp instalado)**

```bash
go build -o /tmp/ccp ./cmd/ccp
/tmp/ccp handoff status        # "Sin handoff activo." exit 1
/tmp/ccp handoff list          # "Sin handoffs registrados." exit 0
/tmp/ccp handoff emco-cc       # mensaje shell-only, exit 1
```

- [ ] **Step 5: Commit final (requiere autorización)**

```bash
# Si quedaron cambios de formato:
# git add -A && git commit -m "chore(handoff): formato y verificación final"
```

---

## Self-review (rellenado por el autor del plan)

**1. Spec coverage:**
- §03 superficie de comandos → Task 10 (dispatch) + Task 11 (shell function/completion). ✓
- §04 marcador handoffs.yaml → Task 6. ✓
- §05 flujo forward (pre-chequeos, copia, marcador, emit, orden transaccional) → Task 7. ✓
- §06 flujo return + reescritura (uuid nuevo, sessionId, aiTitle idempotente, no-destructivo, validación) → Task 5 (reescritura) + Task 8 (orquestación). ✓
- §07 errores/casos borde → Tasks 7/8 (activo, destino==origen, sin activo, jsonl ausente, colisión) + Task 10 (shell-only, status exit codes). Nota: el aviso del hook autocheck por handoff activo es "v1 opcional" en el spec; NO se implementa aquí (documentado como fuera de scope). ✓
- §08 formato CC → respetado por Task 5 (solo sessionId + aiTitle; cwd/árbol intactos). ✓
- §09 pruebas (golden/parity, unit, eval-effect, e2e) → Tasks 11/1-8/13. El e2e binario completo está cubierto por Task 7+8 a nivel core + Task 10 a nivel cli emit. ✓
- §10 ciclo → cubierto end-to-end por Tasks 7+8. ✓

**2. Placeholder scan:** sin TBD/TODO con código faltante. Los pickers TUI tienen stubs explícitos en Task 10 y se completan en Task 12 (orden intencional: el backbone por flags queda testeado antes de la TUI).

**3. Type consistency:** `HandoffForward`/`HandoffEnd` devuelven `(string, error)` (el emit) en core y se consumen igual en cli (Tasks 7/8/10). `Marker`/`Handoffs`/`SessionInfo` consistentes entre Tasks 3/6/7/8. `CCHome`/`SlugForCwd`/`ProjectDir`/`ListSessions`/`CopyTranscript`/`RewriteSession`/`NewUUID` con firmas idénticas donde se referencian.

**Caveat de implementación a verificar en ejecución:** la sintaxis EXACTA del catálogo i18n (`catalog_cli.go`) y del helper de select de `forms.go` deben inspeccionarse en su archivo real (Tasks 9 Step 1, 12 Step 1) antes de escribir — el plan asume el patrón pero el detalle del struct/func se confirma en sitio.

---

v0.1 — 2026-06-19
