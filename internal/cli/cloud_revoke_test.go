package cli

import (
	"strings"
	"testing"
)

// `ccp cloud revoke` corta una credencial, y decirlo entero es parte del
// comando: la orden que el equipo tuviera pendiente también se cierra, porque
// un equipo revocado no vuelve a preguntar.
func TestCloudRevoke(t *testing.T) {
	url, iss := cloudServerIss(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")

	homeA, _ := snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-a"); code != 0 {
		t.Fatalf("login A: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "init"); code != 0 {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	// Cada equipo con su sesión: revocar uno revoca a sus hermanos de sesión,
	// y aquí hace falta que mac-a siga en pie para revocar y para mirar.
	iss.NewSession("sesion-b")
	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-b"); code != 0 {
		t.Fatalf("login B: %d %q %q", code, out, errs)
	}
	// Se revoca desde mac-a: el equipo que se revoca es siempre otro.
	t.Setenv("CCP_HOME", homeA)

	code, out, errs := snapRun(t, "cloud", "revoke", "mac-b")
	if code != 0 || !strings.Contains(out, "mac-b") {
		t.Fatalf("revoke: %d %q %q", code, out, errs)
	}
	if !strings.Contains(out, "pendiente") && !strings.Contains(out, "pending") {
		t.Fatalf("no dice qué pasa con lo que tenía pendiente: %q", out)
	}
	// Y queda apuntado quién revocó a quién.
	code, out, errs = snapRun(t, "cloud", "audit", "--action", "device.revoke")
	if code != 0 || !strings.Contains(out, "device.revoke") {
		t.Fatalf("audit: %d %q %q", code, out, errs)
	}
}
