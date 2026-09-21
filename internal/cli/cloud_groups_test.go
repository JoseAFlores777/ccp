package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// Grupos de dispositivos desde el CLI: crearlos, editarlos, verlos y mirar el
// estado por equipo dentro del grupo.
func TestCloudGrupos(t *testing.T) {
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")

	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-a"); code != 0 {
		t.Fatalf("login A: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "init"); code != 0 {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-b"); code != 0 {
		t.Fatalf("login B: %d %q %q", code, out, errs)
	}

	code, out, errs := snapRun(t, "cloud", "groups", "add", "Macs", "mac-a", "mac-b")
	if code != 0 || !strings.Contains(out, "Macs") {
		t.Fatalf("groups add: %d %q %q", code, out, errs)
	}
	code, out, errs = snapRun(t, "cloud", "groups", "--json")
	if code != 0 {
		t.Fatalf("groups --json: %d %q %q", code, out, errs)
	}
	var gs []struct {
		ID      string    `json:"id"`
		Name    string    `json:"name"`
		Members *[]string `json:"members"`
	}
	if json.Unmarshal([]byte(out), &gs) != nil || len(gs) != 1 || gs[0].Name != "Macs" {
		t.Fatalf("groups --json = %q", out)
	}
	// Las listas son siempre arrays: un consumidor hace `.members | length`.
	if gs[0].Members == nil || *gs[0].Members == nil || len(*gs[0].Members) != 2 {
		t.Fatalf("miembros = %q", out)
	}
	// Los miembros se reescriben enteros: mandar la lista que se ve es lo
	// único que no depende de qué versión del grupo tenía uno delante.
	if code, out, errs = snapRun(t, "cloud", "groups", "set", "Macs", "mac-a"); code != 0 {
		t.Fatalf("groups set: %d %q %q", code, out, errs)
	}
	// El estado por dispositivo: sin órdenes publicadas, nadie tiene nada que
	// contar, y decirlo es distinto de decir «pendiente».
	code, out, errs = snapRun(t, "cloud", "groups", "status", "macs", "--json")
	if code != 0 {
		t.Fatalf("groups status: %d %q %q", code, out, errs)
	}
	var st struct {
		Group struct {
			Name string `json:"name"`
		} `json:"group"`
		Members *[]struct {
			DeviceName string `json:"device_name"`
			State      string `json:"state"`
			Member     bool   `json:"member"`
		} `json:"members"`
	}
	if json.Unmarshal([]byte(out), &st) != nil || st.Group.Name != "Macs" || st.Members == nil {
		t.Fatalf("groups status --json = %q", out)
	}
	if len(*st.Members) != 1 || (*st.Members)[0].DeviceName != "mac-a" || (*st.Members)[0].State != "" {
		t.Fatalf("estado del grupo = %q", out)
	}
	if code, out, errs = snapRun(t, "cloud", "groups", "rm", "Macs", "--yes"); code != 0 {
		t.Fatalf("groups rm: %d %q %q", code, out, errs)
	}
	if code, out, _ = snapRun(t, "cloud", "groups"); code != 0 || !strings.Contains(out, "ccp cloud groups add") {
		t.Fatalf("sin grupos: %d %q", code, out)
	}
	// Un grupo que no existe se dice por su nombre, no con un error del API.
	if code, _, errs = snapRun(t, "cloud", "groups", "status", "Macs"); code != 1 || !strings.Contains(errs, "Macs") {
		t.Fatalf("grupo inexistente: %d %q", code, errs)
	}
}

// Renombrar no es vaciar: `groups set <grupo> --name <nuevo>` sin lista de
// equipos conserva los miembros. Antes se quedaban en cero en silencio, que es
// destruir la membresía con el comando natural para cambiar el nombre.
func TestCloudGruposRenombrarConservaMiembros(t *testing.T) {
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")

	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-a"); code != 0 {
		t.Fatalf("login A: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "init"); code != 0 {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-b"); code != 0 {
		t.Fatalf("login B: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "groups", "add", "Macs", "mac-a", "mac-b"); code != 0 {
		t.Fatalf("groups add: %d %q %q", code, out, errs)
	}

	code, out, errs := snapRun(t, "cloud", "groups", "set", "Macs", "--name", "Mis Macs")
	if code != 0 {
		t.Fatalf("groups set --name: %d %q %q", code, out, errs)
	}
	if !strings.Contains(out, "Mis Macs") || strings.Contains(out, " 0 ") {
		t.Fatalf("renombrar vació el grupo: %q", out)
	}
	code, out, errs = snapRun(t, "cloud", "groups", "--json")
	if code != 0 {
		t.Fatalf("groups --json: %d %q %q", code, out, errs)
	}
	var gs []struct {
		Name    string   `json:"name"`
		Members []string `json:"members"`
	}
	if json.Unmarshal([]byte(out), &gs) != nil || len(gs) != 1 || gs[0].Name != "Mis Macs" {
		t.Fatalf("groups --json = %q", out)
	}
	if len(gs[0].Members) != 2 {
		t.Fatalf("miembros tras renombrar = %q", out)
	}

	// Vaciarlo de verdad se puede, pero hay que pedirlo: sin lista y sin
	// --empty el comando no adivina.
	if code, _, errs = snapRun(t, "cloud", "groups", "set", "Mis Macs"); code != 1 || !strings.Contains(errs, "--empty") {
		t.Fatalf("set sin lista: %d %q", code, errs)
	}
	if code, out, errs = snapRun(t, "cloud", "groups", "set", "Mis Macs", "--empty"); code != 0 {
		t.Fatalf("groups set --empty: %d %q %q", code, out, errs)
	}
	code, out, _ = snapRun(t, "cloud", "groups", "--json")
	gs = gs[:0]
	if json.Unmarshal([]byte(out), &gs) != nil || len(gs) != 1 || len(gs[0].Members) != 0 {
		t.Fatalf("grupo vaciado = %q", out)
	}
}
