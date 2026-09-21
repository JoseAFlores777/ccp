package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

func TestTranslateHome(t *testing.T) {
	in := "rules:\n  - path: /Users/ana/app\n    profile: work\n  - path: /Users/ana\n" +
		`{"command":"/Users/ana/bin/x","cwd":"/Users/ana","other":"/Users/anabel/y"}` + "\n"
	got := string(translateHome([]byte(in), "/Users/ana", "/home/jose"))
	for _, want := range []string{"/home/jose/app", "path: /home/jose\n", `"/home/jose/bin/x"`, `"cwd":"/home/jose"`, "/Users/anabel/y"} {
		if !strings.Contains(got, want) {
			t.Errorf("falta %q en:\n%s", want, got)
		}
	}
	if string(translateHome([]byte(in), "/Users/ana", "/Users/ana")) != in {
		t.Error("con el mismo HOME no debe cambiar nada")
	}
	bin := []byte{0xff, 0xfe, '/', 'U'}
	if string(translateHome(bin, "/U", "/X")) != string(bin) {
		t.Error("tocó un contenido que no es texto")
	}
}

func TestTranslateHomePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/Users/ana", "/home/jose"},
		{"/Users/ana/x", "/home/jose/x"},
		{"/Users/anabel", "/Users/anabel"},
		{"/otro", "/otro"},
	}
	for _, c := range cases {
		if got := translateHomePath(c.in, "/Users/ana", "/home/jose"); got != c.want {
			t.Errorf("translateHomePath(%q) = %q; quiero %q", c.in, got, c.want)
		}
	}
	if got := translateHomePath("/Users/ana/x", "/Users/ana", "/Users/ana"); got != "/Users/ana/x" {
		t.Errorf("con el mismo HOME no debe cambiar nada: %q", got)
	}
}

// Un snapshot hecho con otro HOME se restaura con las rutas de esta máquina:
// la regla de carpeta apunta al proyecto aquí, y el proyecto se encuentra.
func TestSnapshotRestoreTranslatesHome(t *testing.T) {
	home, src, _ := snapFixture(t)
	newHome := os.Getenv("HOME") // snapFixture lo fijó a un temporal
	st, err := OpenSnapshotStore(home)
	if err != nil {
		t.Fatal(err)
	}

	oldHome := "/Users/ana"
	yaml := "version: 2\nprofiles:\n  work:\n    type: official\nrules:\n  - path: " + oldHome + "/proyectos/app\n    profile: work\n"
	local := `{"permissions":{"allow":["Bash(make)"]}}`
	hy, err := st.PutBlob([]byte(yaml), false)
	if err != nil {
		t.Fatal(err)
	}
	hl, err := st.PutBlob([]byte(local), false)
	if err != nil {
		t.Fatal(err)
	}
	key := projectKey(oldHome+"/proyectos/app", "")
	m := &snapshot.Manifest{
		Format: snapshot.FormatVersion, Created: snapNow, Machine: "mac-de-ana", Home: oldHome,
		CCPVersion: Version, Trigger: "manual",
		Items: []snapshot.Item{
			{LPath: "ccp/ccp.yaml", Hash: hy, Size: int64(len(yaml)), Mode: 0o644, Class: snapshot.ClassAuthored},
			{LPath: "project/" + key + "/.claude/settings.local.json", Hash: hl, Size: int64(len(local)), Mode: 0o644,
				Class: snapshot.ClassAuthored, Meta: map[string]string{"path": oldHome + "/proyectos/app"}},
		},
	}
	if err := st.SaveManifest(m); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(newHome, "proyectos", "app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}

	rep, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{Now: snapNow.Add(time.Hour), Machine: "mac-de-jose"})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if rep.HomeFrom != oldHome || rep.HomeTo != newHome {
		t.Errorf("HomeFrom/HomeTo = %q/%q", rep.HomeFrom, rep.HomeTo)
	}
	cfg, err := Load(home)
	if err != nil || len(cfg.Rules) != 1 || cfg.Rules[0].Path != app {
		t.Fatalf("reglas tras restaurar = %+v, %v; quiero %s", cfg.Rules, err, app)
	}
	if got := readStr(t, filepath.Join(app, ".claude", "settings.local.json")); got != local {
		t.Fatalf("settings.local.json = %q", got)
	}
}
