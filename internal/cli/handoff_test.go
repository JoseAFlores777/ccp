package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

func TestHandoffStatusEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	var out, errb bytes.Buffer
	code := Dispatch([]string{"handoff", "status"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (sin activo)", code)
	}
	if !strings.Contains(out.String()+errb.String(), "activo") {
		t.Fatalf("salida inesperada: %q / %q", out.String(), errb.String())
	}
}

func TestHandoffForwardShellOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	var out, errb bytes.Buffer
	// `ccp handoff <perfil>` directo al binario (sin función shell) = shell-only.
	code := Dispatch([]string{"handoff", "emco-cc"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestHandoffEmitForward(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	cfg := &core.Config{Version: core.SchemaVersion,
		Profiles: map[string]core.Profile{"personal-cc": {Type: "official"}, "emco-cc": {Type: "official"}}}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	cwd := "/repo"
	slug := core.SlugForCwd(cwd)
	uuid := "abababab-abab-4bab-8bab-abababababab"
	dir := core.ProjectDir(home+"/profiles/personal-cc/cc-home", slug)
	_ = writeJSONLForTest(t, dir, uuid) // helper local (ver Step 3)

	t.Setenv("CCP_PROFILE", "personal-cc")
	var out, errb bytes.Buffer
	// _handoff <pwd> <to> --session <uuid>
	code := Dispatch([]string{"_handoff", cwd, "emco-cc", "--session", uuid}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "CCP_RESUME_ID="+uuid) {
		t.Fatalf("emit sin resume id: %s", out.String())
	}
}

func writeJSONLForTest(t *testing.T, dir, uuid string) error {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	line := `{"type":"ai-title","aiTitle":"T","sessionId":"` + uuid + `"}` + "\n" +
		`{"type":"user","sessionId":"` + uuid + `","cwd":"/repo"}` + "\n"
	return os.WriteFile(dir+"/"+uuid+".jsonl", []byte(line), 0o644)
}
