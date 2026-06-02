package core

import "testing"

func TestGetDefaults_EmptyHomeReturnsBuiltin(t *testing.T) {
	home := t.TempDir()
	d, err := GetDefaults(home)
	if err != nil {
		t.Fatalf("GetDefaults: %v", err)
	}
	if d != BuiltinDefaults() {
		t.Errorf("GetDefaults = %+v, quiero builtin %+v", d, BuiltinDefaults())
	}
}

func TestSetDefault_PersistsAndRoundTrips(t *testing.T) {
	home := t.TempDir()

	if err := SetDefault(home, "model_pro", "deepseek-reasoner"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	if err := SetDefault(home, "base_url", "https://example.com/anthropic"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}

	d, err := GetDefaults(home)
	if err != nil {
		t.Fatalf("GetDefaults: %v", err)
	}
	if d.ModelPro != "deepseek-reasoner" {
		t.Errorf("ModelPro = %q, quiero deepseek-reasoner", d.ModelPro)
	}
	if d.BaseURL != "https://example.com/anthropic" {
		t.Errorf("BaseURL = %q", d.BaseURL)
	}
	// Los campos no tocados se completan con builtin en show.
	if d.ModelFlash != "deepseek-chat" {
		t.Errorf("ModelFlash = %q, quiero builtin deepseek-chat", d.ModelFlash)
	}
}

func TestSetDefault_SeedsSiblingProviderFields(t *testing.T) {
	home := t.TempDir()
	// Tras setear solo 'effort', los campos hermanos deben quedar persistidos
	// con built-ins (no strings vacíos); editor permanece vacío.
	if err := SetDefault(home, "effort", "low"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	c, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	b := BuiltinDefaults()
	if c.Defaults.BaseURL != b.BaseURL || c.Defaults.ModelPro != b.ModelPro || c.Defaults.ModelFlash != b.ModelFlash {
		t.Errorf("campos hermanos no sembrados: %+v", c.Defaults)
	}
	if c.Defaults.Editor != "" {
		t.Errorf("editor = %q, debe seguir vacío", c.Defaults.Editor)
	}
}

func TestSetDefault_RejectsUnknownKeyAndEmpty(t *testing.T) {
	home := t.TempDir()
	if err := SetDefault(home, "nope", "x"); err == nil {
		t.Error("quiero error para clave desconocida")
	}
	if err := SetDefault(home, "effort", ""); err == nil {
		t.Error("quiero error para valor vacío")
	}
	if err := SetDefault(home, "", "x"); err == nil {
		t.Error("quiero error para clave vacía")
	}
}

func TestSetDefault_DoesNotMutateExistingProfile(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	// Crea un perfil deepseek desde los defaults built-in.
	if err := ProfileAddDeepseek(home, "ds", BuiltinDefaults()); err != nil {
		t.Fatalf("ProfileAddDeepseek: %v", err)
	}

	// Cambiar los defaults NO debe tocar el perfil existente.
	if err := SetDefault(home, "model_pro", "deepseek-reasoner"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}

	c, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Profiles["ds"].ModelPro != "deepseek-chat" {
		t.Errorf("perfil 'ds' ModelPro mutó a %q; debe seguir deepseek-chat", c.Profiles["ds"].ModelPro)
	}
	if c.Defaults.ModelPro != "deepseek-reasoner" {
		t.Errorf("defaults ModelPro = %q, quiero deepseek-reasoner", c.Defaults.ModelPro)
	}
}

func TestSetEditorAndGetEditor(t *testing.T) {
	home := t.TempDir()

	// Sin editor configurado: cae a envEditor si está, si no nano.
	if ed, _ := GetEditor(home, "vim"); ed != "vim" {
		t.Errorf("GetEditor(envEditor=vim) = %q, quiero vim", ed)
	}
	if ed, _ := GetEditor(home, ""); ed != "nano" {
		t.Errorf("GetEditor(envEditor=\"\") = %q, quiero nano", ed)
	}

	// Editor configurado gana sobre $EDITOR.
	if err := SetEditor(home, "code -w"); err != nil {
		t.Fatalf("SetEditor: %v", err)
	}
	if ed, _ := GetEditor(home, "vim"); ed != "code -w" {
		t.Errorf("GetEditor = %q, quiero 'code -w'", ed)
	}
	if err := SetEditor(home, ""); err == nil {
		t.Error("quiero error para editor vacío")
	}
}

func TestResetDefaults_RestoresBuiltin(t *testing.T) {
	home := t.TempDir()
	if err := SetDefault(home, "effort", "low"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	if err := SetEditor(home, "vim"); err != nil {
		t.Fatalf("SetEditor: %v", err)
	}

	if err := ResetDefaults(home); err != nil {
		t.Fatalf("ResetDefaults: %v", err)
	}

	d, err := GetDefaults(home)
	if err != nil {
		t.Fatalf("GetDefaults: %v", err)
	}
	if d != BuiltinDefaults() {
		t.Errorf("tras reset = %+v, quiero builtin %+v", d, BuiltinDefaults())
	}
}

func TestResetDefaults_KeepsProfilesAndRules(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := ProfileAddDeepseek(home, "ds", BuiltinDefaults()); err != nil {
		t.Fatalf("ProfileAddDeepseek: %v", err)
	}
	if err := ResetDefaults(home); err != nil {
		t.Fatalf("ResetDefaults: %v", err)
	}
	c, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := c.Profiles["ds"]; !ok {
		t.Error("reset borró el perfil 'ds'")
	}
}
