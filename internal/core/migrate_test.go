package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildLegacyHome crea un home ccp con estado TSV/meta espejando el fixture
// testdata/golden/basic/ccp-home, más un archivo `config` de defaults.
func buildLegacyHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, "profiles.tsv"),
		"work\tofficial\ndeepseek\tdeepseek\n")
	mustWrite(t, filepath.Join(home, "rules.tsv"),
		"/repos/work\twork\n/repos/labs\tdeepseek\n/repos/labs/secret\twork\n")
	mustWrite(t, filepath.Join(home, "config"),
		"base_url=https://api.deepseek.com/anthropic\nmodel_pro=deepseek-chat\nmodel_flash=deepseek-chat\neffort=high\neditor=nano\n")
	mustWrite(t, filepath.Join(home, "profiles", "work", "meta"),
		"type=official\n")
	mustWrite(t, filepath.Join(home, "profiles", "deepseek", "meta"),
		"type=deepseek\nbase_url=https://api.deepseek.com/anthropic\nmodel_pro=deepseek-chat\nmodel_flash=deepseek-chat\neffort=high\n")
	if err := os.WriteFile(filepath.Join(home, "profiles", "deepseek", "api_key"),
		[]byte("sk-fixture-deepseek-key-0000"), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateTSVMetaToYAML(t *testing.T) {
	home := buildLegacyHome(t)
	if err := Migrate(home, "20260602-120000"); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	c, err := Load(home)
	if err != nil {
		t.Fatalf("Load after migrate: %v", err)
	}

	if c.Version != SchemaVersion {
		t.Errorf("version = %d, want %d", c.Version, SchemaVersion)
	}
	// Defaults desde config.
	if c.Defaults.BaseURL != "https://api.deepseek.com/anthropic" {
		t.Errorf("defaults base_url = %q", c.Defaults.BaseURL)
	}
	if c.Defaults.Editor != "nano" {
		t.Errorf("defaults editor = %q", c.Defaults.Editor)
	}
	// Perfil official.
	if c.Profiles["work"].Type != "official" {
		t.Errorf("work = %+v", c.Profiles["work"])
	}
	if c.Profiles["work"].BaseURL != "" {
		t.Errorf("official profile should carry no base_url, got %q", c.Profiles["work"].BaseURL)
	}
	// Perfil deepseek con sus 4 campos.
	ds := c.Profiles["deepseek"]
	if ds.Type != "deepseek" || ds.ModelPro != "deepseek-chat" ||
		ds.ModelFlash != "deepseek-chat" || ds.Effort != "high" ||
		ds.BaseURL != "https://api.deepseek.com/anthropic" {
		t.Errorf("deepseek profile wrong: %+v", ds)
	}
	// Reglas presentes.
	if len(c.Rules) != 3 {
		t.Fatalf("rules count = %d, want 3: %+v", len(c.Rules), c.Rules)
	}
	wantRules := map[string]string{
		"/repos/work":        "work",
		"/repos/labs":        "deepseek",
		"/repos/labs/secret": "work",
	}
	for _, r := range c.Rules {
		if wantRules[r.Path] != r.Profile {
			t.Errorf("rule %q -> %q, want %q", r.Path, r.Profile, wantRules[r.Path])
		}
	}

	// api_key preservada en disco.
	key, ok := GetKey(home, "deepseek")
	if !ok || key != "sk-fixture-deepseek-key-0000" {
		t.Errorf("api_key lost: ok=%v key=%q", ok, key)
	}

	// Backup dir creado.
	backup := filepath.Join(home, ".backup-pre-go-20260602-120000")
	if !isDir(backup) {
		t.Errorf("backup dir not created: %s", backup)
	}
	if !fileExists(filepath.Join(backup, "profiles.tsv")) {
		t.Errorf("backup missing profiles.tsv")
	}
	if !fileExists(filepath.Join(backup, "profiles", "deepseek", "api_key")) {
		t.Errorf("backup missing api_key copy")
	}
}

func TestSecretsNeverInYAML(t *testing.T) {
	home := buildLegacyHome(t)
	if err := Migrate(home, "20260602-120000"); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "api_key") {
		t.Errorf("ccp.yaml contains 'api_key':\n%s", s)
	}
	if strings.Contains(s, "sk-fixture-deepseek-key-0000") {
		t.Errorf("ccp.yaml leaked the secret key:\n%s", s)
	}
	if strings.Contains(s, "cc-home") {
		t.Errorf("ccp.yaml references cc-home:\n%s", s)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	home := buildLegacyHome(t)
	if err := Migrate(home, "20260602-120000"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	// Segunda corrida con stamp distinto: ccp.yaml ya existe -> no-op.
	if err := Migrate(home, "20990101-000000"); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("migrate not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	// El segundo backup no debe crearse (no-op no toca nada).
	if isDir(filepath.Join(home, ".backup-pre-go-20990101-000000")) {
		t.Error("second migrate created a backup despite no-op")
	}
}

func TestMigrateDsctlChain(t *testing.T) {
	// Estado dsctl, sin estado ccp.
	dsctl := t.TempDir()
	mustWrite(t, filepath.Join(dsctl, "config"),
		"DS_BASE_URL=\"https://api.deepseek.com/anthropic\"\nDS_MODEL_PRO=\"deepseek-chat\"\nDS_MODEL_FLASH=\"deepseek-chat\"\nDS_EFFORT=\"high\"\n")
	if err := os.WriteFile(filepath.Join(dsctl, "api_key"),
		[]byte("sk-dsctl-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dsctl, "rules.tsv"),
		"include\t/repos/labs\nexclude\t/repos/work\n")

	home := t.TempDir()
	if err := MigrateFrom(home, dsctl, "20260602-130000"); err != nil {
		t.Fatalf("MigrateFrom: %v", err)
	}

	c, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	ds := c.Profiles["deepseek"]
	if ds.Type != "deepseek" || ds.BaseURL != "https://api.deepseek.com/anthropic" {
		t.Errorf("dsctl->deepseek profile wrong: %+v", ds)
	}
	// include -> deepseek, exclude -> default.
	got := map[string]string{}
	for _, r := range c.Rules {
		got[r.Path] = r.Profile
	}
	if got["/repos/labs"] != "deepseek" {
		t.Errorf("include rule = %q, want deepseek", got["/repos/labs"])
	}
	if got["/repos/work"] != "default" {
		t.Errorf("exclude rule = %q, want default", got["/repos/work"])
	}
	// Secret migrado a profiles/deepseek/api_key 600, no en YAML.
	key, ok := GetKey(home, "deepseek")
	if !ok || key != "sk-dsctl-key" {
		t.Errorf("dsctl api_key not migrated: ok=%v key=%q", ok, key)
	}
	out, _ := os.ReadFile(yamlPath(home))
	if strings.Contains(string(out), "sk-dsctl-key") {
		t.Errorf("ccp.yaml leaked dsctl secret")
	}
	// Backup del dsctl original.
	if !fileExists(filepath.Join(home, ".backup-pre-go-20260602-130000", "dsctl", "config")) {
		t.Error("dsctl backup not created")
	}
}

func TestSetGetKeyChmod600(t *testing.T) {
	home := t.TempDir()
	if err := SetKey(home, "ds", "secret123"); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(apiKeyPath(home, "ds"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("api_key mode = %o, want 600", fi.Mode().Perm())
	}
	got, ok := GetKey(home, "ds")
	if !ok || got != "secret123" {
		t.Errorf("GetKey = %q,%v", got, ok)
	}
	if _, ok := GetKey(home, "nope"); ok {
		t.Error("GetKey on missing profile should be false")
	}
}
