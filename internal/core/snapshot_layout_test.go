package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// snapFixture monta una máquina falsa completa:
//   - un ~/.claude con configuración y una caché de plugins;
//   - un ~/.claude.json con MCP y estado;
//   - un perfil official y otro deepseek;
//   - la config de las ventanas de Desktop;
//   - un repo con regla de carpeta y archivos locales.
func snapFixture(t *testing.T) (home, src, repo string) {
	t.Helper()
	home = t.TempDir()
	src = filepath.Join(t.TempDir(), ".claude")
	desktopDefault := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	t.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", desktopDefault)
	t.Setenv("HOME", t.TempDir())
	write := func(p, s string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(src, "settings.json"), `{"theme":"dark"}`, 0o644)
	write(filepath.Join(src, "agents", "revisor.md"), "# revisor\n", 0o644)
	write(filepath.Join(src, "plugins", "installed_plugins.json"), `{"plugins":{}}`, 0o644)
	write(filepath.Join(src, "plugins", "cache", "x", "big.bin"), "cache", 0o644)
	write(src+".json", liveClaudeJSON, 0o600)
	if err := os.Symlink(filepath.Join(src, "agents", "revisor.md"), filepath.Join(src, "agents", "alias.md")); err != nil {
		t.Fatal(err)
	}

	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	if err := CfgInitOverlay(home, "work"); err != nil {
		t.Fatal(err)
	}
	write(cfgInstrFile(home, "work"), "# work\n", 0o644)
	write(filepath.Join(ccHomePath(home, "work"), ".claude.json"), `{"machineID":"w","mcpServers":{"jira":{"type":"http","url":"https://j"}}}`, 0o600)
	write(filepath.Join(DesktopDataDir(home, "work"), "claude_desktop_config.json"), `{"mcpServers":{}}`, 0o600)
	if err := ProfileAddDeepseek(home, "deep", BuiltinDefaults()); err != nil {
		t.Fatal(err)
	}
	if err := ProfileSetKey(home, "deep", "sk-1"); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(desktopDefault, "claude_desktop_config.json"), `{"mcpServers":{"obsidian":{"command":"node"}}}`, 0o600)

	repo = t.TempDir()
	write(filepath.Join(repo, ".git", "config"), "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = git@github.com:Org/App.git\n", 0o644)
	write(filepath.Join(repo, ".claude", "settings.local.json"), `{"permissions":{"allow":["Bash(make)"]}}`, 0o644)
	if _, err := RuleSet(home, repo, "work"); err != nil {
		t.Fatal(err)
	}
	return home, src, repo
}

func sourcesByLPath(t *testing.T, srcs []snapshot.Source) map[string]snapshot.Source {
	t.Helper()
	out := map[string]snapshot.Source{}
	for _, s := range srcs {
		out[s.LPath] = s
	}
	return out
}

func TestSnapshotSources(t *testing.T) {
	home, src, repo := snapFixture(t)
	srcs, err := SnapshotSources(home, src, SnapshotSourceOpts{})
	if err != nil {
		t.Fatalf("SnapshotSources: %v", err)
	}
	got := sourcesByLPath(t, srcs)
	key := projectKey(repo, "git@github.com:Org/App.git")
	want := map[string]snapshot.Class{
		"ccp/ccp.yaml":                                    snapshot.ClassAuthored,
		"ccp/profiles/work/overlay/CLAUDE.md":             snapshot.ClassAuthored,
		"ccp/profiles/work/cc-home/.claude.json":          snapshot.ClassSecret,
		"ccp/profiles/deep/api_key":                       snapshot.ClassSecret,
		"desktop/work/claude_desktop_config.json":         snapshot.ClassSecret,
		"desktop/default/claude_desktop_config.json":      snapshot.ClassSecret,
		"claude/settings.json":                            snapshot.ClassAuthored,
		"claude/agents/revisor.md":                        snapshot.ClassAuthored,
		"claude/plugins/installed_plugins.json":           snapshot.ClassAuthored,
		"claude/.claude.json":                             snapshot.ClassSecret,
		"project/" + key + "/.claude/settings.local.json": snapshot.ClassAuthored,
	}
	for l, class := range want {
		s, ok := got[l]
		if !ok {
			t.Errorf("falta %s", l)
			continue
		}
		if s.Class != class {
			t.Errorf("%s: clase %s, quiero %s", l, s.Class, class)
		}
	}
	for l := range got {
		for _, forbidden := range []string{"plugins/cache", "agents/alias.md", "cc-home/agents", "cc-home/settings.json", "cc-home/CLAUDE.md", "handoffs.yaml", "last-settings", "settings.invalid"} {
			if strings.Contains(l, forbidden) {
				t.Errorf("se capturó %s, que no es configuración del usuario", l)
			}
		}
	}
	p := got["project/"+key+"/.claude/settings.local.json"]
	if p.Meta["path"] != repo || p.Meta["remote"] != "git@github.com:Org/App.git" {
		t.Errorf("meta del proyecto = %v", p.Meta)
	}
	// De ~/.claude.json solo viaja la configuración.
	data, err := got["claude/.claude.json"].Read()
	if err != nil || strings.Contains(string(data), "machineID") || !strings.Contains(string(data), "github") {
		t.Errorf("claude/.claude.json = %s, %v", data, err)
	}
}

func TestSnapshotSourcesWithState(t *testing.T) {
	home, src, _ := snapFixture(t)
	transcript := filepath.Join(ccHomePath(home, "work"), "projects", "-repo", "abc.jsonl")
	os.MkdirAll(filepath.Dir(transcript), 0o755)
	os.WriteFile(transcript, []byte(`{"uuid":"1"}`+"\n"), 0o600)
	os.WriteFile(handoffsPath(home), []byte("version: 2\n"), 0o644)

	without, _ := SnapshotSources(home, src, SnapshotSourceOpts{})
	with, err := SnapshotSources(home, src, SnapshotSourceOpts{WithState: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range []string{"ccp/profiles/work/cc-home/projects/-repo/abc.jsonl", "ccp/handoffs.yaml"} {
		if _, ok := sourcesByLPath(t, without)[l]; ok {
			t.Errorf("%s capturado sin WithState", l)
		}
		if s, ok := sourcesByLPath(t, with)[l]; !ok || s.Class != snapshot.ClassState {
			t.Errorf("%s con WithState: %+v, %v", l, s, ok)
		}
	}
}

func TestProjectKey(t *testing.T) {
	same := []string{"git@github.com:Org/App.git", "https://github.com/org/app", "ssh://git@github.com/Org/App.git"}
	for _, r := range same[1:] {
		if projectKey("/a", r) != projectKey("/b", same[0]) {
			t.Errorf("%s y %s deberían ser el mismo proyecto", r, same[0])
		}
	}
	if projectKey("/a", "") == projectKey("/b", "") {
		t.Error("sin remoto, dos rutas distintas deben ser proyectos distintos")
	}
}

func TestSnapshotCapture(t *testing.T) {
	home, src, _ := snapFixture(t)
	st, err := OpenSnapshotStore(home)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	m, err := SnapshotCapture(home, src, st, SnapshotCaptureOpts{Trigger: "manual", Now: now, Machine: "test"})
	if err != nil {
		t.Fatalf("SnapshotCapture: %v", err)
	}
	if m.Trigger != "manual" || m.CCPVersion != Version || len(m.Items) < 10 || m.Home != os.Getenv("HOME") {
		t.Fatalf("m = %+v", m)
	}
	live, err := SnapshotLive(home, src, SnapshotSourceOpts{})
	if err != nil || len(snapshot.Diff(m.Items, live)) != 0 {
		t.Fatalf("recién capturado, el estado vivo debe ser idéntico: %v", err)
	}
}
