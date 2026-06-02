package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findCheck busca un chequeo cuya Label contenga substr.
func findCheck(checks []DoctorCheck, substr string) (DoctorCheck, bool) {
	for _, c := range checks {
		if strings.Contains(c.Label, substr) {
			return c, true
		}
	}
	return DoctorCheck{}, false
}

func TestDoctor_PathBinaries(t *testing.T) {
	home := t.TempDir()

	// Simula que solo 'git' está en PATH.
	orig := lookPath
	defer func() { lookPath = orig }()
	lookPath = func(bin string) (string, error) {
		if bin == "git" {
			return "/usr/bin/git", nil
		}
		return "", fmt.Errorf("not found")
	}

	checks, err := Doctor(home)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	if c, ok := findCheck(checks, "git:"); !ok || !c.OK {
		t.Errorf("git debería estar OK: %+v (ok=%v)", c, ok)
	}
	if c, ok := findCheck(checks, "node:"); !ok || c.OK {
		t.Errorf("node debería faltar: %+v (ok=%v)", c, ok)
	}
	if c, ok := findCheck(checks, "claude:"); !ok || c.OK {
		t.Errorf("claude debería faltar: %+v (ok=%v)", c, ok)
	}
}

func TestDoctor_ProfileLoginAndKey(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	// PATH binaries irrelevantes para este test: stub a "todo presente".
	orig := lookPath
	defer func() { lookPath = orig }()
	lookPath = func(string) (string, error) { return "/usr/bin/x", nil }

	// Perfil official 'work' sin login.
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatalf("ProfileAddOfficial: %v", err)
	}
	// Perfil official 'home' CON login (cc-home/.claude.json).
	if err := ProfileAddOfficial(home, "casa"); err != nil {
		t.Fatalf("ProfileAddOfficial: %v", err)
	}
	claudeJSON := filepath.Join(home, "profiles", "casa", "cc-home", ".claude.json")
	if err := os.WriteFile(claudeJSON, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Perfil deepseek 'ds' sin key.
	if err := ProfileAddDeepseek(home, "ds", BuiltinDefaults()); err != nil {
		t.Fatalf("ProfileAddDeepseek: %v", err)
	}
	// Perfil deepseek 'dskey' CON key.
	if err := ProfileAddDeepseek(home, "dskey", BuiltinDefaults()); err != nil {
		t.Fatalf("ProfileAddDeepseek: %v", err)
	}
	if err := SetKey(home, "dskey", "sk-test"); err != nil {
		t.Fatalf("SetKey: %v", err)
	}

	checks, err := Doctor(home)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	cases := []struct {
		substr string
		wantOK bool
	}{
		{"'work' (oficial)", false},
		{"'casa' (oficial)", true},
		{"'ds' (deepseek)", false},
		{"'dskey' (deepseek)", true},
	}
	for _, tc := range cases {
		c, ok := findCheck(checks, tc.substr)
		if !ok {
			t.Errorf("no se encontró chequeo para %q", tc.substr)
			continue
		}
		if c.OK != tc.wantOK {
			t.Errorf("chequeo %q OK = %v, quiero %v (label=%q)", tc.substr, c.OK, tc.wantOK, c.Label)
		}
	}
}
