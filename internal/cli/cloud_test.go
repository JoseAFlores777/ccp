package cli

// cloud_test.go — `ccp cloud …` de punta a punta contra el API real (en
// memoria) y un Keycloak falso. Dos máquinas: la que crea la bóveda y la que la
// desbloquea con la frase o con el código de recuperación.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
	"github.com/JoseAFlores777/ccp/internal/cloud/oidctest"
	cloudsrv "github.com/JoseAFlores777/ccp/internal/cloud/server"
	cloudstore "github.com/JoseAFlores777/ccp/internal/cloud/store"
)

// cloudServer levanta el API real con persistencia y almacenamiento en memoria
// y un Keycloak falso. Devuelve su URL (http://127.0.0.1:…, que la CLI acepta
// sin https).
func cloudServer(t *testing.T) string {
	url, _ := cloudServerIss(t)
	return url
}

// cloudServerIss es lo mismo, pero devuelve también el emisor falso para las
// pruebas que necesitan cambiar de cuenta a mitad de camino.
func cloudServerIss(t *testing.T) (string, *oidctest.Issuer) {
	t.Helper()
	iss := oidctest.New(t)
	srv := httptest.NewServer(cloudsrv.New(cloudsrv.Config{
		Store: cloudstore.NewMem(), Blobs: blobstest.New(t),
		Verifier: cloudsrv.NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Issuer:   iss.URL, ClientID: oidctest.ClientID,
	}))
	t.Cleanup(srv.Close)
	return srv.URL, iss
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

// Cambiar de cuenta en el MISMO equipo no puede dejar viva la clave de la
// cuenta anterior: si se queda, `push` sella y firma con ella contra la nube de
// la cuenta nueva y esos blobs son ilegibles para siempre, incluso para el
// equipo que los subió.
func TestCloudLoginOtraCuentaOlvidaLaBoveda(t *testing.T) {
	url, iss := cloudServerIss(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")

	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url); code != 0 {
		t.Fatalf("login 1: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "init"); code != 0 {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "snapshot", "create"); code != 0 {
		t.Fatalf("snapshot create: %d %q %q", code, out, errs)
	}

	// La misma máquina, otra cuenta.
	iss.As("user-2", "otro@example.com")
	if code, out, errs := snapRun(t, "cloud", "login", url); code != 0 {
		t.Fatalf("login 2: %d %q %q", code, out, errs)
	}
	_, out, _ := snapRun(t, "cloud", "status", "--json")
	var st struct {
		Email       string `json:"email"`
		Vault       string `json:"vault"`
		PendingPush int    `json:"pending_push"`
	}
	if json.Unmarshal([]byte(out), &st) != nil || st.Email != "otro@example.com" {
		t.Fatalf("status --json = %q", out)
	}
	if st.Vault == "unlocked" {
		t.Fatalf("la bóveda de la cuenta anterior sigue abierta: %q", out)
	}
	// Y sin bóveda no se puede subir nada sellado con la clave ajena.
	if code, out, errs := snapRun(t, "cloud", "push"); code == 0 {
		t.Fatalf("push con la clave de la otra cuenta: %d %q %q", code, out, errs)
	}
}

// Tras cambiar de cuenta, lo que este equipo ya subió a la nube anterior no
// cuenta: la bóveda nueva sella con otra clave y arriba no hay nada. Si `push`
// se fía del mapa de subidos sin mirar de qué cuenta era, la cuenta nueva se
// queda sin respaldo y dice que todo está en orden.
func TestCloudPushTrasCambiarDeCuentaSube(t *testing.T) {
	url, iss := cloudServerIss(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")

	snapEnv(t)
	for _, args := range [][]string{{"cloud", "login", url}, {"cloud", "init"}, {"snapshot", "create"}, {"cloud", "push"}} {
		if code, out, errs := snapRun(t, args...); code != 0 {
			t.Fatalf("%v: %d %q %q", args, code, out, errs)
		}
	}

	iss.As("user-2", "otro@example.com")
	for _, args := range [][]string{{"cloud", "login", url}, {"cloud", "init"}} {
		if code, out, errs := snapRun(t, args...); code != 0 {
			t.Fatalf("%v: %d %q %q", args, code, out, errs)
		}
	}
	if code, out, errs := snapRun(t, "cloud", "push"); code != 0 || !strings.Contains(out, "Subidos 1") {
		t.Fatalf("push en la cuenta nueva: %d %q %q", code, out, errs)
	}
	_, out, _ := snapRun(t, "cloud", "list", "--json")
	var list []struct {
		ID   string `json:"id"`
		Here bool   `json:"here"`
	}
	if json.Unmarshal([]byte(out), &list) != nil || len(list) != 1 || !list[0].Here {
		t.Fatalf("cloud list --json de la cuenta nueva = %q", out)
	}
	_, out, _ = snapRun(t, "cloud", "status", "--json")
	var st struct {
		PendingPush int `json:"pending_push"`
	}
	if json.Unmarshal([]byte(out), &st) != nil || st.PendingPush != 0 {
		t.Fatalf("status --json = %q", out)
	}
}

// cloudServerRoto anuncia otro emisor en /v1/info y falla en /v1/me: es el
// servidor contra el que un login a medias no debe destruir la sesión viva.
func cloudServerRoto(t *testing.T) string {
	t.Helper()
	iss := oidctest.New(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.Info{APIVersion: api.Version, Issuer: iss.URL, ClientID: oidctest.ClientID})
	})
	mux.HandleFunc("/v1/me", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// Un login fallido contra OTRO servidor no puede dejar la máquina sin la sesión
// que ya tenía: el token solo se guarda cuando la sesión está completa.
func TestCloudLoginFallidoNoRompeLaSesionAnterior(t *testing.T) {
	url := cloudServer(t)
	otro := cloudServerRoto(t)
	t.Setenv("CCP_NO_BROWSER", "1")

	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-a"); code != 0 {
		t.Fatalf("login A: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "login", otro, "--name", "mac-b"); code == 0 {
		t.Fatalf("login contra el servidor roto debía fallar: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "devices"); code != 0 || !strings.Contains(out, "mac-a") {
		t.Fatalf("devices tras el login fallido: %d %q %q", code, out, errs)
	}
}
