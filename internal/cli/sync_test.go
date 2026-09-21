package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// frase larga: el mínimo de la bóveda es el mismo que el de la nube.
const fraseSync = "una frase de prueba bien larga"

// TestSyncDeUnEquipoAOtro es el viaje de E2 por el binario: una máquina crea
// la bóveda en una carpeta y publica, y otra —otro CCP_HOME, la misma carpeta
// y la misma frase— se la lleva. El destino es una carpeta temporal: aquí no
// se habla con ningún servidor.
func TestSyncDeUnEquipoAOtro(t *testing.T) {
	t.Setenv("CCP_SYNC_PASSPHRASE", fraseSync)
	destino := t.TempDir()
	_, src := snapEnv(t)
	if code, out, errs := snapRun(t, "snapshot", "create"); code != 0 {
		t.Fatalf("create: %d %q %q", code, out, errs)
	}

	code, out, errs := snapRun(t, "sync", "remote", "add", "icloud", "file://"+destino)
	if code != 0 || !strings.Contains(out, "CÓDIGO DE RECUPERACIÓN") {
		t.Fatalf("remote add: %d %q %q", code, out, errs)
	}
	if code, out, _ = snapRun(t, "sync", "remote", "list"); code != 0 || !strings.Contains(out, "icloud") || !strings.Contains(out, destino) {
		t.Fatalf("remote list: %d %q", code, out)
	}
	if code, out, errs = snapRun(t, "sync", "push"); code != 0 || !strings.Contains(out, "1") {
		t.Fatalf("push: %d %q %q", code, out, errs)
	}
	if code, out, _ = snapRun(t, "sync", "push"); code != 0 || !strings.Contains(out, "Nada que subir") {
		t.Fatalf("segundo push: %d %q", code, out)
	}

	// La otra máquina: CCP_HOME limpio, la misma carpeta y la misma frase.
	t.Setenv("CCP_HOME", t.TempDir())
	if code, out, errs = snapRun(t, "sync", "remote", "add", "icloud", "file://"+destino); code != 0 {
		t.Fatalf("remote add en la otra máquina: %d %q %q", code, out, errs)
	}
	if strings.Contains(out, "CÓDIGO DE RECUPERACIÓN") {
		t.Fatalf("la segunda máquina creó otra bóveda: %q", out)
	}
	if code, out, errs = snapRun(t, "sync", "pull"); code != 0 || !strings.Contains(out, "Bajado") {
		t.Fatalf("pull: %d %q %q", code, out, errs)
	}
	// El snapshot bajado es ya un snapshot local de esta máquina, con sus
	// elementos: lo que se copió no es una referencia, es la configuración.
	code, out, _ = snapRun(t, "snapshot", "list", "--json")
	var list []snapSummary
	if code != 0 || json.Unmarshal([]byte(out), &list) != nil || len(list) != 1 || list[0].Items == 0 {
		t.Fatalf("list tras el pull: %d %q", code, out)
	}

	// Y lo que se bajó se puede aplicar: el plan primero, la escritura después.
	if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"theme":"light"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errs = snapRun(t, "sync", "apply", "latest", "--plan"); code != 0 || !strings.Contains(out, "claude/settings.json") {
		t.Fatalf("apply --plan: %d %q %q", code, out, errs)
	}
	// Sin --yes no escribe nada y lo dice: es la misma regla que restore.
	if code, _, _ = snapRun(t, "sync", "apply", "latest"); code != 1 {
		t.Fatalf("apply sin --yes salió %d", code)
	}
	if got, _ := os.ReadFile(filepath.Join(src, "settings.json")); string(got) != `{"theme":"light"}` {
		t.Fatalf("el plan escribió: %q", got)
	}
	if code, out, errs = snapRun(t, "sync", "apply", "latest", "--yes"); code != 0 {
		t.Fatalf("apply --yes: %d %q %q", code, out, errs)
	}
	if got, _ := os.ReadFile(filepath.Join(src, "settings.json")); string(got) != `{"theme":"dark"}` {
		t.Fatalf("tras aplicar: %q", got)
	}
}

// Sin ningún destino, push no es un error raro: es una máquina que todavía no
// ha elegido dónde publicar, y el mensaje tiene que decir cómo elegirlo.
func TestSyncSinDestinos(t *testing.T) {
	snapEnv(t)
	code, _, errs := snapRun(t, "sync", "push")
	if code != 1 || !strings.Contains(errs, "ccp sync remote add") {
		t.Fatalf("push sin destinos: %d %q", code, errs)
	}
}

// Con varios destinos, adivinar cuál sería subir a la carpeta equivocada: se
// pide --remote por su nombre.
func TestSyncConVariosDestinosPideCual(t *testing.T) {
	t.Setenv("CCP_SYNC_PASSPHRASE", fraseSync)
	snapEnv(t)
	for _, n := range []string{"icloud", "nas"} {
		if code, out, errs := snapRun(t, "sync", "remote", "add", n, "file://"+t.TempDir()); code != 0 {
			t.Fatalf("add %s: %d %q %q", n, code, out, errs)
		}
	}
	code, _, errs := snapRun(t, "sync", "push")
	if code != 1 || !strings.Contains(errs, "--remote") {
		t.Fatalf("push con dos destinos: %d %q", code, errs)
	}
	if code, out, errs := snapRun(t, "sync", "push", "--remote", "nas"); code != 0 {
		t.Fatalf("push --remote nas: %d %q %q", code, out, errs)
	}
	// Quitarlo solo olvida lo de aquí: en la carpeta no se borra nada.
	code, out, _ := snapRun(t, "sync", "remote", "rm", "nas")
	if code != 0 || !strings.Contains(out, "nas") {
		t.Fatalf("remote rm: %d %q", code, out)
	}
	if code, _, errs = snapRun(t, "sync", "push", "--remote", "nas"); code != 1 || !strings.Contains(errs, "nas") {
		t.Fatalf("push a un destino quitado: %d %q", code, errs)
	}
}

// El nombre del destino acaba siendo un directorio: lo que no vale se rechaza
// antes de crear ninguna bóveda.
func TestSyncRechazaNombresImposibles(t *testing.T) {
	t.Setenv("CCP_SYNC_PASSPHRASE", fraseSync)
	home, _ := snapEnv(t)
	destino := t.TempDir()
	code, _, errs := snapRun(t, "sync", "remote", "add", "../fuera", "file://"+destino)
	if code != 1 || errs == "" {
		t.Fatalf("nombre con «..»: %d %q", code, errs)
	}
	if _, err := os.Stat(filepath.Join(destino, "remote.json")); err == nil {
		t.Fatal("se creó la bóveda pese al nombre inválido")
	}
	if _, err := os.Stat(filepath.Join(home, "sync", "remotes.json")); err == nil {
		t.Fatal("se registró un destino inválido")
	}
	// Y el mismo nombre con otra URL tampoco: serían dos bóvedas compartiendo
	// el directorio de estado, y la clave de una no abre la otra.
	if code, _, _ = snapRun(t, "sync", "remote", "add", "icloud", "file://"+destino); code != 0 {
		t.Fatalf("add: %d %q", code, errs)
	}
	if code, _, errs = snapRun(t, "sync", "remote", "add", "icloud", "file://"+t.TempDir()); code != 1 {
		t.Fatalf("add repetido con otra URL: %d %q", code, errs)
	}
}
