package core

import "testing"

// seedProfile crea un ccp.yaml mínimo con un perfil para que RuleSet pueda
// validar existencia.
func seedProfileForRules(t *testing.T, home, name, typ string) {
	t.Helper()
	c := &Config{Version: SchemaVersion, Profiles: map[string]Profile{name: {Type: typ}}}
	if err := Save(home, c); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestRuleSetUpsertAndResolve(t *testing.T) {
	home := t.TempDir()
	seedProfileForRules(t, home, "work", "official")

	if _, err := RuleSet(home, "/tmp/a/b", "work"); err != nil {
		t.Fatalf("RuleSet: %v", err)
	}
	// Reemplazo in-place: mismo path, otro perfil (default).
	if _, err := RuleSet(home, "/tmp/a/b", "default"); err != nil {
		t.Fatalf("RuleSet replace: %v", err)
	}
	rules, err := RulesList(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("esperaba 1 regla tras upsert, hay %d", len(rules))
	}
	if got := Resolve("/tmp/a/b/c", rules); got != "default" {
		t.Fatalf("Resolve tras replace = %q, want default", got)
	}
}

func TestRuleSetRejectsUnknownProfile(t *testing.T) {
	home := t.TempDir()
	seedProfileForRules(t, home, "work", "official")
	if _, err := RuleSet(home, "/tmp/x", "nope"); err == nil {
		t.Fatal("esperaba error con perfil inexistente")
	}
}

func TestRuleDelAndClear(t *testing.T) {
	home := t.TempDir()
	seedProfileForRules(t, home, "work", "official")
	_, _ = RuleSet(home, "/tmp/a", "work")
	_, _ = RuleSet(home, "/tmp/b", "work")

	if _, err := RuleDel(home, "/tmp/a"); err != nil {
		t.Fatalf("RuleDel: %v", err)
	}
	rules, _ := RulesList(home)
	if len(rules) != 1 || rules[0].Path != "/tmp/b" {
		t.Fatalf("RuleDel dejó %v", rules)
	}
	// Idempotente: borrar algo inexistente no es error.
	if _, err := RuleDel(home, "/tmp/no"); err != nil {
		t.Fatalf("RuleDel inexistente debería ser no-op: %v", err)
	}
	if err := RulesClear(home); err != nil {
		t.Fatalf("RulesClear: %v", err)
	}
	rules, _ = RulesList(home)
	if len(rules) != 0 {
		t.Fatalf("RulesClear dejó %d reglas", len(rules))
	}
}
