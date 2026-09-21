package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// `ccp cloud audit` cuenta quién hizo qué y cuándo. Es lo único que el
// servidor puede contar de una configuración que no puede leer.
func TestCloudAudit(t *testing.T) {
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")

	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-a"); code != 0 {
		t.Fatalf("login: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "init"); code != 0 {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}

	code, out, errs := snapRun(t, "cloud", "audit", "--json")
	if code != 0 {
		t.Fatalf("audit --json: %d %q %q", code, out, errs)
	}
	var log []struct {
		At         string         `json:"at"`
		Device     string         `json:"device"`
		DeviceName string         `json:"device_name"`
		Action     string         `json:"action"`
		Detail     map[string]any `json:"detail"`
	}
	if json.Unmarshal([]byte(out), &log) != nil || len(log) != 2 {
		t.Fatalf("audit --json = %q", out)
	}
	// Lo último primero: crear la bóveda pasó después de dar de alta el equipo.
	if log[0].Action != "vault.create" || log[1].Action != "device.create" {
		t.Fatalf("orden = %q", out)
	}
	if log[1].DeviceName != "mac-a" || log[1].At == "" {
		t.Fatalf("entrada = %+v", log[1])
	}
	// El detalle es siempre un objeto: un consumidor hace `.detail.name`.
	if log[0].Detail == nil {
		t.Fatalf("detalle nulo = %q", out)
	}

	code, out, errs = snapRun(t, "cloud", "audit")
	if code != 0 || !strings.Contains(out, "vault.create") || !strings.Contains(out, "mac-a") {
		t.Fatalf("audit: %d %q %q", code, out, errs)
	}
	code, out, errs = snapRun(t, "cloud", "audit", "--action", "device.create", "--json")
	if code != 0 || strings.Contains(out, "vault.create") {
		t.Fatalf("audit --action: %d %q %q", code, out, errs)
	}
	code, out, errs = snapRun(t, "cloud", "audit", "--device", "mac-a", "--limit", "1", "--json")
	if code != 0 {
		t.Fatalf("audit --device: %d %q %q", code, out, errs)
	}
	if json.Unmarshal([]byte(out), &log) != nil || len(log) != 1 {
		t.Fatalf("audit --device --limit = %q", out)
	}

	// Un equipo que no se sabe cuál es no se convierte en «toda la cuenta»:
	// una auditoría que enseña de más se lee como si enseñara lo pedido.
	if code, out, errs = snapRun(t, "cloud", "audit", "--device", "no-existe"); code == 0 {
		t.Fatalf("aceptó un equipo desconocido: %q %q", out, errs)
	}
	if code, out, errs = snapRun(t, "cloud", "audit", "--since", "ayer"); code == 0 {
		t.Fatalf("aceptó una fecha que no lo es: %q %q", out, errs)
	}

	// Vacío por el filtro no es vacío: el registro está lleno, lo que no hay
	// son líneas que encajen. Decir «no hay nada apuntado» en el único comando
	// que existe para auditar lleva a concluir que no se está grabando.
	code, out, errs = snapRun(t, "cloud", "audit", "--action", "revision.publish")
	if code != 0 {
		t.Fatalf("audit --action sin coincidencias: %d %q %q", code, out, errs)
	}
	if strings.Contains(out, "nada apuntado") {
		t.Errorf("filtro sin coincidencias dice que no hay nada apuntado: %q", out)
	}
	if !strings.Contains(out, "filtro") {
		t.Errorf("filtro sin coincidencias no lo dice: %q", out)
	}
}

// Un contador de bytes se lee como un contador de bytes: el detalle llega
// decodificado de JSON (todo número es float64) y el portal, que lee el MISMO
// registro, lo imprime entero. Dos lectores del mismo log tienen que coincidir.
func TestAuditDetalleNumerosGrandes(t *testing.T) {
	var d map[string]any
	if err := json.Unmarshal([]byte(`{"id":"abc","blobs":12,"size":1048576}`), &d); err != nil {
		t.Fatal(err)
	}
	if got, want := auditDetail(d), "blobs=12 id=abc size=1048576"; got != want {
		t.Errorf("auditDetail = %q, quiero %q", got, want)
	}
	var b map[string]any
	if err := json.Unmarshal([]byte(`{"bytes":1500000}`), &b); err != nil {
		t.Fatal(err)
	}
	if got, want := auditDetail(b), "bytes=1500000"; got != want {
		t.Errorf("auditDetail = %q, quiero %q", got, want)
	}
}
