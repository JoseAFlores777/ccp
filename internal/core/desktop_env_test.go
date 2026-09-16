package core

import (
	"strings"
	"testing"
)

// desktop_env_test.go — las barreras de una instancia de perfil.
//
// Cada test de aquí corresponde a un daño que ya ocurrió (2026-09-15), no a un
// riesgo hipotético: una instancia de perfil actualizó el Claude.app del
// usuario, un `open` heredó el proveedor de la terminal, y un `-n` incondicional
// abrió un segundo Chromium sobre el data dir real.

func TestDesktopGuardEnvExcluyeDefault(t *testing.T) {
	if got := desktopGuardEnv("default"); len(got) != 0 {
		t.Fatalf("default no debe llevar barreras, lleva %v — esa instancia ES el Claude del usuario", got)
	}
	got := desktopGuardEnv("work")
	if len(got) != 1 || got[0].Name != DesktopDisableUpdateVar || got[0].Value != "1" {
		t.Fatalf("desktopGuardEnv(work) = %v; quiero %s=1", got, DesktopDisableUpdateVar)
	}
}

func TestPlanDesktopApagaElUpdaterEnPerfil(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{"work": {Type: "official"}})
	h := DesktopHost{GOOS: "darwin", Stat: fakeStat("/Applications/Claude.app")}

	plan, err := PlanDesktop(h, "/h", "work", cfg)
	if err != nil {
		t.Fatalf("PlanDesktop: %v", err)
	}
	joined := strings.Join(plan.Args, " ")
	if !strings.Contains(joined, "--env "+DesktopDisableUpdateVar+"=1") {
		t.Errorf("la instancia de perfil debe arrancar con el updater apagado: %s", joined)
	}
	if !envHas(plan.CleanEnv, DesktopDisableUpdateVar, "1") {
		t.Errorf("y también en CleanEnv: %v", plan.CleanEnv)
	}
}

func TestPlanDesktopNoApagaElUpdaterEnDefault(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{"work": {Type: "official"}})
	h := DesktopHost{GOOS: "darwin", Stat: fakeStat("/Applications/Claude.app")}

	plan, err := PlanDesktop(h, "/h", "default", cfg)
	if err != nil {
		t.Fatalf("PlanDesktop: %v", err)
	}
	if strings.Contains(strings.Join(plan.Args, " "), DesktopDisableUpdateVar) {
		t.Error("default es el Claude del usuario: apagarle las actualizaciones sería secuestrárselo")
	}
	for _, kv := range plan.CleanEnv {
		if strings.HasPrefix(kv, DesktopDisableUpdateVar+"=") {
			t.Errorf("tampoco en CleanEnv: %s", kv)
		}
	}
}

// El `-n` incondicional abría un SEGUNDO proceso Chromium sobre el data dir real
// del usuario cada vez que se hacía `ccp desktop open` sin perfil en un
// directorio sin regla.
func TestPlanDesktopDefaultNoUsaN(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{"work": {Type: "official"}})
	h := DesktopHost{GOOS: "darwin", Stat: fakeStat("/Applications/Claude.app")}

	plan, err := PlanDesktop(h, "/h", "default", cfg)
	if err != nil {
		t.Fatalf("PlanDesktop: %v", err)
	}
	if len(plan.Args) == 0 || plan.Args[0] == "-n" {
		t.Errorf("default debe ACTIVAR la ventana que ya existe, no abrir otra: %v", plan.Args)
	}
	if plan.NewInstance {
		t.Error("NewInstance debería ser false para default sin colisión")
	}

	// Un perfil sí necesita instancia nueva: su data dir es otro.
	plan, err = PlanDesktop(h, "/h", "work", cfg)
	if err != nil {
		t.Fatalf("PlanDesktop(work): %v", err)
	}
	if len(plan.Args) == 0 || plan.Args[0] != "-n" {
		t.Errorf("un perfil sí necesita -n: %v", plan.Args)
	}
}

// La excepción que desbloquea al usuario: con una instancia de perfil ocupando
// el bundle id de Claude, `open -a` activaría ESA ventana y el usuario se
// quedaría sin poder abrir su Claude principal. Fue exactamente lo que pasó.
func TestPlanDesktopDefaultForzaNuevaSiHayInstanciaAjena(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{"work": {Type: "official"}})
	h := DesktopHost{
		GOOS: "darwin", Stat: fakeStat("/Applications/Claude.app"),
		ForeignInstance: true,
	}

	plan, err := PlanDesktop(h, "/h", "default", cfg)
	if err != nil {
		t.Fatalf("PlanDesktop: %v", err)
	}
	if len(plan.Args) == 0 || plan.Args[0] != "-n" {
		t.Errorf("con el id secuestrado, default debe forzar instancia nueva: %v", plan.Args)
	}
	if !plan.NewInstance {
		t.Error("NewInstance debería ser true para poder explicárselo al usuario")
	}
}

// `open` HEREDA el entorno de quien lo invoca y `--env` solo sobrescribe lo que
// nombra. Sin CleanEnv, `ccp desktop open <official>` desde una terminal con un
// perfil deepseek activo mandaba los prompts a otro proveedor.
func TestPlanDesktopLimpiaLasGestionadasHeredadas(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{"work": {Type: "official"}})
	h := DesktopHost{
		GOOS: "darwin", Stat: fakeStat("/Applications/Claude.app"),
		Environ: []string{
			"ANTHROPIC_BASE_URL=https://heredado.example",
			"ANTHROPIC_AUTH_TOKEN=secreto-de-otro-perfil",
			"CLAUDE_CONFIG_DIR=/h/profiles/otro/cc-home",
			"PATH=/usr/bin",
		},
	}

	plan, err := PlanDesktop(h, "/h", "work", cfg)
	if err != nil {
		t.Fatalf("PlanDesktop: %v", err)
	}
	for _, kv := range plan.CleanEnv {
		if strings.HasPrefix(kv, "ANTHROPIC_BASE_URL=") || strings.HasPrefix(kv, "ANTHROPIC_AUTH_TOKEN=") {
			t.Errorf("una gestionada del perfil de la terminal sobrevivió al lanzamiento: %s", kv)
		}
	}
	if !envHas(plan.CleanEnv, "CLAUDE_CONFIG_DIR", "/h/profiles/work/cc-home") {
		t.Errorf("el CLAUDE_CONFIG_DIR debe ser el del perfil lanzado: %v", plan.CleanEnv)
	}
	if !envHas(plan.CleanEnv, "PATH", "/usr/bin") {
		t.Error("lo NO gestionado se conserva: el hijo necesita el resto del entorno")
	}
}

func envHas(env []string, name, value string) bool {
	for _, kv := range env {
		if kv == name+"="+value {
			return true
		}
	}
	return false
}
