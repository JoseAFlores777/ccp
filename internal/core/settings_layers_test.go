package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Por defecto el overlay REEMPLAZA la lista del global (ADR 0002). Con
// "$merge": "union" se suman, sin duplicados y con el global primero. La marca
// nunca llega al settings.json generado.
func TestPermisosUnionSoloSiSePide(t *testing.T) {
	home, src := perfilConBase(t, `{"permissions":{"allow":["Read","Bash(ls)"],"deny":["WebFetch"]}}`,
		`{"permissions":{"allow":["Bash(make)"]}}`)
	gen := leeGenerado(t, home)
	if v, _ := jsonLookup(gen, []string{"permissions", "allow"}); !jsonEqual(v, []any{"Bash(make)"}) {
		t.Fatalf("sin marca, el overlay reemplaza: %v", v)
	}

	mustWrite(t, cfgSettingsFile(home, "work"), `{"permissions":{"$merge":"union","allow":["Bash(make)","Read"]}}`)
	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatal(err)
	}
	gen = leeGenerado(t, home)
	v, _ := jsonLookup(gen, []string{"permissions", "allow"})
	if !jsonEqual(v, []any{"Read", "Bash(ls)", "Bash(make)"}) {
		t.Fatalf("con unión: %v", v)
	}
	if d, _ := jsonLookup(gen, []string{"permissions", "deny"}); !jsonEqual(d, []any{"WebFetch"}) {
		t.Errorf("deny del global = %v", d)
	}
	if _, ok := jsonLookup(gen, []string{"permissions", "$merge"}); ok {
		t.Error("la marca $merge no puede llegar al archivo generado")
	}
}

// Editar una capa por clave: leer, fijar y borrar, sin romper un symlink ni
// cambiar el modo (un overlay 0600 sigue siendo 0600).
func TestSettingsLayerGetSet(t *testing.T) {
	home, src := perfilConBase(t, `{"model":"opus"}`, `{"env":{"A":"1"}}`)
	if err := os.Chmod(cfgSettingsFile(home, "work"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, ok, err := SettingsLayerGet(home, src, "global", []string{"model"})
	if err != nil || !ok || v != "opus" {
		t.Fatalf("global.model = %v %v %v", v, ok, err)
	}
	file, err := SettingsLayerSet(home, src, "profile:work", []string{"hooks", "PreToolUse"},
		[]any{map[string]any{"matcher": "Bash"}}, false)
	if err != nil || file != cfgSettingsFile(home, "work") {
		t.Fatalf("set = %s %v", file, err)
	}
	if fi, _ := os.Stat(file); fi.Mode().Perm() != 0o600 {
		t.Errorf("modo = %v", fi.Mode().Perm())
	}
	v, ok, _ = SettingsLayerGet(home, src, "profile:work", []string{"hooks", "PreToolUse"})
	if !ok || len(v.([]any)) != 1 {
		t.Fatalf("hooks = %v %v", v, ok)
	}
	// Borrar el evento entero es cómo se quita un hook: se reescribe la lista.
	if _, err := SettingsLayerSet(home, src, "profile:work", []string{"hooks", "PreToolUse"}, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := SettingsLayerGet(home, src, "profile:work", []string{"hooks", "PreToolUse"}); ok {
		t.Error("el evento no se borró")
	}
	if v, ok, _ := SettingsLayerGet(home, src, "profile:work", []string{"env", "A"}); !ok || v != "1" {
		t.Error("se perdió lo que ya había en la capa")
	}
	if _, _, err := SettingsLayerGet(home, src, "profile:default", []string{"x"}); err == nil {
		t.Error("default no tiene capa propia")
	}
}

// Un settings.json de capa que no es JSON no se pisa.
func TestSettingsLayerSetNoPisaUnArchivoRoto(t *testing.T) {
	home, src := perfilConBase(t, "", "")
	mustWrite(t, cfgSettingsFile(home, "work"), "{roto")
	if _, err := SettingsLayerSet(home, src, "profile:work", []string{"model"}, "opus", false); err == nil {
		t.Fatal("tenía que fallar")
	}
	if b, _ := os.ReadFile(cfgSettingsFile(home, "work")); string(b) != "{roto" {
		t.Fatalf("se pisó: %s", b)
	}
	_ = filepath.Join
	_ = strings.Contains
	_ = reflect.DeepEqual
}
