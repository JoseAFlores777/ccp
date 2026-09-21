package cli

import (
	"strings"
	"testing"
)

// `ccp cloud rotate` cambia la frase y el código de recuperación sin tocar la
// clave de cuenta: lo que estaba subido se sigue abriendo, y la frase vieja
// deja de servir.
func TestCloudRotate(t *testing.T) {
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")

	homeA, _ := snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-a"); code != 0 {
		t.Fatalf("login: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "init"); code != 0 {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}

	// Sin frase nueva no hay rotación: reutilizar CCP_CLOUD_PASSPHRASE sería
	// «rotar» a la misma frase y no enterarse.
	if code, out, errs := snapRun(t, "cloud", "rotate"); code == 0 {
		t.Fatalf("rotó sin frase nueva: %q %q", out, errs)
	}
	t.Setenv("CCP_CLOUD_NEW_PASSPHRASE", "otra frase de bóveda larga")
	code, out, errs := snapRun(t, "cloud", "rotate")
	if code != 0 {
		t.Fatalf("rotate: %d %q %q", code, out, errs)
	}
	if !recoveryRe.MatchString(out) {
		t.Fatalf("la rotación no enseña el código nuevo: %q", out)
	}

	// Otro equipo desbloquea con la frase nueva y no con la vieja.
	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-b"); code != 0 {
		t.Fatalf("login B: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "unlock"); code == 0 {
		t.Fatalf("la frase vieja sigue abriendo: %q %q", out, errs)
	}
	t.Setenv("CCP_CLOUD_PASSPHRASE", "otra frase de bóveda larga")
	if code, out, errs := snapRun(t, "cloud", "unlock"); code != 0 {
		t.Fatalf("la frase nueva no abre: %d %q %q", code, out, errs)
	}

	// Y el equipo que rotó sigue abierto: la clave de cuenta no cambió, así
	// que rotar no desbloquea ni bloquea a nadie.
	t.Setenv("CCP_HOME", homeA)
	if code, out, errs := snapRun(t, "cloud", "status"); code != 0 {
		t.Fatalf("status tras rotar: %d %q %q", code, out, errs)
	} else if !strings.Contains(out, "desbloqueada") {
		t.Fatalf("rotar dejó bloqueado al equipo que rotó: %q", out)
	}
}
