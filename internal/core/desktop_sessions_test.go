package core

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	dsUUID1 = "37b541db-430c-4cb1-b50c-ff4f4e467809"
	dsUUID2 = "58f44f2d-378b-440a-8754-280e8fe409a3"
	dsUUID3 = "c9194b6e-0000-4000-8000-000000000003"
)

// dsIndex escribe una entrada del índice de Desktop como la escribe la app:
// <data-dir>/claude-code-sessions/<cuenta>/<org>/local_<id>.json.
func dsIndex(t *testing.T, dataDir, local, uuid, title, cwd string, lastMs int64, archived bool) {
	t.Helper()
	dir := filepath.Join(dataDir, "claude-code-sessions", "acct", "org")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"sessionId":"local_%s","cliSessionId":%q,"cwd":%q,"title":%q,"lastActivityAt":%d,"isArchived":%t,"enabledMcpTools":{}}`,
		local, uuid, cwd, title, lastMs, archived)
	if err := os.WriteFile(filepath.Join(dir, "local_"+local+".json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// dsTranscript escribe un transcript en <cc-home>/projects/<slug>/<uuid>.jsonl.
func dsTranscript(t *testing.T, ccHome, cwd, uuid, content string) string {
	t.Helper()
	dir := ProjectDir(ccHome, SlugForCwd(cwd))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, uuid+".jsonl")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func dsMsg(uuid, text string) string {
	return fmt.Sprintf(`{"type":"user","uuid":%q,"sessionId":%q,"cwd":"/repo","message":{"content":%q}}`+"\n",
		uuid, dsUUID1, text)
}

func dsTitle(title string) string {
	return fmt.Sprintf(`{"type":"custom-title","customTitle":%q,"sessionId":%q}`+"\n", title, dsUUID1)
}

func TestDesktopSessionsLeeElIndiceYCasaLosTranscripts(t *testing.T) {
	dataDir, ccHome := t.TempDir(), t.TempDir()
	cwd := "/repo/work"
	now := time.Now()
	dsIndex(t, dataDir, "aaa", dsUUID1, "Validador de tipo de cuota", cwd, now.Add(-time.Hour).UnixMilli(), false)
	dsIndex(t, dataDir, "bbb", dsUUID2, "", cwd, now.UnixMilli(), false)          // sin título en el índice
	dsIndex(t, dataDir, "ccc", dsUUID3, "Remota", cwd, now.UnixMilli()-10, false) // sin transcript
	dsTranscript(t, ccHome, cwd, dsUUID1, dsMsg("u1", "hola"))
	dsTranscript(t, ccHome, cwd, dsUUID2, `{"type":"ai-title","aiTitle":"Título de la IA"}`+"\n")

	got := DesktopSessions("e-cc", dataDir, ccHome)
	if len(got) != 3 {
		t.Fatalf("esperaba 3 sesiones, hay %d: %+v", len(got), got)
	}
	// Orden: la más reciente primero.
	if got[0].UUID != dsUUID2 || got[2].UUID != dsUUID1 {
		t.Fatalf("orden incorrecto: %s, %s, %s", got[0].UUID, got[1].UUID, got[2].UUID)
	}
	if got[0].Title != "Título de la IA" {
		t.Errorf("sin título en el índice debe caer al del transcript, dio %q", got[0].Title)
	}
	if got[1].Transcript != "" {
		t.Errorf("una sesión sin transcript local debe salir con Transcript vacío: %q", got[1].Transcript)
	}
	if got[2].Profile != "e-cc" || got[2].Cwd != cwd || !got[2].Indexed || got[2].Transcript == "" {
		t.Errorf("fila mal leída: %+v", got[2])
	}
}

// Dos entradas del índice sobre el mismo transcript (la nativa y una importada)
// son UNA sesión: listarla dos veces invitaría a copiarla dos veces.
func TestDesktopSessionsDeduplicaPorTranscript(t *testing.T) {
	dataDir, ccHome := t.TempDir(), t.TempDir()
	dsIndex(t, dataDir, "vieja", dsUUID1, "Con título", "/repo", 1000, false)
	dsIndex(t, dataDir, dsUUID1, dsUUID1, "", "/repo", 2000, false)
	dsTranscript(t, ccHome, "/repo", dsUUID1, dsMsg("u1", "x"))

	got := DesktopSessions("p", dataDir, ccHome)
	if len(got) != 1 {
		t.Fatalf("esperaba 1 sesión, hay %d", len(got))
	}
	if got[0].Title != "Con título" || got[0].LastActivity.UnixMilli() != 2000 {
		t.Errorf("debe quedarse la más reciente sin perder el título de la otra: %+v", got[0])
	}
}

// Un índice ilegible o con campos de otro tipo no rompe el listado: el formato
// es interno de Desktop y puede cambiar.
func TestDesktopSessionsToleraEntradasRaras(t *testing.T) {
	dataDir, ccHome := t.TempDir(), t.TempDir()
	dir := filepath.Join(dataDir, "claude-code-sessions", "a", "o")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"local_roto.json":   "{no es json",
		"local_sinid.json":  `{"title":"x"}`,
		"local_badid.json":  `{"cliSessionId":"../../etc/passwd"}`,
		"local_float.json":  fmt.Sprintf(`{"cliSessionId":%q,"lastActivityAt":1.5e12}`, dsUUID1),
		"otra-cosa.json":    fmt.Sprintf(`{"cliSessionId":%q}`, dsUUID2),
		"local_tipos.json":  fmt.Sprintf(`{"cliSessionId":%q,"title":7}`, dsUUID3),
		"local_nada.jsonx":  "",
		"local_vacio.json":  "",
		"local_null.json":   "null",
		"local_array.json":  "[]",
		"local_string.json": `"x"`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := DesktopSessions("p", dataDir, ccHome)
	if len(got) != 1 || got[0].UUID != dsUUID1 {
		t.Fatalf("solo la entrada válida debe sobrevivir, dio %+v", got)
	}
}

func TestMatchDesktopSessionsNiveles(t *testing.T) {
	cands := []DesktopSession{
		{Profile: "a", UUID: dsUUID1, Title: "Validador de tipo de cuota en préstamos"},
		{Profile: "b", UUID: dsUUID2, Title: "Levanta la app"},
		{Profile: "c", UUID: dsUUID3, Title: "Levanta la app (fork)"},
	}
	cases := []struct {
		q    string
		want []string
	}{
		{dsUUID1, []string{dsUUID1}},
		{strings.ToUpper(dsUUID2), []string{dsUUID2}},
		{"37b541db", []string{dsUUID1}},
		{"37b5", []string{dsUUID1}},
		{"37b", nil}, // menos de 4: no es prefijo, y ningún título lo contiene
		// El título exacto gana al trozo: «Levanta la app» no es ambiguo aunque
		// también sea parte de «Levanta la app (fork)».
		{"  levanta   LA app ", []string{dsUUID2}},
		{"VALIDADOR de tipo", []string{dsUUID1}},
		{"levanta", []string{dsUUID2, dsUUID3}},
		{"préstamos", []string{dsUUID1}},
		{"", nil},
	}
	for _, c := range cases {
		var got []string
		for _, s := range MatchDesktopSessions(c.q, cands) {
			got = append(got, s.UUID)
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("MatchDesktopSessions(%q) = %v, esperaba %v", c.q, got, c.want)
		}
	}
}

func TestTranscriptTitleYCwd(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "t.jsonl")
	content := `{"type":"ai-title","aiTitle":"De la IA"}` + "\n" +
		`{"type":"user","uuid":"u","cwd":"/primera"}` + "\n" +
		`{"type":"custom-title","customTitle":"Vieja"}` + "\n" +
		`{"type":"ai-title","aiTitle":"Otra de la IA"}` + "\n" +
		`{"type":"custom-title","customTitle":"  Nueva   del usuario "}` + "\n" +
		`{"type":"user","uuid":"v","cwd":"/segunda"}`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := TranscriptTitle(p); got != "Nueva del usuario" {
		t.Errorf("gana el último custom-title aunque haya ai-title después, dio %q", got)
	}
	if got := TranscriptCwd(p); got != "/primera" {
		t.Errorf("TranscriptCwd = %q", got)
	}
	if err := os.WriteFile(p, []byte(`{"type":"ai-title","aiTitle":"Solo IA"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := TranscriptTitle(p); got != "Solo IA" {
		t.Errorf("sin custom-title cae al ai-title, dio %q", got)
	}
}

// Una línea de varios MB (una imagen pegada) no puede esconder el título que
// viene detrás: es el tope que RewriteSession tuvo que quitar.
func TestTranscriptTitleSinTopeDeLinea(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.jsonl")
	big := `{"type":"user","uuid":"u","message":"` + strings.Repeat("x", 9<<20) + `"}` + "\n"
	if err := os.WriteFile(p, []byte(big+dsTitle("Detrás de la línea gorda")), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := TranscriptTitle(p); got != "Detrás de la línea gorda" {
		t.Errorf("TranscriptTitle = %q", got)
	}
}

func TestDesktopTranscriptSessionEncuentraSesionesDelCLI(t *testing.T) {
	ccHome := t.TempDir()
	dsTranscript(t, ccHome, "/repo", dsUUID2, `{"type":"user","uuid":"u","cwd":"/repo"}`+"\n"+`{"type":"ai-title","aiTitle":"Del CLI"}`+"\n")
	s, ok := DesktopTranscriptSession("work", ccHome, strings.ToUpper(dsUUID2))
	if !ok {
		t.Fatal("debería encontrar el transcript por uuid")
	}
	if s.Indexed || s.Title != "Del CLI" || s.Cwd != "/repo" || s.Profile != "work" || s.LastActivity.IsZero() {
		t.Errorf("sesión mal armada: %+v", s)
	}
	if _, ok := DesktopTranscriptSession("work", ccHome, dsUUID1); ok {
		t.Error("un uuid que no está no debe encontrarse")
	}
}

// dsPlan arma una copia de e-cc a default sobre directorios temporales.
func dsPlan(t *testing.T, content, title string) (DesktopCopyPlan, string) {
	t.Helper()
	srcCC, dstCC := t.TempDir(), t.TempDir()
	path := dsTranscript(t, srcCC, "/repo", dsUUID1, content)
	plan, err := PlanDesktopCopy(DesktopSession{
		Profile: "e-cc", UUID: dsUUID1, Title: title, Cwd: "/repo", Transcript: path,
	}, "default", dstCC)
	if err != nil {
		t.Fatal(err)
	}
	return plan, dstCC
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPlanDesktopCopyConservaProyectoYUUID(t *testing.T) {
	plan, dstCC := dsPlan(t, dsMsg("u1", "hola"), "T")
	wantDir := ProjectDir(dstCC, SlugForCwd("/repo"))
	if plan.DstTranscript != filepath.Join(wantDir, dsUUID1+".jsonl") || plan.DstCompanion != filepath.Join(wantDir, dsUUID1) {
		t.Errorf("destino mal calculado: %+v", plan)
	}
	if _, err := PlanDesktopCopy(DesktopSession{Profile: "x", UUID: dsUUID1, Transcript: plan.SrcTranscript}, "x", dstCC); err == nil {
		t.Error("copiar a sí mismo debe fallar")
	}
	if _, err := PlanDesktopCopy(DesktopSession{Profile: "x", UUID: dsUUID1}, "y", dstCC); err == nil {
		t.Error("sin transcript no hay nada que copiar")
	}
}

// Caso normal: el destino no la tiene. La copia es un archivo propio (Desktop
// rechaza importar un transcript con más de un hard link), privado, con el
// título añadido al final porque el que tenía está fuera de la ventana que lee
// Desktop.
func TestCopyDesktopSessionNueva(t *testing.T) {
	head := dsTitle("Validador") // título al principio…
	body := strings.Repeat(dsMsg("u", strings.Repeat("y", 1000)), 400)
	plan, _ := dsPlan(t, head+body, "Validador") // …y 400 KB detrás

	res, err := CopyDesktopSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != DesktopCopyNew || !res.TitleAppended {
		t.Fatalf("resultado inesperado: %+v", res)
	}
	got := readFile(t, plan.DstTranscript)
	if got != head+body+dsTitle("Validador") {
		t.Error("la copia debe ser el original más una línea custom-title al final")
	}
	info, err := os.Stat(plan.DstTranscript)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("permisos = %v, esperaba 0600", info.Mode().Perm())
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Nlink != 1 {
		t.Errorf("la copia tiene %d hard links; Desktop la rechazaría", st.Nlink)
	}
	if readFile(t, plan.SrcTranscript) != head+body {
		t.Error("el origen no se toca nunca")
	}
}

func TestCopyDesktopSessionNoDuplicaElTitulo(t *testing.T) {
	plan, _ := dsPlan(t, dsMsg("u1", "hola")+dsTitle("Validador"), "Validador")
	res, err := CopyDesktopSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	if res.TitleAppended {
		t.Error("el título ya está al final: no hay que añadirlo")
	}
	if strings.Count(readFile(t, plan.DstTranscript), "custom-title") != 1 {
		t.Error("la copia no debe llevar el título dos veces")
	}
}

func TestCopyDesktopSessionEsIdempotente(t *testing.T) {
	plan, _ := dsPlan(t, dsMsg("u1", "hola"), "Validador")
	if _, err := CopyDesktopSession(plan); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, plan.DstTranscript)
	res, err := CopyDesktopSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != DesktopCopySame || res.TitleAppended {
		t.Errorf("la segunda copia no debe hacer nada: %+v", res)
	}
	if readFile(t, plan.DstTranscript) != first {
		t.Error("la segunda copia cambió el destino")
	}
}

// El caso que importa de verdad: la sesión ya se copió y el usuario siguió
// trabajando en el destino. Pisarla le borraría ese trabajo.
func TestCopyDesktopSessionNoTocaUnDestinoQueSiguio(t *testing.T) {
	src := dsMsg("u1", "hola")
	plan, _ := dsPlan(t, src, "Validador")
	dest := src + dsTitle("Validador") + dsMsg("u2", "seguí en el destino")
	if err := os.MkdirAll(filepath.Dir(plan.DstTranscript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan.DstTranscript, []byte(dest), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := CopyDesktopSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != DesktopCopyAhead {
		t.Errorf("Outcome = %s, esperaba ahead", res.Outcome)
	}
	if readFile(t, plan.DstTranscript) != dest {
		t.Error("el destino que siguió por su cuenta no se toca")
	}
}

// La vuelta de un préstamo: el destino tiene una copia vieja que nadie
// continuó (con los metadatos que Desktop añade al abrirla) y el origen creció.
func TestCopyDesktopSessionPoneAlDiaUnaCopiaVieja(t *testing.T) {
	old := dsMsg("u1", "hola")
	grown := old + dsMsg("u2", "seguí en el origen")
	plan, _ := dsPlan(t, grown, "Validador")
	meta := `{"type":"mode","mode":"normal","sessionId":"x"}` + "\n" + `{"type":"atis-latch","atis":""}` + "\n"
	if err := os.MkdirAll(filepath.Dir(plan.DstTranscript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan.DstTranscript, []byte(old+dsTitle("Validador")+meta), 0o600); err != nil {
		t.Fatal(err)
	}
	outcome, err := InspectDesktopCopy(plan)
	if err != nil || outcome != DesktopCopyUpdated {
		t.Fatalf("Inspect = %s, %v; esperaba updated", outcome, err)
	}
	res, err := CopyDesktopSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != DesktopCopyUpdated {
		t.Errorf("Outcome = %s", res.Outcome)
	}
	if got := readFile(t, plan.DstTranscript); got != grown+dsTitle("Validador") {
		t.Errorf("el destino debe quedar como el origen actual más el título:\n%s", got)
	}
}

// Una copia exacta sin título (hecha a mano, o por `ccp handoff`) solo gana
// la línea de título: no pierde nada.
func TestCopyDesktopSessionAnadeElTituloAUnaCopiaSinEl(t *testing.T) {
	src := dsMsg("u1", "hola")
	plan, _ := dsPlan(t, src, "Validador")
	if err := os.MkdirAll(filepath.Dir(plan.DstTranscript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan.DstTranscript, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := CopyDesktopSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != DesktopCopyUpdated || !res.TitleAppended {
		t.Errorf("resultado inesperado: %+v", res)
	}
}

func TestCopyDesktopSessionDivergidaNoEscribe(t *testing.T) {
	base := dsMsg("u1", "hola")
	plan, _ := dsPlan(t, base+dsMsg("u2", "solo en el origen"), "Validador")
	dest := base + dsMsg("u3", "solo en el destino")
	if err := os.MkdirAll(filepath.Dir(plan.DstTranscript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan.DstTranscript, []byte(dest), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyDesktopSession(plan); !errors.Is(err, ErrDesktopCopyDiverged) {
		t.Fatalf("esperaba ErrDesktopCopyDiverged, dio %v", err)
	}
	if readFile(t, plan.DstTranscript) != dest {
		t.Error("una divergencia no escribe nada")
	}
}

// Una última línea ilegible cuenta como conversación: ante la duda no se
// reemplaza nada.
func TestTrimTrailingMetaSeDetieneEnLoIlegible(t *testing.T) {
	in := []byte(dsMsg("u1", "x") + "{roto\n" + dsTitle("T"))
	if got := trimTrailingMeta(in); !bytes.Equal(got, []byte(dsMsg("u1", "x")+"{roto\n")) {
		t.Errorf("trimTrailingMeta = %q", got)
	}
	only := []byte(dsTitle("T") + dsTitle("U"))
	if got := trimTrailingMeta(only); len(got) != 0 {
		t.Errorf("solo metadatos debe quedar vacío, dio %q", got)
	}
}

// La carpeta hermana (subagentes, workflows) viaja con el transcript, sin
// pisar nada que el destino tenga distinto.
func TestCopyDesktopSessionCopiaLaCarpetaHermana(t *testing.T) {
	plan, _ := dsPlan(t, dsMsg("u1", "hola"), "")
	files := map[string]string{
		"subagents/agent-1.jsonl":        "linea 1\n",
		"workflows/wf_1.json":            `{"estado":"origen"}`,
		"workflows/scripts/fix.js":       "console.log(1)\n",
		"subagents/workflows/wf_1/a.txt": "a",
	}
	for rel, body := range files {
		p := filepath.Join(plan.SrcCompanion, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// El destino ya tiene el workflow con otro estado: no se pisa.
	kept := filepath.Join(plan.DstCompanion, "workflows", "wf_1.json")
	if err := os.MkdirAll(filepath.Dir(kept), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte(`{"estado":"destino"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hosts", filepath.Join(plan.SrcCompanion, "enlace")); err != nil {
		t.Fatal(err)
	}

	res, err := CopyDesktopSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesCopied != 3 || res.FilesKept != 1 {
		t.Errorf("copiados=%d conservados=%d, esperaba 3 y 1", res.FilesCopied, res.FilesKept)
	}
	if readFile(t, kept) != `{"estado":"destino"}` {
		t.Error("un archivo distinto en el destino no se pisa")
	}
	if readFile(t, filepath.Join(plan.DstCompanion, "subagents", "agent-1.jsonl")) != "linea 1\n" {
		t.Error("falta el transcript del subagente")
	}
	if _, err := os.Lstat(filepath.Join(plan.DstCompanion, "enlace")); err == nil {
		t.Error("los symlinks no se recrean")
	}

	// Si el subagente creció en el origen, se pone al día.
	grown := filepath.Join(plan.SrcCompanion, "subagents", "agent-1.jsonl")
	if err := os.WriteFile(grown, []byte("linea 1\nlinea 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err = CopyDesktopSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesCopied != 1 {
		t.Errorf("solo el subagente que creció debería copiarse, dio %d", res.FilesCopied)
	}
}

func TestDesktopIndexed(t *testing.T) {
	dataDir := t.TempDir()
	if DesktopIndexed(dataDir, dsUUID1) {
		t.Error("un índice vacío no tiene nada")
	}
	dsIndex(t, dataDir, "otro-id-local", strings.ToUpper(dsUUID1), "", "/r", 1, false)
	if !DesktopIndexed(dataDir, dsUUID1) {
		t.Error("cuenta cualquier entrada que apunte al transcript, con el id local que sea")
	}
}

const lsappSample = `
104) "Claude" ASN:0x0-0x30d60d3:
    bundleID="com.anthropic.claudefordesktop"
    bundle path="/Applications/Claude.app"
    executable path="/Applications/Claude.app/Contents/MacOS/Claude"
    pid = 74076 type="Foreground" flavor=3 Version="2.2553.1" fileType="APPL" creator="????" Arch=ARM64
105) "Claude (e-cc)" ASN:0x0-0x1:
    bundleID="com.anthropic.claudefordesktop.ccp.e-cc"
    bundle path="/Users/u/Applications/Claude (e-cc).app"
    pid = 61357 type="Foreground"
106) "Claude Helper" ASN:0x0-0x2:
    bundleID="com.anthropic.claudefordesktop.helper"
    bundle path="/Users/u/Applications/Claude (e-cc).app/Contents/ccp/Claude/Contents/Frameworks/Claude Helper.app"
    pid = 61414 type="UIElement"
`

func TestParseLSAppInfo(t *testing.T) {
	apps := ParseLSAppInfo(lsappSample)
	if len(apps) != 3 {
		t.Fatalf("esperaba 3 apps, hay %d: %+v", len(apps), apps)
	}
	if apps[0] != (LSApp{BundleID: "com.anthropic.claudefordesktop", BundlePath: "/Applications/Claude.app", PID: 74076}) {
		t.Errorf("primera app mal leída: %+v", apps[0])
	}
	if apps[1].BundlePath != "/Users/u/Applications/Claude (e-cc).app" || apps[1].PID != 61357 {
		t.Errorf("la ruta con espacios y paréntesis debe leerse entera: %+v", apps[1])
	}
}

func TestDesktopImportRoute(t *testing.T) {
	launcher := &DesktopApp{
		Path:     "/Users/u/Applications/Claude (e-cc).app",
		Manifest: DesktopAppManifest{Profile: "e-cc", BundleID: "com.anthropic.claudefordesktop.ccp.e-cc"},
	}
	dataDir := "/h/profiles/e-cc/desktop"
	ccHome := "/h/profiles/e-cc/cc-home"
	sane := DesktopProc{
		PID: 61357, Exec: launcher.Path + "/Contents/MacOS/Claude-run", DataDir: dataDir,
		Env: map[string]string{"CLAUDE_CONFIG_DIR": ccHome, DesktopDisableUpdateVar: "1"},
	}
	base := func(profile string) DesktopImportInput {
		return DesktopImportInput{
			Profile: profile, MainApp: "/Applications/Claude.app", MainBundleID: "com.anthropic.claudefordesktop",
			Launcher: launcher, DataDir: dataDir, CCHome: ccHome,
			Procs: []DesktopProc{sane}, ProcsOK: true, Apps: ParseLSAppInfo(lsappSample), AppsOK: true,
		}
	}

	t.Run("perfil sano con helpers vivos", func(t *testing.T) {
		got := DesktopImportRoute(base("e-cc"))
		if got.Refuse != "" || got.App != launcher.Path || !got.Running {
			t.Errorf("un helper con su propio id no es un colapso: %+v", got)
		}
	})
	t.Run("perfil sin ventana abierta", func(t *testing.T) {
		in := base("e-cc")
		in.Procs, in.Apps = nil, nil
		if got := DesktopImportRoute(in); got.Refuse != "" || got.Running {
			t.Errorf("sin ventana el enlace la abre: %+v", got)
		}
	})
	t.Run("perfil sin lanzador", func(t *testing.T) {
		in := base("e-cc")
		in.Launcher = nil
		if got := DesktopImportRoute(in); got.Refuse != DesktopImportNoLauncher {
			t.Errorf("Refuse = %q", got.Refuse)
		}
	})
	t.Run("ventana de perfil reencarnada como el Claude principal", func(t *testing.T) {
		in := base("e-cc")
		in.Apps = append(in.Apps, LSApp{BundleID: "com.anthropic.claudefordesktop", BundlePath: launcher.Path + "/Contents/ccp/Claude"})
		if got := DesktopImportRoute(in); got.Refuse != DesktopImportCollapsed {
			t.Errorf("Refuse = %q", got.Refuse)
		}
	})
	t.Run("ventana de perfil sin su CLAUDE_CONFIG_DIR", func(t *testing.T) {
		in := base("e-cc")
		bad := sane
		bad.Env = map[string]string{DesktopDisableUpdateVar: "1"}
		in.Procs = []DesktopProc{bad}
		if got := DesktopImportRoute(in); got.Refuse != DesktopImportUnsafe {
			t.Errorf("Refuse = %q", got.Refuse)
		}
	})
	t.Run("default sano", func(t *testing.T) {
		got := DesktopImportRoute(base("default"))
		if got.Refuse != "" || got.App != "/Applications/Claude.app" || !got.Running {
			t.Errorf("%+v", got)
		}
	})
	t.Run("default con la identidad secuestrada por el espejo de un lanzador", func(t *testing.T) {
		in := base("default")
		in.Apps = append(in.Apps, LSApp{BundleID: "com.anthropic.claudefordesktop", BundlePath: launcher.Path + "/Contents/ccp/Claude"})
		if got := DesktopImportRoute(in); got.Refuse != DesktopImportHijacked {
			t.Errorf("Refuse = %q", got.Refuse)
		}
	})
	t.Run("default con la app principal corriendo el data dir de un perfil", func(t *testing.T) {
		in := base("default")
		in.Procs = append(in.Procs, DesktopProc{PID: 9, Exec: "/Applications/Claude.app/Contents/MacOS/Claude", DataDir: dataDir})
		if got := DesktopImportRoute(in); got.Refuse != DesktopImportHijacked {
			t.Errorf("Refuse = %q", got.Refuse)
		}
	})
	t.Run("sin sondas no se manda nada", func(t *testing.T) {
		in := base("default")
		in.AppsOK = false
		if got := DesktopImportRoute(in); got.Refuse != DesktopImportProbeMissing {
			t.Errorf("Refuse = %q", got.Refuse)
		}
		in = base("e-cc")
		in.ProcsOK = false
		if got := DesktopImportRoute(in); got.Refuse != DesktopImportProbeMissing {
			t.Errorf("Refuse = %q", got.Refuse)
		}
	})
}

func TestDesktopUserDataDir(t *testing.T) {
	home := t.TempDir()
	if got, _ := DesktopUserDataDir(home, "work"); got != filepath.Join(home, "profiles", "work", "desktop") {
		t.Errorf("perfil: %q", got)
	}
	t.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", "/x/Claude")
	if got, _ := DesktopUserDataDir(home, "default"); got != "/x/Claude" {
		t.Errorf("default: %q", got)
	}
}
