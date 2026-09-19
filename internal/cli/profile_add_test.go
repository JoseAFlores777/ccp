package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// profile_add_test.go — la cara CLI del alta a medias (B8). Desde que `profile
// add` genera la config, puede fallar DESPUÉS de haber guardado el perfil. El
// CLI tiene que decirlo en el idioma del usuario —el perfil existe y se arregla
// con `ccp profile sync`— en vez de reenviar la prosa castellana del core.
func TestProfileAddConfigFallidaDiceComoReintentar(t *testing.T) {
	casos := []struct {
		lang, flag, want string
	}{
		{"en", "--official", "Profile 'p' was created, but its config could not be generated"},
		{"es", "--official", "El perfil 'p' se creó, pero no se pudo generar su config"},
		{"en", "--deepseek", "Profile 'p' was created, but its config could not be generated"},
	}
	for _, c := range casos {
		t.Run(c.lang+c.flag, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("CCP_HOME", home)
			t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
			t.Setenv("CCP_LANG", c.lang)
			t.Setenv("NO_COLOR", "1")
			// overlay/ ocupado por un archivo: la config no se puede generar.
			if err := os.MkdirAll(filepath.Join(home, "profiles", "p"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "profiles", "p", "overlay"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			var out, errb bytes.Buffer
			code := Dispatch([]string{"profile", "add", "p", c.flag}, &out, &errb)
			if code != 1 {
				t.Fatalf("exit = %d, quiero 1\nstdout: %s\nstderr: %s", code, out.String(), errb.String())
			}
			if !strings.Contains(errb.String(), c.want) || !strings.Contains(errb.String(), "ccp profile sync p") {
				t.Errorf("stderr no dice que el perfil existe ni cómo reintentar:\n%s", errb.String())
			}
			if c.lang == "en" && strings.Contains(errb.String(), "creado, pero") {
				t.Errorf("en inglés reenvía el marco castellano del core:\n%s", errb.String())
			}
			cfg, err := core.Load(home)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := cfg.Profiles["p"]; !ok {
				t.Error("el perfil no quedó en ccp.yaml")
			}
		})
	}
}
