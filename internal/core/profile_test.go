package core

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// makeFakeClaudeSrc crea un directorio temporal que simula ~/.claude con los 4
// directorios que _seed_cc_home symlinks (plugins, commands, agents, skills).
func makeFakeClaudeSrc(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	for _, item := range []string{"plugins", "commands", "agents", "skills"} {
		if err := os.MkdirAll(filepath.Join(src, item), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return src
}

// -------------------------------------------------------------------------
// ProfileAddOfficial
// -------------------------------------------------------------------------

func TestProfileAddOfficial_CreatesEntryAndCCHome(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatalf("ProfileAddOfficial: %v", err)
	}

	// ccp.yaml debe tener el perfil.
	c, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, ok := c.Profiles["work"]
	if !ok {
		t.Fatal("perfil 'work' no encontrado en ccp.yaml")
	}
	if p.Type != "official" {
		t.Errorf("Type = %q, quiero official", p.Type)
	}

	// cc-home debe existir con los 4 symlinks.
	cch := ccHomePath(home, "work")
	if !isDir(cch) {
		t.Fatalf("cc-home no existe: %s", cch)
	}
	for _, item := range []string{"plugins", "commands", "agents", "skills"} {
		dst := filepath.Join(cch, item)
		fi, err := os.Lstat(dst)
		if err != nil {
			t.Errorf("symlink %s no existe: %v", item, err)
			continue
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s no es un symlink (mode=%v)", item, fi.Mode())
			continue
		}
		target, err := os.Readlink(dst)
		if err != nil {
			t.Fatalf("Readlink %s: %v", item, err)
		}
		wantTarget := filepath.Join(src, item)
		if target != wantTarget {
			t.Errorf("%s apunta a %q, quiero %q", item, target, wantTarget)
		}
	}
}

func TestProfileAddOfficial_RefusesDefault(t *testing.T) {
	home := t.TempDir()
	if err := ProfileAddOfficial(home, "default"); err == nil {
		t.Fatal("debía fallar para 'default'")
	}
}

func TestProfileAddOfficial_RefusesDuplicate(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := ProfileAddOfficial(home, "dupe"); err != nil {
		t.Fatalf("primera creación: %v", err)
	}
	if err := ProfileAddOfficial(home, "dupe"); err == nil {
		t.Fatal("segunda creación debía fallar")
	}
}

// -------------------------------------------------------------------------
// ProfileAddDeepseek
// -------------------------------------------------------------------------

func TestProfileAddDeepseek_FieldsPersistExplicitly(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	d := Defaults{
		BaseURL:    "https://api.deepseek.com/anthropic",
		ModelPro:   "deepseek-chat",
		ModelFlash: "deepseek-chat",
		Effort:     "high",
	}
	if err := ProfileAddDeepseek(home, "ds", d); err != nil {
		t.Fatalf("ProfileAddDeepseek: %v", err)
	}

	// Reload limpio desde bytes para confirmar persistencia explícita.
	raw, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"base_url", "model_pro", "model_flash", "effort"} {
		if !bytes.Contains(raw, []byte(field)) {
			t.Errorf("campo %q no aparece en ccp.yaml:\n%s", field, raw)
		}
	}

	c, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p := c.Profiles["ds"]
	if p.Type != "deepseek" {
		t.Errorf("Type = %q, quiero deepseek", p.Type)
	}
	if p.BaseURL != d.BaseURL {
		t.Errorf("BaseURL = %q, quiero %q", p.BaseURL, d.BaseURL)
	}
	if p.ModelPro != d.ModelPro {
		t.Errorf("ModelPro = %q, quiero %q", p.ModelPro, d.ModelPro)
	}
	if p.ModelFlash != d.ModelFlash {
		t.Errorf("ModelFlash = %q, quiero %q", p.ModelFlash, d.ModelFlash)
	}
	if p.Effort != d.Effort {
		t.Errorf("Effort = %q, quiero %q", p.Effort, d.Effort)
	}
}

func TestProfileAddProvider_KimiAndGLMPersist(t *testing.T) {
	cases := []struct {
		add  func(home, name string, d Defaults) error
		typ  string
		want Defaults
	}{
		{ProfileAddKimi, "kimi", PresetDefaults("kimi")},
		{ProfileAddGLM, "glm", PresetDefaults("glm")},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			home := t.TempDir()
			src := makeFakeClaudeSrc(t)
			t.Setenv("CCP_CLAUDE_SRC", src)

			if err := c.add(home, c.typ, c.want); err != nil {
				t.Fatalf("add %s: %v", c.typ, err)
			}
			cfg, err := Load(home)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			p := cfg.Profiles[c.typ]
			if p.Type != c.typ {
				t.Errorf("Type = %q, quiero %q", p.Type, c.typ)
			}
			if p.BaseURL != c.want.BaseURL || p.ModelPro != c.want.ModelPro ||
				p.ModelFlash != c.want.ModelFlash || p.Effort != c.want.Effort {
				t.Errorf("perfil %s = %+v, quiero %+v", c.typ, p, c.want)
			}
			// set key debe aceptar cualquier proveedor.
			if err := ProfileSetKey(home, c.typ, "sk-test"); err != nil {
				t.Errorf("ProfileSetKey(%s): %v", c.typ, err)
			}
		})
	}
}

func TestProfileAddProvider_RejectsUnknownType(t *testing.T) {
	home := t.TempDir()
	if err := ProfileAddProvider(home, "x", "openai", Defaults{}); err == nil {
		t.Fatal("ProfileAddProvider con tipo desconocido debió fallar")
	}
}

func TestProfileAddDeepseek_CCHomeSymlinks(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := ProfileAddDeepseek(home, "myds", Defaults{
		BaseURL:  "https://api.example.com",
		ModelPro: "model-pro",
	}); err != nil {
		t.Fatalf("ProfileAddDeepseek: %v", err)
	}

	cch := ccHomePath(home, "myds")
	if !isDir(cch) {
		t.Fatalf("cc-home no existe: %s", cch)
	}
	for _, item := range []string{"plugins", "commands", "agents", "skills"} {
		dst := filepath.Join(cch, item)
		fi, err := os.Lstat(dst)
		if err != nil {
			t.Errorf("symlink %s no existe: %v", item, err)
			continue
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s no es un symlink", item)
		}
	}
}

// -------------------------------------------------------------------------
// ProfileSetKey
// -------------------------------------------------------------------------

func TestProfileSetKey_WritesSecretChmod600NotInYAML(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := ProfileAddDeepseek(home, "secret", Defaults{
		BaseURL: "https://api.example.com",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	const testKey = "sk-supersecret-12345"
	if err := ProfileSetKey(home, "secret", testKey); err != nil {
		t.Fatalf("ProfileSetKey: %v", err)
	}

	// Archivo existe con chmod 600.
	keyPath := apiKeyPath(home, "secret")
	fi, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("api_key no existe: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("modo = %o, quiero 600", fi.Mode().Perm())
	}

	// El valor se puede leer.
	got, ok := GetKey(home, "secret")
	if !ok {
		t.Fatal("GetKey devolvió false")
	}
	if got != testKey {
		t.Errorf("GetKey = %q, quiero %q", got, testKey)
	}

	// La clave NO aparece en ccp.yaml.
	raw, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(testKey)) {
		t.Errorf("la clave secreta aparece en ccp.yaml:\n%s", raw)
	}
	if bytes.Contains(raw, []byte("api_key")) {
		t.Errorf("'api_key' aparece en ccp.yaml:\n%s", raw)
	}
}

func TestProfileSetKey_RefusesOfficialType(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := ProfileAddOfficial(home, "ofic"); err != nil {
		t.Fatal(err)
	}
	if err := ProfileSetKey(home, "ofic", "sk-xxx"); err == nil {
		t.Fatal("debía fallar para perfil official")
	}
}

func TestProfileSetKey_RefusesNonExistent(t *testing.T) {
	home := t.TempDir()
	if err := ProfileSetKey(home, "ghost", "sk-xxx"); err == nil {
		t.Fatal("debía fallar para perfil inexistente")
	}
}

// -------------------------------------------------------------------------
// ProfileRm
// -------------------------------------------------------------------------

func TestProfileRm_RemovesFromYAMLAndDir(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := ProfileAddOfficial(home, "bye"); err != nil {
		t.Fatal(err)
	}
	// Confirmar que el directorio existe antes.
	if !isDir(profileDirPath(home, "bye")) {
		t.Fatal("directorio de perfil no creado")
	}

	if err := ProfileRm(home, "bye"); err != nil {
		t.Fatalf("ProfileRm: %v", err)
	}

	c, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Profiles["bye"]; ok {
		t.Error("perfil 'bye' todavía en ccp.yaml después de rm")
	}
	if isDir(profileDirPath(home, "bye")) {
		t.Error("directorio de perfil todavía existe después de rm")
	}
}

func TestProfileRm_RefusesDefault(t *testing.T) {
	home := t.TempDir()
	if err := ProfileRm(home, "default"); err == nil {
		t.Fatal("debía rechazar 'default'")
	}
}

func TestProfileRm_RefusesNonExistent(t *testing.T) {
	home := t.TempDir()
	if err := ProfileRm(home, "ghost"); err == nil {
		t.Fatal("debía fallar para perfil inexistente")
	}
}

// -------------------------------------------------------------------------
// ProfileList
// -------------------------------------------------------------------------

func TestProfileList_Sorted(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	for _, n := range []string{"zzz", "aaa", "mmm"} {
		if err := ProfileAddOfficial(home, n); err != nil {
			t.Fatalf("add %s: %v", n, err)
		}
	}

	names, err := ProfileList(home)
	if err != nil {
		t.Fatal(err)
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("lista no ordenada: %v", names)
	}
	if len(names) != 3 {
		t.Errorf("len = %d, quiero 3", len(names))
	}
	// "default" nunca debe aparecer.
	for _, n := range names {
		if n == "default" {
			t.Error("'default' no debe aparecer en ProfileList")
		}
	}
}

func TestProfileList_Empty(t *testing.T) {
	home := t.TempDir()
	names, err := ProfileList(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("esperaba lista vacía, obtuve %v", names)
	}
}

// -------------------------------------------------------------------------
// ProfileShow
// -------------------------------------------------------------------------

func TestProfileShow_OfficialFormat(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := ProfileAddOfficial(home, "corp"); err != nil {
		t.Fatal(err)
	}

	out, err := ProfileShow(home, "corp")
	if err != nil {
		t.Fatalf("ProfileShow: %v", err)
	}

	for _, want := range []string{"corp", "official", "Config dir:", "Login:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output no contiene %q:\n%s", want, out)
		}
	}
}

func TestProfileShow_DeepseekFormat_NoKey(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := ProfileAddDeepseek(home, "ds", Defaults{
		BaseURL:    "https://api.deepseek.com/anthropic",
		ModelPro:   "deepseek-chat",
		ModelFlash: "deepseek-chat",
		Effort:     "high",
	}); err != nil {
		t.Fatal(err)
	}

	out, err := ProfileShow(home, "ds")
	if err != nil {
		t.Fatalf("ProfileShow: %v", err)
	}

	for _, want := range []string{"deepseek", "Base URL:", "Modelo pro:", "Modelo flash:", "Effort:", "falta"} {
		if !strings.Contains(out, want) {
			t.Errorf("output no contiene %q:\n%s", want, out)
		}
	}
}

func TestProfileShow_DeepseekFormat_WithKey(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := ProfileAddDeepseek(home, "ds2", Defaults{
		BaseURL: "https://api.deepseek.com/anthropic",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ProfileSetKey(home, "ds2", "sk-test"); err != nil {
		t.Fatal(err)
	}

	out, err := ProfileShow(home, "ds2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "OK") {
		t.Errorf("output debería indicar key presente con 'OK':\n%s", out)
	}
}

func TestProfileShow_NonExistent(t *testing.T) {
	home := t.TempDir()
	if _, err := ProfileShow(home, "ghost"); err == nil {
		t.Fatal("debía fallar para perfil inexistente")
	}
}

// -------------------------------------------------------------------------
// seedCCHome — casos edge
// -------------------------------------------------------------------------

func TestSeedCCHome_SkipsExistingSymlinks(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	// Primera siembra.
	if err := seedCCHome(home, "idempotent"); err != nil {
		t.Fatal(err)
	}
	// Segunda siembra idempotente: no debe fallar.
	if err := seedCCHome(home, "idempotent"); err != nil {
		t.Fatalf("segunda siembra falló: %v", err)
	}
}

func TestSeedCCHome_SkipsMissingItemsInSrc(t *testing.T) {
	home := t.TempDir()
	// src sin ningún item.
	src := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := seedCCHome(home, "sparse"); err != nil {
		t.Fatalf("seedCCHome con src vacío: %v", err)
	}
	// No deben existir symlinks en cch.
	cch := ccHomePath(home, "sparse")
	for _, item := range []string{"plugins", "commands", "agents", "skills"} {
		dst := filepath.Join(cch, item)
		if _, err := os.Lstat(dst); err == nil {
			t.Errorf("%s existe cuando no debería (src vacío)", item)
		}
	}
}

// -------------------------------------------------------------------------
// Bash oracle parity (se omite si bash o bin/ccp no están disponibles)
// -------------------------------------------------------------------------

func TestProfileList_BashOracleParity(t *testing.T) {
	binCCP := findBinCCP(t)
	if binCCP == "" {
		t.Skip("bin/ccp no encontrado; saltando comparación con oracle bash")
	}

	// Construir estado idéntico vía oracle bash y vía Go.
	bashHome := t.TempDir()
	goHome := t.TempDir()
	src := makeFakeClaudeSrc(t)

	// Usamos profiles que no requieren login ni key para list.
	profiles := []string{"alf", "beta", "gamma"}

	// Estado bash: crear perfiles vía oracle.
	for _, n := range profiles {
		cmd := exec.Command("bash", binCCP, "profile", "add", n, "--official")
		cmd.Env = append(os.Environ(),
			"CCP_HOME="+bashHome,
			"CCP_CLAUDE_SRC="+src,
			"NO_COLOR=1",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("oracle bash add %s: %v\n%s", n, err, out)
			t.Skip("oracle bash falló; saltando parity test")
		}
	}

	// Capturar salida del oracle bash.
	cmd := exec.Command("bash", binCCP, "profile", "list")
	cmd.Env = append(os.Environ(),
		"CCP_HOME="+bashHome,
		"NO_COLOR=1",
	)
	bashOut, err := cmd.Output()
	if err != nil {
		t.Skipf("oracle profile list falló: %v", err)
	}

	// Estado Go: crear perfiles vía funciones Go.
	for _, n := range profiles {
		if err := ProfileAddOfficial(goHome, n); err != nil {
			t.Fatalf("Go add %s: %v", n, err)
		}
	}
	goNames, err := ProfileList(goHome)
	if err != nil {
		t.Fatalf("ProfileList: %v", err)
	}
	goOut := strings.Join(goNames, "\n") + "\n"

	// El oracle bash imprime un nombre por línea (puede tener trailing whitespace).
	bashLines := normLines(string(bashOut))
	goLines := normLines(goOut)

	if !equalStringSlices(bashLines, goLines) {
		t.Errorf("divergencia list:\nbash: %v\ngo:   %v", bashLines, goLines)
	}
}

// -------------------------------------------------------------------------
// helpers de test
// -------------------------------------------------------------------------

// findBinCCP busca bin/ccp relativo al repo, subiendo desde el directorio de
// test (que es interno/core).
func findBinCCP(t *testing.T) string {
	t.Helper()
	// El test corre en internal/core; el repo está 2 dirs arriba.
	candidates := []string{
		filepath.Join("..", "..", "bin", "ccp"),
		filepath.Join("..", "..", "..", "bin", "ccp"),
	}
	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if fileExists(abs) {
			return abs
		}
	}
	return ""
}

// normLines normaliza líneas: trim whitespace, descarta vacías, ordena.
func normLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		l := strings.TrimSpace(line)
		if l != "" {
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
