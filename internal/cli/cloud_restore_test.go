package cli

// cloud_restore_test.go — `ccp cloud restore` (spec §10.3.1, caminos 1 y 3).
// El camino entero contra el API de verdad: subir desde una máquina, y bajar y
// aplicar en la misma, que es lo que hace una máquina nueva salvo por el login.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cloudUp deja este equipo con sesión, bóveda y un snapshot arriba.
func cloudUp(t *testing.T, url string) {
	t.Helper()
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac"); code != 0 {
		t.Fatalf("login: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "init"); code != 0 {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
}

func TestCloudRestorePideConfirmacionYLuegoAplica(t *testing.T) {
	url := cloudServer(t)
	_, src := snapEnv(t)
	cloudUp(t, url)
	if code, out, errs := snapRun(t, "snapshot", "create"); code != 0 {
		t.Fatalf("create: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "push"); code != 0 {
		t.Fatalf("push: %d %q %q", code, out, errs)
	}
	if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"theme":"light"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Sin --yes: enseña el plan, no escribe y sale 1. La misma regla que
	// `ccp snapshot restore`: una restauración no se dispara por omisión.
	code, out, errs := snapRun(t, "cloud", "restore", "latest")
	if code != 1 || !strings.Contains(out, "claude/settings.json") {
		t.Fatalf("plan: %d %q %q", code, out, errs)
	}
	if got, _ := os.ReadFile(filepath.Join(src, "settings.json")); string(got) != `{"theme":"light"}` {
		t.Fatalf("el plan escribió: %q", got)
	}
	// Y dice lo que queda a mano: el perfil official no tiene sesión aquí.
	if !strings.Contains(out, "work") {
		t.Errorf("el plan tiene que nombrar los pendientes: %q", out)
	}

	code, out, errs = snapRun(t, "cloud", "restore", "latest", "--yes")
	if code != 0 {
		t.Fatalf("restore: %d %q %q", code, out, errs)
	}
	if got, _ := os.ReadFile(filepath.Join(src, "settings.json")); string(got) != `{"theme":"dark"}` {
		t.Fatalf("settings.json = %q; quiero el del snapshot", got)
	}
}

func TestCloudRestoreJSONLlevaPlanProyectosYPendientes(t *testing.T) {
	url := cloudServer(t)
	snapEnv(t)
	cloudUp(t, url)
	snapRun(t, "snapshot", "create")
	snapRun(t, "cloud", "push")

	code, out, errs := snapRun(t, "cloud", "restore", "latest", "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("restore --json: %d %q %q", code, out, errs)
	}
	var rep struct {
		Cloud    string `json:"cloud"`
		Snapshot string `json:"snapshot"`
		Plan     *struct {
			Steps []map[string]any `json:"steps"`
		} `json:"plan"`
		Projects *[]map[string]any `json:"projects"`
		Pending  *[]map[string]any `json:"pending"`
		NoData   *[]string         `json:"no_data"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("json ilegible: %v (%q)", err, out)
	}
	if rep.Cloud == "" || rep.Snapshot == "" || rep.Plan == nil {
		t.Fatalf("faltan campos: %+v", rep)
	}
	// Las listas son arrays, nunca null: quien las consuma hace `| length`.
	if rep.Projects == nil || rep.Pending == nil || rep.NoData == nil {
		t.Errorf("las listas vacías son [], nunca null: %q", out)
	}
}

func TestCloudRestoreRechazaUnMapeoMalEscrito(t *testing.T) {
	url := cloudServer(t)
	snapEnv(t)
	cloudUp(t, url)
	snapRun(t, "snapshot", "create")
	snapRun(t, "cloud", "push")

	code, _, errs := snapRun(t, "cloud", "restore", "latest", "--map", "solo-la-clave", "--yes")
	if code != 1 || !strings.Contains(errs, "--map") {
		t.Fatalf("un --map sin ruta tiene que explicarse: %d %q", code, errs)
	}
	code, _, errs = snapRun(t, "cloud", "restore", "latest", "--map", "abc=relativa", "--yes")
	if code != 1 || !strings.Contains(errs, "--map") {
		t.Fatalf("un --map con ruta relativa tiene que explicarse: %d %q", code, errs)
	}
}

// Una cuenta recién creada no tiene snapshots: eso es un estado, no un fallo,
// y lo dice con las mismas palabras que `pull`.
func TestCloudRestoreSinNadaArribaNoEsUnFallo(t *testing.T) {
	url := cloudServer(t)
	snapEnv(t)
	cloudUp(t, url)
	code, out, errs := snapRun(t, "cloud", "restore", "latest")
	if code != 0 || !strings.Contains(out, "no tiene snapshots") {
		t.Fatalf("restore con la nube vacía: %d %q %q", code, out, errs)
	}
}
