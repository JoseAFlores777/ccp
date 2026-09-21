package cli

// serve_cloud_test.go — los métodos de nube de `ccp serve` (P-21). Nada de red:
// sin sesión el motor contesta con lo que hay en disco, que es justo el estado
// en el que la pantalla se abre la primera vez.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/agent"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
)

// writeReview deja un review.json como el que dejaría el agente al encontrar
// algo ejecutable en una revisión.
func writeReview(t *testing.T, home string, r agent.Review) {
	t.Helper()
	dir := filepath.Join(home, "cloud")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, agent.ReviewFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestServeCloudStatusSinSesion(t *testing.T) {
	serveEnv(t)
	_, got := serveRun(t, req(1, "cloud.status", nil))
	var st struct {
		LoggedIn bool   `json:"logged_in"`
		Vault    string `json:"vault"`
		Policy   string `json:"policy"`
		Pending  int    `json:"pending_review"`
	}
	mustResult(t, got["1"], &st)
	if st.LoggedIn {
		t.Errorf("sin config.json no hay sesión: %+v", st)
	}
	if st.Vault != "unknown" {
		t.Errorf("la bóveda de un equipo sin sesión es desconocida, no %q", st.Vault)
	}
	if st.Policy != agent.PolicyAuto {
		t.Errorf("la política por defecto es auto, no %q", st.Policy)
	}
}

func TestServeCloudReviewYPendientes(t *testing.T) {
	home := serveEnv(t)
	writeReview(t, home, agent.Review{
		Revision: "rev-1", Snapshot: "snap-1",
		Pending: []agent.Pending{{LPath: "claude/settings.json", Why: []agent.Danger{agent.DangerHooks}}},
	})
	_, got := serveRun(t, req(1, "cloud.review", nil), req(2, "cloud.status", nil))
	var r struct {
		Revision  string          `json:"revision"`
		Pending   []agent.Pending `json:"pending"`
		Conflicts []any           `json:"conflicts"`
		Applied   []string        `json:"applied"`
	}
	mustResult(t, got["1"], &r)
	if r.Revision != "rev-1" || len(r.Pending) != 1 || r.Pending[0].LPath != "claude/settings.json" {
		t.Fatalf("cloud.review: %+v", r)
	}
	if r.Pending[0].Why[0] != agent.DangerHooks {
		t.Errorf("el motivo viaja tal cual: %+v", r.Pending[0])
	}
	// Las listas vacías son [] y nunca null: la pantalla hace .length sobre ellas.
	if r.Conflicts == nil || r.Applied == nil {
		t.Errorf("listas nulas en cloud.review: %s", got["1"].Result)
	}
	var st struct {
		Pending int `json:"pending_review"`
	}
	mustResult(t, got["2"], &st)
	if st.Pending != 1 {
		t.Errorf("status debe contar lo que espera confirmación: %+v", st)
	}
}

func TestServeCloudReviewVacio(t *testing.T) {
	serveEnv(t)
	_, got := serveRun(t, req(1, "cloud.review", nil))
	var r struct {
		Revision string          `json:"revision"`
		Pending  []agent.Pending `json:"pending"`
	}
	mustResult(t, got["1"], &r)
	if r.Revision != "" || len(r.Pending) != 0 {
		t.Errorf("sin review.json no hay nada que confirmar: %+v", r)
	}
}

func TestServeCloudPoliticaYResolveSinSesion(t *testing.T) {
	home := serveEnv(t)
	_, got := serveRun(t,
		req(1, "cloud.setPolicy", map[string]any{"policy": "manual"}),
		req(2, "cloud.setPolicy", map[string]any{"policy": "loquesea"}),
	)
	if got["1"].Error != nil {
		t.Fatalf("setPolicy: %+v", got["1"].Error)
	}
	if got["2"].Error == nil || got["2"].Error.Code != "invalid_params" {
		t.Errorf("una política inventada se rechaza: %+v", got["2"])
	}
	cfg, err := client.NewFiles(home).LoadConfig()
	if err != nil {
		t.Fatalf("la política se guarda aunque no haya sesión: %v", err)
	}
	if cfg.Policy != agent.PolicyManual {
		t.Errorf("política guardada: %q", cfg.Policy)
	}
	// Confirmar exige hablar con el servidor: sin sesión falla, y lo dice.
	_, got = serveRun(t, req(1, "cloud.reviewResolve", map[string]any{"approve": []string{"claude/settings.json"}}))
	if got["1"].Error == nil {
		t.Errorf("reviewResolve sin sesión debe fallar: %+v", got["1"])
	}
}
