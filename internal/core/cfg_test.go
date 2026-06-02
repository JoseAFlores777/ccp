package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// normalizeJSON re-decodifica para comparar por valor, ignorando formato.
func normalizeJSON(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("JSON inválido %q: %v", b, err)
	}
	return v
}

// TestMergeJSON verifica la semántica jq `. * $x`: objetos se fusionan
// recursivamente; arrays y escalares se REEMPLAZAN por el overlay.
func TestMergeJSON(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		overlay string
		want    string
	}{
		{
			name:    "base vacio devuelve overlay",
			base:    "",
			overlay: `{"a":1}`,
			want:    `{"a":1}`,
		},
		{
			name:    "overlay vacio conserva base",
			base:    `{"a":1}`,
			overlay: "",
			want:    `{"a":1}`,
		},
		{
			name:    "ambos vacios => objeto vacio",
			base:    "",
			overlay: "",
			want:    `{}`,
		},
		{
			name:    "claves disjuntas se unen",
			base:    `{"a":1}`,
			overlay: `{"b":2}`,
			want:    `{"a":1,"b":2}`,
		},
		{
			name:    "escalar reemplazado por overlay",
			base:    `{"a":1}`,
			overlay: `{"a":2}`,
			want:    `{"a":2}`,
		},
		{
			name:    "objetos anidados se fusionan recursivamente",
			base:    `{"env":{"X":"1","Y":"2"}}`,
			overlay: `{"env":{"Y":"9","Z":"3"}}`,
			want:    `{"env":{"X":"1","Y":"9","Z":"3"}}`,
		},
		{
			name:    "array reemplazado, no concatenado",
			base:    `{"perms":["a","b"]}`,
			overlay: `{"perms":["c"]}`,
			want:    `{"perms":["c"]}`,
		},
		{
			name:    "objeto reemplaza array (tipos mixtos => gana overlay)",
			base:    `{"a":[1,2]}`,
			overlay: `{"a":{"k":"v"}}`,
			want:    `{"a":{"k":"v"}}`,
		},
		{
			name:    "array reemplaza objeto (tipos mixtos => gana overlay)",
			base:    `{"a":{"k":"v"}}`,
			overlay: `{"a":[1,2]}`,
			want:    `{"a":[1,2]}`,
		},
		{
			name:    "merge profundo de 3 niveles",
			base:    `{"hooks":{"PreToolUse":{"keep":1}}}`,
			overlay: `{"hooks":{"PreToolUse":{"add":2},"PostToolUse":{"x":3}}}`,
			want:    `{"hooks":{"PreToolUse":{"keep":1,"add":2},"PostToolUse":{"x":3}}}`,
		},
		{
			name:    "null en overlay no borra (overlay nil-like solo si ausente)",
			base:    `{"a":1}`,
			overlay: `{"a":null}`,
			want:    `{"a":null}`,
		},
		{
			name:    "entero grande preserva precision",
			base:    `{}`,
			overlay: `{"n":9007199254740993}`,
			want:    `{"n":9007199254740993}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MergeJSON([]byte(tc.base), []byte(tc.overlay))
			if err != nil {
				t.Fatalf("MergeJSON error: %v", err)
			}
			if !reflect.DeepEqual(normalizeJSON(t, got), normalizeJSON(t, []byte(tc.want))) {
				t.Errorf("MergeJSON(%s, %s) = %s, quiero %s", tc.base, tc.overlay, got, tc.want)
			}
		})
	}
}

func TestMergeJSONBigIntRoundTrip(t *testing.T) {
	out, err := MergeJSON([]byte(`{}`), []byte(`{"n":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "9007199254740993") {
		t.Errorf("entero grande perdió precisión: %s", out)
	}
}

func TestMergeJSONInvalid(t *testing.T) {
	if _, err := MergeJSON([]byte(`{bad`), []byte(`{}`)); err == nil {
		t.Error("quiero error con base inválido")
	}
	if _, err := MergeJSON([]byte(`{}`), []byte(`{bad`)); err == nil {
		t.Error("quiero error con overlay inválido")
	}
}

// TestCfgRegenerate verifica CLAUDE.md (@import header + global + overlay) y
// settings.json (deep-merge global ⊕ overlay).
func TestCfgRegenerate(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	name := "work"

	// global
	if err := os.WriteFile(filepath.Join(src, "CLAUDE.md"), []byte("global instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"model":"opus","env":{"A":"1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// overlay del perfil
	if err := CfgInitOverlay(home, name); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgInstrFile(home, name), []byte("profile-specific\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgSettingsFile(home, name), []byte(`{"env":{"A":"override","B":"2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CfgRegenerate(home, name, src); err != nil {
		t.Fatalf("CfgRegenerate: %v", err)
	}

	// CLAUDE.md: header + @import global + @import overlay
	cch := ccHomePath(home, name)
	md, err := os.ReadFile(filepath.Join(cch, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	mdStr := string(md)
	for _, want := range []string{
		"# work — generado por ccp",
		"@" + filepath.Join(src, "CLAUDE.md"),
		"@" + cfgInstrFile(home, name),
	} {
		if !strings.Contains(mdStr, want) {
			t.Errorf("CLAUDE.md no contiene %q:\n%s", want, mdStr)
		}
	}

	// settings.json: deep-merge (env.A override, env.B añadido, model conservado)
	sj, err := os.ReadFile(filepath.Join(cch, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := normalizeJSON(t, sj)
	want := normalizeJSON(t, []byte(`{"model":"opus","env":{"A":"override","B":"2"}}`))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("settings.json merge = %s, quiero %s", sj, want)
	}
}

// TestCfgRegenerateNoGlobalClaude: si <src>/CLAUDE.md no existe, solo @import
// del overlay (sin línea del global).
func TestCfgRegenerateNoGlobalClaude(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir() // sin CLAUDE.md ni settings.json
	name := "p"

	if err := CfgRegenerate(home, name, src); err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(filepath.Join(ccHomePath(home, name), "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(md), filepath.Join(src, "CLAUDE.md")) {
		t.Errorf("no debe importar global ausente:\n%s", md)
	}
	if !strings.Contains(string(md), "@"+cfgInstrFile(home, name)) {
		t.Errorf("debe importar overlay:\n%s", md)
	}
	// settings.json = overlay vacío => {}
	sj, _ := os.ReadFile(filepath.Join(ccHomePath(home, name), "settings.json"))
	if got := normalizeJSON(t, sj); !reflect.DeepEqual(got, normalizeJSON(t, []byte(`{}`))) {
		t.Errorf("settings.json sin global/overlay = %s, quiero {}", sj)
	}
}

// TestCfgMigrateLegacy verifica la conversión y su idempotencia.
func TestCfgMigrateLegacy(t *testing.T) {
	home := t.TempDir()
	name := "old"
	cch := ccHomePath(home, name)
	if err := os.MkdirAll(cch, 0o755); err != nil {
		t.Fatal(err)
	}

	// Estado viejo: settings.json copia real + CLAUDE.md symlink.
	legacySettings := filepath.Join(cch, "settings.json")
	if err := os.WriteFile(legacySettings, []byte(`{"old":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "global-CLAUDE.md")
	if err := os.WriteFile(target, []byte("global\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyClaude := filepath.Join(cch, "CLAUDE.md")
	if err := os.Symlink(target, legacyClaude); err != nil {
		t.Fatal(err)
	}

	if err := CfgMigrateLegacy(home, name); err != nil {
		t.Fatalf("CfgMigrateLegacy: %v", err)
	}

	// settings.json movido al overlay
	overlay := cfgSettingsFile(home, name)
	ob, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatalf("overlay settings no creado: %v", err)
	}
	if !strings.Contains(string(ob), `"old":true`) {
		t.Errorf("overlay no tiene el contenido legacy: %s", ob)
	}
	if pathExists(legacySettings) {
		t.Error("settings.json legacy debió moverse, sigue presente")
	}
	// CLAUDE.md symlink eliminado; target intacto
	if isSymlink(legacyClaude) {
		t.Error("CLAUDE.md symlink debió eliminarse")
	}
	if !fileExists(target) {
		t.Error("el target global del symlink NO debe borrarse")
	}

	// Idempotente: segunda corrida no falla ni revierte.
	if err := CfgMigrateLegacy(home, name); err != nil {
		t.Fatalf("segunda CfgMigrateLegacy: %v", err)
	}
	ob2, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatal(err)
	}
	if string(ob2) != string(ob) {
		t.Errorf("idempotencia rota: overlay cambió de %s a %s", ob, ob2)
	}
}

// TestCfgMigrateLegacyNoop: cc-home ya migrado (overlay existe) no debe tocar
// el settings.json regenerado.
func TestCfgMigrateLegacyNoop(t *testing.T) {
	home := t.TempDir()
	name := "p"
	if err := CfgInitOverlay(home, name); err != nil {
		t.Fatal(err)
	}
	// overlay ya existe -> migrate no debe mover un settings.json regenerado.
	cch := ccHomePath(home, name)
	if err := os.MkdirAll(cch, 0o755); err != nil {
		t.Fatal(err)
	}
	regenerated := filepath.Join(cch, "settings.json")
	if err := os.WriteFile(regenerated, []byte(`{"regenerated":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CfgMigrateLegacy(home, name); err != nil {
		t.Fatal(err)
	}
	if !fileExists(regenerated) {
		t.Error("settings.json regenerado no debe moverse cuando overlay ya existe")
	}
}

func TestCfgInitOverlayIdempotent(t *testing.T) {
	home := t.TempDir()
	name := "p"
	if err := CfgInitOverlay(home, name); err != nil {
		t.Fatal(err)
	}
	// Escribe contenido y re-inicializa: no debe sobrescribir.
	if err := os.WriteFile(cfgInstrFile(home, name), []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CfgInitOverlay(home, name); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(cfgInstrFile(home, name))
	if string(b) != "keep me\n" {
		t.Errorf("CfgInitOverlay sobrescribió contenido existente: %q", b)
	}
}

// TestProfileSyncAll regenera todos los perfiles.
func TestProfileSyncAll(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"g":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, n := range []string{"a", "b"} {
		if err := ProfileAddOfficial(home, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := ProfileSync(home, ""); err != nil {
		t.Fatalf("ProfileSync all: %v", err)
	}
	for _, n := range []string{"a", "b"} {
		sj := filepath.Join(ccHomePath(home, n), "settings.json")
		if !fileExists(sj) {
			t.Errorf("perfil %q: settings.json no regenerado", n)
		}
		md := filepath.Join(ccHomePath(home, n), "CLAUDE.md")
		if !fileExists(md) {
			t.Errorf("perfil %q: CLAUDE.md no regenerado", n)
		}
	}
}

func TestProfileSyncRejectsDefault(t *testing.T) {
	home := t.TempDir()
	if err := ProfileSync(home, "default"); err == nil {
		t.Error("ProfileSync default debe fallar")
	}
}

func TestProfileSyncUnknownProfile(t *testing.T) {
	home := t.TempDir()
	if err := ProfileSync(home, "ghost"); err == nil {
		t.Error("ProfileSync de perfil inexistente debe fallar")
	}
}

// TestProfileConfig usa un Launch inyectado para simular la edición.
func TestProfileConfig(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}

	var openedFiles []string
	launch := func(_ string, files ...string) error {
		openedFiles = files
		// Simula que el usuario escribe un overlay válido.
		return os.WriteFile(cfgSettingsFile(home, "work"), []byte(`{"env":{"K":"V"}}`), 0o644)
	}
	if err := ProfileConfig(home, "work", ProfileConfigOpts{Launch: launch}); err != nil {
		t.Fatalf("ProfileConfig: %v", err)
	}
	if len(openedFiles) != 2 {
		t.Fatalf("debe abrir 2 archivos (instr+settings), abrió %d: %v", len(openedFiles), openedFiles)
	}
	// Regeneró el merge con el overlay editado.
	sj, _ := os.ReadFile(filepath.Join(ccHomePath(home, "work"), "settings.json"))
	if !strings.Contains(string(sj), `"K"`) {
		t.Errorf("settings.json no refleja el overlay editado: %s", sj)
	}
}

func TestProfileConfigInvalidJSONNotRegenerated(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	// Primero un buen estado.
	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatal(err)
	}
	goodSettings := filepath.Join(ccHomePath(home, "work"), "settings.json")
	goodBefore, _ := os.ReadFile(goodSettings)

	launch := func(_ string, _ ...string) error {
		return os.WriteFile(cfgSettingsFile(home, "work"), []byte(`{bad json`), 0o644)
	}
	err := ProfileConfig(home, "work", ProfileConfigOpts{Launch: launch})
	if err == nil {
		t.Fatal("ProfileConfig con JSON inválido debe devolver error")
	}
	// El último settings.json bueno se conserva (no regenerado con basura).
	goodAfter, _ := os.ReadFile(goodSettings)
	if string(goodAfter) != string(goodBefore) {
		t.Errorf("settings.json bueno fue tocado pese a JSON inválido: %s", goodAfter)
	}
}

func TestProfileConfigRejectsDefault(t *testing.T) {
	home := t.TempDir()
	if err := ProfileConfig(home, "default", ProfileConfigOpts{}); err == nil {
		t.Error("ProfileConfig default debe fallar")
	}
}

func TestProfileConfigUnknown(t *testing.T) {
	home := t.TempDir()
	if err := ProfileConfig(home, "ghost", ProfileConfigOpts{}); err == nil {
		t.Error("ProfileConfig de perfil inexistente debe fallar")
	}
}

// TestResolveEditor verifica la precedencia defaults -> $EDITOR -> nano.
func TestResolveEditor(t *testing.T) {
	home := t.TempDir()

	t.Setenv("EDITOR", "")
	if got := ResolveEditor(home); got != "nano" {
		t.Errorf("sin defaults ni EDITOR => %q, quiero nano", got)
	}

	t.Setenv("EDITOR", "vim")
	if got := ResolveEditor(home); got != "vim" {
		t.Errorf("con EDITOR=vim => %q, quiero vim", got)
	}

	c := &Config{Version: SchemaVersion, Defaults: Defaults{Editor: "code --wait"}, Profiles: map[string]Profile{}}
	if err := Save(home, c); err != nil {
		t.Fatal(err)
	}
	if got := ResolveEditor(home); got != "code --wait" {
		t.Errorf("con defaults.editor => %q, quiero 'code --wait'", got)
	}
}
