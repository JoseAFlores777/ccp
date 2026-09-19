package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// Un alta a medias (B8: el perfil quedó en ccp.yaml, pero su config no se pudo
// generar) no puede perder la API key que el usuario tecleó en el formulario: el
// perfil existe, así que la key se guarda igual, y el error dice —en su idioma—
// cómo terminar el alta.
func TestCreateProfileConfigFallidaGuardaLaKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	if err := os.MkdirAll(filepath.Join(home, "profiles", "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "profiles", "p", "overlay"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := createProfile(home, i18n.En, "deepseek", "p", core.BuiltinDefaults(), "sk-prueba")
	if err == nil {
		t.Fatal("el alta a medias no devolvió error")
	}
	if !strings.Contains(err.Error(), "profile 'p' was created, but its config could not be generated") ||
		!strings.Contains(err.Error(), "ccp profile sync p") {
		t.Errorf("el error no dice que el perfil existe ni cómo reintentar: %v", err)
	}
	if strings.Contains(err.Error(), "creado, pero") {
		t.Errorf("en inglés reenvía el marco castellano del core: %v", err)
	}
	if k, ok := core.GetKey(home, "p"); !ok || k != "sk-prueba" {
		t.Errorf("la key se perdió: %q %v", k, ok)
	}
}

// El camino normal no cambia: official y proveedor devuelven su mensaje de alta.
func TestCreateProfileOK(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	msg, err := createProfile(home, i18n.En, "official", "o", core.Defaults{}, "")
	if err != nil || msg != i18n.T(i18n.En, "tui.form.official_created", "o") {
		t.Errorf("official: %q %v", msg, err)
	}
	msg, err = createProfile(home, i18n.En, "deepseek", "d", core.BuiltinDefaults(), "sk-d")
	if err != nil || msg != i18n.T(i18n.En, "tui.form.provider_created", "deepseek", "d") {
		t.Errorf("deepseek: %q %v", msg, err)
	}
	if k, ok := core.GetKey(home, "d"); !ok || k != "sk-d" {
		t.Errorf("key de d: %q %v", k, ok)
	}
}
