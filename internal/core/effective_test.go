package core

import (
	"os"
	"path/filepath"
	"testing"
)

// seedEff arma un home con un perfil y contenidos controlados en las dos capas.
func seedEff(t *testing.T, global, overlay string) (home, src, name string) {
	t.Helper()
	src = t.TempDir()
	home = t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	name = "work"
	if err := ProfileAddOfficial(home, name); err != nil {
		t.Fatalf("ProfileAddOfficial: %v", err)
	}
	// ProfileAddOfficial solo crea profiles/<name>/cc-home/ (seedCCHome); el
	// directorio overlay/ lo crea CfgInitOverlay, no ProfileAddOfficial. Sin
	// esto, el os.WriteFile de abajo falla con ENOENT.
	if err := CfgInitOverlay(home, name); err != nil {
		t.Fatalf("CfgInitOverlay: %v", err)
	}
	if global != "" {
		if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(global), 0o644); err != nil {
			t.Fatalf("global: %v", err)
		}
	}
	if overlay != "" {
		if err := os.WriteFile(cfgSettingsFile(home, name), []byte(overlay), 0o600); err != nil {
			t.Fatalf("overlay: %v", err)
		}
	}
	return home, src, name
}

func sectionOf(t *testing.T, e Effective, k EffKind) EffSection {
	t.Helper()
	for _, s := range e.Sections {
		if s.Kind == k {
			return s
		}
	}
	t.Fatalf("falta la sección %v", k)
	return EffSection{}
}

func TestEffectiveEnvMarcaLaCapaQueGana(t *testing.T) {
	home, src, name := seedEff(t,
		`{"env":{"SOLO_GLOBAL":"g","AMBAS":"g"}}`,
		`{"env":{"SOLO_OVERLAY":"o","AMBAS":"o"}}`)
	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("ProfileEffective: %v", err)
	}
	got := map[string]EffRow{}
	for _, r := range sectionOf(t, e, EffEnv).Rows {
		got[r.Key] = r
	}
	if got["SOLO_GLOBAL"].Origin != OriginGlobal || got["SOLO_GLOBAL"].Shadowed {
		t.Errorf("SOLO_GLOBAL mal: %+v", got["SOLO_GLOBAL"])
	}
	if got["SOLO_OVERLAY"].Origin != OriginOverlay || got["SOLO_OVERLAY"].Shadowed {
		t.Errorf("SOLO_OVERLAY mal: %+v", got["SOLO_OVERLAY"])
	}
	if r := got["AMBAS"]; r.Origin != OriginOverlay || !r.Shadowed || r.Value != "o" {
		t.Errorf("AMBAS tiene que ganar el overlay y quedar Shadowed: %+v", r)
	}
}

func TestEffectivePermisosElArrayLoReemplazaElOverlay(t *testing.T) {
	// MergeJSON REEMPLAZA arrays; no los concatena. La procedencia tiene que
	// contar esa verdad y no una unión que no ocurre.
	home, src, name := seedEff(t,
		`{"permissions":{"allow":["Bash(ls)","Bash(cat)"]}}`,
		`{"permissions":{"allow":["Bash(rg)"]}}`)
	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("ProfileEffective: %v", err)
	}
	rows := sectionOf(t, e, EffPermissions).Rows
	if len(rows) != 1 || rows[0].Key != "Bash(rg)" {
		t.Fatalf("el array del overlay reemplaza al global: %+v", rows)
	}
	if rows[0].Origin != OriginOverlay || !rows[0].Shadowed {
		t.Fatalf("la fila tiene que decir que el global perdió: %+v", rows[0])
	}
}

func TestEffectiveOverlayRotoEsErrorDeSeccionNoDeTodo(t *testing.T) {
	home, src, name := seedEff(t, `{"env":{"G":"1"}}`, `{roto`)
	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("un overlay roto no puede tumbar ProfileEffective: %v", err)
	}
	if sectionOf(t, e, EffEnv).Err == nil {
		t.Error("la sección Env tiene que llevar el error del overlay")
	}
	if sectionOf(t, e, EffInstructions).Err != nil {
		t.Error("Instrucciones vive en otro archivo: no puede heredar ese error")
	}
}

func TestEffectiveDefaultNoTieneOverlay(t *testing.T) {
	home, src, _ := seedEff(t, `{"env":{"G":"1"}}`, "")
	e, err := ProfileEffective(home, "default", src)
	if err != nil {
		t.Fatalf("'default' tiene que abrirse, no fallar: %v", err)
	}
	env := sectionOf(t, e, EffEnv)
	if env.File != "" {
		t.Errorf("'default' no tiene archivo de overlay: %q", env.File)
	}
	if len(env.Rows) != 1 || env.Rows[0].Origin != OriginGlobal {
		t.Errorf("todo lo de 'default' viene del global: %+v", env.Rows)
	}
}

func TestEffectiveSinGlobalNoFalla(t *testing.T) {
	home, src, name := seedEff(t, "", `{"env":{"O":"1"}}`)
	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("sin settings.json global no se falla: %v", err)
	}
	rows := sectionOf(t, e, EffEnv).Rows
	if len(rows) != 1 || rows[0].Origin != OriginOverlay {
		t.Errorf("solo overlay: %+v", rows)
	}
}

// TestEffectiveHooksYSensoresConAutoHandoffInstalado fija el caso que el
// mockup del spec usa como ejemplo canónico ("Hooks  11 eventos  overlay ⊕
// auto"): con la capa de sensores instalada, StopFailure tiene que aparecer
// con Origin auto (lo instala AutoHooksFragment vía applyAutoLayer), y la
// sección Sensores tiene que marcar el perfil.
func TestEffectiveHooksYSensoresConAutoHandoffInstalado(t *testing.T) {
	home, src, name := seedEff(t, "", "")
	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.AutoHandoff = &AutoHandoff{Hooks: []string{name}}
	if err := Save(home, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	SetAutoHooksBin("ccp") // determinista: no depende de os.Executable()

	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("ProfileEffective: %v", err)
	}
	var stopFailure *EffRow
	for i, r := range sectionOf(t, e, EffHooks).Rows {
		if r.Key == "StopFailure" {
			stopFailure = &sectionOf(t, e, EffHooks).Rows[i]
		}
	}
	if stopFailure == nil || stopFailure.Origin != OriginAuto {
		t.Fatalf("StopFailure tiene que venir de la capa auto: %+v", sectionOf(t, e, EffHooks).Rows)
	}
	sensors := sectionOf(t, e, EffSensors).Rows
	if len(sensors) != 1 || sensors[0].Origin != OriginAuto {
		t.Fatalf("Sensores tiene que marcar el perfil instalado: %+v", sensors)
	}
}

// TestEffectiveHooksConservaElStopFailureAjenoDelOverlay fija el motivo real de
// HUECO 2: applyAutoLayer no reemplaza el StopFailure del overlay, lo CONSERVA
// detrás del suyo (keepForeignStopFailure, autohooks.go:319) porque MergeJSON
// reemplaza arrays. Si effHookRows solo mirara el fragmento crudo de
// AutoHooksFragment (en vez del resultado REAL de applyAutoLayer), contaría 1
// en vez de 2 y perdería el StopFailure del usuario en el recuento.
func TestEffectiveHooksConservaElStopFailureAjenoDelOverlay(t *testing.T) {
	overlay := `{"hooks":{"StopFailure":[{"matcher":"","hooks":[{"type":"command","command":"echo propio"}]}]}}`
	home, src, name := seedEff(t, "", overlay)
	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.AutoHandoff = &AutoHandoff{Hooks: []string{name}}
	if err := Save(home, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	SetAutoHooksBin("ccp")

	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("ProfileEffective: %v", err)
	}
	for _, r := range sectionOf(t, e, EffHooks).Rows {
		if r.Key == "StopFailure" {
			if r.Value != "2" {
				t.Fatalf("StopFailure tiene que contar el propio + el que instala la capa (2): value=%s", r.Value)
			}
			return
		}
	}
	t.Fatal("falta la fila StopFailure")
}
