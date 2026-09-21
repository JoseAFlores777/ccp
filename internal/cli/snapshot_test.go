package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// snapEnv monta un CCP_HOME con el perfil official «work» y un ~/.claude falso
// con un settings.json. homeConPerfil fija CCP_HOME y CCP_CLAUDE_SRC; HOME y la
// ventana default de Desktop también son temporales.
func snapEnv(t *testing.T) (home, src string) {
	t.Helper()
	home = homeConPerfil(t, "work", "official")
	src = os.Getenv("CCP_CLAUDE_SRC")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", t.TempDir())
	t.Setenv("CCP_LANG", "es")
	if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return home, src
}

func snapRun(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = Dispatch(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestSnapshotCreateListShow(t *testing.T) {
	snapEnv(t)
	code, out, errs := snapRun(t, "snapshot", "create")
	if code != 0 || !strings.Contains(out, "guardado") {
		t.Fatalf("create: %d %q %q", code, out, errs)
	}
	if code, out, _ = snapRun(t, "snapshot", "create"); code != 0 || !strings.Contains(out, "Sin cambios") {
		t.Fatalf("segundo create: %d %q", code, out)
	}
	code, out, _ = snapRun(t, "snapshot", "list", "--json")
	var list []snapSummary
	if code != 0 || json.Unmarshal([]byte(out), &list) != nil || len(list) != 1 || list[0].Trigger != "manual" {
		t.Fatalf("list --json: %d %q", code, out)
	}
	if code, out, _ = snapRun(t, "snapshot", "show", list[0].ID[:8], "--json"); code != 0 || !strings.Contains(out, `"claude/settings.json"`) {
		t.Fatalf("show: %d %q", code, out)
	}
	if code, out, _ = snapRun(t, "snapshot", "list"); code != 0 || !strings.Contains(out, list[0].ID[:12]) {
		t.Fatalf("list en texto: %d %q", code, out)
	}
	// Con etiqueta se guarda aunque no haya cambios: quien etiqueta quiere ese punto.
	if code, out, _ = snapRun(t, "snapshot", "create", "-m", "antes de migrar"); code != 0 || !strings.Contains(out, "guardado") {
		t.Fatalf("create -m: %d %q", code, out)
	}
}

func TestSnapshotListEmptyIsArray(t *testing.T) {
	snapEnv(t)
	if code, out, _ := snapRun(t, "snapshot", "list", "--json"); code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("list --json vacío = %d %q", code, out)
	}
}

func TestSnapshotDiffAgainstLive(t *testing.T) {
	_, src := snapEnv(t)
	snapRun(t, "snapshot", "create")
	os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"theme":"light"}`), 0o644)
	code, out, _ := snapRun(t, "snapshot", "diff", "latest")
	if code != 0 || !strings.Contains(out, "~ claude/settings.json") {
		t.Fatalf("diff: %d %q", code, out)
	}
}

func TestSnapshotBadInput(t *testing.T) {
	snapEnv(t)
	for _, args := range [][]string{
		{"snapshot", "nope"},
		{"snapshot", "create", "--nope"},
		{"snapshot", "show"},
		{"snapshot", "show", "zz"},
	} {
		if code, _, _ := snapRun(t, args...); code != 1 {
			t.Errorf("%v: código %d, quiero 1", args, code)
		}
	}
}
func TestSnapshotRestoreNeedsYes(t *testing.T) {
	_, src := snapEnv(t)
	settings := filepath.Join(src, "settings.json")
	snapRun(t, "snapshot", "create")
	_, out, _ := snapRun(t, "snapshot", "list", "--json")
	var list []snapSummary
	json.Unmarshal([]byte(out), &list)
	// El id concreto y no «latest»: tras restaurar, el snapshot de seguridad
	// pasa a ser el más reciente.
	id := list[0].ID[:12]
	os.WriteFile(settings, []byte(`{"theme":"light"}`), 0o644)

	code, out, errs := snapRun(t, "snapshot", "restore", id)
	if code != 1 || !strings.Contains(out, "claude/settings.json") || !strings.Contains(errs, "--yes") {
		t.Fatalf("sin --yes: %d %q %q", code, out, errs)
	}
	if b, _ := os.ReadFile(settings); string(b) != `{"theme":"light"}` {
		t.Fatal("sin --yes se escribió")
	}
	if code, _, _ = snapRun(t, "snapshot", "restore", id, "--dry-run"); code != 0 {
		t.Fatalf("--dry-run: %d", code)
	}
	code, out, errs = snapRun(t, "snapshot", "restore", id, "--yes")
	if code != 0 || !strings.Contains(out, "Restaurado") {
		t.Fatalf("--yes: %d %q %q", code, out, errs)
	}
	if b, _ := os.ReadFile(settings); string(b) != `{"theme":"dark"}` {
		t.Fatalf("settings.json = %s", b)
	}
	// Ya coincide: repetir no hace nada y no es un error.
	if code, out, _ = snapRun(t, "snapshot", "restore", id, "--yes"); code != 0 || !strings.Contains(out, "Nada que restaurar") {
		t.Fatalf("restore repetido: %d %q", code, out)
	}
	// El estado de antes (light) quedó en el snapshot de seguridad, que ahora es el último.
	_, out, _ = snapRun(t, "snapshot", "list", "--json")
	json.Unmarshal([]byte(out), &list)
	if len(list) != 2 || list[0].Trigger != "pre-restore" {
		t.Fatalf("tras restaurar: %q", out)
	}
}

func TestSnapshotPinAndPrune(t *testing.T) {
	snapEnv(t)
	snapRun(t, "snapshot", "create")
	if code, out, _ := snapRun(t, "snapshot", "pin", "latest", "-m", "bueno"); code != 0 || !strings.Contains(out, "fijado") {
		t.Fatalf("pin: %d %q", code, out)
	}
	code, out, _ := snapRun(t, "snapshot", "prune", "--dry-run", "--json")
	var rep struct {
		Kept   int  `json:"kept"`
		DryRun bool `json:"dry_run"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &rep) != nil || rep.Kept != 1 || !rep.DryRun {
		t.Fatalf("prune --dry-run --json: %d %q", code, out)
	}
	if code, out, _ = snapRun(t, "snapshot", "unpin", "latest"); code != 0 || !strings.Contains(out, "ya no está fijado") {
		t.Fatalf("unpin: %d %q", code, out)
	}
}

func TestSnapshotExportImport(t *testing.T) {
	snapEnv(t)
	t.Setenv("CCP_SNAPSHOT_PASSPHRASE", "frase de prueba larga")
	snapRun(t, "snapshot", "create")
	_, out, _ := snapRun(t, "snapshot", "list", "--json")
	var src []snapSummary
	json.Unmarshal([]byte(out), &src)

	file := filepath.Join(t.TempDir(), "config.ccpsnap")
	if code, out, errs := snapRun(t, "snapshot", "export", "latest", file, "--with-secrets"); code != 0 || !strings.Contains(out, "sellados") {
		t.Fatalf("export: %d %q %q", code, out, errs)
	}
	if fi, err := os.Stat(file); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("un .ccpsnap con secretos debe quedar 0600: %v %v", fi, err)
	}

	// Otra «máquina»: un CCP_HOME vacío.
	t.Setenv("CCP_HOME", t.TempDir())
	if code, out, errs := snapRun(t, "snapshot", "import", file); code != 0 || !strings.Contains(out, "importado") {
		t.Fatalf("import: %d %q %q", code, out, errs)
	}
	_, out, _ = snapRun(t, "snapshot", "list", "--json")
	var dst []snapSummary
	if json.Unmarshal([]byte(out), &dst) != nil || len(dst) != 1 || dst[0].ID != src[0].ID {
		t.Fatalf("tras importar, list = %q; quiero el id %s", out, src[0].ID)
	}
}

// Sin frase no se puede exportar con secretos ni en un pipe ni con una frase corta.
func TestSnapshotExportNeedsPassphrase(t *testing.T) {
	snapEnv(t)
	snapRun(t, "snapshot", "create")
	file := filepath.Join(t.TempDir(), "x.ccpsnap")
	t.Setenv("CCP_SNAPSHOT_PASSPHRASE", "corta")
	if code, _, errs := snapRun(t, "snapshot", "export", "latest", file, "--with-secrets"); code != 1 || !strings.Contains(errs, "12") {
		t.Fatalf("frase corta: %d %q", code, errs)
	}
	if _, err := os.Stat(file); err == nil {
		t.Fatal("se escribió el archivo pese al error")
	}
}

func TestProfileRmTakesSafetySnapshot(t *testing.T) {
	home, _ := snapEnv(t)
	t.Setenv("CCP_NO_AUTO_SNAPSHOT", "")
	code, _, errs := snapRun(t, "profile", "rm", "work")
	if code != 0 || !strings.Contains(errs, "Snapshot de seguridad") {
		t.Fatalf("profile rm: %d %q", code, errs)
	}
	st, _ := core.OpenSnapshotStore(home)
	m, err := st.Latest()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range m.Items {
		if it.LPath == "ccp/ccp.yaml" {
			data, _ := st.GetBlob(it.Hash)
			if !strings.Contains(string(data), "work") {
				t.Fatalf("el snapshot de seguridad no tiene el perfil borrado:\n%s", data)
			}
			return
		}
	}
	t.Fatal("el snapshot de seguridad no tiene ccp.yaml")
}

func TestDailySnapshotOncePerDay(t *testing.T) {
	home, _ := snapEnv(t)
	t.Setenv("CCP_NO_AUTO_SNAPSHOT", "")
	snapRun(t, "profile", "list")
	snapRun(t, "profile", "list")
	st, _ := core.OpenSnapshotStore(home)
	ms, err := st.List()
	if err != nil || len(ms) != 1 || ms[0].Trigger != "daily" {
		t.Fatalf("tras dos comandos: %d snapshots (%v)", len(ms), err)
	}
}

// Un comando de scripting no toca el almacén: ni siquiera lo crea.
func TestScriptingCommandsSkipDailySnapshot(t *testing.T) {
	home, _ := snapEnv(t)
	t.Setenv("CCP_NO_AUTO_SNAPSHOT", "")
	snapRun(t, "status")
	snapRun(t, "resolve", "/")
	if _, err := os.Stat(core.SnapshotStoreDir(home)); err == nil {
		t.Fatal("un comando de scripting creó el almacén de snapshots")
	}
}

func TestAutoSnapshotOffByEnv(t *testing.T) {
	home, _ := snapEnv(t) // TestMain deja CCP_NO_AUTO_SNAPSHOT=1
	if code, _, errs := snapRun(t, "profile", "rm", "work"); code != 0 || strings.Contains(errs, "Snapshot de seguridad") {
		t.Fatalf("con el snapshot automático apagado: %d %q", code, errs)
	}
	if _, err := os.Stat(core.SnapshotStoreDir(home)); err == nil {
		t.Fatal("con CCP_NO_AUTO_SNAPSHOT=1 se creó el almacén")
	}
}

// Un «-m» vacío es la única forma de BORRAR una etiqueta desde el CLI, así que
// la opción se decide por presencia, no por valor: comparar con "" dejaba
// `-m ""` sin efecto y la GUI (que sí borra con la cadena vacía) ofrecía un
// «equivalente CLI» que hacía lo contrario.
func TestSnapshotPinEtiquetaVacia(t *testing.T) {
	snapEnv(t)
	snapRun(t, "snapshot", "create")
	if code, _, errs := snapRun(t, "snapshot", "pin", "latest", "-m", "bueno"); code != 0 {
		t.Fatalf("pin: %d %q", code, errs)
	}
	// Sin «-m» la etiqueta no se toca.
	snapRun(t, "snapshot", "unpin", "latest")
	if l := snapLabel(t); l != "bueno" {
		t.Fatalf("sin -m la etiqueta cambió: %q", l)
	}
	if code, _, errs := snapRun(t, "snapshot", "pin", "latest", "-m", ""); code != 0 {
		t.Fatalf("pin -m vacío: %d %q", code, errs)
	}
	if l := snapLabel(t); l != "" {
		t.Fatalf("-m vacío no borró la etiqueta: %q", l)
	}
}

// snapLabel devuelve la etiqueta del snapshot más reciente.
func snapLabel(t *testing.T) string {
	t.Helper()
	_, out, _ := snapRun(t, "snapshot", "list", "--json")
	var list []snapSummary
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list) == 0 {
		t.Fatalf("list --json: %q", out)
	}
	return list[0].Label
}
