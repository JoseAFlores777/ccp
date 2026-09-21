package portal

import (
	"os/exec"
	"testing"
)

// El login es la parte del portal que ningún test de Go puede tocar —vive en
// el navegador— y la que más duele cuando falla: un parámetro mal escrito en
// el canje del código solo se ve al pulsar «entrar» contra un Keycloak de
// verdad. oidc_test.mjs lo ejecuta con `fetch`, `location` y `sessionStorage`
// de mentira, que es lo más cerca que se puede estar sin levantar un realm.
func TestLoginDelPortal(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("sin node: no se puede ejecutar el login del portal")
	}
	out, err := exec.Command(node, "oidc_test.mjs").CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("el login del portal no se comporta: %v", err)
	}
}
