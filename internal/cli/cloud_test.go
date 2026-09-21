package cli

// cloud_test.go — `ccp cloud …` de punta a punta contra el API real (en
// memoria) y un Keycloak falso. Dos máquinas: la que crea la bóveda y la que la
// desbloquea con la frase o con el código de recuperación.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
	"github.com/JoseAFlores777/ccp/internal/cloud/oidctest"
	cloudsrv "github.com/JoseAFlores777/ccp/internal/cloud/server"
	cloudstore "github.com/JoseAFlores777/ccp/internal/cloud/store"
	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
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

	// El --json de push habla como el resto del CLI: claves snake_case y
	// listas que siempre son arrays (un consumidor hace `.missing | length`).
	code, out, errs = snapRun(t, "cloud", "push", "--json")
	if code != 0 {
		t.Fatalf("push --json: %d %q %q", code, out, errs)
	}
	var pr struct {
		Snapshots *int      `json:"snapshots"`
		Uploaded  *int      `json:"uploaded"`
		Bytes     *int64    `json:"bytes"`
		Missing   *[]string `json:"missing"`
		TooLarge  *[]string `json:"too_large"`
	}
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		t.Fatalf("push --json ilegible: %v (%q)", err, out)
	}
	if pr.Snapshots == nil || pr.Uploaded == nil || pr.Bytes == nil {
		t.Fatalf("push --json sin claves snake_case: %q", out)
	}
	if pr.Missing == nil || *pr.Missing == nil || pr.TooLarge == nil || *pr.TooLarge == nil {
		t.Fatalf("push --json con listas null: %q", out)
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
	// El host de verdad es de terceros en todos estos: comparar prefijos de
	// cadena daba por localhost a un typosquat («127.0.0.1.atacante.tld») y a
	// un userinfo que esconde el host real tras la arroba.
	for _, srv := range []string{
		"http://ccp.example.com",
		"http://127.0.0.1.atacante.tld",
		"http://localhost.atacante.tld",
		"http://127.0.0.1:1@atacante.tld",
		"http://user@localhost:8080",
	} {
		if code, _, errs := snapRun(t, "cloud", "login", srv); code != 1 || !strings.Contains(errs, "https") {
			t.Fatalf("login por http %s: %d %q", srv, code, errs)
		}
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

// TestCloudInitEnsenaElCodigoAunqueNoPuedaGuardarLaAK fija el orden: la bóveda
// se crea una sola vez (el servidor la guarda con ON CONFLICT DO NOTHING y
// nunca guarda el código, solo su envoltura), así que si el disco falla al
// guardar la clave de este equipo lo que NO puede perderse es el código de
// recuperación.
func TestCloudInitEnsenaElCodigoAunqueNoPuedaGuardarLaAK(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root escribe en un directorio sin permisos")
	}
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")

	home, _ := snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-a"); code != 0 {
		t.Fatalf("login: %d %q %q", code, out, errs)
	}
	// El login ya creó <CCP_HOME>/cloud; dejarlo sin escritura es lo que rompe
	// SaveAK (CreateTemp dentro del propio directorio).
	dir := filepath.Join(home, "cloud")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	code, out, errs := snapRun(t, "cloud", "init")
	if !recoveryRe.MatchString(out) {
		t.Fatalf("no enseñó el código de recuperación: %d %q %q", code, out, errs)
	}
	if code == 0 {
		t.Fatalf("guardar la AK falló y aun así salió 0: %q %q", out, errs)
	}
	if !strings.Contains(errs, "ccp cloud unlock") {
		t.Fatalf("no dijo cómo desbloquear este equipo: %q", errs)
	}
}

// La política de este dispositivo es suya, no de la cuenta: volver a entrar
// —porque caducó el refresh token o porque se cambia de cuenta— no puede
// devolver a `auto` un equipo que su dueño puso en `manual`. Si `login`
// reescribe config.json entero sin copiar la política anterior, la defensa
// desaparece sin decir nada y el agente vuelve a aplicar solo.
func TestCloudLoginConservaLaPoliticaDelDispositivo(t *testing.T) {
	url, iss := cloudServerIss(t)
	t.Setenv("CCP_NO_BROWSER", "1")

	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url); code != 0 {
		t.Fatalf("login 1: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "policy", "manual"); code != 0 {
		t.Fatalf("policy manual: %d %q %q", code, out, errs)
	}

	// Misma cuenta, misma máquina: re-autenticación normal.
	if code, out, errs := snapRun(t, "cloud", "login", url); code != 0 {
		t.Fatalf("login 2: %d %q %q", code, out, errs)
	}
	if _, out, _ := snapRun(t, "cloud", "policy"); !strings.Contains(out, "manual") {
		t.Fatalf("la política se perdió al re-entrar: %q", out)
	}

	// Y al cambiar de cuenta también: la máquina sigue siendo la misma.
	iss.As("user-2", "otro@example.com")
	if code, out, errs := snapRun(t, "cloud", "login", url); code != 0 {
		t.Fatalf("login 3: %d %q %q", code, out, errs)
	}
	if _, out, _ := snapRun(t, "cloud", "policy"); !strings.Contains(out, "manual") {
		t.Fatalf("la política se perdió al cambiar de cuenta: %q", out)
	}
}

// La política se puede decidir ANTES de dar de alta la máquina: es la defensa
// del equipo frente a la cuenta, y exigir una sesión para ponerla obliga a
// pasar por el estado que se quiere evitar —dado de alta en `auto`—. `serve`
// ya lo permitía (srvCloudSetPolicy tolera ErrNotLoggedIn); el CLI no, así que
// lo que la GUI anuncia no se podía hacer desde la terminal.
func TestCloudPolicySinSesionYSobreviveAlAlta(t *testing.T) {
	url, _ := cloudServerIss(t)
	t.Setenv("CCP_NO_BROWSER", "1")

	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "policy", "manual"); code != 0 || !strings.Contains(out, "manual") {
		t.Fatalf("policy sin sesión: %d %q %q", code, out, errs)
	}
	if code, out, _ := snapRun(t, "cloud", "policy"); code != 0 || !strings.Contains(out, "manual") {
		t.Fatalf("policy no se guardó sin sesión: %d %q", code, out)
	}
	if code, out, errs := snapRun(t, "cloud", "login", url); code != 0 {
		t.Fatalf("login: %d %q %q", code, out, errs)
	}
	if _, out, _ := snapRun(t, "cloud", "policy"); !strings.Contains(out, "manual") {
		t.Fatalf("el alta tiró la política decidida de antemano: %q", out)
	}
}

// `ccp cloud verify` comprueba la historia entera, no un snapshot suelto: que
// cada eslabón lo firmó esta cuenta y que sigue estando todo lo que esta
// máquina subió. Un snapshot que subimos y ya no está es la única señal de que
// han cortado la cadena por la cabeza.
func TestCloudVerifyMiraLaCadenaEntera(t *testing.T) {
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
	if code, out, errs := snapRun(t, "snapshot", "create"); code != 0 {
		t.Fatalf("snapshot create: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "push"); code != 0 {
		t.Fatalf("push: %d %q %q", code, out, errs)
	}
	code, out, errs := snapRun(t, "cloud", "verify", "--json")
	if code != 0 {
		t.Fatalf("verify de una historia intacta: %d %q %q", code, out, errs)
	}
	var rep struct {
		Links  *int `json:"links"`
		Faults *[]struct {
			Code string `json:"code"`
			ID   string `json:"id"`
		} `json:"faults"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil || rep.Links == nil || *rep.Links != 1 {
		t.Fatalf("verify --json = %q (%v)", out, err)
	}
	if rep.Faults == nil || *rep.Faults == nil || len(*rep.Faults) != 0 {
		t.Fatalf("faults tiene que ser un array vacío, no null: %q", out)
	}

	// El servidor «pierde» un snapshot que esta máquina subió: el estado local
	// sabe que estaba y verify lo canta.
	statePath := filepath.Join(os.Getenv("CCP_HOME"), "cloud", "state.json")
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Pushed map[string]string `json:"pushed"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	st.Pushed["local-que-ya-no-esta"] = strings.Repeat("e", 64)
	raw, _ = json.Marshal(st)
	if err := os.WriteFile(statePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errs = snapRun(t, "cloud", "verify")
	if code != 1 {
		t.Fatalf("verify con un eslabón perdido = %d, quiero 1 (%q %q)", code, out, errs)
	}
	// La línea entera, no un trozo del id: una falta sin %s en su plantilla
	// recibía igualmente un argumento y salía con «%!(EXTRA string=)» pegado.
	quiero := "  " + strings.Repeat("e", 12) + "  " + i18n.T(i18n.Es, "cli.cloud.fault.dropped")
	if !strings.Contains(out, quiero+"\n") {
		t.Fatalf("verify no dice cuál falta tal cual: quiero %q en %q %q", quiero, out, errs)
	}
	if strings.Contains(out+errs, "%!") {
		t.Fatalf("verify imprime un verbo mal formateado: %q %q", out, errs)
	}
}

// F3-2: bajar a un archivo. El .ccpsnap se lleva el snapshot entero a otra
// máquina sin dejarlo en ésta, y el .tar.gz descifrado no sale sin que alguien
// diga que sí a escribir las claves en claro.
func TestCloudPullAArchivo(t *testing.T) {
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")
	t.Setenv("CCP_SNAPSHOT_PASSPHRASE", "frase del archivo larga")

	home, _ := snapEnv(t)
	if err := core.ProfileAddDeepseek(home, "deep", core.BuiltinDefaults()); err != nil {
		t.Fatal(err)
	}
	if err := core.ProfileSetKey(home, "deep", "sk-muy-secreto"); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-a"); code != 0 {
		t.Fatalf("login: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "init"); code != 0 {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "snapshot", "create"); code != 0 {
		t.Fatalf("create: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "push"); code != 0 {
		t.Fatalf("push: %d %q %q", code, out, errs)
	}

	// Máquina B: sin snapshots propios. Lo que baja va al archivo y a ningún
	// almacén.
	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-b"); code != 0 {
		t.Fatalf("login B: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "unlock"); code != 0 {
		t.Fatalf("unlock: %d %q %q", code, out, errs)
	}
	dest := filepath.Join(t.TempDir(), "copia.ccpsnap")
	code, out, errs := snapRun(t, "cloud", "pull", "-o", dest)
	if code != 0 || !strings.Contains(out, "copia.ccpsnap") {
		t.Fatalf("pull -o: %d %q %q", code, out, errs)
	}
	if _, out, _ := snapRun(t, "snapshot", "list", "--json"); !strings.Contains(out, "[]") {
		t.Fatalf("pull -o dejó rastro en el almacén: %q", out)
	}
	if code, out, errs := snapRun(t, "snapshot", "import", dest); code != 0 {
		t.Fatalf("import: %d %q %q", code, out, errs)
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-muy-secreto") {
		t.Fatal("el .ccpsnap lleva la clave en claro")
	}

	// El .tar.gz descifrado: con secretos dentro hay que decirlo en voz alta.
	plain := filepath.Join(t.TempDir(), "copia.tar.gz")
	code, out, errs = snapRun(t, "cloud", "pull", "-o", plain, "--decrypted")
	if code != 1 || !strings.Contains(errs, "claro") {
		t.Fatalf("descifrado sin confirmar: %d %q %q", code, out, errs)
	}
	if _, err := os.Stat(plain); err == nil {
		t.Fatal("se escribió el archivo que no se confirmó")
	}
	if code, out, errs = snapRun(t, "cloud", "pull", "-o", plain, "--decrypted", "--yes"); code != 0 {
		t.Fatalf("descifrado: %d %q %q", code, out, errs)
	}
	if raw, err = os.ReadFile(plain); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gunzipCLI(t, raw)), "sk-muy-secreto") {
		t.Fatal("el .tar.gz descifrado no trae el contenido en claro")
	}
}

func gunzipCLI(t *testing.T, raw []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
