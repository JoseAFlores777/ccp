package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// serveEnv monta un CCP_HOME con el perfil official «work», un HOME falso y los
// directorios de Desktop fuera de la máquina real.
func serveEnv(t *testing.T) string {
	t.Helper()
	home := homeConPerfil(t, "work", "official")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", t.TempDir())
	t.Setenv("CCP_DESKTOP_APPS_DIR", t.TempDir())
	t.Setenv("CCP_LANG", "es")
	return home
}

type serveReply struct {
	Event  string          `json:"event"`
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *serveError     `json:"error"`
}

// serveRun manda las peticiones de una vez y devuelve las respuestas por id. Las
// peticiones corren en paralelo, así que el orden de las respuestas no importa.
func serveRun(t *testing.T, reqs ...string) (ready serveReply, byID map[string]serveReply) {
	t.Helper()
	var out, errb bytes.Buffer
	if code := serveLoop(strings.NewReader(strings.Join(reqs, "\n")+"\n"), &out, &errb); code != 0 {
		t.Fatalf("serveLoop salió con %d: %s", code, errb.String())
	}
	byID = map[string]serveReply{}
	for i, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r serveReply
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("línea %d no es JSON: %q", i, line)
		}
		if i == 0 {
			ready = r
			continue
		}
		byID[string(r.ID)] = r
	}
	return ready, byID
}

func req(id int, method string, params any) string {
	p, _ := json.Marshal(params)
	return fmt.Sprintf(`{"id":%d,"method":%q,"params":%s}`, id, method, p)
}

func mustResult(t *testing.T, r serveReply, into any) {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("error inesperado: %s: %s", r.Error.Code, r.Error.Message)
	}
	if err := json.Unmarshal(r.Result, into); err != nil {
		t.Fatalf("resultado ilegible: %v (%s)", err, r.Result)
	}
}

func TestServeProtocolo(t *testing.T) {
	serveEnv(t)
	ready, got := serveRun(t,
		`{"id":1,"method":"no.existe"}`,
		`{esto no es json`,
		`{"id":"abc","method":"app.info"}`,
	)
	if ready.Event != "ready" {
		t.Fatalf("la primera línea debe ser el evento ready: %+v", ready)
	}
	if r := got["1"]; r.Error == nil || r.Error.Code != "unknown_method" {
		t.Errorf("método desconocido: %+v", r)
	}
	if r := got["null"]; r.Error == nil || r.Error.Code != "parse_error" {
		t.Errorf("JSON roto: %+v", r)
	}
	var info struct {
		Protocol int    `json:"protocol"`
		Home     string `json:"home"`
	}
	mustResult(t, got[`"abc"`], &info)
	if info.Protocol != serveProtocol || info.Home != os.Getenv("CCP_HOME") {
		t.Errorf("app.info: %+v", info)
	}
}

func TestServePerfiles(t *testing.T) {
	serveEnv(t)
	_, got := serveRun(t, req(1, "profiles.add", map[string]any{
		"name": "ds", "type": "deepseek", "base_url": "https://ejemplo.test/anthropic",
	}))
	if got["1"].Error != nil {
		t.Fatalf("add: %+v", got["1"].Error)
	}
	_, got = serveRun(t,
		req(1, "profiles.setKey", map[string]any{"name": "ds", "key": "sk-1"}),
		req(2, "profiles.update", map[string]any{"name": "ds", "model_pro": "modelo-nuevo"}),
		req(3, "profiles.update", map[string]any{"name": "work", "model_pro": "x"}),
		req(4, "profiles.add", map[string]any{"name": "default", "type": "official"}),
	)
	if got["1"].Error != nil || got["2"].Error != nil {
		t.Fatalf("setKey/update: %+v %+v", got["1"].Error, got["2"].Error)
	}
	if got["3"].Error == nil {
		t.Error("un perfil official no tiene modelos que editar")
	}
	if got["4"].Error == nil {
		t.Error("default es reservado")
	}
	_, got = serveRun(t, req(1, "profiles.list", nil))
	var list []srvProfile
	mustResult(t, got["1"], &list)
	if len(list) != 3 || list[0].Name != "default" {
		t.Fatalf("default primero y luego los demás: %+v", list)
	}
	var ds *srvProfile
	for i := range list {
		if list[i].Name == "ds" {
			ds = &list[i]
		}
	}
	if ds == nil || ds.Access != "ok" || ds.ModelPro != "modelo-nuevo" || ds.BaseURL != "https://ejemplo.test/anthropic" {
		t.Errorf("perfil ds mal: %+v", ds)
	}
	if ds.Desktop.Eligible {
		t.Error("un proveedor no puede tener ventana de Desktop")
	}
}

func TestServeReglasYResolve(t *testing.T) {
	serveEnv(t)
	base := t.TempDir()
	sub := filepath.Join(base, "cliente")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	_, got := serveRun(t, req(1, "rules.set", map[string]any{"path": base, "profile": "work"}))
	if got["1"].Error != nil {
		t.Fatal(got["1"].Error)
	}
	_, got = serveRun(t, req(1, "rules.set", map[string]any{"path": sub, "profile": "default"}))
	if got["1"].Error != nil {
		t.Fatal(got["1"].Error)
	}
	_, got = serveRun(t,
		req(1, "resolve", map[string]any{"path": filepath.Join(sub, "src")}),
		req(2, "rules.list", nil),
	)
	var res struct {
		Profile  string                  `json:"profile"`
		Rule     struct{ Path string }   `json:"rule"`
		Shadowed []struct{ Path string } `json:"shadowed"`
	}
	mustResult(t, got["1"], &res)
	if res.Profile != "default" || res.Rule.Path != sub || len(res.Shadowed) != 1 || res.Shadowed[0].Path != base {
		t.Errorf("la regla más profunda gana y la del padre queda descartada: %+v", res)
	}
	var rules []struct {
		Path   string `json:"path"`
		Depth  int    `json:"depth"`
		Parent string `json:"parent"`
		Exists bool   `json:"exists"`
	}
	mustResult(t, got["2"], &rules)
	if len(rules) != 2 || rules[1].Depth != 1 || rules[1].Parent != base || !rules[1].Exists {
		t.Errorf("la subcarpeta es una excepción de su padre: %+v", rules)
	}
}

func TestServeRotacion(t *testing.T) {
	serveEnv(t)
	_, got := serveRun(t, req(1, "auto.status", nil))
	var st map[string]any
	mustResult(t, got["1"], &st)
	if st["present"] != false {
		t.Fatalf("sin bloque auto_handoff: %+v", st)
	}
	_, got = serveRun(t, req(1, "auto.init", nil))
	if got["1"].Error != nil {
		t.Fatal(got["1"].Error)
	}
	_, got = serveRun(t,
		req(1, "auto.policy", map[string]any{"threshold": 80, "max_hops": 2}),
		req(2, "auto.policy", map[string]any{"min_dwell": "no-es-una-duración"}),
		req(3, "auto.allow", map[string]any{"allow_from": map[string][]string{"work": {"fantasma"}}}),
	)
	if got["1"].Error != nil {
		t.Fatal(got["1"].Error)
	}
	if got["2"].Error == nil {
		t.Error("una duración inválida no se guarda")
	}
	if got["3"].Error == nil {
		t.Error("un permiso hacia un perfil que no existe no se guarda")
	}
	_, got = serveRun(t, req(1, "auto.allow", map[string]any{"allow_from": nil}))
	if got["1"].Error != nil {
		t.Fatal(got["1"].Error)
	}
	_, got = serveRun(t, req(1, "auto.status", nil))
	var st2 struct {
		Params struct {
			Threshold int `json:"threshold"`
			MaxHops   int `json:"max_hops"`
		} `json:"params"`
		AllowDeclared bool `json:"allow_declared"`
	}
	mustResult(t, got["1"], &st2)
	if st2.Params.Threshold != 80 || st2.Params.MaxHops != 2 || st2.AllowDeclared {
		t.Errorf("estado tras editar: %+v", st2)
	}
	cfg, err := core.Load(os.Getenv("CCP_HOME"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutoHandoff.AllowFrom != nil {
		t.Error("allow_from null debe quitar el control de permisos")
	}
}

func TestServeConversaciones(t *testing.T) {
	serveEnv(t)
	cc, err := core.CCHome(os.Getenv("CCP_HOME"), "work")
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	dir := core.ProjectDir(cc, core.SlugForCwd(repo))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	uuid := "11111111-2222-4333-8444-555555555555"
	body := fmt.Sprintf(`{"type":"user","uuid":"m1","cwd":%q}`+"\n"+`{"type":"custom-title","customTitle":"Mi conversación"}`+"\n", repo)
	if err := os.WriteFile(filepath.Join(dir, uuid+".jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, got := serveRun(t,
		req(1, "conversations.list", nil),
		req(2, "conversations.list", map[string]any{"cwd": t.TempDir()}),
	)
	var all struct {
		Total int               `json:"total"`
		Items []srvConversation `json:"items"`
	}
	mustResult(t, got["1"], &all)
	if all.Total != 1 || all.Items[0].Title != "Mi conversación" || all.Items[0].Profile != "work" || all.Items[0].Cwd != repo {
		t.Errorf("listado: %+v", all)
	}
	var none struct {
		Total int `json:"total"`
	}
	mustResult(t, got["2"], &none)
	if none.Total != 0 {
		t.Errorf("el filtro por carpeta debe dejar fuera las de otra carpeta: %+v", none)
	}
}

func TestServeSnapshot(t *testing.T) {
	serveEnv(t)
	os.WriteFile(filepath.Join(os.Getenv("CCP_CLAUDE_SRC"), "settings.json"), []byte(`{}`), 0o644)

	_, r := serveRun(t, req(1, "snapshot.create", map[string]any{"label": "desde la gui"}))
	if r["1"].Error != nil {
		t.Fatalf("snapshot.create: %+v", r["1"].Error)
	}
	_, r = serveRun(t, req(2, "snapshot.list", nil))
	var list []snapSummary
	if err := json.Unmarshal(r["2"].Result, &list); err != nil || len(list) != 1 || list[0].Label != "desde la gui" {
		t.Fatalf("snapshot.list = %s, %v", r["2"].Result, err)
	}
	_, r = serveRun(t, req(3, "snapshot.restore", map[string]any{"id": "latest", "dry_run": true}))
	var plan core.SnapshotRestoreReport
	if err := json.Unmarshal(r["3"].Result, &plan); err != nil || len(plan.Steps) == 0 || plan.PreSnapshot != "" {
		t.Fatalf("snapshot.restore dry_run = %s, %v", r["3"].Result, err)
	}
	_, r = serveRun(t, req(4, "snapshot.show", map[string]any{}))
	if r["4"].Error == nil || r["4"].Error.Code != "invalid_params" {
		t.Fatalf("snapshot.show sin id: %+v", r["4"])
	}
}

// La GUI borra perfiles por serve: también ahí queda la red antes de borrar.
func TestServeProfileRemoveTakesSafetySnapshot(t *testing.T) {
	home := serveEnv(t)
	t.Setenv("CCP_NO_AUTO_SNAPSHOT", "")
	_, r := serveRun(t, req(1, "profiles.remove", map[string]any{"name": "work"}))
	if r["1"].Error != nil {
		t.Fatalf("profiles.remove: %+v", r["1"].Error)
	}
	st, _ := core.OpenSnapshotStore(home)
	ms, err := st.List()
	if err != nil || len(ms) == 0 || ms[0].Trigger != "pre-profile-rm" {
		t.Fatalf("tras profiles.remove: %d snapshots (%v)", len(ms), err)
	}
}

// profiles.rename dice si hay que volver a iniciar sesión (B7): la GUI lo
// enseña al confirmar. "ok" sigue ahí para no cambiar la forma de lo que ya se
// respondía, y "relogin" sale siempre, también en false, para que la GUI no
// tenga que distinguir «no hace falta» de «este ccp no lo sabe decir».
func TestServeProfilesRenameDiceSiHayQueVolverAEntrar(t *testing.T) {
	home := serveEnv(t)
	cj := filepath.Join(home, "profiles", "work", "cc-home", ".claude.json")
	if err := os.WriteFile(cj, []byte(`{"oauthAccount":{"emailAddress":"a@b"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := core.ProfileAddOfficial(home, "sinlogin"); err != nil {
		t.Fatal(err)
	}
	_, r := serveRun(t,
		req(1, "profiles.rename", map[string]any{"from": "work", "to": "work2"}),
		req(2, "profiles.rename", map[string]any{"from": "sinlogin", "to": "sinlogin2"}),
	)
	var got struct {
		OK      bool `json:"ok"`
		Relogin bool `json:"relogin"`
	}
	mustResult(t, r["1"], &got)
	if !got.OK || !got.Relogin {
		t.Fatalf("profiles.rename = %+v, quiero ok y relogin", got)
	}
	var raw map[string]any
	mustResult(t, r["2"], &raw)
	if raw["ok"] != true || raw["relogin"] != false {
		t.Fatalf("profiles.rename sin login = %v, quiero ok:true y relogin:false", raw)
	}
}
