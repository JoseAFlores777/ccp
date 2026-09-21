package cli

// cloud_agent_test.go — `ccp cloud agent|review|policy` de punta a punta: el
// «portal» de estas pruebas es el propio test, que firma la revisión con la
// clave de cuenta que la máquina guardó al crear la bóveda. El servidor nunca
// la tiene, así que no hay otra forma de publicar una orden.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/agent"
	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// publicaRevision firma y publica una revisión para ESTE equipo, como haría el
// portal con la AK desbloqueada en el navegador.
func publicaRevision(t *testing.T, home, id, snap, base string) {
	t.Helper()
	ctx := context.Background()
	files := client.NewFiles(home)
	cfg, cl, err := client.Session(ctx, files)
	if err != nil {
		t.Fatal(err)
	}
	acct, err := client.Account(files)
	if err != nil {
		t.Fatal(err)
	}
	parts := crypt.RevisionParts{ID: id, Device: cfg.DeviceID, Snapshot: snap, Base: base}
	if _, err := cl.PublishRevision(ctx, api.RevisionIn{ID: id, DeviceID: cfg.DeviceID,
		Snapshot: snap, Base: base, Sig: acct.SignRevision(parts), Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

// subidoAlaNube devuelve el id en la nube del único snapshot que hay subido.
func subidoAlaNube(t *testing.T, home string) string {
	t.Helper()
	st, err := client.NewFiles(home).LoadState()
	if err != nil {
		t.Fatal(err)
	}
	for _, cloudID := range st.Pushed {
		return cloudID
	}
	t.Fatal("no hay nada subido")
	return ""
}

func escribe(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

const revUno = "0000000000000000000000000000000000000000000000000000000000000001"

func TestCloudAgentAplicaYReviewConfirma(t *testing.T) {
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")
	home, src := snapEnv(t)

	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac"); code != 0 {
		t.Fatalf("login: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "init"); code != 0 {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	escribe(t, filepath.Join(src, "CLAUDE.md"), "uno")
	escribe(t, filepath.Join(src, "hooks", "x.sh"), "echo 1")
	if code, out, errs := snapRun(t, "snapshot", "create"); code != 0 {
		t.Fatalf("snapshot: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "push"); code != 0 {
		t.Fatalf("push: %d %q %q", code, out, errs)
	}
	escribe(t, filepath.Join(src, "CLAUDE.md"), "dos")
	escribe(t, filepath.Join(src, "hooks", "x.sh"), "echo 2")
	publicaRevision(t, home, revUno, subidoAlaNube(t, home), "")

	code, out, errs := snapRun(t, "cloud", "agent", "--once")
	if code != 0 || !strings.Contains(out, "ccp cloud review") {
		t.Fatalf("agent: %d %q %q", code, out, errs)
	}
	if got, _ := os.ReadFile(filepath.Join(src, "CLAUDE.md")); string(got) != "uno" {
		t.Fatalf("las instrucciones se aplican solas; quedó %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(src, "hooks", "x.sh")); string(got) != "echo 2" {
		t.Fatalf("el hook espera confirmación; quedó %q", got)
	}

	// El --json de review enseña lo que espera, con su motivo.
	code, out, errs = snapRun(t, "cloud", "review", "--json")
	if code != 0 {
		t.Fatalf("review --json: %d %q %q", code, out, errs)
	}
	var r struct {
		Pending []struct {
			LPath string   `json:"lpath"`
			Why   []string `json:"why"`
		} `json:"pending"`
		Conflicts []any `json:"conflicts"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	if len(r.Pending) != 1 || r.Pending[0].LPath != "claude/hooks/x.sh" || len(r.Pending[0].Why) == 0 {
		t.Fatalf("esperaba el hook con su motivo: %+v", r)
	}
	// Una lista vacía es [] y nunca null, como en el resto del CLI.
	if r.Conflicts == nil || !strings.Contains(out, `"conflicts": []`) {
		t.Fatalf("sin choques, conflicts tiene que ser una lista vacía: %q", out)
	}

	// Sin terminal y sin decir si sí o si no, no se inventa la respuesta.
	if code, out, errs = snapRun(t, "cloud", "review"); code != 1 || !strings.Contains(errs, "--yes") {
		t.Fatalf("review sin respuesta: %d %q %q", code, out, errs)
	}
	if code, out, errs = snapRun(t, "cloud", "review", "--yes"); code != 0 {
		t.Fatalf("review --yes: %d %q %q", code, out, errs)
	}
	if got, _ := os.ReadFile(filepath.Join(src, "hooks", "x.sh")); string(got) != "echo 1" {
		t.Fatalf("lo confirmado se aplica; quedó %q", got)
	}
	if code, out, errs = snapRun(t, "cloud", "review"); code != 0 || !strings.Contains(out, "Nada") && !strings.Contains(out, "nada") {
		t.Fatalf("ya no queda nada: %d %q %q", code, out, errs)
	}
}

func TestCloudPolicy(t *testing.T) {
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")
	snapEnv(t)
	if code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac"); code != 0 {
		t.Fatalf("login: %d %q %q", code, out, errs)
	}
	if code, out, _ := snapRun(t, "cloud", "policy"); code != 0 || !strings.Contains(out, "auto") {
		t.Fatalf("por defecto es auto: %d %q", code, out)
	}
	if code, out, errs := snapRun(t, "cloud", "policy", "manual"); code != 0 || !strings.Contains(out, "manual") {
		t.Fatalf("policy manual: %d %q %q", code, out, errs)
	}
	if code, out, _ := snapRun(t, "cloud", "status"); code != 0 || !strings.Contains(out, "manual") {
		t.Fatalf("status tiene que decirlo: %d %q", code, out)
	}
	if code, _, errs := snapRun(t, "cloud", "policy", "loquesea"); code != 1 || !strings.Contains(errs, "auto") {
		t.Fatalf("una política inventada se rechaza: %d %q", code, errs)
	}
}

// TestPrintOutcomeTraduceMotivos fija que el motivo de un «sin aplicar» pasa
// por el catálogo: el campo Reason es un código estable, no prosa, así que en
// inglés no puede salir ni español ni el código crudo.
func TestPrintOutcomeTraduceMotivos(t *testing.T) {
	casos := []struct {
		code string
		want string
	}{
		{agent.ReasonNoDelete, "ccp does not delete files when restoring"},
		{agent.ReasonNoCloudData, "its data is not in the cloud"},
		{agent.ReasonNotConfirmed, "it was not confirmed on this machine"},
		{"missing_blob", "the snapshot has no data for it"},
		{"project_missing", "the project folder does not exist on this machine"},
	}
	for _, cs := range casos {
		var buf strings.Builder
		c := cloudCmd{lang: i18n.En, out: &buf, err: &buf}
		c.printOutcome(&agent.Outcome{Skipped: []agent.Skipped{{LPath: "claude/hooks/x.sh", Reason: cs.code}}})
		got := buf.String()
		if !strings.Contains(got, cs.want) {
			t.Fatalf("motivo %q: se esperaba %q en %q", cs.code, cs.want, got)
		}
		if strings.Contains(got, cs.code) {
			t.Fatalf("motivo %q: salió el código crudo en %q", cs.code, got)
		}
	}
}
