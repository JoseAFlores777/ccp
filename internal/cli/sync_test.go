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

// TestSyncDestinoRehechoVuelveASubir fija la regla que faltaba: si la bóveda
// del destino ya no es la que abre la clave guardada —la carpeta se vació, la
// perdió iCloud, la rehizo otra máquina— `remote add` olvida también el estado
// local. Sin eso la clave se reemplazaba pero el mapa de «ya subido» seguía
// intacto, y el siguiente push decía «Nada que subir» contra un destino vacío:
// el usuario creía publicada una configuración que no estaba en ninguna parte.
func TestSyncDestinoRehechoVuelveASubir(t *testing.T) {
	t.Setenv("CCP_SYNC_PASSPHRASE", fraseSync)
	destino := t.TempDir()
	snapEnv(t)
	if code, out, errs := snapRun(t, "snapshot", "create"); code != 0 {
		t.Fatalf("create: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "sync", "remote", "add", "icloud", "file://"+destino); code != 0 {
		t.Fatalf("remote add: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "sync", "push"); code != 0 || strings.Contains(out, "Nada que subir") {
		t.Fatalf("push: %d %q %q", code, out, errs)
	}

	// El destino se vacía por completo: ya no hay bóveda ninguna.
	entradas, err := os.ReadDir(destino)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entradas {
		if err := os.RemoveAll(filepath.Join(destino, e.Name())); err != nil {
			t.Fatal(err)
		}
	}

	if code, out, errs := snapRun(t, "sync", "remote", "add", "icloud", "file://"+destino); code != 0 {
		t.Fatalf("remote add tras vaciar: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "sync", "push"); code != 0 || strings.Contains(out, "Nada que subir") {
		t.Fatalf("push tras vaciar: %d %q %q", code, out, errs)
	}
}

// TestSyncBovedaAjenaNoEsManipulacion: el destino se rehízo desde otra
// máquina —la carpeta perdió el remote.json y otro equipo creó su bóveda— y
// esta, que nunca volvió a añadirlo, sigue con su AK. Antes nadie lo miraba:
// push seguía diciendo «subido» sobre blobs que la otra máquina no podrá abrir
// nunca, y lo ajeno salía como firma inválida, acusando de manipulación lo que
// no es un ataque. Debe decir que la bóveda del destino no es la de este
// equipo, y cómo salir.
func TestSyncBovedaAjenaNoEsManipulacion(t *testing.T) {
	t.Setenv("CCP_SYNC_PASSPHRASE", fraseSync)
	destino := t.TempDir()
	snapEnv(t)
	yo := os.Getenv("CCP_HOME")
	if code, out, errs := snapRun(t, "snapshot", "create"); code != 0 {
		t.Fatalf("create: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "sync", "remote", "add", "icloud", "file://"+destino); code != 0 {
		t.Fatalf("remote add: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "sync", "push"); code != 0 {
		t.Fatalf("push: %d %q %q", code, out, errs)
	}

	// Otra máquina rehace la bóveda: la carpeta perdió el remote.json (lo que
	// hace un servicio de sincronización que no lo propagó) y ella lo crea.
	if err := os.Remove(filepath.Join(destino, "remote.json")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CCP_HOME", t.TempDir())
	if code, out, errs := snapRun(t, "sync", "remote", "add", "otra", "file://"+destino); code != 0 {
		t.Fatalf("remote add de la otra máquina: %d %q %q", code, out, errs)
	}

	// De vuelta aquí, sin volver a añadir el destino.
	t.Setenv("CCP_HOME", yo)
	code, out, errs := snapRun(t, "sync", "pull")
	if code == 0 {
		t.Fatalf("pull contra una bóveda ajena salió 0: %q", out)
	}
	if strings.Contains(errs, "alterado") {
		t.Fatalf("pull acusa de manipulación: %q", errs)
	}
	if !strings.Contains(errs, "bóveda") || !strings.Contains(errs, "icloud") {
		t.Fatalf("pull no explica la bóveda ajena: %q", errs)
	}
}
