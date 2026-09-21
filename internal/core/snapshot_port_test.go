package core

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// portManifest arma un manifiesto de otra máquina con dos proyectos: uno con
// remoto (el mismo repo clonado aquí en otro sitio) y otro sin él.
func portManifest() *snapshot.Manifest {
	conRemoto := projectKey("/Users/ana/code/app", "git@github.com:Org/App.git")
	sinRemoto := projectKey("/Users/ana/notas", "")
	return &snapshot.Manifest{
		Format: snapshot.FormatVersion, ID: "aa", Home: "/Users/ana", Machine: "ana-mbp",
		Items: []snapshot.Item{
			{LPath: "ccp/ccp.yaml", Class: snapshot.ClassAuthored},
			{LPath: "project/" + conRemoto + "/.claude/settings.local.json", Class: snapshot.ClassAuthored,
				Meta: map[string]string{"path": "/Users/ana/code/app", "remote": "git@github.com:Org/App.git"}},
			{LPath: "project/" + conRemoto + "/CLAUDE.local.md", Class: snapshot.ClassAuthored,
				Meta: map[string]string{"path": "/Users/ana/code/app", "remote": "git@github.com:Org/App.git"}},
			{LPath: "project/" + sinRemoto + "/CLAUDE.local.md", Class: snapshot.ClassAuthored,
				Meta: map[string]string{"path": "/Users/ana/notas"}},
		},
	}
}

func mapByKey(ms []ProjectMapping) map[string]ProjectMapping {
	out := map[string]ProjectMapping{}
	for _, m := range ms {
		out[m.Key] = m
	}
	return out
}

func TestSnapshotProjectsEncuentraElRepoPorSuRemoto(t *testing.T) {
	m := portManifest()
	in := ProjectMapInputs{
		HomeTo:     "/home/jose",
		Candidates: []string{"/home/jose/src/app", "/home/jose/src/otro"},
		IsDir:      func(p string) bool { return p == "/home/jose/src/app" || p == "/home/jose/src/otro" },
		Remote: func(p string) string {
			if p == "/home/jose/src/app" {
				return "https://github.com/org/app"
			}
			return ""
		},
	}
	got := mapByKey(SnapshotProjects(m, in))
	app := got[projectKey("/Users/ana/code/app", "git@github.com:Org/App.git")]
	if app.Source != ProjectMapRemote || app.Path != "/home/jose/src/app" {
		t.Errorf("el repo con remoto = %+v; quiero encontrarlo por su remoto", app)
	}
	if app.From != "/home/jose/code/app" {
		t.Errorf("From = %q; la ruta de origen se traduce al HOME de aquí", app.From)
	}
	if len(app.Files) != 2 {
		t.Errorf("Files = %v; el mapeo decide por 2 rutas lógicas", app.Files)
	}
	notas := got[projectKey("/Users/ana/notas", "")]
	if notas.Source != ProjectMapMissing || notas.Path != "" {
		t.Errorf("la carpeta sin git y sin ruta aquí = %+v; quiero missing", notas)
	}
}

func TestSnapshotProjectsPrefiereLaRutaQueYaExisteYLoManualManda(t *testing.T) {
	m := portManifest()
	clave := projectKey("/Users/ana/code/app", "git@github.com:Org/App.git")
	in := ProjectMapInputs{
		HomeTo: "/home/jose", Candidates: []string{"/home/jose/src/app"},
		IsDir:  func(p string) bool { return p == "/home/jose/code/app" || p == "/home/jose/src/app" },
		Remote: func(p string) string { return "https://github.com/org/app" },
	}
	// La ruta que traía el snapshot, ya traducida, existe aquí: es la que el
	// usuario ya usa, y buscar un clon cualquiera la desplazaría.
	if got := mapByKey(SnapshotProjects(m, in))[clave]; got.Source != ProjectMapSnapshot || got.Path != "/home/jose/code/app" {
		t.Errorf("con la ruta traducida existente = %+v; quiero quedarme en ella", got)
	}
	in.Manual = map[string]string{clave: "/home/jose/otro/sitio"}
	if got := mapByKey(SnapshotProjects(m, in))[clave]; got.Source != ProjectMapManual || got.Path != "/home/jose/otro/sitio" {
		t.Errorf("con mapeo a mano = %+v; lo que dice la persona manda", got)
	}
}

func TestSnapshotRestoreEscribeDondeDiceElMapeo(t *testing.T) {
	home, src, repo, st, m := captureFixture(t)
	// El repo se «mudó»: aquí está clonado en otro sitio, como en una máquina
	// nueva. Sin mapeo el proyecto se salta; con él, se escribe en el clon.
	otro := t.TempDir()
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	clave := projectKey(repo, "git@github.com:Org/App.git")
	o := SnapshotRestoreOpts{Now: snapNow.Add(time.Hour), Machine: "test",
		Projects: map[string]string{clave: otro}}
	rep, err := SnapshotRestore(home, src, st, m.ID, o)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	lpath := "project/" + clave + "/.claude/settings.local.json"
	if got := actions(rep)[lpath]; got != "write" {
		t.Fatalf("%s = %q; quiero que el mapeo lo lleve al clon", lpath, got)
	}
	if got := readStr(t, filepath.Join(otro, ".claude", "settings.local.json")); !strings.Contains(got, "Bash(make)") {
		t.Errorf("el archivo del proyecto no llegó al clon: %q", got)
	}
	// Y la ruta que se enseña es la de aquí, no la de la máquina de origen.
	for _, s := range rep.Steps {
		if s.LPath == lpath && s.Meta["path"] != otro {
			t.Errorf("Meta[path] = %q; quiero %q", s.Meta["path"], otro)
		}
	}
	_ = m
}

func TestSnapshotPendingDiceLoginsYComandosQueFaltan(t *testing.T) {
	home, src, _, st, _ := captureFixture(t)
	// Un MCP con un comando que aquí no existe y un hook que llama a otro.
	os.WriteFile(src+".json", []byte(`{"mcpServers":{"obsidian":{"command":"uvx","args":["x"]}}}`), 0o600)
	os.WriteFile(filepath.Join(src, "settings.json"),
		[]byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"/opt/falta.sh --ya"}]}]}}`), 0o644)
	m, err := SnapshotCapture(home, src, st, SnapshotCaptureOpts{Trigger: "manual", Now: snapNow.Add(time.Minute), Machine: "test"})
	if err != nil {
		t.Fatal(err)
	}
	in := SnapshotPendingInputs{
		LoggedIn: map[string]bool{"work": false},
		LookPath: func(b string) (string, error) {
			if b == "node" {
				return "/usr/bin/node", nil
			}
			return "", os.ErrNotExist
		},
		Projects: []ProjectMapping{{Key: "abc", Source: ProjectMapMissing, Clone: "git@github.com:Org/App.git"}},
	}
	got := SnapshotPending(st, m, in)
	kinds := map[string][]string{}
	for _, p := range got {
		kinds[p.Kind] = append(kinds[p.Kind], p.Name)
	}
	if !slices.Contains(kinds[RestorePendingLogin], "work") {
		t.Errorf("logins pendientes = %v; el perfil official sin sesión tiene que salir", kinds[RestorePendingLogin])
	}
	for _, want := range []string{"uvx", "/opt/falta.sh"} {
		if !slices.Contains(kinds[RestorePendingCommand], want) {
			t.Errorf("comandos pendientes = %v; falta %q", kinds[RestorePendingCommand], want)
		}
	}
	if slices.Contains(kinds[RestorePendingCommand], "node") {
		t.Error("un comando que sí está no es un pendiente")
	}
	if !slices.Contains(kinds[RestorePendingProject], "abc") {
		t.Errorf("proyectos pendientes = %v; el que no está aquí tiene que salir", kinds[RestorePendingProject])
	}
	// Dos veces el mismo comando es un pendiente, no dos.
	n := 0
	for _, p := range got {
		if p.Name == "uvx" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("uvx sale %d veces; se agrupa por comando", n)
	}
}
