package cli

// cloud_test.go — `ccp cloud …` de punta a punta contra el API real (en
// memoria) y un Keycloak falso. Dos máquinas: la que crea la bóveda y la que la
// desbloquea con la frase o con el código de recuperación.

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
	"github.com/JoseAFlores777/ccp/internal/cloud/oidctest"
	cloudsrv "github.com/JoseAFlores777/ccp/internal/cloud/server"
	cloudstore "github.com/JoseAFlores777/ccp/internal/cloud/store"
)

// cloudServer levanta el API real con persistencia y almacenamiento en memoria
// y un Keycloak falso. Devuelve su URL (http://127.0.0.1:…, que la CLI acepta
// sin https).
func cloudServer(t *testing.T) string {
	t.Helper()
	iss := oidctest.New(t)
	srv := httptest.NewServer(cloudsrv.New(cloudsrv.Config{
		Store: cloudstore.NewMem(), Blobs: blobstest.New(t),
		Verifier: cloudsrv.NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Issuer:   iss.URL, ClientID: oidctest.ClientID,
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

var recoveryRe = regexp.MustCompile(`[A-Z2-7]{4}(-[A-Z2-7]{4}){7}`)

func TestCloudEndToEnd(t *testing.T) {
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")

	// Máquina A.
	snapEnv(t)
	code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-a")
	if code != 0 || !strings.Contains(out, "ccp cloud init") {
		t.Fatalf("login A: %d %q %q", code, out, errs)
	}
	code, out, errs = snapRun(t, "cloud", "init")
	if code != 0 || !recoveryRe.MatchString(out) {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	if code, out, errs = snapRun(t, "snapshot", "create"); code != 0 {
		t.Fatalf("snapshot create: %d %q %q", code, out, errs)
	}
	if code, out, errs = snapRun(t, "cloud", "push"); code != 0 || !strings.Contains(out, "Subidos 1") {
		t.Fatalf("push: %d %q %q", code, out, errs)
	}

	// Máquina B: otro CCP_HOME, la misma cuenta.
	snapEnv(t)
	if code, out, errs = snapRun(t, "cloud", "login", url, "--name", "mac-b"); code != 0 || !strings.Contains(out, "ccp cloud unlock") {
		t.Fatalf("login B: %d %q %q", code, out, errs)
	}
	if code, out, errs = snapRun(t, "cloud", "unlock"); code != 0 {
		t.Fatalf("unlock: %d %q %q", code, out, errs)
	}
	if code, out, errs = snapRun(t, "cloud", "pull"); code != 0 || !strings.Contains(out, "ccp snapshot restore") {
		t.Fatalf("pull: %d %q %q", code, out, errs)
	}
	_, out, _ = snapRun(t, "snapshot", "list", "--json")
	var list []snapSummary
	if json.Unmarshal([]byte(out), &list) != nil || len(list) != 1 {
		t.Fatalf("tras pull, snapshot list = %q", out)
	}
	_, out, _ = snapRun(t, "cloud", "status", "--json")
	var st struct {
		LoggedIn    bool   `json:"logged_in"`
		Vault       string `json:"vault"`
		PendingPush int    `json:"pending_push"`
	}
	if json.Unmarshal([]byte(out), &st) != nil || !st.LoggedIn || st.Vault != "unlocked" || st.PendingPush != 0 {
		t.Fatalf("status --json = %q", out)
	}
	if code, out, _ = snapRun(t, "cloud", "devices"); code != 0 || !strings.Contains(out, "mac-a") || !strings.Contains(out, "mac-b") {
		t.Fatalf("devices: %d %q", code, out)
	}
	if code, out, _ = snapRun(t, "cloud", "logout"); code != 0 {
		t.Fatalf("logout: %d %q", code, out)
	}
	if code, _, errs = snapRun(t, "cloud", "push"); code != 1 || !strings.Contains(errs, "ccp cloud login") {
		t.Fatalf("push tras logout: %d %q", code, errs)
	}
}

func TestCloudUnlockWithRecoveryCode(t *testing.T) {
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")
	snapEnv(t)
	snapRun(t, "cloud", "login", url)
	_, out, _ := snapRun(t, "cloud", "init")
	code := recoveryRe.FindString(out)
	if code == "" {
		t.Fatalf("init no enseñó el código: %q", out)
	}

	snapEnv(t)
	snapRun(t, "cloud", "login", url)
	t.Setenv("CCP_CLOUD_PASSPHRASE", "otra frase que no es")
	if c, _, errs := snapRun(t, "cloud", "unlock"); c != 1 || !strings.Contains(errs, "no abre") {
		t.Fatalf("frase equivocada: %d %q", c, errs)
	}
	t.Setenv("CCP_CLOUD_RECOVERY", code)
	if c, out, errs := snapRun(t, "cloud", "unlock", "--recovery"); c != 0 {
		t.Fatalf("unlock --recovery: %d %q %q", c, out, errs)
	}
}

func TestCloudRequiresHTTPS(t *testing.T) {
	snapEnv(t)
	if code, _, errs := snapRun(t, "cloud", "login", "http://ccp.example.com"); code != 1 || !strings.Contains(errs, "https") {
		t.Fatalf("login por http: %d %q", code, errs)
	}
}
