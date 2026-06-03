package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripPreservesCommentsAndUnknownKeys(t *testing.T) {
	home := t.TempDir()
	raw := `version: 2 # schema en uso
defaults:
  base_url: https://api.deepseek.com/anthropic
  model_pro: deepseek-chat
  model_flash: deepseek-chat
  effort: high
  editor: nano
# perfil de trabajo
profiles:
  work:
    type: official
  deepseek:
    type: deepseek
    base_url: https://api.deepseek.com/anthropic
    model_pro: deepseek-chat
    model_flash: deepseek-chat
    effort: high
rules:
  - path: /Users/me/work
    profile: work
authored: []
future_field: keepme
nested_future:
  a: 1
  b: 2
`
	if err := os.WriteFile(yamlPath(home), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Profiles["work"].Type != "official" {
		t.Errorf("work type = %q, want official", c.Profiles["work"].Type)
	}
	if c.Profiles["deepseek"].ModelPro != "deepseek-chat" {
		t.Errorf("deepseek model_pro = %q", c.Profiles["deepseek"].ModelPro)
	}
	if _, ok := c.Extra["future_field"]; !ok {
		t.Errorf("Extra missing future_field; got %v", c.Extra)
	}
	if _, ok := c.Extra["nested_future"]; !ok {
		t.Errorf("Extra missing nested_future; got %v", c.Extra)
	}
	// Extra no debe contener claves conocidas.
	for _, k := range []string{"version", "profiles", "rules", "defaults", "authored"} {
		if _, ok := c.Extra[k]; ok {
			t.Errorf("Extra leaked known key %q", k)
		}
	}

	if err := Save(home, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	// Comentarios preservados.
	if !strings.Contains(s, "# schema en uso") {
		t.Errorf("lost line comment; got:\n%s", s)
	}
	if !strings.Contains(s, "# perfil de trabajo") {
		t.Errorf("lost head comment; got:\n%s", s)
	}
	// Claves desconocidas preservadas.
	if !strings.Contains(s, "future_field: keepme") {
		t.Errorf("lost future_field; got:\n%s", s)
	}
	if !strings.Contains(s, "nested_future") {
		t.Errorf("lost nested_future; got:\n%s", s)
	}
}

func TestLoadAbortsOnNewerVersion(t *testing.T) {
	home := t.TempDir()
	raw := "version: 99\nprofiles: {}\n"
	if err := os.WriteFile(yamlPath(home), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(home)
	if err == nil {
		t.Fatal("expected error on version 99, got nil")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("error should mention version 99: %v", err)
	}
}

func TestSaveAbortsOnNewerVersion(t *testing.T) {
	home := t.TempDir()
	c := &Config{Version: 99, Profiles: map[string]Profile{}}
	if err := Save(home, c); err == nil {
		t.Fatal("expected Save to abort on version 99")
	}
	if fileExists(yamlPath(home)) {
		t.Error("ccp.yaml must not be written when version too new")
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	home := t.TempDir()
	c, err := Load(home)
	if err != nil {
		t.Fatalf("Load on empty home: %v", err)
	}
	if c.Version != SchemaVersion {
		t.Errorf("default version = %d, want %d", c.Version, SchemaVersion)
	}
	if c.Profiles == nil {
		t.Error("Profiles should be non-nil")
	}
}

func TestConfigLangRoundTrip(t *testing.T) {
	home := t.TempDir()
	c := &Config{Version: SchemaVersion, Lang: "es",
		Profiles: map[string]Profile{}, Rules: []Rule{}}
	if err := Save(home, c); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(home)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Lang != "es" {
		t.Fatalf("Lang=%q want es", got.Lang)
	}
}

func TestSaveAtomicAndDefaultImplicit(t *testing.T) {
	home := t.TempDir()
	c := &Config{
		Version:  SchemaVersion,
		Profiles: map[string]Profile{"work": {Type: "official"}, "default": {Type: "official"}},
	}
	if err := Save(home, c); err != nil {
		t.Fatal(err)
	}
	// No temp file leftover.
	if fileExists(yamlPath(home) + ".tmp") {
		t.Error("tmp file should not remain after Save")
	}
	// Lock file exists (flock was acquired on it).
	if !fileExists(filepath.Join(home, ".ccp.lock")) {
		t.Error("lock file should exist after Save")
	}
	out, _ := os.ReadFile(yamlPath(home))
	if bytes.Contains(out, []byte("default:")) {
		t.Errorf("default profile must not be serialized; got:\n%s", out)
	}
	// Re-load round-trips work.
	got, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Profiles["default"]; ok {
		t.Error("default should never load into Profiles")
	}
	if got.Profiles["work"].Type != "official" {
		t.Error("work profile lost on round-trip")
	}
}
