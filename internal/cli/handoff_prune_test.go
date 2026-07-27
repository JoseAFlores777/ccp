package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// handoff_prune_test.go — los dos subcomandos de solo-lectura/estado que el
// auto-handoff añadió (`prune`, `sessions`) y la guarda de rc desfasado.
//
// Todo va por Dispatch y no llamando a cmdHandoffPrune/cmdHandoffSessions
// directamente: lo que se está verificando es justo el cableado del switch (un
// subcomando que existe pero no se despacha es indistinguible de uno que no
// existe), además del comportamiento.

// runCLI ejecuta el binario en proceso y devuelve stdout, stderr y exit code.
func runCLI(args ...string) (string, string, int) {
	var out, errb bytes.Buffer
	code := Dispatch(args, &out, &errb)
	return out.String(), errb.String(), code
}

// seedArchived deja un CCP_HOME con n entradas archivadas, la más vieja
// primero (el orden de inserción real: prune recorta por la cabeza).
func seedArchived(t *testing.T, n int) string {
	t.Helper()
	home := t.TempDir()
	var b strings.Builder
	fmt.Fprintf(&b, "version: %d\narchived:\n", core.HandoffsVersion)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "- session: %08d-0000-4000-8000-000000000001\n", i)
		fmt.Fprintf(&b, "  from: p1\n  to: p2\n  slug: -work\n")
		fmt.Fprintf(&b, "  returned_as: %08d-0000-4000-8000-000000000002\n", i)
		fmt.Fprintf(&b, "  since: \"2026-01-01T00:00:00Z\"\n  ended: \"2026-01-01T01:00:00Z\"\n")
	}
	if err := os.WriteFile(filepath.Join(home, "handoffs.yaml"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestHandoffPruneRecortaPorLaCabeza(t *testing.T) {
	home := seedArchived(t, 5)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")

	out, errs, code := runCLI("handoff", "prune", "--keep", "2")
	if code != 0 {
		t.Fatalf("prune falló: code=%d stderr=%q", code, errs)
	}
	if !strings.Contains(out, "3") || !strings.Contains(out, "2") {
		t.Fatalf("el resumen debe decir cuántas quitó y cuántas conserva: %q", out)
	}
	h, err := core.LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Archived) != 2 {
		t.Fatalf("esperaba 2 archivadas, hay %d", len(h.Archived))
	}
	// Se conservan las MÁS RECIENTES: las dos últimas insertadas (índices 3 y 4).
	if !strings.HasPrefix(h.Archived[0].Session, "00000003") ||
		!strings.HasPrefix(h.Archived[1].Session, "00000004") {
		t.Fatalf("prune debe quedarse con la cola, no con la cabeza: %+v", h.Archived)
	}
}

func TestHandoffPruneKeepCeroBorraTodo(t *testing.T) {
	home := seedArchived(t, 3)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")

	if _, errs, code := runCLI("handoff", "prune", "--keep=0"); code != 0 {
		t.Fatalf("--keep=0 (forma pegada) debía funcionar: %s", errs)
	}
	h, _ := core.LoadHandoffs(home)
	if len(h.Archived) != 0 {
		t.Fatalf("keep 0 borra el historial entero: %+v", h.Archived)
	}
}

// El no-op tiene que ser silencioso en disco: prune sin nada que recortar no
// debe reescribir handoffs.yaml (mtime intacto). Si lo reescribiera, un cron
// que lo llame cada hora tocaría el archivo eternamente sin motivo.
func TestHandoffPruneSinNadaQueRecortarNoToqueaElArchivo(t *testing.T) {
	home := seedArchived(t, 3)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	path := filepath.Join(home, "handoffs.yaml")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	out, errs, code := runCLI("handoff", "prune")
	if code != 0 {
		t.Fatalf("prune por defecto falló: %s", errs)
	}
	if !strings.Contains(out, "50") {
		t.Fatalf("el mensaje debe nombrar el keep efectivo (50 por defecto): %q", out)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("un prune no-op no debe reescribir handoffs.yaml")
	}
}

func TestHandoffPruneKeepInvalido(t *testing.T) {
	cases := [][]string{
		{"handoff", "prune", "--keep"},        // sin valor
		{"handoff", "prune", "--keep", "abc"}, // no numérico
		{"handoff", "prune", "--keep", "-1"},  // negativo
		{"handoff", "prune", "--keep="},       // pegado y vacío
		{"handoff", "prune", "--keep=x"},      // pegado y basura
		{"handoff", "prune", "--zzz"},         // flag desconocido
		{"handoff", "prune", "10"},            // posicional (la forma `head -n`)
	}
	for _, args := range cases {
		home := seedArchived(t, 3)
		t.Setenv("CCP_HOME", home)
		t.Setenv("CCP_LANG", "es")
		out, errs, code := runCLI(args...)
		if code != 1 {
			t.Fatalf("%v: esperaba exit 1, got %d (stdout=%q)", args, code, out)
		}
		if errs == "" {
			t.Fatalf("%v: el error debe ir a stderr", args)
		}
		if out != "" {
			t.Fatalf("%v: sin salida en stdout ante error de uso: %q", args, out)
		}
		// Y sobre todo: no debe haber tocado el historial.
		h, _ := core.LoadHandoffs(home)
		if len(h.Archived) != 3 {
			t.Fatalf("%v: un error de uso no puede recortar nada: %+v", args, h.Archived)
		}
	}
}

// Un handoffs.yaml de versión futura se lee como VACÍO: sin el gate, prune
// contestaría «nada que recortar» y el usuario creería que su historial ya
// estaba limpio en vez de enterarse de que su ccp es viejo.
func TestHandoffPruneConVersionFuturaFalla(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	if err := os.WriteFile(filepath.Join(home, "handoffs.yaml"),
		[]byte("version: 99\narchived: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errs, code := runCLI("handoff", "prune")
	if code == 0 {
		t.Fatalf("exit 0 con handoffs.yaml ilegible (stdout=%q)", out)
	}
	if !strings.Contains(errs, "versión más nueva") {
		t.Fatalf("el error debe explicar la versión futura: %q", errs)
	}
}

// seedSessions deja un cc-home de perfil con transcripts para un cwd. Devuelve
// home y el cwd. Los mtimes se separan para poder afirmar el orden.
func seedSessions(t *testing.T, uuids []string) (home, cwd string) {
	t.Helper()
	home = t.TempDir()
	cwd = filepath.Join(home, "repos", "work")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := core.ProjectDir(filepath.Join(home, "profiles", "p1", "cc-home"), core.SlugForCwd(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-24 * time.Hour)
	for i, u := range uuids {
		p := filepath.Join(dir, u+".jsonl")
		line := fmt.Sprintf("{\"type\":\"ai-title\",\"aiTitle\":\"titulo-%d\"}\n", i)
		if err := os.WriteFile(p, []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
		// i creciente => más reciente: la lista debe salir al revés.
		mt := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	return home, cwd
}

func TestHandoffSessionsTexto(t *testing.T) {
	uuids := []string{
		"aaaaaaaa-0000-4000-8000-000000000001",
		"bbbbbbbb-0000-4000-8000-000000000002",
	}
	home, cwd := seedSessions(t, uuids)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_PROFILE", "p1")
	t.Setenv("PWD", cwd)
	t.Setenv("CCP_LANG", "es")

	out, errs, code := runCLI("handoff", "sessions")
	if code != 0 {
		t.Fatalf("sessions falló: %s", errs)
	}
	if !strings.Contains(out, "p1") || !strings.Contains(out, cwd) {
		t.Fatalf("la cabecera debe decir perfil y proyecto: %q", out)
	}
	for _, u := range uuids {
		if !strings.Contains(out, core.ShortUUID(u)) {
			t.Fatalf("falta la sesión %s: %q", u, out)
		}
	}
	// Orden: la más reciente arriba (mismo criterio que el picker).
	if strings.Index(out, core.ShortUUID(uuids[1])) > strings.Index(out, core.ShortUUID(uuids[0])) {
		t.Fatalf("las sesiones deben salir de más nueva a más vieja: %q", out)
	}
	if !strings.Contains(out, "hace ") {
		t.Fatalf("cada fila lleva la antigüedad relativa: %q", out)
	}
}

func TestHandoffSessionsJSON(t *testing.T) {
	uuids := []string{
		"aaaaaaaa-0000-4000-8000-000000000001",
		"bbbbbbbb-0000-4000-8000-000000000002",
	}
	home, cwd := seedSessions(t, uuids)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_PROFILE", "p1")
	t.Setenv("PWD", cwd)

	out, errs, code := runCLI("handoff", "sessions", "--json")
	if code != 0 {
		t.Fatalf("sessions --json falló: %s", errs)
	}
	var rows []handoffSessionJSON
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("stdout no es JSON válido: %v (%q)", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("esperaba 2 filas: %+v", rows)
	}
	if rows[0].UUID != uuids[1] {
		t.Fatalf("la primera fila debe ser la más reciente: %+v", rows)
	}
	if rows[0].Title != "titulo-1" {
		t.Fatalf("falta el título del transcript: %+v", rows[0])
	}
	if _, err := os.Stat(rows[0].Path); err != nil {
		t.Fatalf("path debe apuntar al jsonl real: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, rows[0].MTime); err != nil {
		t.Fatalf("mtime debe ser RFC3339: %q", rows[0].MTime)
	}
}

// Sin sesiones NO es error: `set -e` en un script no puede abortar porque un
// repo aún no tenga conversaciones. Y el JSON tiene que ser `[]`, no `null`,
// para que `jq 'length'` y un bucle de bash funcionen sin guarda.
func TestHandoffSessionsVacioNoEsError(t *testing.T) {
	home, cwd := seedSessions(t, nil)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_PROFILE", "p1")
	t.Setenv("PWD", cwd)
	t.Setenv("CCP_LANG", "es")

	out, _, code := runCLI("handoff", "sessions")
	if code != 0 {
		t.Fatalf("exit %d con lista vacía", code)
	}
	if !strings.Contains(out, "Sin sesiones") {
		t.Fatalf("mensaje de lista vacía: %q", out)
	}
	out, _, code = runCLI("handoff", "sessions", "--json")
	if code != 0 {
		t.Fatalf("--json exit %d con lista vacía", code)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("JSON vacío debe ser [] y no null: %q", out)
	}
}

func TestHandoffSessionsArgsInvalidos(t *testing.T) {
	home, cwd := seedSessions(t, nil)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_PROFILE", "p1")
	t.Setenv("PWD", cwd)
	for _, args := range [][]string{
		{"handoff", "sessions", "--nope"},
		{"handoff", "sessions", "algo"},
	} {
		out, errs, code := runCLI(args...)
		if code != 1 || errs == "" || out != "" {
			t.Fatalf("%v: esperaba exit 1 con error en stderr (code=%d out=%q err=%q)", args, code, out, errs)
		}
	}
}

// Guarda de rc desfasado: con un bloque shell viejo, `ccp handoff prune` cae en
// la rama que lanza claude y llega aquí como «préstale la sesión al perfil
// prune». Sin la guarda, el diagnóstico sería «perfil inexistente: prune», que
// manda al usuario a mirar sus perfiles en vez de a refrescar el rc.
func TestHandoffEmitDetectaRcDesfasado(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_PROFILE", "p1")
	t.Setenv("CCP_LANG", "es")
	for _, sub := range []string{"prune", "sessions"} {
		out, errs, code := runCLI("_handoff", filepath.Join(home, "repo"), sub)
		if code != 1 {
			t.Fatalf("%s: esperaba exit 1, got %d", sub, code)
		}
		if !strings.Contains(errs, "ccp install") || !strings.Contains(errs, sub) {
			t.Fatalf("%s: el error debe nombrar el subcomando y mandar a `ccp install`: %q", sub, errs)
		}
		if out != "" {
			t.Fatalf("%s: no debe emitir delta de entorno: %q", sub, out)
		}
	}
}

// El dispatch nuevo: los cuatro comandos deben ser ALCANZABLES (no caer en
// «comando desconocido»). Es lo único que este test afirma — el comportamiento
// de cada uno lo cubren session_test.go y auto_test.go.
func TestDispatchComandosDeAutoHandoffSonAlcanzables(t *testing.T) {
	t.Setenv("CCP_HOME", t.TempDir())
	t.Setenv("CCP_LANG", "es")
	for _, args := range [][]string{
		{"session", "--help"},
		{"auto", "help"},
		{"_statusline"},
		{"_limit-hook"},
	} {
		_, errs, _ := runCLI(args...)
		if strings.Contains(errs, "desconocido") {
			t.Fatalf("%v no está cableado en el dispatch: %q", args, errs)
		}
	}
}
