package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// desktop_audit_test.go — el doctor, montando a mano los estados que hubo que
// diagnosticar a mano el 2026-09-15.

// La regla que hace útil al doctor: si no pudo mirar, dice Unknown. Un doctor
// que confunde «no lo sé» con «está bien» convierte una duda en falsa certeza.
func TestDesktopAuditSondaAusenteEsUnknownNoOK(t *testing.T) {
	o, _ := desktopAppOpts(t, "work")
	fs := DesktopAudit(DesktopAuditOptions{
		Home: o.Home, Cfg: o.Cfg, AppsDir: o.AppsDir, Profile: "work",
		Probes: DesktopProbes{}, // sin sondas
	})
	if !hasFinding(fs, DesktopFindingProbeMissing, DesktopSevUnknown) {
		t.Fatalf("sin sonda de procesos el resultado debe ser Unknown: %v", fs)
	}
	for _, f := range fs {
		if f.Severity == DesktopSevOK {
			t.Errorf("nada puede declararse OK sin haberlo comprobado: %v", f)
		}
	}
}

func TestDesktopAuditDetectaInstanciaColapsada(t *testing.T) {
	o, _ := desktopAppOpts(t, "work")
	res, err := BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	dataDir := DesktopDataDir(o.Home, "work")

	// El proceso patológico: la app principal con el data dir del perfil.
	patologico := []DesktopProc{{
		PID: 97504, Exec: "/Applications/Claude.app/Contents/MacOS/Claude",
		DataDir: dataDir, Env: map[string]string{},
	}}
	fs := DesktopAudit(DesktopAuditOptions{
		Home: o.Home, Cfg: o.Cfg, AppsDir: o.AppsDir, Profile: "work",
		Probes: DesktopProbes{
			Procs: func() ([]DesktopProc, bool) { return patologico, true },
		},
	})
	if !hasFinding(fs, DesktopFindingForeignExec, DesktopSevError) {
		t.Errorf("falta el hallazgo de ventana ajena: %v", fs)
	}
	if !hasFinding(fs, DesktopFindingNoConfigDir, DesktopSevError) {
		t.Errorf("esa ventana escribe en el ~/.claude global: %v", fs)
	}
	if DesktopWorst(fs) != DesktopSevError {
		t.Errorf("la peor severidad debe ser error: %s", DesktopWorst(fs))
	}

	// La misma instancia, pero bien: arrancada por su lanzador y con todo puesto.
	sano := []DesktopProc{{
		PID: 1, Exec: filepath.Join(res.App.Path, "Contents", "ccp", "Claude", "Contents", "MacOS", "Claude"),
		DataDir: dataDir,
		Env: map[string]string{
			"CLAUDE_CONFIG_DIR":     ccHomePath(o.Home, "work"),
			DesktopDisableUpdateVar: "1",
		},
	}}
	fs = DesktopAudit(DesktopAuditOptions{
		Home: o.Home, Cfg: o.Cfg, AppsDir: o.AppsDir, Profile: "work",
		Probes: DesktopProbes{Procs: func() ([]DesktopProc, bool) { return sano, true }},
	})
	if DesktopWorst(fs) == DesktopSevError {
		t.Errorf("una instancia sana no debe dar error: %v", fs)
	}
}

// El secuestro de identidad: macOS registra la ventana con el id de Claude en
// vez de con el del lanzador, y entonces abrir Claude desde el Dock activa esa.
func TestDesktopAuditDetectaIdentidadColapsada(t *testing.T) {
	o, _ := desktopAppOpts(t, "work")
	if _, err := BuildDesktopApp(o); err != nil {
		t.Fatal(err)
	}
	fs := DesktopAudit(DesktopAuditOptions{
		Home: o.Home, Cfg: o.Cfg, AppsDir: o.AppsDir, Profile: "work",
		Probes: DesktopProbes{
			Procs:           func() ([]DesktopProc, bool) { return nil, true },
			RunningIdentity: func(string) (string, bool) { return "com.anthropic.claudefordesktop", true },
		},
	})
	if !hasFinding(fs, DesktopFindingIdentity, DesktopSevError) {
		t.Fatalf("falta el hallazgo de identidad colapsada: %v", fs)
	}

	// Con su id propio, nada que objetar.
	fs = DesktopAudit(DesktopAuditOptions{
		Home: o.Home, Cfg: o.Cfg, AppsDir: o.AppsDir, Profile: "work",
		Probes: DesktopProbes{
			Procs:           func() ([]DesktopProc, bool) { return nil, true },
			RunningIdentity: func(string) (string, bool) { return DesktopBundleID("work"), true },
		},
	})
	if hasFinding(fs, DesktopFindingIdentity, DesktopSevError) {
		t.Errorf("con el id propio no hay colapso: %v", fs)
	}
}

// El hallazgo que le habría ahorrado al usuario tres horas: sus 40 sesiones no
// se habían borrado, estaban bajo la otra cuenta.
func TestDesktopAuditCuentaLasCuentasDelDataDir(t *testing.T) {
	o, _ := desktopAppOpts(t, "work")
	dataDir := DesktopDataDir(o.Home, "work")
	mkSessions(t, dataDir, "cuenta-trabajo", "org-trabajo", 40)
	mkSessions(t, dataDir, "cuenta-personal", "org-personal", 0)

	fs := DesktopAudit(DesktopAuditOptions{
		Home: o.Home, Cfg: o.Cfg, AppsDir: o.AppsDir, Profile: "work",
		Probes: DesktopProbes{Procs: func() ([]DesktopProc, bool) { return nil, true }},
	})
	f, ok := findFinding(fs, DesktopFindingMultiAccount)
	if !ok {
		t.Fatalf("dos cuentas en el data dir deben reportarse: %v", fs)
	}
	if f.Severity != DesktopSevInfo {
		t.Errorf("no es un fallo, es información: %s", f.Severity)
	}
	if f.Detail != "cuenta-personal=0 cuenta-trabajo=40" {
		t.Errorf("Detail = %q — debe decir cuántas sesiones tiene cada cuenta", f.Detail)
	}

	// Con una sola cuenta no hay nada que explicar.
	o2, _ := desktopAppOpts(t, "personal")
	mkSessions(t, DesktopDataDir(o2.Home, "personal"), "una", "org", 3)
	fs = DesktopAudit(DesktopAuditOptions{
		Home: o2.Home, Cfg: o2.Cfg, AppsDir: o2.AppsDir, Profile: "personal",
		Probes: DesktopProbes{Procs: func() ([]DesktopProc, bool) { return nil, true }},
	})
	if _, ok := findFinding(fs, DesktopFindingMultiAccount); ok {
		t.Errorf("una sola cuenta no es noticia: %v", fs)
	}
}

// El espejo deja de compartir inodo con la fuente cuando ShipIt reemplaza el
// bundle entero, incluso sin cambiar de versión. Sin esta señal, el lanzador
// retiene una copia completa de Claude en disco y nadie se entera.
func TestDesktopAuditDetectaEspejoHuerfano(t *testing.T) {
	o, _ := desktopAppOpts(t, "work")
	src := o.SourceApp
	res, err := BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	probes := DesktopProbes{Procs: func() ([]DesktopProc, bool) { return nil, true }}
	fs := DesktopAudit(DesktopAuditOptions{
		Home: o.Home, Cfg: o.Cfg, AppsDir: o.AppsDir, Profile: "work", Probes: probes,
	})
	if hasFinding(fs, DesktopFindingMirrorOrphan, DesktopSevWarn) {
		t.Fatalf("recién construido comparte inodos: %v", fs)
	}

	// Reemplazar el stub de la fuente como hace ShipIt: mismo contenido, otro inodo.
	stub := filepath.Join(src, "Contents", "MacOS", "Claude")
	data, rerr := os.ReadFile(stub)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if err := os.Remove(stub); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stub, data, 0o755); err != nil {
		t.Fatal(err)
	}

	fs = DesktopAudit(DesktopAuditOptions{
		Home: o.Home, Cfg: o.Cfg, AppsDir: o.AppsDir, Profile: "work", Probes: probes,
	})
	if !hasFinding(fs, DesktopFindingMirrorOrphan, DesktopSevWarn) {
		t.Errorf("el espejo ya no comparte inodo y nadie lo dijo: %v", fs)
	}
	_ = res
}

func TestDesktopFindingsJSONSiempreArray(t *testing.T) {
	b, err := DesktopFindingsJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[]" {
		t.Errorf("sin hallazgos debe emitirse [], no null: %s", b)
	}
	var v []map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Errorf("JSON inválido: %v", err)
	}
}

func TestDesktopWorst(t *testing.T) {
	if got := DesktopWorst(nil); got != DesktopSevOK {
		t.Errorf("sin hallazgos = ok, no %q", got)
	}
	fs := []DesktopFinding{{Severity: DesktopSevInfo}, {Severity: DesktopSevWarn}}
	if got := DesktopWorst(fs); got != DesktopSevWarn {
		t.Errorf("worst = %q", got)
	}
	fs = append(fs, DesktopFinding{Severity: DesktopSevError})
	if got := DesktopWorst(fs); got != DesktopSevError {
		t.Errorf("worst = %q", got)
	}
}

func mkSessions(t *testing.T, dataDir, account, org string, n int) {
	t.Helper()
	dir := filepath.Join(dataDir, "claude-code-sessions", account, org)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		f := filepath.Join(dir, "local_"+string(rune('a'+i%26))+string(rune('a'+i/26))+".json")
		if err := os.WriteFile(f, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func hasFinding(fs []DesktopFinding, code, sev string) bool {
	for _, f := range fs {
		if f.Code == code && f.Severity == sev {
			return true
		}
	}
	return false
}

func findFinding(fs []DesktopFinding, code string) (DesktopFinding, bool) {
	for _, f := range fs {
		if f.Code == code {
			return f, true
		}
	}
	return DesktopFinding{}, false
}
